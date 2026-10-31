package ordering

import vat "github.com/Bengo-Hub/httpware/tax"

// cartTaxDocument builds the shared calculator's document from cart lines. Each line's rate and
// inclusive flag were snapshotted from the catalog (inventory, rates from treasury) at add-to-cart.
// The discount is taken off the taxable subtotal, the same base computeFees takes it off.
func cartTaxDocument(items []CartItem, discount float64) vat.Document {
	doc := vat.Document{Discount: discount, DiscountBase: vat.DiscountOnNet, Lines: make([]vat.Line, len(items))}
	for i, it := range items {
		rate, _ := it.Metadata["tax_rate"].(float64)
		inclusive, _ := it.Metadata["tax_inclusive"].(bool)
		// TotalPrice already includes modifiers: (unit price + modifier total) x quantity.
		doc.Lines[i] = vat.Line{Quantity: 1, UnitPrice: it.TotalPrice, Rate: rate, Inclusive: inclusive}
	}
	return doc
}

// cartTotals works out a cart's taxable subtotal and VAT with the platform's shared calculator:
// VAT rounded on each line, totals summed from the rounded lines.
func cartTotals(items []CartItem) (subtotal, taxTotal float64) {
	res := vat.Compute(cartTaxDocument(items, 0))
	return res.Net, res.Tax
}

// VATAfterDiscount is the cart's VAT once a discount (coupon, promotion or loyalty) is taken off
// the taxable subtotal before VAT, as the VAT Act requires for a discount given at the time of
// sale. A cart loaded without its lines falls back to the stored TaxTotal.
func (c *Cart) VATAfterDiscount(discount float64) float64 {
	if len(c.Items) == 0 {
		return c.TaxTotal
	}
	if discount <= 0 {
		_, tax := cartTotals(c.Items)
		return tax
	}
	return vat.Compute(cartTaxDocument(c.Items, discount)).Tax
}
