package catalog

import "testing"

// TestAffectsAvailability pins the rolling-deploy contract with inventory-api: only an
// explicit false makes a stock event alert-only; events published before the policy existed
// (no field) keep their original availability meaning.
func TestAffectsAvailability(t *testing.T) {
	cases := []struct {
		name    string
		payload map[string]interface{}
		want    bool
	}{
		{"legacy event, no field", map[string]interface{}{"sku": "X"}, true},
		{"opted-in tenant", map[string]interface{}{"affects_availability": true}, true},
		{"manual-only tenant", map[string]interface{}{"affects_availability": false}, false},
		{"malformed value treated as legacy", map[string]interface{}{"affects_availability": "no"}, true},
	}
	for _, c := range cases {
		if got := affectsAvailability(c.payload); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
