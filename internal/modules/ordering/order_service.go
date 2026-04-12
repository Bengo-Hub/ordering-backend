package ordering

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/Bengo-Hub/httpware"
	authclient "github.com/Bengo-Hub/shared-auth-client"
	"github.com/bengobox/ordering-backend/internal/platform/events"
	"github.com/bengobox/ordering-backend/internal/platform/inventory"
	"github.com/bengobox/ordering-backend/internal/platform/logistics"
	"github.com/bengobox/ordering-backend/internal/platform/subscriptions"
	"github.com/bengobox/ordering-backend/internal/platform/treasury"
)

// customerIDValue returns the uuid.UUID value from a *uuid.UUID pointer,
// falling back to uuid.Nil for guest orders.
func customerIDValue(c *uuid.UUID) uuid.UUID {
	if c != nil {
		return *c
	}
	return uuid.Nil
}

// OrderService provides order business logic.
type OrderService struct {
	repo                Repository
	cartSvc             *CartService
	promoSvc            *PromoService
	loyaltySvc          *LoyaltyService
	feeSvc              *FeeService
	stateMachine        *OrderStateMachine
	eventPublisher      *events.Publisher
	inventoryClient     *inventory.Client
	treasuryClient      *treasury.Client
	subscriptionsClient *subscriptions.Client
	logisticsClient     *logistics.Client
	logger              *zap.Logger
}

// NewOrderService creates a new order service.
func NewOrderService(
	repo Repository,
	cartSvc *CartService,
	promoSvc *PromoService,
	loyaltySvc *LoyaltyService,
	feeSvc *FeeService,
	inventoryClient *inventory.Client,
	subscriptionsClient *subscriptions.Client,
	logger *zap.Logger,
) *OrderService {
	return &OrderService{
		repo:                repo,
		cartSvc:             cartSvc,
		promoSvc:            promoSvc,
		loyaltySvc:          loyaltySvc,
		feeSvc:              feeSvc,
		inventoryClient:     inventoryClient,
		subscriptionsClient: subscriptionsClient,
		stateMachine:        NewOrderStateMachine(),
		logger:              logger,
	}
}

// SetTreasuryClient sets the treasury client for refund processing.
func (s *OrderService) SetTreasuryClient(client *treasury.Client) {
	s.treasuryClient = client
}

// SetEventPublisher sets the event publisher for the order service.
// This is called after initialization to avoid circular dependencies.
func (s *OrderService) SetEventPublisher(publisher *events.Publisher) {
	s.eventPublisher = publisher
}

// SetLogisticsClient sets the logistics client for rider rating integration.
func (s *OrderService) SetLogisticsClient(client *logistics.Client) {
	s.logisticsClient = client
}

// checkSubscription enforces that the tenant has an active subscription before
// allowing order creation. Platform owners bypass this check entirely.
func (s *OrderService) checkSubscription(ctx context.Context, tenantID uuid.UUID) error {
	if httpware.IsPlatformOwner(ctx) {
		return nil
	}
	if s.subscriptionsClient == nil {
		return nil // subscriptions enforcement disabled
	}
	// Extract bearer token from auth claims for the inter-service call
	bearerToken := ""
	if claims, ok := authclient.ClaimsFromContext(ctx); ok {
		_ = claims // token is passed via X-API-Key header from client config
	}
	if !s.subscriptionsClient.IsSubscriptionActive(ctx, tenantID, bearerToken) {
		return ErrSubscriptionRequired
	}
	return nil
}

// Checkout creates an order from a cart.
func (s *OrderService) Checkout(ctx context.Context, req CheckoutRequest) (*Order, error) {
	// Enforce active subscription
	if err := s.checkSubscription(ctx, req.TenantID); err != nil {
		return nil, err
	}

	// Check for idempotency
	if req.IdempotencyKey != "" {
		existingOrder, err := s.repo.GetOrderByIdempotencyKey(ctx, req.TenantID, req.IdempotencyKey)
		if err == nil && existingOrder != nil {
			return existingOrder, nil
		}
	}

	// Get cart
	cart, err := s.cartSvc.GetCart(ctx, req.TenantID, req.CartID)
	if err != nil {
		return nil, err
	}

	// Validate cart
	if cart.Status != CartStatusActive {
		return nil, ErrCartAlreadyCheckedOut
	}

	if len(cart.Items) == 0 {
		return nil, ErrCartEmpty
	}

	// Determine fulfillment type (default to delivery)
	fulfillmentType := req.FulfillmentType
	if fulfillmentType == "" {
		fulfillmentType = FulfillmentTypeDelivery
	}

	// Validate scheduled orders
	if fulfillmentType == FulfillmentTypeScheduled {
		if req.ScheduledFor == nil {
			return nil, ErrScheduledForRequired
		}
		if req.ScheduledFor.Before(time.Now().Add(ScheduledMinLeadTime)) {
			return nil, ErrScheduledForTooSoon
		}
	}

	// Validate delivery address if provided (not required for pickup orders)
	var deliveryAddress *CustomerAddress
	if fulfillmentType == FulfillmentTypePickup {
		// Pickup orders don't need a delivery address
	} else if req.DeliveryAddressID != nil {
		deliveryAddress, err = s.repo.GetAddress(ctx, req.TenantID, *req.DeliveryAddressID)
		if err != nil {
			return nil, ErrInvalidDeliveryAddress
		}
		if deliveryAddress.UserID != req.UserID {
			return nil, ErrInvalidDeliveryAddress
		}
	}

	// Apply promo code if provided
	var promoCodeID *uuid.UUID
	var discountTotal float64 = cart.DiscountTotal
	if req.PromoCode != "" {
		result, err := s.promoSvc.ValidatePromoCode(ctx, req.TenantID, cart.OutletID, req.PromoCode, cart.Subtotal, &req.UserID)
		if err != nil || !result.Valid {
			return nil, ErrPromoCodeNotFound
		}
		promoCodeID = result.PromoCodeID
		discountTotal = result.DiscountAmount
	} else if cart.PromoCodeID != nil {
		promoCodeID = cart.PromoCodeID
	}

	// Validate loyalty points
	loyaltyPointsRedeemed := req.LoyaltyPointsRedeemed
	loyaltyDiscount := float64(loyaltyPointsRedeemed) * LoyaltyPointValue
	if loyaltyPointsRedeemed > 0 {
		account, err := s.loyaltySvc.GetAccountByUser(ctx, req.TenantID, req.UserID)
		if err != nil || account.BalancePoints < loyaltyPointsRedeemed {
			return nil, ErrInsufficientLoyaltyPoints
		}
	}

	// Calculate full fee breakdown (service fee, packaging fee, small order fee, etc.)
	fees, feeErr := s.feeSvc.CalculateFees(ctx, req.TenantID, cart, fulfillmentType, discountTotal, loyaltyDiscount)
	if feeErr != nil {
		s.logger.Error("failed to calculate fees", zap.Error(feeErr))
		return nil, feeErr
	}
	grandTotal := fees.GrandTotal
	deliveryFee := fees.DeliveryFee

	// Generate order number
	orderNumber, err := s.repo.GenerateOrderNumber(ctx, req.TenantID, cart.OutletID)
	if err != nil {
		s.logger.Error("failed to generate order number", zap.Error(err))
		return nil, err
	}

	// Reserve stock via inventory service (fail fast if stock unavailable)
	var reservationID *uuid.UUID
	if s.inventoryClient != nil {
		reservationID, err = s.reserveStockForItems(ctx, req.TenantID, uuid.Nil, cart.Items, req.IdempotencyKey)
		if err != nil {
			return nil, err
		}
	}

	// Calculate loyalty points earned
	loyaltyPointsEarned := s.loyaltySvc.CalculatePointsForAmount(grandTotal)

	// Determine payment method and status
	paymentMethod := req.PaymentMethod
	if paymentMethod == "" {
		paymentMethod = PaymentMethodMpesa // default
	}
	paymentStatus := PaymentStatusPending
	if paymentMethod == PaymentMethodCOD {
		paymentStatus = "cod_pending"
	}

	// Create order
	now := time.Now()
	order := &Order{
		TenantID:              req.TenantID,
		OutletID:              cart.OutletID,
		CustomerID:            &req.UserID,
		CartID:                &cart.ID,
		OrderNumber:           orderNumber,
		Status:                OrderStatusPending,
		PaymentStatus:         PaymentStatus(paymentStatus),
		PaymentMethod:         paymentMethod,
		FulfillmentType:       fulfillmentType,
		ScheduledFor:          req.ScheduledFor,
		Currency:              cart.Currency,
		Subtotal:              cart.Subtotal,
		DiscountTotal:         discountTotal,
		TaxTotal:              cart.TaxTotal,
		DeliveryFee:           deliveryFee,
		PackagingFee:          fees.PackagingFee,
		ServiceFee:            fees.ServiceFee,
		SmallOrderFee:         fees.SmallOrderFee,
		GrandTotal:            grandTotal,
		LoyaltyPointsEarned:   loyaltyPointsEarned,
		LoyaltyPointsRedeemed: loyaltyPointsRedeemed,
		DeliveryAddressID:     req.DeliveryAddressID,
		PromoCodeID:           promoCodeID,
		Instructions:          req.Instructions,
		Channel:               req.Channel,
		IdempotencyKey:        req.IdempotencyKey,
		PlacedAt:              &now,
		DeliveryAddress:       deliveryAddress,
		ReservationID:         reservationID,
	}

	if err := s.repo.CreateOrder(ctx, order); err != nil {
		s.logger.Error("failed to create order", zap.Error(err))
		return nil, err
	}

	// Create order items from cart items
	for _, cartItem := range cart.Items {
		orderItem := &OrderItem{
			OrderID:       order.ID,
			InventorySKU: cartItem.InventorySKU,
			VariantID:     cartItem.VariantID,
			NameSnapshot:  cartItem.NameSnapshot,
			Quantity:     cartItem.Quantity,
			UnitPrice:    cartItem.UnitPrice,
			TotalPrice:   cartItem.TotalPrice,
			Notes:        cartItem.Notes,
			Metadata:     cartItem.Metadata,
		}
		if err := s.repo.CreateOrderItem(ctx, orderItem); err != nil {
			s.logger.Error("failed to create order item", zap.Error(err))
		}
	}

	// Record promo redemption
	if promoCodeID != nil {
		if err := s.promoSvc.RecordPromoRedemption(ctx, *promoCodeID, order.ID, req.UserID, discountTotal); err != nil {
			s.logger.Error("failed to record promo redemption", zap.Error(err))
		}
	}

	// Deduct loyalty points if redeemed
	if loyaltyPointsRedeemed > 0 {
		if err := s.loyaltySvc.RedeemPoints(ctx, req.TenantID, req.UserID, loyaltyPointsRedeemed, &order.ID, "Points redeemed for order "+orderNumber); err != nil {
			s.logger.Error("failed to deduct loyalty points", zap.Error(err))
		}
	}

	// Mark cart as checked out
	cart.Status = CartStatusCheckedOut
	if err := s.repo.UpdateCart(ctx, cart); err != nil {
		s.logger.Error("failed to mark cart as checked out", zap.Error(err))
	}

	// Create initial order event
	s.createOrderEvent(ctx, order.ID, "order_created", "", string(OrderStatusPending), nil, &req.UserID, "user", "")

	// Publish order.created event to NATS
	s.publishOrderCreated(ctx, order, len(cart.Items))

	// For scheduled orders, publish a scheduling event so logistics can plan ahead
	if fulfillmentType == FulfillmentTypeScheduled {
		s.publishOrderScheduled(ctx, order)
	}

	s.logger.Info("order created",
		zap.String("id", order.ID.String()),
		zap.String("orderNumber", order.OrderNumber),
		zap.String("fulfillmentType", string(fulfillmentType)),
		zap.Float64("grandTotal", order.GrandTotal))

	return order, nil
}

