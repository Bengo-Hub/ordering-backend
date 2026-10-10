package orderinghandler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bengobox/ordering-backend/internal/modules/ordering"
)

func TestRespondDeliveryError(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"outside", &ordering.DeliveryNotServiceableError{Reason: "outside_delivery_area", NearestArea: "Malaba", NearestAreaKm: 6.1}, 422, "delivery_not_serviceable"},
		{"wrapped outside", fmt.Errorf("checkout: %w", &ordering.DeliveryNotServiceableError{Reason: "excluded_area"}), 422, "delivery_not_serviceable"},
		{"below minimum", &ordering.BelowDeliveryMinimumError{Area: "Malaba", MinOrder: 1500}, 422, "below_delivery_minimum"},
		{"no pin", ordering.ErrDeliveryLocationRequired, 422, "delivery_location_required"},
		{"logistics down", ordering.ErrDeliveryPricingUnavailable, 503, "delivery_pricing_unavailable"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !isDeliveryPricingError(c.err) {
				t.Fatalf("%v should be a delivery pricing error", c.err)
			}
			rec := httptest.NewRecorder()
			if !respondDeliveryError(rec, c.err) {
				t.Fatal("expected the error to be handled")
			}
			if rec.Code != c.status {
				t.Fatalf("status = %d, want %d", rec.Code, c.status)
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body["code"] != c.code || body["error"] == "" {
				t.Fatalf("body = %v", body)
			}
		})
	}

	// Details the checkout shows to the customer.
	rec := httptest.NewRecorder()
	respondDeliveryError(rec, &ordering.DeliveryNotServiceableError{Reason: "outside_delivery_area", NearestArea: "Malaba", NearestAreaKm: 6.1})
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["nearest_area"] != "Malaba" || body["reason"] != "outside_delivery_area" {
		t.Fatalf("nearest area missing: %v", body)
	}

	if isDeliveryPricingError(errors.New("something else")) {
		t.Fatal("unrelated errors must fall through to the generic handler")
	}
	rec = httptest.NewRecorder()
	if respondDeliveryError(rec, errors.New("other")) || rec.Code != http.StatusInternalServerError {
		t.Fatalf("unrelated error: handled=%v status=%d", false, rec.Code)
	}
}
