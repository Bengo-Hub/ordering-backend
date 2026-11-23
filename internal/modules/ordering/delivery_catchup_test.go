package ordering

import "testing"

// A delivery order the rider already has is caught up from the kitchen statuses only. Pickups,
// orders already ready or beyond, and finished or cancelled orders are left alone.
func TestNeedsDeliveryCatchUp(t *testing.T) {
	cases := []struct {
		ft     FulfillmentType
		status OrderStatus
		want   bool
	}{
		{FulfillmentTypeDelivery, OrderStatusConfirmed, true},
		{FulfillmentTypeDelivery, OrderStatusPreparing, true},
		{FulfillmentTypeDelivery, OrderStatusReady, false},
		{FulfillmentTypeDelivery, OrderStatusOutForDelivery, false},
		{FulfillmentTypeDelivery, OrderStatusDelivered, false},
		{FulfillmentTypeDelivery, OrderStatusCancelled, false},
		{FulfillmentTypeDelivery, OrderStatusPending, false},
		{FulfillmentTypePickup, OrderStatusPreparing, false},
	}
	for _, c := range cases {
		got := needsDeliveryCatchUp(&Order{FulfillmentType: c.ft, Status: c.status})
		if got != c.want {
			t.Errorf("needsDeliveryCatchUp(%s, %s) = %v, want %v", c.ft, c.status, got, c.want)
		}
	}
	if needsDeliveryCatchUp(nil) {
		t.Error("nil order must not need a catch-up")
	}
}
