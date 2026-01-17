package catalog

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Service provides catalog business logic.
type Service struct {
	repo   Repository
	logger *zap.Logger
}

// NewService creates a new catalog service.
func NewService(repo Repository, logger *zap.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
	}
}

// --- Category Operations ---

// CreateCategoryRequest represents a request to create a category.
type CreateCategoryRequest struct {
	TenantID     uuid.UUID
	CafeID       uuid.UUID
	ParentID     *uuid.UUID
	Name         string
	Description  string
	DisplayOrder int
	IsActive     bool
	ImageURL     string
}

// CreateCategory creates a new menu category.
func (s *Service) CreateCategory(ctx context.Context, req CreateCategoryRequest) (*Category, error) {
	// Validate name is not empty
	if strings.TrimSpace(req.Name) == "" {
		return nil, ErrCategoryNotFound // Should be validation error
	}

	// Check for duplicate name in same cafe
	existing, err := s.repo.GetCategoryByName(ctx, req.TenantID, req.CafeID, req.Name)
	if err == nil && existing != nil {
		return nil, ErrCategoryAlreadyExists
	}

	// Validate parent category exists if specified
	if req.ParentID != nil {
		parent, err := s.repo.GetCategory(ctx, req.TenantID, *req.ParentID)
		if err != nil {
			return nil, ErrInvalidCategoryParent
		}
		// Ensure parent is in same cafe
		if parent.CafeID != req.CafeID {
			return nil, ErrInvalidCategoryParent
		}
	}

	category := &Category{
		TenantID:     req.TenantID,
		CafeID:       req.CafeID,
		ParentID:     req.ParentID,
		Name:         strings.TrimSpace(req.Name),
		Description:  req.Description,
		DisplayOrder: req.DisplayOrder,
		IsActive:     req.IsActive,
		ImageURL:     req.ImageURL,
	}

	if err := s.repo.CreateCategory(ctx, category); err != nil {
		s.logger.Error("failed to create category", zap.Error(err))
		return nil, err
	}

	s.logger.Info("category created",
		zap.String("id", category.ID.String()),
		zap.String("name", category.Name))

	return category, nil
}

// GetCategory retrieves a category by ID.
func (s *Service) GetCategory(ctx context.Context, tenantID, categoryID uuid.UUID) (*Category, error) {
	return s.repo.GetCategory(ctx, tenantID, categoryID)
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
	Description  *string
	DisplayOrder *int
	IsActive     *bool
	ImageURL     *string
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
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, ErrCategoryNotFound // Should be validation error
		}
		// Check for duplicate name if changed
		if name != category.Name {
			existing, err := s.repo.GetCategoryByName(ctx, req.TenantID, category.CafeID, name)
			if err == nil && existing != nil && existing.ID != category.ID {
				return nil, ErrCategoryAlreadyExists
			}
		}
		category.Name = name
	}

	if req.Description != nil {
		category.Description = *req.Description
	}
	if req.DisplayOrder != nil {
		category.DisplayOrder = *req.DisplayOrder
	}
	if req.IsActive != nil {
		category.IsActive = *req.IsActive
	}
	if req.ImageURL != nil {
		category.ImageURL = *req.ImageURL
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
		if parent.CafeID != category.CafeID {
			return nil, ErrInvalidCategoryParent
		}
		category.ParentID = req.ParentID
	}

	if err := s.repo.UpdateCategory(ctx, category); err != nil {
		s.logger.Error("failed to update category", zap.Error(err))
		return nil, err
	}

	s.logger.Info("category updated",
		zap.String("id", category.ID.String()),
		zap.String("name", category.Name))

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
		s.logger.Error("failed to delete category", zap.Error(err))
		return err
	}

	s.logger.Info("category deleted", zap.String("id", categoryID.String()))
	return nil
}

// --- MenuItem Operations ---

// CreateMenuItemRequest represents a request to create a menu item.
type CreateMenuItemRequest struct {
	TenantID        uuid.UUID
	CafeID          uuid.UUID
	CategoryID      uuid.UUID
	Name            string
	Description     string
	BasePrice       float64
	Currency        string
	IsAvailable     bool
	LeadTimeMinutes int
	ImageURL        string
	SKU             string
	DisplayOrder    int
}

