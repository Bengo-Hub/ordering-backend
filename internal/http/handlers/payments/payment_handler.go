package paymentshandler

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/http/handlers"
	identityhandler "github.com/bengobox/ordering-backend/internal/http/handlers/identity"
	"github.com/bengobox/ordering-backend/internal/modules/identity"
	"github.com/bengobox/ordering-backend/internal/modules/payments"
)

// PaymentHandler exposes payment-related HTTP endpoints.
type PaymentHandler struct {
	log            *zap.Logger
	paymentService *payments.PaymentService
}

// NewPaymentHandler constructs a PaymentHandler instance.
func NewPaymentHandler(log *zap.Logger, paymentService *payments.PaymentService) *PaymentHandler {
	return &PaymentHandler{
		log:            log.Named("payments.PaymentHandler"),
		paymentService: paymentService,
	}
}

// Register mounts payment routes on the supplied router.
func (h *PaymentHandler) Register(r chi.Router, auth *identityhandler.Authenticator) {
	// Customer payment routes
	r.Route("/payments", func(paymentRouter chi.Router) {
		paymentRouter.Use(auth.RequireAuth)

		// Payment intents
		paymentRouter.Route("/intents", func(intentRouter chi.Router) {
			intentRouter.Post("/", h.CreatePaymentIntent)
			intentRouter.Get("/{intentId}", h.GetPaymentIntent)
			intentRouter.Post("/{intentId}/cancel", h.CancelPaymentIntent)
			intentRouter.Get("/{intentId}/status", h.CheckPaymentStatus)
		})

		// M-Pesa specific
		paymentRouter.Post("/mpesa/stk-push", h.InitiateMpesaPayment)

		// Payments list
		paymentRouter.Get("/", h.ListPayments)
		paymentRouter.Get("/{paymentId}", h.GetPayment)
	})

	// Admin refund routes
	r.Route("/admin/refunds", func(refundRouter chi.Router) {
		refundRouter.Use(auth.RequireAuth)
		refundRouter.Use(auth.RequirePermissions(identity.PermissionPaymentsManage))

		refundRouter.Post("/", h.CreateRefund)
		refundRouter.Get("/", h.ListRefunds)
		refundRouter.Get("/{refundId}", h.GetRefund)
	})
}

// --- Request/Response Types ---

// CreatePaymentIntentRequest represents a request to create a payment intent.
type CreatePaymentIntentRequest struct {
	OrderID         string  `json:"orderId"`
	Amount          float64 `json:"amount"`
	Currency        string  `json:"currency"`
	Provider        string  `json:"provider"`
	PaymentMethodID *string `json:"paymentMethodId,omitempty"`
	Description     string  `json:"description,omitempty"`
	IdempotencyKey  string  `json:"idempotencyKey,omitempty"`
}

// MpesaSTKPushRequest represents a request to initiate M-Pesa STK Push.
type MpesaSTKPushRequest struct {
	OrderID        string  `json:"orderId"`
	Amount         float64 `json:"amount"`
	PhoneNumber    string  `json:"phoneNumber"`
	Description    string  `json:"description,omitempty"`
	IdempotencyKey string  `json:"idempotencyKey,omitempty"`
}

// CreateRefundRequest represents a request to create a refund.
type CreateRefundRequest struct {
	PaymentID      string  `json:"paymentId"`
	Amount         float64 `json:"amount"`
	Reason         string  `json:"reason"`
	ReasonNotes    string  `json:"reasonNotes,omitempty"`
	IdempotencyKey string  `json:"idempotencyKey,omitempty"`
}

// PaymentIntentResponse represents a payment intent response.
type PaymentIntentResponse struct {
	ID                     string  `json:"id"`
	OrderID                string  `json:"orderId"`
	Provider               string  `json:"provider"`
	ClientSecret           string  `json:"clientSecret,omitempty"`
	Status                 string  `json:"status"`
	Amount                 float64 `json:"amount"`
	Currency               string  `json:"currency"`
	MpesaCheckoutRequestID string  `json:"mpesaCheckoutRequestId,omitempty"`
	ExpiresAt              *string `json:"expiresAt,omitempty"`
	CreatedAt              string  `json:"createdAt"`
}

// PaymentResponse represents a payment response.
type PaymentResponse struct {
	ID                string  `json:"id"`
	OrderID           string  `json:"orderId"`
	Amount            float64 `json:"amount"`
	Currency          string  `json:"currency"`
	Status            string  `json:"status"`
	Provider          string  `json:"provider"`
	ProviderReference string  `json:"providerReference,omitempty"`
	ProviderReceipt   string  `json:"providerReceipt,omitempty"`
	RefundedAmount    float64 `json:"refundedAmount"`
	ProcessedAt       *string `json:"processedAt,omitempty"`
	CreatedAt         string  `json:"createdAt"`
}

