package catalog

import (
	"context"
	"fmt"
	sharedcache "github.com/Bengo-Hub/cache"
	"github.com/Bengo-Hub/httpware/objectstore"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/catalogoverride"
	"github.com/bengobox/ordering-backend/internal/ent/outlet"
	"github.com/bengobox/ordering-backend/internal/ent/predicate"
	"github.com/bengobox/ordering-backend/internal/ent/userfavorite"
	"github.com/bengobox/ordering-backend/internal/platform/inventory"
)

// Cache TTL tiers for catalog data.
const (
	cacheTTLReference = 5 * time.Minute // semi-static: categories, outlets
	cacheTTLModerate  = 1 * time.Minute // moderate-change: merged items
)

// ProxyService provides catalog operations by merging inventory-api data
// with local CatalogOverride entries and UserFavorite rows.
type ProxyService struct {
	db              *ent.Client
	inventoryClient *inventory.Client
	cache           *sharedcache.Aside
	logger          *zap.Logger
	// delivery prices deliveries for outlet listings (logistics-api).
	delivery DeliveryInfoSource
	// media is the shared object storage (nil: stored values are returned as they are).
	media *objectstore.Store
}

// SetMedia wires the shared object storage that holds override images (keys) and resolves stored
// image values to links.
func (s *ProxyService) SetMedia(m *objectstore.Store) { s.media = m }

// releaseOverrideImage deletes a replaced override image once no other override of the tenant
// shows it (only ordering's own files; inventory links and external URLs are never deleted).
func (s *ProxyService) releaseOverrideImage(ctx context.Context, tenantID uuid.UUID, old, current string) {
	if old == "" || old == current || !s.media.Owned(old) {
		return
	}
	key := s.media.Normalize(old)
	used, err := s.db.CatalogOverride.Query().
		Where(catalogoverride.TenantID(tenantID), catalogoverride.ImageURLOverride(key)).Exist(ctx)
	if err != nil || used {
		return
	}
	if err := s.media.Delete(ctx, key); err != nil {
		s.logger.Warn("replaced override image not deleted from storage", zap.String("key", key), zap.Error(err))
	}
}

// NewProxyService creates a new proxy catalog service.
func NewProxyService(db *ent.Client, inventoryClient *inventory.Client, cacheSvc *sharedcache.Aside, logger *zap.Logger) *ProxyService {
	return &ProxyService{
		db:              db,
		inventoryClient: inventoryClient,
		cache:           cacheSvc,
		logger:          logger.Named("catalog.proxy"),
	}
}

// ---------- Items ----------

// ListItems fetches items from inventory-api, merges with local overrides,
// and attaches favorite status if userID is provided.
func (s *ProxyService) ListItems(ctx context.Context, tenantSlug string, tenantID uuid.UUID, filter CatalogFilter) ([]MergedCatalogItem, int, error) {
	page, err := s.ListItemsPage(ctx, tenantSlug, tenantID, filter)
	if err != nil {
		return nil, 0, err
	}
	return page.Items, page.Total, nil
}

