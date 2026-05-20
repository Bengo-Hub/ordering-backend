package config

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/serviceconfig"
	"github.com/bengobox/ordering-backend/internal/http/handlers"
)

// ServiceConfigHandler handles service-level configuration CRUD for ordering-backend.
type ServiceConfigHandler struct {
	client *ent.Client
	logger *zap.Logger
}

// NewServiceConfigHandler creates a new ServiceConfigHandler.
func NewServiceConfigHandler(client *ent.Client, logger *zap.Logger) *ServiceConfigHandler {
	return &ServiceConfigHandler{
		client: client,
		logger: logger.Named("service-config"),
	}
}

type orderingSCResponse struct {
	ID          uuid.UUID  `json:"id"`
	TenantID    *uuid.UUID `json:"tenant_id,omitempty"`
	ConfigKey   string     `json:"config_key"`
	ConfigValue string     `json:"config_value"`
	ConfigType  string     `json:"config_type"`
	Description string     `json:"description,omitempty"`
	IsSecret    bool       `json:"is_secret"`
	IsOverride  bool       `json:"is_override"`
	CreatedAt   string     `json:"created_at"`
	UpdatedAt   string     `json:"updated_at"`
}

func toOrderingSCResponse(cfg *ent.ServiceConfig, isOverride bool) orderingSCResponse {
	val := cfg.ConfigValue
	if cfg.IsSecret {
		val = "***"
	}
	return orderingSCResponse{
		ID:          cfg.ID,
		TenantID:    cfg.TenantID,
		ConfigKey:   cfg.ConfigKey,
		ConfigValue: val,
		ConfigType:  cfg.ConfigType,
		Description: cfg.Description,
		IsSecret:    cfg.IsSecret,
		IsOverride:  isOverride,
		CreatedAt:   cfg.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   cfg.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// ListPlatformSettings returns all platform-level (tenant_id=nil) service configs.
// GET /api/v1/{tenant}/admin/service-config
func (h *ServiceConfigHandler) ListPlatformSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	configs, err := h.client.ServiceConfig.Query().
		Where(serviceconfig.TenantIDIsNil()).
		All(ctx)
	if err != nil {
		h.logger.Error("failed to list platform configs", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "failed to list platform settings")
		return
	}

	result := make([]orderingSCResponse, 0, len(configs))
	for _, cfg := range configs {
		result = append(result, orderingSCResponse{
			ID:          cfg.ID,
			TenantID:    cfg.TenantID,
			ConfigKey:   cfg.ConfigKey,
			ConfigValue: cfg.ConfigValue, // unmasked for platform admin
			ConfigType:  cfg.ConfigType,
			Description: cfg.Description,
			IsSecret:    cfg.IsSecret,
			IsOverride:  false,
			CreatedAt:   cfg.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:   cfg.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	handlers.RespondJSON(w, http.StatusOK, map[string]any{"data": result, "total": len(result)})
}

// UpsertPlatformSetting updates a platform-level config by key.
// PUT /api/v1/{tenant}/admin/service-config/{key}
func (h *ServiceConfigHandler) UpsertPlatformSetting(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		handlers.RespondError(w, http.StatusBadRequest, "config key is required")
		return
	}

	var req struct {
		ConfigValue string `json:"config_value"`
		Description string `json:"description,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ConfigValue == "" {
		handlers.RespondError(w, http.StatusBadRequest, "config_value is required")
		return
	}

	ctx := r.Context()

	existing, err := h.client.ServiceConfig.Query().
		Where(
			serviceconfig.ConfigKey(key),
			serviceconfig.TenantIDIsNil(),
		).
		First(ctx)
	if err != nil {
		handlers.RespondError(w, http.StatusNotFound, "platform default not found for key: "+key)
		return
	}

	update := existing.Update().SetConfigValue(req.ConfigValue)
	if req.Description != "" {
		update = update.SetDescription(req.Description)
	}

	cfg, err := update.Save(ctx)
	if err != nil {
		h.logger.Error("failed to update platform config", zap.Error(err), zap.String("key", key))
		handlers.RespondError(w, http.StatusInternalServerError, "failed to update platform setting")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, toOrderingSCResponse(cfg, false))
}

// ListTenantSettings returns merged tenant+platform service configs.
// GET /api/v1/{tenant}/settings/service-config
func (h *ServiceConfigHandler) ListTenantSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tenantIDStr := chi.URLParam(r, "tenant")
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant ID")
		return
	}

	platformConfigs, err := h.client.ServiceConfig.Query().
		Where(serviceconfig.TenantIDIsNil()).
		All(ctx)
	if err != nil {
		h.logger.Error("failed to list platform configs", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "failed to list settings")
		return
	}

	tenantConfigs, err := h.client.ServiceConfig.Query().
		Where(serviceconfig.TenantID(tenantID)).
		All(ctx)
	if err != nil {
		h.logger.Error("failed to list tenant configs", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "failed to list settings")
		return
	}

	tenantByKey := make(map[string]*ent.ServiceConfig, len(tenantConfigs))
	for _, tc := range tenantConfigs {
		tenantByKey[tc.ConfigKey] = tc
	}

	result := make([]orderingSCResponse, 0, len(platformConfigs))
	for _, pc := range platformConfigs {
		if tc, ok := tenantByKey[pc.ConfigKey]; ok {
			result = append(result, toOrderingSCResponse(tc, true))
			delete(tenantByKey, pc.ConfigKey)
		} else {
			result = append(result, toOrderingSCResponse(pc, false))
		}
	}
	for _, tc := range tenantByKey {
		result = append(result, toOrderingSCResponse(tc, true))
	}

	handlers.RespondJSON(w, http.StatusOK, map[string]any{"data": result, "total": len(result)})
}

// UpsertTenantSetting creates or updates a tenant-specific config override.
// PUT /api/v1/{tenant}/settings/service-config/{key}
func (h *ServiceConfigHandler) UpsertTenantSetting(w http.ResponseWriter, r *http.Request) {
	tenantIDStr := chi.URLParam(r, "tenant")
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant ID")
		return
	}

	key := chi.URLParam(r, "key")
	if key == "" {
		handlers.RespondError(w, http.StatusBadRequest, "config key is required")
		return
	}

	var req struct {
		ConfigValue string `json:"config_value"`
		Description string `json:"description,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ConfigValue == "" {
		handlers.RespondError(w, http.StatusBadRequest, "config_value is required")
		return
	}

	ctx := r.Context()

	platformDefault, _ := h.client.ServiceConfig.Query().
		Where(serviceconfig.ConfigKey(key), serviceconfig.TenantIDIsNil()).
		First(ctx)

	configType := "string"
	if platformDefault != nil {
		configType = platformDefault.ConfigType
	}

	existing, _ := h.client.ServiceConfig.Query().
		Where(serviceconfig.ConfigKey(key), serviceconfig.TenantID(tenantID)).
		First(ctx)

	var cfg *ent.ServiceConfig
	if existing != nil {
		cfg, err = existing.Update().SetConfigValue(req.ConfigValue).Save(ctx)
	} else {
		create := h.client.ServiceConfig.Create().
			SetTenantID(tenantID).
			SetConfigKey(key).
			SetConfigValue(req.ConfigValue).
			SetConfigType(configType)
		if req.Description != "" {
			create = create.SetDescription(req.Description)
		} else if platformDefault != nil && platformDefault.Description != "" {
			create = create.SetDescription(platformDefault.Description)
		}
		if platformDefault != nil {
			create = create.SetIsSecret(platformDefault.IsSecret)
		}
		cfg, err = create.Save(ctx)
	}

	if err != nil {
		h.logger.Error("failed to upsert tenant config", zap.Error(err), zap.String("key", key))
		handlers.RespondError(w, http.StatusInternalServerError, "failed to save setting")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, toOrderingSCResponse(cfg, true))
}
