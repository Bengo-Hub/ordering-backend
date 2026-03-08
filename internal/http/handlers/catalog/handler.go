package cataloghandler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/Bengo-Hub/httpware"
	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/tenant"
	"github.com/bengobox/ordering-backend/internal/http/handlers"
	identityhandler "github.com/bengobox/ordering-backend/internal/http/handlers/identity"
	"github.com/bengobox/ordering-backend/internal/modules/catalog"
	"github.com/bengobox/ordering-backend/internal/modules/identity"
)

// Handler exposes catalog-related HTTP endpoints.
type Handler struct {
	log     *zap.Logger
	service *catalog.Service
	db      *ent.Client // optional: for resolving tenant by slug on public routes
}

// New constructs a Handler instance. db may be nil; if set, public menu routes can resolve tenant by slug when X-Tenant-ID is not a UUID.
func New(log *zap.Logger, service *catalog.Service, db *ent.Client) *Handler {
	return &Handler{
		log:     log.Named("catalog.Handler"),
		service: service,
		db:      db,
	}
}

// Register mounts catalog routes on the supplied router, using the provided middleware.
func (h *Handler) Register(r chi.Router, auth *identityhandler.Authenticator) {
	// Public menu API (no auth required)
	r.Route("/menu", func(menuRouter chi.Router) {
		menuRouter.Get("/categories", h.ListPublicCategories)
		menuRouter.Get("/items", h.ListPublicMenuItems)
		menuRouter.Get("/items/{id}", h.GetPublicMenuItem)
	})
	// Public cafes/outlets list (no auth required)
	r.Route("/cafes", func(cafesRouter chi.Router) {
		cafesRouter.Get("/", h.ListCafes)
		cafesRouter.Get("/{id}", h.GetCafe)
	})

	// Admin catalog API (auth required)
	r.Route("/catalog", func(catalogRouter chi.Router) {
		catalogRouter.Use(auth.RequireAuth)

		// Categories - requires catalog:manage permission
		catalogRouter.With(auth.RequirePermissions(identity.PermissionCatalogManage)).
			Post("/categories", h.CreateCategory)
		catalogRouter.With(auth.RequirePermissions(identity.PermissionCatalogView)).
			Get("/categories", h.ListCategories)
		catalogRouter.With(auth.RequirePermissions(identity.PermissionCatalogView)).
			Get("/categories/{id}", h.GetCategory)
		catalogRouter.With(auth.RequirePermissions(identity.PermissionCatalogManage)).
			Put("/categories/{id}", h.UpdateCategory)
		catalogRouter.With(auth.RequirePermissions(identity.PermissionCatalogManage)).
			Delete("/categories/{id}", h.DeleteCategory)

		// Menu Items
		catalogRouter.With(auth.RequirePermissions(identity.PermissionCatalogManage)).
			Post("/items", h.CreateMenuItem)
		catalogRouter.With(auth.RequirePermissions(identity.PermissionCatalogView)).
			Get("/items", h.ListMenuItems)
		catalogRouter.With(auth.RequirePermissions(identity.PermissionCatalogView)).
			Get("/items/{id}", h.GetMenuItem)
		catalogRouter.With(auth.RequirePermissions(identity.PermissionCatalogManage)).
			Put("/items/{id}", h.UpdateMenuItem)
		catalogRouter.With(auth.RequirePermissions(identity.PermissionCatalogManage)).
			Delete("/items/{id}", h.DeleteMenuItem)

		// Variants
		catalogRouter.With(auth.RequirePermissions(identity.PermissionCatalogManage)).
			Post("/items/{id}/variants", h.CreateVariant)
		catalogRouter.With(auth.RequirePermissions(identity.PermissionCatalogView)).
			Get("/items/{id}/variants", h.ListVariants)

		// Translations
		catalogRouter.With(auth.RequirePermissions(identity.PermissionCatalogManage)).
			Post("/items/{id}/translations", h.CreateTranslation)
		catalogRouter.With(auth.RequirePermissions(identity.PermissionCatalogView)).
			Get("/items/{id}/translations", h.ListTranslations)

		// Dietary Tags
		catalogRouter.With(auth.RequirePermissions(identity.PermissionCatalogView)).
			Get("/dietary-tags", h.ListDietaryTags)
		catalogRouter.With(auth.RequirePermissions(identity.PermissionCatalogManage)).
			Post("/items/{id}/dietary-tags", h.AddDietaryTag)
		catalogRouter.With(auth.RequirePermissions(identity.PermissionCatalogManage)).
			Delete("/items/{id}/dietary-tags/{code}", h.RemoveDietaryTag)
	})
}

