package fulfilment

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/modules/ordering"
)

// EventHandler handles events for automatic task creation.
type EventHandler struct {
	taskSvc      *TaskService
	orderingSvc  *ordering.OrderService
	orderingRepo ordering.Repository
	logger       *zap.Logger
}

// NewEventHandler creates a new fulfilment event handler.
func NewEventHandler(
	taskSvc *TaskService,
	orderingSvc *ordering.OrderService,
	orderingRepo ordering.Repository,
	logger *zap.Logger,
) *EventHandler {
	return &EventHandler{
		taskSvc:      taskSvc,
		orderingSvc:  orderingSvc,
		orderingRepo: orderingRepo,
		logger:       logger.Named("fulfilment.EventHandler"),
	}
}

// OrderReadyEvent represents the cafe.order.ready event payload.
type OrderReadyEvent struct {
	ID        string                 `json:"id"`
	EventType string                 `json:"event_type"`
	TenantID  string                 `json:"tenant_id"`
	Timestamp string                 `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

// SubscribeToOrderEvents subscribes to order events via NATS for automatic task creation.
func (h *EventHandler) SubscribeToOrderEvents(nc *nats.Conn) error {
	if nc == nil {
		h.logger.Warn("NATS connection not available, skipping order event subscriptions")
		return nil
	}

	// Subscribe to cafe.order.ready events
	_, err := nc.Subscribe("cafe.order.ready", func(msg *nats.Msg) {
		var event OrderReadyEvent
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			h.logger.Error("Failed to unmarshal cafe.order.ready event", zap.Error(err))
			return
		}

		ctx := context.Background()
		if err := h.handleOrderReady(ctx, &event); err != nil {
			h.logger.Error("Failed to handle cafe.order.ready event",
				zap.Error(err),
				zap.String("event_id", event.ID))
			// Don't ack the message so it can be retried
			return
		}

		// Ack the message
		_ = msg.Ack()
	})

	if err != nil {
		return fmt.Errorf("fulfilment: subscribe to cafe.order.ready: %w", err)
	}

	h.logger.Info("Subscribed to order events for automatic task creation",
		zap.Strings("events", []string{"cafe.order.ready"}))

	return nil
}

// handleOrderReady handles the order ready event and creates a delivery task.
func (h *EventHandler) handleOrderReady(ctx context.Context, event *OrderReadyEvent) error {
	// Parse tenant ID
	tenantID, err := uuid.Parse(event.TenantID)
	if err != nil {
		return fmt.Errorf("invalid tenant_id in event: %w", err)
	}

	// Extract order ID from event data
	orderIDStr, ok := event.Data["order_id"].(string)
	if !ok {
		return fmt.Errorf("order_id not found in event data")
	}

	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		return fmt.Errorf("invalid order_id in event: %w", err)
	}

	h.logger.Info("Processing order ready event for automatic task creation",
		zap.String("order_id", orderID.String()),
		zap.String("tenant_id", tenantID.String()))

	// Fetch order details
	order, err := h.orderingRepo.GetOrder(ctx, tenantID, orderID)
	if err != nil {
		return fmt.Errorf("failed to get order: %w", err)
	}

	// Check if order has a delivery address (pickup orders don't need delivery tasks)
	if order.DeliveryAddressID == nil {
		h.logger.Info("Order is not for delivery, skipping task creation",
			zap.String("order_id", orderID.String()))
		return nil
	}

	// Fetch delivery address
	deliveryAddr, err := h.orderingRepo.GetAddress(ctx, tenantID, *order.DeliveryAddressID)
	if err != nil {
		return fmt.Errorf("failed to get delivery address: %w", err)
	}

	// Get tenant slug for logistics API
	tenantSlug := h.getTenantSlugFromEvent(event)

	// Build OrderInfo for task creation
	orderInfo := OrderInfo{
		ID:          orderID,
		TenantID:    tenantID,
		TenantSlug:  tenantSlug,
		OrderNumber: order.OrderNumber,
	}

	// Set customer info
	if deliveryAddr.ContactName != "" {
		orderInfo.CustomerName = deliveryAddr.ContactName
	}
	if deliveryAddr.ContactPhone != "" {
		orderInfo.CustomerPhone = deliveryAddr.ContactPhone
	}

	// Set delivery address
	orderInfo.DeliveryAddress = formatAddress(deliveryAddr)
	if deliveryAddr.Latitude != nil {
		orderInfo.DeliveryLat = *deliveryAddr.Latitude
	}
	if deliveryAddr.Longitude != nil {
		orderInfo.DeliveryLng = *deliveryAddr.Longitude
	}
	orderInfo.DeliveryNotes = deliveryAddr.Instructions

	// Set pickup address from cafe (use event data if available, otherwise use default)
	orderInfo.PickupAddress = h.getPickupAddress(event.Data)
	orderInfo.PickupLat, orderInfo.PickupLng = h.getPickupCoordinates(event.Data)

	// Set item info
	orderInfo.ItemCount = len(order.Items)
	orderInfo.ItemsDescription = h.buildItemsDescription(order.Items)

	// Set instructions
	orderInfo.Instructions = order.Instructions

	// Create delivery task
	taskReq := CreateDeliveryTaskRequest{
		OrderInfo:      orderInfo,
		Priority:       PriorityNormal,
		IdempotencyKey: fmt.Sprintf("order-ready-%s", orderID.String()),
	}

	assignment, err := h.taskSvc.CreateDeliveryTask(ctx, taskReq)
	if err != nil {
		return fmt.Errorf("failed to create delivery task: %w", err)
	}

	h.logger.Info("Automatic delivery task created successfully",
		zap.String("order_id", orderID.String()),
		zap.String("assignment_id", assignment.ID.String()),
		zap.String("logistics_task_id", assignment.LogisticsTaskID))

	return nil
}

// getTenantSlugFromEvent extracts the tenant slug from event data.
func (h *EventHandler) getTenantSlugFromEvent(event *OrderReadyEvent) string {
	// Try to get from event data first
	if slug, ok := event.Data["tenant_slug"].(string); ok && slug != "" {
		return slug
	}

	// Default to tenant ID if slug not available
	return event.TenantID
}

// getPickupAddress gets the pickup address from event data or returns default.
func (h *EventHandler) getPickupAddress(data map[string]interface{}) string {
	// Try to get cafe address from event data
	if cafeInfo, ok := data["cafe_info"].(map[string]interface{}); ok {
		if addr, ok := cafeInfo["address"].(string); ok {
			return addr
		}
	}

	// If pickup address is in metadata
	if pickup, ok := data["pickup_address"].(string); ok {
		return pickup
	}

	// Default placeholder - logistics service will use cafe lookup
	return ""
}

// getPickupCoordinates gets the pickup coordinates from event data.
func (h *EventHandler) getPickupCoordinates(data map[string]interface{}) (float64, float64) {
	// Try to get from cafe info
	if cafeInfo, ok := data["cafe_info"].(map[string]interface{}); ok {
		lat, latOk := cafeInfo["latitude"].(float64)
		lng, lngOk := cafeInfo["longitude"].(float64)
		if latOk && lngOk {
			return lat, lng
		}
	}

	// Try from direct pickup fields
	if lat, ok := data["pickup_latitude"].(float64); ok {
		if lng, ok := data["pickup_longitude"].(float64); ok {
			return lat, lng
		}
	}

	return 0, 0
}

// formatAddress formats a CustomerAddress into a single string.
func formatAddress(addr *ordering.CustomerAddress) string {
	address := addr.AddressLine1
	if addr.AddressLine2 != "" {
		address += ", " + addr.AddressLine2
	}
	if addr.City != "" {
		address += ", " + addr.City
	}
	if addr.County != "" {
		address += ", " + addr.County
	}
	if addr.Country != "" {
		address += ", " + addr.Country
	}
	return address
}

// buildItemsDescription builds a description of order items.
func (h *EventHandler) buildItemsDescription(items []ordering.OrderItem) string {
	if len(items) == 0 {
		return ""
	}

	description := ""
	for i, item := range items {
		if i > 0 {
			description += ", "
		}
		if item.Quantity > 1 {
			description += fmt.Sprintf("%dx %s", item.Quantity, item.NameSnapshot)
		} else {
			description += item.NameSnapshot
		}

		// Limit description length
		if len(description) > 200 {
			description = description[:197] + "..."
			break
		}
	}

	return description
}
