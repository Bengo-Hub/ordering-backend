package catalog

import "testing"

// TestEventQuantity verifies the quantity-aware projection (STK-5) extraction precedence from
// inventory stock-event payloads: available > on_hand > quantity_after, nil when none present.
// JSON numbers decode as float64, which is what the NATS envelope yields.
func TestEventQuantity(t *testing.T) {
	tests := []struct {
		name    string
		payload map[string]interface{}
		want    *float64
	}{
		{"prefers available", map[string]interface{}{"available": 3.0, "on_hand": 5.0}, fptr(3)},
		{"falls back to on_hand", map[string]interface{}{"on_hand": 5.0}, fptr(5)},
		{"falls back to quantity_after", map[string]interface{}{"quantity_after": 7.0}, fptr(7)},
		{"zero is a real value", map[string]interface{}{"available": 0.0}, fptr(0)},
		{"nil when absent", map[string]interface{}{"sku": "X"}, nil},
		{"nil when wrong type", map[string]interface{}{"available": "lots"}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := eventQuantity(tc.payload)
			switch {
			case tc.want == nil && got != nil:
				t.Fatalf("want nil, got %v", *got)
			case tc.want != nil && got == nil:
				t.Fatalf("want %v, got nil", *tc.want)
			case tc.want != nil && got != nil && *got != *tc.want:
				t.Fatalf("want %v, got %v", *tc.want, *got)
			}
		})
	}
}

func TestGetString(t *testing.T) {
	p := map[string]interface{}{"outlet_id": "abc", "n": 5}
	if getString(p, "outlet_id") != "abc" {
		t.Error("expected abc")
	}
	if getString(p, "missing") != "" {
		t.Error("expected empty for missing key")
	}
	if getString(p, "n") != "" {
		t.Error("expected empty for non-string value")
	}
}

func fptr(f float64) *float64 { return &f }