// CreateOrderFromItems creates an order directly from a list of items (convenience endpoint for frontend).
func (s *OrderService) CreateOrderFromItems(ctx context.Context, req CreateOrderFromItemsRequest) (*Order, error) {
	// Enforce active subscription
	if err := s.checkSubscription(ctx, req.TenantID); err != nil {
		return nil, err
	}

	if len(req.Items) == 0 {
		return nil, ErrCartEmpty
	}

	// Determine fulfillment type (default to delivery)
	fulfillmentType := req.FulfillmentType
	if fulfillmentType == "" {
		fulfillmentType = FulfillmentTypeDelivery
	}

	// Cash on delivery is not available for pickup orders
	if fulfillmentType == FulfillmentTypePickup && req.PaymentMethod == "cod" {
		return nil, ErrCashNotAvailableForPickup
	}

	// Validate scheduled orders
	if fulfillmentType == FulfillmentTypeScheduled {
		if req.ScheduledFor == nil {
			return nil, ErrScheduledForRequired
		}
		if req.ScheduledFor.Before(time.Now().Add(ScheduledMinLeadTime)) {
			return nil, ErrScheduledForTooSoon
		}
	}

	var subtotal float64
	for _, it := range req.Items {
		subtotal += it.TotalPrice
	}

	deliveryFee := 0.0
	if fulfillmentType == FulfillmentTypePickup {
		// Pickup orders have no delivery fee
		deliveryFee = 0
	} else if req.DeliveryLat != nil && req.DeliveryLng != nil {
		fee, _, zoneErr := s.CalculateDeliveryFee(ctx, req.TenantID, &req.OutletID, *req.DeliveryLat, *req.DeliveryLng)
		if zoneErr == nil {
			deliveryFee = fee
		} else if zoneErr != ErrDeliveryNotServiceable {
			s.logger.Warn("delivery fee calculation failed, using default", zap.Error(zoneErr))
			deliveryFee = DeliveryFeeBase
		}
		// If ErrDeliveryNotServiceable, we let the order proceed with 0 fee
		// since this is the convenience endpoint and the address might be text-based
	}
	discountTotal := 0.0
	grandTotal := subtotal - discountTotal + deliveryFee

	// Reserve stock via inventory service (fail fast if stock unavailable)
	var reservationID *uuid.UUID
	if s.inventoryClient != nil {
		var reserveErr error
		reservationID, reserveErr = s.reserveStockForOrderItems(ctx, req.TenantID, uuid.Nil, req.Items, "")
		if reserveErr != nil {
			return nil, reserveErr
		}
	}

	orderNumber, err := s.repo.GenerateOrderNumber(ctx, req.TenantID, req.OutletID)
	if err != nil {
		s.logger.Error("failed to generate order number", zap.Error(err))
		// Release reservation if order number generation fails
		if reservationID != nil {
			go s.releaseOrderReservation(context.Background(), &Order{TenantID: req.TenantID, ReservationID: reservationID}, "order_creation_failed")
		}
		return nil, err
	}

	loyaltyPointsEarned := s.loyaltySvc.CalculatePointsForAmount(grandTotal)
	instructions := req.DeliveryAddress
	if req.DeliveryNotes != "" {
		instructions = instructions + "\n" + req.DeliveryNotes
	}

	now := time.Now()
	order := &Order{
		TenantID:            req.TenantID,
		OutletID:            req.OutletID,
		CustomerID:          &req.UserID,
		CartID:              nil,
		OrderNumber:         orderNumber,
		Status:              OrderStatusPending,
		PaymentStatus:       PaymentStatusPending,
		FulfillmentType:     fulfillmentType,
		ScheduledFor:        req.ScheduledFor,
		Currency:            "KES",
		Subtotal:            subtotal,
		DiscountTotal:       discountTotal,
		TaxTotal:            0,
		DeliveryFee:         deliveryFee,
		GrandTotal:          grandTotal,
		LoyaltyPointsEarned: loyaltyPointsEarned,
		Instructions:        instructions,
		Channel:             req.Channel,
		ReservationID:       reservationID,
		PlacedAt:            &now,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	if err := s.repo.CreateOrder(ctx, order); err != nil {
		s.logger.Error("failed to create order", zap.Error(err))
		// Release reservation on order creation failure
		if reservationID != nil {
			go s.releaseOrderReservation(context.Background(), &Order{TenantID: req.TenantID, ReservationID: reservationID}, "order_creation_failed")
		}
		return nil, err
	}

	for _, it := range req.Items {
		orderItem := &OrderItem{
			OrderID:       order.ID,
			InventorySKU: it.InventorySKU,
			NameSnapshot:  it.Name,
			Quantity:     it.Quantity,
			UnitPrice:    it.UnitPrice,
			TotalPrice:   it.TotalPrice,
		}
		if err := s.repo.CreateOrderItem(ctx, orderItem); err != nil {
			s.logger.Error("failed to create order item", zap.Error(err))
		}
	}

	s.createOrderEvent(ctx, order.ID, "order_created", "", string(OrderStatusPending), nil, &req.UserID, "user", "")
	s.publishOrderCreated(ctx, order, len(req.Items))

	// For scheduled orders, publish a scheduling event so logistics can plan ahead
	if fulfillmentType == FulfillmentTypeScheduled {
		s.publishOrderScheduled(ctx, order)
	}

	s.logger.Info("order created from items",
		zap.String("id", order.ID.String()),
		zap.String("orderNumber", order.OrderNumber),
		zap.String("fulfillmentType", string(fulfillmentType)),
		zap.Float64("grandTotal", order.GrandTotal))

	return order, nil
}

// GuestCheckout creates an order for a guest user. It accepts items directly from the
// frontend local cart. If items are provided in the request they are used; otherwise it
// falls back to looking up a backend cart by session (for backwards compatibility).
// Guest orders do not earn loyalty points. A nil UUID is used for CustomerID.
func (s *OrderService) GuestCheckout(ctx context.Context, req GuestCheckoutRequest) (*Order, error) {
	// Determine fulfillment type (default to delivery)
	fulfillmentType := req.FulfillmentType
	if fulfillmentType == "" {
		fulfillmentType = FulfillmentTypeDelivery
	}

	// Resolve items: prefer items from request payload, fall back to backend cart
	var orderItems []CreateOrderItemInput
	var cartID *uuid.UUID

	if len(req.Items) > 0 {
		orderItems = req.Items
	} else {
		// Legacy path: look up backend cart by session
		guestCart, err := s.repo.GetActiveCartBySession(ctx, req.TenantID, req.OutletID, req.SessionID)
		if err != nil || guestCart == nil {
			return nil, ErrCartNotFound
		}
		items, err := s.repo.ListCartItems(ctx, guestCart.ID)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			return nil, ErrCartEmpty
		}
		cartID = &guestCart.ID
		for _, ci := range items {
			orderItems = append(orderItems, CreateOrderItemInput{
				InventorySKU: ci.InventorySKU,
				Name:         ci.NameSnapshot,
				Quantity:     ci.Quantity,
				UnitPrice:    ci.UnitPrice,
				TotalPrice:   ci.TotalPrice,
			})
		}
		// Mark cart as checked out
		defer func() {
			guestCart.Status = CartStatusCheckedOut
			if uerr := s.repo.UpdateCart(ctx, guestCart); uerr != nil {
				s.logger.Error("failed to mark guest cart as checked out", zap.Error(uerr))
			}
		}()
	}

	if len(orderItems) == 0 {
		return nil, ErrCartEmpty
	}

	// Calculate totals from items
	var subtotal float64
	for _, it := range orderItems {
		subtotal += it.TotalPrice
	}

	deliveryFee := 0.0
	if fulfillmentType == FulfillmentTypePickup {
		deliveryFee = 0
	} else if req.DeliveryLat != nil && req.DeliveryLng != nil {
		fee, _, zoneErr := s.CalculateDeliveryFee(ctx, req.TenantID, &req.OutletID, *req.DeliveryLat, *req.DeliveryLng)
		if zoneErr == nil {
			deliveryFee = fee
		} else if zoneErr != ErrDeliveryNotServiceable {
			s.logger.Warn("guest delivery fee calculation failed, using default", zap.Error(zoneErr))
			deliveryFee = DeliveryFeeBase
		}
	}
	grandTotal := subtotal + deliveryFee

	// Generate order number
	orderNumber, err := s.repo.GenerateOrderNumber(ctx, req.TenantID, req.OutletID)
	if err != nil {
		s.logger.Error("failed to generate order number", zap.Error(err))
		return nil, err
	}

	// Build instructions with delivery info and contact details
	instructions := req.DeliveryAddress
	if req.DeliveryNotes != "" {
		instructions = instructions + "\n" + req.DeliveryNotes
	}
	if req.Instructions != "" {
		instructions = instructions + "\n" + req.Instructions
	}

	// Reserve stock via inventory service
	var reservationID *uuid.UUID
	if s.inventoryClient != nil {
		reservationID, err = s.reserveStockForOrderItems(ctx, req.TenantID, uuid.Nil, orderItems, "")
		if err != nil {
			return nil, err
		}
	}

	now := time.Now()
	order := &Order{
		TenantID:              req.TenantID,
		OutletID:              req.OutletID,
		CustomerID:            nil, // guest orders have no customer
		CartID:                cartID,
		OrderNumber:           orderNumber,
		Status:                OrderStatusPending,
		PaymentStatus:         PaymentStatusPending,
		FulfillmentType:       fulfillmentType,
		Currency:              "KES",
		Subtotal:              subtotal,
		DiscountTotal:         0,
		TaxTotal:              0,
		DeliveryFee:           deliveryFee,
		GrandTotal:            grandTotal,
		LoyaltyPointsEarned:   0,
		LoyaltyPointsRedeemed: 0,
		Instructions:          instructions,
		Channel:               req.Channel,
		Source:                 "guest",
		ReservationID:         reservationID,
		PlacedAt:              &now,
		CreatedAt:             now,
		UpdatedAt:             now,
		Metadata: map[string]interface{}{
			"guest":        true,
			"contactName":  req.ContactName,
			"contactEmail": req.ContactEmail,
			"contactPhone": req.ContactPhone,
			"sessionId":    req.SessionID,
		},
	}

	if err := s.repo.CreateOrder(ctx, order); err != nil {
		s.logger.Error("failed to create guest order", zap.Error(err))
		if reservationID != nil {
			go s.releaseOrderReservation(context.Background(), &Order{TenantID: req.TenantID, ReservationID: reservationID}, "order_creation_failed")
		}
		return nil, err
	}

	// Create order items
	for _, it := range orderItems {
		orderItem := &OrderItem{
			OrderID:      order.ID,
			InventorySKU: it.InventorySKU,
			NameSnapshot: it.Name,
			Quantity:     it.Quantity,
			UnitPrice:    it.UnitPrice,
			TotalPrice:   it.TotalPrice,
		}
		if err := s.repo.CreateOrderItem(ctx, orderItem); err != nil {
			s.logger.Error("failed to create guest order item", zap.Error(err))
		}
	}

	s.createOrderEvent(ctx, order.ID, "order_created", "", string(OrderStatusPending), nil, nil, "guest", "")
	s.publishOrderCreated(ctx, order, len(orderItems))

	s.logger.Info("guest order created",
		zap.String("id", order.ID.String()),
		zap.String("orderNumber", order.OrderNumber),
		zap.String("contactName", req.ContactName),
		zap.String("contactEmail", req.ContactEmail),
		zap.String("contactPhone", req.ContactPhone),
		zap.Float64("grandTotal", order.GrandTotal))

	return order, nil
}