// ListItemsPage is ListItems plus the price range filter, the locally applied sort keys and,
// when filter.WithFacets is set, the facets of the matching set (counted before the price
// range is applied, so the slider always spans the whole selection).
func (s *ProxyService) ListItemsPage(ctx context.Context, tenantSlug string, tenantID uuid.UUID, filter CatalogFilter) (CatalogPage, error) {
	// 1. Fetch the full candidate set (type/category/brand/sort applied server-side), then merge,
	// filter and only THEN paginate. Most storefront filters (not_for_sale, sellable category,
	// availability, featured, search, tags, section) can only be evaluated after merging with
	// local overrides, so paginating the inventory fetch first returned short, uneven pages
	// (urban-loft: 84 of 100 on page 1), a total that counted hidden items, and searches that
	// only looked at the current page. The candidate set is cached briefly (inventoryCatalog);
	// availability comes from overrides, which are never cached, so toggles show immediately.
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := max(filter.Offset, 0)
	invItems, err := s.inventoryCatalog(ctx, tenantSlug, filter.ItemType, filter.CategoryID, inventorySort(filter.Sort), filter.BrandID)
	if err != nil {
		return CatalogPage{}, fmt.Errorf("catalog: list inventory items: %w", err)
	}

	// 2. Load overrides for this tenant, scoped to a real outlet — NEVER unscoped. A historical
	// bug let this query load every outlet_id ever recorded against the tenant with no ordering,
	// so when a SKU had override rows under more than one outlet_id (including, for at least one
	// tenant, a stale/orphaned ID that happened to collide with a DIFFERENT tenant's outlet UUID),
	// whichever row Postgres returned last silently won in the per-SKU map below — non-deterministic
	// and, in that collision case, actively wrong data. Always constrain to the tenant's own real
	// outlets (or the single one the caller asked for) instead.
	realOutletIDs, err := s.tenantOutletIDs(ctx, tenantID)
	if err != nil {
		return CatalogPage{}, fmt.Errorf("catalog: load outlets: %w", err)
	}
	overrideQuery := s.db.CatalogOverride.Query().
		Where(catalogoverride.TenantID(tenantID))
	if filter.OutletID != nil {
		overrideQuery = overrideQuery.Where(catalogoverride.OutletID(*filter.OutletID))
	} else if len(realOutletIDs) > 0 {
		overrideQuery = overrideQuery.Where(catalogoverride.OutletIDIn(realOutletIDs...))
	}
	overrides, err := overrideQuery.All(ctx)
	if err != nil {
		return CatalogPage{}, fmt.Errorf("catalog: load overrides: %w", err)
	}
	overrideMap := make(map[string]*ent.CatalogOverride, len(overrides))
	for _, o := range overrides {
		overrideMap[o.InventorySku] = o
	}

	// 3. Load user favorites (if authenticated)
	favSet := make(map[string]struct{})
	if filter.UserID != nil {
		favs, favErr := s.db.UserFavorite.Query().
			Where(
				userfavorite.TenantID(tenantID),
				userfavorite.UserID(*filter.UserID),
			).All(ctx)
		if favErr == nil {
			for _, f := range favs {
				favSet[f.InventorySku] = struct{}{}
			}
		}
	}

	// 4. Merge — all inventory items appear; override enriches them when present.
	// Initialise non-nil so an empty result serializes as [] (not null → "couldn't load catalog").
	merged := make([]MergedCatalogItem, 0, len(invItems))
	for _, inv := range invItems {
		// not_for_sale = purchasable-only stock (e.g. ingredients): never listed on the storefront.
		if inv.NotForSale {
			continue
		}
		// Defense in depth against mistyped/mistagged raw materials: not_for_sale is the
		// authoritative flag, but staff sometimes create recipe-input items as GOODS
		// instead of INGREDIENT (e.g. a "Raw Ingredients"-category item never flagged
		// not_for_sale) — the inventory-api type filter alone won't catch that. This
		// mirrors pos-api's per-item categoryAllowedForUseCase gate and, unlike the
		// ListCategories nav filter alone, also protects the general item browse/search
		// listing (a customer could otherwise still find "Cabbage" via search even
		// though its category never appears in the category nav).
		if !isStorefrontSellableCategory(inv.CategoryName) {
			continue
		}
		override := overrideMap[inv.SKU] // nil when no override exists — mergeItem handles it
		item := mergeItem(inv, override, favSet)
		item.ImageURL = s.media.URL(item.ImageURL) // an override image is a key; inventory's are links

		// Apply filters
		if filter.ItemType != "" && !strings.EqualFold(item.Type, filter.ItemType) {
			continue
		}
		if len(filter.Tags) > 0 {
			tagSet := make(map[string]struct{}, len(item.Tags))
			for _, t := range item.Tags {
				tagSet[strings.ToLower(t)] = struct{}{}
			}
			skip := false
			for _, required := range filter.Tags {
				if _, ok := tagSet[strings.ToLower(required)]; !ok {
					skip = true
					break
				}
			}
			if skip {
				continue
			}
		}
		if filter.Search != "" {
			q := strings.ToLower(filter.Search)
			if !strings.Contains(strings.ToLower(item.Name), q) &&
				!strings.Contains(strings.ToLower(item.Description), q) &&
				!strings.Contains(strings.ToLower(item.InventorySKU), q) {
				continue
			}
		}
		if filter.IsFeatured != nil && item.IsFeatured != *filter.IsFeatured {
			continue
		}
		if filter.IsAvailable != nil && item.IsAvailable != *filter.IsAvailable {
			continue
		}
		if filter.Section != "" && item.DisplaySection != filter.Section {
			continue
		}
		// No category re-check here: inventory-api already applied filter.CategoryID, including
		// its sub-categories (a "Wines" section lists Red/White/House Wine items). An exact-match
		// check would drop every item that lives in a sub-section.
		if filter.BrandID != nil && (item.BrandID == nil || *item.BrandID != *filter.BrandID) {
			continue
		}
		if len(filter.SKUs) > 0 && !containsFold(filter.SKUs, item.InventorySKU) {
			continue
		}
		if len(filter.CategoryNames) > 0 && !containsFold(filter.CategoryNames, item.CategoryName) {
			continue
		}

		merged = append(merged, item)
	}

	var facets *CatalogFacets
	if filter.WithFacets {
		facets = buildFacets(merged)
	}
	merged = filterByPrice(merged, filter.MinPrice, filter.MaxPrice)
	sortItems(merged, filter.Sort)

	// Paginate the filtered result so every page is full and the total counts only what a
	// customer can actually see.
	pageItems, total := pageOf(merged, offset, limit)
	return CatalogPage{Items: pageItems, Total: total, Facets: facets}, nil
}

// containsFold reports whether list holds v, ignoring case and surrounding spaces.
func containsFold(list []string, v string) bool {
	v = strings.TrimSpace(v)
	return v != "" && slices.ContainsFunc(list, func(s string) bool { return strings.EqualFold(strings.TrimSpace(s), v) })
}

// pageOf slices one page out of an already-filtered result and returns it with the full
// count. Out-of-range offsets yield an empty (non-nil) page so the JSON is [] not null.
func pageOf(items []MergedCatalogItem, offset, limit int) ([]MergedCatalogItem, int) {
	total := len(items)
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || offset >= total {
		return []MergedCatalogItem{}, total
	}
	return items[offset:min(offset+limit, total)], total
}

