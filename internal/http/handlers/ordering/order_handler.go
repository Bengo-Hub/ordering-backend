package orderinghandler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/http/handlers"
	identityhandler "github.com/bengobox/ordering-backend/internal/http/handlers/identity"
	"github.com/bengobox/ordering-backend/internal/modules/fulfilment"
	"github.com/bengobox/ordering-backend/internal/modules/identity"
	"github.com/bengobox/ordering-backend/internal/modules/ordering"
)

// OrderHandler exposes order-related HTTP endpoints.
type OrderHandler struct {
	log          *zap.Logger
	orderService *ordering.OrderService
	taskService  *fulfilment.TaskService
}

// NewOrderHandler constructs an OrderHandler instance.
func NewOrderHandler(log *zap.Logger, orderService *ordering.OrderService) *OrderHandler {
	return &OrderHandler{
		log:          log.Named("ordering.OrderHandler"),
		orderService: orderService,
	}
}

// SetTaskService sets the fulfilment task service for tracking integration.
func (h *OrderHandler) SetTaskService(ts *fulfilment.TaskService) {
	h.taskService = ts
}

// Register mounts order routes on the supplied router.
func (h *OrderHandler) Register(r chi.Router, auth *identityhandler.Authenticator) {
	// Customer order routes
	r.Route("/orders", func(orderRouter chi.Router) {
		orderRouter.Use(auth.RequireAuth)

		// Customer-facing endpoints
		orderRouter.Get("/", h.ListOrders)
		orderRouter.Post("/", h.CreateOrder)
		orderRouter.Get("/{orderId}", h.GetOrder)
		orderRouter.Post("/{orderId}/cancel", h.CancelOrder)
		orderRouter.Post("/{orderId}/rate", h.RateOrder)
		orderRouter.Post("/{orderId}/reorder", h.Reorder)

		// Live order tracking (SSE)
		orderRouter.Get("/{orderId}/track", h.TrackOrder)
	})

	// Outlet rating (public, no auth required)
	r.Get("/outlets/{outletId}/rating", h.GetOutletRating)

	// Checkout endpoint
	r.Route("/checkout", func(checkoutRouter chi.Router) {
		// Guest checkout (no auth required)
		checkoutRouter.Post("/guest", h.GuestCheckout)

		// Authenticated checkout (grouped to avoid middleware-after-routes panic)
		checkoutRouter.Group(func(authedCheckout chi.Router) {
			authedCheckout.Use(auth.RequireAuth)
			authedCheckout.Post("/", h.Checkout)
			authedCheckout.Post("/validate", h.ValidateCheckout)
		})
	})

	// Admin order management routes
	r.Route("/admin/orders", func(adminRouter chi.Router) {
		adminRouter.Use(auth.RequireAuth)
		adminRouter.Use(auth.RequirePermissions(identity.PermissionOrdersManage))

		adminRouter.Get("/", h.AdminListOrders)
		adminRouter.Get("/summary", h.GetAnalyticsSummary)
		adminRouter.Post("/bulk-status", h.BulkUpdateOrderStatus)
		adminRouter.Get("/{orderId}", h.AdminGetOrder)
		adminRouter.Get("/{orderId}/events", h.GetOrderEvents)
		adminRouter.Put("/{orderId}/status", h.UpdateOrderStatus)
		adminRouter.Post("/{orderId}/cancel", h.AdminCancelOrder)
		adminRouter.Post("/{orderId}/refund", h.RefundOrder)
	})
}

// --- Request/Response Types ---

// CheckoutRequestDTO represents a checkout request.
type CheckoutRequestDTO struct {
	CartID                string  `json:"cartId"`
	DeliveryAddressID     *string `json:"deliveryAddressId,omitempty"`
	PromoCode             string  `json:"promoCode,omitempty"`
	LoyaltyPointsRedeemed int     `json:"loyaltyPointsRedeemed,omitempty"`
	Instructions          string  `json:"instructions,omitempty"`
	Channel               string  `json:"channel,omitempty"`
	IdempotencyKey        string  `json:"idempotencyKey,omitempty"`
	FulfillmentType       string  `json:"fulfillmentType,omitempty"`
	ScheduledAt           string  `json:"scheduledAt,omitempty"`
}