// --- Request/Response Types ---

type CreateCategoryRequest struct {
	CafeID       string  `json:"cafeId"`
	ParentID     *string `json:"parentId,omitempty"`
	Name         string  `json:"name"`
	Description  string  `json:"description,omitempty"`
	DisplayOrder int     `json:"displayOrder"`
	IsActive     bool    `json:"isActive"`
	ImageURL     string  `json:"imageUrl,omitempty"`
}

type UpdateCategoryRequest struct {
	ParentID     *string `json:"parentId,omitempty"`
	ClearParent  bool    `json:"clearParent,omitempty"`
	Name         *string `json:"name,omitempty"`
	Description  *string `json:"description,omitempty"`
	DisplayOrder *int    `json:"displayOrder,omitempty"`
	IsActive     *bool   `json:"isActive,omitempty"`
	ImageURL     *string `json:"imageUrl,omitempty"`
}

type CreateMenuItemRequest struct {
	CafeID          string  `json:"cafeId"`
	CategoryID      string  `json:"categoryId"`
	Name            string  `json:"name"`
	Description     string  `json:"description,omitempty"`
	BasePrice       float64 `json:"basePrice"`
	Currency        string  `json:"currency,omitempty"`
	IsAvailable     bool    `json:"isAvailable"`
	LeadTimeMinutes int     `json:"leadTimeMinutes,omitempty"`
	ImageURL        string  `json:"imageUrl,omitempty"`
	SKU             string  `json:"sku,omitempty"`
	DisplayOrder    int     `json:"displayOrder"`
}

type UpdateMenuItemRequest struct {
	CategoryID      *string  `json:"categoryId,omitempty"`
	Name            *string  `json:"name,omitempty"`
	Description     *string  `json:"description,omitempty"`
	BasePrice       *float64 `json:"basePrice,omitempty"`
	Currency        *string  `json:"currency,omitempty"`
	IsAvailable     *bool    `json:"isAvailable,omitempty"`
	LeadTimeMinutes *int     `json:"leadTimeMinutes,omitempty"`
	ImageURL        *string  `json:"imageUrl,omitempty"`
	SKU             *string  `json:"sku,omitempty"`
	DisplayOrder    *int     `json:"displayOrder,omitempty"`
}

type CreateVariantRequest struct {
	Name         string  `json:"name"`
	PriceDelta   float64 `json:"priceDelta"`
	IsAvailable  bool    `json:"isAvailable"`
	SKU          string  `json:"sku,omitempty"`
	DisplayOrder int     `json:"displayOrder"`
}

