package ordering

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/bengobox/ordering-backend/internal/modules/catalog"
)

func TestResolveFulfillment(t *testing.T) {
	future := time.Now().Add(2 * time.Hour)
	tooSoon := time.Now().Add(5 * time.Minute)

	cases := []struct {
		name    string
		raw     FulfillmentType
		at      *time.Time
		want    FulfillmentType
		wantErr error
	}{
		{"empty defaults to delivery", "", nil, FulfillmentTypeDelivery, nil},
		{"delivery", "delivery", nil, FulfillmentTypeDelivery, nil},
		{"pickup", "pickup", nil, FulfillmentTypePickup, nil},
		{"takeaway alias", "takeaway", nil, FulfillmentTypePickup, nil},
		{"storefront schedule mode is a delivery", "schedule", &future, FulfillmentTypeDelivery, nil},
		{"legacy scheduled value is a delivery", "scheduled", &future, FulfillmentTypeDelivery, nil},
		{"schedule without a time is rejected", "schedule", nil, "", ErrScheduledForRequired},
		{"schedule inside the lead time is rejected", "schedule", &tooSoon, "", ErrScheduledForTooSoon},
		{"a pickup with a time too soon is rejected", "pickup", &tooSoon, "", ErrScheduledForTooSoon},
		{"scheduled pickup keeps pickup", "pickup", &future, FulfillmentTypePickup, nil},
		{"online dine-in is not offered", "dine_in", nil, "", ErrDineInNotOffered},
		{"online dine-in is refused case-insensitively", " Dine_In ", nil, "", ErrDineInNotOffered},
		{"case and space tolerant", "  Schedule ", &future, FulfillmentTypeDelivery, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveFulfillment(tc.raw, tc.at)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestValidateFulfilmentTransition(t *testing.T) {
	cases := []struct {
		name string
		ft   FulfillmentType
		from OrderStatus
		to   OrderStatus
		ok   bool
	}{
		{"pickup ready to completed (collected)", FulfillmentTypePickup, OrderStatusReady, OrderStatusCompleted, true},
		{"pickup never goes out for delivery", FulfillmentTypePickup, OrderStatusReady, OrderStatusOutForDelivery, false},
		{"pickup never delivered", FulfillmentTypePickup, OrderStatusOutForDelivery, OrderStatusDelivered, false},
		{"delivery cannot skip the drop-off", FulfillmentTypeDelivery, OrderStatusReady, OrderStatusCompleted, false},
		{"delivery ready to out for delivery", FulfillmentTypeDelivery, OrderStatusReady, OrderStatusOutForDelivery, true},
		{"delivery delivered to completed", FulfillmentTypeDelivery, OrderStatusDelivered, OrderStatusCompleted, true},
		{"legacy scheduled behaves as delivery", FulfillmentTypeScheduled, OrderStatusReady, OrderStatusCompleted, false},
		{"cancel always allowed here", FulfillmentTypePickup, OrderStatusPreparing, OrderStatusCancelled, true},
		{"dine in completes", FulfillmentTypeDineIn, OrderStatusReady, OrderStatusCompleted, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateFulfilmentTransition(tc.ft, tc.from, tc.to)
			if tc.ok && err != nil {
				t.Fatalf("expected allowed, got %v", err)
			}
			if !tc.ok && !errors.Is(err, ErrInvalidStatusTransition) {
				t.Fatalf("expected ErrInvalidStatusTransition, got %v", err)
			}
		})
	}
}

func TestOutletLinePayloadCarriesKitchenAndBookingFields(t *testing.T) {
	item := OrderItem{
		InventorySKU: "AMERICANO",
		NameSnapshot: "Americano",
		Quantity:     2,
		UnitPrice:    250,
		TotalPrice:   500,
		Metadata: map[string]interface{}{
			"category":   "Coffees",
			"notes":      "extra hot",
			"modifiers":  []map[string]interface{}{{"group_name": "Milk", "option_name": "Oat"}},
			"is_service": false,
			"internal":   "ignored",
		},
	}
	line := outletLinePayload(item)
	if line["category"] != "Coffees" {
		t.Fatalf("category missing: %v", line)
	}
	if line["notes"] != "extra hot" {
		t.Fatalf("notes missing: %v", line)
	}
	if _, ok := line["modifiers"]; !ok {
		t.Fatalf("modifiers missing: %v", line)
	}
	meta, _ := line["metadata"].(map[string]interface{})
	if _, leaked := meta["internal"]; leaked {
		t.Fatalf("unrelated metadata leaked to the outlet: %v", meta)
	}
	if line["quantity"] != 2 || line["total_price"] != 500.0 {
		t.Fatalf("quantity/total wrong: %v", line)
	}
}

func TestOutletLinePayloadServiceBooking(t *testing.T) {
	line := outletLinePayload(OrderItem{
		InventorySKU: "HAIRCUT",
		NameSnapshot: "Haircut",
		Quantity:     1,
		Metadata: map[string]interface{}{
			"is_service":       true,
			"appointment_date": "2026-10-01",
			"appointment_time": "14:30",
			"staff_id":         "abc",
		},
	})
	meta, _ := line["metadata"].(map[string]interface{})
	if meta["is_service"] != true || meta["appointment_time"] != "14:30" || meta["staff_id"] != "abc" {
		t.Fatalf("booking fields not carried: %v", meta)
	}
}

func TestOutletLinePayloadNoMetadata(t *testing.T) {
	line := outletLinePayload(OrderItem{InventorySKU: "X", NameSnapshot: "X", Quantity: 1})
	if _, ok := line["metadata"]; ok {
		t.Fatalf("no metadata expected: %v", line)
	}
}

func TestOutletNotes(t *testing.T) {
	if got := outletNotes(&Order{}); got != "" {
		t.Fatalf("empty order should have no notes, got %q", got)
	}
	o := &Order{Metadata: orderNotesMetadata(nil, "  no onions ", true)}
	if got := outletNotes(o); got != "no onions | Cutlery requested" {
		t.Fatalf("got %q", got)
	}
	if md := orderNotesMetadata(nil, "", false); md != nil {
		t.Fatalf("no notes should leave metadata nil, got %v", md)
	}
}

func TestWithCatalogSnapshot(t *testing.T) {
	id := uuid.New()
	item := &catalog.MergedCatalogItem{
		CategoryName: "Burgers",
		InventoryID:  id,
		Type:         "RECIPE",
		Metadata:     map[string]any{"duration_minutes": 45},
	}
	client := map[string]interface{}{"category": "Tampered", "appointment_time": "10:00"}
	got := withCatalogSnapshot(client, item)
	if got["category"] != "Burgers" {
		t.Fatalf("catalog category must win over the client value, got %v", got["category"])
	}
	if got["inventory_item_id"] != id.String() || got["item_type"] != "RECIPE" {
		t.Fatalf("catalog identity missing: %v", got)
	}
	if got["appointment_time"] != "10:00" {
		t.Fatalf("client booking field lost: %v", got)
	}
	if got["duration_minutes"] != 45 {
		t.Fatalf("duration not copied: %v", got)
	}
	if client["category"] != "Tampered" {
		t.Fatalf("input metadata must not be mutated")
	}
}

func TestAlreadyHandedOff(t *testing.T) {
	if alreadyHandedOff(&Order{}) {
		t.Fatal("fresh order is not handed off")
	}
	if !alreadyHandedOff(&Order{Metadata: map[string]interface{}{metaOutletHandoffAt: "2026-09-25T10:00:00Z"}}) {
		t.Fatal("stamped order should count as handed off")
	}
}
