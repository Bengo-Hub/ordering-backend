package fulfilment

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	sharedevents "github.com/Bengo-Hub/shared-events"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/modules/ordering"
	"github.com/bengobox/ordering-backend/internal/platform/events"
	"github.com/bengobox/ordering-backend/internal/platform/treasury"
)

// uuidPtrString returns the string representation of a *uuid.UUID, or uuid.Nil's string for nil.
func uuidPtrString(u *uuid.UUID) string {
	if u != nil {
		return u.String()
	}
	return uuid.Nil.String()
}

// LogisticsEventHandler handles logistics task events to update order/assignment state.
type LogisticsEventHandler struct {
	repo           Repository
	orderingSvc    *ordering.OrderService
	orderingRepo   ordering.Repository
	eventPublisher *events.Publisher
	treasuryClient *treasury.Client
	logger         *zap.Logger
}

// NewLogisticsEventHandler creates a new logistics event handler.
func NewLogisticsEventHandler(
	repo Repository,
	orderingSvc *ordering.OrderService,
	orderingRepo ordering.Repository,
	eventPublisher *events.Publisher,
	logger *zap.Logger,
) *LogisticsEventHandler {
	return &LogisticsEventHandler{
		repo:           repo,
		orderingSvc:    orderingSvc,
		orderingRepo:   orderingRepo,
		eventPublisher: eventPublisher,
		logger:         logger.Named("fulfilment.logistics_events"),
	}
}

// SetTreasuryClient sets the treasury client for COD settlement on delivery.
func (h *LogisticsEventHandler) SetTreasuryClient(client *treasury.Client) {
	h.treasuryClient = client
}

const (
	logisticsStreamName     = "logistics"
	logisticsStreamSubjects = "logistics.>"
	logisticsStreamMaxAge   = 72 * time.Hour
)

// ensureLogisticsStream ensures the "logistics" JetStream stream exists.
func ensureLogisticsStream(js nats.JetStreamContext) error {
	_, err := js.StreamInfo(logisticsStreamName)
	if err == nil {
		return nil
	}
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     logisticsStreamName,
		Subjects: []string{logisticsStreamSubjects},
		MaxAge:   logisticsStreamMaxAge,
		Storage:  nats.FileStorage,
		Replicas: 1,
	})
	if err != nil {
		return fmt.Errorf("create logistics stream: %w", err)
	}
	return nil
}

// subscribeLogisticsDurable sets up a single JetStream durable push subscription for a logistics subject.
func (h *LogisticsEventHandler) subscribeLogisticsDurable(
	js nats.JetStreamContext,
	subject string,
	durable string,
	handler func(context.Context, *sharedevents.Event) error,
) error {
	sub, err := js.Subscribe(subject, func(msg *nats.Msg) {
		evt, parseErr := sharedevents.FromJSON(msg.Data)
		if parseErr != nil {
			h.logger.Error("failed to parse logistics event envelope",
				zap.String("subject", subject),
				zap.Error(parseErr))
			_ = msg.Ack()
			return
		}
		ctx := context.Background()
		if err := handler(ctx, evt); err != nil {
			h.logger.Error("logistics event handler error, will redeliver",
				zap.String("subject", subject),
				zap.Error(err))
			_ = msg.Nak()
			return
		}
		_ = msg.Ack()
	},
		nats.Durable(durable),
		nats.DeliverAll(),
		nats.AckExplicit(),
		nats.AckWait(30*time.Second),
		nats.MaxDeliver(5),
		nats.BindStream(logisticsStreamName),
	)
	if err != nil {
		return fmt.Errorf("subscribe to %s (durable=%s): %w", subject, durable, err)
	}
	_ = sub
	return nil
}