// UpdateStatusRequest represents a request to update order status.
type UpdateStatusRequest struct {
	Status string `json:"status"`
}

// CancelOrderRequest represents a request to cancel an order.
type CancelOrderRequest struct {
	Reason string `json:"reason"`
}

type RateOrderRequest struct {
	Rating       int    `json:"rating"`
	Comment      string `json:"comment"`
	RiderRating  *int   `json:"riderRating,omitempty"`
	RiderComment string `json:"riderComment,omitempty"`
}

// GuestCheckoutRequestDTO is the request body for POST /checkout/guest (guest checkout, no auth).
type GuestCheckoutRequestDTO struct {
	OutletID        string   `json:"outletId"`
	SessionID       string   `json:"sessionId"`
	ContactEmail    string   `json:"contactEmail"`
	ContactPhone    string   `json:"contactPhone"`
	DeliveryAddress string   `json:"deliveryAddress"`
	DeliveryLat     *float64 `json:"deliveryLat,omitempty"`
	DeliveryLng     *float64 `json:"deliveryLng,omitempty"`
	DeliveryNotes   string   `json:"deliveryNotes,omitempty"`
	PaymentMethod   string   `json:"paymentMethod"` // "mpesa" | "cod"
	Instructions    string   `json:"instructions,omitempty"`
	Channel         string   `json:"channel,omitempty"`
}

// CreateOrderRequestDTO is the request body for POST /orders (create order from items, frontend contract).
type CreateOrderRequestDTO struct {
	OutletID         string         `json:"outletId"`
	Items            []OrderItemDTO `json:"items"`
	DeliveryAddress  string         `json:"deliveryAddress"`
	DeliveryLat      *float64       `json:"deliveryLat,omitempty"`
	DeliveryLng      *float64       `json:"deliveryLng,omitempty"`
	DeliveryNotes    string         `json:"deliveryNotes,omitempty"`
	PaymentMethod    string         `json:"paymentMethod"` // "mpesa" | "cod"
	PromoCode        string         `json:"promoCode,omitempty"`
	FulfillmentType  string         `json:"fulfillmentType,omitempty"`
	ScheduledAt      string         `json:"scheduledAt,omitempty"`
}

// OrderItemDTO is a single item in CreateOrderRequestDTO.
type OrderItemDTO struct {
	InventorySKU string  `json:"inventorySku"`
	Name         string  `json:"name"`
	Quantity     int     `json:"quantity"`
	UnitPrice    float64 `json:"unitPrice"`
	TotalPrice   float64 `json:"totalPrice"`
}

// ListOrdersResponse represents the paginated order list response.
type ListOrdersResponse struct {
	Data  []ordering.Order `json:"data"`
	Total int              `json:"total"`
	Limit int              `json:"limit"`
	Page  int              `json:"page"`
}

// --- Helper Functions ---

func (h *OrderHandler) handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ordering.ErrSubscriptionRequired):
		handlers.RespondError(w, http.StatusPaymentRequired, err.Error())

	case errors.Is(err, ordering.ErrOrderNotFound):
		handlers.RespondError(w, http.StatusNotFound, err.Error())

	case errors.Is(err, ordering.ErrCartNotFound),
		errors.Is(err, ordering.ErrCartItemNotFound):
		handlers.RespondError(w, http.StatusNotFound, err.Error())

	case errors.Is(err, ordering.ErrOrderAlreadyExists):
		handlers.RespondError(w, http.StatusConflict, err.Error())

	case errors.Is(err, ordering.ErrInvalidStatusTransition),
		errors.Is(err, ordering.ErrOrderCannotBeCancelled),
		errors.Is(err, ordering.ErrOrderNotRefundable),
		errors.Is(err, ordering.ErrOrderAlreadyRefunded):
		handlers.RespondError(w, http.StatusConflict, err.Error())

	case errors.Is(err, ordering.ErrRefundFailed):
		handlers.RespondError(w, http.StatusBadGateway, err.Error())

	case errors.Is(err, ordering.ErrCartEmpty),
		errors.Is(err, ordering.ErrCartAlreadyCheckedOut),
		errors.Is(err, ordering.ErrInvalidDeliveryAddress),
		errors.Is(err, ordering.ErrInvalidQuantity),
		errors.Is(err, ordering.ErrInvalidOrderStatus),
		errors.Is(err, ordering.ErrPromoCodeNotFound),
		errors.Is(err, ordering.ErrPromoCodeExpired),
		errors.Is(err, ordering.ErrPromoCodeMaxUses),
		errors.Is(err, ordering.ErrInsufficientLoyaltyPoints),
		errors.Is(err, ordering.ErrScheduledForRequired),
		errors.Is(err, ordering.ErrScheduledForTooSoon):
		handlers.RespondError(w, http.StatusBadRequest, err.Error())

	case errors.Is(err, ordering.ErrUnauthorized):
		handlers.RespondError(w, http.StatusForbidden, err.Error())

	default:
		h.log.Error("internal error", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "internal server error")
	}
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