// RefundResponse represents a refund response.
type RefundResponse struct {
	ID          string  `json:"id"`
	PaymentID   string  `json:"paymentId"`
	OrderID     string  `json:"orderId"`
	Amount      float64 `json:"amount"`
	Currency    string  `json:"currency"`
	Status      string  `json:"status"`
	Reason      string  `json:"reason"`
	ReasonNotes string  `json:"reasonNotes,omitempty"`
	RequestedAt string  `json:"requestedAt"`
	ProcessedAt *string `json:"processedAt,omitempty"`
}

// ListPaymentsResponse represents a paginated payments list.
type ListPaymentsResponse struct {
	Data  []PaymentResponse `json:"data"`
	Total int               `json:"total"`
	Limit int               `json:"limit"`
	Page  int               `json:"page"`
}

// ListRefundsResponse represents a paginated refunds list.
type ListRefundsResponse struct {
	Data  []RefundResponse `json:"data"`
	Total int              `json:"total"`
	Limit int              `json:"limit"`
	Page  int              `json:"page"`
}

// --- Helper Functions ---

func getPaymentUserFromContext(r *http.Request) (*identity.User, uuid.UUID, error) {
	user, ok := identity.UserFromContext(r.Context())
	if !ok {
		return nil, uuid.Nil, errors.New("user not found in context")
	}
	tenantID, err := uuid.Parse(user.TenantID)
	if err != nil {
		return nil, uuid.Nil, errors.New("invalid tenant ID")
	}
	return user, tenantID, nil
}

func (h *PaymentHandler) handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, payments.ErrPaymentIntentNotFound):
		handlers.RespondError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, payments.ErrPaymentNotFound):
		handlers.RespondError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, payments.ErrRefundNotFound):
		handlers.RespondError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, payments.ErrInvalidAmount):
		handlers.RespondError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, payments.ErrInvalidPhoneNumber):
		handlers.RespondError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, payments.ErrPaymentIntentNotPending):
		handlers.RespondError(w, http.StatusConflict, err.Error())
	case errors.Is(err, payments.ErrPaymentNotSucceeded):
		handlers.RespondError(w, http.StatusConflict, err.Error())
	case errors.Is(err, payments.ErrRefundAmountExceedsPayment):
		handlers.RespondError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, payments.ErrDuplicatePaymentIntent):
		handlers.RespondError(w, http.StatusConflict, err.Error())
	default:
		h.log.Error("unexpected error", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "internal server error")
	}
}

func mapPaymentIntentToResponse(intent *payments.PaymentIntent) PaymentIntentResponse {
	resp := PaymentIntentResponse{
		ID:                     intent.ID.String(),
		OrderID:                intent.OrderID.String(),
		Provider:               string(intent.Provider),
		ClientSecret:           intent.ClientSecret,
		Status:                 string(intent.Status),
		Amount:                 intent.Amount,
		Currency:               intent.Currency,
		MpesaCheckoutRequestID: intent.MpesaCheckoutRequestID,
		CreatedAt:              intent.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if intent.ExpiresAt != nil {
		exp := intent.ExpiresAt.Format("2006-01-02T15:04:05Z")
		resp.ExpiresAt = &exp
	}
	return resp
}

func mapPaymentToResponse(p *payments.Payment) PaymentResponse {
	resp := PaymentResponse{
		ID:                p.ID.String(),
		OrderID:           p.OrderID.String(),
		Amount:            p.Amount,
		Currency:          p.Currency,
		Status:            string(p.Status),
		Provider:          string(p.Provider),
		ProviderReference: p.ProviderReference,
		ProviderReceipt:   p.ProviderReceipt,
		RefundedAmount:    p.RefundedAmount,
		CreatedAt:         p.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if p.ProcessedAt != nil {
		proc := p.ProcessedAt.Format("2006-01-02T15:04:05Z")
		resp.ProcessedAt = &proc
	}
	return resp
}

func mapRefundToResponse(r *payments.Refund) RefundResponse {
	resp := RefundResponse{
		ID:          r.ID.String(),
		PaymentID:   r.PaymentID.String(),
		OrderID:     r.OrderID.String(),
		Amount:      r.Amount,
		Currency:    r.Currency,
		Status:      string(r.Status),
		Reason:      string(r.Reason),
		ReasonNotes: r.ReasonNotes,
		RequestedAt: r.RequestedAt.Format("2006-01-02T15:04:05Z"),
	}
	if r.ProcessedAt != nil {
		proc := r.ProcessedAt.Format("2006-01-02T15:04:05Z")
		resp.ProcessedAt = &proc
	}
	return resp
}

// --- Endpoint Handlers ---

// CreatePaymentIntent creates a new payment intent.
// @Summary Create payment intent
// @Tags Payments
// @Accept json
// @Produce json
// @Param request body CreatePaymentIntentRequest true "Payment intent request"
// @Success 201 {object} PaymentIntentResponse
// @Router /payments/intents [post]
func (h *PaymentHandler) CreatePaymentIntent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, tenantID, err := getPaymentUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req CreatePaymentIntentRequest
	if err := handlers.DecodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	orderID, err := uuid.Parse(req.OrderID)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid orderId")
		return
	}

	var paymentMethodID *uuid.UUID
	if req.PaymentMethodID != nil {
		pmID, err := uuid.Parse(*req.PaymentMethodID)
		if err != nil {
			handlers.RespondError(w, http.StatusBadRequest, "invalid paymentMethodId")
			return
		}
		paymentMethodID = &pmID
	}

	currency := req.Currency
	if currency == "" {
		currency = "KES"
	}

	intent, err := h.paymentService.CreatePaymentIntent(ctx, payments.CreatePaymentIntentRequest{
		TenantID:        tenantID,
		OrderID:         orderID,
		Amount:          req.Amount,
		Currency:        currency,
		Provider:        payments.PaymentProvider(req.Provider),
		PaymentMethodID: paymentMethodID,
		Description:     req.Description,
		IdempotencyKey:  req.IdempotencyKey,
		CustomerEmail:   user.Email,
		CustomerPhone:   user.Phone,
	})
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusCreated, mapPaymentIntentToResponse(intent))
}