// SubscribeToLogisticsEvents subscribes to logistics task events via JetStream durable consumers.
func (h *LogisticsEventHandler) SubscribeToLogisticsEvents(js nats.JetStreamContext) error {
	if err := ensureLogisticsStream(js); err != nil {
		return fmt.Errorf("fulfilment: ensure logistics stream: %w", err)
	}

	subs := []struct {
		subject string
		durable string
		handler func(context.Context, *sharedevents.Event) error
	}{
		{"logistics.task.completed", "ord-logistics-task-completed", h.handleTaskCompleted},
		{"logistics.task.assigned", "ord-logistics-task-assigned", h.handleTaskAssigned},
		{"logistics.task.en_route", "ord-logistics-task-en-route", func(ctx context.Context, evt *sharedevents.Event) error {
			return h.handleTaskStatusUpdate(ctx, evt, "en_route")
		}},
		{"logistics.task.failed", "ord-logistics-task-failed", h.handleTaskFailed},
	}

	for _, s := range subs {
		if err := h.subscribeLogisticsDurable(js, s.subject, s.durable, s.handler); err != nil {
			return err
		}
	}

	h.logger.Info("logistics event subscriptions active (JetStream)",
		zap.Strings("subjects", []string{
			"logistics.task.completed", "logistics.task.assigned",
			"logistics.task.en_route", "logistics.task.failed",
		}))
	return nil
}

// handleTaskCompleted auto-completes an order when a logistics task is completed.
func (h *LogisticsEventHandler) handleTaskCompleted(ctx context.Context, evt *sharedevents.Event) error {
	data := evt.Payload

	orderIDStr, _ := data["external_reference"].(string)
	if orderIDStr == "" {
		orderIDStr, _ = data["order_id"].(string)
	}
	if orderIDStr == "" {
		return fmt.Errorf("no external_reference or order_id in task.completed event")
	}

	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		return fmt.Errorf("invalid order_id %q: %w", orderIDStr, err)
	}

	tenantID := evt.TenantID
	if tenantID == uuid.Nil {
		return fmt.Errorf("invalid tenant_id in task.completed event")
	}

	order, err := h.orderingRepo.GetOrder(ctx, tenantID, orderID)
	if err != nil {
		return fmt.Errorf("get order %s: %w", orderID, err)
	}

	if order.Status != ordering.OrderStatusOutForDelivery {
		h.logger.Info("order not in out_for_delivery status, skipping auto-complete",
			zap.String("order_id", orderID.String()),
			zap.String("current_status", string(order.Status)))
		return nil
	}

	_, err = h.orderingSvc.UpdateOrderStatus(
		ctx, tenantID, orderID,
		ordering.OrderStatusDelivered,
		nil, "system", "",
	)
	if err != nil {
		return fmt.Errorf("transition order to delivered: %w", err)
	}

	cashCollected, _ := data["cash_collected"].(bool)
	if order.PaymentMethod == ordering.PaymentMethodCOD && cashCollected {
		order.PaymentStatus = ordering.PaymentStatusPaid
		if updateErr := h.orderingRepo.UpdateOrder(ctx, order); updateErr != nil {
			h.logger.Error("failed to update COD payment status",
				zap.Error(updateErr),
				zap.String("order_id", orderID.String()))
		} else {
			h.logger.Info("COD payment marked as paid",
				zap.String("order_id", orderID.String()))
		}

		if h.treasuryClient != nil {
			amountCollected, _ := data["amount_collected"].(float64)
			if amountCollected <= 0 {
				amountCollected = order.GrandTotal
			}
			settleReq := treasury.SettleCODPaymentRequest{
				TenantID:   tenantID,
				OrderID:    orderID.String(),
				AmountPaid: amountCollected,
				Currency:   order.Currency,
			}
			if _, settleErr := h.treasuryClient.SettleCODPayment(ctx, settleReq); settleErr != nil {
				h.logger.Error("failed to settle COD payment intent in treasury",
					zap.Error(settleErr),
					zap.String("order_id", orderID.String()))
			} else {
				h.logger.Info("COD payment intent settled in treasury",
					zap.String("order_id", orderID.String()))
			}
		}
	}

	taskIDStr, _ := data["task_id"].(string)
	if taskIDStr != "" {
		assignment, assignErr := h.repo.GetAssignmentByLogisticsTaskID(ctx, taskIDStr)
		if assignErr == nil {
			now := time.Now()
			assignment.Status = AssignmentStatusCompleted
			assignment.CompletedAt = &now
			_ = h.repo.UpdateAssignment(ctx, assignment)
		}
	}

	deliveredData := map[string]interface{}{
		"order_id":     orderID.String(),
		"order_number": order.OrderNumber,
		"customer_id":  uuidPtrString(order.CustomerID),
		"delivered_at": time.Now().UTC().Format(time.RFC3339),
	}
	if order.PaymentMethod == ordering.PaymentMethodCOD {
		amountCollected, _ := data["amount_collected"].(float64)
		deliveredData["payment_method"] = "cod"
		deliveredData["cash_collected"] = cashCollected
		deliveredData["amount_collected"] = amountCollected
	}
	if h.eventPublisher != nil {
		deliveredEvent := events.NewEvent("ordering.order.delivered", tenantID, deliveredData)
		_ = h.eventPublisher.Publish(ctx, "ordering.order.delivered", deliveredEvent)
	}

	h.logger.Info("order auto-completed from logistics task",
		zap.String("order_id", orderID.String()),
		zap.String("order_number", order.OrderNumber))
	return nil
}