// CreateMenuItem creates a new menu item.
func (s *Service) CreateMenuItem(ctx context.Context, req CreateMenuItemRequest) (*MenuItem, error) {
	// Validate name
	if strings.TrimSpace(req.Name) == "" {
		return nil, ErrMenuItemNotFound // Should be validation error
	}

	// Validate price
	if req.BasePrice < 0 {
		return nil, ErrInvalidPrice
	}

	// Validate category exists
	category, err := s.repo.GetCategory(ctx, req.TenantID, req.CategoryID)
	if err != nil {
		return nil, ErrInvalidCategory
	}
	if category.CafeID != req.CafeID {
		return nil, ErrInvalidCategory
	}

	// Check for duplicate SKU if provided
	if req.SKU != "" {
		existing, err := s.repo.GetMenuItemBySKU(ctx, req.TenantID, req.SKU)
		if err == nil && existing != nil {
			return nil, ErrMenuItemAlreadyExists
		}
	}

	currency := req.Currency
	if currency == "" {
		currency = DefaultCurrency
	}

	item := &MenuItem{
		TenantID:        req.TenantID,
		CafeID:          req.CafeID,
		CategoryID:      req.CategoryID,
		Name:            strings.TrimSpace(req.Name),
		Description:     req.Description,
		BasePrice:       req.BasePrice,
		Currency:        currency,
		IsAvailable:     req.IsAvailable,
		LeadTimeMinutes: req.LeadTimeMinutes,
		ImageURL:        req.ImageURL,
		SKU:             req.SKU,
		DisplayOrder:    req.DisplayOrder,
	}

	if err := s.repo.CreateMenuItem(ctx, item); err != nil {
		s.logger.Error("failed to create menu item", zap.Error(err))
		return nil, err
	}

	s.logger.Info("menu item created",
		zap.String("id", item.ID.String()),
		zap.String("name", item.Name))

	return item, nil
}

// GetMenuItem retrieves a menu item by ID.
func (s *Service) GetMenuItem(ctx context.Context, tenantID, itemID uuid.UUID) (*MenuItem, error) {
	return s.repo.GetMenuItem(ctx, tenantID, itemID)
}

// ListMenuItems lists menu items with optional filters.
func (s *Service) ListMenuItems(ctx context.Context, filter MenuItemFilter) ([]MenuItem, int, error) {
	return s.repo.ListMenuItems(ctx, filter)
}

// UpdateMenuItemRequest represents a request to update a menu item.
type UpdateMenuItemRequest struct {
	TenantID        uuid.UUID
	ItemID          uuid.UUID
	CategoryID      *uuid.UUID
	Name            *string
	Description     *string
	BasePrice       *float64
	Currency        *string
	IsAvailable     *bool
	LeadTimeMinutes *int
	ImageURL        *string
	SKU             *string
	DisplayOrder    *int
}

// UpdateMenuItem updates an existing menu item.
func (s *Service) UpdateMenuItem(ctx context.Context, req UpdateMenuItemRequest) (*MenuItem, error) {
	item, err := s.repo.GetMenuItem(ctx, req.TenantID, req.ItemID)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, ErrMenuItemNotFound // Should be validation error
		}
		item.Name = name
	}

	if req.Description != nil {
		item.Description = *req.Description
	}

	if req.BasePrice != nil {
		if *req.BasePrice < 0 {
			return nil, ErrInvalidPrice
		}
		item.BasePrice = *req.BasePrice
	}

	if req.Currency != nil {
		item.Currency = *req.Currency
	}

	if req.IsAvailable != nil {
		item.IsAvailable = *req.IsAvailable
	}

	if req.LeadTimeMinutes != nil {
		item.LeadTimeMinutes = *req.LeadTimeMinutes
	}

	if req.ImageURL != nil {
		item.ImageURL = *req.ImageURL
	}

	if req.SKU != nil && *req.SKU != item.SKU {
		// Check for duplicate SKU
		existing, err := s.repo.GetMenuItemBySKU(ctx, req.TenantID, *req.SKU)
		if err == nil && existing != nil && existing.ID != item.ID {
			return nil, ErrMenuItemAlreadyExists
		}
		item.SKU = *req.SKU
	}

	if req.DisplayOrder != nil {
		item.DisplayOrder = *req.DisplayOrder
	}

	if req.CategoryID != nil {
		category, err := s.repo.GetCategory(ctx, req.TenantID, *req.CategoryID)
		if err != nil {
			return nil, ErrInvalidCategory
		}
		if category.CafeID != item.CafeID {
			return nil, ErrInvalidCategory
		}
		item.CategoryID = *req.CategoryID
	}

	if err := s.repo.UpdateMenuItem(ctx, item); err != nil {
		s.logger.Error("failed to update menu item", zap.Error(err))
		return nil, err
	}

	s.logger.Info("menu item updated",
		zap.String("id", item.ID.String()),
		zap.String("name", item.Name))

	return item, nil
}

