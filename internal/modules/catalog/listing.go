package catalog

import (
	"cmp"
	"math"
	"slices"
	"strings"
)

// Storefront sort keys. Only SortNewest is passed on to inventory-api (it orders by creation
// time there); the rest are applied here, over the merged set ListItems already holds, so
// every page is sorted across the whole catalogue instead of one fetched page.
const (
	SortNewest    = "newest"
	SortPriceAsc  = "price_asc"
	SortPriceDesc = "price_desc"
	SortName      = "name"
)

// inventorySort is the sort key forwarded to inventory-api. Everything but "newest" is applied
// locally, so it maps to the default order and shares one cached candidate set.
func inventorySort(sort string) string {
	if sort == SortNewest {
		return SortNewest
	}
	return ""
}

// FacetCount is one value of a listing facet with the number of products that carry it.
type FacetCount struct {
	ID    string `json:"id,omitempty"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// CatalogFacets summarises the products matching every filter except the price range, so the
// storefront can draw its filter panel (price slider bounds, brand and category lists with
// counts, variant sizes and colours) from the same request that returns the page.
type CatalogFacets struct {
	PriceMin   float64                 `json:"priceMin"`
	PriceMax   float64                 `json:"priceMax"`
	Brands     []FacetCount            `json:"brands"`
	Categories []FacetCount            `json:"categories"`
	Attributes map[string][]FacetCount `json:"attributes,omitempty"`
}

// CatalogPage is one page of the storefront listing plus its facets (nil unless requested).
type CatalogPage struct {
	Items  []MergedCatalogItem
	Total  int
	Facets *CatalogFacets
}

// setPriceRange fills PriceFrom/PriceTo for a product with priced, active variants, which is
// what the card shows as "From KSh x" and the product page as "KSh x - KSh y". A product without
// variants keeps both nil and is shown at BasePrice.
func setPriceRange(item *MergedCatalogItem) {
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, v := range item.Variants {
		if !v.IsActive || v.Price <= 0 {
			continue
		}
		lo, hi = min(lo, v.Price), max(hi, v.Price)
	}
	if math.IsInf(lo, 1) {
		return
	}
	item.PriceFrom, item.PriceTo = &lo, &hi
}

// listPrice is the price a listing sorts and filters on: the lowest variant price when the
// product has variants, else its own price.
func listPrice(item MergedCatalogItem) float64 {
	if item.PriceFrom != nil {
		return *item.PriceFrom
	}
	return item.BasePrice
}

// filterByPrice keeps products whose list price falls inside [minPrice, maxPrice]; a nil bound
// is open.
func filterByPrice(items []MergedCatalogItem, minPrice, maxPrice *float64) []MergedCatalogItem {
	if minPrice == nil && maxPrice == nil {
		return items
	}
	return slices.DeleteFunc(items, func(it MergedCatalogItem) bool {
		p := listPrice(it)
		return (minPrice != nil && p < *minPrice) || (maxPrice != nil && p > *maxPrice)
	})
}

// sortItems orders the merged set in place for the locally applied sort keys. Ties fall back to
// the name so pages stay stable between requests. Unknown keys keep inventory's order.
func sortItems(items []MergedCatalogItem, sort string) {
	byName := func(a, b MergedCatalogItem) int {
		return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	}
	switch sort {
	case SortPriceAsc:
		slices.SortStableFunc(items, func(a, b MergedCatalogItem) int {
			return cmp.Or(cmp.Compare(listPrice(a), listPrice(b)), byName(a, b))
		})
	case SortPriceDesc:
		slices.SortStableFunc(items, func(a, b MergedCatalogItem) int {
			return cmp.Or(cmp.Compare(listPrice(b), listPrice(a)), byName(a, b))
		})
	case SortName:
		slices.SortStableFunc(items, byName)
	}
}

// buildFacets counts brands, categories and variant attribute values over items. Price bounds
// skip free (price 0) products so a complimentary item never drags the slider to 0.
func buildFacets(items []MergedCatalogItem) *CatalogFacets {
	f := &CatalogFacets{Brands: []FacetCount{}, Categories: []FacetCount{}}
	brands := map[string]*FacetCount{}
	categories := map[string]*FacetCount{}
	attrs := map[string]map[string]*FacetCount{}
	lo, hi := math.Inf(1), 0.0

	bump := func(m map[string]*FacetCount, key, id, name string) {
		if c := m[key]; c != nil {
			c.Count++
			return
		}
		m[key] = &FacetCount{ID: id, Name: name, Count: 1}
	}

	for _, it := range items {
		if p := listPrice(it); p > 0 {
			lo = min(lo, p)
			hi = max(hi, p)
		}
		if it.PriceTo != nil {
			hi = max(hi, *it.PriceTo)
		}
		if name := strings.TrimSpace(it.BrandName); name != "" {
			id := ""
			if it.BrandID != nil {
				id = it.BrandID.String()
			}
			bump(brands, strings.ToLower(name), id, name)
		}
		if name := strings.TrimSpace(it.CategoryName); name != "" {
			id := ""
			if it.CategoryID != nil {
				id = it.CategoryID.String()
			}
			bump(categories, strings.ToLower(name), id, name)
		}
		// A product counts once per attribute value however many of its variants carry it.
		seen := map[string]struct{}{}
		for _, v := range it.Variants {
			if !v.IsActive {
				continue
			}
			for axis, value := range v.Attributes {
				axis, value = strings.ToLower(strings.TrimSpace(axis)), strings.TrimSpace(value)
				if axis == "" || value == "" {
					continue
				}
				key := axis + "\x00" + strings.ToLower(value)
				if _, dup := seen[key]; dup {
					continue
				}
				seen[key] = struct{}{}
				if attrs[axis] == nil {
					attrs[axis] = map[string]*FacetCount{}
				}
				bump(attrs[axis], strings.ToLower(value), "", value)
			}
		}
	}

	if !math.IsInf(lo, 1) {
		f.PriceMin, f.PriceMax = lo, hi
	}
	f.Brands = sortedCounts(brands)
	f.Categories = sortedCounts(categories)
	if len(attrs) > 0 {
		f.Attributes = make(map[string][]FacetCount, len(attrs))
		for axis, m := range attrs {
			f.Attributes[axis] = sortedCounts(m)
		}
	}
	return f
}

// sortedCounts lists a facet's values by count (most first), then name.
func sortedCounts(m map[string]*FacetCount) []FacetCount {
	out := make([]FacetCount, 0, len(m))
	for _, c := range m {
		out = append(out, *c)
	}
	slices.SortFunc(out, func(a, b FacetCount) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)))
	})
	return out
}
