package catalog

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines the interface for catalog data persistence.
type Repository interface {
	// Categories
	CreateCategory(ctx context.Context, category *Category) error
	GetCategory(ctx context.Context, tenantID, categoryID uuid.UUID) (*Category, error)
	ListCategories(ctx context.Context, filter CategoryFilter) ([]Category, int, error)
	UpdateCategory(ctx context.Context, category *Category) error
	DeleteCategory(ctx context.Context, tenantID, categoryID uuid.UUID) error
	CountCategoryItems(ctx context.Context, categoryID uuid.UUID) (int, error)
	CountCategoryChildren(ctx context.Context, categoryID uuid.UUID) (int, error)
	// Catalog Items
	CreateCatalogItem(ctx context.Context, item *CatalogItem) error
	GetCatalogItemBySKU(ctx context.Context, tenantID uuid.UUID, sku string) (*CatalogItem, error)
	GetCatalogItem(ctx context.Context, tenantID, itemID uuid.UUID) (*CatalogItem, error)
	ListCatalogItems(ctx context.Context, filter CatalogItemFilter) ([]CatalogItem, int, error)
	UpdateCatalogItem(ctx context.Context, item *CatalogItem) error
	DeleteCatalogItem(ctx context.Context, tenantID, itemID uuid.UUID) error
	ToggleFavorite(ctx context.Context, userID, itemID uuid.UUID) (bool, error)

	// Dietary Tags
	CreateDietaryTag(ctx context.Context, tag *DietaryTag) error
	GetDietaryTag(ctx context.Context, code string) (*DietaryTag, error)
	ListDietaryTags(ctx context.Context) ([]DietaryTag, error)
	DeleteDietaryTag(ctx context.Context, code string) error
	AddDietaryTagToItem(ctx context.Context, catalogItemID uuid.UUID, tagCode string) error
	RemoveDietaryTagFromItem(ctx context.Context, catalogItemID uuid.UUID, tagCode string) error
	ListItemDietaryTags(ctx context.Context, catalogItemID uuid.UUID) ([]DietaryTag, error)

	// Assets
	CreateAsset(ctx context.Context, asset *Asset) error
	GetAsset(ctx context.Context, assetID uuid.UUID) (*Asset, error)
	ListAssets(ctx context.Context, catalogItemID uuid.UUID) ([]Asset, error)
	DeleteAsset(ctx context.Context, assetID uuid.UUID) error

	// Schedules
	CreateSchedule(ctx context.Context, schedule *Schedule) error
	GetSchedule(ctx context.Context, scheduleID uuid.UUID) (*Schedule, error)
	ListSchedules(ctx context.Context, catalogItemID uuid.UUID) ([]Schedule, error)
	UpdateSchedule(ctx context.Context, schedule *Schedule) error
	DeleteSchedule(ctx context.Context, scheduleID uuid.UUID) error

	// Public API helpers
	GetPublicMenu(ctx context.Context, req PublicCatalogRequest) ([]PublicCatalogItem, int, error)
	GetPublicCatalogItem(ctx context.Context, tenantID, itemID uuid.UUID, locale string) (*PublicCatalogItem, error)
	GetPublicCategories(ctx context.Context, tenantID, outletID uuid.UUID) ([]PublicCategory, error)

	// Outlets
	ListOutlets(ctx context.Context, tenantID uuid.UUID) ([]OutletSummary, error)
	GetOutlet(ctx context.Context, tenantID, outletID uuid.UUID) (*OutletSummary, error)
}