// DeleteMenuItem deletes a menu item.
func (s *Service) DeleteMenuItem(ctx context.Context, tenantID, itemID uuid.UUID) error {
	// Check if item exists
	_, err := s.repo.GetMenuItem(ctx, tenantID, itemID)
	if err != nil {
		return err
	}

	if err := s.repo.DeleteMenuItem(ctx, tenantID, itemID); err != nil {
		s.logger.Error("failed to delete menu item", zap.Error(err))
		return err
	}

	s.logger.Info("menu item deleted", zap.String("id", itemID.String()))
	return nil
}

// --- Variant Operations ---

// CreateVariantRequest represents a request to create a variant.
type CreateVariantRequest struct {
	TenantID     uuid.UUID
	MenuItemID   uuid.UUID
	Name         string
	PriceDelta   float64
	IsAvailable  bool
	SKU          string
	DisplayOrder int
}

// CreateVariant creates a new menu item variant.
func (s *Service) CreateVariant(ctx context.Context, req CreateVariantRequest) (*Variant, error) {
	// Validate menu item exists
	_, err := s.repo.GetMenuItem(ctx, req.TenantID, req.MenuItemID)
	if err != nil {
		return nil, ErrMenuItemNotFound
	}

	variant := &Variant{
		MenuItemID:   req.MenuItemID,
		Name:         strings.TrimSpace(req.Name),
		PriceDelta:   req.PriceDelta,
		IsAvailable:  req.IsAvailable,
		SKU:          req.SKU,
		DisplayOrder: req.DisplayOrder,
	}

	if err := s.repo.CreateVariant(ctx, variant); err != nil {
		s.logger.Error("failed to create variant", zap.Error(err))
		return nil, err
	}

	return variant, nil
}

// ListVariants lists variants for a menu item.
func (s *Service) ListVariants(ctx context.Context, menuItemID uuid.UUID) ([]Variant, error) {
	return s.repo.ListVariants(ctx, menuItemID)
}

// UpdateVariant updates a variant.
func (s *Service) UpdateVariant(ctx context.Context, variant *Variant) error {
	return s.repo.UpdateVariant(ctx, variant)
}

// DeleteVariant deletes a variant.
func (s *Service) DeleteVariant(ctx context.Context, variantID uuid.UUID) error {
	return s.repo.DeleteVariant(ctx, variantID)
}

// --- Translation Operations ---

// CreateTranslationRequest represents a request to create a translation.
type CreateTranslationRequest struct {
	TenantID    uuid.UUID
	MenuItemID  uuid.UUID
	Locale      string
	Name        string
	Description string
}

// CreateTranslation creates a new translation for a menu item.
func (s *Service) CreateTranslation(ctx context.Context, req CreateTranslationRequest) (*Translation, error) {
	// Validate locale
	if !isValidLocale(req.Locale) {
		return nil, ErrInvalidLocale
	}

	// Validate menu item exists
	_, err := s.repo.GetMenuItem(ctx, req.TenantID, req.MenuItemID)
	if err != nil {
		return nil, ErrMenuItemNotFound
	}

	// Check for existing translation
	existing, err := s.repo.GetTranslation(ctx, req.MenuItemID, req.Locale)
	if err == nil && existing != nil {
		return nil, ErrTranslationAlreadyExists
	}

	translation := &Translation{
		MenuItemID:  req.MenuItemID,
		Locale:      req.Locale,
		Name:        strings.TrimSpace(req.Name),
		Description: req.Description,
	}

	if err := s.repo.CreateTranslation(ctx, translation); err != nil {
		s.logger.Error("failed to create translation", zap.Error(err))
		return nil, err
	}

	return translation, nil
}

