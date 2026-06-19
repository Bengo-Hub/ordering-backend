package config

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/serviceconfig"
	"github.com/bengobox/ordering-backend/internal/http/handlers"
	"github.com/bengobox/ordering-backend/internal/modules/googlebusiness"
)

// EncryptionKeyHandler exposes platform-owner endpoints to inspect and rotate the
// credential-encryption key (AES-256-GCM) stored as a platform-level ServiceConfig
// (config_key="encryption_key", tenant_id IS NULL).
//
// It NEVER returns raw key material — only a non-reversible fingerprint, the source,
// and metadata. After a write it invalidates the shared KeyProvider cache so the new
// key takes effect immediately.
type EncryptionKeyHandler struct {
	client *ent.Client
	keys   *googlebusiness.KeyProvider
	logger *zap.Logger
}

// NewEncryptionKeyHandler constructs the handler with the ent client and the shared
// KeyProvider (the same instance used for credential encryption at rest).
func NewEncryptionKeyHandler(client *ent.Client, keys *googlebusiness.KeyProvider, logger *zap.Logger) *EncryptionKeyHandler {
	return &EncryptionKeyHandler{
		client: client,
		keys:   keys,
		logger: logger.Named("encryption-key"),
	}
}

// encryptionKeyStatus is the response shape for both GET and PUT. It deliberately omits
// the raw key — only a fingerprint is exposed.
type encryptionKeyStatus struct {
	Configured     bool    `json:"configured"`
	Source         string  `json:"source"`
	KeyFingerprint string  `json:"key_fingerprint"`
	UpdatedAt      *string `json:"updated_at"`
}

// buildStatus assembles the public status from the KeyProvider, never touching key bytes.
func (h *EncryptionKeyHandler) buildStatus(r *http.Request) encryptionKeyStatus {
	ctx := r.Context()
	source := string(h.keys.PrimarySource(ctx))
	st := encryptionKeyStatus{
		Configured:     source != "",
		Source:         source,
		KeyFingerprint: h.keys.Fingerprint(ctx),
	}
	if upd := h.keys.DBUpdatedAt(ctx); upd != nil {
		s := upd.UTC().Format(time.RFC3339)
		st.UpdatedAt = &s
	}
	return st
}

// GetEncryptionKey reports the current PRIMARY key status.
// GET /api/v1/{tenant}/admin/encryption-key  (platform-owner gated)
func (h *EncryptionKeyHandler) GetEncryptionKey(w http.ResponseWriter, r *http.Request) {
	handlers.RespondJSON(w, http.StatusOK, h.buildStatus(r))
}

// PutEncryptionKey rotates the platform credential-encryption key.
// PUT /api/v1/{tenant}/admin/encryption-key  (platform-owner gated)
//
// Body: {"key"?: "<base64 of exactly 32 bytes>", "generate"?: bool}.
// generate==true or an empty key → 32 crypto/rand bytes. A provided key must
// base64-decode to exactly 32 bytes else 400. The key is upserted as a platform-level
// ServiceConfig (is_secret=true) and the KeyProvider cache is invalidated. The raw key
// is NEVER returned or logged.
func (h *EncryptionKeyHandler) PutEncryptionKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key      string `json:"key"`
		Generate bool   `json:"generate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var keyBytes []byte
	if req.Generate || req.Key == "" {
		gen, err := googlebusiness.GenerateKey()
		if err != nil {
			h.logger.Error("failed to generate encryption key", zap.Error(err))
			handlers.RespondError(w, http.StatusInternalServerError, "failed to generate key")
			return
		}
		keyBytes = gen
	} else {
		decoded, err := base64.StdEncoding.DecodeString(req.Key)
		if err != nil || len(decoded) != 32 {
			handlers.RespondError(w, http.StatusBadRequest, "key must be base64 of exactly 32 bytes")
			return
		}
		keyBytes = decoded
	}

	encoded := base64.StdEncoding.EncodeToString(keyBytes)
	ctx := r.Context()

	existing, err := h.client.ServiceConfig.Query().
		Where(
			serviceconfig.ConfigKey(googlebusiness.EncryptionKeyConfigKey),
			serviceconfig.TenantIDIsNil(),
		).
		First(ctx)
	switch {
	case err == nil:
		_, uerr := existing.Update().
			SetConfigValue(encoded).
			SetConfigType("string").
			SetIsSecret(true).
			SetDescription("AES-256-GCM key for credential encryption at rest").
			Save(ctx)
		if uerr != nil {
			h.logger.Error("failed to update encryption key config", zap.Error(uerr))
			handlers.RespondError(w, http.StatusInternalServerError, "failed to save encryption key")
			return
		}
	case ent.IsNotFound(err):
		_, cerr := h.client.ServiceConfig.Create().
			SetConfigKey(googlebusiness.EncryptionKeyConfigKey).
			SetConfigValue(encoded).
			SetConfigType("string").
			SetIsSecret(true).
			SetDescription("AES-256-GCM key for credential encryption at rest").
			Save(ctx)
		if cerr != nil {
			h.logger.Error("failed to create encryption key config", zap.Error(cerr))
			handlers.RespondError(w, http.StatusInternalServerError, "failed to save encryption key")
			return
		}
	default:
		h.logger.Error("failed to query encryption key config", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "failed to save encryption key")
		return
	}

	// Make the new key take effect immediately for encrypt/decrypt.
	h.keys.Invalidate()

	h.logger.Info("platform credential-encryption key rotated")
	handlers.RespondJSON(w, http.StatusOK, h.buildStatus(r))
}
