package ordering

import (
	"context"
	"fmt"
	"time"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/cart"
	"github.com/bengobox/ordering-backend/internal/ent/cartitem"
	"github.com/bengobox/ordering-backend/internal/ent/order"
	"github.com/bengobox/ordering-backend/internal/ent/orderevent"
	"github.com/bengobox/ordering-backend/internal/ent/orderitem"
	"github.com/bengobox/ordering-backend/internal/ent/menuitem"
	"github.com/google/uuid"
	"github.com/bengobox/ordering-backend/internal/modules/catalog"
)

// EntRepository implements Repository using Ent ORM.
type EntRepository struct {
	client *ent.Client
}

// NewEntRepository creates a new Ent-based ordering repository.
func NewEntRepository(client *ent.Client) *EntRepository {
	return &EntRepository{client: client}
}

// --- Cart Methods ---

func (r *EntRepository) CreateCart(ctx context.Context, c *Cart) error {
	builder := r.client.Cart.Create().
		SetTenantID(c.TenantID).
		SetCafeID(c.CafeID).
		SetStatus(cart.Status(c.Status)).
		SetCurrency(c.Currency).
		SetSubtotal(c.Subtotal).
		SetDiscountTotal(c.DiscountTotal).
		SetTaxTotal(c.TaxTotal).
		SetDeliveryFee(c.DeliveryFee).
		SetLoyaltyPointsRedeemed(c.LoyaltyPointsRedeemed)

	if c.UserID != nil {
		builder.SetUserID(*c.UserID)
	}
	if c.SessionID != "" {
		builder.SetSessionID(c.SessionID)
	}
	if c.PromoCodeID != nil {
		builder.SetPromoCodeID(*c.PromoCodeID)
	}
	if c.ExpiresAt != nil {
		builder.SetExpiresAt(*c.ExpiresAt)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}

	c.ID = created.ID
	c.CreatedAt = created.CreatedAt
	c.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *EntRepository) GetCart(ctx context.Context, tenantID, cartID uuid.UUID) (*Cart, error) {
	c, err := r.client.Cart.Query().
		Where(
			cart.ID(cartID),
			cart.TenantID(tenantID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrCartNotFound
		}
		return nil, err
	}
	return entCartToDomain(c), nil
}

func (r *EntRepository) GetActiveCartByUser(ctx context.Context, tenantID, cafeID, userID uuid.UUID) (*Cart, error) {
	c, err := r.client.Cart.Query().
		Where(
			cart.TenantID(tenantID),
			cart.CafeID(cafeID),
			cart.UserID(userID),
			cart.StatusEQ(cart.StatusActive),
		).
		Order(ent.Desc(cart.FieldCreatedAt)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrCartNotFound
		}
		return nil, err
	}
	return entCartToDomain(c), nil
}

func (r *EntRepository) GetActiveCartBySession(ctx context.Context, tenantID, cafeID uuid.UUID, sessionID string) (*Cart, error) {
	c, err := r.client.Cart.Query().
		Where(
			cart.TenantID(tenantID),
			cart.CafeID(cafeID),
			cart.SessionID(sessionID),
			cart.StatusEQ(cart.StatusActive),
		).
		Order(ent.Desc(cart.FieldCreatedAt)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrCartNotFound
		}
		return nil, err
	}
	return entCartToDomain(c), nil
}

func (r *EntRepository) UpdateCart(ctx context.Context, c *Cart) error {
	builder := r.client.Cart.UpdateOneID(c.ID).
		SetStatus(cart.Status(c.Status)).
		SetSubtotal(c.Subtotal).
		SetDiscountTotal(c.DiscountTotal).
		SetTaxTotal(c.TaxTotal).
		SetDeliveryFee(c.DeliveryFee).
		SetLoyaltyPointsRedeemed(c.LoyaltyPointsRedeemed)

	if c.PromoCodeID != nil {
		builder.SetPromoCodeID(*c.PromoCodeID)
	} else {
		builder.ClearPromoCodeID()
	}
	if c.ExpiresAt != nil {
		builder.SetExpiresAt(*c.ExpiresAt)
	}

	updated, err := builder.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrCartNotFound
		}
		return err
	}

	c.UpdatedAt = updated.UpdatedAt
	return nil
}

