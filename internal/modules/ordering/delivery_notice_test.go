package ordering

import "testing"

func snapshotOf(method string, needs bool) map[string]interface{} {
	return map[string]interface{}{
		metaDeliveryQuote: map[string]interface{}{
			"method": method, "zone_name": "Alupe", "distance_km": 7.4, "distance_type": "road",
			"eta_minutes": float64(40), "needs_confirmation": needs,
		},
	}
}

func TestDeliveryNoticeAsksForConfirmationOnlyOnDistancePricedDeliveries(t *testing.T) {
	// Sega: between Ugunja and Busia, outside every named area, priced by distance.
	order := &Order{FulfillmentType: FulfillmentTypeDelivery, DeliveryFee: 380, Metadata: snapshotOf("per_km", true)}
	order.Metadata[metaDeliveryQuote].(map[string]interface{})["zone_name"] = ""
	n := deliveryNoticeOf(order, "Sega market")
	if n == nil || !n.NeedsConfirmation || n.Fee != 380 || n.Free || n.DistanceKm != 7.4 || n.DistanceType != "road" || n.EtaMinutes != 40 || n.Address != "Sega market" {
		t.Fatalf("distance-priced delivery notice = %+v", n)
	}

	// A named area's fee is the published tariff: shown, but nothing to confirm.
	area := &Order{FulfillmentType: FulfillmentTypeDelivery, DeliveryFee: 100, Metadata: snapshotOf("zone", false)}
	if n := deliveryNoticeOf(area, "Alupe"); n == nil || n.NeedsConfirmation || n.Area != "Alupe" || n.Method != "zone" {
		t.Fatalf("area-priced delivery notice = %+v", n)
	}
}

func TestDeliveryNoticeFollowsTheTenantPolicyAndTheChargedFee(t *testing.T) {
	// The tenant switched confirmation off in logistics: the quote carries no flag.
	off := &Order{FulfillmentType: FulfillmentTypeDelivery, DeliveryFee: 380, Metadata: snapshotOf("per_km", false)}
	if n := deliveryNoticeOf(off, ""); n.NeedsConfirmation {
		t.Fatalf("policy off must not ask for confirmation: %+v", n)
	}
	// A distance delivery made free (free_delivery_minimum) has no fee to confirm.
	free := &Order{FulfillmentType: FulfillmentTypeDelivery, DeliveryFee: 0, Metadata: snapshotOf("per_km", true)}
	if n := deliveryNoticeOf(free, ""); n.NeedsConfirmation || !n.Free {
		t.Fatalf("a free delivery has nothing to confirm: %+v", n)
	}
}

func TestDeliveryNoticeIsNilForOrdersThatAreNotDelivered(t *testing.T) {
	if deliveryNoticeOf(nil, "") != nil {
		t.Fatal("nil order")
	}
	for _, ft := range []FulfillmentType{FulfillmentTypePickup, FulfillmentTypeDineIn} {
		if n := deliveryNoticeOf(&Order{FulfillmentType: ft, Metadata: snapshotOf("per_km", true)}, "x"); n != nil {
			t.Fatalf("%s must not carry a delivery notice: %+v", ft, n)
		}
	}
	// Orders from before quote snapshots still get address and fee.
	old := &Order{FulfillmentType: FulfillmentTypeDelivery, DeliveryFee: 150}
	if n := deliveryNoticeOf(old, "Bulanda"); n == nil || n.Fee != 150 || n.NeedsConfirmation || n.Address != "Bulanda" {
		t.Fatalf("legacy order notice = %+v", n)
	}
}

func TestNotServiceableMessageSaysHowFarDeliveriesGo(t *testing.T) {
	far := &DeliveryNotServiceableError{Reason: "outside_delivery_area", NearestArea: "Malaba", NearestAreaKm: 31, DeliveryReachKm: 28.4, OutletKm: 62.2}
	if got := far.Error(); got != "we do not deliver to this location yet (we deliver up to 28 km from the outlet and this is 62 km away)" {
		t.Fatalf("far message = %q", got)
	}
	near := &DeliveryNotServiceableError{Reason: "outside_delivery_area", NearestArea: "Malaba", NearestAreaKm: 6.1}
	if got := near.Error(); got != "we do not deliver to this location yet (nearest area: Malaba, 6.1 km away)" {
		t.Fatalf("near message = %q", got)
	}
}
