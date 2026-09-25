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

// handleTaskCreated records the OrderAssignment as soon as logistics-api creates the delivery
// task (before a rider is assigned) — logistics-api's own OrderReadyConsumer owns task creation
// end-to-end for "ordering.order.ready"; this handler only tracks that outcome locally. It is
// idempotent: if an assignment already exists for the order (e.g. handleTaskAssigned's own
// create-if-missing fallback already ran first), it is left untouched.
func (h *LogisticsEventHandler) handleTaskCreated(ctx context.Context, evt *sharedevents.Event) error {
	orderID, tenantID, ok := h.orderRefFromEvent(evt)
	if !ok {
		// Not every logistics task originates from an ordering order (e.g. manual fleet
		// tasks) — nothing to track locally, not an error.
		return nil
	}

	data := evt.Payload
	taskIDStr, _ := data["task_id"].(string)
	if taskIDStr == "" {
		return nil
	}

	if _, err := h.repo.GetAssignmentByOrderID(ctx, tenantID, orderID); err == nil {
		return nil // already tracked (e.g. race with handleTaskAssigned's fallback create)
	}

	newAssignment := &OrderAssignment{
		TenantID:        tenantID,
		OrderID:         orderID,
		LogisticsTaskID: taskIDStr,
		Status:          AssignmentStatusPending,
		Priority:        PriorityNormal,
	}
	if createErr := h.repo.CreateAssignment(ctx, newAssignment); createErr != nil {
		h.logger.Error("failed to create assignment from task.created event",
			zap.Error(createErr),
			zap.String("task_id", taskIDStr),
			zap.String("order_id", orderID.String()))
		return fmt.Errorf("create assignment from task.created: %w", createErr)
	}
	return nil
}

// updateAssignmentFromEvent moves the local OrderAssignment for the event's task to status (and
// records the rider when the event carries one). Best-effort: a missing assignment is not an error.
func (h *LogisticsEventHandler) updateAssignmentFromEvent(ctx context.Context, evt *sharedevents.Event, status AssignmentStatus) {
	data := evt.Payload
	taskIDStr, _ := data["task_id"].(string)
	if taskIDStr == "" {
		return
	}
	assignment, err := h.repo.GetAssignmentByLogisticsTaskID(ctx, taskIDStr)
	if err != nil || assignment == nil {
		return
	}
	now := time.Now()
	assignment.Status = status
	switch status {
	case AssignmentStatusAccepted:
		assignment.AcceptedAt = &now
	case AssignmentStatusCompleted, AssignmentStatusCancelled:
		assignment.CompletedAt = &now
	}
	if fleetMemberID, _ := data["fleet_member_id"].(string); fleetMemberID != "" {
		assignment.RiderID = fleetMemberID
	}
	if uerr := h.repo.UpdateAssignment(ctx, assignment); uerr != nil {
		h.logger.Error("failed to update assignment",
			zap.Error(uerr), zap.String("task_id", taskIDStr), zap.String("status", string(status)))
	}
}

// stampDeliveryState records rider-side progress on the order (delivery_status plus the rider's
// fleet member id and task id) so the storefront tracker, the POS online-orders queue and the
// ordering staff dashboard can show "Rider assigned", "Rider at the outlet" and so on, without
// adding statuses to the order state machine.
func (h *LogisticsEventHandler) stampDeliveryState(ctx context.Context, tenantID, orderID uuid.UUID, evt *sharedevents.Event, state string) {
	patch := map[string]interface{}{
		"delivery_status":            state,
		"delivery_status_updated_at": time.Now().UTC().Format(time.RFC3339),
	}
	if fm, _ := evt.Payload["fleet_member_id"].(string); fm != "" {
		patch["rider_id"] = fm
	}
	if name, _ := evt.Payload["rider_name"].(string); name != "" {
		patch["rider_name"] = name
	}
	if taskID, _ := evt.Payload["task_id"].(string); taskID != "" {
		patch["logistics_task_id"] = taskID
	}
	if err := h.orderingRepo.MergeOrderMetadata(ctx, tenantID, orderID, patch); err != nil {
		h.logger.Warn("failed to stamp delivery state on order",
			zap.String("order_id", orderID.String()), zap.String("state", state), zap.Error(err))
	}
}

