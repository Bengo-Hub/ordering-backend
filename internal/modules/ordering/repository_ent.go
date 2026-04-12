package ordering

import (
	"context"
	"fmt"
	"time"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/cart"
	"github.com/bengobox/ordering-backend/internal/ent/cartitem"
	"github.com/bengobox/ordering-backend/internal/ent/deliveryzone"
	"github.com/bengobox/ordering-backend/internal/ent/order"
	"github.com/bengobox/ordering-backend/internal/ent/outletrating"
	"github.com/bengobox/ordering-backend/internal/ent/orderevent"
	"github.com/bengobox/ordering-backend/internal/ent/orderitem"
	tenantpredicate "github.com/bengobox/ordering-backend/internal/ent/tenant"
	"github.com/google/uuid"
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
		SetOutletID(c.OutletID).
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

func (r *EntRepository) GetActiveCartByUser(ctx context.Context, tenantID, outletID, userID uuid.UUID) (*Cart, error) {
	c, err := r.client.Cart.Query().
		Where(
			cart.TenantID(tenantID),
			cart.OutletID(outletID),
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

func (r *EntRepository) GetActiveCartBySession(ctx context.Context, tenantID, outletID uuid.UUID, sessionID string) (*Cart, error) {
	c, err := r.client.Cart.Query().
		Where(
			cart.TenantID(tenantID),
			cart.OutletID(outletID),
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

	if filter.OutletID != nil {
		query = query.Where(cart.OutletID(*filter.OutletID))
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

func (r *EntRepository) ListDistinctCartTenantIDs(ctx context.Context) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := r.client.Cart.Query().
		Where(cart.StatusEQ(cart.StatusActive)).
		GroupBy(cart.FieldTenantID).
		Scan(ctx, &ids)
	if err != nil {
		return nil, fmt.Errorf("list distinct cart tenant IDs: %w", err)
	}
	return ids, nil
}

// --- CartItem Methods ---

func (r *EntRepository) CreateCartItem(ctx context.Context, item *CartItem) error {
	builder := r.client.CartItem.Create().
		SetCartID(item.CartID).
		SetInventorySku(item.InventorySKU).
		SetNameSnapshot(item.NameSnapshot).
		SetQuantity(item.Quantity).
		SetUnitPrice(item.UnitPrice).
		SetTotalPrice(item.TotalPrice).
		SetNotes(item.Notes)

	if item.VariantID != nil {
		builder.SetVariantID(*item.VariantID)
	}

	// Persist modifiers and modifier_total in metadata
	meta := item.Metadata
	if meta == nil {
		meta = make(map[string]interface{})
	}
	if len(item.Modifiers) > 0 {
		meta["modifiers"] = item.Modifiers
		meta["modifier_total"] = item.ModifierTotal
	}
	if len(meta) > 0 {
		builder.SetMetadata(meta)
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

func (r *EntRepository) GetCartItemBySKU(ctx context.Context, cartID uuid.UUID, sku string, variantID *uuid.UUID) (*CartItem, error) {
	query := r.client.CartItem.Query().
		Where(
			cartitem.CartID(cartID),
			cartitem.InventorySku(sku),
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
		SetOutletID(o.OutletID).
		SetNillableCustomerID(o.CustomerID).
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
	if o.FulfillmentType != "" {
		builder.SetFulfillmentType(order.FulfillmentType(o.FulfillmentType))
	}
	if o.ScheduledFor != nil {
		builder.SetScheduledFor(*o.ScheduledFor)
	}
	builder.SetPackagingFee(o.PackagingFee)
	builder.SetServiceFee(o.ServiceFee)
	builder.SetSmallOrderFee(o.SmallOrderFee)
	if o.ReservationID != nil {
		builder.SetReservationID(*o.ReservationID)
	}
	if o.PaymentMethod != "" {
		builder.SetPaymentMethod(order.PaymentMethod(o.PaymentMethod))
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

	if filter.OutletID != nil {
		query = query.Where(order.OutletID(*filter.OutletID))
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

	orders, err := query.WithItems().All(ctx)
	if err != nil {
		return nil, 0, err
	}

	result := make([]Order, len(orders))
	for i, o := range orders {
		domainOrder := entOrderToDomain(o)
		// Map eager-loaded order items
		if edges := o.Edges.Items; len(edges) > 0 {
			domainOrder.Items = make([]OrderItem, len(edges))
			for j, item := range edges {
				domainOrder.Items[j] = *entOrderItemToDomain(item)
			}
		}
		result[i] = *domainOrder
	}
	return result, total, nil
}


// --- Scheduled Orders ---

func (r *EntRepository) ListScheduledOrdersDue(ctx context.Context, prepBuffer time.Duration) ([]Order, error) {
	cutoff := time.Now().Add(prepBuffer)
	ents, err := r.client.Order.Query().
		Where(
			order.FulfillmentTypeEQ(order.FulfillmentTypeScheduled),
			order.StatusEQ(order.StatusConfirmed),
			order.ScheduledForNotNil(),
			order.ScheduledForLTE(cutoff),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]Order, 0, len(ents))
	for _, o := range ents {
		result = append(result, *entOrderToDomain(o))
	}
	return result, nil
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
		CancelledOrders:   0,
		OrdersByStatus:    make(map[string]int),
		RevenueByCurrency: make(map[string]float64),
		TopSellingItems:   make([]ItemSalesSummary, 0),
		Trend:             make([]DailyMetric, 0),
	}

	trendMap := make(map[string]*DailyMetric)
	// Pre-fill trend map with all dates in range
	for d := dateFrom; !d.After(dateTo); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")
		trendMap[dateStr] = &DailyMetric{Date: dateStr}
	}

	for _, o := range orders {
		summary.TotalOrders++
		summary.TotalRevenue += o.GrandTotal
		summary.OrdersByStatus[string(o.Status)]++
		summary.RevenueByCurrency[o.Currency] += o.GrandTotal
		if o.Status == order.StatusCancelled {
			summary.CancelledOrders++
		}

		dateKey := o.CreatedAt.Format("2006-01-02")
		if m, ok := trendMap[dateKey]; ok {
			m.Orders++
			m.Revenue += o.GrandTotal
		}
	}

	// Simple aggregation for items
	orderIDs := make([]uuid.UUID, 0)
	for _, o := range orders {
		if o.Status != order.StatusCancelled {
			orderIDs = append(orderIDs, o.ID)
		}
	}

	if len(orderIDs) > 0 {
		items, err := r.client.OrderItem.Query().
			Where(orderitem.OrderIDIn(orderIDs...)).
			All(ctx)
		if err == nil {
			itemStats := make(map[string]*ItemSalesSummary)
			for _, it := range items {
				if _, ok := itemStats[it.InventorySku]; !ok {
					itemStats[it.InventorySku] = &ItemSalesSummary{
						InventorySKU: it.InventorySku,
						NameSnapshot: it.NameSnapshot,
					}
				}
				itemStats[it.InventorySku].Quantity += it.Quantity
				itemStats[it.InventorySku].Revenue += it.TotalPrice
			}
			for _, stats := range itemStats {
				summary.TopSellingItems = append(summary.TopSellingItems, *stats)
			}
		}
	}

	// Populate trend from map in chronological order
	for d := dateFrom; !d.After(dateTo); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")
		summary.Trend = append(summary.Trend, *trendMap[dateStr])
	}

	return summary, nil
}

func (r *EntRepository) GenerateOrderNumber(ctx context.Context, tenantID, outletID uuid.UUID) (string, error) {
	// Get today's date prefix
	today := time.Now().Format("20060102")

	// Count orders today for this outlet
	count, err := r.client.Order.Query().
		Where(
			order.TenantID(tenantID),
			order.OutletID(outletID),
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
		SetInventorySku(item.InventorySKU).
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
		OutletID:              c.OutletID,
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
		InventorySKU: item.InventorySku,
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

	// Extract modifiers from metadata
	if item.Metadata != nil {
		if modTotal, ok := item.Metadata["modifier_total"]; ok {
			if v, ok := modTotal.(float64); ok {
				ci.ModifierTotal = v
			}
		}
		if rawMods, ok := item.Metadata["modifiers"]; ok {
			if modsSlice, ok := rawMods.([]interface{}); ok {
				for _, raw := range modsSlice {
					if m, ok := raw.(map[string]interface{}); ok {
						mod := CartItemModifier{
							PriceAdjustment: 0,
						}
						if v, ok := m["group_id"].(string); ok {
							mod.GroupID, _ = uuid.Parse(v)
						}
						if v, ok := m["group_name"].(string); ok {
							mod.GroupName = v
						}
						if v, ok := m["option_id"].(string); ok {
							mod.OptionID, _ = uuid.Parse(v)
						}
						if v, ok := m["option_name"].(string); ok {
							mod.OptionName = v
						}
						if v, ok := m["price_adjustment"].(float64); ok {
							mod.PriceAdjustment = v
						}
						ci.Modifiers = append(ci.Modifiers, mod)
					}
				}
			}
		}
	}

	return ci
}

func entOrderToDomain(o *ent.Order) *Order {
	ord := &Order{
		ID:                    o.ID,
		TenantID:              o.TenantID,
		OutletID:              o.OutletID,
		CustomerID:            o.CustomerID,
		OrderNumber:           o.OrderNumber,
		Status:                OrderStatus(o.Status),
		PaymentStatus:         PaymentStatus(o.PaymentStatus),
		PaymentMethod:         PaymentMethod(o.PaymentMethod),
		Currency:              o.Currency,
		Subtotal:              o.Subtotal,
		DiscountTotal:         o.DiscountTotal,
		TaxTotal:              o.TaxTotal,
		DeliveryFee:           o.DeliveryFee,
		FulfillmentType:       FulfillmentType(o.FulfillmentType),
		PackagingFee:          o.PackagingFee,
		ServiceFee:            o.ServiceFee,
		SmallOrderFee:         o.SmallOrderFee,
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
	if o.ScheduledFor != nil {
		ord.ScheduledFor = o.ScheduledFor
	}
	if o.ReservationID != nil {
		ord.ReservationID = o.ReservationID
	}

	return ord
}


func entOrderItemToDomain(item *ent.OrderItem) *OrderItem {
	oi := &OrderItem{
		ID:           item.ID,
		OrderID:      item.OrderID,
		InventorySKU: item.InventorySku,
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

// GetTenantFeatures returns the features JSON map from the TenantSetting for the given tenant.
func (r *EntRepository) GetTenantFeatures(ctx context.Context, tenantID uuid.UUID) (map[string]interface{}, error) {
	t, err := r.client.Tenant.Query().
		Where(tenantpredicate.ID(tenantID)).
		WithSettings().
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("get tenant features: %w", err)
	}
	if t.Edges.Settings == nil {
		return map[string]interface{}{}, nil
	}
	return t.Edges.Settings.Features, nil
}

// ListActiveDeliveryZones returns active delivery zones for a tenant (and optionally a specific outlet).
func (r *EntRepository) ListActiveDeliveryZones(ctx context.Context, tenantID uuid.UUID, outletID *uuid.UUID) ([]DeliveryZone, error) {
	query := r.client.DeliveryZone.Query().
		Where(deliveryzone.TenantID(tenantID), deliveryzone.IsActive(true)).
		Order(ent.Asc(deliveryzone.FieldSortOrder))

	if outletID != nil {
		query = query.Where(deliveryzone.OutletID(*outletID))
	}

	zones, err := query.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active delivery zones: %w", err)
	}

	result := make([]DeliveryZone, len(zones))
	for i, z := range zones {
		result[i] = DeliveryZone{
			ID:                   z.ID,
			TenantID:             z.TenantID,
			OutletID:             z.OutletID,
			Name:                 z.Name,
			Slug:                 z.Slug,
			ZonePolygon:          z.ZonePolygon,
			DeliveryFee:          z.DeliveryFee,
			MinimumOrder:         z.MinimumOrder,
			EstimatedTimeMinutes: z.EstimatedTimeMinutes,
			IsActive:             z.IsActive,
			SortOrder:            z.SortOrder,
		}
	}

	return result, nil
}

// --- OutletRating Methods ---

func (r *EntRepository) GetOutletRating(ctx context.Context, tenantID, outletID uuid.UUID) (*OutletRatingData, error) {
	rating, err := r.client.OutletRating.Query().
		Where(
			outletrating.TenantID(tenantID),
			outletrating.OutletID(outletID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrOutletRatingNotFound
		}
		return nil, err
	}
	return entOutletRatingToDomain(rating), nil
}

func (r *EntRepository) UpsertOutletRating(ctx context.Context, rating *OutletRatingData) error {
	err := r.client.OutletRating.Create().
		SetTenantID(rating.TenantID).
		SetOutletID(rating.OutletID).
		SetAverageRating(rating.AverageRating).
		SetTotalRatings(rating.TotalRatings).
		SetTotalReviews(rating.TotalReviews).
		SetFiveStar(rating.FiveStar).
		SetFourStar(rating.FourStar).
		SetThreeStar(rating.ThreeStar).
		SetTwoStar(rating.TwoStar).
		SetOneStar(rating.OneStar).
		OnConflictColumns("outlet_id").
		UpdateAverageRating().
		UpdateTotalRatings().
		UpdateTotalReviews().
		UpdateFiveStar().
		UpdateFourStar().
		UpdateThreeStar().
		UpdateTwoStar().
		UpdateOneStar().
		UpdateUpdatedAt().
		Exec(ctx)
	return err
}

func entOutletRatingToDomain(r *ent.OutletRating) *OutletRatingData {
	return &OutletRatingData{
		ID:            r.ID,
		TenantID:      r.TenantID,
		OutletID:      r.OutletID,
		AverageRating: r.AverageRating,
		TotalRatings:  r.TotalRatings,
		TotalReviews:  r.TotalReviews,
		FiveStar:      r.FiveStar,
		FourStar:      r.FourStar,
		ThreeStar:     r.ThreeStar,
		TwoStar:       r.TwoStar,
		OneStar:       r.OneStar,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
}