// handleTaskAssigned updates order assignment and transitions order status when a rider is assigned.
func (h *LogisticsEventHandler) handleTaskAssigned(ctx context.Context, evt *sharedevents.Event) error {
	data := evt.Payload

	orderIDStr, _ := data["external_reference"].(string)
	if orderIDStr == "" {
		orderIDStr, _ = data["order_id"].(string)
	}
	if orderIDStr == "" {
		return fmt.Errorf("no external_reference or order_id in task.assigned event")
	}

	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		return fmt.Errorf("invalid order_id %q: %w", orderIDStr, err)
	}

	tenantID := evt.TenantID
	if tenantID == uuid.Nil {
		return fmt.Errorf("invalid tenant_id in task.assigned event")
	}

	fleetMemberID, _ := data["fleet_member_id"].(string)
	taskIDStr, _ := data["task_id"].(string)

	if taskIDStr != "" {
		assignment, assignErr := h.repo.GetAssignmentByLogisticsTaskID(ctx, taskIDStr)
		if assignErr == nil && assignment != nil {
			now := time.Now()
			assignment.RiderID = fleetMemberID
			assignment.Status = AssignmentStatusAssigned
			assignment.AssignedAt = &now
			if updateErr := h.repo.UpdateAssignment(ctx, assignment); updateErr != nil {
				h.logger.Error("failed to update assignment",
					zap.Error(updateErr),
					zap.String("task_id", taskIDStr))
			}
		} else {
			now := time.Now()
			newAssignment := &OrderAssignment{
				TenantID:        tenantID,
				OrderID:         orderID,
				LogisticsTaskID: taskIDStr,
				RiderID:         fleetMemberID,
				Status:          AssignmentStatusAssigned,
				Priority:        PriorityNormal,
				AssignedAt:      &now,
			}
			if createErr := h.repo.CreateAssignment(ctx, newAssignment); createErr != nil {
				h.logger.Error("failed to create assignment from task.assigned event",
					zap.Error(createErr),
					zap.String("task_id", taskIDStr))
			}
		}
	}

	order, err := h.orderingRepo.GetOrder(ctx, tenantID, orderID)
	if err != nil {
		return fmt.Errorf("get order %s: %w", orderID, err)
	}

	if order.Status == ordering.OrderStatusReady {
		_, err = h.orderingSvc.UpdateOrderStatus(
			ctx, tenantID, orderID,
			ordering.OrderStatusOutForDelivery,
			nil, "system", "",
		)
		if err != nil {
			h.logger.Error("failed to transition order to out_for_delivery",
				zap.Error(err),
				zap.String("order_id", orderID.String()))
		} else {
			h.logger.Info("order transitioned to out_for_delivery on rider assignment",
				zap.String("order_id", orderID.String()),
				zap.String("rider_id", fleetMemberID))
		}
	}

	return nil
}

