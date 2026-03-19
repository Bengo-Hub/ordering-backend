package ordering

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/Bengo-Hub/httpware"
	authclient "github.com/Bengo-Hub/shared-auth-client"
	"github.com/bengobox/ordering-backend/internal/platform/events"
	"github.com/bengobox/ordering-backend/internal/platform/inventory"
	"github.com/bengobox/ordering-backend/internal/platform/subscriptions"
)

// OrderService provides order business logic.
type OrderService struct {
	repo                Repository
	cartSvc             *CartService
	promoSvc            *PromoService
	loyaltySvc          *LoyaltyService
	stateMachine        *OrderStateMachine
	eventPublisher      *events.Publisher
	inventoryClient     *inventory.Client
	subscriptionsClient *subscriptions.Client
	logger              *zap.Logger
}

// NewOrderService creates a new order service.
func NewOrderService(
	repo Repository,
	cartSvc *CartService,
	promoSvc *PromoService,
	loyaltySvc *LoyaltyService,
	inventoryClient *inventory.Client,
	subscriptionsClient *subscriptions.Client,
	logger *zap.Logger,
) *OrderService {
	return &OrderService{
		repo:                repo,
		cartSvc:             cartSvc,
		promoSvc:            promoSvc,
		loyaltySvc:          loyaltySvc,
		inventoryClient:     inventoryClient,
		subscriptionsClient: subscriptionsClient,
		stateMachine:        NewOrderStateMachine(),
		logger:              logger,
	}
}