// inventoryCatalog returns every active inventory item for the scope, paging through
// inventory-api (its limit is capped at inventoryPageSize) and caching the result for
// cacheTTLModerate so a storefront listing or item lookup doesn't re-fetch the catalog on every
// request. Only inventory data is cached; overrides (availability, price, featured) are always
// read live.
func (s *ProxyService) inventoryCatalog(ctx context.Context, tenantSlug, itemType string, categoryID *uuid.UUID, sort string, brandID *uuid.UUID) ([]inventory.ItemResponse, error) {
	if s.cache == nil {
		return s.fetchAllInventoryItems(ctx, tenantSlug, itemType, categoryID, sort, brandID)
	}
	idKey := func(id *uuid.UUID) string {
		if id == nil {
			return "-"
		}
		return id.String()
	}
	key := fmt.Sprintf("ordering:invitems:%s:%s:%s:%s:%s", tenantSlug, itemType, idKey(categoryID), idKey(brandID), sort)
	return sharedcache.GetOrSet(ctx, s.cache, key, cacheTTLModerate, func(ctx context.Context) ([]inventory.ItemResponse, error) {
		return s.fetchAllInventoryItems(ctx, tenantSlug, itemType, categoryID, sort, brandID)
	})
}

// inventoryPageSize matches inventory-api's shared pagination.MaxLimit. Any larger requested
// limit is silently clamped server-side, so paging must use exactly this size.
const inventoryPageSize = 100

// fetchAllInventoryItems pages through inventory-api's item listing until every row is retrieved.
// A single request with a large limit is silently truncated to inventoryPageSize server-side —
// callers that need the full catalog (e.g. scanning for featured items scattered across it) must
// page through it explicitly.
func (s *ProxyService) fetchAllInventoryItems(ctx context.Context, tenantSlug, itemType string, categoryID *uuid.UUID, sort string, brandID *uuid.UUID) ([]inventory.ItemResponse, error) {
	items := make([]inventory.ItemResponse, 0, 2*inventoryPageSize)
	for page := 1; page <= 50; page++ { // runaway guard: 50 pages = 5k items
		pageItems, total, err := s.inventoryClient.ListItems(ctx, tenantSlug, itemType, inventoryPageSize, page, categoryID, sort, brandID)
		if err != nil {
			return nil, err
		}
		items = append(items, pageItems...)
		if len(pageItems) < inventoryPageSize || len(items) >= total {
			break
		}
	}
	return items, nil
}

// GetItem fetches a single item from inventory-api by SKU, merges with override.
func (s *ProxyService) GetItem(ctx context.Context, tenantSlug string, tenantID uuid.UUID, sku string, userID *uuid.UUID) (*MergedCatalogItem, error) {
	// Locate the item by SKU across the WHOLE orderable catalog. This used to read only the
	// first inventory page (100 rows): on a larger catalog (urban-loft has 400+) every item past
	// row 100 returned not-found, so its detail page 404'd and add-to-cart failed as
	// "unavailable". Served from the same short-lived cache as the listing.
	invItems, err := s.inventoryCatalog(ctx, tenantSlug, "GOODS,RECIPE,SERVICE", nil, "", nil)
	if err != nil {
		return nil, fmt.Errorf("catalog: list inventory items: %w", err)
	}

	// A variant SKU (a child inventory item served under its parent) is found through the parent's
	// variants list in the same cached catalog, so no extra inventory call is made per lookup.
	invItem, variantIdx := findCatalogEntry(invItems, sku)
	if invItem == nil {
		return nil, ErrItemNotFound
	}
	// not_for_sale = purchasable-only stock (e.g. ingredients): hidden from the item detail
	// AND from checkout — the cart service resolves items through this lookup, so a
	// not-for-sale SKU can never be added to a cart either.
	if invItem.NotForSale {
		return nil, ErrItemNotFound
	}
	// Same defense-in-depth as ListItems: a mistyped/mistagged raw-material item sitting
	// in a component category (e.g. "Raw Ingredients") must not be directly reachable by
	// SKU either, even if a customer guesses/deep-links its product-detail URL.
	if !isStorefrontSellableCategory(invItem.CategoryName) {
		return nil, ErrItemNotFound
	}

	// Load override — scoped to a real outlet of this tenant, same reasoning as ListItems above:
	// an unscoped .First() would return whichever of possibly several outlet-keyed rows Postgres
	// happens to return first, not necessarily the one belonging to this tenant.
	overrideOutletIDs, err := s.tenantOutletIDs(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("catalog: load outlets: %w", err)
	}
	loadOverride := func(overrideSKU string) *ent.CatalogOverride {
		overrideQuery := s.db.CatalogOverride.Query().
			Where(
				catalogoverride.TenantID(tenantID),
				catalogoverride.InventorySku(overrideSKU),
			)
		if len(overrideOutletIDs) > 0 {
			overrideQuery = overrideQuery.Where(catalogoverride.OutletIDIn(overrideOutletIDs...))
		}
		o, _ := overrideQuery.First(ctx)
		return o
	}
	override := loadOverride(invItem.SKU)
	// The variant's own override row (created by inventory.item.created for the child item and
	// kept fresh by the stock events, all keyed by the child SKU) is honoured for the variant.
	var variantOverride *ent.CatalogOverride
	if variantIdx >= 0 {
		variantOverride = loadOverride(invItem.Variants[variantIdx].SKU)
	}

	// Load favorite
	favSet := make(map[string]struct{})
	if userID != nil {
		exists, _ := s.db.UserFavorite.Query().
			Where(
				userfavorite.TenantID(tenantID),
				userfavorite.UserID(*userID),
				userfavorite.InventorySku(sku),
			).Exist(ctx)
		if exists {
			favSet[sku] = struct{}{}
		}
	}

	item := mergeItem(*invItem, override, favSet)
	if variantIdx >= 0 {
		item = resolveVariantItem(item, *invItem, override, variantIdx, variantOverride)
		_, item.IsFavorite = favSet[sku]
	}
	item.ImageURL = s.media.URL(item.ImageURL)
	return &item, nil
}

