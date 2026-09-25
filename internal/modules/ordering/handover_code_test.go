package ordering

import "testing"

func TestNeedsHandoverCode(t *testing.T) {
	product := map[string]interface{}{"inventory_item_id": "x"}
	haircut := map[string]interface{}{"is_service": true}
	ticket := map[string]interface{}{"is_ticket": true}

	cases := []struct {
		name  string
		ft    FulfillmentType
		lines []map[string]interface{}
		want  bool
	}{
		{"delivery", FulfillmentTypeDelivery, []map[string]interface{}{product}, true},
		{"legacy scheduled delivery", FulfillmentTypeScheduled, nil, true},
		{"pickup with goods", FulfillmentTypePickup, []map[string]interface{}{product}, true},
		{"pickup with no line metadata", FulfillmentTypePickup, []map[string]interface{}{nil}, true},
		{"pickup mixing a service and goods", FulfillmentTypePickup, []map[string]interface{}{haircut, product}, true},
		{"service booking only", FulfillmentTypePickup, []map[string]interface{}{haircut}, false},
		{"tickets only", FulfillmentTypePickup, []map[string]interface{}{ticket, ticket}, false},
		{"dine in", FulfillmentTypeDineIn, []map[string]interface{}{product}, false},
	}
	for _, tc := range cases {
		if got := needsHandoverCode(tc.ft, tc.lines); got != tc.want {
			t.Errorf("%s: needsHandoverCode = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestHandoverCodeRouting(t *testing.T) {
	delivery := &Order{FulfillmentType: FulfillmentTypeDelivery, PODCode: "123456"}
	pickup := &Order{FulfillmentType: FulfillmentTypePickup, PODCode: "654321"}

	if deliveryCode(delivery) != "123456" || collectionCode(delivery) != "" {
		t.Fatalf("delivery code must only go out as the rider code")
	}
	// A pickup code must never reach messages that say "give this code to the rider".
	if deliveryCode(pickup) != "" || collectionCode(pickup) != "654321" {
		t.Fatalf("pickup code must only go out as the collection code")
	}
	if deliveryCode(nil) != "" || collectionCode(nil) != "" {
		t.Fatalf("nil order must give no code")
	}
}
