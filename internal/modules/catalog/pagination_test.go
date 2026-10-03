package catalog

import "testing"

// TestPageOfFullPages pins that storefront pages are cut from the FILTERED result: every page
// but the last is full, and the total counts only visible items.
func TestPageOfFullPages(t *testing.T) {
	items := make([]MergedCatalogItem, 318)
	for i := range items {
		items[i].InventorySKU = string(rune('A' + i%26))
	}
	p1, total := pageOf(items, 0, 100)
	if len(p1) != 100 || total != 318 {
		t.Fatalf("page 1: got %d items, total %d; want 100, 318", len(p1), total)
	}
	p4, _ := pageOf(items, 300, 100)
	if len(p4) != 18 {
		t.Fatalf("last page: got %d items, want 18", len(p4))
	}
	past, _ := pageOf(items, 400, 100)
	if past == nil || len(past) != 0 {
		t.Fatalf("past the end must be an empty non-nil page, got %v", past)
	}
	neg, _ := pageOf(items, -5, 10)
	if len(neg) != 10 {
		t.Fatalf("negative offset must clamp to 0, got %d items", len(neg))
	}
}