// findCatalogEntry finds the catalog row for sku. A direct item match returns (item, -1). When no
// item has that SKU, the parent whose variants list holds it is returned with the variant's index.
// Variant SKUs compare without case or surrounding spaces, since storefront links and scanned codes
// do not always keep the exact casing.
func findCatalogEntry(items []inventory.ItemResponse, sku string) (*inventory.ItemResponse, int) {
	for i := range items {
		if items[i].SKU == sku {
			return &items[i], -1
		}
	}
	want := strings.TrimSpace(sku)
	if want == "" {
		return nil, -1
	}
	for i := range items {
		for j := range items[i].Variants {
			if strings.EqualFold(strings.TrimSpace(items[i].Variants[j].SKU), want) {
				return &items[i], j
			}
		}
	}
	return nil, -1
}

// variantAttributeOrder puts colour before size so names read "T-shirt, Red / M". Other
// attributes follow in alphabetical order so the name is stable between requests.
var variantAttributeOrder = map[string]int{"colour": 0, "color": 0, "size": 1}

// VariantDisplayName builds the customer-facing name of a variant: the parent name followed by
// the attribute values ("T-shirt, Red / M"). Without attributes it falls back to the variant's
// own name, prefixed with the parent name unless it already starts with it.
func VariantDisplayName(parentName string, v CatalogVariant) string {
	keys := make([]string, 0, len(v.Attributes))
	for k, val := range v.Attributes {
		if strings.TrimSpace(val) != "" {
			keys = append(keys, k)
		}
	}
	if len(keys) > 0 {
		slices.SortFunc(keys, func(a, b string) int {
			ra, oka := variantAttributeOrder[strings.ToLower(a)]
			rb, okb := variantAttributeOrder[strings.ToLower(b)]
			switch {
			case oka && okb && ra != rb:
				return ra - rb
			case oka && !okb:
				return -1
			case !oka && okb:
				return 1
			}
			return strings.Compare(a, b)
		})
		values := make([]string, 0, len(keys))
		for _, k := range keys {
			values = append(values, strings.TrimSpace(v.Attributes[k]))
		}
		return parentName + ", " + strings.Join(values, " / ")
	}
	name := strings.TrimSpace(v.Name)
	switch {
	case name == "":
		return parentName
	case parentName == "" || strings.HasPrefix(strings.ToLower(name), strings.ToLower(parentName)):
		return name
	}
	return parentName + ", " + name
}

// resolveVariantItem turns the merged parent item into the merged view of one of its variants.
// The variant is a child inventory item, so its SKU and item id replace the parent's and every
// stock reservation or deduction keyed by them hits the child's own stock.
//
// Price: the child's own override price, else the variant price from inventory, else the parent
// price. Availability: the parent must be orderable (active, its override not switched off), the
// variant active and the child's own override (when one exists) available. Stock never gates the
// sale: availability is staff-owned platform wide (auto_hide_on_stock_out, default off), so a
// sold-out child is hidden only through its own override, which the stock events switch off when
// the tenant opted in. on_hand is passed through for the "only N left" badge.
func resolveVariantItem(parent MergedCatalogItem, inv inventory.ItemResponse, parentOverride *ent.CatalogOverride, idx int, variantOverride *ent.CatalogOverride) MergedCatalogItem {
	item := parent
	v := parent.Variants[idx]

	item.Variant = &v
	item.ParentSKU = inv.SKU
	item.PriceFrom, item.PriceTo = nil, nil // one variant has one price
	item.InventorySKU = v.SKU
	if v.ID != uuid.Nil {
		item.InventoryID = v.ID
	}
	item.Name = VariantDisplayName(inv.Name, v)
	if v.ImageURL != "" {
		item.ImageURL = v.ImageURL
	}
	if variantOverride != nil && variantOverride.ImageURLOverride != "" {
		item.ImageURL = variantOverride.ImageURLOverride
	}

	price := v.Price
	if variantOverride != nil && variantOverride.BasePrice > 0 {
		price = math.Ceil(variantOverride.BasePrice)
	}
	if price <= 0 {
		price = parent.BasePrice
	}
	item.BasePrice = price
	// The parent's costing and tax amounts were computed for the parent's price, not this one.
	item.CostPrice, item.SuggestedPrice, item.SellingPrice, item.FoodCostPct = nil, nil, nil, nil
	item.FoodCostStatus = ""
	item.NetPrice, item.TaxAmount = nil, nil

	available := inv.IsActive && !inv.NotForSale && v.IsActive &&
		(parentOverride == nil || parentOverride.IsAvailable)
	if variantOverride != nil && !variantOverride.IsAvailable {
		available = false
	}

	qty := v.OnHand
	if qty == nil && variantOverride != nil {
		qty = variantOverride.AvailableQuantity
	}
	if qty != nil && *qty <= 0 {
		qty = nil // same display rule as mergeItem: no "0 left" on an item that is not hidden
	}
	item.AvailableQuantity = qty

	item.IsComplimentary = false
	if price == 0 {
		if variantOverride != nil && variantOverride.IsAvailable && available {
			item.IsComplimentary = true
		} else {
			available = false
		}
	}
	item.IsAvailable = available
	return item
}

// ---------- Categories ----------

