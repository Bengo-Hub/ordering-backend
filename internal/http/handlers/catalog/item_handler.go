package cataloghandler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/bengobox/ordering-backend/internal/http/handlers"
	"github.com/bengobox/ordering-backend/internal/modules/catalog"
)

// CreateCatalogItem creates a new menu item.
// @Summary Create a new menu item
// @Description Creates a new menu item within a category
// @Tags Catalog
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param payload body CreateCatalogItemRequest true "Menu item data"
// @Success 201 {object} catalog.CatalogItem
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Failure 409 {object} handlers.ErrorResponse
// @Router /catalog/items [post]
func (h *Handler) CreateCatalogItem(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	var req CreateCatalogItemRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	outletID, err := uuid.Parse(req.OutletID)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid outlet ID")
		return
	}

	categoryID, err := uuid.Parse(req.CategoryID)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid category ID")
		return
	}

	inventoryItemID, err := uuid.Parse(req.InventoryItemID)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid inventory item ID")
		return
	}

	var recipeID *uuid.UUID
	if req.RecipeID != nil && *req.RecipeID != "" {
		rid, err := uuid.Parse(*req.RecipeID)
		if err == nil {
			recipeID = &rid
		}
	}

	item, err := h.service.CreateCatalogItem(r.Context(), catalog.CreateCatalogItemRequest{
		TenantID:        tenantID,
		OutletID:        outletID,
		CategoryID:      categoryID,
		InventoryItemID: inventoryItemID,
		RecipeID:        recipeID,
		IsAvailable:     req.IsAvailable,
		IsFeatured:      req.IsFeatured,
		LeadTimeMinutes: req.LeadTimeMinutes,
		SKU:             req.SKU,
		DisplayOrder:    req.DisplayOrder,
	})
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusCreated, item)
}

// GetCatalogItem retrieves a menu item by ID.
// @Summary Get a menu item
// @Description Retrieves a menu item by its ID including variants and translations
// @Tags Catalog
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param id path string true "Menu item ID"
// @Success 200 {object} catalog.CatalogItem
// @Failure 404 {object} handlers.ErrorResponse
// @Router /catalog/items/{id} [get]
func (h *Handler) GetCatalogItem(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	itemID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid item ID")
		return
	}

	item, err := h.service.GetCatalogItem(r.Context(), tenantID, itemID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, item)
}

// ListCatalogItems lists all menu items with optional filters.
// @Summary List menu items
// @Description Lists all menu items with optional filters
// @Tags Catalog
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param outlet_id query string false "Filter by outlet ID"
// @Param category_id query string false "Filter by category ID"
// @Param is_available query boolean false "Filter by availability"
// @Param search query string false "Search by name or description"
// @Param min_price query number false "Minimum price filter"
// @Param max_price query number false "Maximum price filter"
// @Param limit query integer false "Page size (default 50)"
// @Param page query integer false "Page number (default 1)"
// @Success 200 {object} ListResponse
// @Router /catalog/items [get]
func (h *Handler) ListCatalogItems(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	limit, offset, page := getPagination(r)
	filter := catalog.CatalogItemFilter{
		TenantID: tenantID,
		Search:   r.URL.Query().Get("search"),
		Locale:   getLocale(r),
		Limit:    limit,
		Offset:   offset,
	}

	if outletIDStr := r.URL.Query().Get("outlet_id"); outletIDStr != "" {
		outletID, err := uuid.Parse(outletIDStr)
		if err == nil {
			filter.OutletID = &outletID
		}
	}

	if categoryIDStr := r.URL.Query().Get("category_id"); categoryIDStr != "" {
		categoryID, err := uuid.Parse(categoryIDStr)
		if err == nil {
			filter.CategoryID = &categoryID
		}
	}

	if isAvailableStr := r.URL.Query().Get("is_available"); isAvailableStr != "" {
		isAvailable := isAvailableStr == "true"
		filter.IsAvailable = &isAvailable
	}

	items, total, err := h.service.ListCatalogItems(r.Context(), filter)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, ListResponse{
		Data:  items,
		Total: total,
		Limit: limit,
		Page:  page,
	})
}

// UpdateCatalogItem updates an existing menu item.
// @Summary Update a menu item
// @Description Updates an existing menu item
// @Tags Catalog
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param id path string true "Menu item ID"
// @Param payload body UpdateCatalogItemRequest true "Updated item data"
// @Success 200 {object} catalog.CatalogItem
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Router /catalog/items/{id} [put]
func (h *Handler) UpdateCatalogItem(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	itemID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid item ID")
		return
	}

	var req UpdateCatalogItemRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var recipeID *uuid.UUID
	if req.RecipeID != nil && *req.RecipeID != "" {
		rid, err := uuid.Parse(*req.RecipeID)
		if err == nil {
			recipeID = &rid
		}
	}

	updateReq := catalog.UpdateCatalogItemRequest{
		TenantID:        tenantID,
		ItemID:          itemID,
		RecipeID:        recipeID,
		IsAvailable:     req.IsAvailable,
		IsFeatured:      req.IsFeatured,
		LeadTimeMinutes: req.LeadTimeMinutes,
		SKU:             req.SKU,
		DisplayOrder:    req.DisplayOrder,
	}

	item, err := h.service.UpdateCatalogItem(r.Context(), updateReq)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, item)
}

