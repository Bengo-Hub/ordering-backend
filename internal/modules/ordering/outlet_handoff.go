package ordering

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
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

// needsHandoverCode reports whether the order gets a 6-digit hand-over code (stored in PODCode):
// delivery orders give it to the rider as proof of delivery, pickup orders show it at the counter
// so staff hand the bag to the right person. A pickup made only of bookings (a haircut, an event
// ticket) has nothing to collect, so it gets no code.
func needsHandoverCode(ft FulfillmentType, lineMetadata []map[string]interface{}) bool {
	if isDeliveryFulfilment(ft) {
		return true
	}
	return ft == FulfillmentTypePickup && !bookingOnly(lineMetadata)
}

// bookingOnly reports whether every line is a service appointment or event ticket.
func bookingOnly(lineMetadata []map[string]interface{}) bool {
	if len(lineMetadata) == 0 {
		return false
	}
	for _, meta := range lineMetadata {
		service, _ := meta["is_service"].(bool)
		ticket, _ := meta["is_ticket"].(bool)
		if !service && !ticket {
			return false
		}
	}
	return true
}

func cartLineMetadata(items []CartItem) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(items))
	for _, it := range items {
		out = append(out, it.Metadata)
	}
	return out
}

func inputLineMetadata(items []CreateOrderItemInput) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(items))
	for _, it := range items {
		out = append(out, it.Metadata)
	}
	return out
}

// deliveryCode is the rider proof-of-delivery code; empty for pickup orders so messages that
// say "give this code to the rider" never carry a counter collection code.
func deliveryCode(order *Order) string {
	if order == nil || !isDeliveryFulfilment(order.FulfillmentType) {
		return ""
	}
	return order.PODCode
}

// collectionCode is the code a pickup customer shows at the counter; empty for other orders.
func collectionCode(order *Order) string {
	if order == nil || order.FulfillmentType != FulfillmentTypePickup {
		return ""
	}
	return order.PODCode
}

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

// ConfigKeyAutoAccept is the tenant service-config key that switches order acceptance to automatic.
// Acceptance is MANUAL unless it is set to true: every order waits for the outlet to accept it.
const ConfigKeyAutoAccept = "orders.auto_accept"

// metaOutletOfferedAt marks when an order was offered to the outlet for acceptance.
const metaOutletOfferedAt = "outlet_offered_at"

// autoAcceptEnabled reports whether the tenant accepts orders automatically.
func (s *OrderService) autoAcceptEnabled(ctx context.Context, tenantID uuid.UUID) bool {
	v, ok := s.repo.GetConfigValue(ctx, tenantID, ConfigKeyAutoAccept)
	if !ok {
		return false
	}
	on, err := strconv.ParseBool(strings.Trim(strings.TrimSpace(v), `"`))
	return err == nil && on
}

// readyForAcceptance reports whether a pending order may be accepted: it is paid, or it is paid
// later (cash / M-Pesa on collection or delivery, or a customer-keyed M-Pesa payment the outlet
// checks). An unpaid online-payment order is not: the customer may still abandon the payment.
func readyForAcceptance(order *Order) bool {
	return order != nil && (order.PaymentStatus == PaymentStatusPaid || isOfflinePayment(order))
}

// acceptOrOffer is called when a pending order becomes acceptable (placed with a pay-later method,
// or its online payment landed). With automatic acceptance it is confirmed straight away (and
// handed to the kitchen); otherwise it is offered to the outlet, whose POS queue rings with
// Accept / Reject, and nothing reaches the kitchen until someone accepts it.
func (s *OrderService) acceptOrOffer(ctx context.Context, order *Order) {
	if order == nil || order.Status != OrderStatusPending || !readyForAcceptance(order) {
		return
	}
	if s.autoAcceptEnabled(ctx, order.TenantID) {
		updated, err := s.UpdateOrderStatus(ctx, order.TenantID, order.ID, OrderStatusConfirmed, nil, "system", "")
		if err != nil {
			s.logger.Warn("auto-accept failed; offering the order to the outlet instead",
				zap.String("order_id", order.ID.String()), zap.Error(err))
		} else {
			order.Status = updated.Status
			order.ConfirmedAt = updated.ConfirmedAt
			return
		}
	}
	s.offerToOutlet(ctx, order)
}

// offerToOutlet publishes ordering.order.awaiting_acceptance once per order.
func (s *OrderService) offerToOutlet(ctx context.Context, order *Order) {
	if order == nil || !isOutletFulfilled(order.FulfillmentType) {
		return
	}
	if order.Metadata != nil {
		if v, _ := order.Metadata[metaOutletOfferedAt].(string); v != "" {
			return
		}
	}
	s.publishOutletHandoff(ctx, order, true)
	if err := s.repo.MergeOrderMetadata(ctx, order.TenantID, order.ID, map[string]interface{}{
		metaOutletOfferedAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		s.logger.Warn("failed to stamp outlet offer time", zap.String("order_id", order.ID.String()), zap.Error(err))
	}
}

// autoAcceptCOD is kept for the checkout call sites: a pay-later order is acceptable as soon as it
// is placed, so it is accepted or offered to the outlet according to the tenant's policy.
func (s *OrderService) autoAcceptCOD(ctx context.Context, order *Order) {
	s.acceptOrOffer(ctx, order)
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
