package catalog

import (
	"time"

	"github.com/google/uuid"
)

// DefaultCurrency is the default currency for catalog items.
const DefaultCurrency = "KES"

// CatalogVariant is a sellable product variation surfaced from inventory-api.
type CatalogVariant struct {
	ID         uuid.UUID         `json:"id"`
	SKU        string            `json:"sku"`
	Name       string            `json:"name"`
	Price      float64           `json:"price"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Barcode    string            `json:"barcode,omitempty"`
	IsActive   bool              `json:"isActive"`
}

// MergedCatalogItem is the merged view of inventory master data + local CatalogOverride.
type MergedCatalogItem struct {
	// Inventory fields
	InventorySKU string           `json:"sku"`
	InventoryID  uuid.UUID        `json:"inventoryId,omitempty"`
	Name         string           `json:"name"`
	Description  string           `json:"description,omitempty"`
	Type         string           `json:"type,omitempty"`
	IsActive     bool             `json:"isActive"`
	ImageURL     string           `json:"imageUrl,omitempty"`
	CategoryID   *uuid.UUID       `json:"categoryId,omitempty"`
	CategoryName string           `json:"categoryName,omitempty"`
	BrandID      *uuid.UUID       `json:"brandId,omitempty"`
	BrandName    string           `json:"brandName,omitempty"`
	BrandCode    string           `json:"brandCode,omitempty"`
	Manufacturer string           `json:"manufacturer,omitempty"` // retail/pharmacy
	Model        string           `json:"model,omitempty"`        // retail only
	HasVariants  bool             `json:"hasVariants,omitempty"`
	Variants     []CatalogVariant `json:"variants,omitempty"`
	Tags         []string         `json:"tags,omitempty"`
	Metadata     map[string]any   `json:"metadata,omitempty"`

	// Event fields (SERVICE type only) — sourced from inventory
	TotalCapacity *int       `json:"totalCapacity,omitempty"`
	EventStartAt  *time.Time `json:"eventStartAt,omitempty"`
	EventEndAt    *time.Time `json:"eventEndAt,omitempty"`
	EventVenue    string     `json:"eventVenue,omitempty"`

	// Override fields (from CatalogOverride)
	BasePrice         float64 `json:"basePrice"`
	Currency          string  `json:"currency"`
	IsAvailable       bool    `json:"isAvailable"`
	// AvailableQuantity is the quantity-aware availability projection (STK-5): the last
	// on-hand/producible quantity seen from inventory stock events. Nil = unknown (no stock
	// event observed yet). Lets the storefront show real stock levels ("3 left") rather than
	// only a sold-out boolean. is_available remains the authoritative orderable gate.
	AvailableQuantity *float64 `json:"availableQuantity,omitempty"`
	// IsComplimentary marks a no-charge accompaniment: price 0 but explicitly enabled via override,
	// shown as "Free" and not billed, while its recipe/BOM stock is still deducted on sale.
	IsComplimentary   bool    `json:"isComplimentary"`
	IsFeatured        bool    `json:"isFeatured"`
	LeadTimeMinutes   *int    `json:"leadTimeMinutes,omitempty"`
	DisplayOrder      int     `json:"displayOrder"`
	DisplaySection    string  `json:"displaySection,omitempty"`
	PackagingFee      float64 `json:"packagingFee"`
	ServiceFeePercent float64 `json:"serviceFeePercent"`

	// Recipe-costing fields (sourced from inventory, added 2026-06-01)
	CostPrice      *float64 `json:"costPrice,omitempty"`
	SuggestedPrice *float64 `json:"suggestedPrice,omitempty"`
	SellingPrice   *float64 `json:"sellingPrice,omitempty"`
	FoodCostPct    *float64 `json:"foodCostPct,omitempty"`
	FoodCostStatus string   `json:"foodCostStatus,omitempty"` // "OK - healthy" | "OK - above target FC%" | "LOSS"

	// Tax — enriched by inventory-api from treasury-api (source of truth)
	TaxCodeID    string   `json:"taxCodeId,omitempty"`
	TaxInclusive bool     `json:"taxInclusive,omitempty"`
	TaxRate      *float64 `json:"taxRate,omitempty"`
	NetPrice     *float64 `json:"netPrice,omitempty"`
	TaxAmount    *float64 `json:"taxAmount,omitempty"`

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
	// ParentID/ParentName/Depth/Path/SortOrder let the storefront build a real
	// parent→children category tree (the "Shop by Category" sidebar/flyout for
	// retail/pharmacy/wholesale tenants) instead of a flat list.
	ParentID   *string `json:"parentId,omitempty"`
	ParentName string  `json:"parentName,omitempty"`
	Depth      int     `json:"depth"`
	Path       string  `json:"path,omitempty"`
	SortOrder  int     `json:"sortOrder,omitempty"`
}

// InventoryBrand represents an item brand from inventory-api.
type InventoryBrand struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Code      string `json:"code"`
	LogoURL   string `json:"logoUrl,omitempty"`
	IsActive  bool   `json:"isActive"`
	SortOrder int    `json:"sortOrder"`
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
	// BookingDepositPercent is the deposit % charged up front for event-ticket / service-
	// appointment checkouts (0 = pay in full). The storefront uses it to show the deposit split.
	BookingDepositPercent int `json:"bookingDepositPercent"`
}

// CatalogFilter defines filter options for listing merged catalog items.
type CatalogFilter struct {
	TenantID    uuid.UUID
	OutletID    *uuid.UUID
	CategoryID  *uuid.UUID
	Search      string
	IsFeatured  *bool
	IsAvailable *bool
	Section     string
	UserID      *uuid.UUID // for checking favorites
	Limit       int
	Offset      int
	ItemType    string   // e.g. "SERVICE", "GOODS", "RECIPE" — empty means all
	Tags        []string // items must have ALL of these tags
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
