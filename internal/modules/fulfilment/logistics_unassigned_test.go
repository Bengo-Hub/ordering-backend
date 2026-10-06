package fulfilment

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/bengobox/ordering-backend/internal/modules/ordering"
	"github.com/bengobox/ordering-backend/internal/platform/logistics"
)

func TestIsOrderingTaskEvent(t *testing.T) {
	cases := []struct {
		name    string
		payload map[string]interface{}
		want    bool
	}{
		{"no source (older events)", map[string]interface{}{}, true},
		{"ordering", map[string]interface{}{"source_service": "ordering"}, true},
		{"ordering with case and spaces", map[string]interface{}{"source_service": " Ordering "}, true},
		{"pos till delivery", map[string]interface{}{"source_service": "pos", "external_reference": uuid.NewString()}, false},
		{"inventory transfer", map[string]interface{}{"source_service": "inventory"}, false},
		{"non string source", map[string]interface{}{"source_service": 5}, true},
	}
	for _, c := range cases {
		if got := isOrderingTaskEvent(c.payload); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestEventRiderID(t *testing.T) {
	if got := eventRiderID(map[string]interface{}{"fleet_member_id": uuid.Nil.String()}); got != "" {
		t.Fatalf("nil uuid should read as no rider, got %q", got)
	}
	if got := eventRiderID(map[string]interface{}{}); got != "" {
		t.Fatalf("missing rider should be empty, got %q", got)
	}
	if got := eventRiderID(map[string]interface{}{"fleet_member_id": "r1"}); got != "r1" {
		t.Fatalf("got %q", got)
	}
}

func TestRiderReleasable(t *testing.T) {
	held := func(rider string, s AssignmentStatus) *OrderAssignment {
		return &OrderAssignment{RiderID: rider, Status: s}
	}
	cases := []struct {
		name  string
		a     *OrderAssignment
		event string
		want  bool
	}{
		{"nil assignment", nil, "r1", false},
		{"already released (replay)", held("", AssignmentStatusPending), "r1", false},
		{"same rider assigned", held("r1", AssignmentStatusAssigned), "r1", true},
		{"same rider accepted", held("r1", AssignmentStatusAccepted), "r1", true},
		{"same rider heading to outlet", held("r1", AssignmentStatusEnRoutePickup), "r1", true},
		{"same rider at outlet", held("r1", AssignmentStatusArrivedPickup), "r1", true},
		{"event names no rider", held("r1", AssignmentStatusAssigned), "", true},
		{"newer rider landed first", held("r2", AssignmentStatusAssigned), "r1", false},
		{"already picked up", held("r1", AssignmentStatusPickedUp), "r1", false},
		{"completed", held("r1", AssignmentStatusCompleted), "r1", false},
	}
	for _, c := range cases {
		if got := riderReleasable(c.a, c.event); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestUnassignedAssignmentMetadata(t *testing.T) {
	at := time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)
	orig := map[string]interface{}{"keep": 1}
	got := unassignedAssignmentMetadata(orig, "bike trouble", at)
	if got["needs_rider"] != true || got["unassigned_reason"] != "bike trouble" || got["keep"] != 1 {
		t.Fatalf("unexpected %v", got)
	}
	if _, touched := orig["needs_rider"]; touched {
		t.Fatal("original metadata mutated")
	}
	if _, ok := unassignedAssignmentMetadata(nil, "", at)["unassigned_reason"]; ok {
		t.Fatal("empty reason stored")
	}
}

func TestUnassignedOrderPatch(t *testing.T) {
	at := time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)

	t.Run("ready order with the rider clears rider and flags needs_rider", func(t *testing.T) {
		meta := map[string]interface{}{"rider_id": "r1", "rider_name": "Ann", "rider_phone": "07", "delivery_status": "rider_assigned"}
		p := unassignedOrderPatch(ordering.OrderStatusReady, meta, "r1", "too far", at)
		if p == nil {
			t.Fatal("expected a patch")
		}
		if p["delivery_status"] != "needs_rider" || p["rider_unassigned_reason"] != "too far" {
			t.Fatalf("unexpected %v", p)
		}
		for _, k := range []string{"rider_id", "rider_name", "rider_phone"} {
			v, ok := p[k]
			if !ok || v != nil {
				t.Fatalf("%s not cleared: %v", k, p)
			}
		}
	})

	t.Run("still cooking is flagged too", func(t *testing.T) {
		if unassignedOrderPatch(ordering.OrderStatusPreparing, map[string]interface{}{"rider_id": "r1"}, "r1", "", at) == nil {
			t.Fatal("expected a patch")
		}
	})

	t.Run("newer rider already stamped is kept", func(t *testing.T) {
		if p := unassignedOrderPatch(ordering.OrderStatusReady, map[string]interface{}{"rider_id": "r2"}, "r1", "", at); p != nil {
			t.Fatalf("newer rider overwritten: %v", p)
		}
	})

	t.Run("replay writes nothing", func(t *testing.T) {
		meta := map[string]interface{}{"rider_id": nil, "delivery_status": "needs_rider"}
		if p := unassignedOrderPatch(ordering.OrderStatusReady, meta, "r1", "", at); p != nil {
			t.Fatalf("replay wrote %v", p)
		}
	})

	t.Run("finished or on the road orders are left alone", func(t *testing.T) {
		for _, s := range []ordering.OrderStatus{
			ordering.OrderStatusOutForDelivery, ordering.OrderStatusDelivered, ordering.OrderStatusCompleted,
			ordering.OrderStatusCancelled, ordering.OrderStatusRefunded,
		} {
			if p := unassignedOrderPatch(s, map[string]interface{}{"rider_id": "r1"}, "r1", "", at); p != nil {
				t.Fatalf("%s patched: %v", s, p)
			}
		}
	})
}

func TestTrackingFromLogistics(t *testing.T) {
	a := &OrderAssignment{ID: uuid.New(), OrderID: uuid.New(), RiderID: "fm-1"}
	eta := time.Date(2026, 10, 6, 12, 30, 0, 0, time.UTC)

	t.Run("rider in progress", func(t *testing.T) {
		got := trackingFromLogistics(a, &logistics.TrackingInfo{
			Status:        "en_route_dropoff",
			RiderName:     "Ann",
			RiderPhone:    "0700",
			ETAMinutes:    12,
			ETAAt:         &eta,
			DistanceKm:    3.4,
			RiderLocation: &logistics.RiderLocation{RiderID: "fm-1", Latitude: -1.2, Longitude: 36.8},
		})
		if got.OrderID != a.OrderID || got.AssignmentID != a.ID {
			t.Fatal("ids not carried")
		}
		if got.RiderName != "Ann" || got.RiderPhone != "0700" || got.Status != "en_route_dropoff" {
			t.Fatalf("rider fields: %+v", got)
		}
		if got.ETAMinutes == nil || *got.ETAMinutes != 12 || got.DistanceKm == nil || *got.DistanceKm != 3.4 {
			t.Fatalf("eta fields: %+v", got)
		}
		if got.RiderLatitude == nil || *got.RiderLatitude != -1.2 || *got.RiderLongitude != 36.8 {
			t.Fatalf("position: %+v", got)
		}
	})

	t.Run("no fix and no eta yet", func(t *testing.T) {
		got := trackingFromLogistics(a, &logistics.TrackingInfo{Status: "assigned"})
		if got.ETAMinutes != nil || got.DistanceKm != nil {
			t.Fatal("zero eta or distance should be left out")
		}
		if got.RiderLatitude != nil {
			t.Fatal("position without a fix")
		}
		if got.RiderID != "fm-1" {
			t.Fatalf("rider id should fall back to the assignment, got %q", got.RiderID)
		}
	})
}
