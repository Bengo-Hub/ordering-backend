package cataloghandler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/bengobox/ordering-backend/internal/http/handlers"
	"github.com/bengobox/ordering-backend/internal/modules/catalog"
)

// CreateMenuItem creates a new menu item.
// @Summary Create a new menu item
// @Description Creates a new menu item within a category
// @Tags Catalog
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param payload body CreateMenuItemRequest true "Menu item data"
// @Success 201 {object} catalog.MenuItem
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Failure 409 {object} handlers.ErrorResponse
// @Router /catalog/items [post]
func (h *Handler) CreateMenuItem(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	var req CreateMenuItemRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	cafeID, err := uuid.Parse(req.CafeID)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid cafe ID")
		return
	}

	categoryID, err := uuid.Parse(req.CategoryID)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid category ID")
		return
	}

	item, err := h.service.CreateMenuItem(r.Context(), catalog.CreateMenuItemRequest{
		TenantID:        tenantID,
		CafeID:          cafeID,
		CategoryID:      categoryID,
		Name:            req.Name,
		Description:     req.Description,
		BasePrice:       req.BasePrice,
		Currency:        req.Currency,
		IsAvailable:     req.IsAvailable,
		LeadTimeMinutes: req.LeadTimeMinutes,
		ImageURL:        req.ImageURL,
		SKU:             req.SKU,
		DisplayOrder:    req.DisplayOrder,
	})
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusCreated, item)
}

// GetMenuItem retrieves a menu item by ID.
// @Summary Get a menu item
// @Description Retrieves a menu item by its ID including variants and translations
// @Tags Catalog
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param id path string true "Menu item ID"
// @Success 200 {object} catalog.MenuItem
// @Failure 404 {object} handlers.ErrorResponse
// @Router /catalog/items/{id} [get]
func (h *Handler) GetMenuItem(w http.ResponseWriter, r *http.Request) {
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

	item, err := h.service.GetMenuItem(r.Context(), tenantID, itemID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, item)
}

