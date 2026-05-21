package fulfilment

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
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
	repo            Repository
	orderingSvc     *ordering.OrderService
	orderingRepo    ordering.Repository
	eventPublisher  *events.Publisher
	treasuryClient  *treasury.Client
	logger          *zap.Logger
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

// LogisticsTaskEvent represents a logistics task event payload (shared-events envelope).
type LogisticsTaskEvent struct {
	ID        string                 `json:"id"`
	EventType string                 `json:"event_type"`
	TenantID  string                 `json:"tenant_id"`
	Data      map[string]interface{} `json:"payload"`
	Timestamp string                 `json:"timestamp"`
}

// SubscribeToLogisticsEvents subscribes to logistics task events via NATS.
func (h *LogisticsEventHandler) SubscribeToLogisticsEvents(nc *nats.Conn) error {
	if nc == nil {
		h.logger.Warn("NATS connection not available, skipping logistics event subscriptions")
		return nil
	}

	// logistics.task.completed -> auto-complete order
	_, err := nc.Subscribe("logistics.task.completed", func(msg *nats.Msg) {
		var evt LogisticsTaskEvent
		if err := json.Unmarshal(msg.Data, &evt); err != nil {
			h.logger.Error("failed to unmarshal logistics.task.completed event", zap.Error(err))
			return
		}
		ctx := context.Background()
		if err := h.handleTaskCompleted(ctx, &evt); err != nil {
			h.logger.Error("failed to handle logistics.task.completed event", zap.Error(err))
			return
		}
		_ = msg.Ack()
	})
	if err != nil {
		return fmt.Errorf("subscribe to logistics.task.completed: %w", err)
	}

	// logistics.task.assigned -> update order assignment and status
	_, err = nc.Subscribe("logistics.task.assigned", func(msg *nats.Msg) {
		var evt LogisticsTaskEvent
		if err := json.Unmarshal(msg.Data, &evt); err != nil {
			h.logger.Error("failed to unmarshal logistics.task.assigned event", zap.Error(err))
			return
		}
		ctx := context.Background()
		if err := h.handleTaskAssigned(ctx, &evt); err != nil {
			h.logger.Error("failed to handle logistics.task.assigned event", zap.Error(err))
			return
		}
		_ = msg.Ack()
	})
	if err != nil {
		return fmt.Errorf("subscribe to logistics.task.assigned: %w", err)
	}

	// logistics.task.en_route -> update assignment status for tracking
	_, err = nc.Subscribe("logistics.task.en_route", func(msg *nats.Msg) {
		var evt LogisticsTaskEvent
		if err := json.Unmarshal(msg.Data, &evt); err != nil {
			h.logger.Error("failed to unmarshal logistics.task.en_route event", zap.Error(err))
			return
		}
		ctx := context.Background()
		if err := h.handleTaskStatusUpdate(ctx, &evt, "en_route"); err != nil {
			h.logger.Error("failed to handle logistics.task.en_route event", zap.Error(err))
		}
		_ = msg.Ack()
	})
	if err != nil {
		return fmt.Errorf("subscribe to logistics.task.en_route: %w", err)
	}

	// logistics.task.failed -> handle delivery failures
	_, err = nc.Subscribe("logistics.task.failed", func(msg *nats.Msg) {
		var evt LogisticsTaskEvent
		if err := json.Unmarshal(msg.Data, &evt); err != nil {
			h.logger.Error("failed to unmarshal logistics.task.failed event", zap.Error(err))
			return
		}
		ctx := context.Background()
		if err := h.handleTaskFailed(ctx, &evt); err != nil {
			h.logger.Error("failed to handle logistics.task.failed event", zap.Error(err))
		}
		_ = msg.Ack()
	})
	if err != nil {
		return fmt.Errorf("subscribe to logistics.task.failed: %w", err)
	}

	h.logger.Info("logistics event subscriptions active",
		zap.Strings("subjects", []string{
			"logistics.task.completed", "logistics.task.assigned",
			"logistics.task.en_route", "logistics.task.failed",
		}))
	return nil
}