// ListCategories proxies to inventory-api categories. Only categories that (a) have
// at least one item linked (has_items=true at source — never sync empty categories),
// (b) are finished/sellable on a storefront, and (c) match the outlet use case when
// one is supplied are returned. useCase may be "" (no outlet selected → all sellable
// categories with items).
func (s *ProxyService) ListCategories(ctx context.Context, tenantSlug, useCase string) ([]InventoryCategory, error) {
	// Cached WITHOUT the use_case filter (mirrors inventory-api's own ListCategories handler
	// comment: "applied post-cache so the cached list stays use-case-agnostic") so every outlet
	// on a tenant shares one cache entry instead of one per use_case.
	var sellable []InventoryCategory
	var err error
	if s.cache != nil {
		key := fmt.Sprintf("ordering:categories:%s", tenantSlug)
		sellable, err = sharedcache.GetOrSet(ctx, s.cache, key, cacheTTLReference, func(ctx context.Context) ([]InventoryCategory, error) {
			return s.listSellableCategoriesFromSource(ctx, tenantSlug)
		})
	} else {
		sellable, err = s.listSellableCategoriesFromSource(ctx, tenantSlug)
	}
	if err != nil {
		return nil, err
	}

	if useCase == "" {
		return sellable, nil
	}
	result := make([]InventoryCategory, 0, len(sellable))
	for _, c := range sellable {
		// Outlet use-case gating: a hardware/electronics store must never list food/pizza
		// categories, a restaurant must never list detergents, etc.
		if categoryAllowedForUseCase(c.Name, useCase) {
			result = append(result, c)
		}
	}
	return result, nil
}

// listSellableCategoriesFromSource fetches categories from inventory-api and keeps only
// finished/sellable ones (hides component/service categories — raw ingredients, modifiers,
// add-ons, conference/room/facility/salon). Not use-case-filtered — see ListCategories.
func (s *ProxyService) listSellableCategoriesFromSource(ctx context.Context, tenantSlug string) ([]InventoryCategory, error) {
	cats, err := s.inventoryClient.ListCategories(ctx, tenantSlug, true /* hasItems */)
	if err != nil {
		return nil, fmt.Errorf("catalog: list categories: %w", err)
	}

	result := make([]InventoryCategory, 0, len(cats))
	for _, c := range cats {
		if !isStorefrontSellableCategory(c.Name) {
			continue
		}
		result = append(result, InventoryCategory{
			ID:          c.ID,
			Name:        c.Name,
			Code:        c.Code,
			Description: c.Description,
			Icon:        c.Icon,
			IsActive:    c.IsActive,
			ParentID:    c.ParentID,
			ParentName:  c.ParentName,
			Depth:       c.Depth,
			Path:        c.Path,
			SortOrder:   c.SortOrder,
		})
	}
	return result, nil
}

// categoryAllowedForUseCase reports whether a category name is appropriate for the
// given outlet use case. Case-insensitive substring matching keeps minor name
// variations from breaking the filter. An empty useCase (no outlet selected) or an
// empty category name is always allowed. Mirrors the proven pos-api logic so the
// storefront and the POS terminal agree on which categories belong to which vertical.
func categoryAllowedForUseCase(categoryName, useCase string) bool {
	cat := strings.ToLower(strings.TrimSpace(categoryName))
	if cat == "" || useCase == "" {
		return true
	}
	contains := func(needles ...string) bool {
		for _, n := range needles {
			if strings.Contains(cat, n) {
				return true
			}
		}
		return false
	}
	isPharmacy := contains("pharmacy", "chemist", "drug", "medicine", "pharmaceutical")
	isFood := contains("breakfast", "beverage", "pastry", "pastries", "bakery", "pizza",
		"salad", "sandwich", "wrap", "main course", "light bite", "dessert",
		"hot beverage", "cold beverage", "starter", "appetiz", "grill", "drink")
	isServices := contains("beauty", "spa", "event", "experience", "wellness", "conference",
		"meeting", "facility", "amenity", "salon", "massage", "room rate", "room type")
	isRetail := contains("retail")
	isRetailMerchandise := contains("detergent", "cleaning", "cleaner", "household", "home care",
		"homeware", "kitchenware", "hardware", "electronic", "appliance", "apparel", "clothing",
		"fashion", "footwear", "cosmetic", "toiletr", "stationery", "personal care", "furniture", "grocery")

	switch strings.ToLower(strings.TrimSpace(useCase)) {
	case "retail":
		return !isPharmacy && !isFood && !isServices
	case "pharmacy":
		return isPharmacy
	case "hospitality", "quick_service", "food", "food_delivery", "restaurant", "bar", "cafe":
		return !isPharmacy && !isRetail && !isRetailMerchandise && !isServices
	case "services", "salon", "spa", "beauty", "wellness":
		return !isPharmacy && !isFood && !isRetail && !isRetailMerchandise
	default:
		return true
	}
}

// ---------- Brands ----------

// ListBrands proxies to inventory-api brands so the storefront can offer a Brands tab / filter.
func (s *ProxyService) ListBrands(ctx context.Context, tenantSlug string) ([]InventoryBrand, error) {
	brands, err := s.inventoryClient.ListBrands(ctx, tenantSlug)
	if err != nil {
		return nil, fmt.Errorf("catalog: list brands: %w", err)
	}

	result := make([]InventoryBrand, 0, len(brands))
	for _, b := range brands {
		if !b.IsActive {
			continue
		}
		result = append(result, InventoryBrand{
			ID:        b.ID,
			Name:      b.Name,
			Code:      b.Code,
			LogoURL:   b.LogoURL,
			IsActive:  b.IsActive,
			SortOrder: b.SortOrder,
		})
	}
	return result, nil
}

