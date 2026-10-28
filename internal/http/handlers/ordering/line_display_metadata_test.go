package orderinghandler

import "testing"

func TestLineDisplayMetadata(t *testing.T) {
	if got := lineDisplayMetadata(map[string]interface{}{"guest_session": "x"}); got != nil {
		t.Fatalf("plain line must carry no metadata, got %v", got)
	}
	got := lineDisplayMetadata(map[string]interface{}{
		"variant_attributes": map[string]interface{}{"size": "M"},
		"modifiers":          []interface{}{map[string]interface{}{"option_name": "Extra cheese"}},
		"variant_sku":        "TSHIRT-RED-M",
	})
	if len(got) != 2 || got["modifiers"] == nil || got["variant_attributes"] == nil {
		t.Fatalf("want only the display keys, got %v", got)
	}
}
