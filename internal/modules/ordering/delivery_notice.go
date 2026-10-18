package ordering

import "github.com/bengobox/ordering-backend/internal/platform/events"

// deliveryNoticeOf is what the order confirmation tells the customer about delivery: the
// address, the fee charged and how it was worked out, read from the quote snapshot stored on
// the order at checkout (metadata.delivery_quote). It is nil for orders that are not delivered.
//
// A delivery priced by distance (outside the named areas) carries NeedsConfirmation when the
// tenant's logistics policy asks for it, so the email asks the customer to check the area,
// distance and fee. The fee shown is the charged fee, after any free-delivery or discount rule.
func deliveryNoticeOf(order *Order, address string) *events.DeliveryNotice {
	if order == nil || !isDeliveryFulfilment(order.FulfillmentType) {
		return nil
	}
	n := &events.DeliveryNotice{Address: address, Fee: order.DeliveryFee, Free: order.DeliveryFee == 0}
	snap, ok := order.Metadata[metaDeliveryQuote].(map[string]interface{})
	if !ok {
		return n
	}
	n.Area, _ = snap["zone_name"].(string)
	n.Method, _ = snap["method"].(string)
	n.DistanceType, _ = snap["distance_type"].(string)
	n.DistanceKm = numberMeta(snap["distance_km"])
	n.EtaMinutes = int(numberMeta(snap["eta_minutes"]))
	// The confirmation is for a distance price the customer has not seen published; an area
	// fee (or a free delivery) is already the tenant's published tariff.
	needs, _ := snap["needs_confirmation"].(bool)
	n.NeedsConfirmation = needs && n.Method == "per_km" && order.DeliveryFee > 0
	return n
}

// numberMeta reads a JSON number out of order metadata (float64 after a round trip, int in
// memory).
func numberMeta(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	}
	return 0
}