// handleTaskAssigned records the rider assignment. The order itself stays "ready": the food is
// still at the counter until the rider actually collects it (see handleTaskPickedUp).
func (h *LogisticsEventHandler) handleTaskAssigned(ctx context.Context, evt *sharedevents.Event) error {
	orderID, tenantID, ok := h.orderRefFromEvent(evt)
	if !ok {
		// Not an ordering order (manual fleet task) or no tenant: nothing to track.
		return nil
	}
	data := evt.Payload
	fleetMemberID, _ := data["fleet_member_id"].(string)
	taskIDStr, _ := data["task_id"].(string)

	if taskIDStr != "" {
		if assignment, assignErr := h.repo.GetAssignmentByLogisticsTaskID(ctx, taskIDStr); assignErr == nil && assignment != nil {
			now := time.Now()
			assignment.RiderID = fleetMemberID
			assignment.Status = AssignmentStatusAssigned
			assignment.AssignedAt = &now
			if updateErr := h.repo.UpdateAssignment(ctx, assignment); updateErr != nil {
				h.logger.Error("failed to update assignment", zap.Error(updateErr), zap.String("task_id", taskIDStr))
			}
		} else {
			now := time.Now()
			if createErr := h.repo.CreateAssignment(ctx, &OrderAssignment{
				TenantID:        tenantID,
				OrderID:         orderID,
				LogisticsTaskID: taskIDStr,
				RiderID:         fleetMemberID,
				Status:          AssignmentStatusAssigned,
				Priority:        PriorityNormal,
				AssignedAt:      &now,
			}); createErr != nil {
				h.logger.Error("failed to create assignment from task.assigned event",
					zap.Error(createErr), zap.String("task_id", taskIDStr))
			}
		}
	}
	h.stampDeliveryState(ctx, tenantID, orderID, evt, "rider_assigned")
	return nil
}

// handleTaskAccepted records that the rider accepted the job. The order stays "ready" until pickup.
func (h *LogisticsEventHandler) handleTaskAccepted(ctx context.Context, evt *sharedevents.Event) error {
	h.updateAssignmentFromEvent(ctx, evt, AssignmentStatusAccepted)
	if orderID, tenantID, ok := h.orderRefFromEvent(evt); ok {
		h.stampDeliveryState(ctx, tenantID, orderID, evt, "rider_accepted")
	}
	return nil
}

// handleTaskLeg records an intermediate rider step (heading to the outlet, at the outlet, heading
// to or at the customer) on the assignment and the order.
func (h *LogisticsEventHandler) handleTaskLeg(ctx context.Context, evt *sharedevents.Event, status AssignmentStatus) error {
	h.updateAssignmentFromEvent(ctx, evt, status)
	if orderID, tenantID, ok := h.orderRefFromEvent(evt); ok {
		h.stampDeliveryState(ctx, tenantID, orderID, evt, string(status))
	}
	return nil
}

// handleTaskPickedUp is the moment the order leaves the outlet: it moves ready -> out_for_delivery,
// which sends the customer the "on its way" notification. The legacy single-leg rider flow's
// en_route status lands here too.
func (h *LogisticsEventHandler) handleTaskPickedUp(ctx context.Context, evt *sharedevents.Event) error {
	h.updateAssignmentFromEvent(ctx, evt, AssignmentStatusPickedUp)
	orderID, tenantID, ok := h.orderRefFromEvent(evt)
	if !ok {
		return nil
	}
	h.stampDeliveryState(ctx, tenantID, orderID, evt, "picked_up")
	order, err := h.orderingRepo.GetOrder(ctx, tenantID, orderID)
	if err != nil {
		return fmt.Errorf("get order %s: %w", orderID, err)
	}
	if order.Status != ordering.OrderStatusReady {
		// Already on its way (duplicate event) or the kitchen never marked it ready; the delivered
		// event still closes it out through advanceToDelivered.
		return nil
	}
	if _, terr := h.orderingSvc.UpdateOrderStatus(ctx, tenantID, orderID,
		ordering.OrderStatusOutForDelivery, nil, "system", ""); terr != nil {
		return fmt.Errorf("transition order to out_for_delivery: %w", terr)
	}
	h.logger.Info("order out_for_delivery on rider pickup", zap.String("order_id", orderID.String()))
	return nil
}

