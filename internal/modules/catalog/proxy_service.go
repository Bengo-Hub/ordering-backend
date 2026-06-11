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
	// Apply the category filter SERVER-SIDE so a selected category returns its items + the correct
	// total (the old client-side filter ran after pagination → empty/null page for any category).
	invItems, invTotal, err := s.inventoryClient.ListItems(ctx, tenantSlug, filter.ItemType, limit, page, filter.CategoryID)
	if err != nil {
		return nil, 0, fmt.Errorf("catalog: list inventory items: %w", err)
	}

	// 2. Load all overrides for this tenant (optionally scoped to outlet)
	overrideQuery := s.db.CatalogOverride.Query().
		Where(catalogoverride.TenantID(tenantID))
	if filter.OutletID != nil {
		overrideQuery = overrideQuery.Where(catalogoverride.OutletID(*filter.OutletID))
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

	return merged, invTotal, nil
}

// GetItem fetches a single item from inventory-api by SKU, merges with override.
func (s *ProxyService) GetItem(ctx context.Context, tenantSlug string, tenantID uuid.UUID, sku string, userID *uuid.UUID) (*MergedCatalogItem, error) {
	// Fetch all orderable types to locate the item by SKU (single-item lookup, no pagination).
	invItems, _, err := s.inventoryClient.ListItems(ctx, tenantSlug, "GOODS,RECIPE,SERVICE", 100, 1, nil)
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

	// Load override
	override, _ := s.db.CatalogOverride.Query().
		Where(
			catalogoverride.TenantID(tenantID),
			catalogoverride.InventorySku(sku),
		).
		First(ctx)

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

// ListCategories proxies to inventory-api categories.
func (s *ProxyService) ListCategories(ctx context.Context, tenantSlug string) ([]InventoryCategory, error) {
	cats, err := s.inventoryClient.ListCategories(ctx, tenantSlug)
	if err != nil {
		return nil, fmt.Errorf("catalog: list categories: %w", err)
	}

	result := make([]InventoryCategory, 0, len(cats))
	for _, c := range cats {
		// Only finished, sellable categories belong on the storefront — hide component/service
		// categories (raw ingredients, modifiers, add-ons, conference/room/facility/salon).
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
		})
	}
	return result, nil
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
		ID:           o.ID,
		Name:         o.Name,
		Slug:         o.Slug,
		Description:  o.Description,
		Address:      o.Address,
		Phone:        o.Phone,
		Email:        o.Email,
		Location:     o.Location,
		Latitude:     o.Latitude,
		Longitude:    o.Longitude,
		OpeningHours: o.OpeningHours,
		ImageURL:     o.ImageURL,
		Status:       o.Status,
		UseCase:      o.UseCase,
		IsOpen:       IsOutletOpen(o.OpeningHours),
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
		Tags:          inv.Tags,
		Metadata:      inv.Metadata,
		TotalCapacity: inv.TotalCapacity,
		EventStartAt:  inv.EventStartAt,
		EventEndAt:    inv.EventEndAt,
		EventVenue:    inv.EventVenue,
		Currency:      DefaultCurrency,
		IsAvailable:   inv.IsActive,
	}

	// Propagate new costing fields from inventory.
	item.CostPrice      = inv.CostPrice
	item.SuggestedPrice = inv.SuggestedPrice
	item.SellingPrice   = inv.SellingPrice
	item.FoodCostPct    = inv.FoodCostPct
	item.FoodCostStatus = inv.FoodCostStatus
	// Tax (enriched by inventory-api from treasury-api).
	item.TaxCodeID    = inv.TaxCodeID
	item.TaxInclusive = inv.TaxInclusive
	item.TaxRate      = inv.TaxRate
	item.NetPrice     = inv.NetPrice
	item.TaxAmount    = inv.TaxAmount

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

	// Items with no price cannot be ordered online regardless of override.
	// A price must be set in inventory (cost_price/suggested_price/selling_price)
	// OR in a catalog override before the item appears in the public catalog.
	if item.BasePrice == 0 {
		item.IsAvailable = false
	}

	_, item.IsFavorite = favSet[inv.SKU]

	return item
}
