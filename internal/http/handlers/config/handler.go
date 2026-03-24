package config

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/tenant"
	"github.com/bengobox/ordering-backend/internal/http/handlers"
)

// Handler serves public tenant/brand config (no auth required).
type Handler struct {
	log *zap.Logger
	db  *ent.Client
}

// New constructs a config handler.
func New(log *zap.Logger, db *ent.Client) *Handler {
	return &Handler{log: log.Named("config.Handler"), db: db}
}

// PublicConfigResponse is the public tenant/brand config returned by GET /config.
type PublicConfigResponse struct {
	Name          string            `json:"name"`
	ShortName     string            `json:"short_name"`
	LogoURL       string            `json:"logo_url,omitempty"`
	PrimaryColor  string            `json:"primary_color,omitempty"`
	SecondaryColor string           `json:"secondary_color,omitempty"`
	SupportEmail  string            `json:"support_email,omitempty"`
	SupportPhone  string            `json:"support_phone,omitempty"`
	Tagline       string            `json:"tagline,omitempty"`
	BrandPalette  map[string]string `json:"brand_palette,omitempty"`
	Features      map[string]bool   `json:"features,omitempty"`
}

// GetConfig returns public tenant display name and brand (logo, colors) for the tenant in the URL.
// Public endpoint: no authentication required.
// @Summary Get tenant public config (brand)
// @Description Returns tenant display name and optional brand (logo_url, primary_color, secondary_color) for theming
// @Tags Config
// @Produce json
// @Param tenant path string true "Tenant slug (e.g. urban-loft)"
// @Success 200 {object} config.PublicConfigResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Router /config [get]
func (h *Handler) GetConfig(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "tenant")
	if slug == "" {
		handlers.RespondError(w, http.StatusBadRequest, "tenant slug required")
		return
	}

	ctx := r.Context()
	t, err := h.db.Tenant.Query().
		Where(tenant.SlugEQ(slug)).
		WithSettings().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			handlers.RespondError(w, http.StatusNotFound, "tenant not found")
			return
		}
		h.log.Error("get tenant config", zap.String("slug", slug), zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Tenant branding (logo, colors, contact info) is owned by auth-api.
	// This endpoint returns only locally-available fields (name, slug) plus
	// ordering-specific settings (features). Branding data should be fetched
	// from auth-api GET /api/v1/tenants/by-slug/{slug} by the frontend and
	// cached in TanStack Query with JWT TTL.
	resp := PublicConfigResponse{
		Name:      t.Name,
		ShortName: t.Slug,
	}
	if resp.Name == "" {
		resp.Name = t.Slug
	}
	if resp.ShortName == "" {
		resp.ShortName = resp.Name
	}

	// Ordering-specific features from settings (service-owned data)
	if t.Edges.Settings != nil {
		st := t.Edges.Settings
		if st.Features != nil {
			resp.Features = make(map[string]bool)
			for k, v := range st.Features {
				if b, ok := v.(bool); ok {
					resp.Features[k] = b
				}
			}
		}
	}

	handlers.RespondJSON(w, http.StatusOK, resp)
}
