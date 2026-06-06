package orderinghandler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	httpware "github.com/Bengo-Hub/httpware"
	"github.com/Bengo-Hub/pagination"
	authclient "github.com/Bengo-Hub/shared-auth-client"

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
	// Public order lookup (for guest post-checkout verification and tracking)
	r.Get("/orders/guest/{orderId}", h.GetGuestOrder)
	// Public order rating from the emailed "Leave Review" link (no auth; the order UUID is the capability).
	r.Post("/orders/guest/{orderId}/rate", h.RateOrderGuest)

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

	// Wallet payment — authenticated users can pay for their own orders via wallet
	r.Route("/orders/{orderId}", func(orderDetailRouter chi.Router) {
		orderDetailRouter.Use(auth.RequireAuth)
		orderDetailRouter.Post("/pay/wallet", h.PayWithWallet)
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
		adminRouter.Put("/{orderId}/rider", h.AdminAssignRider)
		adminRouter.Post("/{orderId}/cancel", h.AdminCancelOrder)
		adminRouter.Post("/{orderId}/refund", h.RefundOrder)
		adminRouter.Post("/{orderId}/payment/initiate", h.AdminInitiateOrderPayment)
		adminRouter.Delete("/{orderId}", h.DeleteOrder)
	})
}

// --- Request/Response Types ---

// CheckoutRequestDTO represents a checkout request.
// Supports two modes: cart-based (cartId) or items-based (outletId + items).
type CheckoutRequestDTO struct {
	CartID                string         `json:"cartId"`
	OutletID              string         `json:"outletId,omitempty"`
	Items                 []OrderItemDTO `json:"items,omitempty"`
	DeliveryAddressID     *string        `json:"deliveryAddressId,omitempty"`
	DeliveryAddress       string         `json:"deliveryAddress,omitempty"`
	DeliveryNotes         string         `json:"deliveryNotes,omitempty"`
	PromoCode             string         `json:"promoCode,omitempty"`
	LoyaltyPointsRedeemed int            `json:"loyaltyPointsRedeemed,omitempty"`
	Instructions          string         `json:"instructions,omitempty"`
	Channel               string         `json:"channel,omitempty"`
	IdempotencyKey        string         `json:"idempotencyKey,omitempty"`
	FulfillmentType       string         `json:"fulfillmentType,omitempty"`
	ScheduledAt           string         `json:"scheduledAt,omitempty"`
	PaymentMethod         string         `json:"paymentMethod,omitempty"` // "mpesa" | "cod"
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
	OutletID        string         `json:"outletId"`
	SessionID       string         `json:"sessionId"`
	ContactEmail    string         `json:"contactEmail"`
	ContactPhone    string         `json:"contactPhone"`
	ContactName     string         `json:"contactName"`
	Items           []OrderItemDTO `json:"items"`
	FulfillmentType string         `json:"fulfillmentType,omitempty"`
	DeliveryAddress string         `json:"deliveryAddress"`
	DeliveryLat     *float64       `json:"deliveryLat,omitempty"`
	DeliveryLng     *float64       `json:"deliveryLng,omitempty"`
	DeliveryNotes   string         `json:"deliveryNotes,omitempty"`
	PaymentMethod   string         `json:"paymentMethod"` // "mpesa" | "cod"
	Instructions    string         `json:"instructions,omitempty"`
	Channel         string         `json:"channel,omitempty"`
	IdempotencyKey  string         `json:"idempotencyKey,omitempty"`
	ScheduledAt     string         `json:"scheduledAt,omitempty"`
}

// CreateOrderRequestDTO is the request body for POST /orders (create order from items, frontend contract).
type CreateOrderRequestDTO struct {
	OutletID        string         `json:"outletId"`
	Items           []OrderItemDTO `json:"items"`
	DeliveryAddress string         `json:"deliveryAddress"`
	DeliveryLat     *float64       `json:"deliveryLat,omitempty"`
	DeliveryLng     *float64       `json:"deliveryLng,omitempty"`
	DeliveryNotes   string         `json:"deliveryNotes,omitempty"`
	PaymentMethod   string         `json:"paymentMethod"` // "mpesa" | "cod"
	PromoCode       string         `json:"promoCode,omitempty"`
	FulfillmentType string         `json:"fulfillmentType,omitempty"`
	ScheduledAt     string         `json:"scheduledAt,omitempty"`
}