type CreateTranslationRequest struct {
	Locale      string `json:"locale"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type AddDietaryTagRequest struct {
	Code string `json:"code"`
}

type ListResponse struct {
	Data  interface{} `json:"data"`
	Total int         `json:"total"`
	Limit int         `json:"limit"`
	Page  int         `json:"page"`
}

// --- Helper Functions ---

func decodeJSON(r *http.Request, dst interface{}) error {
	return json.NewDecoder(r.Body).Decode(dst)
}

func (h *Handler) handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, catalog.ErrCategoryNotFound),
		errors.Is(err, catalog.ErrMenuItemNotFound),
		errors.Is(err, catalog.ErrVariantNotFound),
		errors.Is(err, catalog.ErrTranslationNotFound),
		errors.Is(err, catalog.ErrDietaryTagNotFound),
		errors.Is(err, catalog.ErrAssetNotFound),
		errors.Is(err, catalog.ErrScheduleNotFound):
		handlers.RespondError(w, http.StatusNotFound, err.Error())

	case errors.Is(err, catalog.ErrCategoryAlreadyExists),
		errors.Is(err, catalog.ErrMenuItemAlreadyExists),
		errors.Is(err, catalog.ErrVariantAlreadyExists),
		errors.Is(err, catalog.ErrTranslationAlreadyExists),
		errors.Is(err, catalog.ErrDietaryTagAlreadyExists):
		handlers.RespondError(w, http.StatusConflict, err.Error())

	case errors.Is(err, catalog.ErrCategoryHasItems),
		errors.Is(err, catalog.ErrCategoryHasChildren),
		errors.Is(err, catalog.ErrInvalidCategoryParent),
		errors.Is(err, catalog.ErrInvalidCategory),
		errors.Is(err, catalog.ErrInvalidPrice),
		errors.Is(err, catalog.ErrInvalidSKU),
		errors.Is(err, catalog.ErrInvalidLocale),
		errors.Is(err, catalog.ErrInvalidAssetType),
		errors.Is(err, catalog.ErrInvalidAssetURL),
		errors.Is(err, catalog.ErrInvalidScheduleTime),
		errors.Is(err, catalog.ErrInvalidDayOfWeek):
		handlers.RespondError(w, http.StatusBadRequest, err.Error())

	case errors.Is(err, catalog.ErrUnauthorized):
		handlers.RespondError(w, http.StatusForbidden, err.Error())

	default:
		h.log.Error("internal error", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "internal server error")
	}
}

func getTenantID(r *http.Request) (uuid.UUID, error) {
	// Get tenant ID from context (set by auth middleware)
	tenantIDStr := r.Header.Get("X-Tenant-ID")
	if tenantIDStr == "" {
		// Try from context
		if val := r.Context().Value("tenant_id"); val != nil {
			if id, ok := val.(uuid.UUID); ok {
				return id, nil
			}
			if str, ok := val.(string); ok {
				return uuid.Parse(str)
			}
		}
		return uuid.Nil, errors.New("tenant ID not found")
	}
	return uuid.Parse(tenantIDStr)
}

// getTenantIDForPublic returns tenant UUID for public routes. It tries getTenantID first; if that fails (e.g. X-Tenant-ID is a slug like "tenant-urban-loft"), it resolves tenant by slug from context or URL when h.db is set.
func (h *Handler) getTenantIDForPublic(r *http.Request) (uuid.UUID, error) {
	id, err := getTenantID(r)
	if err == nil {
		return id, nil
	}
	if h.db == nil {
		return uuid.Nil, err
	}
	slug := httpware.GetTenantSlug(r.Context())
	if slug == "" {
		slug = chi.URLParam(r, "tenant")
	}
	if slug == "" {
		return uuid.Nil, errors.New("tenant ID or slug required")
	}
	t, err := h.db.Tenant.Query().
		Where(tenant.SlugEQ(slug)).
		Only(r.Context())
	if err != nil {
		if ent.IsNotFound(err) {
			return uuid.Nil, errors.New("tenant not found")
		}
		return uuid.Nil, err
	}
	return t.ID, nil
}

func getCafeID(r *http.Request) (uuid.UUID, error) {
	cafeIDStr := r.URL.Query().Get("cafe_id")
	if cafeIDStr == "" {
		cafeIDStr = r.Header.Get("X-Cafe-ID")
	}
	if cafeIDStr == "" {
		return uuid.Nil, nil // Optional
	}
	return uuid.Parse(cafeIDStr)
}

func getPagination(r *http.Request) (limit, offset, page int) {
	limit = 50
	page = 1

	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	if p := r.URL.Query().Get("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}

	offset = (page - 1) * limit
	return
}

func getLocale(r *http.Request) string {
	// Check query param first
	if locale := r.URL.Query().Get("locale"); locale != "" {
		return locale
	}
	// Check Accept-Language header
	if lang := r.Header.Get("Accept-Language"); lang != "" {
		// Extract primary language
		if idx := len(lang); idx > 2 {
			return lang[:2]
		}
		return lang
	}
	return catalog.LocaleEnglish
}