// isStorefrontSellableCategory reports whether a category should appear on the customer storefront.
// Excludes recipe/modifier components and service-booking categories that are never directly orderable
// online (raw ingredients, modifiers, add-ons, conference/room/facility/salon). Accompaniments (ugali,
// sides) ARE kept — they are real menu items.
func isStorefrontSellableCategory(name string) bool {
	c := strings.ToLower(strings.TrimSpace(name))
	if c == "" {
		return true
	}
	for _, bad := range []string{
		"raw ingredient", "raw material", "ingredient", "modifier", "add-on", "add on",
		"conference", "meeting", "facility", "amenity", "room rate", "room type", "salon", "massage",
	} {
		if strings.Contains(c, bad) {
			return false
		}
	}
	return true
}

// ---------- Overrides ----------

// UpsertOverride creates or updates a CatalogOverride entry.
func (s *ProxyService) UpsertOverride(ctx context.Context, req OverrideUpsertRequest) (*ent.CatalogOverride, error) {
	currency := req.Currency
	if currency == "" {
		currency = DefaultCurrency
	}

	// Try to find existing
	existing, _ := s.db.CatalogOverride.Query().
		Where(
			catalogoverride.TenantID(req.TenantID),
			catalogoverride.OutletID(req.OutletID),
			catalogoverride.InventorySku(req.InventorySKU),
		).First(ctx)

	if existing != nil {
		// Update
		builder := s.db.CatalogOverride.UpdateOneID(existing.ID).
			SetCurrency(currency)
		if req.BasePrice != nil {
			builder.SetBasePrice(*req.BasePrice)
		}

		if req.IsAvailable != nil {
			builder.SetIsAvailable(*req.IsAvailable)
		}
		if req.IsFeatured != nil {
			builder.SetIsFeatured(*req.IsFeatured)
		}
		if req.LeadTimeMinutes != nil {
			builder.SetLeadTimeMinutes(*req.LeadTimeMinutes)
		}
		if req.DisplayOrder != nil {
			builder.SetDisplayOrder(*req.DisplayOrder)
		}
		if req.DisplaySection != "" {
			builder.SetDisplaySection(req.DisplaySection)
		}
		if req.PackagingFee != nil {
			builder.SetPackagingFee(*req.PackagingFee)
		}
		if req.ServiceFeePercent != nil {
			builder.SetServiceFeePercent(*req.ServiceFeePercent)
		}
		if req.ImageURLOverride != "" {
			builder.SetImageURLOverride(s.media.Normalize(req.ImageURLOverride))
		}

		saved, err := builder.Save(ctx)
		if err == nil {
			s.releaseOverrideImage(ctx, req.TenantID, existing.ImageURLOverride, saved.ImageURLOverride)
		}
		return saved, err
	}

	// Create
	builder := s.db.CatalogOverride.Create().
		SetTenantID(req.TenantID).
		SetOutletID(req.OutletID).
		SetInventorySku(req.InventorySKU).
		SetCurrency(currency)
	if req.BasePrice != nil {
		builder.SetBasePrice(*req.BasePrice) // unset = 0 = "no price override, use inventory's"
	}

	if req.IsAvailable != nil {
		builder.SetIsAvailable(*req.IsAvailable)
	}
	if req.IsFeatured != nil {
		builder.SetIsFeatured(*req.IsFeatured)
	}
	if req.LeadTimeMinutes != nil {
		builder.SetLeadTimeMinutes(*req.LeadTimeMinutes)
	}
	if req.DisplayOrder != nil {
		builder.SetDisplayOrder(*req.DisplayOrder)
	}
	if req.DisplaySection != "" {
		builder.SetDisplaySection(req.DisplaySection)
	}
	if req.PackagingFee != nil {
		builder.SetPackagingFee(*req.PackagingFee)
	}
	if req.ServiceFeePercent != nil {
		builder.SetServiceFeePercent(*req.ServiceFeePercent)
	}
	if req.ImageURLOverride != "" {
		builder.SetImageURLOverride(s.media.Normalize(req.ImageURLOverride))
	}

	return builder.Save(ctx)
}

// DeleteOverride removes a CatalogOverride entry by SKU (every outlet's), and frees the override
// images no remaining override uses.
func (s *ProxyService) DeleteOverride(ctx context.Context, tenantID uuid.UUID, sku string) error {
	where := []predicate.CatalogOverride{catalogoverride.TenantID(tenantID), catalogoverride.InventorySku(sku)}
	images, err := s.db.CatalogOverride.Query().Where(append(where, catalogoverride.ImageURLOverrideNEQ(""))...).
		Select(catalogoverride.FieldImageURLOverride).Strings(ctx)
	if err != nil {
		return err
	}
	if _, err := s.db.CatalogOverride.Delete().Where(where...).Exec(ctx); err != nil {
		return err
	}
	for _, img := range images {
		s.releaseOverrideImage(ctx, tenantID, img, "")
	}
	return nil
}

// ---------- Favorites ----------

// ToggleFavorite toggles a UserFavorite entry. Returns true if now favorited.
func (s *ProxyService) ToggleFavorite(ctx context.Context, tenantID, userID uuid.UUID, sku string) (bool, error) {
	existing, _ := s.db.UserFavorite.Query().
		Where(
			userfavorite.TenantID(tenantID),
			userfavorite.UserID(userID),
			userfavorite.InventorySku(sku),
		).First(ctx)

	if existing != nil {
		// Remove
		err := s.db.UserFavorite.DeleteOneID(existing.ID).Exec(ctx)
		return false, err
	}

	// Add
	_, err := s.db.UserFavorite.Create().
		SetTenantID(tenantID).
		SetUserID(userID).
		SetInventorySku(sku).
		Save(ctx)
	return true, err
}

