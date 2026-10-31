package ordering

import "testing"

func cartItem(total, rate float64, inclusive bool) CartItem {
	return CartItem{TotalPrice: total, Metadata: map[string]interface{}{"tax_rate": rate, "tax_inclusive": inclusive}}
}

// VAT is rounded on each line; the old float sum was never rounded.
func TestCartTotalsRoundPerLine(t *testing.T) {
	items := []CartItem{cartItem(0.1, 16, false), cartItem(0.1, 16, false), cartItem(0.1, 16, false)}
	sub, tax := cartTotals(items)
	if sub != 0.3 || tax != 0.06 {
		t.Fatalf("subtotal %v tax %v, want 0.30 and 0.06", sub, tax)
	}
	sub, tax = cartTotals([]CartItem{cartItem(116, 16, true), cartItem(300, 0, true)})
	if sub != 400 || tax != 16 {
		t.Fatalf("inclusive cart: subtotal %v tax %v, want 400 and 16", sub, tax)
	}
}

// A coupon comes off the taxable subtotal before VAT.
func TestCartVATAfterDiscount(t *testing.T) {
	c := &Cart{Items: []CartItem{cartItem(85.6, 16, false), cartItem(288, 16, false), cartItem(240, 16, false)}}
	if got := c.VATAfterDiscount(0); got != 98.18 {
		t.Fatalf("no discount: %v, want 98.18", got)
	}
	if got := c.VATAfterDiscount(61.36); got != 88.36 {
		t.Fatalf("10%% off: %v, want 88.36", got)
	}
	if got := (&Cart{TaxTotal: 12.5}).VATAfterDiscount(5); got != 12.5 {
		t.Fatalf("a cart without its lines keeps its stored VAT: %v", got)
	}
}