func (r *EntRepository) DeleteCart(ctx context.Context, tenantID, cartID uuid.UUID) error {
	_, err := r.client.Cart.Delete().
		Where(
			cart.ID(cartID),
			cart.TenantID(tenantID),
		).Exec(ctx)
	return err
}

func (r *EntRepository) ListCarts(ctx context.Context, filter CartFilter) ([]Cart, int, error) {
	query := r.client.Cart.Query().
		Where(cart.TenantID(filter.TenantID))

	if filter.CafeID != nil {
		query = query.Where(cart.CafeID(*filter.CafeID))
	}
	if filter.UserID != nil {
		query = query.Where(cart.UserID(*filter.UserID))
	}
	if filter.SessionID != "" {
		query = query.Where(cart.SessionID(filter.SessionID))
	}
	if filter.Status != nil {
		query = query.Where(cart.StatusEQ(cart.Status(*filter.Status)))
	}

	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	query = query.Order(ent.Desc(cart.FieldCreatedAt))
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	carts, err := query.All(ctx)
	if err != nil {
		return nil, 0, err
	}

	result := make([]Cart, len(carts))
	for i, c := range carts {
		result[i] = *entCartToDomain(c)
	}
	return result, total, nil
}

func (r *EntRepository) ExpireOldCarts(ctx context.Context, tenantID uuid.UUID) (int, error) {
	affected, err := r.client.Cart.Update().
		Where(
			cart.TenantID(tenantID),
			cart.StatusEQ(cart.StatusActive),
			cart.ExpiresAtLT(time.Now()),
		).
		SetStatus(cart.StatusExpired).
		Save(ctx)
	return affected, err
}

// --- CartItem Methods ---

func (r *EntRepository) CreateCartItem(ctx context.Context, item *CartItem) error {
	builder := r.client.CartItem.Create().
		SetCartID(item.CartID).
		SetMenuItemID(item.MenuItemID).
		SetNameSnapshot(item.NameSnapshot).
		SetQuantity(item.Quantity).
		SetUnitPrice(item.UnitPrice).
		SetTotalPrice(item.TotalPrice).
		SetNotes(item.Notes)

	if item.VariantID != nil {
		builder.SetVariantID(*item.VariantID)
	}
	if item.Metadata != nil {
		builder.SetMetadata(item.Metadata)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}

	item.ID = created.ID
	item.CreatedAt = created.CreatedAt
	item.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *EntRepository) GetCartItem(ctx context.Context, cartID, itemID uuid.UUID) (*CartItem, error) {
	item, err := r.client.CartItem.Query().
		Where(
			cartitem.ID(itemID),
			cartitem.CartID(cartID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrCartItemNotFound
		}
		return nil, err
	}
	return entCartItemToDomain(item), nil
}

func (r *EntRepository) GetCartItemByMenuItem(ctx context.Context, cartID, menuItemID uuid.UUID, variantID *uuid.UUID) (*CartItem, error) {
	query := r.client.CartItem.Query().
		Where(
			cartitem.CartID(cartID),
			cartitem.MenuItemID(menuItemID),
		)

	if variantID != nil {
		query = query.Where(cartitem.VariantID(*variantID))
	} else {
		query = query.Where(cartitem.VariantIDIsNil())
	}

	item, err := query.Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrCartItemNotFound
		}
		return nil, err
	}
	return entCartItemToDomain(item), nil
}

func (r *EntRepository) UpdateCartItem(ctx context.Context, item *CartItem) error {
	builder := r.client.CartItem.UpdateOneID(item.ID).
		SetQuantity(item.Quantity).
		SetTotalPrice(item.TotalPrice).
		SetNotes(item.Notes)

	if item.Metadata != nil {
		builder.SetMetadata(item.Metadata)
	}

	updated, err := builder.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrCartItemNotFound
		}
		return err
	}

	item.UpdatedAt = updated.UpdatedAt
	return nil
}

func (r *EntRepository) DeleteCartItem(ctx context.Context, cartID, itemID uuid.UUID) error {
	_, err := r.client.CartItem.Delete().
		Where(
			cartitem.ID(itemID),
			cartitem.CartID(cartID),
		).Exec(ctx)
	return err
}

func (r *EntRepository) ListCartItems(ctx context.Context, cartID uuid.UUID) ([]CartItem, error) {
	items, err := r.client.CartItem.Query().
		Where(cartitem.CartID(cartID)).
		Order(ent.Asc(cartitem.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]CartItem, len(items))
	for i, item := range items {
		result[i] = *entCartItemToDomain(item)
	}
	return result, nil
}