func parseOrderStatus(status string) (ordering.OrderStatus, error) {
	switch status {
	case "pending":
		return ordering.OrderStatusPending, nil
	case "confirmed":
		return ordering.OrderStatusConfirmed, nil
	case "preparing":
		return ordering.OrderStatusPreparing, nil
	case "ready":
		return ordering.OrderStatusReady, nil
	case "out_for_delivery":
		return ordering.OrderStatusOutForDelivery, nil
	case "delivered":
		return ordering.OrderStatusDelivered, nil
	case "completed":
		return ordering.OrderStatusCompleted, nil
	case "cancelled":
		return ordering.OrderStatusCancelled, nil
	case "refunded":
		return ordering.OrderStatusRefunded, nil
	default:
		return "", errors.New("invalid order status")
	}
}

func parseOrderChannel(channel string) ordering.OrderChannel {
	switch channel {
	case "mobile_app":
		return ordering.OrderChannelMobileApp
	case "kiosk":
		return ordering.OrderChannelKiosk
	case "phone":
		return ordering.OrderChannelPhone
	case "api":
		return ordering.OrderChannelAPI
	default:
		return ordering.OrderChannelWeb
	}
}

func getClientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	return r.RemoteAddr
}

// --- Handlers ---

// Checkout processes a cart checkout and creates an order.
// @Summary Checkout cart
// @Description Creates an order from the current cart
// @Tags Checkout
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param payload body CheckoutRequestDTO true "Checkout data"
// @Success 201 {object} ordering.Order
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Failure 409 {object} handlers.ErrorResponse
// @Router /checkout [post]
func (h *OrderHandler) Checkout(w http.ResponseWriter, r *http.Request) {
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

	var req CheckoutRequestDTO
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	cartID, err := uuid.Parse(req.CartID)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid cart ID")
		return
	}

	var deliveryAddressID *uuid.UUID
	if req.DeliveryAddressID != nil && *req.DeliveryAddressID != "" {
		id, err := uuid.Parse(*req.DeliveryAddressID)
		if err != nil {
			handlers.RespondError(w, http.StatusBadRequest, "invalid delivery address ID")
			return
		}
		deliveryAddressID = &id
	}

	// Parse fulfillment type and scheduled time
	fulfillmentType := ordering.FulfillmentType(req.FulfillmentType)
	var scheduledFor *time.Time
	if req.ScheduledAt != "" {
		t, err := time.Parse(time.RFC3339, req.ScheduledAt)
		if err != nil {
			handlers.RespondError(w, http.StatusBadRequest, "invalid scheduledAt format, expected RFC3339")
			return
		}
		scheduledFor = &t
	}

	order, err := h.orderService.Checkout(r.Context(), ordering.CheckoutRequest{
		TenantID:              tenantID,
		CartID:                cartID,
		UserID:                user.ID,
		DeliveryAddressID:     deliveryAddressID,
		PromoCode:             req.PromoCode,
		LoyaltyPointsRedeemed: req.LoyaltyPointsRedeemed,
		Instructions:          req.Instructions,
		Channel:               parseOrderChannel(req.Channel),
		IdempotencyKey:        req.IdempotencyKey,
		FulfillmentType:       fulfillmentType,
		ScheduledFor:          scheduledFor,
	})
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusCreated, order)
}

