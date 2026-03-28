package catalog

import (
	"time"

	"github.com/google/uuid"
)

// DefaultCurrency is the default currency for catalog items.
const DefaultCurrency = "KES"

// MergedCatalogItem is the merged view of inventory master data + local CatalogOverride.
type MergedCatalogItem struct {
	// Inventory fields
	InventorySKU string     `json:"sku"`
	InventoryID  uuid.UUID  `json:"inventoryId,omitempty"`
	Name         string     `json:"name"`
	Description  string     `json:"description,omitempty"`
	Type         string     `json:"type,omitempty"`
	IsActive     bool       `json:"isActive"`
	ImageURL     string     `json:"imageUrl,omitempty"`
	CategoryID   *uuid.UUID `json:"categoryId,omitempty"`
	CategoryName string     `json:"categoryName,omitempty"`

	// Override fields (from CatalogOverride)
	BasePrice         float64 `json:"basePrice"`
	Currency          string  `json:"currency"`
	IsAvailable       bool    `json:"isAvailable"`
	IsFeatured        bool    `json:"isFeatured"`
	LeadTimeMinutes   *int    `json:"leadTimeMinutes,omitempty"`
	DisplayOrder      int     `json:"displayOrder"`
	DisplaySection    string  `json:"displaySection,omitempty"`
	PackagingFee      float64 `json:"packagingFee"`
	ServiceFeePercent float64 `json:"serviceFeePercent"`

	// Computed fields
	IsFavorite bool `json:"isFavorite"`

	// Timestamps (from override)
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// InventoryCategory represents a category from inventory-api.
type InventoryCategory struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Code        string `json:"code"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	IsActive    bool   `json:"isActive"`
}

// OutletSummary represents an outlet for public display.
type OutletSummary struct {
	ID           uuid.UUID      `json:"id"`
	Name         string         `json:"name"`
	Slug         string         `json:"slug,omitempty"`
	Description  string         `json:"description,omitempty"`
	Address      string         `json:"address,omitempty"`
	Phone        string         `json:"phone,omitempty"`
	Email        string         `json:"email,omitempty"`
	Location     string         `json:"location,omitempty"`
	Latitude     *float64       `json:"latitude,omitempty"`
	Longitude    *float64       `json:"longitude,omitempty"`
	OpeningHours map[string]any `json:"openingHours,omitempty"`
	ImageURL     string         `json:"imageUrl,omitempty"`
	Status       string         `json:"status,omitempty"`
	UseCase      string         `json:"useCase,omitempty"`
	IsOpen       bool           `json:"isOpen"`
}

// CatalogFilter defines filter options for listing merged catalog items.
type CatalogFilter struct {
	TenantID   uuid.UUID
	OutletID   *uuid.UUID
	CategoryID *uuid.UUID
	Search     string
	IsFeatured  *bool
	IsAvailable *bool
	Section     string
	UserID      *uuid.UUID // for checking favorites
	Limit       int
	Offset      int
}

// OverrideUpsertRequest is the request payload for creating/updating a CatalogOverride.
type OverrideUpsertRequest struct {
	TenantID          uuid.UUID `json:"-"`
	OutletID          uuid.UUID `json:"outletId"`
	InventorySKU      string    `json:"sku"`
	BasePrice         float64   `json:"basePrice"`
	Currency          string    `json:"currency,omitempty"`
	IsAvailable       *bool     `json:"isAvailable,omitempty"`
	IsFeatured        *bool     `json:"isFeatured,omitempty"`
	LeadTimeMinutes   *int      `json:"leadTimeMinutes,omitempty"`
	DisplayOrder      *int      `json:"displayOrder,omitempty"`
	DisplaySection    string    `json:"displaySection,omitempty"`
	PackagingFee      *float64  `json:"packagingFee,omitempty"`
	ServiceFeePercent *float64  `json:"serviceFeePercent,omitempty"`
	ImageURLOverride  string    `json:"imageUrlOverride,omitempty"`
}

// ItemImage represents an image in a multi-image gallery for a catalog item.
type ItemImage struct {
	URL          string `json:"url"`
	Type         string `json:"type"`
	IsPrimary    bool   `json:"is_primary"`
	DisplayOrder int    `json:"display_order"`
}

// ListResponse is a generic paginated response.
type ListResponse struct {
	Data  interface{} `json:"data"`
	Total int         `json:"total"`
	Limit int         `json:"limit"`
	Page  int         `json:"page"`
}
