package config

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	cache "github.com/Bengo-Hub/cache"
	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/tenant"
	"github.com/bengobox/ordering-backend/internal/http/handlers"
)

// Handler serves public tenant/brand config (no auth required).
type Handler struct {
	log     *zap.Logger
	db      *ent.Client
	cache   *cache.Aside
	authURL string
}

// New constructs a config handler.
// cache and authURL enable loading tenant branding from the shared Redis cache
// (populated by auth-api). Pass nil cache to disable branding enrichment.
func New(log *zap.Logger, db *ent.Client, c *cache.Aside, authURL string) *Handler {
	return &Handler{log: log.Named("config.Handler"), db: db, cache: c, authURL: authURL}
}

// PublicConfigResponse is the public tenant/brand config returned by GET /config.
// This is the single source of truth ordering-frontend uses for ALL tenant display
// concerns (name, logo, colors, and use_case-driven layout selection) — see
// useOrderingConfig/useBrandConfig, which replaced a second, independently-broken
// tenant fetch (shared-ui-lib's TenantBrandingProvider, mounted above the org-slug
// route segment so its slug context was permanently empty).
type PublicConfigResponse struct {
	Name           string            `json:"name"`
	ShortName      string            `json:"short_name"`
	LogoURL        string            `json:"logo_url,omitempty"`
	PrimaryColor   string            `json:"primary_color,omitempty"`
	SecondaryColor string            `json:"secondary_color,omitempty"`
	SupportEmail   string            `json:"support_email,omitempty"`
	SupportPhone   string            `json:"support_phone,omitempty"`
	Tagline        string            `json:"tagline,omitempty"`
	BrandPalette   map[string]string `json:"brand_palette,omitempty"`
	Features       map[string]bool   `json:"features,omitempty"`
	// UseCase/UseCases drive the storefront's per-vertical layout (hospitality vs
	// retail vs services, etc.) — see normalizeOrderingUseCase on the frontend.
	UseCase  string   `json:"use_case,omitempty"`
	UseCases []string `json:"use_cases,omitempty"`
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
		Name:      t.Name,
		ShortName: t.Slug,
	}
	if resp.Name == "" {
		resp.Name = t.Slug
	}
	if resp.ShortName == "" {
		resp.ShortName = resp.Name
	}

	// Enrich with branding from Redis-cached auth-api tenant data.
	if h.cache != nil && h.authURL != "" {
		details, err := cache.GetTenantDetails(ctx, h.cache, h.authURL, slug, cache.DefaultTenantTTL)
		if err != nil {
			h.log.Debug("tenant branding cache miss", zap.String("slug", slug), zap.Error(err))
		} else {
			branding := cache.GetTenantBranding(details)
			resp.LogoURL = branding.LogoURL
			resp.PrimaryColor = branding.PrimaryColor
			resp.SecondaryColor = branding.SecondaryColor
			resp.SupportEmail = branding.Email
			resp.SupportPhone = branding.Phone
			if branding.Name != "" {
				resp.Name = branding.Name
			}
			resp.UseCase = details.UseCase
			resp.UseCases = details.UseCases
			// Populate brand_palette from all brand colors
			if details.BrandColors != nil {
				resp.BrandPalette = make(map[string]string)
				for k, v := range details.BrandColors {
					if s, ok := v.(string); ok {
						resp.BrandPalette[k] = s
					}
				}
			}
		}
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
