package catalog

import (
	"testing"

	"github.com/google/uuid"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/platform/inventory"
)

func f64(v float64) *float64 { return &v }

func tshirtCatalog(redID, blueID uuid.UUID) []inventory.ItemResponse {
	return []inventory.ItemResponse{
		{ID: uuid.New(), SKU: "MUG-1", Name: "Mug", IsActive: true, SellingPrice: f64(300)},
		{
			ID: uuid.New(), SKU: "TS-1", Name: "T-shirt", Type: "GOODS", IsActive: true,
			CategoryName: "Clothing", SellingPrice: f64(1000), TaxRate: f64(16), NetPrice: f64(862), HasVariants: true,
			Variants: []inventory.ItemVariantResponse{
				{ID: redID, SKU: "TS-1-RED-M", Name: "Red M", Price: 1199.4, IsActive: true,
					Attributes: map[string]string{"size": "M", "colour": "Red"}, OnHand: f64(4), ImageURL: "red.jpg"},
				{ID: blueID, SKU: "TS-1-BLU-L", Name: "Blue L", Price: 0, IsActive: false,
					Attributes: map[string]string{"size": "L", "colour": "Blue"}},
			},
		},
	}
}

func TestFindCatalogEntry(t *testing.T) {
	redID, blueID := uuid.New(), uuid.New()
	items := tshirtCatalog(redID, blueID)

	if got, idx := findCatalogEntry(items, "MUG-1"); got == nil || got.SKU != "MUG-1" || idx != -1 {
		t.Fatalf("direct item: got %v idx %d", got, idx)
	}
	got, idx := findCatalogEntry(items, "  ts-1-red-m ")
	if got == nil || got.SKU != "TS-1" || idx != 0 {
		t.Fatalf("variant lookup (case and space insensitive): got %v idx %d", got, idx)
	}
	if got, _ := findCatalogEntry(items, "NOPE"); got != nil {
		t.Fatalf("unknown SKU should miss, got %v", got.SKU)
	}
}

func TestResolveVariantItem(t *testing.T) {
	redID, blueID := uuid.New(), uuid.New()
	inv := tshirtCatalog(redID, blueID)[1]
	parentOverride := &ent.CatalogOverride{IsAvailable: true}
	parent := mergeItem(inv, parentOverride, nil)

	t.Run("variant price, name, image and stock", func(t *testing.T) {
		item := resolveVariantItem(parent, inv, parentOverride, 0, nil)
		if item.InventorySKU != "TS-1-RED-M" || item.InventoryID != redID || item.ParentSKU != "TS-1" {
			t.Fatalf("identity: sku=%s id=%s parent=%s", item.InventorySKU, item.InventoryID, item.ParentSKU)
		}
		if item.Variant == nil || item.Variant.ID != redID {
			t.Fatalf("variant not set: %#v", item.Variant)
		}
		if item.BasePrice != 1200 {
			t.Errorf("price = %v, want 1200 (variant price rounded up)", item.BasePrice)
		}
		if item.Name != "T-shirt, Red / M" {
			t.Errorf("name = %q", item.Name)
		}
		if item.ImageURL != "red.jpg" {
			t.Errorf("image = %q", item.ImageURL)
		}
		if !item.IsAvailable || item.AvailableQuantity == nil || *item.AvailableQuantity != 4 {
			t.Errorf("availability = %v qty %v", item.IsAvailable, item.AvailableQuantity)
		}
		if item.NetPrice != nil {
			t.Errorf("parent net price must not leak onto the variant")
		}
	})

	t.Run("inactive variant falls back to parent price but is unavailable", func(t *testing.T) {
		item := resolveVariantItem(parent, inv, parentOverride, 1, nil)
		if item.IsAvailable {
			t.Fatal("inactive variant must be unavailable")
		}
		if item.BasePrice != 1000 {
			t.Errorf("price = %v, want parent price 1000", item.BasePrice)
		}
	})

	t.Run("child override is honoured", func(t *testing.T) {
		item := resolveVariantItem(parent, inv, parentOverride, 0, &ent.CatalogOverride{IsAvailable: false})
		if item.IsAvailable {
			t.Fatal("child override switched off must make the variant unavailable")
		}
		item = resolveVariantItem(parent, inv, parentOverride, 0, &ent.CatalogOverride{IsAvailable: true, BasePrice: 1500})
		if item.BasePrice != 1500 || !item.IsAvailable {
			t.Errorf("child override price: %v available %v", item.BasePrice, item.IsAvailable)
		}
	})

	t.Run("zero stock stays orderable (manual availability)", func(t *testing.T) {
		soldOut := inv
		soldOut.Variants = append([]inventory.ItemVariantResponse(nil), inv.Variants...)
		soldOut.Variants[0].OnHand = f64(0)
		p := mergeItem(soldOut, parentOverride, nil)
		item := resolveVariantItem(p, soldOut, parentOverride, 0, nil)
		if !item.IsAvailable {
			t.Fatal("stock must not gate a variant; only its override can switch it off")
		}
		if item.AvailableQuantity != nil {
			t.Fatal("zero on-hand must not surface as a \"0 left\" badge")
		}
	})

	t.Run("parent switched off hides every variant", func(t *testing.T) {
		off := &ent.CatalogOverride{IsAvailable: false}
		if resolveVariantItem(mergeItem(inv, off, nil), inv, off, 0, nil).IsAvailable {
			t.Fatal("variant of an unavailable parent must be unavailable")
		}
	})
}

func TestVariantDisplayName(t *testing.T) {
	cases := []struct {
		v    CatalogVariant
		want string
	}{
		{CatalogVariant{Attributes: map[string]string{"size": "M", "colour": "Red"}}, "T-shirt, Red / M"},
		{CatalogVariant{Attributes: map[string]string{"material": "Cotton", "size": "S"}}, "T-shirt, S / Cotton"},
		{CatalogVariant{Name: "T-shirt XL"}, "T-shirt XL"},
		{CatalogVariant{Name: "XL"}, "T-shirt, XL"},
		{CatalogVariant{}, "T-shirt"},
	}
	for _, c := range cases {
		if got := VariantDisplayName("T-shirt", c.v); got != c.want {
			t.Errorf("VariantDisplayName(%v) = %q, want %q", c.v, got, c.want)
		}
	}
}
