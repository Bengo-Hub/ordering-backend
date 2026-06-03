package cataloghandler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/Bengo-Hub/httpware"
	authclient "github.com/Bengo-Hub/shared-auth-client"
	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/tenant"
	"github.com/bengobox/ordering-backend/internal/http/handlers"
	identityhandler "github.com/bengobox/ordering-backend/internal/http/handlers/identity"
	"github.com/bengobox/ordering-backend/internal/modules/catalog"
	"github.com/bengobox/ordering-backend/internal/modules/identity"
)

// Handler exposes catalog-related HTTP endpoints (proxy model).
type Handler struct {
	log     *zap.Logger
	service *catalog.ProxyService
	db      *ent.Client
}

// New constructs a Handler instance.
func New(log *zap.Logger, service *catalog.ProxyService, db *ent.Client) *Handler {
	return &Handler{
		log:     log.Named("catalog.Handler"),
		service: service,
		db:      db,
	}
}

// Register mounts catalog routes on the supplied router.
func (h *Handler) Register(r chi.Router, auth *identityhandler.Authenticator) {
	// Public catalog API (no auth required)
	r.Route("/catalog", func(catalogRouter chi.Router) {
		// Public read-only endpoints
		catalogRouter.Get("/categories", h.ListPublicCategories)
		catalogRouter.Get("/items", h.ListPublicItems)
		catalogRouter.Get("/items/{sku}", h.GetPublicItem)

		catalogRouter.Get("/items/{sku}/images", h.GetItemImages)

		// Public event-ticket storefront (no auth): browse events + per-tier availability
		catalogRouter.Get("/events", h.ListPublicEvents)
		catalogRouter.Get("/events/{id}", h.GetPublicEvent)

		// Optional auth for toggling favorites
		catalogRouter.Group(func(authRouter chi.Router) {
			authRouter.Use(auth.OptionalAuth)
			authRouter.Post("/items/{sku}/favorite", h.ToggleFavorite)
		})

		// Admin catalog override management (auth + permissions required)
		// multi_outlet feature gate removed: single-outlet tenants must be able to manage their catalog
		catalogRouter.Group(func(adminRouter chi.Router) {
			adminRouter.Use(auth.RequireAuth)

			adminRouter.With(auth.RequirePermissions(identity.PermissionCatalogManage)).
				Post("/overrides", h.UpsertOverride)
			adminRouter.With(auth.RequirePermissions(identity.PermissionCatalogManage)).
				Put("/overrides/{sku}", h.UpdateOverride)
			adminRouter.With(auth.RequirePermissions(identity.PermissionCatalogManage)).
				Delete("/overrides/{sku}", h.DeleteOverride)
		})

		// Admin catalog list endpoints (auth required)
		catalogRouter.Route("/admin", func(adminRouter chi.Router) {
			adminRouter.Use(auth.RequireAuth)
			adminRouter.With(auth.RequirePermissions(identity.PermissionCatalogView)).
				Get("/items", h.AdminListItems)
			adminRouter.With(auth.RequirePermissions(identity.PermissionCatalogView)).
				Get("/categories", h.AdminListCategories)
		})
	})

	// Public outlets list (no auth required)
	r.Route("/outlets", func(outletRouter chi.Router) {
		outletRouter.Get("/", h.ListOutlets)
		outletRouter.Get("/{id}", h.GetOutlet)
		outletRouter.Get("/{id}/time-slots", h.GetOutletTimeSlots)
	})
}

// --- Request Types ---

type OverrideRequest struct {
	OutletID          string   `json:"outletId"`
	SKU               string   `json:"sku"`
	BasePrice         float64  `json:"basePrice"`
	Currency          string   `json:"currency,omitempty"`
	IsAvailable       *bool    `json:"isAvailable,omitempty"`
	IsFeatured        *bool    `json:"isFeatured,omitempty"`
	LeadTimeMinutes   *int     `json:"leadTimeMinutes,omitempty"`
	DisplayOrder      *int     `json:"displayOrder,omitempty"`
	DisplaySection    string   `json:"displaySection,omitempty"`
	PackagingFee      *float64 `json:"packagingFee,omitempty"`
	ServiceFeePercent *float64 `json:"serviceFeePercent,omitempty"`
	ImageURLOverride  string   `json:"imageUrlOverride,omitempty"`
}

