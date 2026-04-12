package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

// Publisher handles publishing events to NATS JetStream.
type Publisher struct {
	js     nats.JetStreamContext
	logger *zap.Logger
}

// NewPublisher creates a new event publisher using JetStream for durability.
func NewPublisher(js nats.JetStreamContext, logger *zap.Logger) *Publisher {
	return &Publisher{
		js:     js,
		logger: logger.Named("events.publisher"),
	}
}

// Event represents a CloudEvents-compatible event envelope.
type Event struct {
	ID              string                 `json:"id"`
	Source          string                 `json:"source"`
	SpecVersion     string                 `json:"specversion"`
	Type            string                 `json:"type"`
	DataContentType string                 `json:"datacontenttype"`
	Time            string                 `json:"time"`
	TenantID        string                 `json:"tenantId,omitempty"`
	TenantSlug      string                 `json:"tenant_slug,omitempty"`
	Data            map[string]interface{} `json:"data"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// NewEvent creates a new event with the given type and data.
func NewEvent(eventType string, tenantID uuid.UUID, data map[string]interface{}) Event {
	return Event{
		ID:              uuid.New().String(),
		Source:          "ordering-service",
		SpecVersion:     "1.0",
		Type:            eventType,
		DataContentType: "application/json",
		Time:            time.Now().UTC().Format(time.RFC3339),
		TenantID:        tenantID.String(),
		Data:            data,
		Metadata: map[string]interface{}{
			"correlation_id": uuid.New().String(),
			"source":         "ordering-service",
		},
	}
}

// NewEventWithSlug creates a new event including tenant_slug.
func NewEventWithSlug(eventType string, tenantID uuid.UUID, tenantSlug string, data map[string]interface{}) Event {
	e := NewEvent(eventType, tenantID, data)
	e.TenantSlug = tenantSlug
	return e
}

// Publish publishes an event to the specified subject via JetStream.
func (p *Publisher) Publish(ctx context.Context, subject string, event Event) error {
	if p.js == nil {
		p.logger.Warn("JetStream not available, skipping event publish",
			zap.String("subject", subject),
			zap.String("event_type", event.Type))
		return nil
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	// Build message with headers for traceability
	headers := make(nats.Header)
	headers.Set("event-id", event.ID)
	headers.Set("event-type", event.Type)
	if event.TenantID != "" {
		headers.Set("tenant-id", event.TenantID)
	}

	msg := nats.NewMsg(subject)
	msg.Data = data
	msg.Header = headers

	if _, err := p.js.PublishMsg(msg); err != nil {
		p.logger.Error("failed to publish event to JetStream",
			zap.Error(err),
			zap.String("subject", subject),
			zap.String("event_id", event.ID))
		return fmt.Errorf("publish event: %w", err)
	}

	p.logger.Debug("event published to JetStream",
		zap.String("subject", subject),
		zap.String("event_type", event.Type),
		zap.String("event_id", event.ID))

	return nil
}

// --- Order Events ---

// OrderCreatedData represents data for order.created event.
type OrderCreatedData struct {
	OrderID       uuid.UUID `json:"order_id"`
	OrderNumber   string    `json:"order_number"`
	CustomerID    uuid.UUID `json:"customer_id"`
	CustomerEmail string    `json:"customer_email,omitempty"`
	CustomerName  string    `json:"customer_name,omitempty"`
	CustomerPhone string    `json:"customer_phone,omitempty"`
	OutletID      uuid.UUID `json:"outlet_id"`
	TotalAmount   float64   `json:"total_amount"`
	Currency      string    `json:"currency"`
	ItemCount     int       `json:"item_count"`
}

// PublishOrderCreated publishes an order.created event.
func (p *Publisher) PublishOrderCreated(ctx context.Context, tenantID uuid.UUID, data OrderCreatedData) error {
	event := NewEvent("ordering.order.created", tenantID, map[string]interface{}{
		"order_id":       data.OrderID.String(),
		"order_number":   data.OrderNumber,
		"customer_id":    data.CustomerID.String(),
		"customer_email": data.CustomerEmail,
		"customer_name":  data.CustomerName,
		"customer_phone": data.CustomerPhone,
		"outlet_id":      data.OutletID.String(),
		"total_amount":   data.TotalAmount,
		"currency":       data.Currency,
		"item_count":     data.ItemCount,
		"notification": map[string]interface{}{
			"target":          "customer",
			"recipient_email": data.CustomerEmail,
			"recipient_phone": data.CustomerPhone,
		},
	})

	return p.Publish(ctx, "ordering.order.created", event)
}

// OrderStatusChangedData represents data for order.status.changed event.
type OrderStatusChangedData struct {
	OrderID        uuid.UUID `json:"order_id"`
	OrderNumber    string    `json:"order_number"`
	CustomerID     uuid.UUID `json:"customer_id"`
	CustomerEmail  string    `json:"customer_email,omitempty"`
	CustomerName   string    `json:"customer_name,omitempty"`
	CustomerPhone  string    `json:"customer_phone,omitempty"`
	PreviousStatus string    `json:"previous_status"`
	NewStatus      string    `json:"new_status"`
	ChangedAt      time.Time `json:"changed_at"`
}

// PublishOrderStatusChanged publishes an order.status.changed event.
func (p *Publisher) PublishOrderStatusChanged(ctx context.Context, tenantID uuid.UUID, data OrderStatusChangedData) error {
	event := NewEvent("ordering.order.status.changed", tenantID, map[string]interface{}{
		"order_id":        data.OrderID.String(),
		"order_number":    data.OrderNumber,
		"customer_id":     data.CustomerID.String(),
		"customer_email":  data.CustomerEmail,
		"customer_name":   data.CustomerName,
		"customer_phone":  data.CustomerPhone,
		"previous_status": data.PreviousStatus,
		"new_status":      data.NewStatus,
		"changed_at":      data.ChangedAt.Format(time.RFC3339),
		"notification": map[string]interface{}{
			"target":          "customer",
			"recipient_email": data.CustomerEmail,
			"recipient_phone": data.CustomerPhone,
		},
	})

	return p.Publish(ctx, "ordering.order.status.changed", event)
}

// OrderReadyData represents data for order.ready event.
type OrderReadyData struct {
	OrderID          uuid.UUID                `json:"order_id"`
	OrderNumber      string                   `json:"order_number"`
	OutletID         uuid.UUID                `json:"outlet_id"`
	CustomerID       uuid.UUID                `json:"customer_id"`
	DeliveryAddress  map[string]interface{}   `json:"delivery_address,omitempty"`
	OutletLocation   map[string]interface{}   `json:"outlet_location,omitempty"`
	Items            []map[string]interface{} `json:"items,omitempty"`
	PaymentMethod    string                   `json:"payment_method,omitempty"`
	CashOnDelivery   float64                  `json:"cash_on_delivery,omitempty"`
	DeliveryFee      float64                  `json:"delivery_fee,omitempty"`
	GrandTotal       float64                  `json:"grand_total,omitempty"`
	CustomerName     string                   `json:"customer_name,omitempty"`
	CustomerEmail    string                   `json:"customer_email,omitempty"`
	CustomerPhone    string                   `json:"customer_phone,omitempty"`
	Instructions     string                   `json:"instructions,omitempty"`
	FulfillmentType  string                   `json:"fulfillment_type,omitempty"`
}

// PublishOrderReady publishes an order.ready event.
func (p *Publisher) PublishOrderReady(ctx context.Context, tenantID uuid.UUID, data OrderReadyData) error {
	eventData := map[string]interface{}{
		"order_id":         data.OrderID.String(),
		"order_number":     data.OrderNumber,
		"outlet_id":        data.OutletID.String(),
		"customer_id":      data.CustomerID.String(),
		"delivery_address": data.DeliveryAddress,
		"outlet_location":  data.OutletLocation,
		"items":            data.Items,
		"notification": map[string]interface{}{
			"target": "customer",
		},
	}

	if data.PaymentMethod != "" {
		eventData["payment_method"] = data.PaymentMethod
	}
	if data.CashOnDelivery > 0 {
		eventData["cash_on_delivery"] = data.CashOnDelivery
	}
	if data.DeliveryFee > 0 {
		eventData["delivery_fee"] = data.DeliveryFee
	}
	if data.GrandTotal > 0 {
		eventData["grand_total"] = data.GrandTotal
	}
	if data.CustomerName != "" {
		eventData["customer_name"] = data.CustomerName
	}
	if data.CustomerEmail != "" {
		eventData["customer_email"] = data.CustomerEmail
		eventData["notification"] = map[string]interface{}{
			"target":          "customer",
			"recipient_email": data.CustomerEmail,
			"recipient_phone": data.CustomerPhone,
		}
	}
	if data.CustomerPhone != "" {
		eventData["customer_phone"] = data.CustomerPhone
	}
	if data.Instructions != "" {
		eventData["instructions"] = data.Instructions
	}
	if data.FulfillmentType != "" {
		eventData["fulfillment_type"] = data.FulfillmentType
	}

	event := NewEvent("ordering.order.ready", tenantID, eventData)
	return p.Publish(ctx, "ordering.order.ready", event)
}

// OrderCancelledData represents data for order.cancelled event.
type OrderCancelledData struct {
	OrderID       uuid.UUID `json:"order_id"`
	OrderNumber   string    `json:"order_number"`
	CustomerID    uuid.UUID `json:"customer_id"`
	CustomerEmail string    `json:"customer_email,omitempty"`
	CustomerName  string    `json:"customer_name,omitempty"`
	CustomerPhone string    `json:"customer_phone,omitempty"`
	Reason        string    `json:"reason"`
	CancelledBy   string    `json:"cancelled_by"`
	CancelledAt   time.Time `json:"cancelled_at"`
}

// PublishOrderCancelled publishes an order.cancelled event.
func (p *Publisher) PublishOrderCancelled(ctx context.Context, tenantID uuid.UUID, data OrderCancelledData) error {
	event := NewEvent("ordering.order.cancelled", tenantID, map[string]interface{}{
		"order_id":       data.OrderID.String(),
		"order_number":   data.OrderNumber,
		"customer_id":    data.CustomerID.String(),
		"customer_email": data.CustomerEmail,
		"customer_name":  data.CustomerName,
		"customer_phone": data.CustomerPhone,
		"reason":         data.Reason,
		"cancelled_by":   data.CancelledBy,
		"cancelled_at":   data.CancelledAt.Format(time.RFC3339),
		"notification": map[string]interface{}{
			"target":          "customer",
			"recipient_email": data.CustomerEmail,
			"recipient_phone": data.CustomerPhone,
		},
	})

	return p.Publish(ctx, "ordering.order.cancelled", event)
}

// OrderCompletedData represents data for order.completed event.
type OrderCompletedData struct {
	OrderID       uuid.UUID `json:"order_id"`
	OrderNumber   string    `json:"order_number"`
	CustomerID    uuid.UUID `json:"customer_id"`
	CustomerEmail string    `json:"customer_email,omitempty"`
	CustomerName  string    `json:"customer_name,omitempty"`
	CustomerPhone string    `json:"customer_phone,omitempty"`
	TotalAmount   float64   `json:"total_amount"`
	Currency      string    `json:"currency"`
	CompletedAt   time.Time `json:"completed_at"`
}

// PublishOrderCompleted publishes an order.completed event.
func (p *Publisher) PublishOrderCompleted(ctx context.Context, tenantID uuid.UUID, data OrderCompletedData) error {
	event := NewEvent("ordering.order.completed", tenantID, map[string]interface{}{
		"order_id":       data.OrderID.String(),
		"order_number":   data.OrderNumber,
		"customer_id":    data.CustomerID.String(),
		"customer_email": data.CustomerEmail,
		"customer_name":  data.CustomerName,
		"customer_phone": data.CustomerPhone,
		"total_amount":   data.TotalAmount,
		"currency":       data.Currency,
		"completed_at":   data.CompletedAt.Format(time.RFC3339),
		"notification": map[string]interface{}{
			"target":          "customer",
			"recipient_email": data.CustomerEmail,
			"recipient_phone": data.CustomerPhone,
		},
	})

	return p.Publish(ctx, "ordering.order.completed", event)
}

// OrderRefundedData represents data for order.refunded event.
type OrderRefundedData struct {
	OrderID       uuid.UUID `json:"order_id"`
	OrderNumber   string    `json:"order_number"`
	CustomerID    uuid.UUID `json:"customer_id"`
	CustomerEmail string    `json:"customer_email,omitempty"`
	CustomerName  string    `json:"customer_name,omitempty"`
	CustomerPhone string    `json:"customer_phone,omitempty"`
	TotalAmount   float64   `json:"total_amount"`
	Currency      string    `json:"currency"`
	Reason        string    `json:"reason"`
	RefundedAt    time.Time `json:"refunded_at"`
}

// PublishOrderRefunded publishes an order.refunded event.
func (p *Publisher) PublishOrderRefunded(ctx context.Context, tenantID uuid.UUID, data OrderRefundedData) error {
	event := NewEvent("ordering.order.refunded", tenantID, map[string]interface{}{
		"order_id":       data.OrderID.String(),
		"order_number":   data.OrderNumber,
		"customer_id":    data.CustomerID.String(),
		"customer_email": data.CustomerEmail,
		"customer_name":  data.CustomerName,
		"customer_phone": data.CustomerPhone,
		"total_amount":   data.TotalAmount,
		"currency":       data.Currency,
		"reason":         data.Reason,
		"refunded_at":    data.RefundedAt.Format(time.RFC3339),
		"notification": map[string]interface{}{
			"target":          "customer",
			"recipient_email": data.CustomerEmail,
			"recipient_phone": data.CustomerPhone,
		},
	})

	return p.Publish(ctx, "ordering.order.refunded", event)
}

// --- Payment Events ---

// PaymentInitiatedData represents data for payment.initiated event.
type PaymentInitiatedData struct {
	PaymentIntentID uuid.UUID `json:"payment_intent_id"`
	OrderID         uuid.UUID `json:"order_id"`
	Amount          float64   `json:"amount"`
	Currency        string    `json:"currency"`
	Provider        string    `json:"provider"`
}

// PublishPaymentInitiated publishes a payment.initiated event.
func (p *Publisher) PublishPaymentInitiated(ctx context.Context, tenantID uuid.UUID, data PaymentInitiatedData) error {
	event := NewEvent("cafe.payment.initiated", tenantID, map[string]interface{}{
		"payment_intent_id": data.PaymentIntentID.String(),
		"order_id":          data.OrderID.String(),
		"amount":            data.Amount,
		"currency":          data.Currency,
		"provider":          data.Provider,
	})

	return p.Publish(ctx, "cafe.payment.initiated", event)
}

// PaymentCompletedData represents data for payment.completed event.
type PaymentCompletedData struct {
	PaymentID         uuid.UUID `json:"payment_id"`
	OrderID           uuid.UUID `json:"order_id"`
	Amount            float64   `json:"amount"`
	Currency          string    `json:"currency"`
	ProviderReference string    `json:"provider_reference"`
}

// PublishPaymentCompleted publishes a payment.completed event.
func (p *Publisher) PublishPaymentCompleted(ctx context.Context, tenantID uuid.UUID, data PaymentCompletedData) error {
	event := NewEvent("cafe.payment.completed", tenantID, map[string]interface{}{
		"payment_id":         data.PaymentID.String(),
		"order_id":           data.OrderID.String(),
		"amount":             data.Amount,
		"currency":           data.Currency,
		"provider_reference": data.ProviderReference,
	})

	return p.Publish(ctx, "cafe.payment.completed", event)
}

// PaymentFailedData represents data for payment.failed event.
type PaymentFailedData struct {
	PaymentIntentID uuid.UUID `json:"payment_intent_id"`
	OrderID         uuid.UUID `json:"order_id"`
	ErrorCode       string    `json:"error_code"`
	ErrorMessage    string    `json:"error_message"`
}

// PublishPaymentFailed publishes a payment.failed event.
func (p *Publisher) PublishPaymentFailed(ctx context.Context, tenantID uuid.UUID, data PaymentFailedData) error {
	event := NewEvent("cafe.payment.failed", tenantID, map[string]interface{}{
		"payment_intent_id": data.PaymentIntentID.String(),
		"order_id":          data.OrderID.String(),
		"error_code":        data.ErrorCode,
		"error_message":     data.ErrorMessage,
	})

	return p.Publish(ctx, "cafe.payment.failed", event)
}

// --- Loyalty Events ---

// LoyaltyPointsAwardedData represents data for loyalty.points_awarded event.
type LoyaltyPointsAwardedData struct {
	UserID      uuid.UUID `json:"user_id"`
	OrderID     uuid.UUID `json:"order_id"`
	Points      int       `json:"points"`
	TotalPoints int       `json:"total_points"`
	Reason      string    `json:"reason"`
}

// PublishLoyaltyPointsAwarded publishes a loyalty.points_awarded event.
func (p *Publisher) PublishLoyaltyPointsAwarded(ctx context.Context, tenantID uuid.UUID, data LoyaltyPointsAwardedData) error {
	event := NewEvent("cafe.loyalty.points_awarded", tenantID, map[string]interface{}{
		"user_id":      data.UserID.String(),
		"order_id":     data.OrderID.String(),
		"points":       data.Points,
		"total_points": data.TotalPoints,
		"reason":       data.Reason,
	})

	return p.Publish(ctx, "cafe.loyalty.points_awarded", event)
}

// --- Scheduled Order Events ---

// OrderScheduledData represents data for order.scheduled event.
type OrderScheduledData struct {
	OrderID       uuid.UUID `json:"order_id"`
	OrderNumber   string    `json:"order_number"`
	CustomerID    uuid.UUID `json:"customer_id"`
	CustomerEmail string    `json:"customer_email,omitempty"`
	CustomerName  string    `json:"customer_name,omitempty"`
	CustomerPhone string    `json:"customer_phone,omitempty"`
	OutletID      uuid.UUID `json:"outlet_id"`
	ScheduledFor  time.Time `json:"scheduled_for"`
	TotalAmount   float64   `json:"total_amount"`
	Currency      string    `json:"currency"`
}

// PublishOrderScheduled publishes an order.scheduled event (for logistics planning).
func (p *Publisher) PublishOrderScheduled(ctx context.Context, tenantID uuid.UUID, data OrderScheduledData) error {
	event := NewEvent("ordering.order.scheduled", tenantID, map[string]interface{}{
		"order_id":       data.OrderID.String(),
		"order_number":   data.OrderNumber,
		"customer_id":    data.CustomerID.String(),
		"customer_email": data.CustomerEmail,
		"customer_name":  data.CustomerName,
		"customer_phone": data.CustomerPhone,
		"outlet_id":      data.OutletID.String(),
		"scheduled_for":  data.ScheduledFor.Format(time.RFC3339),
		"total_amount":   data.TotalAmount,
		"currency":       data.Currency,
		"notification": map[string]interface{}{
			"target":          "customer",
			"recipient_email": data.CustomerEmail,
			"recipient_phone": data.CustomerPhone,
		},
	})

	return p.Publish(ctx, "ordering.order.scheduled", event)
}

// --- Catalog Events (for POS sync) ---

// CatalogUpdatedData represents data for catalog.updated event.
type CatalogUpdatedData struct {
	ChangeType string    `json:"change_type"` // item_added, item_updated, item_removed, category_updated
	EntityType string    `json:"entity_type"` // menu_item, category, modifier_group
	EntityID   uuid.UUID `json:"entity_id"`
	EntityName string    `json:"entity_name,omitempty"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// PublishCatalogUpdated publishes a catalog.updated event (for POS sync).
func (p *Publisher) PublishCatalogUpdated(ctx context.Context, tenantID uuid.UUID, data CatalogUpdatedData) error {
	event := NewEvent("ordering.catalog.updated", tenantID, map[string]interface{}{
		"change_type": data.ChangeType,
		"entity_type": data.EntityType,
		"entity_id":   data.EntityID.String(),
		"entity_name": data.EntityName,
		"updated_at":  data.UpdatedAt.Format(time.RFC3339),
	})

	return p.Publish(ctx, "ordering.catalog.updated", event)
}

// --- Pickup Order Events (for POS handoff) ---

// OrderForPickupData represents data for order.for_pickup event.
type OrderForPickupData struct {
	OrderID       uuid.UUID                `json:"order_id"`
	OrderNumber   string                   `json:"order_number"`
	CustomerID    uuid.UUID                `json:"customer_id"`
	CustomerEmail string                   `json:"customer_email,omitempty"`
	CustomerName  string                   `json:"customer_name"`
	CustomerPhone string                   `json:"customer_phone"`
	OutletID      uuid.UUID                `json:"outlet_id"`
	Items         []map[string]interface{} `json:"items"`
	PickupTime    *time.Time               `json:"pickup_time,omitempty"`
	Notes         string                   `json:"notes,omitempty"`
}

// PublishOrderForPickup publishes an order.for_pickup event (for POS handoff).
func (p *Publisher) PublishOrderForPickup(ctx context.Context, tenantID uuid.UUID, data OrderForPickupData) error {
	eventData := map[string]interface{}{
		"order_id":       data.OrderID.String(),
		"order_number":   data.OrderNumber,
		"customer_id":    data.CustomerID.String(),
		"customer_email": data.CustomerEmail,
		"customer_name":  data.CustomerName,
		"customer_phone": data.CustomerPhone,
		"outlet_id":      data.OutletID.String(),
		"items":          data.Items,
		"notes":          data.Notes,
	}

	if data.PickupTime != nil {
		eventData["pickup_time"] = data.PickupTime.Format(time.RFC3339)
	}

	eventData["notification"] = map[string]interface{}{
		"target":          "customer",
		"recipient_email": data.CustomerEmail,
		"recipient_name":  data.CustomerName,
		"recipient_phone": data.CustomerPhone,
	}

	event := NewEvent("ordering.order.for_pickup", tenantID, eventData)

	return p.Publish(ctx, "ordering.order.for_pickup", event)
}
