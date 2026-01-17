package catalog

import (
	"time"

	"github.com/google/uuid"
)

// Category represents a menu category for organizing items.
type Category struct {
	ID           uuid.UUID   `json:"id"`
	TenantID     uuid.UUID   `json:"tenantId"`
	CafeID       uuid.UUID   `json:"cafeId"`
	ParentID     *uuid.UUID  `json:"parentId,omitempty"`
	Name         string      `json:"name"`
	Description  string      `json:"description,omitempty"`
	DisplayOrder int         `json:"displayOrder"`
	IsActive     bool        `json:"isActive"`
	ImageURL     string      `json:"imageUrl,omitempty"`
	Children     []Category  `json:"children,omitempty"`
	ItemCount    int         `json:"itemCount,omitempty"`
	CreatedAt    time.Time   `json:"createdAt"`
	UpdatedAt    time.Time   `json:"updatedAt"`
}

// MenuItem represents a menu item available for ordering.
type MenuItem struct {
	ID              uuid.UUID              `json:"id"`
	TenantID        uuid.UUID              `json:"tenantId"`
	CafeID          uuid.UUID              `json:"cafeId"`
	CategoryID      uuid.UUID              `json:"categoryId"`
	Category        *Category              `json:"category,omitempty"`
	Name            string                 `json:"name"`
	Description     string                 `json:"description,omitempty"`
	BasePrice       float64                `json:"basePrice"`
	Currency        string                 `json:"currency"`
	IsAvailable     bool                   `json:"isAvailable"`
	LeadTimeMinutes int                    `json:"leadTimeMinutes,omitempty"`
	ImageURL        string                 `json:"imageUrl,omitempty"`
	Nutrition       map[string]interface{} `json:"nutrition,omitempty"`
	SKU             string                 `json:"sku,omitempty"`
	DisplayOrder    int                    `json:"displayOrder"`
	Variants        []Variant              `json:"variants,omitempty"`
	Translations    []Translation          `json:"translations,omitempty"`
	DietaryTags     []DietaryTag           `json:"dietaryTags,omitempty"`
	Assets          []Asset                `json:"assets,omitempty"`
	Schedules       []Schedule             `json:"schedules,omitempty"`
	CreatedAt       time.Time              `json:"createdAt"`
	UpdatedAt       time.Time             `json:"updatedAt"`
}

// Variant represents a size/flavor variation of a menu item with price adjustment.
type Variant struct {
	ID           uuid.UUID `json:"id"`
	MenuItemID   uuid.UUID `json:"menuItemId"`
	Name         string    `json:"name"`
	PriceDelta   float64   `json:"priceDelta"`
	IsAvailable  bool      `json:"isAvailable"`
	SKU          string    `json:"sku,omitempty"`
	DisplayOrder int       `json:"displayOrder"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// Translation represents localized content for a menu item.
type Translation struct {
	ID          uuid.UUID `json:"id"`
	MenuItemID  uuid.UUID `json:"menuItemId"`
	Locale      string    `json:"locale"` // e.g., "en", "sw"
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// DietaryTag represents a dietary restriction or preference tag.
type DietaryTag struct {
	Code        string    `json:"code"` // Primary key: vegetarian, vegan, gluten-free, etc.
	Label       string    `json:"label"`
	Description string    `json:"description,omitempty"`
	IconURL     string    `json:"iconUrl,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

// Asset represents media associated with a menu item.
type Asset struct {
	ID         uuid.UUID              `json:"id"`
	MenuItemID uuid.UUID              `json:"menuItemId"`
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

// Schedule represents an availability window for a menu item.
type Schedule struct {
	ID         uuid.UUID `json:"id"`
	MenuItemID uuid.UUID `json:"menuItemId"`
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
	CafeID   *uuid.UUID
	ParentID *uuid.UUID
	IsActive *bool
	Search   string
	Limit    int
	Offset   int
}

// MenuItemFilter defines filter options for listing menu items.
type MenuItemFilter struct {
	TenantID    uuid.UUID
	CafeID      *uuid.UUID
	CategoryID  *uuid.UUID
	IsAvailable *bool
	Search      string
	DietaryTags []string
	MinPrice    *float64
	MaxPrice    *float64
	Locale      string
	Limit       int
	Offset      int
}

// PublicMenuRequest represents a request for the public menu API.
type PublicMenuRequest struct {
	TenantSlug  string
	CafeID      *uuid.UUID
	CategoryID  *uuid.UUID
	Locale      string
	DietaryTags []string
	Search      string
	Limit       int
	Offset      int
}

// PublicMenuItem is a read-only view of a menu item for public consumption.
type PublicMenuItem struct {
	ID              uuid.UUID    `json:"id"`
	CategoryID      uuid.UUID    `json:"categoryId"`
	CategoryName    string       `json:"categoryName"`
	Name            string       `json:"name"`
	Description     string       `json:"description,omitempty"`
	BasePrice       float64      `json:"basePrice"`
	Currency        string       `json:"currency"`
	ImageURL        string       `json:"imageUrl,omitempty"`
	LeadTimeMinutes int          `json:"leadTimeMinutes,omitempty"`
	Variants        []Variant    `json:"variants,omitempty"`
	DietaryTags     []DietaryTag `json:"dietaryTags,omitempty"`
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
