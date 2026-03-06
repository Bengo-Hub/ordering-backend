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
	GetCategoryByName(ctx context.Context, tenantID, cafeID uuid.UUID, name string) (*Category, error)
	ListCategories(ctx context.Context, filter CategoryFilter) ([]Category, int, error)
	UpdateCategory(ctx context.Context, category *Category) error
	DeleteCategory(ctx context.Context, tenantID, categoryID uuid.UUID) error
	CountCategoryItems(ctx context.Context, categoryID uuid.UUID) (int, error)
	CountCategoryChildren(ctx context.Context, categoryID uuid.UUID) (int, error)

	// Menu Items
	CreateMenuItem(ctx context.Context, item *MenuItem) error
	GetMenuItem(ctx context.Context, tenantID, itemID uuid.UUID) (*MenuItem, error)
	GetMenuItemBySKU(ctx context.Context, tenantID uuid.UUID, sku string) (*MenuItem, error)
	ListMenuItems(ctx context.Context, filter MenuItemFilter) ([]MenuItem, int, error)
	UpdateMenuItem(ctx context.Context, item *MenuItem) error
	DeleteMenuItem(ctx context.Context, tenantID, itemID uuid.UUID) error

	// Variants
	CreateVariant(ctx context.Context, variant *Variant) error
	GetVariant(ctx context.Context, variantID uuid.UUID) (*Variant, error)
	ListVariants(ctx context.Context, menuItemID uuid.UUID) ([]Variant, error)
	UpdateVariant(ctx context.Context, variant *Variant) error
	DeleteVariant(ctx context.Context, variantID uuid.UUID) error

	// Translations
	CreateTranslation(ctx context.Context, translation *Translation) error
	GetTranslation(ctx context.Context, menuItemID uuid.UUID, locale string) (*Translation, error)
	ListTranslations(ctx context.Context, menuItemID uuid.UUID) ([]Translation, error)
	UpdateTranslation(ctx context.Context, translation *Translation) error
	DeleteTranslation(ctx context.Context, menuItemID uuid.UUID, locale string) error

	// Dietary Tags
	CreateDietaryTag(ctx context.Context, tag *DietaryTag) error
	GetDietaryTag(ctx context.Context, code string) (*DietaryTag, error)
	ListDietaryTags(ctx context.Context) ([]DietaryTag, error)
	DeleteDietaryTag(ctx context.Context, code string) error
	AddDietaryTagToItem(ctx context.Context, menuItemID uuid.UUID, tagCode string) error
	RemoveDietaryTagFromItem(ctx context.Context, menuItemID uuid.UUID, tagCode string) error
	ListItemDietaryTags(ctx context.Context, menuItemID uuid.UUID) ([]DietaryTag, error)

	// Assets
	CreateAsset(ctx context.Context, asset *Asset) error
	GetAsset(ctx context.Context, assetID uuid.UUID) (*Asset, error)
	ListAssets(ctx context.Context, menuItemID uuid.UUID) ([]Asset, error)
	DeleteAsset(ctx context.Context, assetID uuid.UUID) error

	// Schedules
	CreateSchedule(ctx context.Context, schedule *Schedule) error
	GetSchedule(ctx context.Context, scheduleID uuid.UUID) (*Schedule, error)
	ListSchedules(ctx context.Context, menuItemID uuid.UUID) ([]Schedule, error)
	UpdateSchedule(ctx context.Context, schedule *Schedule) error
	DeleteSchedule(ctx context.Context, scheduleID uuid.UUID) error

	// Public API helpers
	GetPublicMenu(ctx context.Context, req PublicMenuRequest) ([]PublicMenuItem, int, error)
	GetPublicCategories(ctx context.Context, tenantID, cafeID uuid.UUID) ([]PublicCategory, error)

	// Cafes (distinct cafe/outlet IDs for a tenant, for frontend outlet list)
	GetDistinctCafeIDs(ctx context.Context, tenantID uuid.UUID) ([]uuid.UUID, error)
}