// handleTaskCompleted auto-completes an order when a logistics task is completed.
func (h *LogisticsEventHandler) handleTaskCompleted(ctx context.Context, evt *LogisticsTaskEvent) error {
	data := evt.Data

	// external_reference is the order_id
	orderIDStr, _ := data["external_reference"].(string)
	if orderIDStr == "" {
		// Fall back to order_id field
		orderIDStr, _ = data["order_id"].(string)
	}
	if orderIDStr == "" {
		return fmt.Errorf("no external_reference or order_id in task.completed event")
	}

	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		return fmt.Errorf("invalid order_id %q: %w", orderIDStr, err)
	}

	tenantID, err := uuid.Parse(evt.TenantID)
	if err != nil {
		return fmt.Errorf("invalid tenant_id: %w", err)
	}

	// Get the order
	order, err := h.orderingRepo.GetOrder(ctx, tenantID, orderID)
	if err != nil {
		return fmt.Errorf("get order %s: %w", orderID, err)
	}

	// Only transition if order is currently out_for_delivery
	if order.Status != ordering.OrderStatusOutForDelivery {
		h.logger.Info("order not in out_for_delivery status, skipping auto-complete",
			zap.String("order_id", orderID.String()),
			zap.String("current_status", string(order.Status)))
		return nil
	}

	// Transition order to delivered
	_, err = h.orderingSvc.UpdateOrderStatus(
		ctx, tenantID, orderID,
		ordering.OrderStatusDelivered,
		nil, "system", "",
	)
	if err != nil {
		return fmt.Errorf("transition order to delivered: %w", err)
	}

	// Handle COD payment: if cash was collected, update payment status to paid
	// and settle the payment intent in treasury so the transaction is recorded.
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

		// Settle the treasury payment intent so it moves from pending → succeeded
		// and a PaymentTransaction record is created for accounting.
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

	// Update assignment status
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

	// Publish ordering.order.delivered event
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
func (h *LogisticsEventHandler) handleTaskAssigned(ctx context.Context, evt *LogisticsTaskEvent) error {
	data := evt.Data

	// external_reference is the order_id
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

	tenantID, err := uuid.Parse(evt.TenantID)
	if err != nil {
		return fmt.Errorf("invalid tenant_id: %w", err)
	}

	fleetMemberID, _ := data["fleet_member_id"].(string)
	taskIDStr, _ := data["task_id"].(string)

	// Update or create order assignment
	if taskIDStr != "" {
		assignment, assignErr := h.repo.GetAssignmentByLogisticsTaskID(ctx, taskIDStr)
		if assignErr == nil && assignment != nil {
			// Update existing assignment
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
			// Create new assignment
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

	// If order is currently "ready", transition to "out_for_delivery"
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
func (h *LogisticsEventHandler) handleTaskStatusUpdate(ctx context.Context, evt *LogisticsTaskEvent, status string) error {
	data := evt.Data
	taskIDStr, _ := data["task_id"].(string)
	if taskIDStr == "" {
		return nil
	}

	assignment, err := h.repo.GetAssignmentByLogisticsTaskID(ctx, taskIDStr)
	if err != nil {
		return nil // No assignment for this task
	}

	assignment.Status = AssignmentStatus(status)
	if updateErr := h.repo.UpdateAssignment(ctx, assignment); updateErr != nil {
		h.logger.Error("failed to update assignment status",
			zap.Error(updateErr),
			zap.String("task_id", taskIDStr),
			zap.String("status", status))
	}

	return nil
}

// handleTaskFailed handles delivery failure events from logistics.
func (h *LogisticsEventHandler) handleTaskFailed(ctx context.Context, evt *LogisticsTaskEvent) error {
	data := evt.Data

	orderIDStr, _ := data["external_reference"].(string)
	if orderIDStr == "" {
		orderIDStr, _ = data["order_id"].(string)
	}
	if orderIDStr == "" {
		return nil
	}

	tenantID, err := uuid.Parse(evt.TenantID)
	if err != nil {
		return fmt.Errorf("invalid tenant_id: %w", err)
	}

	taskIDStr, _ := data["task_id"].(string)
	failureReason, _ := data["failure_reason"].(string)

	// Update assignment status to failed
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

	// Publish delivery failure event for notifications (do NOT auto-cancel order)
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
