package catalog

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/platform/cache"
)

// Cache TTL tiers for catalog data.
const (
	cacheTTLReference = 5 * time.Minute  // semi-static: categories, outlets, dietary tags
	cacheTTLModerate  = 1 * time.Minute  // moderate-change: menu items
)

// InventoryClient defines the interface for inventory-api integration.
type InventoryClient interface {
	GetOrCreateItem(ctx context.Context, tenantSlug, sku string, req interface{}) (interface{}, error)
}

// Service provides catalog business logic.
type Service struct {
	repo       Repository
	cache      *cache.Service
	logger     *zap.Logger
	tenantSlug string // cached for inventory calls (set via SetTenantSlug)
}

// NewService creates a new catalog service.
func NewService(repo Repository, logger *zap.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
	}
}

// SetCache injects the cache service (optional; caching is skipped if nil).
func (s *Service) SetCache(c *cache.Service) {
	s.cache = c
}

// invalidateCatalogCache clears cached catalog data for a tenant after mutations.
func (s *Service) invalidateCatalogCache(ctx context.Context, tenantID *uuid.UUID) {
	if s.cache == nil || tenantID == nil {
		return
	}
	tid := tenantID.String()
	_ = s.cache.DeletePattern(ctx, fmt.Sprintf("pub:categories:%s:*", tid))
	_ = s.cache.DeletePattern(ctx, fmt.Sprintf("outlets:%s", tid))
	_ = s.cache.DeletePattern(ctx, fmt.Sprintf("pub:menu:%s:*", tid))
}

// --- Category Operations ---

// CreateCategoryRequest represents a request to create a category.
type CreateCategoryRequest struct {
	TenantID     *uuid.UUID
	OutletID     *uuid.UUID
	ParentID     *uuid.UUID
	Name         string
	Slug         string
	Description  string
	ImageURL     string
	DisplayOrder int
	IsActive     bool
}

// CreateCategory creates a new menu category.
func (s *Service) CreateCategory(ctx context.Context, req CreateCategoryRequest) (*Category, error) {

	// Validate parent category exists if specified
	if req.ParentID != nil {
		var tenantID uuid.UUID
		if req.TenantID != nil {
			tenantID = *req.TenantID
		}
		parent, err := s.repo.GetCategory(ctx, tenantID, *req.ParentID)
		if err != nil {
			return nil, ErrInvalidCategoryParent
		}
		// Ensure parent is in same outlet
		if (parent.OutletID == nil && req.OutletID != nil) || (parent.OutletID != nil && req.OutletID == nil) || (parent.OutletID != nil && req.OutletID != nil && *parent.OutletID != *req.OutletID) {
			return nil, ErrInvalidCategoryParent
		}
	}

	category := &Category{
		TenantID:     req.TenantID,
		OutletID:     req.OutletID,
		ParentID:     req.ParentID,
		Name:         req.Name,
		Slug:         req.Slug,
		Description:  req.Description,
		ImageURL:     req.ImageURL,
		DisplayOrder: req.DisplayOrder,
		IsActive:     req.IsActive,
	}

	if err := s.repo.CreateCategory(ctx, category); err != nil {
		s.logger.Error("failed to create category", zap.Error(err))
		return nil, err
	}

	s.logger.Info("category created",
		zap.String("id", category.ID.String()))

	s.invalidateCatalogCache(ctx, req.TenantID)
	return category, nil
}

// GetCategory retrieves a category by ID.
func (s *Service) GetCategory(ctx context.Context, tenantID, categoryID uuid.UUID) (*Category, error) {
	return s.repo.GetCategory(ctx, tenantID, categoryID)
}

// GetOutlet retrieves a specific outlet by ID.
func (s *Service) GetOutlet(ctx context.Context, tenantID, outletID uuid.UUID) (*OutletSummary, error) {
	// For now, we project the outlet display info from its categories/items
	return s.repo.GetOutlet(ctx, tenantID, outletID)
}

// ListCategories lists categories with optional filters.
func (s *Service) ListCategories(ctx context.Context, filter CategoryFilter) ([]Category, int, error) {
	return s.repo.ListCategories(ctx, filter)
}