// --- Handlers ---

// ListPublicItems lists public catalog items (no auth required).
func (h *Handler) ListPublicItems(w http.ResponseWriter, r *http.Request) {
	tenantID, tenantSlug, err := h.resolveTenant(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	limit, offset, page := getPagination(r)
	filter := catalog.CatalogFilter{
		TenantID: tenantID,
		Search:   r.URL.Query().Get("search"),
		Section:  r.URL.Query().Get("section"),
		ItemType: strings.ToUpper(r.URL.Query().Get("item_type")),
		Limit:    limit,
		Offset:   offset,
	}

	if tagsParam := r.URL.Query().Get("tags"); tagsParam != "" {
		for _, t := range strings.Split(tagsParam, ",") {
			if t = strings.TrimSpace(t); t != "" {
				filter.Tags = append(filter.Tags, t)
			}
		}
	}

	if outletIDStr := r.URL.Query().Get("outlet_id"); outletIDStr != "" {
		outletID, err := uuid.Parse(outletIDStr)
		if err == nil {
			filter.OutletID = &outletID
		}
	}

	if catIDStr := r.URL.Query().Get("category_id"); catIDStr != "" {
		catID, err := uuid.Parse(catIDStr)
		if err == nil {
			filter.CategoryID = &catID
		}
	}

	if f := r.URL.Query().Get("featured"); f == "true" || f == "false" {
		v := f == "true"
		filter.IsFeatured = &v
	}

	// For public listings, default to available only
	avail := true
	filter.IsAvailable = &avail

	// Check for authenticated user (for favorites)
	if claims, ok := authclient.ClaimsFromContext(r.Context()); ok {
		userID, err := uuid.Parse(claims.Subject)
		if err == nil {
			filter.UserID = &userID
		}
	}

	items, total, err := h.service.ListItems(r.Context(), tenantSlug, tenantID, filter)
	if err != nil {
		h.log.Error("list items failed", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, catalog.ListResponse{
		Data:  items,
		Total: total,
		Limit: limit,
		Page:  page,
	})
}

// GetPublicItem retrieves a single catalog item by SKU (no auth required).
func (h *Handler) GetPublicItem(w http.ResponseWriter, r *http.Request) {
	tenantID, tenantSlug, err := h.resolveTenant(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	sku := chi.URLParam(r, "sku")
	if sku == "" {
		handlers.RespondError(w, http.StatusBadRequest, "sku is required")
		return
	}

	var userID *uuid.UUID
	if claims, ok := authclient.ClaimsFromContext(r.Context()); ok {
		uid, err := uuid.Parse(claims.Subject)
		if err == nil {
			userID = &uid
		}
	}

	item, err := h.service.GetItem(r.Context(), tenantSlug, tenantID, sku, userID)
	if err != nil {
		if errors.Is(err, catalog.ErrItemNotFound) {
			handlers.RespondError(w, http.StatusNotFound, "item not found")
			return
		}
		h.log.Error("get item failed", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, item)
}

// ListPublicCategories lists categories from inventory-api (no auth required).
func (h *Handler) ListPublicCategories(w http.ResponseWriter, r *http.Request) {
	_, tenantSlug, err := h.resolveTenant(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	categories, err := h.service.ListCategories(r.Context(), tenantSlug)
	if err != nil {
		h.log.Error("list categories failed", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, categories)
}

// GetItemImages returns the multi-image gallery for a catalog item by SKU.
func (h *Handler) GetItemImages(w http.ResponseWriter, r *http.Request) {
	_, tenantSlug, err := h.resolveTenant(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	sku := chi.URLParam(r, "sku")
	if sku == "" {
		handlers.RespondError(w, http.StatusBadRequest, "sku is required")
		return
	}

	images, err := h.service.GetItemImages(r.Context(), tenantSlug, sku)
	if err != nil {
		h.log.Error("get item images failed", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, images)
}

// ToggleFavorite toggles favorite status for a catalog item by SKU.
func (h *Handler) ToggleFavorite(w http.ResponseWriter, r *http.Request) {
	claims, ok := authclient.ClaimsFromContext(r.Context())
	if !ok {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "invalid user")
		return
	}

	tenantID, _, err := h.resolveTenant(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	sku := chi.URLParam(r, "sku")
	if sku == "" {
		handlers.RespondError(w, http.StatusBadRequest, "sku is required")
		return
	}

	isFavorite, err := h.service.ToggleFavorite(r.Context(), tenantID, userID, sku)
	if err != nil {
		h.log.Error("toggle favorite failed", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"isFavorite": isFavorite,
	})
}

// UpsertOverride creates or updates a CatalogOverride (admin only).
func (h *Handler) UpsertOverride(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := h.resolveTenant(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req OverrideRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	outletID, err := uuid.Parse(req.OutletID)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid outlet ID")
		return
	}

	override, err := h.service.UpsertOverride(r.Context(), catalog.OverrideUpsertRequest{
		TenantID:          tenantID,
		OutletID:          outletID,
		InventorySKU:      req.SKU,
		BasePrice:         req.BasePrice,
		Currency:          req.Currency,
		IsAvailable:       req.IsAvailable,
		IsFeatured:        req.IsFeatured,
		LeadTimeMinutes:   req.LeadTimeMinutes,
		DisplayOrder:      req.DisplayOrder,
		DisplaySection:    req.DisplaySection,
		PackagingFee:      req.PackagingFee,
		ServiceFeePercent: req.ServiceFeePercent,
		ImageURLOverride:  req.ImageURLOverride,
	})
	if err != nil {
		h.log.Error("upsert override failed", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, override)
}

// UpdateOverride updates a CatalogOverride by SKU (admin only).
func (h *Handler) UpdateOverride(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := h.resolveTenant(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	sku := chi.URLParam(r, "sku")
	if sku == "" {
		handlers.RespondError(w, http.StatusBadRequest, "sku is required")
		return
	}

	var req OverrideRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	outletID, err := uuid.Parse(req.OutletID)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid outlet ID")
		return
	}

	override, err := h.service.UpsertOverride(r.Context(), catalog.OverrideUpsertRequest{
		TenantID:          tenantID,
		OutletID:          outletID,
		InventorySKU:      sku,
		BasePrice:         req.BasePrice,
		Currency:          req.Currency,
		IsAvailable:       req.IsAvailable,
		IsFeatured:        req.IsFeatured,
		LeadTimeMinutes:   req.LeadTimeMinutes,
		DisplayOrder:      req.DisplayOrder,
		DisplaySection:    req.DisplaySection,
		PackagingFee:      req.PackagingFee,
		ServiceFeePercent: req.ServiceFeePercent,
		ImageURLOverride:  req.ImageURLOverride,
	})
	if err != nil {
		h.log.Error("update override failed", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, override)
}

// DeleteOverride deletes a CatalogOverride by SKU (admin only).
func (h *Handler) DeleteOverride(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := h.resolveTenant(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	sku := chi.URLParam(r, "sku")
	if sku == "" {
		handlers.RespondError(w, http.StatusBadRequest, "sku is required")
		return
	}

	if err := h.service.DeleteOverride(r.Context(), tenantID, sku); err != nil {
		h.log.Error("delete override failed", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// AdminListItems lists all catalog items (admin, shows all including unavailable).
func (h *Handler) AdminListItems(w http.ResponseWriter, r *http.Request) {
	tenantID, tenantSlug, err := h.resolveTenant(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	limit, offset, page := getPagination(r)
	filter := catalog.CatalogFilter{
		TenantID: tenantID,
		Search:   r.URL.Query().Get("search"),
		Limit:    limit,
		Offset:   offset,
	}

	if outletIDStr := r.URL.Query().Get("outlet_id"); outletIDStr != "" {
		outletID, err := uuid.Parse(outletIDStr)
		if err == nil {
			filter.OutletID = &outletID
		}
	}

	items, total, err := h.service.ListItems(r.Context(), tenantSlug, tenantID, filter)
	if err != nil {
		h.log.Error("admin list items failed", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, catalog.ListResponse{
		Data:  items,
		Total: total,
		Limit: limit,
		Page:  page,
	})
}

// AdminListCategories lists categories from inventory-api (admin).
func (h *Handler) AdminListCategories(w http.ResponseWriter, r *http.Request) {
	_, tenantSlug, err := h.resolveTenant(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	categories, err := h.service.ListCategories(r.Context(), tenantSlug)
	if err != nil {
		h.log.Error("admin list categories failed", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, catalog.ListResponse{
		Data:  categories,
		Total: len(categories),
		Limit: len(categories),
		Page:  1,
	})
}

// ListOutlets returns the list of outlets for the tenant (public, no auth).
// Supports discovery query parameters: sort, lat, lng, category, offers,
// min_rating, max_delivery_fee, max_delivery_time, pickup, q, page, limit.
func (h *Handler) ListOutlets(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := h.resolveTenant(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "tenant required (use path slug or X-Tenant-ID)")
		return
	}

	q := r.URL.Query()

	// Check if any discovery params are present; if not, fall back to simple list
	hasDiscovery := q.Get("sort") != "" || q.Get("lat") != "" || q.Get("category") != "" ||
		q.Get("offers") != "" || q.Get("min_rating") != "" || q.Get("max_delivery_fee") != "" ||
		q.Get("max_delivery_time") != "" || q.Get("pickup") != "" || q.Get("q") != ""

	if !hasDiscovery {
		// Simple listing (backward compatible)
		list, listErr := h.service.ListOutlets(r.Context(), tenantID)
		if listErr != nil {
			h.log.Error("list outlets failed", zap.Error(listErr))
			handlers.RespondError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		handlers.RespondJSON(w, http.StatusOK, catalog.ListResponse{
			Data:  list,
			Total: len(list),
			Limit: len(list),
			Page:  1,
		})
		return
	}

	// Discovery mode
	params := catalog.OutletListParams{
		Sort:     q.Get("sort"),
		Category: q.Get("category"),
		Offers:   q.Get("offers") == "true",
		Pickup:   q.Get("pickup") == "true",
		Query:    q.Get("q"),
		Page:     handlers.ParseInt(q.Get("page"), 1),
		Limit:    handlers.ParseInt(q.Get("limit"), 50),
	}

	if latStr := q.Get("lat"); latStr != "" {
		if lat, parseErr := strconv.ParseFloat(latStr, 64); parseErr == nil {
			params.Lat = &lat
		}
	}
	if lngStr := q.Get("lng"); lngStr != "" {
		if lng, parseErr := strconv.ParseFloat(lngStr, 64); parseErr == nil {
			params.Lng = &lng
		}
	}
	if v := q.Get("min_rating"); v != "" {
		if f, parseErr := strconv.ParseFloat(v, 64); parseErr == nil {
			params.MinRating = &f
		}
	}
	if v := q.Get("max_delivery_fee"); v != "" {
		if f, parseErr := strconv.ParseFloat(v, 64); parseErr == nil {
			params.MaxDeliveryFee = &f
		}
	}
	if v := q.Get("max_delivery_time"); v != "" {
		if n, parseErr := strconv.Atoi(v); parseErr == nil {
			params.MaxDeliveryTime = &n
		}
	}

	results, total, discErr := h.service.ListOutletsDiscovery(r.Context(), tenantID, params)
	if discErr != nil {
		h.log.Error("list outlets discovery failed", zap.Error(discErr))
		handlers.RespondError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, catalog.ListResponse{
		Data:  results,
		Total: total,
		Limit: params.Limit,
		Page:  params.Page,
	})
}

// GetOutlet returns a single outlet by ID for the tenant (public, no auth).
func (h *Handler) GetOutlet(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := h.resolveTenant(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "tenant required (use path slug or X-Tenant-ID)")
		return
	}

	outletID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid outlet id")
		return
	}

	out, err := h.service.GetOutlet(r.Context(), tenantID, outletID)
	if err != nil {
		if errors.Is(err, catalog.ErrOutletNotFound) {
			handlers.RespondError(w, http.StatusNotFound, "outlet not found")
			return
		}
		h.log.Error("get outlet failed", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, out)
}

// GetOutletTimeSlots returns available delivery/pickup time slots for a given outlet and date.
// Query params: date (YYYY-MM-DD, defaults to today), slot_minutes (default 30), lead_minutes (default 45).
func (h *Handler) GetOutletTimeSlots(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := h.resolveTenant(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "tenant required")
		return
	}

	outletID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid outlet id")
		return
	}

	out, err := h.service.GetOutlet(r.Context(), tenantID, outletID)
	if err != nil {
		if errors.Is(err, catalog.ErrOutletNotFound) {
			handlers.RespondError(w, http.StatusNotFound, "outlet not found")
			return
		}
		h.log.Error("get outlet for time-slots failed", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	q := r.URL.Query()

	// Parse date (default: today). Use local timezone so weekday/today checks
	// in GenerateTimeSlots are correct (time.Parse returns UTC by default).
	date := time.Now()
	if dateStr := q.Get("date"); dateStr != "" {
		if parsed, pErr := time.ParseInLocation("2006-01-02", dateStr, time.Now().Location()); pErr == nil {
			date = parsed
		}
	}

	slotMinutes := 30
	if v := q.Get("slot_minutes"); v != "" {
		if n, pErr := strconv.Atoi(v); pErr == nil && n > 0 && n <= 120 {
			slotMinutes = n
		}
	}

	leadMinutes := 45
	if v := q.Get("lead_minutes"); v != "" {
		if n, pErr := strconv.Atoi(v); pErr == nil && n > 0 {
			leadMinutes = n
		}
	}

	slots := catalog.GenerateTimeSlots(out.OpeningHours, date, slotMinutes, leadMinutes)
	if slots == nil {
		slots = []catalog.TimeSlot{}
	}

	handlers.RespondJSON(w, http.StatusOK, map[string]any{
		"outlet_id":    outletID,
		"date":         date.Format("2006-01-02"),
		"slot_minutes": slotMinutes,
		"slots":        slots,
	})
}

// --- Helper Functions ---

func decodeJSON(r *http.Request, dst interface{}) error {
	return json.NewDecoder(r.Body).Decode(dst)
}

// resolveTenant returns tenantID and tenantSlug. It tries UUID first, then slug lookup.
func (h *Handler) resolveTenant(r *http.Request) (uuid.UUID, string, error) {
	ctx := r.Context()

	// Platform owner query-param override
	if httpware.IsPlatformOwner(ctx) {
		if q := r.URL.Query().Get("tenantId"); q != "" {
			id, err := uuid.Parse(q)
			if err == nil {
				slug := h.slugForTenant(ctx, id)
				return id, slug, nil
			}
		}
	}

	// httpware context (from TenantV2 middleware)
	if tenantIDStr := httpware.GetTenantID(ctx); tenantIDStr != "" {
		id, err := uuid.Parse(tenantIDStr)
		if err == nil {
			slug := httpware.GetTenantSlug(ctx)
			if slug == "" {
				slug = h.slugForTenant(ctx, id)
			}
			return id, slug, nil
		}
	}

	// Try slug from httpware context
	slug := httpware.GetTenantSlug(ctx)
	if slug != "" {
		id, err := h.tenantIDForSlug(ctx, slug)
		if err == nil {
			return id, slug, nil
		}
	}

	// Fallback: header
	tenantIDStr := r.Header.Get("X-Tenant-ID")
	if tenantIDStr != "" {
		id, err := uuid.Parse(tenantIDStr)
		if err == nil {
			s := h.slugForTenant(ctx, id)
			return id, s, nil
		}
		// Might be a slug
		tid, err := h.tenantIDForSlug(ctx, tenantIDStr)
		if err == nil {
			return tid, tenantIDStr, nil
		}
	}

	// Context value fallback
	if val := ctx.Value("tenant_id"); val != nil {
		if id, ok := val.(uuid.UUID); ok {
			s := h.slugForTenant(ctx, id)
			return id, s, nil
		}
		if str, ok := val.(string); ok {
			id, err := uuid.Parse(str)
			if err == nil {
				s := h.slugForTenant(ctx, id)
				return id, s, nil
			}
		}
	}

	return uuid.Nil, "", errors.New("tenant ID or slug required")
}

func (h *Handler) tenantIDForSlug(ctx context.Context, slug string) (uuid.UUID, error) {
	if h.db == nil {
		return uuid.Nil, errors.New("no db for slug lookup")
	}
	t, err := h.db.Tenant.Query().
		Where(tenant.SlugEQ(slug)).
		Only(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	return t.ID, nil
}

func (h *Handler) slugForTenant(ctx context.Context, id uuid.UUID) string {
	if h.db == nil {
		return ""
	}
	t, err := h.db.Tenant.Get(ctx, id)
	if err != nil {
		return ""
	}
	return t.Slug
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