// InitiateMpesaPayment initiates an M-Pesa STK Push payment.
// @Summary Initiate M-Pesa STK Push
// @Tags Payments
// @Accept json
// @Produce json
// @Param request body MpesaSTKPushRequest true "M-Pesa STK Push request"
// @Success 201 {object} PaymentIntentResponse
// @Router /payments/mpesa/stk-push [post]
func (h *PaymentHandler) InitiateMpesaPayment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, tenantID, err := getPaymentUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req MpesaSTKPushRequest
	if err := handlers.DecodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	orderID, err := uuid.Parse(req.OrderID)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid orderId")
		return
	}

	intent, err := h.paymentService.InitiateMpesaPayment(ctx, payments.InitiateMpesaPaymentRequest{
		TenantID:       tenantID,
		OrderID:        orderID,
		Amount:         req.Amount,
		PhoneNumber:    req.PhoneNumber,
		Description:    req.Description,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusCreated, mapPaymentIntentToResponse(intent))
}

// GetPaymentIntent retrieves a payment intent by ID.
// @Summary Get payment intent
// @Tags Payments
// @Produce json
// @Param intentId path string true "Payment Intent ID"
// @Success 200 {object} PaymentIntentResponse
// @Router /payments/intents/{intentId} [get]
func (h *PaymentHandler) GetPaymentIntent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, tenantID, err := getPaymentUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	intentID, err := uuid.Parse(chi.URLParam(r, "intentId"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid intentId")
		return
	}

	intent, err := h.paymentService.GetPaymentIntent(ctx, tenantID, intentID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, mapPaymentIntentToResponse(intent))
}