// ---------- Outlets ----------

// ListOutlets returns all active outlets for a tenant (cached).
func (s *ProxyService) ListOutlets(ctx context.Context, tenantID uuid.UUID) ([]OutletSummary, error) {
	if s.cache != nil {
		key := fmt.Sprintf("ordering:outlets:%s", tenantID)
		return sharedcache.GetOrSet(ctx, s.cache, key, cacheTTLReference, func(ctx context.Context) ([]OutletSummary, error) {
			return s.listOutletsFromDB(ctx, tenantID)
		})
	}
	return s.listOutletsFromDB(ctx, tenantID)
}

func (s *ProxyService) listOutletsFromDB(ctx context.Context, tenantID uuid.UUID) ([]OutletSummary, error) {
	outlets, err := s.db.Outlet.Query().
		Where(outlet.TenantID(tenantID), outlet.StatusEQ("active")).
		All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]OutletSummary, len(outlets))
	for i, o := range outlets {
		result[i] = entOutletToSummary(o)
		result[i].ImageURL = s.media.URL(o.ImageURL)
	}
	return result, nil
}

// TenantOutletIDs exposes tenantOutletIDs to handlers that must apply a tenant-wide override
// (e.g. a staff availability toggle sent without an outlet).
func (s *ProxyService) TenantOutletIDs(ctx context.Context, tenantID uuid.UUID) ([]uuid.UUID, error) {
	return s.tenantOutletIDs(ctx, tenantID)
}

// tenantOutletIDs returns every outlet ID actually belonging to this tenant (cached via
// ListOutlets), used to scope catalog-override lookups so a stale/foreign outlet_id on an
// override row can never be read as if it were one of this tenant's own outlets.
func (s *ProxyService) tenantOutletIDs(ctx context.Context, tenantID uuid.UUID) ([]uuid.UUID, error) {
	outlets, err := s.ListOutlets(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(outlets))
	for i, o := range outlets {
		ids[i] = o.ID
	}
	return ids, nil
}

// GetOutlet retrieves a single outlet by ID.
func (s *ProxyService) GetOutlet(ctx context.Context, tenantID, outletID uuid.UUID) (*OutletSummary, error) {
	o, err := s.db.Outlet.Query().
		Where(outlet.ID(outletID), outlet.TenantID(tenantID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrOutletNotFound
		}
		return nil, err
	}
	summary := entOutletToSummary(o)
	summary.ImageURL = s.media.URL(o.ImageURL)
	return &summary, nil
}

// ---------- Item Images ----------

// GetItemImages proxies to inventory-api's item assets endpoint to return
// a multi-image gallery for the given item SKU.
func (s *ProxyService) GetItemImages(ctx context.Context, tenantSlug, sku string) ([]ItemImage, error) {
	path := fmt.Sprintf("/v1/%s/inventory/items/%s/assets", tenantSlug, sku)

	resp, err := s.inventoryClient.ServiceClient().Get(ctx, path, nil)
	if err != nil {
		return nil, fmt.Errorf("catalog: get item images: %w", err)
	}

	if !resp.IsSuccess() {
		// If no assets found, return empty list rather than error
		return []ItemImage{}, nil
	}

	var assets []struct {
		URL          string `json:"url"`
		Type         string `json:"type"`
		IsPrimary    bool   `json:"is_primary"`
		DisplayOrder int    `json:"display_order"`
	}
	if err := resp.DecodeJSON(&assets); err != nil {
		return nil, fmt.Errorf("catalog: decode item images: %w", err)
	}

	images := make([]ItemImage, len(assets))
	for i, a := range assets {
		images[i] = ItemImage{
			URL:          a.URL,
			Type:         a.Type,
			IsPrimary:    a.IsPrimary,
			DisplayOrder: a.DisplayOrder,
		}
	}
	return images, nil
}

// ---------- Helpers ----------

func entOutletToSummary(o *ent.Outlet) OutletSummary {
	return OutletSummary{
		ID:                    o.ID,
		Name:                  o.Name,
		Slug:                  o.Slug,
		Description:           o.Description,
		Address:               o.Address,
		Phone:                 o.Phone,
		Email:                 o.Email,
		Location:              o.Location,
		Latitude:              o.Latitude,
		Longitude:             o.Longitude,
		OpeningHours:          o.OpeningHours,
		ImageURL:              o.ImageURL,
		Status:                o.Status,
		UseCase:               o.UseCase,
		IsOpen:                IsOutletOpen(o.OpeningHours),
		BookingDepositPercent: o.BookingDepositPercent,
	}
}