// ValidateCheckout validates checkout data without creating an order.
// @Summary Validate checkout
// @Description Validates checkout data and returns any errors
// @Tags Checkout
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param payload body CheckoutRequestDTO true "Checkout data"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Router /checkout/validate [post]
func (h *OrderHandler) ValidateCheckout(w http.ResponseWriter, r *http.Request) {
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

	var req CheckoutRequestDTO
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	validationErrors := make([]string, 0)

	// Validate cart ID
	if req.CartID == "" {
		validationErrors = append(validationErrors, "cart ID is required")
	} else if _, err := uuid.Parse(req.CartID); err != nil {
		validationErrors = append(validationErrors, "invalid cart ID format")
	}

	// Validate delivery address if provided
	if req.DeliveryAddressID != nil && *req.DeliveryAddressID != "" {
		if _, err := uuid.Parse(*req.DeliveryAddressID); err != nil {
			validationErrors = append(validationErrors, "invalid delivery address ID format")
		}
	}

	// Validate loyalty points
	if req.LoyaltyPointsRedeemed < 0 {
		validationErrors = append(validationErrors, "loyalty points redeemed cannot be negative")
	}

	if len(validationErrors) > 0 {
		handlers.RespondJSON(w, http.StatusOK, map[string]interface{}{
			"valid":  false,
			"errors": validationErrors,
		})
		return
	}

	// Additional validation could be done here (cart exists, has items, etc.)
	_ = tenantID
	_ = user

	handlers.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"valid":  true,
		"errors": []string{},
	})
}

// CreateOrder creates an order from a list of items (convenience endpoint matching frontend contract).
// @Summary Create order from items
// @Description Creates an order directly from items (outletId, items, deliveryAddress, paymentMethod). Alternative to cart + checkout flow.
// @Tags Orders
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param payload body CreateOrderRequestDTO true "Order data"
// @Success 201 {object} ordering.Order
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Failure 409 {object} handlers.ErrorResponse
// @Router /orders [post]
func (h *OrderHandler) CreateOrder(w http.ResponseWriter, r *http.Request) {
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

	var req CreateOrderRequestDTO
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Pickup orders don't require a delivery address
	isPickup := req.FulfillmentType == "pickup"
	if req.OutletID == "" || len(req.Items) == 0 {
		handlers.RespondError(w, http.StatusBadRequest, "outletId and items are required")
		return
	}
	if !isPickup && req.DeliveryAddress == "" {
		handlers.RespondError(w, http.StatusBadRequest, "deliveryAddress is required for delivery orders")
		return
	}

	outletID, err := uuid.Parse(req.OutletID)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid outletId")
		return
	}

	items := make([]ordering.CreateOrderItemInput, 0, len(req.Items))
	for _, it := range req.Items {
		if it.InventorySKU == "" {
			handlers.RespondError(w, http.StatusBadRequest, "inventorySku is required for each item")
			return
		}
		items = append(items, ordering.CreateOrderItemInput{
			InventorySKU: it.InventorySKU,
			Name:         it.Name,
			Quantity:     it.Quantity,
			UnitPrice:    it.UnitPrice,
			TotalPrice:   it.TotalPrice,
		})
	}

	channel := ordering.OrderChannelWeb
	if r.Header.Get("X-Order-Channel") != "" {
		channel = ordering.OrderChannel(r.Header.Get("X-Order-Channel"))
	}

	// Parse fulfillment type and scheduled time
	fulfillmentType := ordering.FulfillmentType(req.FulfillmentType)
	var scheduledFor *time.Time
	if req.ScheduledAt != "" {
		t, err := time.Parse(time.RFC3339, req.ScheduledAt)
		if err != nil {
			handlers.RespondError(w, http.StatusBadRequest, "invalid scheduledAt format, expected RFC3339")
			return
		}
		scheduledFor = &t
	}

	order, err := h.orderService.CreateOrderFromItems(r.Context(), ordering.CreateOrderFromItemsRequest{
		TenantID:        tenantID,
		OutletID:        outletID,
		UserID:          user.ID,
		Items:           items,
		DeliveryAddress: req.DeliveryAddress,
		DeliveryLat:     req.DeliveryLat,
		DeliveryLng:     req.DeliveryLng,
		DeliveryNotes:   req.DeliveryNotes,
		PaymentMethod:   req.PaymentMethod,
		PromoCode:       req.PromoCode,
		Channel:         channel,
		FulfillmentType: fulfillmentType,
		ScheduledFor:    scheduledFor,
	})
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusCreated, order)
}