// UpdateCategoryRequest represents a request to update a category.
type UpdateCategoryRequest struct {
	TenantID     uuid.UUID
	CategoryID   uuid.UUID
	Name         *string
	Slug         *string
	Description  *string
	ImageURL     *string
	DisplayOrder *int
	IsActive     *bool
	ParentID     *uuid.UUID
	ClearParent  bool
}

// UpdateCategory updates an existing category.
func (s *Service) UpdateCategory(ctx context.Context, req UpdateCategoryRequest) (*Category, error) {
	category, err := s.repo.GetCategory(ctx, req.TenantID, req.CategoryID)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		category.Name = *req.Name
	}
	if req.Slug != nil {
		category.Slug = *req.Slug
	}
	if req.Description != nil {
		category.Description = *req.Description
	}
	if req.ImageURL != nil {
		category.ImageURL = *req.ImageURL
	}
	if req.DisplayOrder != nil {
		category.DisplayOrder = *req.DisplayOrder
	}
	if req.IsActive != nil {
		category.IsActive = *req.IsActive
	}
	if req.ClearParent {
		category.ParentID = nil
	} else if req.ParentID != nil {
		// Validate parent exists and is different from self
		if *req.ParentID == category.ID {
			return nil, ErrInvalidCategoryParent
		}
		parent, err := s.repo.GetCategory(ctx, req.TenantID, *req.ParentID)
		if err != nil {
			return nil, ErrInvalidCategoryParent
		}
		if (parent.OutletID == nil && category.OutletID != nil) || (parent.OutletID != nil && category.OutletID == nil) || (parent.OutletID != nil && category.OutletID != nil && *parent.OutletID != *category.OutletID) {
			return nil, ErrInvalidCategoryParent
		}
		category.ParentID = req.ParentID
	}

	if err := s.repo.UpdateCategory(ctx, category); err != nil {
		s.logger.Error("failed to update category", zap.Error(err))
		return nil, err
	}

	s.logger.Info("category updated", zap.String("id", category.ID.String()))
	s.invalidateCatalogCache(ctx, &req.TenantID)
	return category, nil
}

// DeleteCategory deletes a category if it has no items or children.
func (s *Service) DeleteCategory(ctx context.Context, tenantID, categoryID uuid.UUID) error {
	// Check if category exists
	_, err := s.repo.GetCategory(ctx, tenantID, categoryID)
	if err != nil {
		return err
	}

	// Check for items
	itemCount, err := s.repo.CountCategoryItems(ctx, categoryID)
	if err != nil {
		return err
	}
	if itemCount > 0 {
		return ErrCategoryHasItems
	}

	// Check for children
	childCount, err := s.repo.CountCategoryChildren(ctx, categoryID)
	if err != nil {
		return err
	}
	if childCount > 0 {
		return ErrCategoryHasChildren
	}

	if err := s.repo.DeleteCategory(ctx, tenantID, categoryID); err != nil {
		return err
	}
	s.invalidateCatalogCache(ctx, &tenantID)
	return nil
}

// --- CatalogItem Operations ---

// CreateCatalogItemRequest represents a request to create a catalog item.
type CreateCatalogItemRequest struct {
	TenantID        uuid.UUID
	OutletID        uuid.UUID
	CategoryID      uuid.UUID
	InventoryItemID *uuid.UUID
	RecipeID        *uuid.UUID
	Name            string
	Description     string
	BasePrice       float64
	Currency        string
	ImageURL        string
	IsAvailable     bool
	IsFeatured      bool
	LeadTimeMinutes int
	SKU             string
	DisplayOrder    int
}

