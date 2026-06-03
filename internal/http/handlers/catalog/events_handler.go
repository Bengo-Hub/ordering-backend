package cataloghandler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/http/handlers"
	"github.com/bengobox/ordering-backend/internal/modules/catalog"
)

// ListPublicEvents lists a tenant's SERVICE-type events for the public storefront (no auth).
func (h *Handler) ListPublicEvents(w http.ResponseWriter, r *http.Request) {
	_, tenantSlug, err := h.resolveTenant(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	limit, _, page := getPagination(r)
	events, total, err := h.service.ListEvents(r.Context(), tenantSlug, limit, page)
	if err != nil {
		h.log.Error("list public events failed", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "failed to list events")
		return
	}
	handlers.RespondJSON(w, http.StatusOK, catalog.ListResponse{Data: events, Total: total, Limit: limit, Page: page})
}

// GetPublicEvent returns a single event with live per-tier availability for the public page (no auth).
func (h *Handler) GetPublicEvent(w http.ResponseWriter, r *http.Request) {
	_, tenantSlug, err := h.resolveTenant(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	eventID := chi.URLParam(r, "id")
	if eventID == "" {
		handlers.RespondError(w, http.StatusBadRequest, "event id is required")
		return
	}
	event, err := h.service.GetEventDetail(r.Context(), tenantSlug, eventID)
	if err != nil {
		handlers.RespondError(w, http.StatusNotFound, "event not found")
		return
	}
	handlers.RespondJSON(w, http.StatusOK, event)
}
