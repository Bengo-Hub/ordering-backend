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

	resp := PublicConfigResponse{
		Name:         t.Name,
		ShortName:    t.Slug,
		SupportEmail: t.ContactEmail,
		SupportPhone: t.ContactPhone,
	}
	if resp.Name == "" {
		resp.Name = t.Slug
	}
	if resp.ShortName == "" {
		resp.ShortName = resp.Name
	}

	// Logo from tenant metadata if present
	if t.Metadata != nil {
		if v, ok := t.Metadata["logo_url"].(string); ok && v != "" {
			resp.LogoURL = v
		}
	}

	// Brand palette and features from settings
	if t.Edges.Settings != nil {
		st := t.Edges.Settings
		if st.BrandPalette != nil {
			resp.BrandPalette = make(map[string]string)
			for k, v := range st.BrandPalette {
				if s, ok := v.(string); ok {
					resp.BrandPalette[k] = s
				}
			}
			if c, ok := resp.BrandPalette["primary"]; ok {
				resp.PrimaryColor = c
			}
			if c, ok := resp.BrandPalette["secondary"]; ok {
				resp.SecondaryColor = c
			}
		}
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
