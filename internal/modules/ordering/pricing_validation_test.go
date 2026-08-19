package ordering

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/bengobox/ordering-backend/internal/modules/catalog"
)

// TestFindModifierOption verifies the (groupID, optionID) lookup against an item's real
// modifier groups — the core of the price-tampering fix: a submitted modifier selection is
// only trusted if it matches a real option on the item.
func TestFindModifierOption(t *testing.T) {
	groupID := uuid.New()
	optionID := uuid.New()
	groups := []catalog.CatalogModifierGroup{
		{
			ID:   groupID.String(),
			Name: "Toppings",
			Options: []catalog.CatalogModifierOption{
				{ID: optionID.String(), Name: "Extra Cheese", PriceAdjustment: 50},
			},
		},
	}

	t.Run("finds a real option", func(t *testing.T) {
		got := findModifierOption(groups, groupID, optionID)
		if got == nil {
			t.Fatal("expected a match, got nil")
		}
		if got.PriceAdjustment != 50 {
			t.Errorf("price = %v, want 50", got.PriceAdjustment)
		}
	})

	t.Run("rejects an unknown option id (tampered/stale selection)", func(t *testing.T) {
		got := findModifierOption(groups, groupID, uuid.New())
		if got != nil {
			t.Fatalf("expected no match, got %#v", got)
		}
	})

	t.Run("rejects an unknown group id", func(t *testing.T) {
		got := findModifierOption(groups, uuid.New(), optionID)
		if got != nil {
			t.Fatalf("expected no match, got %#v", got)
		}
	})
}

// TestValidateAndPriceItems_NoCatalogService verifies the fail-open path when catalogSvc
// isn't wired (e.g. a lightweight test harness) — items must pass through completely
// unchanged rather than panicking on a nil dependency.
func TestValidateAndPriceItems_NoCatalogService(t *testing.T) {
	s := &OrderService{}
	items := []CreateOrderItemInput{
		{InventorySKU: "SKU-1", UnitPrice: 999, TotalPrice: 999, Quantity: 1},
	}
	got, err := s.validateAndPriceItems(context.Background(), "tenant-slug", uuid.New(), items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].UnitPrice != 999 {
		t.Fatalf("expected items unchanged when catalogSvc is nil, got %#v", got)
	}
}