// ListMenuItems lists all menu items with optional filters.
// @Summary List menu items
// @Description Lists all menu items with optional filters
// @Tags Catalog
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param cafe_id query string false "Filter by cafe ID"
// @Param category_id query string false "Filter by category ID"
// @Param is_available query boolean false "Filter by availability"
// @Param search query string false "Search by name or description"
// @Param min_price query number false "Minimum price filter"
// @Param max_price query number false "Maximum price filter"
// @Param limit query integer false "Page size (default 50)"
// @Param page query integer false "Page number (default 1)"
// @Success 200 {object} ListResponse
// @Router /catalog/items [get]
func (h *Handler) ListMenuItems(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	limit, offset, page := getPagination(r)
	filter := catalog.MenuItemFilter{
		TenantID: tenantID,
		Search:   r.URL.Query().Get("search"),
		Locale:   getLocale(r),
		Limit:    limit,
		Offset:   offset,
	}

	if cafeIDStr := r.URL.Query().Get("cafe_id"); cafeIDStr != "" {
		cafeID, err := uuid.Parse(cafeIDStr)
		if err == nil {
			filter.CafeID = &cafeID
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

	items, total, err := h.service.ListMenuItems(r.Context(), filter)
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

// UpdateMenuItem updates an existing menu item.
// @Summary Update a menu item
// @Description Updates an existing menu item
// @Tags Catalog
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param id path string true "Menu item ID"
// @Param payload body UpdateMenuItemRequest true "Updated item data"
// @Success 200 {object} catalog.MenuItem
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Router /catalog/items/{id} [put]
func (h *Handler) UpdateMenuItem(w http.ResponseWriter, r *http.Request) {
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

	var req UpdateMenuItemRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	updateReq := catalog.UpdateMenuItemRequest{
		TenantID:        tenantID,
		ItemID:          itemID,
		Name:            req.Name,
		Description:     req.Description,
		BasePrice:       req.BasePrice,
		Currency:        req.Currency,
		IsAvailable:     req.IsAvailable,
		LeadTimeMinutes: req.LeadTimeMinutes,
		ImageURL:        req.ImageURL,
		SKU:             req.SKU,
		DisplayOrder:    req.DisplayOrder,
	}

	if req.CategoryID != nil && *req.CategoryID != "" {
		catID, err := uuid.Parse(*req.CategoryID)
		if err != nil {
			handlers.RespondError(w, http.StatusBadRequest, "invalid category ID")
			return
		}
		updateReq.CategoryID = &catID
	}

	item, err := h.service.UpdateMenuItem(r.Context(), updateReq)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, item)
}

// DeleteMenuItem deletes a menu item.
// @Summary Delete a menu item
// @Description Deletes a menu item
// @Tags Catalog
// @Param Authorization header string true "Bearer token"
// @Param id path string true "Menu item ID"
// @Success 204 "No Content"
// @Failure 404 {object} handlers.ErrorResponse
// @Router /catalog/items/{id} [delete]
func (h *Handler) DeleteMenuItem(w http.ResponseWriter, r *http.Request) {
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

	if err := h.service.DeleteMenuItem(r.Context(), tenantID, itemID); err != nil {
		h.handleError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListPublicMenuItems lists public menu items (no auth required).
// @Summary List public menu items
// @Description Lists available menu items for public display with localization support
// @Tags Menu
// @Produce json
// @Param cafe_id query string false "Filter by cafe ID"
// @Param category_id query string false "Filter by category ID"
// @Param search query string false "Search by name"
// @Param locale query string false "Locale for translations (default: en)"
// @Param limit query integer false "Page size (default 50)"
// @Param page query integer false "Page number (default 1)"
// @Success 200 {object} ListResponse
// @Router /menu/items [get]
func (h *Handler) ListPublicMenuItems(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		tenantIDStr := r.URL.Query().Get("tenant_id")
		if tenantIDStr == "" {
			handlers.RespondError(w, http.StatusBadRequest, "tenant_id required")
			return
		}
		tenantID, err = uuid.Parse(tenantIDStr)
		if err != nil {
			handlers.RespondError(w, http.StatusBadRequest, "invalid tenant_id")
			return
		}
	}

	limit, offset, page := getPagination(r)
	locale := getLocale(r)

	req := catalog.PublicMenuRequest{
		Locale: locale,
		Search: r.URL.Query().Get("search"),
		Limit:  limit,
		Offset: offset,
	}

	if cafeIDStr := r.URL.Query().Get("cafe_id"); cafeIDStr != "" {
		cafeID, err := uuid.Parse(cafeIDStr)
		if err == nil {
			req.CafeID = &cafeID
		}
	}

	if categoryIDStr := r.URL.Query().Get("category_id"); categoryIDStr != "" {
		categoryID, err := uuid.Parse(categoryIDStr)
		if err == nil {
			req.CategoryID = &categoryID
		}
	}

	// Set tenant ID on request for repo
	_ = tenantID // Note: We need to pass this through the service

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

// GetPublicMenuItem retrieves a public menu item (no auth required).
// @Summary Get a public menu item
// @Description Retrieves a menu item for public display
// @Tags Menu
// @Produce json
// @Param id path string true "Menu item ID"
// @Param locale query string false "Locale for translations (default: en)"
// @Success 200 {object} catalog.PublicMenuItem
// @Failure 404 {object} handlers.ErrorResponse
// @Router /menu/items/{id} [get]
func (h *Handler) GetPublicMenuItem(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		tenantIDStr := r.URL.Query().Get("tenant_id")
		if tenantIDStr == "" {
			handlers.RespondError(w, http.StatusBadRequest, "tenant_id required")
			return
		}
		tenantID, err = uuid.Parse(tenantIDStr)
		if err != nil {
			handlers.RespondError(w, http.StatusBadRequest, "invalid tenant_id")
			return
		}
	}

	itemID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid item ID")
		return
	}

	item, err := h.service.GetMenuItem(r.Context(), tenantID, itemID)
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

// CreateVariant creates a new variant for a menu item.
// @Summary Create a menu item variant
// @Description Creates a new variant (e.g., size, flavor) for a menu item
// @Tags Catalog
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param id path string true "Menu item ID"
// @Param payload body CreateVariantRequest true "Variant data"
// @Success 201 {object} catalog.Variant
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Router /catalog/items/{id}/variants [post]
func (h *Handler) CreateVariant(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	menuItemID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid item ID")
		return
	}

	var req CreateVariantRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	variant, err := h.service.CreateVariant(r.Context(), catalog.CreateVariantRequest{
		TenantID:     tenantID,
		MenuItemID:   menuItemID,
		Name:         req.Name,
		PriceDelta:   req.PriceDelta,
		IsAvailable:  req.IsAvailable,
		SKU:          req.SKU,
		DisplayOrder: req.DisplayOrder,
	})
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusCreated, variant)
}

// ListVariants lists variants for a menu item.
// @Summary List menu item variants
// @Description Lists all variants for a menu item
// @Tags Catalog
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param id path string true "Menu item ID"
// @Success 200 {array} catalog.Variant
// @Failure 404 {object} handlers.ErrorResponse
// @Router /catalog/items/{id}/variants [get]
func (h *Handler) ListVariants(w http.ResponseWriter, r *http.Request) {
	menuItemID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid item ID")
		return
	}

	variants, err := h.service.ListVariants(r.Context(), menuItemID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, variants)
}

// CreateTranslation creates a new translation for a menu item.
// @Summary Create a menu item translation
// @Description Creates a localized translation for a menu item
// @Tags Catalog
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param id path string true "Menu item ID"
// @Param payload body CreateTranslationRequest true "Translation data"
// @Success 201 {object} catalog.Translation
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Failure 409 {object} handlers.ErrorResponse
// @Router /catalog/items/{id}/translations [post]
func (h *Handler) CreateTranslation(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	menuItemID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid item ID")
		return
	}

	var req CreateTranslationRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	translation, err := h.service.CreateTranslation(r.Context(), catalog.CreateTranslationRequest{
		TenantID:    tenantID,
		MenuItemID:  menuItemID,
		Locale:      req.Locale,
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusCreated, translation)
}

// ListTranslations lists translations for a menu item.
// @Summary List menu item translations
// @Description Lists all translations for a menu item
// @Tags Catalog
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param id path string true "Menu item ID"
// @Success 200 {array} catalog.Translation
// @Failure 404 {object} handlers.ErrorResponse
// @Router /catalog/items/{id}/translations [get]
func (h *Handler) ListTranslations(w http.ResponseWriter, r *http.Request) {
	menuItemID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid item ID")
		return
	}

	translations, err := h.service.ListTranslations(r.Context(), menuItemID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, translations)
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

	menuItemID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid item ID")
		return
	}

	var req AddDietaryTagRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.service.AddDietaryTagToItem(r.Context(), tenantID, menuItemID, req.Code); err != nil {
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
	menuItemID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid item ID")
		return
	}

	tagCode := chi.URLParam(r, "code")
	if tagCode == "" {
		handlers.RespondError(w, http.StatusBadRequest, "tag code required")
		return
	}

	if err := h.service.RemoveDietaryTagFromItem(r.Context(), menuItemID, tagCode); err != nil {
		h.handleError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
