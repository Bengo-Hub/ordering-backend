package fulfilment

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	sharedevents "github.com/Bengo-Hub/shared-events"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/modules/ordering"
)

const (
	orderingStreamName     = "ordering"
	orderingStreamSubjects = "ordering.>"
	orderingStreamMaxAge   = 72 * time.Hour
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

// ensureOrderingStream ensures the "ordering" JetStream stream exists.
// The stream is primarily managed by ordering-backend itself (see platform/events/nats.go),
// but we guard here in case this handler is initialised before that call.
func ensureOrderingStream(js nats.JetStreamContext) error {
	_, err := js.StreamInfo(orderingStreamName)
	if err == nil {
		return nil
	}
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     orderingStreamName,
		Subjects: []string{orderingStreamSubjects},
		MaxAge:   orderingStreamMaxAge,
		Storage:  nats.FileStorage,
		Replicas: 1,
	})
	if err != nil {
		return fmt.Errorf("create ordering stream: %w", err)
	}
	return nil
}

// SubscribeToOrderEvents subscribes to order events via JetStream for automatic task creation.
func (h *EventHandler) SubscribeToOrderEvents(js nats.JetStreamContext) error {
	if err := ensureOrderingStream(js); err != nil {
		return fmt.Errorf("fulfilment: ensure ordering stream: %w", err)
	}

	sharedevents.SubscribeQueueWithRebind(h.logger, js, orderingStreamName, "ordering.order.ready", "ord-fulfilment-order-ready", func(msg *nats.Msg) {
		evt, parseErr := sharedevents.FromJSON(msg.Data)
		if parseErr != nil {
			h.logger.Error("Failed to parse ordering.order.ready envelope", zap.Error(parseErr))
			_ = msg.Ack()
			return
		}

		ctx := context.Background()
		if err := h.handleOrderReady(ctx, evt); err != nil {
			h.logger.Error("Failed to handle ordering.order.ready event",
				zap.Error(err),
				zap.String("event_id", evt.ID.String()))
			_ = msg.Nak()
			return
		}
		_ = msg.Ack()
	},
		nats.Durable("ord-fulfilment-order-ready"),
		nats.DeliverAll(),
		nats.AckExplicit(),
		nats.AckWait(30*time.Second),
		nats.MaxDeliver(5),
		nats.BindStream(orderingStreamName),
	)

	h.logger.Info("Subscribed to order events for automatic task creation (JetStream)",
		zap.Strings("subjects", []string{"ordering.order.ready"}))
	return nil
}

// handleOrderReady handles the order ready event and creates a delivery task.
func (h *EventHandler) handleOrderReady(ctx context.Context, evt *sharedevents.Event) error {
	tenantID := evt.TenantID
	if tenantID == uuid.Nil {
		return fmt.Errorf("invalid tenant_id in event")
	}

	orderIDStr, ok := evt.Payload["order_id"].(string)
	if !ok || orderIDStr == "" {
		return fmt.Errorf("order_id not found in event payload")
	}

	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		return fmt.Errorf("invalid order_id in event: %w", err)
	}

	h.logger.Info("Processing order ready event for automatic task creation",
		zap.String("order_id", orderID.String()),
		zap.String("tenant_id", tenantID.String()))

	order, err := h.orderingRepo.GetOrder(ctx, tenantID, orderID)
	if err != nil {
		return fmt.Errorf("failed to get order: %w", err)
	}

	if order.DeliveryAddressID == nil {
		h.logger.Info("Order is not for delivery, skipping task creation",
			zap.String("order_id", orderID.String()))
		return nil
	}

	deliveryAddr, err := h.orderingRepo.GetAddress(ctx, tenantID, *order.DeliveryAddressID)
	if err != nil {
		return fmt.Errorf("failed to get delivery address: %w", err)
	}

	tenantSlug, _ := evt.Payload["tenant_slug"].(string)
	if tenantSlug == "" {
		tenantSlug = tenantID.String()
	}

	orderInfo := OrderInfo{
		ID:          orderID,
		TenantID:    tenantID,
		TenantSlug:  tenantSlug,
		OrderNumber: order.OrderNumber,
	}

	if deliveryAddr.ContactName != "" {
		orderInfo.CustomerName = deliveryAddr.ContactName
	}
	if deliveryAddr.ContactPhone != "" {
		orderInfo.CustomerPhone = deliveryAddr.ContactPhone
	}

	orderInfo.DeliveryAddress = formatAddress(deliveryAddr)
	if deliveryAddr.Latitude != nil {
		orderInfo.DeliveryLat = *deliveryAddr.Latitude
	}
	if deliveryAddr.Longitude != nil {
		orderInfo.DeliveryLng = *deliveryAddr.Longitude
	}
	orderInfo.DeliveryNotes = deliveryAddr.Instructions

	orderInfo.PickupAddress = getPickupAddress(evt.Payload)
	orderInfo.PickupLat, orderInfo.PickupLng = getPickupCoordinates(evt.Payload)

	orderInfo.ItemCount = len(order.Items)
	orderInfo.ItemsDescription = buildItemsDescription(order.Items)
	orderInfo.Instructions = order.Instructions
	orderInfo.DeliveryFee = order.DeliveryFee

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

// getPickupAddress gets the pickup address from event payload or returns default.
// PublishOrderReady emits the pickup point under `outlet_location` (name/latitude/
// longitude); older/other producers used `cafe_info`/`pickup_address`. Read outlet_location
// first so auto-created delivery tasks actually carry a pickup point.
func getPickupAddress(data map[string]interface{}) string {
	if loc, ok := data["outlet_location"].(map[string]interface{}); ok {
		if addr, ok := loc["address"].(string); ok && addr != "" {
			return addr
		}
		if name, ok := loc["name"].(string); ok && name != "" {
			return name
		}
	}
	if cafeInfo, ok := data["cafe_info"].(map[string]interface{}); ok {
		if addr, ok := cafeInfo["address"].(string); ok {
			return addr
		}
	}
	if pickup, ok := data["pickup_address"].(string); ok {
		return pickup
	}
	return ""
}

// getPickupCoordinates gets the pickup coordinates from event payload.
func getPickupCoordinates(data map[string]interface{}) (float64, float64) {
	if loc, ok := data["outlet_location"].(map[string]interface{}); ok {
		lat, latOk := toFloat(loc["latitude"])
		lng, lngOk := toFloat(loc["longitude"])
		if latOk && lngOk {
			return lat, lng
		}
	}
	if cafeInfo, ok := data["cafe_info"].(map[string]interface{}); ok {
		lat, latOk := toFloat(cafeInfo["latitude"])
		lng, lngOk := toFloat(cafeInfo["longitude"])
		if latOk && lngOk {
			return lat, lng
		}
	}
	lat, latOk := toFloat(data["pickup_latitude"])
	lng, lngOk := toFloat(data["pickup_longitude"])
	if latOk && lngOk {
		return lat, lng
	}
	return 0, 0
}

// toFloat coerces a JSON number (float64) or numeric string into a float64.
func toFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case string:
		if f, err := strconv.ParseFloat(n, 64); err == nil {
			return f, true
		}
	}
	return 0, false
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
func buildItemsDescription(items []ordering.OrderItem) string {
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
		if len(description) > 200 {
			description = description[:197] + "..."
			break
		}
	}
	return description
}