// handleTaskStatusUpdate updates the assignment status when a task transitions (e.g. en_route).
func (h *LogisticsEventHandler) handleTaskStatusUpdate(ctx context.Context, evt *sharedevents.Event, status string) error {
	data := evt.Payload
	taskIDStr, _ := data["task_id"].(string)
	if taskIDStr == "" {
		return nil
	}

	tenantID := evt.TenantID

	assignment, err := h.repo.GetAssignmentByLogisticsTaskID(ctx, taskIDStr)
	if err != nil {
		return nil
	}

	assignment.Status = AssignmentStatus(status)
	if updateErr := h.repo.UpdateAssignment(ctx, assignment); updateErr != nil {
		h.logger.Error("failed to update assignment status",
			zap.Error(updateErr),
			zap.String("task_id", taskIDStr),
			zap.String("status", status))
	}

	if status == "en_route" && tenantID != uuid.Nil {
		order, getErr := h.orderingRepo.GetOrder(ctx, tenantID, assignment.OrderID)
		if getErr == nil && (order.Status == ordering.OrderStatusReady || order.Status == ordering.OrderStatusConfirmed || order.Status == ordering.OrderStatusPreparing) {
			if _, transErr := h.orderingSvc.UpdateOrderStatus(ctx, tenantID, assignment.OrderID, ordering.OrderStatusOutForDelivery, nil, "system", ""); transErr != nil {
				h.logger.Error("failed to transition order to out_for_delivery on en_route",
					zap.Error(transErr),
					zap.String("order_id", assignment.OrderID.String()))
			} else {
				h.logger.Info("order transitioned to out_for_delivery on rider en_route",
					zap.String("order_id", assignment.OrderID.String()))
			}
		}
	}

	return nil
}

// handleTaskFailed handles delivery failure events from logistics.
func (h *LogisticsEventHandler) handleTaskFailed(ctx context.Context, evt *sharedevents.Event) error {
	data := evt.Payload

	orderIDStr, _ := data["external_reference"].(string)
	if orderIDStr == "" {
		orderIDStr, _ = data["order_id"].(string)
	}
	if orderIDStr == "" {
		return nil
	}

	tenantID := evt.TenantID
	if tenantID == uuid.Nil {
		return fmt.Errorf("invalid tenant_id in task.failed event")
	}

	taskIDStr, _ := data["task_id"].(string)
	failureReason, _ := data["failure_reason"].(string)

	if taskIDStr != "" {
		assignment, assignErr := h.repo.GetAssignmentByLogisticsTaskID(ctx, taskIDStr)
		if assignErr == nil {
			now := time.Now()
			assignment.Status = AssignmentStatusFailed
			assignment.FailureReason = failureReason
			assignment.CompletedAt = &now
			_ = h.repo.UpdateAssignment(ctx, assignment)
		}
	}

	if h.eventPublisher != nil {
		failedEvent := events.NewEvent("ordering.order.delivery_failed", tenantID, map[string]interface{}{
			"order_id":       orderIDStr,
			"task_id":        taskIDStr,
			"failure_reason": failureReason,
			"failed_at":      time.Now().UTC().Format(time.RFC3339),
			"notification": map[string]interface{}{
				"target": "admin",
			},
		})
		_ = h.eventPublisher.Publish(ctx, "ordering.order.delivery_failed", failedEvent)
	}

	h.logger.Warn("delivery task failed",
		zap.String("order_id", orderIDStr),
		zap.String("task_id", taskIDStr),
		zap.String("reason", failureReason))
	return nil
}