// CreateCatalogItem creates a new catalog item.
func (s *Service) CreateCatalogItem(ctx context.Context, req CreateCatalogItemRequest) (*CatalogItem, error) {
	// Check for duplicate SKU if provided
	if req.SKU != "" {
		existing, err := s.repo.GetCatalogItemBySKU(ctx, req.TenantID, req.SKU)
		if err == nil && existing != nil {
			return nil, ErrCatalogItemAlreadyExists
		}
	}

	currency := req.Currency
	if currency == "" {
		currency = DefaultCurrency
	}

	item := &CatalogItem{
		TenantID:        req.TenantID,
		OutletID:        req.OutletID,
		CategoryID:      req.CategoryID,
		InventoryItemID: req.InventoryItemID,
		RecipeID:        req.RecipeID,
		Name:            req.Name,
		Description:     req.Description,
		BasePrice:       req.BasePrice,
		Currency:        currency,
		ImageURL:        req.ImageURL,
		IsAvailable:     req.IsAvailable,
		IsFeatured:      req.IsFeatured,
		LeadTimeMinutes: req.LeadTimeMinutes,
		SKU:             req.SKU,
		DisplayOrder:    req.DisplayOrder,
	}

	if err := s.repo.CreateCatalogItem(ctx, item); err != nil {
		s.logger.Error("failed to create catalog item", zap.Error(err))
		return nil, err
	}

	s.logger.Info("catalog item created",
		zap.String("id", item.ID.String()))

	s.invalidateCatalogCache(ctx, &req.TenantID)
	return item, nil
}

// GetCatalogItem retrieves a catalog item by ID.
func (s *Service) GetCatalogItem(ctx context.Context, tenantID, itemID uuid.UUID) (*CatalogItem, error) {
	return s.repo.GetCatalogItem(ctx, tenantID, itemID)
}

// ListCatalogItems lists catalog items with optional filters.
func (s *Service) ListCatalogItems(ctx context.Context, filter CatalogItemFilter) ([]CatalogItem, int, error) {
	return s.repo.ListCatalogItems(ctx, filter)
}

// UpdateCatalogItemRequest represents a request to update a catalog item.
type UpdateCatalogItemRequest struct {
	TenantID        uuid.UUID
	ItemID          uuid.UUID
	CategoryID      *uuid.UUID
	RecipeID        *uuid.UUID
	Name            *string
	Description     *string
	BasePrice       *float64
	Currency        *string
	ImageURL        *string
	IsAvailable     *bool
	IsFeatured      *bool
	LeadTimeMinutes *int
	SKU             *string
	DisplayOrder    *int
}

// UpdateCatalogItem updates an existing catalog item.
func (s *Service) UpdateCatalogItem(ctx context.Context, req UpdateCatalogItemRequest) (*CatalogItem, error) {
	item, err := s.repo.GetCatalogItem(ctx, req.TenantID, req.ItemID)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		item.Name = *req.Name
	}
	if req.Description != nil {
		item.Description = *req.Description
	}
	if req.BasePrice != nil {
		item.BasePrice = *req.BasePrice
	}
	if req.Currency != nil {
		item.Currency = *req.Currency
	}
	if req.ImageURL != nil {
		item.ImageURL = *req.ImageURL
	}
	if req.IsAvailable != nil {
		item.IsAvailable = *req.IsAvailable
	}
	if req.IsFeatured != nil {
		item.IsFeatured = *req.IsFeatured
	}
	if req.RecipeID != nil {
		item.RecipeID = req.RecipeID
	}
	if req.LeadTimeMinutes != nil {
		item.LeadTimeMinutes = *req.LeadTimeMinutes
	}

	if req.SKU != nil && *req.SKU != item.SKU {
		item.SKU = *req.SKU
	}

	if req.DisplayOrder != nil {
		item.DisplayOrder = *req.DisplayOrder
	}

	if req.CategoryID != nil {
		/*
		category, err := s.repo.GetCategory(ctx, req.TenantID, *req.CategoryID)
		if err != nil {
			return nil, ErrInvalidCategory
		}
		*/
		item.CategoryID = *req.CategoryID
	}

	if err := s.repo.UpdateCatalogItem(ctx, item); err != nil {
		s.logger.Error("failed to update catalog item", zap.Error(err))
		return nil, err
	}

	s.logger.Info("catalog item updated", zap.String("id", item.ID.String()))
	s.invalidateCatalogCache(ctx, &req.TenantID)
	return item, nil
}