// GetOrder retrieves an order by ID.
// @Summary Get order
// @Description Retrieves an order by its ID
// @Tags Orders
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param orderId path string true "Order ID"
// @Success 200 {object} ordering.Order
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Failure 403 {object} handlers.ErrorResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Router /orders/{orderId} [get]
func (h *OrderHandler) GetOrder(w http.ResponseWriter, r *http.Request) {
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

	// Verify order belongs to user (unless they have orders:manage permission)
	if order.CustomerID != user.ID && !user.HasPermission(identity.PermissionOrdersManage) {
		handlers.RespondError(w, http.StatusForbidden, "access denied")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, order)
}

// ListOrders lists the current user's orders.
// @Summary List orders
// @Description Lists orders for the current user with optional filters
// @Tags Orders
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param status query string false "Filter by status"
// @Param date_from query string false "Filter orders from date (RFC3339)"
// @Param date_to query string false "Filter orders to date (RFC3339)"
// @Param limit query integer false "Page size (default 50)"
// @Param page query integer false "Page number (default 1)"
// @Success 200 {object} ListOrdersResponse
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Router /orders [get]
func (h *OrderHandler) ListOrders(w http.ResponseWriter, r *http.Request) {
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

	limit, offset, page := getPagination(r)

	filter := ordering.OrderFilter{
		TenantID:   tenantID,
		CustomerID: &user.ID,
		Limit:      limit,
		Offset:     offset,
	}

	// Parse optional filters
	if statusStr := r.URL.Query().Get("status"); statusStr != "" {
		status, err := parseOrderStatus(statusStr)
		if err == nil {
			filter.Status = &status
		}
	}

	if dateFromStr := r.URL.Query().Get("date_from"); dateFromStr != "" {
		if dateFrom, err := time.Parse(time.RFC3339, dateFromStr); err == nil {
			filter.DateFrom = &dateFrom
		}
	}

	if dateToStr := r.URL.Query().Get("date_to"); dateToStr != "" {
		if dateTo, err := time.Parse(time.RFC3339, dateToStr); err == nil {
			filter.DateTo = &dateTo
		}
	}

	orders, total, err := h.orderService.ListOrders(r.Context(), filter)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, ListOrdersResponse{
		Data:  orders,
		Total: total,
		Limit: limit,
		Page:  page,
	})
}

// CancelOrder cancels an order.
// @Summary Cancel order
// @Description Cancels an order with a reason (customer endpoint)
// @Tags Orders
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param orderId path string true "Order ID"
// @Param payload body CancelOrderRequest true "Cancellation reason"
// @Success 200 {object} ordering.Order
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Failure 403 {object} handlers.ErrorResponse
// @Failure 409 {object} handlers.ErrorResponse
// @Router /orders/{orderId}/cancel [post]
func (h *OrderHandler) CancelOrder(w http.ResponseWriter, r *http.Request) {
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

	// Check order ownership first
	existingOrder, err := h.orderService.GetOrder(r.Context(), tenantID, orderID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	if existingOrder.CustomerID != user.ID {
		handlers.RespondError(w, http.StatusForbidden, "access denied")
		return
	}

	var req CancelOrderRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	order, err := h.orderService.CancelOrder(r.Context(), tenantID, orderID, req.Reason, &user.ID, "customer", getClientIP(r))
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, order)
}

// RateOrder submits a 1-5 star rating for a delivered or completed order.
// @Summary Rate an order
// @Description Allows a customer to rate a delivered or completed order (1-5 stars)
// @Tags Orders
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param orderId path string true "Order ID"
// @Param payload body RateOrderRequest true "Rating payload"
// @Success 200 {object} ordering.Order
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Failure 409 {object} handlers.ErrorResponse
// @Router /orders/{orderId}/rate [post]
func (h *OrderHandler) RateOrder(w http.ResponseWriter, r *http.Request) {
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

	var req RateOrderRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	opts := ordering.RateOrderOpts{
		RiderRating:  req.RiderRating,
		RiderComment: req.RiderComment,
	}
	order, err := h.orderService.RateOrder(r.Context(), tenantID, orderID, user.ID, req.Rating, req.Comment, opts)
	if err != nil {
		switch err {
		case ordering.ErrInvalidRating:
			handlers.RespondError(w, http.StatusBadRequest, err.Error())
		case ordering.ErrOrderNotRatable:
			handlers.RespondError(w, http.StatusConflict, err.Error())
		case ordering.ErrAlreadyRated:
			handlers.RespondError(w, http.StatusConflict, err.Error())
		case ordering.ErrUnauthorized:
			handlers.RespondError(w, http.StatusForbidden, "access denied")
		default:
			h.handleError(w, err)
		}
		return
	}

	handlers.RespondJSON(w, http.StatusOK, order)
}