// OrderItemDTO is a single item in CreateOrderRequestDTO.
type OrderItemDTO struct {
	InventorySKU string                 `json:"inventorySku"`
	Name         string                 `json:"name"`
	Quantity     int                    `json:"quantity"`
	UnitPrice    float64                `json:"unitPrice"`
	TotalPrice   float64                `json:"totalPrice"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// ListOrdersResponse represents the paginated order list response.
type ListOrdersResponse struct {
	Data    []ordering.Order `json:"data"`
	Total   int              `json:"total"`
	Limit   int              `json:"limit"`
	Page    int              `json:"page"`
	HasMore bool             `json:"hasMore"`
}

// AdminOrderItemSummary is the lightweight item included in AdminOrderSummary.
type AdminOrderItemSummary struct {
	ID           string  `json:"id"`
	NameSnapshot string  `json:"nameSnapshot"`
	Quantity     int     `json:"quantity"`
	UnitPrice    float64 `json:"unitPrice"`
	TotalPrice   float64 `json:"totalPrice"`
}

// AdminOrderSummary is a lightweight order representation for admin list views.
// It omits internal IDs, per-fee breakdowns, events, and rating fields that are
// not displayed on the order management dashboard, keeping the payload lean.
type AdminOrderSummary struct {
	ID              string                   `json:"id"`
	OrderNumber     string                   `json:"orderNumber"`
	Status          ordering.OrderStatus     `json:"status"`
	PaymentStatus   ordering.PaymentStatus   `json:"paymentStatus"`
	PaymentMethod   ordering.PaymentMethod   `json:"paymentMethod"`
	FulfillmentType ordering.FulfillmentType `json:"fulfillmentType"`
	Currency        string                   `json:"currency"`
	Subtotal        float64                  `json:"subtotal"`
	DiscountTotal   float64                  `json:"discountTotal"`
	DeliveryFee     float64                  `json:"deliveryFee"`
	GrandTotal      float64                  `json:"grandTotal"`
	CustomerName    string                   `json:"customerName,omitempty"`
	CustomerEmail   string                   `json:"customerEmail,omitempty"`
	CustomerPhone   string                   `json:"customerPhone,omitempty"`
	Channel         string                   `json:"channel,omitempty"`
	Source          string                   `json:"source,omitempty"`
	Instructions    string                   `json:"instructions,omitempty"`
	DeliveryAddress string                   `json:"deliveryAddress,omitempty"`
	// Metadata kept so the frontend can resolve guest contact info when needed.
	Metadata  map[string]interface{}  `json:"metadata,omitempty"`
	Items     []AdminOrderItemSummary `json:"items"`
	PlacedAt  *time.Time              `json:"placedAt,omitempty"`
	CreatedAt time.Time               `json:"createdAt"`
}

// AdminListOrdersResponse is the paginated response for admin order list views.
type AdminListOrdersResponse struct {
	Data    []AdminOrderSummary `json:"data"`
	Total   int                 `json:"total"`
	Limit   int                 `json:"limit"`
	Page    int                 `json:"page"`
	HasMore bool                `json:"hasMore"`
}

// toAdminOrderSummary converts a full Order to the lightweight AdminOrderSummary.
func toAdminOrderSummary(o ordering.Order) AdminOrderSummary {
	items := make([]AdminOrderItemSummary, 0, len(o.Items))
	for _, it := range o.Items {
		items = append(items, AdminOrderItemSummary{
			ID:           it.ID.String(),
			NameSnapshot: it.NameSnapshot,
			Quantity:     it.Quantity,
			UnitPrice:    it.UnitPrice,
			TotalPrice:   it.TotalPrice,
		})
	}

	deliveryAddr := ""
	if o.DeliveryAddress != nil {
		deliveryAddr = o.DeliveryAddress.AddressLine1
		if o.DeliveryAddress.AddressLine2 != "" {
			deliveryAddr += ", " + o.DeliveryAddress.AddressLine2
		}
	}

	return AdminOrderSummary{
		ID:              o.ID.String(),
		OrderNumber:     o.OrderNumber,
		Status:          o.Status,
		PaymentStatus:   o.PaymentStatus,
		PaymentMethod:   o.PaymentMethod,
		FulfillmentType: o.FulfillmentType,
		Currency:        o.Currency,
		Subtotal:        o.Subtotal,
		DiscountTotal:   o.DiscountTotal,
		DeliveryFee:     o.DeliveryFee,
		GrandTotal:      o.GrandTotal,
		CustomerName:    o.CustomerName,
		CustomerEmail:   o.CustomerEmail,
		CustomerPhone:   o.CustomerPhone,
		Channel:         string(o.Channel),
		Source:          o.Source,
		Instructions:    o.Instructions,
		DeliveryAddress: deliveryAddr,
		Metadata:        o.Metadata,
		Items:           items,
		PlacedAt:        o.PlacedAt,
		CreatedAt:       o.CreatedAt,
	}
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

	case errors.Is(err, ordering.ErrStockNotAvailable):
		handlers.RespondError(w, http.StatusConflict, err.Error())

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

	case errors.Is(err, ordering.ErrDeliveryNotServiceable):
		handlers.RespondError(w, http.StatusUnprocessableEntity, "We don't deliver to this location yet.")

	default:
		h.log.Error("internal error", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "internal server error")
	}
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

	// Items-based checkout: frontend sends items directly from local cart state
	if len(req.Items) > 0 && req.OutletID != "" {
		outletID, err := uuid.Parse(req.OutletID)
		if err != nil {
			handlers.RespondError(w, http.StatusBadRequest, "invalid outletId")
			return
		}

		items := make([]ordering.CreateOrderItemInput, 0, len(req.Items))
		for _, it := range req.Items {
			items = append(items, ordering.CreateOrderItemInput{
				InventorySKU: it.InventorySKU,
				Name:         it.Name,
				Quantity:     it.Quantity,
				UnitPrice:    it.UnitPrice,
				TotalPrice:   it.TotalPrice,
				Metadata:     it.Metadata,
			})
		}

		// Resolve delivery address: prefer explicit address string, fall back to
		// looking up the addressId via the repo if only an ID was provided.
		deliveryAddress := req.DeliveryAddress
		var deliveryLat, deliveryLng *float64
		if deliveryAddress == "" && req.DeliveryAddressID != nil && *req.DeliveryAddressID != "" {
			addrID, parseErr := uuid.Parse(*req.DeliveryAddressID)
			if parseErr == nil {
				addr, addrErr := h.orderService.GetCustomerAddress(r.Context(), tenantID, addrID)
				if addrErr == nil && addr != nil {
					deliveryAddress = addr.AddressLine1
					if addr.AddressLine2 != "" {
						deliveryAddress += ", " + addr.AddressLine2
					}
					deliveryLat = addr.Latitude
					deliveryLng = addr.Longitude
				}
			}
		}

		channel := ordering.OrderChannelWeb
		if req.Channel != "" {
			channel = ordering.OrderChannel(req.Channel)
		}

		order, err := h.orderService.CreateOrderFromItems(r.Context(), ordering.CreateOrderFromItemsRequest{
			TenantID:        tenantID,
			OutletID:        outletID,
			UserID:          user.ID,
			Items:           items,
			DeliveryAddress: deliveryAddress,
			DeliveryLat:     deliveryLat,
			DeliveryLng:     deliveryLng,
			DeliveryNotes:   req.DeliveryNotes,
			PromoCode:       req.PromoCode,
			Channel:         channel,
			FulfillmentType: fulfillmentType,
			ScheduledFor:    scheduledFor,
			PaymentMethod:   req.PaymentMethod,
		})
		if err != nil {
			h.handleError(w, err)
			return
		}
		// Build checkout result with payment intent. Pass the authenticated user's email AND phone so
		// the CRM upsert links the order to the customer (a logged-in buyer's phone was previously
		// dropped here, skipping the upsert entirely).
		checkoutResult := h.orderService.BuildCheckoutResult(r.Context(), order, user.Email, user.Phone)
		handlers.RespondJSON(w, http.StatusCreated, checkoutResult)
		return
	}

	// Cart-based checkout (legacy): requires cartId
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
		PaymentMethod:         ordering.PaymentMethod(req.PaymentMethod),
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
			Metadata:     it.Metadata,
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
	if (order.CustomerID == nil || *order.CustomerID != user.ID) && !user.HasPermission(identity.PermissionOrdersManage) {
		handlers.RespondError(w, http.StatusForbidden, "access denied")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, order)
}

// GetGuestOrder retrieves an order by ID for guest users (no auth, verified by session_id).
// @Summary Get guest order
// @Description Retrieves a guest order. Requires session_id query param matching the order's session.
// @Tags Orders
// @Produce json
// @Param orderId path string true "Order ID"
// @Param session_id query string true "Guest session ID"
// @Success 200 {object} ordering.Order
// @Failure 403 {object} handlers.ErrorResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Router /orders/guest/{orderId} [get]
func (h *OrderHandler) GetGuestOrder(w http.ResponseWriter, r *http.Request) {
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

	sessionID := r.URL.Query().Get("session_id")

	order, err := h.orderService.GetOrder(r.Context(), tenantID, orderID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	// Verify the session only when a session_id was supplied. When it is absent, the
	// unguessable order UUID is the capability — this lets emailed order links open for
	// both guest and authenticated-user orders without login.
	if sessionID != "" {
		if order.Metadata == nil || order.Metadata["sessionId"] != sessionID {
			handlers.RespondError(w, http.StatusForbidden, "access denied")
			return
		}
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

	p := pagination.Parse(r)

	filter := ordering.OrderFilter{
		TenantID:   tenantID,
		CustomerID: &user.ID,
		Limit:      p.Limit,
		Offset:     p.Offset,
	}

	// Apply outlet context filter if X-Outlet-ID was sent
	if outletIDStr := httpware.GetOutletID(r.Context()); outletIDStr != "" {
		if outletUID, err := uuid.Parse(outletIDStr); err == nil {
			filter.OutletID = &outletUID
		}
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
		Data:    orders,
		Total:   total,
		Limit:   p.Limit,
		Page:    p.Page,
		HasMore: p.Offset+len(orders) < total,
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

	if existingOrder.CustomerID == nil || *existingOrder.CustomerID != user.ID {
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

// RateOrderGuest rates an order without authentication, used by the emailed
// "Leave Review" link. The unguessable order UUID is the access capability.
func (h *OrderHandler) RateOrderGuest(w http.ResponseWriter, r *http.Request) {
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
	var req RateOrderRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	opts := ordering.RateOrderOpts{RiderRating: req.RiderRating, RiderComment: req.RiderComment}
	order, err := h.orderService.RateOrderGuest(r.Context(), tenantID, orderID, req.Rating, req.Comment, opts)
	if err != nil {
		switch err {
		case ordering.ErrInvalidRating:
			handlers.RespondError(w, http.StatusBadRequest, err.Error())
		case ordering.ErrOrderNotRatable:
			handlers.RespondError(w, http.StatusConflict, err.Error())
		case ordering.ErrAlreadyRated:
			handlers.RespondError(w, http.StatusConflict, err.Error())
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

	p := pagination.Parse(r)

	filter := ordering.OrderFilter{
		TenantID: tenantID,
		Limit:    p.Limit,
		Offset:   p.Offset,
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

	summaries := make([]AdminOrderSummary, 0, len(orders))
	for _, o := range orders {
		summaries = append(summaries, toAdminOrderSummary(o))
	}

	handlers.RespondJSON(w, http.StatusOK, AdminListOrdersResponse{
		Data:    summaries,
		Total:   total,
		Limit:   p.Limit,
		Page:    p.Page,
		HasMore: p.Offset+len(summaries) < total,
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

// InitiateOrderPaymentRequestDTO is the (optional) body for staff/rider-triggered payment initiation.
// Both fields are optional: paymentMethod defaults to "paystack" (the primary gateway, which supports
// all M-Pesa rails — STK push, paybill, till — plus card); phoneNumber defaults to the order's customer phone.
type InitiateOrderPaymentRequestDTO struct {
	PaymentMethod string `json:"paymentMethod,omitempty"`
	PhoneNumber   string `json:"phoneNumber,omitempty"`
}

// InitiateOrderPaymentResponseDTO is returned after a staff/rider triggers payment on an order.
type InitiateOrderPaymentResponseDTO struct {
	Status            string `json:"status,omitempty"`
	CustomerMessage   string `json:"customerMessage,omitempty"`
	CheckoutRequestID string `json:"checkoutRequestId,omitempty"`
	AuthorizationURL  string `json:"authorizationUrl,omitempty"`
	RedirectURL       string `json:"redirectUrl,omitempty"`
}

// AdminInitiateOrderPayment triggers payment (e.g. an M-Pesa STK push) on an existing order's
// payment intent, so a rider/staff member can prompt the customer to pay at delivery. Reuses
// treasury's per-intent initiate primitive via the S2S client (the public route is never exposed
// to staff clients).
// @Summary Initiate payment for an order (admin/rider)
// @Description Triggers payment (e.g. M-Pesa STK push) on the order's existing payment intent. Body is optional: paymentMethod defaults to "paystack", phoneNumber defaults to the order's customer phone.
// @Tags Admin Orders
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param orderId path string true "Order ID"
// @Param payload body InitiateOrderPaymentRequestDTO false "Optional payment-method / phone override"
// @Success 200 {object} InitiateOrderPaymentResponseDTO
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Failure 403 {object} handlers.ErrorResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Failure 409 {object} handlers.ErrorResponse
// @Failure 502 {object} handlers.ErrorResponse
// @Failure 503 {object} handlers.ErrorResponse
// @Router /admin/orders/{orderId}/payment/initiate [post]
func (h *OrderHandler) AdminInitiateOrderPayment(w http.ResponseWriter, r *http.Request) {
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

	// Body is optional — a bare POST defaults to paystack + the order's customer phone.
	var req InitiateOrderPaymentRequestDTO
	if r.Body != nil {
		_ = decodeJSON(r, &req)
	}

	resp, err := h.orderService.InitiateOrderPayment(r.Context(), ordering.InitiateOrderPaymentInput{
		TenantID:      tenantID,
		OrderID:       orderID,
		PaymentMethod: req.PaymentMethod,
		PhoneNumber:   req.PhoneNumber,
	})
	if err != nil {
		switch {
		case errors.Is(err, ordering.ErrOrderNotFound):
			handlers.RespondError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, ordering.ErrNoPaymentIntent):
			handlers.RespondError(w, http.StatusConflict, err.Error())
		case errors.Is(err, ordering.ErrTreasuryNotConfigured):
			handlers.RespondError(w, http.StatusServiceUnavailable, err.Error())
		case errors.Is(err, ordering.ErrPaymentInitiateFailed):
			handlers.RespondError(w, http.StatusBadGateway, err.Error())
		default:
			h.handleError(w, err)
		}
		return
	}

	handlers.RespondJSON(w, http.StatusOK, InitiateOrderPaymentResponseDTO{
		Status:            resp.Status,
		CustomerMessage:   resp.CustomerMessage,
		CheckoutRequestID: resp.CheckoutRequestID,
		AuthorizationURL:  resp.AuthorizationURL,
		RedirectURL:       resp.RedirectURL,
	})
}

// DeleteOrder permanently deletes an order and its related data (items, events).
// @Summary Delete order (admin)
// @Description Permanently deletes an order. Only cancelled/refunded/completed orders can be deleted.
// @Tags Admin Orders
// @Param orderId path string true "Order ID"
// @Success 204 "No Content"
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 409 {object} handlers.ErrorResponse
// @Router /admin/orders/{orderId} [delete]
func (h *OrderHandler) DeleteOrder(w http.ResponseWriter, r *http.Request) {
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

	if err := h.orderService.DeleteOrder(r.Context(), tenantID, orderID); err != nil {
		h.handleError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
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

	// Convert DTO items to domain items
	var items []ordering.CreateOrderItemInput
	for _, it := range req.Items {
		items = append(items, ordering.CreateOrderItemInput{
			InventorySKU: it.InventorySKU,
			Name:         it.Name,
			Quantity:     it.Quantity,
			UnitPrice:    it.UnitPrice,
			TotalPrice:   it.TotalPrice,
			Metadata:     it.Metadata,
		})
	}

	order, err := h.orderService.GuestCheckout(r.Context(), ordering.GuestCheckoutRequest{
		TenantID:        tenantID,
		OutletID:        outletID,
		SessionID:       req.SessionID,
		ContactEmail:    req.ContactEmail,
		ContactPhone:    req.ContactPhone,
		ContactName:     req.ContactName,
		Items:           items,
		DeliveryAddress: req.DeliveryAddress,
		DeliveryLat:     req.DeliveryLat,
		DeliveryLng:     req.DeliveryLng,
		DeliveryNotes:   req.DeliveryNotes,
		PaymentMethod:   req.PaymentMethod,
		Instructions:    req.Instructions,
		Channel:         parseOrderChannel(req.Channel),
		FulfillmentType: ordering.FulfillmentType(req.FulfillmentType),
	})
	if err != nil {
		h.handleError(w, err)
		return
	}

	checkoutResult := h.orderService.BuildCheckoutResult(r.Context(), order, req.ContactEmail, req.ContactPhone)
	handlers.RespondJSON(w, http.StatusCreated, checkoutResult)
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
		"total":   len(req.OrderIDs),
		"success": successCount,
		"failed":  failCount,
		"results": results,
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

// PayWithWallet debits the authenticated user's treasury wallet and marks the order as paid.
// POST /{slug}/orders/{orderId}/pay/wallet
func (h *OrderHandler) PayWithWallet(w http.ResponseWriter, r *http.Request) {
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

	user, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	order, err := h.orderService.PayWithWallet(r.Context(), tenantID, orderID, user.ID)
	if err != nil {
		h.log.Error("wallet payment failed", zap.Error(err))
		handlers.RespondError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	handlers.RespondJSON(w, http.StatusOK, map[string]any{
		"success":        true,
		"order_id":       order.ID,
		"payment_status": order.PaymentStatus,
		"order_status":   order.Status,
	})
}

// AssignRiderRequest is the request body for PUT /admin/orders/{orderId}/rider.
type AssignRiderRequest struct {
	RiderID string `json:"rider_id"`
}

// AdminAssignRider assigns a fleet member (rider) to a delivery order.
// If no delivery task exists yet, one is auto-created from the order data before assignment.
// On success the order status is automatically set to out_for_delivery.
// @Summary Assign a rider to a delivery order
// @Description Assigns a fleet member (rider) to a delivery order, auto-creating the logistics task if needed. Only valid for delivery-fulfilment orders; the order is then moved to out_for_delivery.
// @Tags Admin Orders
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param orderId path string true "Order ID"
// @Param payload body AssignRiderRequest true "Rider to assign"
// @Success 200 {object} fulfilment.OrderAssignment
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Failure 422 {object} handlers.ErrorResponse
// @Failure 503 {object} handlers.ErrorResponse
// @Router /admin/orders/{orderId}/rider [put]
func (h *OrderHandler) AdminAssignRider(w http.ResponseWriter, r *http.Request) {
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

	var req AssignRiderRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.RiderID == "" {
		handlers.RespondError(w, http.StatusBadRequest, "rider_id is required")
		return
	}

	if h.taskService == nil {
		handlers.RespondError(w, http.StatusServiceUnavailable, "logistics service unavailable")
		return
	}

	// A rider can only be assigned to a delivery order — reject pickup/dine-in/scheduled up front
	// (previously this was only checked in the auto-create-task branch, so a stale non-delivery task
	// could still be assigned a rider).
	order, oErr := h.orderService.GetOrder(r.Context(), tenantID, orderID)
	if oErr != nil {
		h.log.Error("order not found for rider assignment", zap.Error(oErr), zap.String("order_id", orderID.String()))
		handlers.RespondError(w, http.StatusNotFound, "order not found")
		return
	}
	if order.FulfillmentType != ordering.FulfillmentTypeDelivery {
		handlers.RespondError(w, http.StatusUnprocessableEntity, "order is not a delivery order")
		return
	}

	tenantSlug := ""
	if claims, ok := authclient.ClaimsFromContext(r.Context()); ok {
		tenantSlug = claims.GetTenantSlug()
	}

	// Attempt assignment; auto-create delivery task if none exists yet.
	assignment, err := h.taskService.AssignRider(r.Context(), tenantSlug, tenantID, orderID, req.RiderID)
	if err != nil {
		if !errors.Is(err, fulfilment.ErrAssignmentNotFound) {
			h.log.Error("failed to assign rider", zap.Error(err), zap.String("order_id", orderID.String()))
			handlers.RespondError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}

		// No task exists yet — auto-create the delivery task from the order fetched above.
		orderInfo := buildOrderInfoFromOrder(order, tenantSlug)
		createReq := fulfilment.CreateDeliveryTaskRequest{
			OrderInfo:      orderInfo,
			Priority:       fulfilment.PriorityNormal,
			IdempotencyKey: order.ID.String() + ":create",
		}

		if _, cErr := h.taskService.CreateDeliveryTask(r.Context(), createReq); cErr != nil {
			h.log.Error("failed to auto-create delivery task", zap.Error(cErr), zap.String("order_id", orderID.String()))
			handlers.RespondError(w, http.StatusUnprocessableEntity, "failed to create delivery task: "+cErr.Error())
			return
		}

		// Retry assignment after task creation.
		assignment, err = h.taskService.AssignRider(r.Context(), tenantSlug, tenantID, orderID, req.RiderID)
		if err != nil {
			h.log.Error("failed to assign rider after task creation", zap.Error(err), zap.String("order_id", orderID.String()))
			handlers.RespondError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
	}

	// Auto-transition order status to out_for_delivery.
	if _, sErr := h.orderService.UpdateOrderStatus(r.Context(), tenantID, orderID,
		ordering.OrderStatusOutForDelivery, nil, "system", r.RemoteAddr); sErr != nil {
		h.log.Warn("failed to mark order out_for_delivery after rider assign",
			zap.Error(sErr), zap.String("order_id", orderID.String()))
	}

	handlers.RespondJSON(w, http.StatusOK, assignment)
}

// buildOrderInfoFromOrder converts an Order to the OrderInfo needed for task creation.
func buildOrderInfoFromOrder(order *ordering.Order, tenantSlug string) fulfilment.OrderInfo {
	info := fulfilment.OrderInfo{
		ID:            order.ID,
		TenantID:      order.TenantID,
		TenantSlug:    tenantSlug,
		OrderNumber:   order.OrderNumber,
		CustomerName:  order.CustomerName,
		CustomerPhone: order.CustomerPhone,
		Instructions:  order.Instructions,
		ItemCount:     len(order.Items),
		PODCode:       order.PODCode,
		DeliveryFee:   order.DeliveryFee,
	}

	if order.DeliveryAddress != nil {
		addr := order.DeliveryAddress
		addrStr := addr.AddressLine1
		if addr.AddressLine2 != "" {
			addrStr += ", " + addr.AddressLine2
		}
		if addr.City != "" {
			addrStr += ", " + addr.City
		}
		info.DeliveryAddress = addrStr
		if addr.Latitude != nil {
			info.DeliveryLat = *addr.Latitude
		}
		if addr.Longitude != nil {
			info.DeliveryLng = *addr.Longitude
		}
		info.DeliveryNotes = addr.Instructions
		if addr.ContactName != "" {
			info.CustomerName = addr.ContactName
		}
		if addr.ContactPhone != "" {
			info.CustomerPhone = addr.ContactPhone
		}
	}

	// Build items description
	if len(order.Items) > 0 {
		names := make([]string, 0, len(order.Items))
		for _, item := range order.Items {
			names = append(names, item.NameSnapshot)
		}
		info.ItemsDescription = strings.Join(names, ", ")
	}

	return info
}
