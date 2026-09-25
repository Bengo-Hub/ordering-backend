package ordering

import (
	"context"
	"strings"
	"time"

	"go.uber.org/zap"
)

// metaOutletHandoffAt marks (in order.metadata) the moment an order was handed to the outlet,
// i.e. ordering.order.confirmed was published so pos-api created the POS/KDS record. It lets the
// scheduler hand a scheduled order over exactly once when its prep window opens.
const metaOutletHandoffAt = "outlet_handoff_at"

// resolveFulfillment maps the storefront's fulfilment mode onto the stored enum and validates the
// requested time. The storefront offers Delivery, Pickup and Schedule; "schedule" means a delivery
// at a chosen time, so it is stored as a delivery with scheduled_for set (the legacy "scheduled"
// value is treated the same way). Storing it as its own fulfilment type used to fail the ent enum
// ("schedule" is not a value) and lost whether the customer wanted delivery or pickup.
func resolveFulfillment(raw FulfillmentType, scheduledFor *time.Time) (FulfillmentType, error) {
	mode := strings.ToLower(strings.TrimSpace(string(raw)))
	scheduleMode := mode == "schedule" || mode == "scheduled"
	if scheduleMode && scheduledFor == nil {
		return "", ErrScheduledForRequired
	}
	if scheduledFor != nil && scheduledFor.Before(time.Now().Add(ScheduledMinLeadTime)) {
		return "", ErrScheduledForTooSoon
	}
	switch mode {
	case "pickup", "takeaway", "collect", "click_and_collect":
		return FulfillmentTypePickup, nil
	case "dine_in":
		return FulfillmentTypeDineIn, nil
	default:
		return FulfillmentTypeDelivery, nil
	}
}

// isOutletFulfilled reports whether an order is prepared and handed over by an outlet (pickup or
// delivery, including legacy "scheduled" rows), as opposed to dine-in tickets.
func isOutletFulfilled(ft FulfillmentType) bool {
	return ft == FulfillmentTypePickup || ft == FulfillmentTypeDelivery || ft == FulfillmentTypeScheduled
}

// isDeliveryFulfilment reports whether the order ends with a rider drop-off.
func isDeliveryFulfilment(ft FulfillmentType) bool {
	return ft == FulfillmentTypeDelivery || ft == FulfillmentTypeScheduled
}

// IsDeliveryFulfilment is the exported form of isDeliveryFulfilment for handlers.
func IsDeliveryFulfilment(ft FulfillmentType) bool { return isDeliveryFulfilment(ft) }

// validateFulfilmentTransition rejects status moves that make no sense for the order's fulfilment
// type: a pickup order is never handed to a rider, and a delivery order is only complete once the
// rider has delivered it (ready -> completed would skip the drop-off, the COD collection on the
// doorstep and the delivery confirmation).
func validateFulfilmentTransition(ft FulfillmentType, from, to OrderStatus) error {
	delivery := isDeliveryFulfilment(ft)
	switch {
	case !delivery && (to == OrderStatusOutForDelivery || to == OrderStatusDelivered):
		return ErrInvalidStatusTransition
	case delivery && from == OrderStatusReady && to == OrderStatusCompleted:
		return ErrInvalidStatusTransition
	}
	return nil
}

// handOffToOutlet publishes ordering.order.confirmed so pos-api creates the outlet's POS record,
// KDS tickets, kitchen chits or appointment. A scheduled order whose prep window has not opened
// yet is held back; OrderScheduler hands it over later. The hand-off time is stamped on the order
// so the scheduler never repeats it (pos-api is idempotent on the online order id regardless).
func (s *OrderService) handOffToOutlet(ctx context.Context, order *Order) {
	if order == nil || !isOutletFulfilled(order.FulfillmentType) {
		return
	}
	if order.ScheduledFor != nil && order.ScheduledFor.After(time.Now().Add(ScheduledPrepTimeBuffer)) {
		s.logger.Info("scheduled order held until its prep window",
			zap.String("order_id", order.ID.String()),
			zap.Time("scheduled_for", *order.ScheduledFor))
		return
	}
	s.publishOrderConfirmed(ctx, order)
	stamp := map[string]interface{}{metaOutletHandoffAt: time.Now().UTC().Format(time.RFC3339)}
	if err := s.repo.MergeOrderMetadata(ctx, order.TenantID, order.ID, stamp); err != nil {
		s.logger.Warn("failed to stamp outlet hand-off time",
			zap.String("order_id", order.ID.String()), zap.Error(err))
	}
}

// alreadyHandedOff reports whether the outlet already received the order.
func alreadyHandedOff(order *Order) bool {
	if order == nil || order.Metadata == nil {
		return false
	}
	v, _ := order.Metadata[metaOutletHandoffAt].(string)
	return v != ""
}

// autoAcceptCOD confirms a freshly placed cash-on-delivery / pay-at-counter order straight away.
// Online-payment orders confirm themselves when the payment lands; a COD order has no such
// trigger, so without this it sat in "pending" until someone opened the ordering staff dashboard
// and was invisible to the POS queue and the kitchen in the meantime. Outlet staff can still
// reject it from the POS online orders queue, which cancels it here and notifies the customer.
func (s *OrderService) autoAcceptCOD(ctx context.Context, order *Order) {
	if order == nil || order.PaymentMethod != PaymentMethodCOD || order.Status != OrderStatusPending {
		return
	}
	updated, err := s.UpdateOrderStatus(ctx, order.TenantID, order.ID, OrderStatusConfirmed, nil, "system", "")
	if err != nil {
		s.logger.Warn("auto-accept of COD order failed; it stays pending for manual acceptance",
			zap.String("order_id", order.ID.String()), zap.Error(err))
		return
	}
	order.Status = updated.Status
	order.ConfirmedAt = updated.ConfirmedAt
}

// orderNotesMetadata folds the customer's kitchen notes and utensils request into order metadata
// so they reach the outlet (POS mirror, KDS ticket header, kitchen chit).
func orderNotesMetadata(metadata map[string]interface{}, orderNotes string, requestUtensils bool) map[string]interface{} {
	notes := strings.TrimSpace(orderNotes)
	if notes == "" && !requestUtensils {
		return metadata
	}
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	if notes != "" {
		metadata["order_notes"] = notes
	}
	if requestUtensils {
		metadata["request_utensils"] = true
	}
	return metadata
}

// outletNotes renders the order-level notes the outlet should see on the ticket.
func outletNotes(order *Order) string {
	if order == nil || order.Metadata == nil {
		return ""
	}
	parts := []string{}
	if n, _ := order.Metadata["order_notes"].(string); strings.TrimSpace(n) != "" {
		parts = append(parts, strings.TrimSpace(n))
	}
	if u, _ := order.Metadata["request_utensils"].(bool); u {
		parts = append(parts, "Cutlery requested")
	}
	return strings.Join(parts, " | ")
}
