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
	// Public catalog API (no auth required) — generic naming for all use cases
	r.Route("/catalog", func(catalogRouter chi.Router) {
		// Public read-only endpoints
		catalogRouter.Get("/categories", h.ListPublicCategories)
		catalogRouter.Get("/items", h.ListPublicCatalogItems)
		catalogRouter.Get("/items/{id}", h.GetPublicCatalogItem)

		// Auth required for toggling favorites
		catalogRouter.Group(func(authRouter chi.Router) {
			authRouter.Use(auth.OptionalAuth)
			authRouter.Post("/items/{id}/favorite", h.ToggleFavorite)
		})

		// Admin catalog routes (auth + permissions required)
		catalogRouter.Group(func(adminRouter chi.Router) {
			adminRouter.Use(auth.RequireAuth)

			// Categories management
			adminRouter.With(auth.RequirePermissions(identity.PermissionCatalogManage)).
				Post("/categories", h.CreateCategory)
			adminRouter.With(auth.RequirePermissions(identity.PermissionCatalogView)).
				Get("/categories/{id}", h.GetCategory)
			adminRouter.With(auth.RequirePermissions(identity.PermissionCatalogManage)).
				Put("/categories/{id}", h.UpdateCategory)
			adminRouter.With(auth.RequirePermissions(identity.PermissionCatalogManage)).
				Delete("/categories/{id}", h.DeleteCategory)

			// Catalog Items management
			adminRouter.With(auth.RequirePermissions(identity.PermissionCatalogManage)).
				Post("/items", h.CreateCatalogItem)
			adminRouter.With(auth.RequirePermissions(identity.PermissionCatalogManage)).
				Put("/items/{id}", h.UpdateCatalogItem)
			adminRouter.With(auth.RequirePermissions(identity.PermissionCatalogManage)).
				Delete("/items/{id}", h.DeleteCatalogItem)

			// Dietary Tags
			adminRouter.With(auth.RequirePermissions(identity.PermissionCatalogView)).
				Get("/dietary-tags", h.ListDietaryTags)
			adminRouter.With(auth.RequirePermissions(identity.PermissionCatalogManage)).
				Post("/items/{id}/dietary-tags", h.AddDietaryTag)
			adminRouter.With(auth.RequirePermissions(identity.PermissionCatalogManage)).
				Delete("/items/{id}/dietary-tags/{code}", h.RemoveDietaryTag)
		})
	})

	// Public outlets list (no auth required)
	r.Route("/outlets", func(outletRouter chi.Router) {
		outletRouter.Get("/", h.ListOutlets)
		outletRouter.Get("/{id}", h.GetOutlet)
	})

	// Admin-only catalog list endpoints (auth required, shows all items including unavailable)
	r.Route("/catalog/admin", func(adminRouter chi.Router) {
		adminRouter.Use(auth.RequireAuth)
		adminRouter.With(auth.RequirePermissions(identity.PermissionCatalogView)).
			Get("/categories", h.ListCategories)
		adminRouter.With(auth.RequirePermissions(identity.PermissionCatalogView)).
			Get("/items", h.ListCatalogItems)
		adminRouter.With(auth.RequirePermissions(identity.PermissionCatalogView)).
			Get("/items/{id}", h.GetCatalogItem)
	})
}

// --- Request/Response Types ---

type CreateCategoryRequest struct {
	OutletID     string  `json:"outletId"`
	ParentID     *string `json:"parentId,omitempty"`
	Name         string  `json:"name"`
	Slug         string  `json:"slug,omitempty"`
	Description  string  `json:"description,omitempty"`
	ImageURL     string  `json:"imageUrl,omitempty"`
	DisplayOrder int     `json:"displayOrder"`
	IsActive     bool    `json:"isActive"`
}

type UpdateCategoryRequest struct {
	ParentID     *string `json:"parentId,omitempty"`
	ClearParent  bool    `json:"clearParent,omitempty"`
	Name         *string `json:"name,omitempty"`
	Slug         *string `json:"slug,omitempty"`
	Description  *string `json:"description,omitempty"`
	ImageURL     *string `json:"imageUrl,omitempty"`
	DisplayOrder *int    `json:"displayOrder,omitempty"`
	IsActive     *bool   `json:"isActive,omitempty"`
}

type CreateCatalogItemRequest struct {
	OutletID        string  `json:"outletId"`
	CategoryID      string  `json:"categoryId"`
	InventoryItemID string  `json:"inventoryItemId,omitempty"`
	RecipeID        *string `json:"recipeId,omitempty"`
	Name            string  `json:"name"`
	Description     string  `json:"description,omitempty"`
	BasePrice       float64 `json:"basePrice"`
	Currency        string  `json:"currency,omitempty"`
	ImageURL        string  `json:"imageUrl,omitempty"`
	IsAvailable     bool    `json:"isAvailable"`
	IsFeatured      bool    `json:"isFeatured"`
	LeadTimeMinutes int     `json:"leadTimeMinutes,omitempty"`
	SKU             string  `json:"sku,omitempty"`
	DisplayOrder    int     `json:"displayOrder"`
}

type UpdateCatalogItemRequest struct {
	CategoryID      *string  `json:"categoryId,omitempty"`
	RecipeID        *string  `json:"recipeId,omitempty"`
	Name            *string  `json:"name,omitempty"`
	Description     *string  `json:"description,omitempty"`
	BasePrice       *float64 `json:"basePrice,omitempty"`
	Currency        *string  `json:"currency,omitempty"`
	ImageURL        *string  `json:"imageUrl,omitempty"`
	IsAvailable     *bool    `json:"isAvailable,omitempty"`
	IsFeatured      *bool    `json:"isFeatured,omitempty"`
	LeadTimeMinutes *int     `json:"leadTimeMinutes,omitempty"`
	SKU             *string  `json:"sku,omitempty"`
	DisplayOrder    *int     `json:"displayOrder,omitempty"`
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
		errors.Is(err, catalog.ErrCatalogItemNotFound),
		errors.Is(err, catalog.ErrTranslationNotFound),
		errors.Is(err, catalog.ErrDietaryTagNotFound),
		errors.Is(err, catalog.ErrAssetNotFound),
		errors.Is(err, catalog.ErrScheduleNotFound):
		handlers.RespondError(w, http.StatusNotFound, err.Error())

	case errors.Is(err, catalog.ErrCategoryAlreadyExists),
		errors.Is(err, catalog.ErrCatalogItemAlreadyExists),
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
	ctx := r.Context()

	// Platform owner query-param override
	if httpware.IsPlatformOwner(ctx) {
		if q := r.URL.Query().Get("tenantId"); q != "" {
			return uuid.Parse(q)
		}
	}

	// httpware context (from TenantV2 middleware — preferred)
	if tenantIDStr := httpware.GetTenantID(ctx); tenantIDStr != "" {
		return uuid.Parse(tenantIDStr)
	}

	// Fallback: header or context value
	tenantIDStr := r.Header.Get("X-Tenant-ID")
	if tenantIDStr == "" {
		if val := ctx.Value("tenant_id"); val != nil {
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

func getUserID(r *http.Request) (uuid.UUID, error) {
	// Try from context (set by auth middleware)
	if val := r.Context().Value("user_id"); val != nil {
		if id, ok := val.(uuid.UUID); ok {
			return id, nil
		}
		if str, ok := val.(string); ok {
			return uuid.Parse(str)
		}
	}
	return uuid.Nil, errors.New("user ID not found")
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