// DeleteCatalogItem deletes a catalog item.
func (s *Service) DeleteCatalogItem(ctx context.Context, tenantID, itemID uuid.UUID) error {
	// Check if item exists
	_, err := s.repo.GetCatalogItem(ctx, tenantID, itemID)
	if err != nil {
		return err
	}

	if err := s.repo.DeleteCatalogItem(ctx, tenantID, itemID); err != nil {
		s.logger.Error("failed to delete catalog item", zap.Error(err))
		return err
	}

	s.logger.Info("catalog item deleted", zap.String("id", itemID.String()))
	s.invalidateCatalogCache(ctx, &tenantID)
	return nil
}

// --- DietaryTag Operations ---

// CreateDietaryTag creates a new dietary tag.
func (s *Service) CreateDietaryTag(ctx context.Context, tag *DietaryTag) error {
	return s.repo.CreateDietaryTag(ctx, tag)
}

// GetDietaryTag retrieves a dietary tag.
func (s *Service) GetDietaryTag(ctx context.Context, code string) (*DietaryTag, error) {
	return s.repo.GetDietaryTag(ctx, code)
}

// ListDietaryTags lists all dietary tags.
// ListDietaryTags returns all dietary tags (cached 5 min — static reference data).
func (s *Service) ListDietaryTags(ctx context.Context) ([]DietaryTag, error) {
	if s.cache != nil {
		var result []DietaryTag
		err := s.cache.GetOrSet(ctx, "dietary-tags", &result, cacheTTLReference, func() (interface{}, error) {
			return s.repo.ListDietaryTags(ctx)
		})
		return result, err
	}
	return s.repo.ListDietaryTags(ctx)
}

// AddDietaryTagToItem adds a dietary tag to a catalog item.
func (s *Service) AddDietaryTagToItem(ctx context.Context, tenantID, catalogItemID uuid.UUID, tagCode string) error {
	// Validate catalog item exists
	_, err := s.repo.GetCatalogItem(ctx, tenantID, catalogItemID)
	if err != nil {
		return ErrCatalogItemNotFound
	}

	// Validate tag exists
	_, err = s.repo.GetDietaryTag(ctx, tagCode)
	if err != nil {
		return ErrDietaryTagNotFound
	}

	return s.repo.AddDietaryTagToItem(ctx, catalogItemID, tagCode)
}

// RemoveDietaryTagFromItem removes a dietary tag from a catalog item.
func (s *Service) RemoveDietaryTagFromItem(ctx context.Context, catalogItemID uuid.UUID, tagCode string) error {
	return s.repo.RemoveDietaryTagFromItem(ctx, catalogItemID, tagCode)
}

// ToggleFavorite toggles whether a catalog item is a favorite for the specified user.
func (s *Service) ToggleFavorite(ctx context.Context, userID, itemID uuid.UUID) (bool, error) {
	return s.repo.ToggleFavorite(ctx, userID, itemID)
}

// --- Asset Operations ---

// CreateAsset creates a new asset for a catalog item.
func (s *Service) CreateAsset(ctx context.Context, tenantID uuid.UUID, asset *Asset) error {
	// Validate catalog item exists
	_, err := s.repo.GetCatalogItem(ctx, tenantID, asset.CatalogItemID)
	if err != nil {
		return ErrCatalogItemNotFound
	}

	// Validate asset type
	if asset.AssetType != AssetTypeImage && asset.AssetType != AssetTypeVideo {
		return ErrInvalidAssetType
	}

	// Validate URL
	if strings.TrimSpace(asset.URL) == "" {
		return ErrInvalidAssetURL
	}

	return s.repo.CreateAsset(ctx, asset)
}

// ListAssets lists all assets for a menu item.
func (s *Service) ListAssets(ctx context.Context, catalogItemID uuid.UUID) ([]Asset, error) {
	return s.repo.ListAssets(ctx, catalogItemID)
}

// DeleteAsset deletes an asset.
func (s *Service) DeleteAsset(ctx context.Context, assetID uuid.UUID) error {
	return s.repo.DeleteAsset(ctx, assetID)
}

// --- Schedule Operations ---

