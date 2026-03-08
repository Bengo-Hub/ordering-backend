package cataloghandler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/bengobox/ordering-backend/internal/http/handlers"
	"github.com/bengobox/ordering-backend/internal/modules/catalog"
)

// CreateCategory creates a new menu category.
// @Summary Create a new category
// @Description Creates a new menu category for organizing menu items
// @Tags Catalog
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param payload body CreateCategoryRequest true "Category data"
// @Success 201 {object} catalog.Category
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Failure 409 {object} handlers.ErrorResponse
// @Router /catalog/categories [post]
func (h *Handler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	var req CreateCategoryRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	cafeID, err := uuid.Parse(req.CafeID)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid cafe ID")
		return
	}

	var parentID *uuid.UUID
	if req.ParentID != nil && *req.ParentID != "" {
		pid, err := uuid.Parse(*req.ParentID)
		if err != nil {
			handlers.RespondError(w, http.StatusBadRequest, "invalid parent ID")
			return
		}
		parentID = &pid
	}

	category, err := h.service.CreateCategory(r.Context(), catalog.CreateCategoryRequest{
		TenantID:     tenantID,
		CafeID:       cafeID,
		ParentID:     parentID,
		Name:         req.Name,
		Description:  req.Description,
		DisplayOrder: req.DisplayOrder,
		IsActive:     req.IsActive,
		ImageURL:     req.ImageURL,
	})
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusCreated, category)
}

// GetCategory retrieves a category by ID.
// @Summary Get a category
// @Description Retrieves a menu category by its ID
// @Tags Catalog
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param id path string true "Category ID"
// @Success 200 {object} catalog.Category
// @Failure 404 {object} handlers.ErrorResponse
// @Router /catalog/categories/{id} [get]
func (h *Handler) GetCategory(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	categoryID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid category ID")
		return
	}

	category, err := h.service.GetCategory(r.Context(), tenantID, categoryID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, category)
}

// ListCategories lists all categories with optional filters.
// @Summary List categories
// @Description Lists all menu categories with optional filters
// @Tags Catalog
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param cafe_id query string false "Filter by cafe ID"
// @Param parent_id query string false "Filter by parent ID"
// @Param is_active query boolean false "Filter by active status"
// @Param search query string false "Search by name"
// @Param limit query integer false "Page size (default 50)"
// @Param page query integer false "Page number (default 1)"
// @Success 200 {object} ListResponse
// @Router /catalog/categories [get]
func (h *Handler) ListCategories(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	limit, offset, page := getPagination(r)
	filter := catalog.CategoryFilter{
		TenantID: tenantID,
		Search:   r.URL.Query().Get("search"),
		Limit:    limit,
		Offset:   offset,
	}

	if cafeIDStr := r.URL.Query().Get("cafe_id"); cafeIDStr != "" {
		cafeID, err := uuid.Parse(cafeIDStr)
		if err == nil {
			filter.CafeID = &cafeID
		}
	}

	if parentIDStr := r.URL.Query().Get("parent_id"); parentIDStr != "" {
		parentID, err := uuid.Parse(parentIDStr)
		if err == nil {
			filter.ParentID = &parentID
		}
	}

	if isActiveStr := r.URL.Query().Get("is_active"); isActiveStr != "" {
		isActive := isActiveStr == "true"
		filter.IsActive = &isActive
	}

	categories, total, err := h.service.ListCategories(r.Context(), filter)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, ListResponse{
		Data:  categories,
		Total: total,
		Limit: limit,
		Page:  page,
	})
}

// UpdateCategory updates an existing category.
// @Summary Update a category
// @Description Updates an existing menu category
// @Tags Catalog
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param id path string true "Category ID"
// @Param payload body UpdateCategoryRequest true "Updated category data"
// @Success 200 {object} catalog.Category
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Failure 409 {object} handlers.ErrorResponse
// @Router /catalog/categories/{id} [put]
func (h *Handler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	categoryID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid category ID")
		return
	}

	var req UpdateCategoryRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	updateReq := catalog.UpdateCategoryRequest{
		TenantID:     tenantID,
		CategoryID:   categoryID,
		Name:         req.Name,
		Description:  req.Description,
		DisplayOrder: req.DisplayOrder,
		IsActive:     req.IsActive,
		ImageURL:     req.ImageURL,
		ClearParent:  req.ClearParent,
	}

	if req.ParentID != nil && *req.ParentID != "" {
		pid, err := uuid.Parse(*req.ParentID)
		if err != nil {
			handlers.RespondError(w, http.StatusBadRequest, "invalid parent ID")
			return
		}
		updateReq.ParentID = &pid
	}

	category, err := h.service.UpdateCategory(r.Context(), updateReq)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, category)
}

// DeleteCategory deletes a category.
// @Summary Delete a category
// @Description Deletes a menu category if it has no items or children
// @Tags Catalog
// @Param Authorization header string true "Bearer token"
// @Param id path string true "Category ID"
// @Success 204 "No Content"
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Router /catalog/categories/{id} [delete]
func (h *Handler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	categoryID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid category ID")
		return
	}

	if err := h.service.DeleteCategory(r.Context(), tenantID, categoryID); err != nil {
		h.handleError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListPublicCategories lists public categories (no auth required).
// @Summary List public categories
// @Description Lists active menu categories for public display. Tenant can be provided via X-Tenant-ID (UUID), X-Tenant-Slug, or URL path. cafe_id is optional; when omitted, the first outlet for the tenant is used.
// @Tags Menu
// @Produce json
// @Param cafe_id query string false "Cafe ID (optional; defaults to first outlet)"
// @Success 200 {array} catalog.PublicCategory
// @Router /menu/categories [get]
func (h *Handler) ListPublicCategories(w http.ResponseWriter, r *http.Request) {
	tenantID, err := h.getTenantIDForPublic(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var cafeID uuid.UUID
	cafeIDStr := r.URL.Query().Get("cafe_id")
	if cafeIDStr != "" {
		cafeID, err = uuid.Parse(cafeIDStr)
		if err != nil {
			handlers.RespondError(w, http.StatusBadRequest, "invalid cafe_id")
			return
		}
	} else {
		cafes, err := h.service.ListCafes(r.Context(), tenantID)
		if err != nil || len(cafes) == 0 {
			handlers.RespondJSON(w, http.StatusOK, []catalog.PublicCategory{})
			return
		}
		cafeID = cafes[0].ID
	}

	categories, err := h.service.GetPublicCategories(r.Context(), tenantID, cafeID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, categories)
}