// GetOutletRating returns the materialized rating aggregate for an outlet.
// @Summary Get outlet rating
// @Description Returns average rating, star breakdown, and review count for an outlet
// @Tags Outlets
// @Produce json
// @Param outletId path string true "Outlet ID"
// @Success 200 {object} ordering.OutletRatingData
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Router /outlets/{outletId}/rating [get]
func (h *OrderHandler) GetOutletRating(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	outletID, err := uuid.Parse(chi.URLParam(r, "outletId"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid outlet ID")
		return
	}

	rating, err := h.orderService.GetOutletRating(r.Context(), tenantID, outletID)
	if err != nil {
		if errors.Is(err, ordering.ErrOutletRatingNotFound) {
			// Return a zero-value rating instead of 404 for outlets with no ratings yet
			handlers.RespondJSON(w, http.StatusOK, ordering.OutletRatingData{
				TenantID: tenantID,
				OutletID: outletID,
			})
			return
		}
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, rating)
}

// --- Admin Handlers ---

// AdminListOrders lists all orders with filters (admin).
// @Summary List all orders (admin)
// @Description Lists all orders with optional filters
// @Tags Admin Orders
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param outlet_id query string false "Filter by outlet ID"
// @Param customer_id query string false "Filter by customer ID"
// @Param status query string false "Filter by status"
// @Param payment_status query string false "Filter by payment status"
// @Param date_from query string false "Filter orders from date (RFC3339)"
// @Param date_to query string false "Filter orders to date (RFC3339)"
// @Param search query string false "Search by order number"
// @Param limit query integer false "Page size (default 50)"
// @Param page query integer false "Page number (default 1)"
// @Success 200 {object} ListOrdersResponse
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Failure 403 {object} handlers.ErrorResponse
// @Router /admin/orders [get]
func (h *OrderHandler) AdminListOrders(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	limit, offset, page := getPagination(r)

	filter := ordering.OrderFilter{
		TenantID: tenantID,
		Limit:    limit,
		Offset:   offset,
		Search:   r.URL.Query().Get("search"),
	}

	// Parse optional filters
	if outletIDStr := r.URL.Query().Get("outlet_id"); outletIDStr != "" {
		if outletID, err := uuid.Parse(outletIDStr); err == nil {
			filter.OutletID = &outletID
		}
	}

	if customerIDStr := r.URL.Query().Get("customer_id"); customerIDStr != "" {
		if customerID, err := uuid.Parse(customerIDStr); err == nil {
			filter.CustomerID = &customerID
		}
	}

	if statusStr := r.URL.Query().Get("status"); statusStr != "" {
		status, err := parseOrderStatus(statusStr)
		if err == nil {
			filter.Status = &status
		}
	}

	if paymentStatusStr := r.URL.Query().Get("payment_status"); paymentStatusStr != "" {
		switch paymentStatusStr {
		case "pending":
			ps := ordering.PaymentStatusPending
			filter.PaymentStatus = &ps
		case "authorized":
			ps := ordering.PaymentStatusAuthorized
			filter.PaymentStatus = &ps
		case "paid":
			ps := ordering.PaymentStatusPaid
			filter.PaymentStatus = &ps
		case "failed":
			ps := ordering.PaymentStatusFailed
			filter.PaymentStatus = &ps
		case "refunded":
			ps := ordering.PaymentStatusRefunded
			filter.PaymentStatus = &ps
		}
	}

	if dateFromStr := r.URL.Query().Get("date_from"); dateFromStr != "" {
		if dateFrom, err := time.Parse(time.RFC3339, dateFromStr); err == nil {
			filter.DateFrom = &dateFrom
		}
	}

	if dateToStr := r.URL.Query().Get("date_to"); dateToStr != "" {
		if dateTo, err := time.Parse(time.RFC3339, dateToStr); err == nil {
			filter.DateTo = &dateTo
		}
	}

	orders, total, err := h.orderService.ListOrders(r.Context(), filter)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, ListOrdersResponse{
		Data:  orders,
		Total: total,
		Limit: limit,
		Page:  page,
	})
}