func (r *EntRepository) ClearCartItems(ctx context.Context, cartID uuid.UUID) error {
	_, err := r.client.CartItem.Delete().
		Where(cartitem.CartID(cartID)).
		Exec(ctx)
	return err
}

// --- Order Methods ---

func (r *EntRepository) CreateOrder(ctx context.Context, o *Order) error {
	builder := r.client.Order.Create().
		SetTenantID(o.TenantID).
		SetCafeID(o.CafeID).
		SetCustomerID(o.CustomerID).
		SetOrderNumber(o.OrderNumber).
		SetStatus(order.Status(o.Status)).
		SetPaymentStatus(order.PaymentStatus(o.PaymentStatus)).
		SetCurrency(o.Currency).
		SetSubtotal(o.Subtotal).
		SetDiscountTotal(o.DiscountTotal).
		SetTaxTotal(o.TaxTotal).
		SetDeliveryFee(o.DeliveryFee).
		SetTipTotal(o.TipTotal).
		SetGrandTotal(o.GrandTotal).
		SetLoyaltyPointsEarned(o.LoyaltyPointsEarned).
		SetLoyaltyPointsRedeemed(o.LoyaltyPointsRedeemed).
		SetChannel(order.Channel(o.Channel))

	if o.CartID != nil {
		builder.SetCartID(*o.CartID)
	}
	if o.DeliveryAddressID != nil {
		builder.SetDeliveryAddressID(*o.DeliveryAddressID)
	}
	if o.PromoCodeID != nil {
		builder.SetPromoCodeID(*o.PromoCodeID)
	}
	if o.Instructions != "" {
		builder.SetInstructions(o.Instructions)
	}
	if o.Source != "" {
		builder.SetSource(o.Source)
	}
	if o.IdempotencyKey != "" {
		builder.SetIdempotencyKey(o.IdempotencyKey)
	}
	if o.PlacedAt != nil {
		builder.SetPlacedAt(*o.PlacedAt)
	}
	if o.Metadata != nil {
		builder.SetMetadata(o.Metadata)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}

	o.ID = created.ID
	o.CreatedAt = created.CreatedAt
	o.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *EntRepository) GetOrder(ctx context.Context, tenantID, orderID uuid.UUID) (*Order, error) {
	o, err := r.client.Order.Query().
		Where(
			order.ID(orderID),
			order.TenantID(tenantID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	return entOrderToDomain(o), nil
}

func (r *EntRepository) GetOrderByNumber(ctx context.Context, tenantID uuid.UUID, orderNumber string) (*Order, error) {
	o, err := r.client.Order.Query().
		Where(
			order.TenantID(tenantID),
			order.OrderNumber(orderNumber),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	return entOrderToDomain(o), nil
}

func (r *EntRepository) GetOrderByIdempotencyKey(ctx context.Context, tenantID uuid.UUID, key string) (*Order, error) {
	o, err := r.client.Order.Query().
		Where(
			order.TenantID(tenantID),
			order.IdempotencyKey(key),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	return entOrderToDomain(o), nil
}

func (r *EntRepository) UpdateOrder(ctx context.Context, o *Order) error {
	builder := r.client.Order.UpdateOneID(o.ID).
		SetStatus(order.Status(o.Status)).
		SetPaymentStatus(order.PaymentStatus(o.PaymentStatus))

	if o.ConfirmedAt != nil {
		builder.SetConfirmedAt(*o.ConfirmedAt)
	}
	if o.ReadyAt != nil {
		builder.SetReadyAt(*o.ReadyAt)
	}
	if o.DeliveredAt != nil {
		builder.SetDeliveredAt(*o.DeliveredAt)
	}
	if o.CompletedAt != nil {
		builder.SetCompletedAt(*o.CompletedAt)
	}
	if o.CancelledAt != nil {
		builder.SetCancelledAt(*o.CancelledAt)
	}
	if o.CancellationReason != "" {
		builder.SetCancellationReason(o.CancellationReason)
	}
	if o.Metadata != nil {
		builder.SetMetadata(o.Metadata)
	}
	if o.Rating != nil {
		builder.SetRating(*o.Rating)
	}
	if o.RatingComment != "" {
		builder.SetRatingComment(o.RatingComment)
	}
	if o.RatedAt != nil {
		builder.SetRatedAt(*o.RatedAt)
	}

	updated, err := builder.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrOrderNotFound
		}
		return err
	}

	o.UpdatedAt = updated.UpdatedAt
	return nil
}

func (r *EntRepository) ListOrders(ctx context.Context, filter OrderFilter) ([]Order, int, error) {
	query := r.client.Order.Query().
		Where(order.TenantID(filter.TenantID))

	if filter.CafeID != nil {
		query = query.Where(order.CafeID(*filter.CafeID))
	}
	if filter.CustomerID != nil {
		query = query.Where(order.CustomerID(*filter.CustomerID))
	}
	if filter.Status != nil {
		query = query.Where(order.StatusEQ(order.Status(*filter.Status)))
	}
	if filter.PaymentStatus != nil {
		query = query.Where(order.PaymentStatusEQ(order.PaymentStatus(*filter.PaymentStatus)))
	}
	if filter.DateFrom != nil {
		query = query.Where(order.CreatedAtGTE(*filter.DateFrom))
	}
	if filter.DateTo != nil {
		query = query.Where(order.CreatedAtLTE(*filter.DateTo))
	}
	if filter.Search != "" {
		query = query.Where(order.OrderNumberContains(filter.Search))
	}

	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	query = query.Order(ent.Desc(order.FieldCreatedAt))
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	orders, err := query.All(ctx)
	if err != nil {
		return nil, 0, err
	}

	result := make([]Order, len(orders))
	for i, o := range orders {
		result[i] = *entOrderToDomain(o)
	}
	return result, total, nil
}


// --- Analytics ---

func (r *EntRepository) GetAnalyticsSummary(ctx context.Context, tenantID uuid.UUID, dateFrom, dateTo time.Time) (*AnalyticsSummary, error) {
	orders, err := r.client.Order.Query().
		Where(
			order.TenantID(tenantID),
			order.CreatedAtGTE(dateFrom),
			order.CreatedAtLTE(dateTo),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	summary := &AnalyticsSummary{
		TotalOrders:       0,
		TotalRevenue:      0,
		OrdersByStatus:    make(map[string]int),
		RevenueByCurrency: make(map[string]float64),
		TopSellingItems:   make([]ItemSalesSummary, 0),
		Trend:             make([]DailyMetric, 0),
	}

	trendMap := make(map[string]*DailyMetric)

	for _, o := range orders {
		summary.TotalOrders++
		summary.TotalRevenue += o.GrandTotal
		summary.OrdersByStatus[string(o.Status)]++
		summary.RevenueByCurrency[o.Currency] += o.GrandTotal

		dateKey := o.CreatedAt.Format("2006-01-02")
		if _, ok := trendMap[dateKey]; !ok {
			trendMap[dateKey] = &DailyMetric{Date: dateKey}
		}
		trendMap[dateKey].Orders++
		trendMap[dateKey].Revenue += o.GrandTotal
	}

	// Calculate cancelled orders
	cancelledOrdersCount, err := r.client.Order.Query().
		Where(
			order.TenantID(tenantID),
			order.CreatedAtGTE(dateFrom),
			order.CreatedAtLTE(dateTo),
			order.StatusEQ(order.StatusCancelled), // Fixed: Use order.StatusCancelled
		).
		Count(ctx)
	if err != nil {
		return nil, err
	}
	summary.CancelledOrders = cancelledOrdersCount

	// Simple aggregation for items (optimization: use direct SQL or items table list)
	orderIDs := make([]uuid.UUID, len(orders))
	for i, o := range orders {
		orderIDs[i] = o.ID
	}

	if len(orderIDs) > 0 {
		items, err := r.client.OrderItem.Query().
			Where(orderitem.OrderIDIn(orderIDs...)).
			All(ctx)
		if err == nil {
			itemStats := make(map[uuid.UUID]*ItemSalesSummary)
			for _, it := range items {
				if _, ok := itemStats[it.MenuItemID]; !ok {
					itemStats[it.MenuItemID] = &ItemSalesSummary{
						MenuItemID:   it.MenuItemID,
						NameSnapshot: it.NameSnapshot,
					}
				}
				itemStats[it.MenuItemID].Quantity += it.Quantity
				itemStats[it.MenuItemID].Revenue += it.TotalPrice
			}
			for _, stats := range itemStats {
				summary.TopSellingItems = append(summary.TopSellingItems, *stats)
			}
		}
	}

	// Populate trend from map
	summary.Trend = make([]DailyMetric, 0, len(trendMap))
	for _, m := range trendMap {
		summary.Trend = append(summary.Trend, *m)
	}

	return summary, nil
}

func (r *EntRepository) GenerateOrderNumber(ctx context.Context, tenantID, cafeID uuid.UUID) (string, error) {
	// Get today's date prefix
	today := time.Now().Format("20060102")

	// Count orders today for this cafe
	count, err := r.client.Order.Query().
		Where(
			order.TenantID(tenantID),
			order.CafeID(cafeID),
			order.CreatedAtGTE(time.Now().Truncate(24*time.Hour)),
		).
		Count(ctx)
	if err != nil {
		return "", err
	}

	// Format: YYYYMMDD-NNNN (e.g., 20260117-0001)
	return fmt.Sprintf("%s-%04d", today, count+1), nil
}

// --- OrderItem Methods ---

func (r *EntRepository) CreateOrderItem(ctx context.Context, item *OrderItem) error {
	builder := r.client.OrderItem.Create().
		SetOrderID(item.OrderID).
		SetMenuItemID(item.MenuItemID).
		SetNameSnapshot(item.NameSnapshot).
		SetQuantity(item.Quantity).
		SetUnitPrice(item.UnitPrice).
		SetTotalPrice(item.TotalPrice).
		SetNotes(item.Notes)

	if item.VariantID != nil {
		builder.SetVariantID(*item.VariantID)
	}
	if item.Metadata != nil {
		builder.SetMetadata(item.Metadata)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}

	item.ID = created.ID
	return nil
}

func (r *EntRepository) GetOrderItem(ctx context.Context, orderID, itemID uuid.UUID) (*OrderItem, error) {
	item, err := r.client.OrderItem.Query().
		Where(
			orderitem.ID(itemID),
			orderitem.OrderID(orderID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	return entOrderItemToDomain(item), nil
}

func (r *EntRepository) ListOrderItems(ctx context.Context, orderID uuid.UUID) ([]OrderItem, error) {
	items, err := r.client.OrderItem.Query().
		Where(orderitem.OrderID(orderID)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]OrderItem, len(items))
	for i, item := range items {
		result[i] = *entOrderItemToDomain(item)
	}
	return result, nil
}

// --- OrderEvent Methods ---

func (r *EntRepository) CreateOrderEvent(ctx context.Context, event *OrderEvent) error {
	builder := r.client.OrderEvent.Create().
		SetOrderID(event.OrderID).
		SetEventType(event.EventType).
		SetOccurredAt(event.OccurredAt)

	if event.FromStatus != "" {
		builder.SetFromStatus(event.FromStatus)
	}
	if event.ToStatus != "" {
		builder.SetToStatus(event.ToStatus)
	}
	if event.Payload != nil {
		builder.SetPayload(event.Payload)
	}
	if event.ActorUserID != nil {
		builder.SetActorUserID(*event.ActorUserID)
	}
	if event.ActorType != "" {
		builder.SetActorType(event.ActorType)
	}
	if event.IPAddress != "" {
		builder.SetIPAddress(event.IPAddress)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}

	event.ID = created.ID
	return nil
}

func (r *EntRepository) ListOrderEvents(ctx context.Context, orderID uuid.UUID) ([]OrderEvent, error) {
	events, err := r.client.OrderEvent.Query().
		Where(orderevent.OrderID(orderID)).
		Order(ent.Asc(orderevent.FieldOccurredAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]OrderEvent, len(events))
	for i, e := range events {
		result[i] = *entOrderEventToDomain(e)
	}
	return result, nil
}

// --- Domain Conversion Functions ---

func entCartToDomain(c *ent.Cart) *Cart {
	cart := &Cart{
		ID:                    c.ID,
		TenantID:              c.TenantID,
		CafeID:                c.CafeID,
		SessionID:             c.SessionID,
		Status:                CartStatus(c.Status),
		Currency:              c.Currency,
		Subtotal:              c.Subtotal,
		DiscountTotal:         c.DiscountTotal,
		TaxTotal:              c.TaxTotal,
		DeliveryFee:           c.DeliveryFee,
		LoyaltyPointsRedeemed: c.LoyaltyPointsRedeemed,
		CreatedAt:             c.CreatedAt,
		UpdatedAt:             c.UpdatedAt,
	}

	if c.UserID != nil {
		cart.UserID = c.UserID
	}
	if c.PromoCodeID != nil {
		cart.PromoCodeID = c.PromoCodeID
	}
	if c.ExpiresAt != nil {
		cart.ExpiresAt = c.ExpiresAt
	}

	return cart
}

func entCartItemToDomain(item *ent.CartItem) *CartItem {
	ci := &CartItem{
		ID:           item.ID,
		CartID:       item.CartID,
		MenuItemID:   item.MenuItemID,
		NameSnapshot: item.NameSnapshot,
		Quantity:     item.Quantity,
		UnitPrice:    item.UnitPrice,
		TotalPrice:   item.TotalPrice,
		Notes:        item.Notes,
		Metadata:     item.Metadata,
		CreatedAt:    item.CreatedAt,
		UpdatedAt:    item.UpdatedAt,
	}

	if item.VariantID != nil {
		ci.VariantID = item.VariantID
	}

	return ci
}

func entOrderToDomain(o *ent.Order) *Order {
	ord := &Order{
		ID:                    o.ID,
		TenantID:              o.TenantID,
		CafeID:                o.CafeID,
		CustomerID:            o.CustomerID,
		OrderNumber:           o.OrderNumber,
		Status:                OrderStatus(o.Status),
		PaymentStatus:         PaymentStatus(o.PaymentStatus),
		Currency:              o.Currency,
		Subtotal:              o.Subtotal,
		DiscountTotal:         o.DiscountTotal,
		TaxTotal:              o.TaxTotal,
		DeliveryFee:           o.DeliveryFee,
		TipTotal:              o.TipTotal,
		GrandTotal:            o.GrandTotal,
		LoyaltyPointsEarned:   o.LoyaltyPointsEarned,
		LoyaltyPointsRedeemed: o.LoyaltyPointsRedeemed,
		Instructions:          o.Instructions,
		Channel:               OrderChannel(o.Channel),
		Source:                o.Source,
		IdempotencyKey:        o.IdempotencyKey,
		CancellationReason:    o.CancellationReason,
		Metadata:              o.Metadata,
		CreatedAt:             o.CreatedAt,
		UpdatedAt:             o.UpdatedAt,
	}

	if o.CartID != nil {
		ord.CartID = o.CartID
	}
	if o.DeliveryAddressID != nil {
		ord.DeliveryAddressID = o.DeliveryAddressID
	}
	if o.PromoCodeID != nil {
		ord.PromoCodeID = o.PromoCodeID
	}
	if o.PlacedAt != nil {
		ord.PlacedAt = o.PlacedAt
	}
	if o.ConfirmedAt != nil {
		ord.ConfirmedAt = o.ConfirmedAt
	}
	if o.ReadyAt != nil {
		ord.ReadyAt = o.ReadyAt
	}
	if o.DeliveredAt != nil {
		ord.DeliveredAt = o.DeliveredAt
	}
	if o.CompletedAt != nil {
		ord.CompletedAt = o.CompletedAt
	}
	if o.CancelledAt != nil {
		ord.CancelledAt = o.CancelledAt
	}
	if o.Rating != nil {
		ord.Rating = o.Rating
	}
	if o.RatingComment != "" {
		ord.RatingComment = o.RatingComment
	}
	if o.RatedAt != nil {
		ord.RatedAt = o.RatedAt
	}

	return ord
}

func (r *EntRepository) GetAnalyticsSummary(ctx context.Context, tenantID uuid.UUID, dateFrom, dateTo time.Time) (*AnalyticsSummary, error) {
	// 1. Fetch total counts and revenue
	orders, err := r.client.Order.Query().
		Where(
			order.TenantID(tenantID),
			order.CreatedAtGTE(dateFrom),
			order.CreatedAtLTE(dateTo),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	summary := &AnalyticsSummary{
		OrdersByStatus:    make(map[string]int),
		RevenueByCurrency: make(map[string]float64),
		Trend:             make([]DailyMetric, 0),
	}

	summary.TotalOrders = len(orders)
	for _, o := range orders {
		summary.TotalRevenue += o.GrandTotal
		summary.OrdersByStatus[string(o.Status)]++
		summary.RevenueByCurrency[o.Currency] += o.GrandTotal
	}

	// 2. Trend (group by day)
	trendMap := make(map[string]*DailyMetric)
	for d := dateFrom; !d.After(dateTo); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")
		trendMap[dateStr] = &DailyMetric{Date: dateStr}
	}

	for _, o := range orders {
		dateStr := o.CreatedAt.Format("2006-01-02")
		if m, ok := trendMap[dateStr]; ok {
			m.Orders++
			m.Revenue += o.GrandTotal
		}
	}

	for d := dateFrom; !d.After(dateTo); d = d.AddDate(0, 0, 1) {
		summary.Trend = append(summary.Trend, *trendMap[d.Format("2006-01-02")])
	}

	// 3. Top Selling Items
	var itemSales []struct {
		MenuItemID   uuid.UUID `json:"menu_item_id"`
		NameSnapshot string    `json:"name_snapshot"`
		Quantity     int       `json:"quantity"`
		Revenue      float64   `json:"revenue"`
	}

	// Ent raw query for aggregation
	err = r.client.OrderItem.Query().
		Where(
			orderitem.HasOrderWith(
				order.TenantID(tenantID),
				order.CreatedAtGTE(dateFrom),
				order.CreatedAtLTE(dateTo),
				order.StatusNotIn(order.StatusCancelled),
			),
		).
		GroupBy(orderitem.FieldMenuItemID, orderitem.FieldNameSnapshot).
		Aggregate(ent.Sum(orderitem.FieldQuantity), ent.Sum(orderitem.FieldTotalPrice)).
		Scan(ctx, &itemSales)

	if err == nil {
		for _, s := range itemSales {
			summary.TopSellingItems = append(summary.TopSellingItems, ItemSalesSummary{
				MenuItemID:   s.MenuItemID,
				NameSnapshot: s.NameSnapshot,
				Quantity:     s.Quantity,
				Revenue:      s.Revenue,
			})
		}
	}

	return summary, nil
}

func entOrderItemToDomain(item *ent.OrderItem) *OrderItem {
	oi := &OrderItem{
		ID:           item.ID,
		OrderID:      item.OrderID,
		MenuItemID:   item.MenuItemID,
		NameSnapshot: item.NameSnapshot,
		Quantity:     item.Quantity,
		UnitPrice:    item.UnitPrice,
		TotalPrice:   item.TotalPrice,
		Notes:        item.Notes,
		Metadata:     item.Metadata,
	}

	if item.VariantID != nil {
		oi.VariantID = item.VariantID
	}

	return oi
}

func entOrderEventToDomain(e *ent.OrderEvent) *OrderEvent {
	event := &OrderEvent{
		ID:         e.ID,
		OrderID:    e.OrderID,
		EventType:  e.EventType,
		FromStatus: e.FromStatus,
		ToStatus:   e.ToStatus,
		Payload:    e.Payload,
		ActorType:  e.ActorType,
		IPAddress:  e.IPAddress,
		OccurredAt: e.OccurredAt,
	}

	if e.ActorUserID != nil {
		event.ActorUserID = e.ActorUserID
	}

	return event
}

// --- Cross-module Lookups ---

func (r *EntRepository) GetTenantByID(ctx context.Context, id uuid.UUID) (*Tenant, error) {
	t, err := r.client.Tenant.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &Tenant{
		ID:   t.ID,
		Slug: t.Slug,
		Name: t.Name,
	}, nil
}

func (r *EntRepository) GetMenuItemByID(ctx context.Context, tenantID, id uuid.UUID) (*catalog.MenuItem, error) {
	mi, err := r.client.MenuItem.Query().
		Where(
			menuitem.ID(id),
			menuitem.TenantID(tenantID),
		).
		Only(ctx)
	if err != nil {
		return nil, err
	}
	return entMenuItemToDomain(mi), nil
}

func entMenuItemToDomain(mi *ent.MenuItem) *catalog.MenuItem {
	cm := &catalog.MenuItem{
		ID:              mi.ID,
		TenantID:        mi.TenantID,
		Name:            mi.Name,
		Description:     mi.Description,
		BasePrice:       mi.BasePrice,
		Currency:        mi.Currency,
		IsAvailable:     mi.IsAvailable,
		SKU:             mi.Sku,
		DisplayOrder:    mi.DisplayOrder,
		CreatedAt:       mi.CreatedAt,
		UpdatedAt:       mi.UpdatedAt,
	}

	if mi.LeadTimeMinutes != nil {
		cm.LeadTimeMinutes = *mi.LeadTimeMinutes
	}

	return cm
}
