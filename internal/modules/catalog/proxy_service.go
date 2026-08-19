package catalog

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/catalogoverride"
	"github.com/bengobox/ordering-backend/internal/ent/outlet"
	"github.com/bengobox/ordering-backend/internal/ent/userfavorite"
	"github.com/bengobox/ordering-backend/internal/platform/cache"
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
	cache           *cache.Service
	logger          *zap.Logger
}

// NewProxyService creates a new proxy catalog service.
func NewProxyService(db *ent.Client, inventoryClient *inventory.Client, cacheSvc *cache.Service, logger *zap.Logger) *ProxyService {
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
	// 1. Fetch inventory items with server-side type filter and pagination.
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	page := 1
	if filter.Offset > 0 {
		page = (filter.Offset / limit) + 1
	}
	// The "featured" flag lives on local CatalogOverride rows, NOT on inventory items, so it
	// cannot be pushed into the paginated inventory fetch. Featured items are scattered across
	// the whole catalog; fetching only the first `limit` inventory rows and THEN filtering by
	// featured (below) surfaces just the handful of featured items that happen to land on the
	// first page. When a featured listing is requested, scan the full catalog up-front and
	// re-apply the caller's limit to the filtered result before returning.
	var invItems []inventory.ItemResponse
	var invTotal int
	var err error
	featuredListing := filter.IsFeatured != nil && *filter.IsFeatured
	if featuredListing {
		// inventory-api clamps any requested limit to its shared pagination cap (100), so a
		// single large-limit request silently truncates the catalog past the first ~100 items —
		// must page through all of it, not request one oversized page.
		invItems, err = s.fetchAllInventoryItems(ctx, tenantSlug, filter.ItemType, filter.CategoryID, filter.Sort)
		if err != nil {
			return nil, 0, fmt.Errorf("catalog: list inventory items: %w", err)
		}
		invTotal = len(invItems)
	} else {
		// Apply the category filter SERVER-SIDE so a selected category returns its items + the
		// correct total (the old client-side filter ran after pagination → empty/null page for
		// any category).
		invItems, invTotal, err = s.inventoryClient.ListItems(ctx, tenantSlug, filter.ItemType, limit, page, filter.CategoryID, filter.Sort)
		if err != nil {
			return nil, 0, fmt.Errorf("catalog: list inventory items: %w", err)
		}
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
		return nil, 0, fmt.Errorf("catalog: load outlets: %w", err)
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
		return nil, 0, fmt.Errorf("catalog: load overrides: %w", err)
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
		override := overrideMap[inv.SKU] // nil when no override exists — mergeItem handles it
		item := mergeItem(inv, override, favSet)

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
		if filter.CategoryID != nil && (item.CategoryID == nil || *item.CategoryID != *filter.CategoryID) {
			continue
		}

		merged = append(merged, item)
	}

	// Featured listings over-fetched the whole catalog to find scattered featured items; report
	// the true featured count and honour the caller's requested page size on the filtered result.
	if featuredListing {
		total := len(merged)
		if len(merged) > limit {
			merged = merged[:limit]
		}
		return merged, total, nil
	}

	return merged, invTotal, nil
}

// inventoryPageSize matches inventory-api's shared pagination.MaxLimit. Any larger requested
// limit is silently clamped server-side, so paging must use exactly this size.
const inventoryPageSize = 100

// fetchAllInventoryItems pages through inventory-api's item listing until every row is retrieved.
// A single request with a large limit is silently truncated to inventoryPageSize server-side —
// callers that need the full catalog (e.g. scanning for featured items scattered across it) must
// page through it explicitly.
func (s *ProxyService) fetchAllInventoryItems(ctx context.Context, tenantSlug, itemType string, categoryID *uuid.UUID, sort string) ([]inventory.ItemResponse, error) {
	items := make([]inventory.ItemResponse, 0, 2*inventoryPageSize)
	for page := 1; page <= 50; page++ { // runaway guard: 50 pages = 5k items
		pageItems, total, err := s.inventoryClient.ListItems(ctx, tenantSlug, itemType, inventoryPageSize, page, categoryID, sort)
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
	// Fetch all orderable types to locate the item by SKU (single-item lookup, no pagination).
	invItems, _, err := s.inventoryClient.ListItems(ctx, tenantSlug, "GOODS,RECIPE,SERVICE", 100, 1, nil, "")
	if err != nil {
		return nil, fmt.Errorf("catalog: list inventory items: %w", err)
	}

	var invItem *inventory.ItemResponse
	for i := range invItems {
		if invItems[i].SKU == sku {
			invItem = &invItems[i]
			break
		}
	}
	if invItem == nil {
		return nil, ErrItemNotFound
	}
	// not_for_sale = purchasable-only stock (e.g. ingredients): hidden from the item detail
	// AND from checkout — the cart service resolves items through this lookup, so a
	// not-for-sale SKU can never be added to a cart either.
	if invItem.NotForSale {
		return nil, ErrItemNotFound
	}

	// Load override — scoped to a real outlet of this tenant, same reasoning as ListItems above:
	// an unscoped .First() would return whichever of possibly several outlet-keyed rows Postgres
	// happens to return first, not necessarily the one belonging to this tenant.
	overrideOutletIDs, err := s.tenantOutletIDs(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("catalog: load outlets: %w", err)
	}
	overrideQuery := s.db.CatalogOverride.Query().
		Where(
			catalogoverride.TenantID(tenantID),
			catalogoverride.InventorySku(sku),
		)
	if len(overrideOutletIDs) > 0 {
		overrideQuery = overrideQuery.Where(catalogoverride.OutletIDIn(overrideOutletIDs...))
	}
	override, _ := overrideQuery.First(ctx)

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
	return &item, nil
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
		key := fmt.Sprintf("categories:%s", tenantSlug)
		err = s.cache.GetOrSet(ctx, key, &sellable, cacheTTLReference, func() (interface{}, error) {
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
			SetBasePrice(req.BasePrice).
			SetCurrency(currency)

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
			builder.SetImageURLOverride(req.ImageURLOverride)
		}

		return builder.Save(ctx)
	}

	// Create
	builder := s.db.CatalogOverride.Create().
		SetTenantID(req.TenantID).
		SetOutletID(req.OutletID).
		SetInventorySku(req.InventorySKU).
		SetBasePrice(req.BasePrice).
		SetCurrency(currency)

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
		builder.SetImageURLOverride(req.ImageURLOverride)
	}

	return builder.Save(ctx)
}

// DeleteOverride removes a CatalogOverride entry by SKU.
func (s *ProxyService) DeleteOverride(ctx context.Context, tenantID uuid.UUID, sku string) error {
	_, err := s.db.CatalogOverride.Delete().
		Where(
			catalogoverride.TenantID(tenantID),
			catalogoverride.InventorySku(sku),
		).Exec(ctx)
	return err
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
		key := fmt.Sprintf("outlets:%s", tenantID)
		var result []OutletSummary
		err := s.cache.GetOrSet(ctx, key, &result, cacheTTLReference, func() (interface{}, error) {
			return s.listOutletsFromDB(ctx, tenantID)
		})
		return result, err
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
	}
	return result, nil
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
			item.Variants = append(item.Variants, CatalogVariant{
				ID:         v.ID,
				SKU:        v.SKU,
				Name:       v.Name,
				Price:      v.Price,
				Attributes: v.Attributes,
				Barcode:    v.Barcode,
				IsActive:   v.IsActive,
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

	return item
}