// AdminGetOrder retrieves any order by ID (admin).
// @Summary Get order (admin)
// @Description Retrieves any order by its ID
// @Tags Admin Orders
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param orderId path string true "Order ID"
// @Success 200 {object} ordering.Order
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Failure 403 {object} handlers.ErrorResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Router /admin/orders/{orderId} [get]
func (h *OrderHandler) AdminGetOrder(w http.ResponseWriter, r *http.Request) {
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

	order, err := h.orderService.GetOrder(r.Context(), tenantID, orderID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, order)
}

// UpdateOrderStatus updates the status of an order.
// @Summary Update order status
// @Description Updates the status of an order (admin only)
// @Tags Admin Orders
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param orderId path string true "Order ID"
// @Param payload body UpdateStatusRequest true "New status"
// @Success 200 {object} ordering.Order
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Failure 403 {object} handlers.ErrorResponse
// @Failure 409 {object} handlers.ErrorResponse
// @Router /admin/orders/{orderId}/status [put]
func (h *OrderHandler) UpdateOrderStatus(w http.ResponseWriter, r *http.Request) {
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

	var req UpdateStatusRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	newStatus, err := parseOrderStatus(req.Status)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid status")
		return
	}

	order, err := h.orderService.UpdateOrderStatus(r.Context(), tenantID, orderID, newStatus, &user.ID, "staff", getClientIP(r))
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, order)
}

// AdminCancelOrder cancels an order (admin).
// @Summary Cancel order (admin)
// @Description Cancels an order with a reason
// @Tags Admin Orders
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param orderId path string true "Order ID"
// @Param payload body CancelOrderRequest true "Cancellation reason"
// @Success 200 {object} ordering.Order
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Failure 403 {object} handlers.ErrorResponse
// @Failure 409 {object} handlers.ErrorResponse
// @Router /admin/orders/{orderId}/cancel [post]
func (h *OrderHandler) AdminCancelOrder(w http.ResponseWriter, r *http.Request) {
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

	var req CancelOrderRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	order, err := h.orderService.CancelOrder(r.Context(), tenantID, orderID, req.Reason, &user.ID, "staff", getClientIP(r))
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, order)
}

// RefundOrderRequest represents a request to refund an order.
type RefundOrderRequestDTO struct {
	Reason string `json:"reason"`
}

// RefundOrder handles POST /admin/orders/{orderId}/refund
func (h *OrderHandler) RefundOrder(w http.ResponseWriter, r *http.Request) {
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

	var req RefundOrderRequestDTO
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Reason == "" {
		handlers.RespondError(w, http.StatusBadRequest, "reason is required")
		return
	}

	order, err := h.orderService.RefundOrder(r.Context(), ordering.RefundOrderRequest{
		TenantID: tenantID,
		OrderID:  orderID,
		Reason:   req.Reason,
		ActorID:  &user.ID,
	})
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, order)
}

// GetAnalyticsSummary returns aggregated metrics for the dashboard.
// @Summary Get analytics summary (admin)
// @Description Returns high-level metrics (revenue, orders, trends)
// @Tags Admin Orders
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param date_from query string false "Start date (RFC3339)"
// @Param date_to query string false "End date (RFC3339)"
// @Success 200 {object} ordering.AnalyticsSummary
// @Failure 401 {object} handlers.ErrorResponse
// @Failure 403 {object} handlers.ErrorResponse
// @Router /admin/orders/summary [get]
func (h *OrderHandler) GetAnalyticsSummary(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	dateFrom := time.Now().AddDate(0, 0, -30) // Default last 30 days
	if df := r.URL.Query().Get("date_from"); df != "" {
		if p, err := time.Parse(time.RFC3339, df); err == nil {
			dateFrom = p
		}
	}

	dateTo := time.Now()
	if dt := r.URL.Query().Get("date_to"); dt != "" {
		if p, err := time.Parse(time.RFC3339, dt); err == nil {
			dateTo = p
		}
	}

	summary, err := h.orderService.GetAnalyticsSummary(r.Context(), tenantID, dateFrom, dateTo)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, summary)
}

