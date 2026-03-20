package catalog

import (
	"time"

	"github.com/google/uuid"
)

// Category represents a menu category for organizing items.
type Category struct {
	ID           uuid.UUID   `json:"id"`
	TenantID     *uuid.UUID  `json:"tenantId,omitempty"`
	OutletID     *uuid.UUID  `json:"outletId,omitempty"`
	ParentID     *uuid.UUID  `json:"parentId,omitempty"`
	Name         string      `json:"name"`
	Slug         string      `json:"slug,omitempty"`
	Description  string      `json:"description,omitempty"`
	ImageURL     string      `json:"imageUrl,omitempty"`
	DisplayOrder int         `json:"displayOrder"`
	IsActive     bool        `json:"isActive"`
	Children     []Category  `json:"children,omitempty"`
	ItemCount    int         `json:"itemCount,omitempty"`
	CreatedAt    time.Time   `json:"createdAt"`
	UpdatedAt    time.Time   `json:"updatedAt"`
}

// CatalogItem represents a catalog item available for ordering.
type CatalogItem struct {
	ID              uuid.UUID              `json:"id"`
	TenantID        uuid.UUID              `json:"tenantId"`
	OutletID        uuid.UUID              `json:"outletId"`
	InventoryItemID *uuid.UUID             `json:"inventoryItemId,omitempty"`
	CategoryID      uuid.UUID              `json:"categoryId"`
	Category        *Category              `json:"category,omitempty"`
	Name            string                 `json:"name"`      // Projected from inventory
	Description     string                 `json:"description,omitempty"`
	BasePrice       float64                `json:"basePrice"` // Projected from inventory
	Currency        string                 `json:"currency"`
	IsAvailable     bool                   `json:"isAvailable"`
	IsFeatured      bool                   `json:"isFeatured"`
	LeadTimeMinutes int                    `json:"leadTimeMinutes,omitempty"`
	RecipeID        *uuid.UUID             `json:"recipeId,omitempty"`
	SKU             string                 `json:"sku,omitempty"`
	ImageURL        string                 `json:"imageUrl,omitempty"`
	DisplayOrder    int                    `json:"displayOrder"`
	DietaryTags     []DietaryTag           `json:"dietaryTags,omitempty"`
	Assets          []Asset                `json:"assets,omitempty"`
	Schedules       []Schedule             `json:"schedules,omitempty"`
	Variants        []Variant              `json:"variants,omitempty"`
	IsFavorite      bool                   `json:"isFavorite"`
	CreatedAt       time.Time              `json:"createdAt"`
	UpdatedAt       time.Time              `json:"updatedAt"`
}

// Variant represents a specific version of a catalog item (e.g., size, flavor).
type Variant struct {
	ID          uuid.UUID `json:"id"`
	CatalogItemID uuid.UUID `json:"catalogItemId"`
	Name        string    `json:"name"`
	PriceDelta  float64   `json:"priceDelta"`
	IsAvailable bool      `json:"isAvailable"`
	SKU         string    `json:"sku,omitempty"`
}


// DietaryTag represents a dietary restriction or preference tag.
type DietaryTag struct {
	Code        string    `json:"code"` // Primary key: vegetarian, vegan, gluten-free, etc.
	Label       string    `json:"label"`
	Description string    `json:"description,omitempty"`
	IconURL     string    `json:"iconUrl,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

// Asset represents media associated with a catalog item.
type Asset struct {
	ID         uuid.UUID              `json:"id"`
	CatalogItemID uuid.UUID              `json:"catalogItemId"`
	AssetType  AssetType              `json:"assetType"`
	URL        string                 `json:"url"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt  time.Time              `json:"createdAt"`
}

// AssetType enumerates supported asset types.
type AssetType string

const (
	AssetTypeImage AssetType = "image"
	AssetTypeVideo AssetType = "video"
)

// Schedule represents an availability window for a catalog item.
type Schedule struct {
	ID         uuid.UUID `json:"id"`
	CatalogItemID uuid.UUID `json:"catalogItemId"`
	DayOfWeek  int       `json:"dayOfWeek"` // 0=Sunday, 6=Saturday
	TimeStart  string    `json:"timeStart"` // HH:MM format
	TimeEnd    string    `json:"timeEnd"`   // HH:MM format
	CreatedAt  time.Time `json:"createdAt"`
}

// Supported locales for translations.
const (
	LocaleEnglish = "en"
	LocaleSwahili = "sw"
)

// SupportedLocales returns the list of supported locales.
func SupportedLocales() []string {
	return []string{LocaleEnglish, LocaleSwahili}
}

// DefaultCurrency is the default currency for menu items.
const DefaultCurrency = "KES"

// CategoryFilter defines filter options for listing categories.
type CategoryFilter struct {
	TenantID uuid.UUID
	OutletID *uuid.UUID
	ParentID *uuid.UUID
	IsActive *bool
	Search   string
	Limit    int
	Offset   int
}

// CatalogItemFilter defines filter options for listing catalog items.
type CatalogItemFilter struct {
	TenantID    uuid.UUID
	OutletID    *uuid.UUID
	CategoryID  *uuid.UUID
	IsAvailable *bool
	Search      string
	DietaryTags []string
	MinPrice    *float64
	MaxPrice    *float64
	Locale      string
	Limit       int
	Offset      int
	UserID      *uuid.UUID // ID of the user whose favorites we are interested in
	FavoriteOnly bool       // Filter for favorites only
}

// PublicCatalogRequest represents a request for the public catalog API.
type PublicCatalogRequest struct {
	TenantID   uuid.UUID
	TenantSlug string
	OutletID   *uuid.UUID
	CategoryID *uuid.UUID
	Locale     string
	DietaryTags []string
	Search     string
	Limit      int
	Offset     int
	UserID     *uuid.UUID // Authenticated user ID
	FavoriteOnly bool       // Filter for favorites only
}

// PublicCatalogItem is a read-only view of a catalog item for public consumption.
type PublicCatalogItem struct {
	ID              uuid.UUID    `json:"id"`
	CategoryID      uuid.UUID    `json:"categoryId"`
	CategoryName    string       `json:"categoryName"`
	Name            string       `json:"name"`
	Description     string       `json:"description,omitempty"`
	BasePrice       float64      `json:"basePrice"`
	Currency        string       `json:"currency"`
	ImageURL        string       `json:"imageUrl,omitempty"`
	LeadTimeMinutes int          `json:"leadTimeMinutes,omitempty"`
	DietaryTags     []DietaryTag `json:"dietaryTags,omitempty"`
	IsFavorite      bool         `json:"isFavorite"`
}

// PublicCategory is a read-only view of a category for public consumption.
type PublicCategory struct {
	ID          uuid.UUID        `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	ImageURL    string           `json:"imageUrl,omitempty"`
	ItemCount   int              `json:"itemCount"`
	Children    []PublicCategory `json:"children,omitempty"`
}

// OutletSummary is a minimal outlet for listing (id and display name).
type OutletSummary struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	ImageURL string    `json:"imageUrl,omitempty"`
}
