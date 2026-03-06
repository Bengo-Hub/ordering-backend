package cataloghandler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/bengobox/ordering-backend/internal/http/handlers"
)

// ListCafes returns the list of cafes/outlets for the tenant (public, no auth).
// @Summary List cafes for tenant
// @Description Returns distinct cafes (outlets) that have menu data for the tenant
// @Tags Menu
// @Produce json
// @Success 200 {array} catalog.CafeSummary
// @Router /cafes [get]
func (h *Handler) ListCafes(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		tenantIDStr := r.URL.Query().Get("tenant_id")
		if tenantIDStr == "" {
			handlers.RespondError(w, http.StatusBadRequest, "tenant_id or X-Tenant-ID required")
			return
		}
		tenantID, err = uuid.Parse(tenantIDStr)
		if err != nil {
			handlers.RespondError(w, http.StatusBadRequest, "invalid tenant_id")
			return
		}
	}

	list, err := h.service.ListCafes(r.Context(), tenantID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	// Return paginated shape so frontend can use same parser (data, total, limit, page)
	handlers.RespondJSON(w, http.StatusOK, ListResponse{
		Data:  list,
		Total: len(list),
		Limit: len(list),
		Page:  1,
	})
}

// GetCafe returns a single cafe by ID for the tenant (public, no auth).
// @Summary Get cafe by ID
// @Description Returns one cafe/outlet if it belongs to the tenant
// @Tags Menu
// @Produce json
// @Param id path string true "Cafe ID"
// @Success 200 {object} catalog.CafeSummary
// @Failure 404 {object} handlers.ErrorResponse
// @Router /cafes/{id} [get]
func (h *Handler) GetCafe(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "tenant required")
		return
	}

	idStr := chi.URLParam(r, "id")
	cafeID, err := uuid.Parse(idStr)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid cafe id")
		return
	}

	list, err := h.service.ListCafes(r.Context(), tenantID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	for _, c := range list {
		if c.ID == cafeID {
			handlers.RespondJSON(w, http.StatusOK, c)
			return
		}
	}

	handlers.RespondError(w, http.StatusNotFound, "cafe not found")
}