// GuestCheckout creates an order from a guest cart without requiring auth.
// @Summary Guest checkout
// @Description Creates an order from a guest cart identified by session ID (no auth required)
// @Tags Checkout
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param payload body GuestCheckoutRequestDTO true "Guest checkout data"
// @Success 201 {object} ordering.Order
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Router /checkout/guest [post]
func (h *OrderHandler) GuestCheckout(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	var req GuestCheckoutRequestDTO
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.SessionID == "" {
		handlers.RespondError(w, http.StatusBadRequest, "sessionId is required")
		return
	}

	if req.ContactEmail == "" && req.ContactPhone == "" {
		handlers.RespondError(w, http.StatusBadRequest, "contactEmail or contactPhone is required for guest checkout")
		return
	}

	if req.OutletID == "" {
		handlers.RespondError(w, http.StatusBadRequest, "outletId is required")
		return
	}

	outletID, err := uuid.Parse(req.OutletID)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid outletId")
		return
	}

	order, err := h.orderService.GuestCheckout(r.Context(), ordering.GuestCheckoutRequest{
		TenantID:        tenantID,
		OutletID:        outletID,
		SessionID:       req.SessionID,
		ContactEmail:    req.ContactEmail,
		ContactPhone:    req.ContactPhone,
		DeliveryAddress: req.DeliveryAddress,
		DeliveryLat:     req.DeliveryLat,
		DeliveryLng:     req.DeliveryLng,
		DeliveryNotes:   req.DeliveryNotes,
		PaymentMethod:   req.PaymentMethod,
		Instructions:    req.Instructions,
		Channel:         parseOrderChannel(req.Channel),
	})
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusCreated, order)
}

// Reorder creates a new cart populated with items from a past order.
func (h *OrderHandler) Reorder(w http.ResponseWriter, r *http.Request) {
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

	cart, err := h.orderService.ReorderFromPastOrder(r.Context(), tenantID, user.ID, orderID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusCreated, cart)
}

// BulkUpdateOrderStatus handles POST /admin/orders/bulk-status
func (h *OrderHandler) BulkUpdateOrderStatus(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	var req struct {
		OrderIDs []string `json:"orderIds"`
		Status   string   `json:"status"`
	}
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.OrderIDs) == 0 || req.Status == "" {
		handlers.RespondError(w, http.StatusBadRequest, "orderIds and status are required")
		return
	}

	if len(req.OrderIDs) > 50 {
		handlers.RespondError(w, http.StatusBadRequest, "maximum 50 orders per bulk operation")
		return
	}

	targetStatus := ordering.OrderStatus(req.Status)
	var results []map[string]interface{}
	var successCount, failCount int

	for _, idStr := range req.OrderIDs {
		orderID, parseErr := uuid.Parse(idStr)
		if parseErr != nil {
			results = append(results, map[string]interface{}{
				"order_id": idStr, "success": false, "error": "invalid order ID",
			})
			failCount++
			continue
		}

		_, updateErr := h.orderService.UpdateOrderStatus(
			r.Context(), tenantID, orderID,
			targetStatus,
			nil, "admin", "",
		)
		if updateErr != nil {
			results = append(results, map[string]interface{}{
				"order_id": idStr, "success": false, "error": updateErr.Error(),
			})
			failCount++
		} else {
			results = append(results, map[string]interface{}{
				"order_id": idStr, "success": true,
			})
			successCount++
		}
	}

	handlers.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"total":    len(req.OrderIDs),
		"success":  successCount,
		"failed":   failCount,
		"results":  results,
	})
}

// GetOrderEvents handles GET /admin/orders/{orderId}/events
func (h *OrderHandler) GetOrderEvents(w http.ResponseWriter, r *http.Request) {
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

	events, err := h.orderService.GetOrderEvents(r.Context(), tenantID, orderID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, events)
}
