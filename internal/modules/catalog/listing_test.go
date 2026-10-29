package catalog

import (
	"testing"

	"github.com/google/uuid"
)

func listingFixture() []MergedCatalogItem {
	nike := uuid.New()
	shirt := MergedCatalogItem{
		Name: "T-shirt", BasePrice: 0, CategoryName: "Clothes", BrandName: "Nike", BrandID: &nike,
		Variants: []CatalogVariant{
			{Price: 900, IsActive: true, Attributes: map[string]string{"size": "M", "colour": "Red"}},
			{Price: 1100, IsActive: true, Attributes: map[string]string{"size": "L", "colour": "Red"}},
			{Price: 50, IsActive: false, Attributes: map[string]string{"size": "XS", "colour": "Blue"}},
		},
	}
	setPriceRange(&shirt)
	return []MergedCatalogItem{
		{Name: "Cap", BasePrice: 400, CategoryName: "Clothes", BrandName: "nike", BrandID: &nike},
		shirt,
		{Name: "Kettle", BasePrice: 2500, CategoryName: "Home"},
		{Name: "Gift wrap", BasePrice: 0, CategoryName: "Home"},
	}
}

func names(items []MergedCatalogItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Name
	}
	return out
}

func TestSetPriceRangeSkipsInactiveVariants(t *testing.T) {
	shirt := listingFixture()[1]
	if shirt.PriceFrom == nil || *shirt.PriceFrom != 900 || *shirt.PriceTo != 1100 {
		t.Fatalf("range = %v..%v, want 900..1100", shirt.PriceFrom, shirt.PriceTo)
	}
	plain := MergedCatalogItem{BasePrice: 400}
	setPriceRange(&plain)
	if plain.PriceFrom != nil {
		t.Fatal("a product without variants has no range")
	}
}

func TestSortItems(t *testing.T) {
	items := listingFixture()
	sortItems(items, SortPriceAsc)
	if got := names(items); got[0] != "Gift wrap" || got[1] != "Cap" || got[2] != "T-shirt" || got[3] != "Kettle" {
		t.Fatalf("price_asc = %v", got)
	}
	sortItems(items, SortPriceDesc)
	if got := names(items); got[0] != "Kettle" || got[1] != "T-shirt" {
		t.Fatalf("price_desc = %v", got)
	}
	sortItems(items, SortName)
	if got := names(items); got[0] != "Cap" || got[3] != "T-shirt" {
		t.Fatalf("name = %v", got)
	}
	if inventorySort(SortPriceAsc) != "" || inventorySort(SortNewest) != SortNewest {
		t.Fatal("only newest goes to inventory-api")
	}
}

func TestFilterByPriceUsesFromPrice(t *testing.T) {
	lo, hi := 800.0, 1000.0
	got := names(filterByPrice(listingFixture(), &lo, &hi))
	if len(got) != 1 || got[0] != "T-shirt" {
		t.Fatalf("800..1000 = %v, want the T-shirt (from 900)", got)
	}
	if n := len(filterByPrice(listingFixture(), nil, nil)); n != 4 {
		t.Fatalf("no bounds kept %d of 4", n)
	}
}

func TestAttributeFilter(t *testing.T) {
	want := ParseAttributeFilter(" Size : m , colour:RED, bad, :x")
	if len(want) != 2 || want["size"] != "m" || want["colour"] != "red" {
		t.Fatalf("parsed %v", want)
	}
	got := names(filterByAttributes(listingFixture(), want))
	if len(got) != 1 || got[0] != "T-shirt" {
		t.Fatalf("size M in red = %v, want the T-shirt", got)
	}
	if n := len(filterByAttributes(listingFixture(), map[string]string{"colour": "blue"})); n != 0 {
		t.Fatalf("blue exists only on an inactive variant, kept %d", n)
	}
	if n := len(filterByAttributes(listingFixture(), nil)); n != 4 {
		t.Fatalf("no filter kept %d of 4", n)
	}
}

func TestBuildFacets(t *testing.T) {
	f := buildFacets(listingFixture())
	if f.PriceMin != 400 || f.PriceMax != 2500 {
		t.Fatalf("price bounds %v..%v, want 400..2500 (free item ignored)", f.PriceMin, f.PriceMax)
	}
	if len(f.Brands) != 1 || f.Brands[0].Count != 2 {
		t.Fatalf("brands = %+v, want Nike x2 merged without case", f.Brands)
	}
	if len(f.Categories) != 2 || f.Categories[0].Name != "Clothes" || f.Categories[0].Count != 2 {
		t.Fatalf("categories = %+v", f.Categories)
	}
	if c := f.Attributes["colour"]; len(c) != 1 || c[0].Name != "Red" || c[0].Count != 1 {
		t.Fatalf("colour = %+v, want Red counted once per product, inactive Blue left out", c)
	}
	if s := f.Attributes["size"]; len(s) != 2 {
		t.Fatalf("size = %+v, want L and M", s)
	}
}