// DeleteCatalogItem deletes a menu item.
// @Summary Delete a menu item
// @Description Deletes a menu item
// @Tags Catalog
// @Param Authorization header string true "Bearer token"
// @Param id path string true "Menu item ID"
// @Success 204 "No Content"
// @Failure 404 {object} handlers.ErrorResponse
// @Router /catalog/items/{id} [delete]
func (h *Handler) DeleteCatalogItem(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	itemID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid item ID")
		return
	}

	if err := h.service.DeleteCatalogItem(r.Context(), tenantID, itemID); err != nil {
		h.handleError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListPublicCatalogItems lists public menu items (no auth required).
// @Summary List public menu items
// @Description Lists available menu items for public display with localization support
// @Tags Menu
// @Produce json
// @Param outlet_id query string false "Filter by outlet ID"
// @Param category_id query string false "Filter by category ID"
// @Param search query string false "Search by name"
// @Param locale query string false "Locale for translations (default: en)"
// @Param limit query integer false "Page size (default 50)"
// @Param page query integer false "Page number (default 1)"
// @Success 200 {object} ListResponse
// @Router /menu/items [get]
func (h *Handler) ListPublicCatalogItems(w http.ResponseWriter, r *http.Request) {
	tenantID, err := h.getTenantIDForPublic(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	limit, offset, page := getPagination(r)
	locale := getLocale(r)

	req := catalog.PublicCatalogRequest{
		TenantID: tenantID,
		Locale:   locale,
		Search:   r.URL.Query().Get("search"),
		Limit:    limit,
		Offset:   offset,
	}

	if outletIDStr := r.URL.Query().Get("outlet_id"); outletIDStr != "" {
		outletID, err := uuid.Parse(outletIDStr)
		if err == nil {
			req.OutletID = &outletID
		}
	}

	if categoryIDStr := r.URL.Query().Get("category_id"); categoryIDStr != "" {
		categoryID, err := uuid.Parse(categoryIDStr)
		if err == nil {
			req.CategoryID = &categoryID
		}
	}

	// Handle favorites filtering for public menu (requires auth)
	if r.URL.Query().Get("favorite") == "true" {
		userID, err := getUserID(r)
		if err == nil {
			req.UserID = &userID
			req.FavoriteOnly = true
		}
	}

	items, total, err := h.service.GetPublicMenu(r.Context(), req)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, ListResponse{
		Data:  items,
		Total: total,
		Limit: limit,
		Page:  page,
	})
}

// GetPublicCatalogItem retrieves a public menu item (no auth required).
// @Summary Get a public menu item
// @Description Retrieves a menu item for public display
// @Tags Menu
// @Produce json
// @Param id path string true "Menu item ID"
// @Param locale query string false "Locale for translations (default: en)"
// @Success 200 {object} catalog.PublicCatalogItem
// @Failure 404 {object} handlers.ErrorResponse
// @Router /menu/items/{id} [get]
func (h *Handler) GetPublicCatalogItem(w http.ResponseWriter, r *http.Request) {
	tenantID, err := h.getTenantIDForPublic(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	itemID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid item ID")
		return
	}

	item, err := h.service.GetCatalogItem(r.Context(), tenantID, itemID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	// Check availability for public API
	if !item.IsAvailable {
		handlers.RespondError(w, http.StatusNotFound, "menu item not found")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, item)
}


// ListDietaryTags lists all dietary tags.
// @Summary List dietary tags
// @Description Lists all available dietary tags
// @Tags Catalog
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Success 200 {array} catalog.DietaryTag
// @Router /catalog/dietary-tags [get]
func (h *Handler) ListDietaryTags(w http.ResponseWriter, r *http.Request) {
	tags, err := h.service.ListDietaryTags(r.Context())
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, tags)
}

// AddDietaryTag adds a dietary tag to a menu item.
// @Summary Add dietary tag to item
// @Description Adds a dietary tag to a menu item
// @Tags Catalog
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param id path string true "Menu item ID"
// @Param payload body AddDietaryTagRequest true "Dietary tag code"
// @Success 200 {object} map[string]string
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Router /catalog/items/{id}/dietary-tags [post]
func (h *Handler) AddDietaryTag(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	catalogItemID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid item ID")
		return
	}

	var req AddDietaryTagRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.service.AddDietaryTagToItem(r.Context(), tenantID, catalogItemID, req.Code); err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, map[string]string{"status": "added"})
}

// RemoveDietaryTag removes a dietary tag from a menu item.
// @Summary Remove dietary tag from item
// @Description Removes a dietary tag from a menu item
// @Tags Catalog
// @Param Authorization header string true "Bearer token"
// @Param id path string true "Menu item ID"
// @Param code path string true "Dietary tag code"
// @Success 204 "No Content"
// @Failure 404 {object} handlers.ErrorResponse
// @Router /catalog/items/{id}/dietary-tags/{code} [delete]
func (h *Handler) RemoveDietaryTag(w http.ResponseWriter, r *http.Request) {
	catalogItemID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid item ID")
		return
	}

	tagCode := chi.URLParam(r, "code")
	if tagCode == "" {
		handlers.RespondError(w, http.StatusBadRequest, "tag code required")
		return
	}

	if err := h.service.RemoveDietaryTagFromItem(r.Context(), catalogItemID, tagCode); err != nil {
		h.handleError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ToggleFavorite toggles favorite status for a menu item.
// @Summary Toggle favorite status
// @Description Adds or removes a menu item from the current user's favorites
// @Tags Menu
// @Param Authorization header string true "Bearer token"
// @Param id path string true "Menu item ID"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} handlers.ErrorResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Router /menu/items/{id}/favorite [post]
func (h *Handler) ToggleFavorite(w http.ResponseWriter, r *http.Request) {
	userID, err := getUserID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	itemID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid item ID")
		return
	}

	isFavorite, err := h.service.ToggleFavorite(r.Context(), userID, itemID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"isFavorite": isFavorite,
	})
}
