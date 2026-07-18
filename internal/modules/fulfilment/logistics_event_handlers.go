package fulfilment

import (
	"context"
	"fmt"
	"strings"
	"time"

	sharedevents "github.com/Bengo-Hub/shared-events"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/modules/ordering"
	"github.com/bengobox/ordering-backend/internal/platform/events"
	"github.com/bengobox/ordering-backend/internal/platform/treasury"
)

// stripRefPrefix removes a "<kind>:" prefix from a logistics task reference.
// Ordering dispatches delivery tasks with external_reference="order:<uuid>", and
// logistics echoes that back verbatim as external_reference/order_id on its task
// events — so the value must have its "order:" prefix stripped before it parses
// as a UUID. A bare UUID (no ":") is returned unchanged.
func stripRefPrefix(s string) string {
	if i := strings.LastIndex(s, ":"); i >= 0 {
		return s[i+1:]
	}
	return s
}

// uuidPtrString returns the string representation of a *uuid.UUID, or uuid.Nil's string for nil.
func uuidPtrString(u *uuid.UUID) string {
	if u != nil {
		return u.String()
	}
	return uuid.Nil.String()
}

// orderRefFromEvent extracts the (orderID, tenantID) referenced by a logistics task event,
// preferring external_reference (the ordering order id) then order_id. Returns ok=false when no
// usable reference / tenant is present.
func (h *LogisticsEventHandler) orderRefFromEvent(evt *sharedevents.Event) (uuid.UUID, uuid.UUID, bool) {
	data := evt.Payload
	orderIDStr, _ := data["external_reference"].(string)
	if orderIDStr == "" {
		orderIDStr, _ = data["order_id"].(string)
	}
	if orderIDStr == "" {
		return uuid.Nil, uuid.Nil, false
	}
	orderID, err := uuid.Parse(stripRefPrefix(orderIDStr))
	if err != nil {
		return uuid.Nil, uuid.Nil, false
	}
	if evt.TenantID == uuid.Nil {
		return uuid.Nil, uuid.Nil, false
	}
	return orderID, evt.TenantID, true
}

// handleTaskAccepted updates the assignment to "accepted" and transitions the order to
// out_for_delivery when a rider accepts the task. Mirrors handleTaskAssigned's order transition so
// orders that go straight from assigned→accepted (skipping a separate en_route signal) still move.
func (h *LogisticsEventHandler) handleTaskAccepted(ctx context.Context, evt *sharedevents.Event) error {
	data := evt.Payload
	taskIDStr, _ := data["task_id"].(string)
	if taskIDStr != "" {
		if assignment, aerr := h.repo.GetAssignmentByLogisticsTaskID(ctx, taskIDStr); aerr == nil && assignment != nil {
			now := time.Now()
			assignment.Status = AssignmentStatusAccepted
			assignment.AcceptedAt = &now
			if fleetMemberID, _ := data["fleet_member_id"].(string); fleetMemberID != "" {
				assignment.RiderID = fleetMemberID
			}
			if uerr := h.repo.UpdateAssignment(ctx, assignment); uerr != nil {
				h.logger.Error("failed to update assignment on task.accepted",
					zap.Error(uerr), zap.String("task_id", taskIDStr))
			}
		}
	}

	orderID, tenantID, ok := h.orderRefFromEvent(evt)
	if !ok {
		return nil
	}
	order, err := h.orderingRepo.GetOrder(ctx, tenantID, orderID)
	if err != nil {
		return fmt.Errorf("get order %s: %w", orderID, err)
	}
	// Only the ready->out_for_delivery edge is valid; if the order already moved on (e.g. en_route
	// already fired) this is a no-op.
	if order.Status == ordering.OrderStatusReady {
		if _, terr := h.orderingSvc.UpdateOrderStatus(ctx, tenantID, orderID,
			ordering.OrderStatusOutForDelivery, nil, "system", ""); terr != nil {
			h.logger.Error("failed to transition order to out_for_delivery on task.accepted",
				zap.Error(terr), zap.String("order_id", orderID.String()))
		} else {
			h.logger.Info("order out_for_delivery on rider accept", zap.String("order_id", orderID.String()))
		}
	}
	return nil
}

// handleTaskDelivered marks the assignment completed and transitions the order to delivered when the
// rider reports delivery (the task.delivered status event, distinct from the PoD-driven
// task.completed). Idempotent via the order-status guard.
func (h *LogisticsEventHandler) handleTaskDelivered(ctx context.Context, evt *sharedevents.Event) error {
	data := evt.Payload
	taskIDStr, _ := data["task_id"].(string)
	if taskIDStr != "" {
		if assignment, aerr := h.repo.GetAssignmentByLogisticsTaskID(ctx, taskIDStr); aerr == nil && assignment != nil {
			now := time.Now()
			assignment.Status = AssignmentStatusCompleted
			assignment.CompletedAt = &now
			if uerr := h.repo.UpdateAssignment(ctx, assignment); uerr != nil {
				h.logger.Error("failed to update assignment on task.delivered",
					zap.Error(uerr), zap.String("task_id", taskIDStr))
			}
		}
	}

	orderID, tenantID, ok := h.orderRefFromEvent(evt)
	if !ok {
		return nil
	}
	order, err := h.orderingRepo.GetOrder(ctx, tenantID, orderID)
	if err != nil {
		return fmt.Errorf("get order %s: %w", orderID, err)
	}
	if order.Status == ordering.OrderStatusOutForDelivery {
		if _, terr := h.orderingSvc.UpdateOrderStatus(ctx, tenantID, orderID,
			ordering.OrderStatusDelivered, nil, "system", ""); terr != nil {
			return fmt.Errorf("transition order to delivered: %w", terr)
		}
		h.logger.Info("order delivered from logistics task.delivered", zap.String("order_id", orderID.String()))
	}
	return nil
}

// handleTaskCompleted auto-completes an order when a logistics task is completed.
func (h *LogisticsEventHandler) handleTaskCompleted(ctx context.Context, evt *sharedevents.Event) error {
	data := evt.Payload
	orderID, tenantID, ok := h.orderRefFromEvent(evt)
	if !ok {
		return fmt.Errorf("no usable order reference / tenant in task.completed event")
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
		deliveredEvent := events.NewEvent("ordering.order.delivered", orderID, tenantID, deliveredData)
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
		// Natural aggregate id: the order id when the reference parses, else a stable
		// tenant-namespaced SHA1 of the raw reference.
		aggID, aggErr := uuid.Parse(stripRefPrefix(orderIDStr))
		if aggErr != nil {
			aggID = uuid.NewSHA1(tenantID, []byte(orderIDStr))
		}
		failedEvent := events.NewEvent("ordering.order.delivery_failed", aggID, tenantID, map[string]interface{}{
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