// CancelPaymentIntent cancels a pending payment intent.
// @Summary Cancel payment intent
// @Tags Payments
// @Param intentId path string true "Payment Intent ID"
// @Success 204
// @Router /payments/intents/{intentId}/cancel [post]
func (h *PaymentHandler) CancelPaymentIntent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, tenantID, err := getPaymentUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	intentID, err := uuid.Parse(chi.URLParam(r, "intentId"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid intentId")
		return
	}

	if err := h.paymentService.CancelPaymentIntent(ctx, tenantID, intentID); err != nil {
		h.handleError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// CheckPaymentStatus checks the current status of a payment intent.
// @Summary Check payment status
// @Tags Payments
// @Produce json
// @Param intentId path string true "Payment Intent ID"
// @Success 200 {object} PaymentIntentResponse
// @Router /payments/intents/{intentId}/status [get]
func (h *PaymentHandler) CheckPaymentStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, tenantID, err := getPaymentUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	intentID, err := uuid.Parse(chi.URLParam(r, "intentId"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid intentId")
		return
	}

	intent, err := h.paymentService.CheckPaymentStatus(ctx, tenantID, intentID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, mapPaymentIntentToResponse(intent))
}

// GetPayment retrieves a payment by ID.
// @Summary Get payment
// @Tags Payments
// @Produce json
// @Param paymentId path string true "Payment ID"
// @Success 200 {object} PaymentResponse
// @Router /payments/{paymentId} [get]
func (h *PaymentHandler) GetPayment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, tenantID, err := getPaymentUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	paymentID, err := uuid.Parse(chi.URLParam(r, "paymentId"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid paymentId")
		return
	}

	payment, err := h.paymentService.GetPayment(ctx, tenantID, paymentID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, mapPaymentToResponse(payment))
}

// ListPayments lists payments with optional filters.
// @Summary List payments
// @Tags Payments
// @Produce json
// @Param orderId query string false "Filter by order ID"
// @Param limit query int false "Limit" default(20)
// @Param page query int false "Page" default(1)
// @Success 200 {object} ListPaymentsResponse
// @Router /payments [get]
func (h *PaymentHandler) ListPayments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, tenantID, err := getPaymentUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	limit := handlers.ParseInt(r.URL.Query().Get("limit"), 20)
	if limit < 1 || limit > 100 {
		limit = 20
	}
	page := handlers.ParseInt(r.URL.Query().Get("page"), 1)
	if page < 1 {
		page = 1
	}

	filter := payments.PaymentFilter{
		TenantID: tenantID,
		Limit:    limit,
		Offset:   (page - 1) * limit,
	}

	if orderIDStr := r.URL.Query().Get("orderId"); orderIDStr != "" {
		orderID, err := uuid.Parse(orderIDStr)
		if err == nil {
			filter.OrderID = &orderID
		}
	}

	paymentsList, total, err := h.paymentService.ListPayments(ctx, filter)
	if err != nil {
		h.handleError(w, err)
		return
	}

	data := make([]PaymentResponse, len(paymentsList))
	for i, p := range paymentsList {
		data[i] = mapPaymentToResponse(&p)
	}

	handlers.RespondJSON(w, http.StatusOK, ListPaymentsResponse{
		Data:  data,
		Total: total,
		Limit: limit,
		Page:  page,
	})
}

// CreateRefund creates a new refund request.
// @Summary Create refund
// @Tags Refunds
// @Accept json
// @Produce json
// @Param request body CreateRefundRequest true "Refund request"
// @Success 201 {object} RefundResponse
// @Router /admin/refunds [post]
func (h *PaymentHandler) CreateRefund(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, tenantID, err := getPaymentUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req CreateRefundRequest
	if err := handlers.DecodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	paymentID, err := uuid.Parse(req.PaymentID)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid paymentId")
		return
	}

	refund, err := h.paymentService.CreateRefund(ctx, payments.CreateRefundRequest{
		TenantID:       tenantID,
		PaymentID:      paymentID,
		Amount:         req.Amount,
		Reason:         payments.RefundReason(req.Reason),
		ReasonNotes:    req.ReasonNotes,
		RequestedBy:    user.ID,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusCreated, mapRefundToResponse(refund))
}

// GetRefund retrieves a refund by ID.
// @Summary Get refund
// @Tags Refunds
// @Produce json
// @Param refundId path string true "Refund ID"
// @Success 200 {object} RefundResponse
// @Router /admin/refunds/{refundId} [get]
func (h *PaymentHandler) GetRefund(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, tenantID, err := getPaymentUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	refundID, err := uuid.Parse(chi.URLParam(r, "refundId"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid refundId")
		return
	}

	refund, err := h.paymentService.GetRefund(ctx, tenantID, refundID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, mapRefundToResponse(refund))
}

// ListRefunds lists refunds with optional filters.
// @Summary List refunds
// @Tags Refunds
// @Produce json
// @Param paymentId query string false "Filter by payment ID"
// @Param orderId query string false "Filter by order ID"
// @Param limit query int false "Limit" default(20)
// @Param page query int false "Page" default(1)
// @Success 200 {object} ListRefundsResponse
// @Router /admin/refunds [get]
func (h *PaymentHandler) ListRefunds(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, tenantID, err := getPaymentUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	limit := handlers.ParseInt(r.URL.Query().Get("limit"), 20)
	if limit < 1 || limit > 100 {
		limit = 20
	}
	page := handlers.ParseInt(r.URL.Query().Get("page"), 1)
	if page < 1 {
		page = 1
	}

	filter := payments.RefundFilter{
		TenantID: tenantID,
		Limit:    limit,
		Offset:   (page - 1) * limit,
	}

	if paymentIDStr := r.URL.Query().Get("paymentId"); paymentIDStr != "" {
		paymentID, err := uuid.Parse(paymentIDStr)
		if err == nil {
			filter.PaymentID = &paymentID
		}
	}

	if orderIDStr := r.URL.Query().Get("orderId"); orderIDStr != "" {
		orderID, err := uuid.Parse(orderIDStr)
		if err == nil {
			filter.OrderID = &orderID
		}
	}

	refunds, total, err := h.paymentService.ListRefunds(ctx, filter)
	if err != nil {
		h.handleError(w, err)
		return
	}

	data := make([]RefundResponse, len(refunds))
	for i, ref := range refunds {
		data[i] = mapRefundToResponse(&ref)
	}

	handlers.RespondJSON(w, http.StatusOK, ListRefundsResponse{
		Data:  data,
		Total: total,
		Limit: limit,
		Page:  page,
	})
}
