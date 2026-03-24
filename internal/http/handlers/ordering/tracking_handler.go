package orderinghandler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/http/handlers"
	"github.com/bengobox/ordering-backend/internal/modules/identity"
	"github.com/bengobox/ordering-backend/internal/modules/ordering"
)

// TrackOrder streams real-time order status updates via Server-Sent Events (SSE).
// The connection stays open and pushes events whenever the order status changes.
// The stream closes automatically when the order reaches a terminal state
// (delivered, completed, or cancelled).
//
// @Summary Track order (SSE)
// @Description Server-Sent Events stream for real-time order tracking
// @Tags Orders
// @Produce text/event-stream
// @Param Authorization header string true "Bearer token"
// @Param orderId path string true "Order ID"
// @Success 200 {string} string "SSE stream"
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Failure 403 {object} handlers.ErrorResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Failure 500 {object} handlers.ErrorResponse
// @Router /orders/{orderId}/track [get]
func (h *OrderHandler) TrackOrder(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	user, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	orderID, err := uuid.Parse(chi.URLParam(r, "orderId"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid order ID")
		return
	}

	// Validate order exists and belongs to the authenticated user
	order, err := h.orderService.GetOrder(r.Context(), tenantID, orderID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	if order.CustomerID != user.ID && !user.HasPermission(identity.PermissionOrdersManage) {
		handlers.RespondError(w, http.StatusForbidden, "access denied")
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		handlers.RespondError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// Send initial order state
	sendSSEEvent(w, flusher, "order_status", order)

	// If already terminal, close immediately
	if isTerminalStatus(order.Status) {
		sendSSEEvent(w, flusher, "order_complete", order)
		return
	}

	// Poll for updates every 5 seconds
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	lastStatus := order.Status
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			current, err := h.orderService.GetOrder(r.Context(), tenantID, orderID)
			if err != nil {
				h.log.Debug("tracking poll error", zap.Error(err))
				continue
			}

			if current.Status != lastStatus {
				sendSSEEvent(w, flusher, "order_status", current)
				lastStatus = current.Status
			}

			// If order is out for delivery, include a rider location placeholder
			if current.Status == ordering.OrderStatusOutForDelivery {
				sendSSEEvent(w, flusher, "rider_location", map[string]interface{}{
					"status": "out_for_delivery",
				})
			}

			// Stop streaming when order reaches a terminal state
			if isTerminalStatus(current.Status) {
				sendSSEEvent(w, flusher, "order_complete", current)
				return
			}
		}
	}
}

// sendSSEEvent writes a single SSE event to the response writer and flushes.
func sendSSEEvent(w http.ResponseWriter, flusher http.Flusher, event string, data interface{}) {
	jsonData, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, jsonData)
	flusher.Flush()
}

// isTerminalStatus returns true if the order status is a terminal state.
func isTerminalStatus(status ordering.OrderStatus) bool {
	switch status {
	case ordering.OrderStatusDelivered, ordering.OrderStatusCompleted, ordering.OrderStatusCancelled, ordering.OrderStatusRefunded:
		return true
	default:
		return false
	}
}