// advanceToDelivered walks a delivery order to "delivered" from ready or out_for_delivery. The
// transition itself (UpdateOrderStatus) settles cash on delivery with treasury, consumes the stock
// reservation, awards loyalty and publishes ordering.order.delivered exactly once.
func (h *LogisticsEventHandler) advanceToDelivered(ctx context.Context, tenantID, orderID uuid.UUID) error {
	order, err := h.orderingRepo.GetOrder(ctx, tenantID, orderID)
	if err != nil {
		return fmt.Errorf("get order %s: %w", orderID, err)
	}
	if order.Status == ordering.OrderStatusReady {
		if _, terr := h.orderingSvc.UpdateOrderStatus(ctx, tenantID, orderID,
			ordering.OrderStatusOutForDelivery, nil, "system", ""); terr != nil {
			return fmt.Errorf("transition order to out_for_delivery: %w", terr)
		}
		order.Status = ordering.OrderStatusOutForDelivery
	}
	if order.Status != ordering.OrderStatusOutForDelivery {
		h.logger.Info("order not awaiting delivery, skipping",
			zap.String("order_id", orderID.String()), zap.String("current_status", string(order.Status)))
		return nil
	}
	if _, terr := h.orderingSvc.UpdateOrderStatus(ctx, tenantID, orderID,
		ordering.OrderStatusDelivered, nil, "system", ""); terr != nil {
		return fmt.Errorf("transition order to delivered: %w", terr)
	}
	return nil
}

// handleTaskDelivered closes the order when the rider reports delivery (status event).
func (h *LogisticsEventHandler) handleTaskDelivered(ctx context.Context, evt *sharedevents.Event) error {
	h.updateAssignmentFromEvent(ctx, evt, AssignmentStatusCompleted)
	orderID, tenantID, ok := h.orderRefFromEvent(evt)
	if !ok {
		return nil
	}
	h.stampDeliveryState(ctx, tenantID, orderID, evt, "delivered")
	return h.advanceToDelivered(ctx, tenantID, orderID)
}

// handleTaskCompleted closes the order when the rider submits proof of delivery (task.completed).
// COD settlement and the delivered notification happen inside the delivered transition; they used
// to run a second time here (a duplicate treasury settle and a duplicate "delivered" email).
func (h *LogisticsEventHandler) handleTaskCompleted(ctx context.Context, evt *sharedevents.Event) error {
	orderID, tenantID, ok := h.orderRefFromEvent(evt)
	if !ok {
		return fmt.Errorf("no usable order reference / tenant in task.completed event")
	}
	h.updateAssignmentFromEvent(ctx, evt, AssignmentStatusCompleted)
	if collected, _ := evt.Payload["cash_collected"].(bool); collected {
		amount, _ := evt.Payload["amount_collected"].(float64)
		if err := h.orderingRepo.MergeOrderMetadata(ctx, tenantID, orderID, map[string]interface{}{
			"cod_collected_by_rider": true,
			"cod_amount_collected":   amount,
		}); err != nil {
			h.logger.Warn("failed to record rider COD collection", zap.String("order_id", orderID.String()), zap.Error(err))
		}
	}
	h.stampDeliveryState(ctx, tenantID, orderID, evt, "delivered")
	if err := h.advanceToDelivered(ctx, tenantID, orderID); err != nil {
		return err
	}
	h.logger.Info("order delivered from logistics proof of delivery", zap.String("order_id", orderID.String()))
	return nil
}

// handleTaskCancelled handles a rider or dispatcher cancelling the delivery task. The order is not
// cancelled (the food is still at the outlet or with the rider); it is flagged as needing a new
// rider so the outlet and the dispatcher can re-assign, and an admin alert is raised.
func (h *LogisticsEventHandler) handleTaskCancelled(ctx context.Context, evt *sharedevents.Event) error {
	h.updateAssignmentFromEvent(ctx, evt, AssignmentStatusCancelled)
	orderID, tenantID, ok := h.orderRefFromEvent(evt)
	if !ok {
		return nil
	}
	order, err := h.orderingRepo.GetOrder(ctx, tenantID, orderID)
	if err != nil {
		return fmt.Errorf("get order %s: %w", orderID, err)
	}
	switch order.Status {
	case ordering.OrderStatusCancelled, ordering.OrderStatusRefunded, ordering.OrderStatusDelivered, ordering.OrderStatusCompleted:
		return nil // the order itself is finished; nothing to re-dispatch
	}
	h.stampDeliveryState(ctx, tenantID, orderID, evt, "needs_rider")
	if h.eventPublisher != nil {
		reason, _ := evt.Payload["reason"].(string)
		alert := events.NewEvent("ordering.order.delivery_failed", orderID, tenantID, map[string]interface{}{
			"order_id":       orderID.String(),
			"order_number":   order.OrderNumber,
			"task_id":        evt.Payload["task_id"],
			"failure_reason": strings.TrimSpace("Delivery task cancelled. " + reason),
			"failed_at":      time.Now().UTC().Format(time.RFC3339),
			"notification":   map[string]interface{}{"target": "admin"},
		})
		_ = h.eventPublisher.Publish(ctx, "ordering.order.delivery_failed", alert)
	}
	h.logger.Warn("delivery task cancelled; order needs a new rider", zap.String("order_id", orderID.String()))
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