// processStockConsumption handles background ingredient deduction from inventory based on recipes.
func (s *OrderService) processStockConsumption(ctx context.Context, order *Order) {
	if s.inventoryClient == nil {
		s.logger.Warn("inventory client not initialized, skipping stock consumption", zap.String("orderID", order.ID.String()))
		return
	}

	// Fetch tenant details for slug
	tenant, err := s.repo.GetTenantByID(ctx, order.TenantID)
	if err != nil {
		s.logger.Error("failed to get tenant for stock consumption", zap.Error(err), zap.String("tenantID", order.TenantID.String()))
		return
	}

	// Fetch order items if not loaded
	if len(order.Items) == 0 {
		items, err := s.repo.ListOrderItems(ctx, order.ID)
		if err != nil {
			s.logger.Error("failed to list order items for stock consumption", zap.Error(err), zap.String("orderID", order.ID.String()))
			return
		}
		order.Items = items
	}

	consumptionMap := make(map[string]float64) // SKU -> Quantity

	for _, item := range order.Items {
		// Item already carries its inventory SKU — no need to look up CatalogItem
		sku := item.InventorySKU
		if sku == "" {
			s.logger.Warn("order item missing inventory SKU", zap.String("itemID", item.ID.String()))
			continue
		}

		// Lookup recipe by SKU
		recipe, err := s.inventoryClient.GetRecipeBySKU(ctx, tenant.Slug, sku)
		if err != nil {
			// It's possible some items don't have recipes associated
			s.logger.Debug("no recipe found for catalog item", zap.String("sku", sku))
			continue
		}

		// Calculate ingredient needs: (ingredient.qty / recipe.output_qty) * order_item.qty
		for _, ing := range recipe.Ingredients {
			neededQty := (ing.Quantity / recipe.OutputQty) * float64(item.Quantity)
			consumptionMap[ing.SKU] += neededQty
		}
	}

	if len(consumptionMap) == 0 {
		return
	}

	// Convert map to consumption request items
	consumptionItems := make([]inventory.ConsumptionItem, 0, len(consumptionMap))
	for sku, qty := range consumptionMap {
		consumptionItems = append(consumptionItems, inventory.ConsumptionItem{
			SKU:      sku,
			Quantity: qty,
		})
	}

	req := inventory.ConsumptionRequest{
		TenantID: order.TenantID,
		OrderID:  order.ID,
		Items:    consumptionItems,
		Reason:   "sale",
	}

	resp, err := s.inventoryClient.RecordConsumption(ctx, tenant.Slug, req)
	if err != nil {
		s.logger.Error("failed to record stock consumption in inventory service", zap.Error(err), zap.String("orderID", order.ID.String()))
		return
	}

	s.logger.Info("stock consumption recorded successfully",
		zap.String("orderID", order.ID.String()),
		zap.String("consumptionID", resp.ID.String()))
}

