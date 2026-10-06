package orderinghandler

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/http/handlers"
	"github.com/bengobox/ordering-backend/internal/modules/identity"
	"github.com/bengobox/ordering-backend/internal/modules/ordering"
)

// RetryOrderPayment lets the signed-in customer pay an unpaid online order again.
// @Summary Retry an order payment
// @Description While the order's payment retry window is open, returns a payment intent to pay (the current one when it was never completed, else a fresh one) and its treasury initiate URL. Only the order's customer or staff with orders.manage.
// @Tags Orders
// @Produce json
// @Param orderId path string true "Order ID"
// @Success 200 {object} ordering.PaymentRetryResult
// @Failure 403 {object} handlers.ErrorResponse
// @Failure 409 {object} handlers.ErrorResponse
// @Failure 410 {object} handlers.ErrorResponse
// @Failure 429 {object} handlers.ErrorResponse
// @Router /orders/{orderId}/payment/retry [post]
func (h *OrderHandler) RetryOrderPayment(w http.ResponseWriter, r *http.Request) {
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
	order, err := h.orderService.GetOrder(r.Context(), tenantID, orderID)
	if err != nil {
		h.handleError(w, err)
		return
	}
	if (order.CustomerID == nil || *order.CustomerID != user.ID) && !user.HasPermission(identity.PermissionOrdersManage) {
		handlers.RespondError(w, http.StatusForbidden, "access denied")
		return
	}
	h.respondPaymentRetry(w, r, tenantID, orderID)
}

// RetryGuestOrderPayment is RetryOrderPayment for the public order page (emailed links and guest
// checkout). The unguessable order id is the capability, as on GET /orders/guest/{orderId}; when a
// session_id is sent it must match the order's.
// @Summary Retry a guest order payment
// @Tags Orders
// @Produce json
// @Param orderId path string true "Order ID"
// @Param session_id query string false "Guest session ID"
// @Success 200 {object} ordering.PaymentRetryResult
// @Failure 403 {object} handlers.ErrorResponse
// @Failure 409 {object} handlers.ErrorResponse
// @Failure 410 {object} handlers.ErrorResponse
// @Failure 429 {object} handlers.ErrorResponse
// @Router /orders/guest/{orderId}/payment/retry [post]
func (h *OrderHandler) RetryGuestOrderPayment(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}
	orderID, err := uuid.Parse(chi.URLParam(r, "orderId"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid order ID")
		return
	}
	if sessionID := r.URL.Query().Get("session_id"); sessionID != "" {
		order, gErr := h.orderService.GetOrder(r.Context(), tenantID, orderID)
		if gErr != nil {
			h.handleError(w, gErr)
			return
		}
		if order.Metadata == nil || order.Metadata["sessionId"] != sessionID {
			handlers.RespondError(w, http.StatusForbidden, "access denied")
			return
		}
	}
	h.respondPaymentRetry(w, r, tenantID, orderID)
}

func (h *OrderHandler) respondPaymentRetry(w http.ResponseWriter, r *http.Request, tenantID, orderID uuid.UUID) {
	result, err := h.orderService.RetryOrderPayment(r.Context(), tenantID, orderID)
	if err != nil {
		status := paymentRetryErrorStatus(err)
		if status == 0 {
			h.handleError(w, err)
			return
		}
		if status >= http.StatusInternalServerError {
			h.log.Warn("payment retry failed", zap.String("order_id", orderID.String()), zap.Error(err))
		}
		handlers.RespondError(w, status, err.Error())
		return
	}
	handlers.RespondJSON(w, http.StatusOK, result)
}

// paymentRetryErrorStatus maps retry refusals to HTTP statuses; 0 means "not a retry error".
func paymentRetryErrorStatus(err error) int {
	switch {
	case errors.Is(err, ordering.ErrPaymentAlreadyCompleted),
		errors.Is(err, ordering.ErrPaymentInFlight),
		errors.Is(err, ordering.ErrPaymentNotRetryable):
		return http.StatusConflict
	case errors.Is(err, ordering.ErrPaymentRetryClosed):
		return http.StatusGone
	case errors.Is(err, ordering.ErrPaymentRetryTooSoon), errors.Is(err, ordering.ErrPaymentRetryLimit):
		return http.StatusTooManyRequests
	case errors.Is(err, ordering.ErrTreasuryNotConfigured):
		return http.StatusServiceUnavailable
	case errors.Is(err, ordering.ErrPaymentInitiateFailed):
		return http.StatusBadGateway
	}
	return 0
}
