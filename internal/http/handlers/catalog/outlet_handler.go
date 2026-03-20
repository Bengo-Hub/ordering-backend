package cataloghandler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/bengobox/ordering-backend/internal/http/handlers"
)

// ListOutlets returns the list of outlets for the tenant (public, no auth).
// @Summary List outlets for tenant
// @Description Returns distinct outlets (outlets) that have menu data for the tenant
// @Tags Outlets
// @Produce json
// @Success 200 {array} catalog.OutletSummary
// @Router /outlets [get]
func (h *Handler) ListOutlets(w http.ResponseWriter, r *http.Request) {
	// Resolve tenant by slug from path (or X-Tenant-ID header) so public calls work with /api/v1/{tenant}/outlets
	tenantID, err := h.getTenantIDForPublic(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "tenant required (use path slug or X-Tenant-ID)")
		return
	}

	list, err := h.service.ListOutlets(r.Context(), tenantID)
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

// GetOutlet returns a single outlet by ID for the tenant (public, no auth).
// @Summary Get outlet by ID
// @Description Returns one outlet/outlet if it belongs to the tenant
// @Tags Outlets
// @Produce json
// @Param id path string true "Cafe ID"
// @Success 200 {object} catalog.OutletSummary
// @Failure 404 {object} handlers.ErrorResponse
// @Router /outlets/{id} [get]
func (h *Handler) GetOutlet(w http.ResponseWriter, r *http.Request) {
	tenantID, err := h.getTenantIDForPublic(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "tenant required (use path slug or X-Tenant-ID)")
		return
	}

	idStr := chi.URLParam(r, "id")
	outletID, err := uuid.Parse(idStr)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid outlet id")
		return
	}

	list, err := h.service.ListOutlets(r.Context(), tenantID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	for _, c := range list {
		if c.ID == outletID {
			handlers.RespondJSON(w, http.StatusOK, c)
			return
		}
	}

	handlers.RespondError(w, http.StatusNotFound, "outlet not found")
}