// GetOrder retrieves an order by ID.
func (s *OrderService) GetOrder(ctx context.Context, tenantID, orderID uuid.UUID) (*Order, error) {
	order, err := s.repo.GetOrder(ctx, tenantID, orderID)
	if err != nil {
		return nil, err
	}

	// Load order items
	items, err := s.repo.ListOrderItems(ctx, orderID)
	if err == nil {
		order.Items = items
	}

	// Load events
	events, err := s.repo.ListOrderEvents(ctx, orderID)
	if err == nil {
		order.Events = events
	}

	// Load delivery address
	if order.DeliveryAddressID != nil {
		address, err := s.repo.GetAddress(ctx, tenantID, *order.DeliveryAddressID)
		if err == nil {
			order.DeliveryAddress = address
		}
	}

	// Enrich customer contact info
	s.enrichOrderContact(ctx, order)

	return order, nil
}

// DeleteOrder permanently removes an order and its items/events.
func (s *OrderService) DeleteOrder(ctx context.Context, tenantID, orderID uuid.UUID) error {
	order, err := s.repo.GetOrder(ctx, tenantID, orderID)
	if err != nil {
		return err
	}

	// Release any inventory reservation
	if order.ReservationID != nil {
		go s.releaseOrderReservation(context.Background(), order, "order_deleted")
	}

	return s.repo.DeleteOrder(ctx, tenantID, orderID)
}

