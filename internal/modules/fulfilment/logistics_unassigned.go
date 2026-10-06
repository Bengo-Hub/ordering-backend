package fulfilment

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	sharedevents "github.com/Bengo-Hub/shared-events"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/modules/ordering"
)

// isOrderingTaskEvent reports whether a logistics task event belongs to an ordering order. Tasks
// raised by other services carry their own source_service (POS till deliveries send "pos" with a
// bare POS order id as the reference), and ordering must leave those alone. Older events without
// a source_service are treated as ordering's, since ordering was the only producer then.
func isOrderingTaskEvent(payload map[string]interface{}) bool {
	src, _ := payload["source_service"].(string)
	src = strings.ToLower(strings.TrimSpace(src))
	return src == "" || src == "ordering"
}

// eventRiderID returns the fleet member named by a task event, or "" when there is none.
// logistics-api sends the nil UUID when no rider held the task.
func eventRiderID(payload map[string]interface{}) string {
	id, _ := payload["fleet_member_id"].(string)
	id = strings.TrimSpace(id)
	if id == uuid.Nil.String() {
		return ""
	}
	return id
}

// riderReleasable reports whether a logistics.task.unassigned event should free the local
// assignment. It must still be held, by the rider the event names when it names one (a newer
// assignment that landed first is kept), and the order must not have been collected yet.
func riderReleasable(a *OrderAssignment, eventRider string) bool {
	if a == nil || a.RiderID == "" {
		return false
	}
	if eventRider != "" && a.RiderID != eventRider {
		return false
	}
	switch a.Status {
	case AssignmentStatusPending, AssignmentStatusAssigned, AssignmentStatusAccepted,
		AssignmentStatusEnRoutePickup, AssignmentStatusArrivedPickup:
		return true
	}
	return false
}

// unassignedAssignmentMetadata adds the needs_rider stamp to an assignment's metadata without
// mutating the original map.
func unassignedAssignmentMetadata(current map[string]interface{}, reason string, at time.Time) map[string]interface{} {
	out := make(map[string]interface{}, len(current)+3)
	for k, v := range current {
		out[k] = v
	}
	out["needs_rider"] = true
	out["unassigned_at"] = at.UTC().Format(time.RFC3339)
	if reason != "" {
		out["unassigned_reason"] = reason
	}
	return out
}

// unassignedOrderPatch decides the order metadata change for a logistics.task.unassigned event:
// delivery_status becomes needs_rider and the rider's id, name and phone are cleared, so the
// tracker and the outlet stop showing a rider who is no longer coming. It returns nil (write
// nothing) when the order is finished or already on the road, when a different rider has since
// been stamped, or when the order already shows needs_rider with no rider (a replayed event).
func unassignedOrderPatch(status ordering.OrderStatus, meta map[string]interface{}, eventRider, reason string, at time.Time) map[string]interface{} {
	switch status {
	case ordering.OrderStatusOutForDelivery, ordering.OrderStatusDelivered, ordering.OrderStatusCompleted,
		ordering.OrderStatusCancelled, ordering.OrderStatusRefunded, ordering.OrderStatusPaymentTimeout:
		return nil
	}
	current, _ := meta["rider_id"].(string)
	if current != "" && eventRider != "" && current != eventRider {
		return nil
	}
	if state, _ := meta["delivery_status"].(string); current == "" && state == "needs_rider" {
		return nil
	}
	patch := map[string]interface{}{
		"delivery_status":            "needs_rider",
		"delivery_status_updated_at": at.UTC().Format(time.RFC3339),
		"rider_id":                   nil,
		"rider_name":                 nil,
		"rider_phone":                nil,
	}
	if reason != "" {
		patch["rider_unassigned_reason"] = reason
	}
	return patch
}

// handleTaskUnassigned handles logistics.task.unassigned: the rider declined the job or a
// dispatcher took it back before pickup. The task is open again, so the assignment returns to
// pending without a rider and the order is flagged needs_rider. A later task.assigned (auto
// dispatch or the dispatcher) fills the rider in again. Replays change nothing.
func (h *LogisticsEventHandler) handleTaskUnassigned(ctx context.Context, evt *sharedevents.Event) error {
	orderID, tenantID, ok := h.orderRefFromEvent(evt)
	if !ok {
		return nil
	}
	eventRider := eventRiderID(evt.Payload)
	reason, _ := evt.Payload["reason"].(string)
	reason = strings.TrimSpace(reason)
	now := time.Now().UTC()

	if taskID, _ := evt.Payload["task_id"].(string); taskID != "" {
		a, err := h.repo.GetAssignmentByLogisticsTaskID(ctx, taskID)
		switch {
		case errors.Is(err, ErrAssignmentNotFound):
			// Never tracked locally; the order flag below is still worth setting.
		case err != nil:
			return fmt.Errorf("get assignment for task %s: %w", taskID, err)
		case riderReleasable(a, eventRider):
			meta := unassignedAssignmentMetadata(a.Metadata, reason, now)
			if _, rerr := h.repo.ReleaseAssignmentRider(ctx, a.ID, a.RiderID, meta); rerr != nil {
				return fmt.Errorf("release assignment rider: %w", rerr)
			}
		}
	}

	wrote, err := h.orderingRepo.MergeOrderMetadataIf(ctx, tenantID, orderID,
		func(status ordering.OrderStatus, meta map[string]interface{}) map[string]interface{} {
			return unassignedOrderPatch(status, meta, eventRider, reason, now)
		})
	if err != nil {
		return fmt.Errorf("flag order %s needs_rider: %w", orderID, err)
	}
	if wrote {
		h.logger.Info("rider released; order needs a rider",
			zap.String("order_id", orderID.String()), zap.String("reason", reason))
	}
	return nil
}