// CreateSchedule creates a new schedule for a catalog item.
func (s *Service) CreateSchedule(ctx context.Context, tenantID uuid.UUID, schedule *Schedule) error {
	// Validate catalog item exists
	_, err := s.repo.GetCatalogItem(ctx, tenantID, schedule.CatalogItemID)
	if err != nil {
		return ErrCatalogItemNotFound
	}

	// Validate day of week
	if schedule.DayOfWeek < 0 || schedule.DayOfWeek > 6 {
		return ErrInvalidDayOfWeek
	}

	// Validate time format (basic check)
	if !isValidTimeFormat(schedule.TimeStart) || !isValidTimeFormat(schedule.TimeEnd) {
		return ErrInvalidScheduleTime
	}

	return s.repo.CreateSchedule(ctx, schedule)
}

// ListSchedules lists all schedules for a menu item.
func (s *Service) ListSchedules(ctx context.Context, catalogItemID uuid.UUID) ([]Schedule, error) {
	return s.repo.ListSchedules(ctx, catalogItemID)
}

// UpdateSchedule updates a schedule.
func (s *Service) UpdateSchedule(ctx context.Context, schedule *Schedule) error {
	return s.repo.UpdateSchedule(ctx, schedule)
}

// DeleteSchedule deletes a schedule.
func (s *Service) DeleteSchedule(ctx context.Context, scheduleID uuid.UUID) error {
	return s.repo.DeleteSchedule(ctx, scheduleID)
}

// --- Public API Operations ---

// GetPublicMenu retrieves the public menu. Caller must set req.TenantID.
// Delegates to the repository which correctly maps all fields.
func (s *Service) GetPublicMenu(ctx context.Context, req PublicCatalogRequest) ([]PublicCatalogItem, int, error) {
	return s.repo.GetPublicMenu(ctx, req)
}

// GetPublicCatalogItem retrieves a single catalog item for public display.
func (s *Service) GetPublicCatalogItem(ctx context.Context, tenantID, itemID uuid.UUID, locale string) (*PublicCatalogItem, error) {
	return s.repo.GetPublicCatalogItem(ctx, tenantID, itemID, locale)
}

// GetPublicCategories retrieves public categories (cached 5 min).
func (s *Service) GetPublicCategories(ctx context.Context, tenantID, outletID uuid.UUID) ([]PublicCategory, error) {
	if s.cache != nil {
		key := fmt.Sprintf("pub:categories:%s:%s", tenantID, outletID)
		var result []PublicCategory
		err := s.cache.GetOrSet(ctx, key, &result, cacheTTLReference, func() (interface{}, error) {
			return s.repo.GetPublicCategories(ctx, tenantID, outletID)
		})
		return result, err
	}
	return s.repo.GetPublicCategories(ctx, tenantID, outletID)
}

// ListOutlets returns all outlets for a tenant (cached 5 min).
func (s *Service) ListOutlets(ctx context.Context, tenantID uuid.UUID) ([]OutletSummary, error) {
	if s.cache != nil {
		key := fmt.Sprintf("outlets:%s", tenantID)
		var result []OutletSummary
		err := s.cache.GetOrSet(ctx, key, &result, cacheTTLReference, func() (interface{}, error) {
			return s.repo.ListOutlets(ctx, tenantID)
		})
		return result, err
	}
	return s.repo.ListOutlets(ctx, tenantID)
}

// resolveDefaultOutlet returns the first outlet for a tenant.
func (s *Service) resolveDefaultOutlet(ctx context.Context, tenantID uuid.UUID) (uuid.UUID, error) {
	outlets, err := s.ListOutlets(ctx, tenantID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("catalog: list outlets for tenant: %w", err)
	}
	if len(outlets) == 0 {
		return uuid.Nil, fmt.Errorf("catalog: no outlets found for tenant %s", tenantID)
	}
	return outlets[0].ID, nil
}

// --- Helper Functions ---

func isValidLocale(locale string) bool {
	for _, l := range SupportedLocales() {
		if strings.EqualFold(l, locale) {
			return true
		}
	}
	return false
}

func isValidTimeFormat(t string) bool {
	// Basic HH:MM format validation
	if len(t) != 5 {
		return false
	}
	if t[2] != ':' {
		return false
	}
	return true
}