// GetOrderEvents retrieves all lifecycle events for an order.
func (s *OrderService) GetOrderEvents(ctx context.Context, tenantID, orderID uuid.UUID) ([]OrderEvent, error) {
	// Verify order exists and belongs to tenant
	_, err := s.repo.GetOrder(ctx, tenantID, orderID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListOrderEvents(ctx, orderID)
}

// GetOrderByNumber retrieves an order by order number.
func (s *OrderService) GetOrderByNumber(ctx context.Context, tenantID uuid.UUID, orderNumber string) (*Order, error) {
	return s.repo.GetOrderByNumber(ctx, tenantID, orderNumber)
}

// ListOrders lists orders with filters.
func (s *OrderService) ListOrders(ctx context.Context, filter OrderFilter) ([]Order, int, error) {
	orders, total, err := s.repo.ListOrders(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	for i := range orders {
		s.enrichOrderContact(ctx, &orders[i])
	}
	return orders, total, nil
}

// enrichOrderContact populates CustomerName/Email/Phone on an order from metadata or user lookup.
func (s *OrderService) enrichOrderContact(ctx context.Context, order *Order) {
	ci := s.orderContactInfo(ctx, order)
	order.CustomerName = ci.Name
	order.CustomerEmail = ci.Email
	order.CustomerPhone = ci.Phone
}

// GetCustomerAddress retrieves a customer address by ID (used by checkout to resolve addressId).
func (s *OrderService) GetCustomerAddress(ctx context.Context, tenantID, addressID uuid.UUID) (*CustomerAddress, error) {
	return s.repo.GetAddress(ctx, tenantID, addressID)
}

// GetAnalyticsSummary returns aggregated metrics for the dashboard.
func (s *OrderService) GetAnalyticsSummary(ctx context.Context, tenantID uuid.UUID, dateFrom, dateTo time.Time) (*AnalyticsSummary, error) {
	return s.repo.GetAnalyticsSummary(ctx, tenantID, dateFrom, dateTo)
}

// UpdateOrderStatus transitions an order to a new status.
func (s *OrderService) UpdateOrderStatus(ctx context.Context, tenantID, orderID uuid.UUID, newStatus OrderStatus, actorID *uuid.UUID, actorType, ipAddress string) (*Order, error) {
	order, err := s.repo.GetOrder(ctx, tenantID, orderID)
	if err != nil {
		return nil, err
	}

	// Validate transition
	if !s.stateMachine.CanTransition(order.Status, newStatus) {
		return nil, ErrInvalidStatusTransition
	}

	// Update timestamps based on status
	now := time.Now()
	oldStatus := order.Status
	order.Status = newStatus

	switch newStatus {
	case OrderStatusConfirmed:
		order.ConfirmedAt = &now
	case OrderStatusReady:
		order.ReadyAt = &now
	case OrderStatusDelivered:
		order.DeliveredAt = &now
	case OrderStatusCompleted:
		order.CompletedAt = &now
		// Award loyalty points on completion
		if order.CustomerID != nil {
			if err := s.loyaltySvc.EarnPoints(ctx, tenantID, *order.CustomerID, order.LoyaltyPointsEarned, &order.ID, "Points earned for order "+order.OrderNumber); err != nil {
				s.logger.Error("failed to award loyalty points", zap.Error(err))
			}
		}
		// Consume (finalize) the inventory reservation so reserved stock is deducted
		go s.consumeOrderReservation(context.Background(), order)
		// Process stock consumption based on recipes (BOM)
		go s.processStockConsumption(context.Background(), order)
	case OrderStatusCancelled:
		order.CancelledAt = &now
		// Release inventory reservation on cancellation via status update
		go s.releaseOrderReservation(context.Background(), order, "order_cancelled")
	}

	if err := s.repo.UpdateOrder(ctx, order); err != nil {
		return nil, err
	}

	// Create order event
	s.createOrderEvent(ctx, order.ID, "status_changed", string(oldStatus), string(newStatus), nil, actorID, actorType, ipAddress)

	// Publish order.status.changed event to NATS
	s.publishOrderStatusChanged(ctx, order, oldStatus, newStatus)

	s.logger.Info("order status updated",
		zap.String("id", order.ID.String()),
		zap.String("from", string(oldStatus)),
		zap.String("to", string(newStatus)))

	return order, nil
}

// CancelOrder cancels an order with a reason.
func (s *OrderService) CancelOrder(ctx context.Context, tenantID, orderID uuid.UUID, reason string, actorID *uuid.UUID, actorType, ipAddress string) (*Order, error) {
	order, err := s.repo.GetOrder(ctx, tenantID, orderID)
	if err != nil {
		return nil, err
	}

	// Check if order can be cancelled
	if !s.stateMachine.CanTransition(order.Status, OrderStatusCancelled) {
		return nil, ErrOrderCannotBeCancelled
	}

	now := time.Now()
	oldStatus := order.Status
	order.Status = OrderStatusCancelled
	order.CancelledAt = &now
	order.CancellationReason = reason

	if err := s.repo.UpdateOrder(ctx, order); err != nil {
		return nil, err
	}

	// Release inventory reservation if one exists
	go s.releaseOrderReservation(context.Background(), order, "order_cancelled")

	// Refund loyalty points if they were redeemed
	if order.LoyaltyPointsRedeemed > 0 && order.CustomerID != nil {
		if err := s.loyaltySvc.EarnPoints(ctx, tenantID, *order.CustomerID, order.LoyaltyPointsRedeemed, &order.ID, "Points refunded for cancelled order "+order.OrderNumber); err != nil {
			s.logger.Error("failed to refund loyalty points", zap.Error(err))
		}
	}

	// Create order event
	payload := map[string]interface{}{"reason": reason}
	s.createOrderEvent(ctx, order.ID, "order_cancelled", string(oldStatus), string(OrderStatusCancelled), payload, actorID, actorType, ipAddress)

	// Publish order.cancelled event to NATS
	s.publishOrderCancelled(ctx, order, reason, actorType)

	s.logger.Info("order cancelled",
		zap.String("id", order.ID.String()),
		zap.String("reason", reason))

	return order, nil
}

// RateOrder submits a customer rating (1-5 stars) for a delivered/completed order.
// RateOrderOpts holds optional rider rating fields for the RateOrder call.
type RateOrderOpts struct {
	RiderRating  *int
	RiderComment string
}

func (s *OrderService) RateOrder(ctx context.Context, tenantID, orderID, customerID uuid.UUID, rating int, comment string, opts ...RateOrderOpts) (*Order, error) {
	if rating < 1 || rating > 5 {
		return nil, ErrInvalidRating
	}

	order, err := s.repo.GetOrder(ctx, tenantID, orderID)
	if err != nil {
		return nil, err
	}

	// Only the order owner can rate
	if order.CustomerID == nil || *order.CustomerID != customerID {
		return nil, ErrUnauthorized
	}

	// Only delivered or completed orders can be rated
	if order.Status != OrderStatusDelivered && order.Status != OrderStatusCompleted {
		return nil, ErrOrderNotRatable
	}

	// Prevent double-rating
	if order.Rating != nil {
		return nil, ErrAlreadyRated
	}

	now := time.Now()
	order.Rating = &rating
	order.RatingComment = comment
	order.RatedAt = &now

	if err := s.repo.UpdateOrder(ctx, order); err != nil {
		return nil, err
	}

	// Recalculate outlet rating aggregate
	go s.updateOutletRating(context.Background(), order.TenantID, order.OutletID, rating, comment)

	// Publish order.rated event
	if s.eventPublisher != nil {
		evt := events.NewEvent("ordering.order.rated", tenantID, map[string]interface{}{
			"order_id":    order.ID.String(),
			"order_number": order.OrderNumber,
			"customer_id": customerID.String(),
			"outlet_id":   order.OutletID.String(),
			"rating":      rating,
			"comment":     comment,
		})
		_ = s.eventPublisher.Publish(ctx, "ordering.order.rated", evt)
	}

	// Rate the rider via logistics if rider rating was provided and order has a delivery assignment
	if len(opts) > 0 && opts[0].RiderRating != nil && s.logisticsClient != nil {
		riderRating := *opts[0].RiderRating
		if riderRating >= 1 && riderRating <= 5 {
			// Get tenant slug for the logistics API call
			tenant, tenantErr := s.repo.GetTenantByID(ctx, tenantID)
			if tenantErr == nil {
				// Find the logistics task ID from order assignments
				// The fulfilment module stores LogisticsTaskID in OrderAssignment
				go func() {
					rateErr := s.logisticsClient.RateRider(context.Background(), tenant.Slug, order.ID.String(), logistics.RateRiderRequest{
						Rating:  riderRating,
						Comment: opts[0].RiderComment,
					})
					if rateErr != nil {
						s.logger.Warn("failed to rate rider via logistics",
							zap.Error(rateErr),
							zap.String("order_id", order.ID.String()))
					}
				}()
			}
		}
	}

	s.logger.Info("order rated",
		zap.String("id", order.ID.String()),
		zap.Int("rating", rating))

	return order, nil
}

// updateOutletRating recalculates and upserts the materialized outlet rating aggregate.
func (s *OrderService) updateOutletRating(ctx context.Context, tenantID, outletID uuid.UUID, newRating int, comment string) {
	existing, err := s.repo.GetOutletRating(ctx, tenantID, outletID)
	if err != nil {
		// Not found — initialize
		existing = &OutletRatingData{
			TenantID: tenantID,
			OutletID: outletID,
		}
	}

	// Increment star bucket
	switch newRating {
	case 5:
		existing.FiveStar++
	case 4:
		existing.FourStar++
	case 3:
		existing.ThreeStar++
	case 2:
		existing.TwoStar++
	case 1:
		existing.OneStar++
	}

	existing.TotalRatings++
	if comment != "" {
		existing.TotalReviews++
	}

	// Recalculate weighted average
	totalStars := float64(existing.FiveStar*5 + existing.FourStar*4 + existing.ThreeStar*3 + existing.TwoStar*2 + existing.OneStar*1)
	existing.AverageRating = totalStars / float64(existing.TotalRatings)

	if err := s.repo.UpsertOutletRating(ctx, existing); err != nil {
		s.logger.Error("failed to upsert outlet rating", zap.Error(err),
			zap.String("outletID", outletID.String()))
	}
}

// GetOutletRating retrieves the materialized rating aggregate for an outlet.
func (s *OrderService) GetOutletRating(ctx context.Context, tenantID, outletID uuid.UUID) (*OutletRatingData, error) {
	return s.repo.GetOutletRating(ctx, tenantID, outletID)
}

// RefundOrder processes a full refund for an order.
// It validates the order status, initiates a refund via treasury-api,
// releases inventory reservations, and publishes an order.refunded event.
func (s *OrderService) RefundOrder(ctx context.Context, req RefundOrderRequest) (*Order, error) {
	order, err := s.repo.GetOrder(ctx, req.TenantID, req.OrderID)
	if err != nil {
		return nil, err
	}

	// Validate order can be refunded (must be delivered or completed)
	if order.Status != OrderStatusDelivered && order.Status != OrderStatusCompleted {
		return nil, ErrOrderNotRefundable
	}

	// Check if already refunded
	if order.PaymentStatus == PaymentStatusRefunded {
		return nil, ErrOrderAlreadyRefunded
	}

	// Validate state machine transition
	if !s.stateMachine.CanTransition(order.Status, OrderStatusRefunded) {
		return nil, ErrOrderNotRefundable
	}

	// Initiate refund via treasury-api if a treasury client is configured
	// and the order has a payment (grand total > 0)
	if s.treasuryClient != nil && order.GrandTotal > 0 {
		// Look up payment intent for this order from metadata or payments module
		// The order stores payment_intent_id in metadata after payment processing
		var paymentID uuid.UUID
		if order.Metadata != nil {
			if pidStr, ok := order.Metadata["payment_intent_id"].(string); ok {
				paymentID, _ = uuid.Parse(pidStr)
			}
		}

		if paymentID != uuid.Nil {
			refundReq := treasury.RefundRequest{
				TenantID:       req.TenantID,
				PaymentID:      paymentID,
				Amount:         order.GrandTotal,
				Reason:         req.Reason,
				IdempotencyKey: fmt.Sprintf("refund-%s", req.OrderID.String()),
			}

			_, refundErr := s.treasuryClient.CreateRefund(ctx, refundReq)
			if refundErr != nil {
				s.logger.Error("failed to create refund via treasury",
					zap.Error(refundErr),
					zap.String("orderID", req.OrderID.String()))
				return nil, fmt.Errorf("%w: %v", ErrRefundFailed, refundErr)
			}
		}
	}

	// Release inventory reservation if one exists
	if order.ReservationID != nil && s.inventoryClient != nil {
		tenant, tenantErr := s.repo.GetTenantByID(ctx, req.TenantID)
		if tenantErr == nil {
			if releaseErr := s.inventoryClient.ReleaseReservation(ctx, tenant.Slug, *order.ReservationID, "order_refunded"); releaseErr != nil {
				s.logger.Warn("failed to release reservation on refund",
					zap.Error(releaseErr),
					zap.String("reservationID", order.ReservationID.String()))
			}
		}
	}

	// Update order status
	oldStatus := order.Status
	now := time.Now()
	order.Status = OrderStatusRefunded
	order.PaymentStatus = PaymentStatusRefunded
	order.CancellationReason = req.Reason
	order.CancelledAt = &now

	if err := s.repo.UpdateOrder(ctx, order); err != nil {
		return nil, fmt.Errorf("failed to update order status to refunded: %w", err)
	}

	// Create order event
	payload := map[string]interface{}{
		"reason": req.Reason,
	}
	actorType := "staff"
	s.createOrderEvent(ctx, order.ID, "order_refunded", string(oldStatus), string(OrderStatusRefunded), payload, req.ActorID, actorType, "")

	// Publish order.refunded event to NATS
	s.publishOrderRefunded(ctx, order, req.Reason)

	// Publish status change event
	s.publishOrderStatusChanged(ctx, order, oldStatus, OrderStatusRefunded)

	s.logger.Info("order refunded",
		zap.String("id", order.ID.String()),
		zap.String("orderNumber", order.OrderNumber),
		zap.String("reason", req.Reason),
		zap.Float64("grandTotal", order.GrandTotal))

	return order, nil
}

// publishOrderRefunded publishes an order.refunded event to NATS.
func (s *OrderService) publishOrderRefunded(ctx context.Context, order *Order, reason string) {
	if s.eventPublisher == nil {
		return
	}

	ci := s.orderContactInfo(ctx, order)
	data := events.OrderRefundedData{
		OrderID:       order.ID,
		OrderNumber:   order.OrderNumber,
		CustomerID:    customerIDValue(order.CustomerID),
		CustomerEmail: ci.Email,
		CustomerName:  ci.Name,
		CustomerPhone: ci.Phone,
		TotalAmount:   order.GrandTotal,
		Currency:      order.Currency,
		Reason:        reason,
		RefundedAt:    time.Now(),
	}

	if err := s.eventPublisher.PublishOrderRefunded(ctx, order.TenantID, data); err != nil {
		s.logger.Error("failed to publish order.refunded event",
			zap.Error(err),
			zap.String("order_id", order.ID.String()))
	}
}

// CalculateDeliveryFee determines the delivery fee by matching a coordinate against
// active DeliveryZone polygons. Returns the fee and estimated time, or an error if
// the location is not serviceable.
func (s *OrderService) CalculateDeliveryFee(ctx context.Context, tenantID uuid.UUID, outletID *uuid.UUID, lat, lng float64) (float64, int, error) {
	zones, err := s.repo.ListActiveDeliveryZones(ctx, tenantID, outletID)
	if err != nil {
		return 0, 0, err
	}

	for _, zone := range zones {
		if zone.ZonePolygon == nil {
			continue
		}
		if pointInZonePolygon(lat, lng, zone.ZonePolygon) {
			return zone.DeliveryFee, zone.EstimatedTimeMinutes, nil
		}
	}

	// No zone matched — fall back to constants if no zones are configured
	if len(zones) == 0 {
		return DeliveryFeeBase, 30, nil
	}

	return 0, 0, ErrDeliveryNotServiceable
}

// pointInZonePolygon checks if a lat/lng point falls within a GeoJSON polygon.
// The polygon is expected to be in GeoJSON format with "coordinates" as [][][]float64
// (the first ring is the outer boundary). Uses the ray-casting algorithm.
func pointInZonePolygon(lat, lng float64, polygon map[string]interface{}) bool {
	coords, ok := polygon["coordinates"]
	if !ok {
		return false
	}

	// GeoJSON Polygon: coordinates is [][][]float64 (array of rings, each ring is array of [lng, lat])
	rings, ok := coords.([]interface{})
	if !ok || len(rings) == 0 {
		return false
	}

	// Use the outer ring (first ring)
	outerRing, ok := rings[0].([]interface{})
	if !ok || len(outerRing) < 3 {
		return false
	}

	// Extract points
	type point struct{ x, y float64 }
	points := make([]point, 0, len(outerRing))
	for _, p := range outerRing {
		pair, ok := p.([]interface{})
		if !ok || len(pair) < 2 {
			continue
		}
		pLng, ok1 := toFloat64(pair[0])
		pLat, ok2 := toFloat64(pair[1])
		if !ok1 || !ok2 {
			continue
		}
		points = append(points, point{pLng, pLat})
	}

	if len(points) < 3 {
		return false
	}

	// Ray-casting algorithm
	inside := false
	n := len(points)
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		yi, xi := points[i].y, points[i].x
		yj, xj := points[j].y, points[j].x

		if ((yi > lat) != (yj > lat)) &&
			(lng < (xj-xi)*(lat-yi)/(yj-yi)+xi) {
			inside = !inside
		}
	}

	return inside
}

// toFloat64 converts a JSON number to float64.
func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

// UpdatePaymentStatus updates the payment status of an order.
func (s *OrderService) UpdatePaymentStatus(ctx context.Context, tenantID, orderID uuid.UUID, newStatus PaymentStatus, payload map[string]interface{}) (*Order, error) {
	order, err := s.repo.GetOrder(ctx, tenantID, orderID)
	if err != nil {
		return nil, err
	}

	oldStatus := order.PaymentStatus
	order.PaymentStatus = newStatus

	// Auto-confirm order on successful payment
	if newStatus == PaymentStatusPaid && order.Status == OrderStatusPending {
		now := time.Now()
		order.Status = OrderStatusConfirmed
		order.ConfirmedAt = &now
	}

	if err := s.repo.UpdateOrder(ctx, order); err != nil {
		return nil, err
	}

	// Create order event
	s.createOrderEvent(ctx, order.ID, "payment_"+string(newStatus), string(oldStatus), string(newStatus), payload, nil, "system", "")

	s.logger.Info("order payment status updated",
		zap.String("id", order.ID.String()),
		zap.String("paymentStatus", string(newStatus)))

	return order, nil
}

// createOrderEvent creates an order event record.
func (s *OrderService) createOrderEvent(ctx context.Context, orderID uuid.UUID, eventType, fromStatus, toStatus string, payload map[string]interface{}, actorID *uuid.UUID, actorType, ipAddress string) {
	event := &OrderEvent{
		OrderID:     orderID,
		EventType:   eventType,
		FromStatus:  fromStatus,
		ToStatus:    toStatus,
		Payload:     payload,
		ActorUserID: actorID,
		ActorType:   actorType,
		IPAddress:   ipAddress,
		OccurredAt:  time.Now(),
	}

	if err := s.repo.CreateOrderEvent(ctx, event); err != nil {
		s.logger.Error("failed to create order event", zap.Error(err))
	}
}

// --- Inventory Reservation Helpers ---

// reserveStockForItems checks availability and creates an inventory reservation
// for a set of cart items (or order items). Returns the reservation ID on success,
// or a wrapped ErrStockNotAvailable on failure.
func (s *OrderService) reserveStockForItems(ctx context.Context, tenantID, orderID uuid.UUID, items []CartItem, idempotencyKey string) (*uuid.UUID, error) {
	tenant, err := s.repo.GetTenantByID(ctx, tenantID)
	if err != nil {
		s.logger.Warn("failed to get tenant for inventory reservation, skipping", zap.Error(err))
		return nil, nil
	}

	reservationItems := make([]inventory.ReservationItem, 0, len(items))
	for _, ci := range items {
		if ci.InventorySKU != "" {
			reservationItems = append(reservationItems, inventory.ReservationItem{
				SKU:      ci.InventorySKU,
				Quantity: ci.Quantity,
			})
		}
	}
	if len(reservationItems) == 0 {
		return nil, nil
	}

	// Check bulk availability first to return actionable item-level details
	skus := make([]string, len(reservationItems))
	for i, ri := range reservationItems {
		skus[i] = ri.SKU
	}
	availabilities, bulkErr := s.inventoryClient.CheckBulkAvailability(ctx, tenant.Slug, skus)
	if bulkErr == nil {
		// Build a lookup map
		availMap := make(map[string]int, len(availabilities))
		for _, a := range availabilities {
			availMap[a.SKU] = a.Available
		}
		var unavailable []string
		for _, ri := range reservationItems {
			avail, found := availMap[ri.SKU]
			if !found || avail < ri.Quantity {
				unavailable = append(unavailable, ri.SKU)
			}
		}
		if len(unavailable) > 0 {
			s.logger.Warn("insufficient stock for items",
				zap.Strings("skus", unavailable),
				zap.String("tenantID", tenantID.String()))
			return nil, fmt.Errorf("%w: insufficient stock for SKUs %v", ErrStockNotAvailable, unavailable)
		}
	}

	// Use the provided orderID, or generate a placeholder if not yet known
	resOrderID := orderID
	if resOrderID == uuid.Nil {
		resOrderID = uuid.New()
	}

	reserveReq := inventory.ReservationRequest{
		TenantID:       tenantID,
		OrderID:        resOrderID,
		Items:          reservationItems,
		IdempotencyKey: idempotencyKey,
	}
	reservation, reserveErr := s.inventoryClient.CreateReservation(ctx, tenant.Slug, reserveReq)
	if reserveErr != nil {
		s.logger.Warn("stock reservation failed", zap.Error(reserveErr))
		return nil, fmt.Errorf("%w: %v", ErrStockNotAvailable, reserveErr)
	}

	// Check for partial reservations
	for _, ri := range reservation.Items {
		if !ri.IsFullyReserved {
			s.logger.Warn("partial reservation detected, releasing",
				zap.String("sku", ri.SKU),
				zap.Int("requested", ri.RequestedQty),
				zap.Int("reserved", ri.ReservedQty))
			// Release the partial reservation and fail the order
			_ = s.inventoryClient.ReleaseReservation(ctx, tenant.Slug, reservation.ID, "partial_reservation_rejected")
			return nil, fmt.Errorf("%w: item %s only has %d available (requested %d)",
				ErrStockNotAvailable, ri.SKU, ri.AvailableQty, ri.RequestedQty)
		}
	}

	return &reservation.ID, nil
}

// reserveStockForOrderItems is a convenience wrapper that converts OrderItems
// to the CartItem shape expected by reserveStockForItems.
func (s *OrderService) reserveStockForOrderItems(ctx context.Context, tenantID, orderID uuid.UUID, items []CreateOrderItemInput, idempotencyKey string) (*uuid.UUID, error) {
	cartItems := make([]CartItem, 0, len(items))
	for _, it := range items {
		if it.InventorySKU != "" {
			cartItems = append(cartItems, CartItem{
				InventorySKU: it.InventorySKU,
				Quantity:     it.Quantity,
			})
		}
	}
	return s.reserveStockForItems(ctx, tenantID, orderID, cartItems, idempotencyKey)
}

// releaseOrderReservation releases the inventory reservation for an order, if one exists.
func (s *OrderService) releaseOrderReservation(ctx context.Context, order *Order, reason string) {
	if order.ReservationID == nil || s.inventoryClient == nil {
		return
	}
	tenant, err := s.repo.GetTenantByID(ctx, order.TenantID)
	if err != nil {
		s.logger.Error("failed to get tenant for reservation release", zap.Error(err))
		return
	}
	if releaseErr := s.inventoryClient.ReleaseReservation(ctx, tenant.Slug, *order.ReservationID, reason); releaseErr != nil {
		s.logger.Warn("failed to release inventory reservation",
			zap.Error(releaseErr),
			zap.String("reservationID", order.ReservationID.String()),
			zap.String("orderID", order.ID.String()))
	} else {
		s.logger.Info("inventory reservation released",
			zap.String("reservationID", order.ReservationID.String()),
			zap.String("orderID", order.ID.String()),
			zap.String("reason", reason))
	}
}

// consumeOrderReservation consumes (finalizes) the inventory reservation for a completed order.
func (s *OrderService) consumeOrderReservation(ctx context.Context, order *Order) {
	if order.ReservationID == nil || s.inventoryClient == nil {
		return
	}
	tenant, err := s.repo.GetTenantByID(ctx, order.TenantID)
	if err != nil {
		s.logger.Error("failed to get tenant for reservation consumption", zap.Error(err))
		return
	}
	if consumeErr := s.inventoryClient.ConsumeReservation(ctx, tenant.Slug, *order.ReservationID); consumeErr != nil {
		s.logger.Warn("failed to consume inventory reservation",
			zap.Error(consumeErr),
			zap.String("reservationID", order.ReservationID.String()),
			zap.String("orderID", order.ID.String()))
	} else {
		s.logger.Info("inventory reservation consumed",
			zap.String("reservationID", order.ReservationID.String()),
			zap.String("orderID", order.ID.String()))
	}
}

// --- Event Publishing Helpers ---

// orderContactInfo extracts customer name, email, phone from an order.
// For guest orders these come from order.Metadata; for authenticated orders
// from the user profile (looked up via auth-client cache when available).
type contactInfo struct {
	Name  string
	Email string
	Phone string
}

func (s *OrderService) orderContactInfo(ctx context.Context, order *Order) contactInfo {
	ci := contactInfo{}

	// Guest orders: contact info is in metadata
	if order.Metadata != nil {
		if v, ok := order.Metadata["contactName"].(string); ok {
			ci.Name = v
		}
		if v, ok := order.Metadata["contactEmail"].(string); ok {
			ci.Email = v
		}
		if v, ok := order.Metadata["contactPhone"].(string); ok {
			ci.Phone = v
		}
	}

	// Authenticated orders: resolve from the customer edge on the order
	// (loaded when order is fetched with WithCustomer, or look up from DB)
	if ci.Email == "" && order.CustomerID != nil {
		uci, err := s.repo.FindUserByID(ctx, *order.CustomerID)
		if err == nil && uci != nil {
			if ci.Email == "" {
				ci.Email = uci.Email
			}
			if ci.Name == "" {
				// If FullName is just the email, derive a display name from the email prefix
				if uci.FullName != "" && uci.FullName != uci.Email {
					ci.Name = uci.FullName
				} else if uci.Email != "" {
					parts := strings.SplitN(uci.Email, "@", 2)
					ci.Name = strings.ReplaceAll(parts[0], ".", " ")
					// Title-case the derived name
					words := strings.Fields(ci.Name)
					for i, w := range words {
						if len(w) > 0 {
							words[i] = strings.ToUpper(w[:1]) + w[1:]
						}
					}
					ci.Name = strings.Join(words, " ")
				}
			}
			if ci.Phone == "" {
				ci.Phone = uci.Phone
			}
		}
	}

	return ci
}

// publishOrderCreated publishes an order.created event to NATS.
func (s *OrderService) publishOrderCreated(ctx context.Context, order *Order, itemCount int) {
	if s.eventPublisher == nil {
		return
	}

	ci := s.orderContactInfo(ctx, order)
	data := events.OrderCreatedData{
		OrderID:       order.ID,
		OrderNumber:   order.OrderNumber,
		CustomerID:    customerIDValue(order.CustomerID),
		CustomerEmail: ci.Email,
		CustomerName:  ci.Name,
		CustomerPhone: ci.Phone,
		OutletID:      order.OutletID,
		TotalAmount:   order.GrandTotal,
		Currency:      order.Currency,
		ItemCount:     itemCount,
	}

	if err := s.eventPublisher.PublishOrderCreated(ctx, order.TenantID, data); err != nil {
		s.logger.Error("failed to publish order.created event",
			zap.Error(err),
			zap.String("order_id", order.ID.String()))
	}
}

// publishOrderStatusChanged publishes an order.status.changed event to NATS.
func (s *OrderService) publishOrderStatusChanged(ctx context.Context, order *Order, oldStatus, newStatus OrderStatus) {
	if s.eventPublisher == nil {
		return
	}

	ci := s.orderContactInfo(ctx, order)
	data := events.OrderStatusChangedData{
		OrderID:        order.ID,
		OrderNumber:    order.OrderNumber,
		CustomerID:     customerIDValue(order.CustomerID),
		CustomerEmail:  ci.Email,
		CustomerName:   ci.Name,
		CustomerPhone:  ci.Phone,
		PreviousStatus: string(oldStatus),
		NewStatus:      string(newStatus),
		ChangedAt:      time.Now(),
	}

	if err := s.eventPublisher.PublishOrderStatusChanged(ctx, order.TenantID, data); err != nil {
		s.logger.Error("failed to publish order.status.changed event",
			zap.Error(err),
			zap.String("order_id", order.ID.String()))
	}

	// Publish specific events for key status changes
	switch newStatus {
	case OrderStatusReady:
		if order.FulfillmentType == FulfillmentTypePickup {
			// Pickup orders: notify customer for pickup instead of logistics dispatch
			s.publishOrderForPickup(ctx, order)
		} else {
			s.publishOrderReady(ctx, order)
		}
	case OrderStatusCompleted:
		s.publishOrderCompleted(ctx, order)
	}
}

// publishOrderScheduled publishes an order.scheduled event to NATS so logistics can plan ahead.
func (s *OrderService) publishOrderScheduled(ctx context.Context, order *Order) {
	if s.eventPublisher == nil {
		return
	}

	if order.ScheduledFor == nil {
		return
	}

	ci := s.orderContactInfo(ctx, order)
	data := events.OrderScheduledData{
		OrderID:       order.ID,
		OrderNumber:   order.OrderNumber,
		CustomerID:    customerIDValue(order.CustomerID),
		CustomerEmail: ci.Email,
		CustomerName:  ci.Name,
		CustomerPhone: ci.Phone,
		OutletID:      order.OutletID,
		ScheduledFor:  *order.ScheduledFor,
		TotalAmount:   order.GrandTotal,
		Currency:      order.Currency,
	}

	if err := s.eventPublisher.PublishOrderScheduled(ctx, order.TenantID, data); err != nil {
		s.logger.Error("failed to publish order.scheduled event",
			zap.Error(err),
			zap.String("order_id", order.ID.String()))
	}
}

// publishOrderForPickup publishes an order.for_pickup event to NATS (notify customer for pickup).
func (s *OrderService) publishOrderForPickup(ctx context.Context, order *Order) {
	if s.eventPublisher == nil {
		return
	}

	ci := s.orderContactInfo(ctx, order)
	data := events.OrderForPickupData{
		OrderID:       order.ID,
		OrderNumber:   order.OrderNumber,
		CustomerID:    customerIDValue(order.CustomerID),
		CustomerEmail: ci.Email,
		CustomerName:  ci.Name,
		CustomerPhone: ci.Phone,
		OutletID:      order.OutletID,
	}

	if err := s.eventPublisher.PublishOrderForPickup(ctx, order.TenantID, data); err != nil {
		s.logger.Error("failed to publish order.for_pickup event",
			zap.Error(err),
			zap.String("order_id", order.ID.String()))
	}
}

// publishOrderReady publishes an order.ready event to NATS (for logistics integration).
func (s *OrderService) publishOrderReady(ctx context.Context, order *Order) {
	if s.eventPublisher == nil {
		return
	}

	ci := s.orderContactInfo(ctx, order)
	data := events.OrderReadyData{
		OrderID:         order.ID,
		OrderNumber:     order.OrderNumber,
		OutletID:        order.OutletID,
		CustomerID:      customerIDValue(order.CustomerID),
		CustomerEmail:   ci.Email,
		CustomerName:    ci.Name,
		CustomerPhone:   ci.Phone,
		PaymentMethod:   string(order.PaymentMethod),
		DeliveryFee:     order.DeliveryFee,
		GrandTotal:      order.GrandTotal,
		Instructions:    order.Instructions,
		FulfillmentType: string(order.FulfillmentType),
	}
	// For COD orders, set the cash collection amount
	if order.PaymentMethod == PaymentMethodCOD {
		data.CashOnDelivery = order.GrandTotal
	}

	// Add delivery address if available
	if order.DeliveryAddress != nil {
		data.DeliveryAddress = map[string]interface{}{
			"id":            order.DeliveryAddress.ID.String(),
			"label":         order.DeliveryAddress.Label,
			"address_line1": order.DeliveryAddress.AddressLine1,
			"address_line2": order.DeliveryAddress.AddressLine2,
			"city":          order.DeliveryAddress.City,
			"county":        order.DeliveryAddress.County,
			"country":       order.DeliveryAddress.Country,
			"latitude":      order.DeliveryAddress.Latitude,
			"longitude":     order.DeliveryAddress.Longitude,
			"contact_name":  order.DeliveryAddress.ContactName,
			"contact_phone": order.DeliveryAddress.ContactPhone,
			"instructions":  order.DeliveryAddress.Instructions,
		}
		if order.DeliveryAddress.ContactName != "" {
			data.CustomerName = order.DeliveryAddress.ContactName
		}
		if order.DeliveryAddress.ContactPhone != "" {
			data.CustomerPhone = order.DeliveryAddress.ContactPhone
		}
	}

	// Add outlet location for logistics pickup coordinates
	outletName, outletLat, outletLng, err := s.repo.GetOutletLocation(ctx, order.TenantID, order.OutletID)
	if err != nil {
		s.logger.Warn("failed to get outlet location for order.ready event", zap.Error(err))
	} else if outletLat != nil && outletLng != nil {
		data.OutletLocation = map[string]interface{}{
			"name":      outletName,
			"latitude":  *outletLat,
			"longitude": *outletLng,
		}
	}

	if err := s.eventPublisher.PublishOrderReady(ctx, order.TenantID, data); err != nil {
		s.logger.Error("failed to publish order.ready event",
			zap.Error(err),
			zap.String("order_id", order.ID.String()))
	}
}

// publishOrderCompleted publishes an order.completed event to NATS.
func (s *OrderService) publishOrderCompleted(ctx context.Context, order *Order) {
	if s.eventPublisher == nil {
		return
	}

	ci := s.orderContactInfo(ctx, order)
	data := events.OrderCompletedData{
		OrderID:       order.ID,
		OrderNumber:   order.OrderNumber,
		CustomerID:    customerIDValue(order.CustomerID),
		CustomerEmail: ci.Email,
		CustomerName:  ci.Name,
		CustomerPhone: ci.Phone,
		TotalAmount:   order.GrandTotal,
		Currency:      order.Currency,
		CompletedAt:   time.Now(),
	}

	if err := s.eventPublisher.PublishOrderCompleted(ctx, order.TenantID, data); err != nil {
		s.logger.Error("failed to publish order.completed event",
			zap.Error(err),
			zap.String("order_id", order.ID.String()))
	}
}

// publishOrderCancelled publishes an order.cancelled event to NATS.
func (s *OrderService) publishOrderCancelled(ctx context.Context, order *Order, reason, cancelledBy string) {
	if s.eventPublisher == nil {
		return
	}

	ci := s.orderContactInfo(ctx, order)
	data := events.OrderCancelledData{
		OrderID:       order.ID,
		OrderNumber:   order.OrderNumber,
		CustomerID:    customerIDValue(order.CustomerID),
		CustomerEmail: ci.Email,
		CustomerName:  ci.Name,
		CustomerPhone: ci.Phone,
		Reason:        reason,
		CancelledBy:   cancelledBy,
		CancelledAt:   time.Now(),
	}

	if err := s.eventPublisher.PublishOrderCancelled(ctx, order.TenantID, data); err != nil {
		s.logger.Error("failed to publish order.cancelled event",
			zap.Error(err),
			zap.String("order_id", order.ID.String()))
	}
}

// ReorderFromPastOrder creates a new cart populated with items from a past order.
// Items that are no longer available are silently skipped.
func (s *OrderService) ReorderFromPastOrder(ctx context.Context, tenantID, userID, orderID uuid.UUID) (*Cart, error) {
	// 1. Get past order with items
	order, err := s.repo.GetOrder(ctx, tenantID, orderID)
	if err != nil {
		return nil, err
	}

	// Ensure the order belongs to this user
	if order.CustomerID == nil || *order.CustomerID != userID {
		return nil, ErrUnauthorized
	}

	// Load order items
	items, err := s.repo.ListOrderItems(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrCartEmpty
	}

	// 2. Create new cart
	cart, err := s.cartSvc.GetOrCreateCart(ctx, tenantID, order.OutletID, &userID, "")
	if err != nil {
		return nil, err
	}

	// 3. For each order item, add to cart (best-effort: skip unavailable)
	for _, item := range items {
		_, addErr := s.cartSvc.AddItem(ctx, AddItemRequest{
			TenantID:     tenantID,
			OutletID:     order.OutletID,
			UserID:       &userID,
			InventorySKU: item.InventorySKU,
			VariantID:    item.VariantID,
			Quantity:     item.Quantity,
			Notes:        item.Notes,
		})
		if addErr != nil {
			s.logger.Warn("skipping unavailable item during reorder",
				zap.String("sku", item.InventorySKU),
				zap.Error(addErr))
			continue
		}
	}

	// 4. Return cart with items
	return s.cartSvc.GetCart(ctx, tenantID, cart.ID)
}

// OrderStateMachine defines valid order status transitions.
type OrderStateMachine struct {
	transitions map[OrderStatus][]OrderStatus
}

// NewOrderStateMachine creates a new order state machine.
func NewOrderStateMachine() *OrderStateMachine {
	return &OrderStateMachine{
		transitions: map[OrderStatus][]OrderStatus{
			OrderStatusPending: {
				OrderStatusConfirmed,
				OrderStatusCancelled,
			},
			OrderStatusConfirmed: {
				OrderStatusPreparing,
				OrderStatusCancelled,
			},
			OrderStatusPreparing: {
				OrderStatusReady,
				OrderStatusCancelled,
			},
			OrderStatusReady: {
				OrderStatusOutForDelivery,
				OrderStatusCompleted, // For pickup orders
				OrderStatusCancelled,
			},
			OrderStatusOutForDelivery: {
				OrderStatusDelivered,
				OrderStatusCancelled,
			},
			OrderStatusDelivered: {
				OrderStatusCompleted,
				OrderStatusRefunded,
			},
			OrderStatusCompleted: {
				OrderStatusRefunded,
			},
			OrderStatusCancelled: {
				// No transitions from cancelled
			},
			OrderStatusRefunded: {
				// No transitions from refunded
			},
		},
	}
}

// CanTransition checks if a status transition is valid.
func (m *OrderStateMachine) CanTransition(from, to OrderStatus) bool {
	allowedTransitions, ok := m.transitions[from]
	if !ok {
		return false
	}

	for _, allowed := range allowedTransitions {
		if allowed == to {
			return true
		}
	}

	return false
}

// GetAllowedTransitions returns the allowed transitions from a status.
func (m *OrderStateMachine) GetAllowedTransitions(from OrderStatus) []OrderStatus {
	return m.transitions[from]
}

// ValidateStatusTransition validates and returns a helpful error message.
func (m *OrderStateMachine) ValidateStatusTransition(from, to OrderStatus) error {
	if m.CanTransition(from, to) {
		return nil
	}

	allowed := m.GetAllowedTransitions(from)
	if len(allowed) == 0 {
		return fmt.Errorf("order status '%s' is final and cannot be changed", from)
	}

	return fmt.Errorf("cannot transition from '%s' to '%s'; allowed transitions: %v", from, to, allowed)
}
