package ordering

import "testing"

func TestOutletLocationPayload(t *testing.T) {
	lat, lng := -1.28, 36.82

	t.Run("no coordinates still sends name address and phone", func(t *testing.T) {
		got := outletLocationPayload(OutletLocation{Name: "Westlands", Address: "Mpaka Rd", Phone: "0700000000"})
		if got == nil {
			t.Fatal("outlet without coordinates was dropped")
		}
		if got["name"] != "Westlands" || got["address"] != "Mpaka Rd" || got["phone"] != "0700000000" {
			t.Fatalf("unexpected payload %v", got)
		}
		if _, ok := got["latitude"]; ok {
			t.Fatal("latitude sent without coordinates")
		}
		if _, ok := got["longitude"]; ok {
			t.Fatal("longitude sent without coordinates")
		}
	})

	t.Run("coordinates sent when both present", func(t *testing.T) {
		got := outletLocationPayload(OutletLocation{Name: "CBD", Latitude: &lat, Longitude: &lng})
		if got["latitude"] != lat || got["longitude"] != lng {
			t.Fatalf("coordinates missing: %v", got)
		}
	})

	t.Run("half a coordinate pair is not sent", func(t *testing.T) {
		got := outletLocationPayload(OutletLocation{Name: "CBD", Latitude: &lat})
		if _, ok := got["latitude"]; ok {
			t.Fatal("latitude sent without longitude")
		}
	})

	t.Run("blank fields are left out", func(t *testing.T) {
		got := outletLocationPayload(OutletLocation{Name: "  ", Phone: "0711"})
		if _, ok := got["name"]; ok {
			t.Fatal("blank name sent")
		}
		if got["phone"] != "0711" {
			t.Fatalf("phone missing: %v", got)
		}
	})

	t.Run("empty outlet gives nil", func(t *testing.T) {
		if got := outletLocationPayload(OutletLocation{}); got != nil {
			t.Fatalf("want nil, got %v", got)
		}
	})
}
