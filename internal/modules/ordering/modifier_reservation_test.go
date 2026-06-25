package ordering

import (
	"testing"

	"github.com/google/uuid"
)

// TestModifierLinesForCartItem verifies that a cart line's selected modifier options are
// mapped to the inventory reservation/consumption modifier contract (STK-4): each option
// carries its inventory modifier-option id so inventory can resolve it to a consumable SKU
// and deduct modifier stock the same way POS sales do. Price-only options without an id are
// dropped, and a line with no modifiers yields nil.
func TestModifierLinesForCartItem(t *testing.T) {
	optA := uuid.New()
	optB := uuid.New()

	t.Run("no modifiers yields nil", func(t *testing.T) {
		got := modifierLinesForCartItem(CartItem{Quantity: 2})
		if got != nil {
			t.Fatalf("expected nil, got %#v", got)
		}
	})

	t.Run("maps option ids and scales per-unit", func(t *testing.T) {
		ci := CartItem{
			Quantity: 3,
			Modifiers: []CartItemModifier{
				{OptionID: optA, OptionName: "Extra Cheese", PriceAdjustment: 50},
				{OptionID: optB, OptionName: "Large", PriceAdjustment: 30},
			},
		}
		got := modifierLinesForCartItem(ci)
		if len(got) != 2 {
			t.Fatalf("expected 2 modifier lines, got %d (%#v)", len(got), got)
		}
		// Each line carries the option id and a per-unit quantity of 1 (inventory scales by line qty).
		if got[0].InventoryModifierOptionID != optA.String() {
			t.Errorf("line0 option id = %s, want %s", got[0].InventoryModifierOptionID, optA.String())
		}
		if got[0].Quantity != 1 {
			t.Errorf("line0 per-unit qty = %v, want 1", got[0].Quantity)
		}
		if got[1].InventoryModifierOptionID != optB.String() {
			t.Errorf("line1 option id = %s, want %s", got[1].InventoryModifierOptionID, optB.String())
		}
	})

	t.Run("drops options without an option id", func(t *testing.T) {
		ci := CartItem{
			Quantity: 1,
			Modifiers: []CartItemModifier{
				{OptionName: "No Sauce"},          // price-only, no inventory option id
				{OptionID: optA, OptionName: "X"}, // valid
			},
		}
		got := modifierLinesForCartItem(ci)
		if len(got) != 1 {
			t.Fatalf("expected 1 modifier line, got %d (%#v)", len(got), got)
		}
		if got[0].InventoryModifierOptionID != optA.String() {
			t.Errorf("kept wrong option: %s", got[0].InventoryModifierOptionID)
		}
	})
}
