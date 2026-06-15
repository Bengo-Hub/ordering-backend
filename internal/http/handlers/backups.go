package handlers

import (
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	httpware "github.com/Bengo-Hub/httpware"

	"github.com/bengobox/ordering-backend/internal/modules/backup"
)

// BackupsHandler serves tenant-scoped backups (this tenant's data only). The artifact is a
// gzipped-JSON export of the tenant's rows, tracked in the `backups` table. Permission
// gating (ordering.config.view / config.manage) is applied by the router.
type BackupsHandler struct {
	log           *zap.Logger
	svc           *backup.Service
	retentionDays int
}

// NewBackupsHandler builds the handler.
func NewBackupsHandler(log *zap.Logger, svc *backup.Service, retentionDays int) *BackupsHandler {
	if retentionDays <= 0 {
		retentionDays = backup.DefaultRetentionDays
	}
	return &BackupsHandler{log: log.Named("backups"), svc: svc, retentionDays: retentionDays}
}

// Register mounts the backup endpoints on the given (already tenant-scoped) router under
// /backups.
func (h *BackupsHandler) Register(r chi.Router) {
	r.Route("/backups", func(br chi.Router) {
		br.Get("/", h.List)
		br.Post("/", h.Create)
		br.Post("/churn", h.Churn)
		br.Get("/{name}/download", h.Download)
		br.Delete("/{name}", h.Delete)
	})
}

// resolveTenant extracts the tenant UUID from context (TenantV2 / JIT-sync backfill), with
// a platform-owner ?tenantId= override.
func resolveTenant(r *http.Request) (uuid.UUID, bool) {
	ctx := r.Context()
	if httpware.IsPlatformOwner(ctx) {
		if q := r.URL.Query().Get("tenantId"); q != "" {
			if id, err := uuid.Parse(q); err == nil {
				return id, true
			}
		}
	}
	if s := httpware.GetTenantID(ctx); s != "" {
		if id, err := uuid.Parse(s); err == nil && id != uuid.Nil {
			return id, true
		}
	}
	if p := chi.URLParam(r, "tenant"); p != "" {
		if id, err := uuid.Parse(p); err == nil && id != uuid.Nil {
			return id, true
		}
	}
	return uuid.Nil, false
}

func (h *BackupsHandler) List(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := resolveTenant(r)
	if !ok {
		RespondError(w, http.StatusBadRequest, "tenant context required")
		return
	}
	items, err := h.svc.List(r.Context(), tenantID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list backups")
		return
	}
	RespondJSON(w, http.StatusOK, map[string]any{"backups": items})
}

func (h *BackupsHandler) Create(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := resolveTenant(r)
	if !ok {
		RespondError(w, http.StatusBadRequest, "tenant context required")
		return
	}
	info, err := h.svc.Generate(r.Context(), tenantID)
	if err != nil {
		h.log.Error("generate backup", zap.Error(err))
		RespondError(w, http.StatusInternalServerError, "failed to generate backup")
		return
	}
	RespondJSON(w, http.StatusCreated, info)
}

// Churn manually triggers retention cleanup (deletes files + tracking rows older than the
// configured retention window across the cluster). The daily scheduler runs this too.
func (h *BackupsHandler) Churn(w http.ResponseWriter, r *http.Request) {
	removed, err := h.svc.Churn(r.Context(), h.retentionDays)
	if err != nil {
		h.log.Error("churn backups", zap.Error(err))
		RespondError(w, http.StatusInternalServerError, "failed to churn backups")
		return
	}
	RespondJSON(w, http.StatusOK, map[string]any{"removed": removed, "retention_days": h.retentionDays})
}

func (h *BackupsHandler) Download(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := resolveTenant(r)
	if !ok {
		RespondError(w, http.StatusBadRequest, "tenant context required")
		return
	}
	name := chi.URLParam(r, "name")
	rc, err := h.svc.Open(tenantID, name)
	if err != nil {
		RespondError(w, http.StatusNotFound, "backup not found")
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, name))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}

func (h *BackupsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := resolveTenant(r)
	if !ok {
		RespondError(w, http.StatusBadRequest, "tenant context required")
		return
	}
	if err := h.svc.Delete(r.Context(), tenantID, chi.URLParam(r, "name")); err != nil {
		RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