func mergeItem(inv inventory.ItemResponse, override *ent.CatalogOverride, favSet map[string]struct{}) MergedCatalogItem {
	item := MergedCatalogItem{
		InventorySKU:  inv.SKU,
		InventoryID:   inv.ID,
		Name:          inv.Name,
		Description:   inv.Description,
		Type:          inv.Type,
		IsActive:      inv.IsActive,
		ImageURL:      inv.ImageURL,
		CategoryID:    inv.CategoryID,
		CategoryName:  inv.CategoryName,
		BrandID:       inv.BrandID,
		BrandName:     inv.BrandName,
		BrandCode:     inv.BrandCode,
		Manufacturer:  inv.Manufacturer,
		Model:         inv.Model,
		Condition:     inv.Condition,
		HasVariants:   inv.HasVariants,
		Tags:          inv.Tags,
		Metadata:      inv.Metadata,
		TotalCapacity: inv.TotalCapacity,
		EventStartAt:  inv.EventStartAt,
		EventEndAt:    inv.EventEndAt,
		EventVenue:    inv.EventVenue,
		Currency:      DefaultCurrency,
		IsAvailable:   inv.IsActive,
	}

	// Surface sellable product variations from inventory.
	if len(inv.Variants) > 0 {
		item.Variants = make([]CatalogVariant, 0, len(inv.Variants))
		for _, v := range inv.Variants {
			price := v.Price
			if price > 0 {
				price = math.Ceil(price) // same whole-number rounding as BasePrice below
			}
			item.Variants = append(item.Variants, CatalogVariant{
				ID:         v.ID,
				SKU:        v.SKU,
				Name:       v.Name,
				Price:      price,
				Attributes: v.Attributes,
				Barcode:    v.Barcode,
				IsActive:   v.IsActive,
				OnHand:     v.OnHand,
				ImageURL:   v.ImageURL,
			})
		}
		item.HasVariants = true
	}

	// Surface selectable modifier groups (checkboxes/radios) from inventory.
	if len(inv.ModifierGroups) > 0 {
		item.ModifierGroups = make([]CatalogModifierGroup, 0, len(inv.ModifierGroups))
		for _, g := range inv.ModifierGroups {
			options := make([]CatalogModifierOption, 0, len(g.Options))
			for _, o := range g.Options {
				options = append(options, CatalogModifierOption{
					ID:              o.ID,
					Name:            o.Name,
					SKU:             o.SKU,
					PriceAdjustment: o.PriceAdjustment,
					IsDefault:       o.IsDefault,
				})
			}
			item.ModifierGroups = append(item.ModifierGroups, CatalogModifierGroup{
				ID:            g.ID,
				Name:          g.Name,
				IsRequired:    g.IsRequired,
				MinSelections: g.MinSelections,
				MaxSelections: g.MaxSelections,
				Options:       options,
			})
		}
	}

	// Propagate new costing fields from inventory.
	item.CostPrice = inv.CostPrice
	item.SuggestedPrice = inv.SuggestedPrice
	item.SellingPrice = inv.SellingPrice
	item.FoodCostPct = inv.FoodCostPct
	item.FoodCostStatus = inv.FoodCostStatus
	// Tax (enriched by inventory-api from treasury-api).
	item.TaxCodeID = inv.TaxCodeID
	item.TaxInclusive = inv.TaxInclusive
	item.TaxRate = inv.TaxRate
	item.NetPrice = inv.NetPrice
	item.TaxAmount = inv.TaxAmount

	// Seed price from inventory: selling_price > suggested_price > cost_price fallback.
	if inv.SellingPrice != nil && *inv.SellingPrice > 0 {
		item.BasePrice = *inv.SellingPrice
	} else if inv.SuggestedPrice != nil && *inv.SuggestedPrice > 0 {
		item.BasePrice = *inv.SuggestedPrice
	} else if inv.CostPrice != nil && *inv.CostPrice > 0 {
		item.BasePrice = *inv.CostPrice
	}

	if override != nil {
		if override.BasePrice > 0 {
			item.BasePrice = override.BasePrice
		}
		item.Currency = override.Currency
		item.IsAvailable = override.IsAvailable
		item.AvailableQuantity = override.AvailableQuantity
		item.IsFeatured = override.IsFeatured
		item.LeadTimeMinutes = override.LeadTimeMinutes
		item.DisplayOrder = override.DisplayOrder
		item.DisplaySection = override.DisplaySection
		item.PackagingFee = override.PackagingFee
		item.ServiceFeePercent = override.ServiceFeePercent
		item.CreatedAt = override.CreatedAt
		item.UpdatedAt = override.UpdatedAt

		if override.ImageURLOverride != "" {
			item.ImageURL = override.ImageURLOverride
		}
	}

	// available_quantity is informational. An item staff left available is orderable even when
	// the system's stock reads zero or negative (manual-only availability: the gap is a
	// system/physical mismatch, settled by the next stock receipt or count), so a non-positive
	// projection is dropped rather than shown as "0 left" or filtered out as out of stock.
	if item.IsAvailable && item.AvailableQuantity != nil && *item.AvailableQuantity <= 0 {
		item.AvailableQuantity = nil
	}

	// Round price up to next whole number — no decimal prices on the ordering app.
	if item.BasePrice > 0 {
		item.BasePrice = math.Ceil(item.BasePrice)
	}

	// Zero-price gate, with an exception for COMPLIMENTARY accompaniments. By default a no-price item
	// cannot be ordered online. BUT a price-0 item explicitly enabled via a catalog override
	// (override.IsAvailable) is treated as a no-charge accompaniment: it stays orderable, is flagged
	// so the app shows "Free", and is not billed — while its recipe/BOM stock is still deducted on
	// fulfilment (the inventory consumer deducts by recipe/modifier SKU regardless of price).
	if item.BasePrice == 0 {
		if override != nil && override.IsAvailable {
			item.IsComplimentary = true
		} else {
			item.IsAvailable = false
		}
	}

	// Defence in depth: not_for_sale marks purchasable-only stock (e.g. ingredients).
	// The list/detail paths already exclude such items entirely; if a merged item is
	// ever built for one anyway, no override (not even a complimentary price-0 enable)
	// may make it orderable.
	if inv.NotForSale {
		item.IsAvailable = false
		item.IsComplimentary = false
	}

	_, item.IsFavorite = favSet[inv.SKU]
	setPriceRange(&item)

	return item
}
