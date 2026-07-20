package catalog

import (
	"testing"

	"github.com/bengobox/ordering-backend/internal/platform/inventory"
)

func TestMergeItem_NotForSaleNeverOrderable(t *testing.T) {
	price := 250.0
	inv := inventory.ItemResponse{
		SKU:          "ING-FLOUR-1KG",
		Name:         "Baking Flour 1kg",
		Type:         "GOODS",
		IsActive:     true,
		NotForSale:   true,
		SellingPrice: &price,
	}

	item := mergeItem(inv, nil, map[string]struct{}{})
	if item.IsAvailable {
		t.Errorf("not_for_sale item must never be available, got IsAvailable=true")
	}
	if item.IsComplimentary {
		t.Errorf("not_for_sale item must never be complimentary")
	}
}

func TestMergeItem_SellableItemStaysAvailable(t *testing.T) {
	price := 250.0
	inv := inventory.ItemResponse{
		SKU:          "PIZZA-MARG",
		Name:         "Margherita Pizza",
		Type:         "RECIPE",
		IsActive:     true,
		SellingPrice: &price,
	}

	item := mergeItem(inv, nil, map[string]struct{}{})
	if !item.IsAvailable {
		t.Errorf("active priced item should be available, got IsAvailable=false")
	}
	if item.BasePrice != 250 {
		t.Errorf("BasePrice = %v, want 250", item.BasePrice)
	}
}