// SetEventPublisher sets the event publisher for the order service.
// This is called after initialization to avoid circular dependencies.
func (s *OrderService) SetEventPublisher(publisher *events.Publisher) {
	s.eventPublisher = publisher
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

	// Validate delivery address if provided
	var deliveryAddress *CustomerAddress
	if req.DeliveryAddressID != nil {
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

	// Calculate totals
	grandTotal := cart.Subtotal - discountTotal - loyaltyDiscount + cart.TaxTotal + cart.DeliveryFee

	// Generate order number
	orderNumber, err := s.repo.GenerateOrderNumber(ctx, req.TenantID, cart.OutletID)
	if err != nil {
		s.logger.Error("failed to generate order number", zap.Error(err))
		return nil, err
	}

	// Calculate loyalty points earned
	loyaltyPointsEarned := int(grandTotal * float64(LoyaltyPointsPerUnit))

	// Create order
	now := time.Now()
	order := &Order{
		TenantID:              req.TenantID,
		OutletID:              cart.OutletID,
		CustomerID:            req.UserID,
		CartID:                &cart.ID,
		OrderNumber:           orderNumber,
		Status:                OrderStatusPending,
		PaymentStatus:         PaymentStatusPending,
		Currency:              cart.Currency,
		Subtotal:              cart.Subtotal,
		DiscountTotal:         discountTotal,
		TaxTotal:              cart.TaxTotal,
		DeliveryFee:           cart.DeliveryFee,
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
	}

	if err := s.repo.CreateOrder(ctx, order); err != nil {
		s.logger.Error("failed to create order", zap.Error(err))
		return nil, err
	}

	// Create order items from cart items
	for _, cartItem := range cart.Items {
		orderItem := &OrderItem{
			OrderID:       order.ID,
			CatalogItemID: cartItem.CatalogItemID,
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

	s.logger.Info("order created",
		zap.String("id", order.ID.String()),
		zap.String("orderNumber", order.OrderNumber),
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

	var subtotal float64
	for _, it := range req.Items {
		subtotal += it.TotalPrice
	}

	deliveryFee := 0.0
	discountTotal := 0.0
	grandTotal := subtotal - discountTotal + deliveryFee

	orderNumber, err := s.repo.GenerateOrderNumber(ctx, req.TenantID, req.OutletID)
	if err != nil {
		s.logger.Error("failed to generate order number", zap.Error(err))
		return nil, err
	}

	loyaltyPointsEarned := int(grandTotal * float64(LoyaltyPointsPerUnit))
	instructions := req.DeliveryAddress
	if req.DeliveryNotes != "" {
		instructions = instructions + "\n" + req.DeliveryNotes
	}

	now := time.Now()
	order := &Order{
		TenantID:            req.TenantID,
		OutletID:            req.OutletID,
		CustomerID:          req.UserID,
		CartID:              nil,
		OrderNumber:         orderNumber,
		Status:              OrderStatusPending,
		PaymentStatus:       PaymentStatusPending,
		Currency:            "KES",
		Subtotal:            subtotal,
		DiscountTotal:       discountTotal,
		TaxTotal:            0,
		DeliveryFee:         deliveryFee,
		GrandTotal:          grandTotal,
		LoyaltyPointsEarned: loyaltyPointsEarned,
		Instructions:        instructions,
		Channel:             req.Channel,
		PlacedAt:            &now,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	if err := s.repo.CreateOrder(ctx, order); err != nil {
		s.logger.Error("failed to create order", zap.Error(err))
		return nil, err
	}

	for _, it := range req.Items {
		orderItem := &OrderItem{
			OrderID:       order.ID,
			CatalogItemID: it.CatalogItemID,
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

	s.logger.Info("order created from items",
		zap.String("id", order.ID.String()),
		zap.String("orderNumber", order.OrderNumber),
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
		// Get catalog item to find its SKU code
		catalogItem, err := s.repo.GetCatalogItemByID(ctx, order.TenantID, item.CatalogItemID)
		if err != nil {
			s.logger.Warn("failed to get catalog item for recipe lookup", zap.Error(err), zap.String("catalogItemID", item.CatalogItemID.String()))
			continue
		}

		// Lookup recipe by SKU
		recipe, err := s.inventoryClient.GetRecipeBySKU(ctx, tenant.Slug, catalogItem.SKU)
		if err != nil {
			// It's possible some items don't have recipes associated
			s.logger.Debug("no recipe found for catalog item", zap.String("sku", catalogItem.SKU))
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

	return order, nil
}

// GetOrderByNumber retrieves an order by order number.
func (s *OrderService) GetOrderByNumber(ctx context.Context, tenantID uuid.UUID, orderNumber string) (*Order, error) {
	return s.repo.GetOrderByNumber(ctx, tenantID, orderNumber)
}

// ListOrders lists orders with filters.
func (s *OrderService) ListOrders(ctx context.Context, filter OrderFilter) ([]Order, int, error) {
	return s.repo.ListOrders(ctx, filter)
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
		if err := s.loyaltySvc.EarnPoints(ctx, tenantID, order.CustomerID, order.LoyaltyPointsEarned, &order.ID, "Points earned for order "+order.OrderNumber); err != nil {
			s.logger.Error("failed to award loyalty points", zap.Error(err))
		}
		// Process stock consumption based on recipes (BOM)
		go s.processStockConsumption(context.Background(), order)
	case OrderStatusCancelled:
		order.CancelledAt = &now
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

	// Refund loyalty points if they were redeemed
	if order.LoyaltyPointsRedeemed > 0 {
		if err := s.loyaltySvc.EarnPoints(ctx, tenantID, order.CustomerID, order.LoyaltyPointsRedeemed, &order.ID, "Points refunded for cancelled order "+order.OrderNumber); err != nil {
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
func (s *OrderService) RateOrder(ctx context.Context, tenantID, orderID, customerID uuid.UUID, rating int, comment string) (*Order, error) {
	if rating < 1 || rating > 5 {
		return nil, ErrInvalidRating
	}

	order, err := s.repo.GetOrder(ctx, tenantID, orderID)
	if err != nil {
		return nil, err
	}

	// Only the order owner can rate
	if order.CustomerID != customerID {
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

	// Publish order.rated event
	if s.eventPublisher != nil {
		evt := events.NewEvent("ordering.order.rated", tenantID, map[string]interface{}{
			"order_id":    order.ID.String(),
			"order_number": order.OrderNumber,
			"customer_id": customerID.String(),
			"rating":      rating,
			"comment":     comment,
		})
		_ = s.eventPublisher.Publish(ctx, "ordering.order.rated", evt)
	}

	s.logger.Info("order rated",
		zap.String("id", order.ID.String()),
		zap.Int("rating", rating))

	return order, nil
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

// --- Event Publishing Helpers ---

// publishOrderCreated publishes an order.created event to NATS.
func (s *OrderService) publishOrderCreated(ctx context.Context, order *Order, itemCount int) {
	if s.eventPublisher == nil {
		return
	}

	data := events.OrderCreatedData{
		OrderID:     order.ID,
		OrderNumber: order.OrderNumber,
		CustomerID:  order.CustomerID,
		OutletID:    order.OutletID,
		TotalAmount: order.GrandTotal,
		Currency:    order.Currency,
		ItemCount:   itemCount,
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

	data := events.OrderStatusChangedData{
		OrderID:        order.ID,
		OrderNumber:    order.OrderNumber,
		CustomerID:     order.CustomerID,
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
		s.publishOrderReady(ctx, order)
	case OrderStatusCompleted:
		s.publishOrderCompleted(ctx, order)
	}
}

// publishOrderReady publishes an order.ready event to NATS (for logistics integration).
func (s *OrderService) publishOrderReady(ctx context.Context, order *Order) {
	if s.eventPublisher == nil {
		return
	}

	data := events.OrderReadyData{
		OrderID:     order.ID,
		OrderNumber: order.OrderNumber,
		OutletID:    order.OutletID,
		CustomerID:  order.CustomerID,
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

	data := events.OrderCompletedData{
		OrderID:     order.ID,
		OrderNumber: order.OrderNumber,
		CustomerID:  order.CustomerID,
		TotalAmount: order.GrandTotal,
		Currency:    order.Currency,
		CompletedAt: time.Now(),
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

	data := events.OrderCancelledData{
		OrderID:     order.ID,
		OrderNumber: order.OrderNumber,
		CustomerID:  order.CustomerID,
		Reason:      reason,
		CancelledBy: cancelledBy,
		CancelledAt: time.Now(),
	}

	if err := s.eventPublisher.PublishOrderCancelled(ctx, order.TenantID, data); err != nil {
		s.logger.Error("failed to publish order.cancelled event",
			zap.Error(err),
			zap.String("order_id", order.ID.String()))
	}
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