// GetTranslation retrieves a translation.
func (s *Service) GetTranslation(ctx context.Context, menuItemID uuid.UUID, locale string) (*Translation, error) {
	return s.repo.GetTranslation(ctx, menuItemID, locale)
}

// ListTranslations lists all translations for a menu item.
func (s *Service) ListTranslations(ctx context.Context, menuItemID uuid.UUID) ([]Translation, error) {
	return s.repo.ListTranslations(ctx, menuItemID)
}

// UpdateTranslation updates a translation.
func (s *Service) UpdateTranslation(ctx context.Context, translation *Translation) error {
	return s.repo.UpdateTranslation(ctx, translation)
}

// DeleteTranslation deletes a translation.
func (s *Service) DeleteTranslation(ctx context.Context, menuItemID uuid.UUID, locale string) error {
	return s.repo.DeleteTranslation(ctx, menuItemID, locale)
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
func (s *Service) ListDietaryTags(ctx context.Context) ([]DietaryTag, error) {
	return s.repo.ListDietaryTags(ctx)
}

// AddDietaryTagToItem adds a dietary tag to a menu item.
func (s *Service) AddDietaryTagToItem(ctx context.Context, tenantID, menuItemID uuid.UUID, tagCode string) error {
	// Validate menu item exists
	_, err := s.repo.GetMenuItem(ctx, tenantID, menuItemID)
	if err != nil {
		return ErrMenuItemNotFound
	}

	// Validate tag exists
	_, err = s.repo.GetDietaryTag(ctx, tagCode)
	if err != nil {
		return ErrDietaryTagNotFound
	}

	return s.repo.AddDietaryTagToItem(ctx, menuItemID, tagCode)
}

// RemoveDietaryTagFromItem removes a dietary tag from a menu item.
func (s *Service) RemoveDietaryTagFromItem(ctx context.Context, menuItemID uuid.UUID, tagCode string) error {
	return s.repo.RemoveDietaryTagFromItem(ctx, menuItemID, tagCode)
}

// --- Asset Operations ---

// CreateAsset creates a new asset for a menu item.
func (s *Service) CreateAsset(ctx context.Context, tenantID uuid.UUID, asset *Asset) error {
	// Validate menu item exists
	_, err := s.repo.GetMenuItem(ctx, tenantID, asset.MenuItemID)
	if err != nil {
		return ErrMenuItemNotFound
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
func (s *Service) ListAssets(ctx context.Context, menuItemID uuid.UUID) ([]Asset, error) {
	return s.repo.ListAssets(ctx, menuItemID)
}

// DeleteAsset deletes an asset.
func (s *Service) DeleteAsset(ctx context.Context, assetID uuid.UUID) error {
	return s.repo.DeleteAsset(ctx, assetID)
}

// --- Schedule Operations ---

// CreateSchedule creates a new schedule for a menu item.
func (s *Service) CreateSchedule(ctx context.Context, tenantID uuid.UUID, schedule *Schedule) error {
	// Validate menu item exists
	_, err := s.repo.GetMenuItem(ctx, tenantID, schedule.MenuItemID)
	if err != nil {
		return ErrMenuItemNotFound
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
func (s *Service) ListSchedules(ctx context.Context, menuItemID uuid.UUID) ([]Schedule, error) {
	return s.repo.ListSchedules(ctx, menuItemID)
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

// GetPublicMenu retrieves the public menu.
func (s *Service) GetPublicMenu(ctx context.Context, req PublicMenuRequest) ([]PublicMenuItem, int, error) {
	// Set default locale
	if req.Locale == "" {
		req.Locale = LocaleEnglish
	}

	// Set default limit
	if req.Limit == 0 {
		req.Limit = 50
	}

	return s.repo.GetPublicMenu(ctx, req)
}

// GetPublicCategories retrieves public categories.
func (s *Service) GetPublicCategories(ctx context.Context, tenantID, cafeID uuid.UUID) ([]PublicCategory, error) {
	return s.repo.GetPublicCategories(ctx, tenantID, cafeID)
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
