package inventory

import (
	"context"
	"fmt"
	"net/url"
	"time"

	serviceclient "github.com/Bengo-Hub/shared-service-client"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/config"
)

// Client provides methods to interact with the inventory service.
type Client struct {
	baseURL       string
	apiKey        string
	serviceClient *serviceclient.Client
	logger        *zap.Logger
}

// NewClient creates a new inventory service client.
func NewClient(cfg config.InventoryConfig, logger *zap.Logger) *Client {
	scCfg := serviceclient.DefaultConfig(
		cfg.ServiceURL,
		"ordering-service",
		logger.Named("inventory.client"),
	)
	scCfg.Timeout = cfg.RequestTimeout

	return &Client{
		baseURL:       cfg.ServiceURL,
		apiKey:        cfg.APIKey,
		serviceClient: serviceclient.New(scCfg),
		logger:        logger.Named("inventory.client"),
	}
}

// StockAvailability represents stock availability for an item.
type StockAvailability struct {
	ItemID        uuid.UUID `json:"item_id"`
	SKU           string    `json:"sku"`
	WarehouseID   uuid.UUID `json:"warehouse_id"`
	OnHand        float64   `json:"on_hand"`
	Available     float64   `json:"available"`
	Reserved      float64   `json:"reserved"`
	UnitOfMeasure string    `json:"unit_of_measure"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ReservationRequest represents a request to reserve stock.
type ReservationRequest struct {
	TenantID       uuid.UUID         `json:"tenant_id"`
	OrderID        uuid.UUID         `json:"order_id"`
	WarehouseID    uuid.UUID         `json:"warehouse_id,omitempty"`
	Items          []ReservationItem `json:"items"`
	ExpiresAt      *time.Time        `json:"expires_at,omitempty"`
	IdempotencyKey string            `json:"idempotency_key,omitempty"`
}

// ReservationItem represents an item to reserve.
type ReservationItem struct {
	SKU      string `json:"sku"`
	Quantity int    `json:"quantity"`
	// Modifiers are selected modifier options on this line whose stock must also be
	// reserved (e.g. "Extra Cheese"). inventory-api resolves the option id → consumable
	// SKU and explodes/deducts it, mirroring the pos.sale.finalized modifiers contract.
	Modifiers []ModifierLine `json:"modifiers,omitempty"`
}

// ModifierLine is a selected modifier option carried on a reservation/consumption line.
// We send the inventory modifier-option id; inventory owns the option→SKU mapping.
type ModifierLine struct {
	InventoryModifierOptionID string  `json:"inventory_modifier_option_id,omitempty"`
	SKU                       string  `json:"sku,omitempty"`
	Quantity                  float64 `json:"quantity,omitempty"`
}

// ReservationResponse represents a stock reservation response.
type ReservationResponse struct {
	ID          uuid.UUID      `json:"id"`
	TenantID    uuid.UUID      `json:"tenant_id"`
	OrderID     uuid.UUID      `json:"order_id"`
	Status      string         `json:"status"` // pending, confirmed, released, consumed
	Items       []ReservedItem `json:"items"`
	ExpiresAt   *time.Time     `json:"expires_at,omitempty"`
	ConfirmedAt *time.Time     `json:"confirmed_at,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

// ReservedItem represents a reserved item.
type ReservedItem struct {
	SKU             string  `json:"sku"`
	RequestedQty    float64 `json:"requested_qty"`
	ReservedQty     float64 `json:"reserved_qty"`
	AvailableQty    float64 `json:"available_qty"`
	IsFullyReserved bool    `json:"is_fully_reserved"`
}

// RecipeResponse represents a recipe/BOM from inventory.
type RecipeResponse struct {
	ID          uuid.UUID          `json:"id"`
	TenantID    uuid.UUID          `json:"tenant_id"`
	Name        string             `json:"name"`
	SKU         string             `json:"sku"`
	OutputQty   float64            `json:"output_qty"`
	Ingredients []RecipeIngredient `json:"ingredients"`
	IsActive    bool               `json:"is_active"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
}

// RecipeIngredient represents an ingredient in a recipe.
type RecipeIngredient struct {
	ItemID uuid.UUID `json:"item_id"`
	// inventory-api serializes the ingredient SKU/name as item_sku/item_name (not
	// sku/name); reading the wrong tag left SKU empty, so the BOM stock-consumption
	// path sent blank-SKU items and inventory rejected them ("item not found: sku=").
	SKU           string  `json:"item_sku"`
	Name          string  `json:"item_name"`
	Quantity      float64 `json:"quantity"`
	UnitOfMeasure string  `json:"unit_of_measure"`
}

// ConsumptionRequest represents a request to record stock consumption.
type ConsumptionRequest struct {
	TenantID       uuid.UUID         `json:"tenant_id"`
	OrderID        uuid.UUID         `json:"order_id"`
	WarehouseID    uuid.UUID         `json:"warehouse_id,omitempty"`
	Items          []ConsumptionItem `json:"items"`
	Reason         string            `json:"reason,omitempty"` // sale, waste, adjustment
	IdempotencyKey string            `json:"idempotency_key,omitempty"`
}

// ConsumptionItem represents an item to consume.
type ConsumptionItem struct {
	SKU      string  `json:"sku"`
	Quantity float64 `json:"quantity"`
	// Modifiers are selected modifier options whose stock must also be consumed.
	Modifiers []ModifierLine `json:"modifiers,omitempty"`
}

// ConsumptionResponse represents a consumption response.
type ConsumptionResponse struct {
	ID          uuid.UUID `json:"id"`
	TenantID    uuid.UUID `json:"tenant_id"`
	OrderID     uuid.UUID `json:"order_id"`
	Status      string    `json:"status"`
	ProcessedAt time.Time `json:"processed_at"`
}

// APIError represents an error response from the inventory API.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("inventory API error: %s - %s", e.Code, e.Message)
}

// headers returns common headers for requests. tenantRef (slug or UUID) is sent as an
// explicit tenant header: event-consumer call paths carry no tenant in the request
// context, so the serviceclient's context propagation is inert there and inventory-api
// must get the tenant from the header/path ("stock not deducted" bug class).
func (c *Client) headers(tenantRef, idempotencyKey string) map[string]string {
	h := map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}

	if c.apiKey != "" {
		h["X-API-Key"] = c.apiKey
	}

	if tenantRef != "" {
		if _, err := uuid.Parse(tenantRef); err == nil {
			h["X-Tenant-ID"] = tenantRef
		} else {
			h["X-Tenant-Slug"] = tenantRef
		}
	}

	if idempotencyKey != "" {
		h["Idempotency-Key"] = idempotencyKey
	}

	return h
}

// parseError parses an error response from the API.
func (c *Client) parseError(resp *serviceclient.Response) error {
	var apiErr APIError
	if err := resp.DecodeJSON(&apiErr); err != nil {
		return fmt.Errorf("inventory API error (status %d)", resp.StatusCode)
	}
	return &apiErr
}

// GetStockAvailability retrieves stock availability for an item.
func (c *Client) GetStockAvailability(ctx context.Context, tenantSlug string, sku string) (*StockAvailability, error) {
	path := fmt.Sprintf("/v1/%s/inventory/items/%s", tenantSlug, url.PathEscape(sku))

	resp, err := c.serviceClient.Get(ctx, path, c.headers(tenantSlug, ""))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}

	var result StockAvailability
	if err := resp.DecodeJSON(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// CheckBulkAvailability checks stock availability for multiple items.
func (c *Client) CheckBulkAvailability(ctx context.Context, tenantSlug string, skus []string) ([]StockAvailability, error) {
	path := fmt.Sprintf("/v1/%s/inventory/availability", tenantSlug)

	reqBody := map[string]interface{}{
		"skus": skus,
	}

	resp, err := c.serviceClient.Post(ctx, path, reqBody, c.headers(tenantSlug, ""))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}

	var result []StockAvailability
	if err := resp.DecodeJSON(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return result, nil
}

// CreateReservation creates a stock reservation for an order.
func (c *Client) CreateReservation(ctx context.Context, tenantSlug string, req ReservationRequest) (*ReservationResponse, error) {
	path := fmt.Sprintf("/v1/%s/inventory/reservations", tenantSlug)

	resp, err := c.serviceClient.Post(ctx, path, req, c.headers(tenantSlug, req.IdempotencyKey))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}

	var result ReservationResponse
	if err := resp.DecodeJSON(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// GetReservation retrieves a reservation by ID.
func (c *Client) GetReservation(ctx context.Context, tenantSlug string, reservationID uuid.UUID) (*ReservationResponse, error) {
	path := fmt.Sprintf("/v1/%s/inventory/reservations/%s", tenantSlug, reservationID.String())

	resp, err := c.serviceClient.Get(ctx, path, c.headers(tenantSlug, ""))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}

	var result ReservationResponse
	if err := resp.DecodeJSON(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// GetReservationByOrderID retrieves a reservation by order ID.
func (c *Client) GetReservationByOrderID(ctx context.Context, tenantSlug string, orderID uuid.UUID) (*ReservationResponse, error) {
	path := fmt.Sprintf("/v1/%s/inventory/reservations?order_id=%s", tenantSlug, orderID.String())

	resp, err := c.serviceClient.Get(ctx, path, c.headers(tenantSlug, ""))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}

	var results []ReservationResponse
	if err := resp.DecodeJSON(&results); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(results) == 0 {
		return nil, &APIError{Code: "NOT_FOUND", Message: "reservation not found"}
	}

	return &results[0], nil
}

// ReleaseReservation releases a stock reservation.
func (c *Client) ReleaseReservation(ctx context.Context, tenantSlug string, reservationID uuid.UUID, reason string) error {
	path := fmt.Sprintf("/v1/%s/inventory/reservations/%s/release", tenantSlug, reservationID.String())
	reqBody := map[string]interface{}{"reason": reason}

	resp, err := c.serviceClient.Post(ctx, path, reqBody, c.headers(tenantSlug, ""))
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}

	if !resp.IsSuccess() {
		return c.parseError(resp)
	}

	return nil
}

// ConsumeReservation converts a reservation to actual consumption.
func (c *Client) ConsumeReservation(ctx context.Context, tenantSlug string, reservationID uuid.UUID) error {
	path := fmt.Sprintf("/v1/%s/inventory/reservations/%s/consume", tenantSlug, reservationID.String())

	resp, err := c.serviceClient.Post(ctx, path, nil, c.headers(tenantSlug, ""))
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}

	if !resp.IsSuccess() {
		return c.parseError(resp)
	}

	return nil
}

// RecordConsumption records stock consumption (for direct consumption without reservation).
func (c *Client) RecordConsumption(ctx context.Context, tenantSlug string, req ConsumptionRequest) (*ConsumptionResponse, error) {
	path := fmt.Sprintf("/v1/%s/inventory/consumption", tenantSlug)

	resp, err := c.serviceClient.Post(ctx, path, req, c.headers(tenantSlug, req.IdempotencyKey))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}

	var result ConsumptionResponse
	if err := resp.DecodeJSON(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// GetRecipe retrieves a recipe/BOM by ID.
func (c *Client) GetRecipe(ctx context.Context, tenantSlug string, recipeID uuid.UUID) (*RecipeResponse, error) {
	path := fmt.Sprintf("/v1/%s/inventory/recipes/%s", tenantSlug, recipeID.String())

	resp, err := c.serviceClient.Get(ctx, path, c.headers(tenantSlug, ""))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}

	var result RecipeResponse
	if err := resp.DecodeJSON(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// GetRecipeBySKU retrieves a recipe/BOM by SKU.
func (c *Client) GetRecipeBySKU(ctx context.Context, tenantSlug string, sku string) (*RecipeResponse, error) {
	path := fmt.Sprintf("/v1/%s/inventory/recipes?sku=%s", tenantSlug, url.QueryEscape(sku))

	resp, err := c.serviceClient.Get(ctx, path, c.headers(tenantSlug, ""))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}

	var results []RecipeResponse
	if err := resp.DecodeJSON(&results); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(results) == 0 {
		return nil, &APIError{Code: "NOT_FOUND", Message: "recipe not found"}
	}

	return &results[0], nil
}

// CreateItemRequest represents a request to create an inventory item.
type CreateItemRequest struct {
	SKU          string         `json:"sku"`
	Name         string         `json:"name"`
	Description  string         `json:"description,omitempty"`
	Type         string         `json:"type"`                    // GOODS, SERVICE, RECIPE, INGREDIENT
	CategorySlug string         `json:"category_slug,omitempty"` // inventory category
	UnitName     string         `json:"unit_name,omitempty"`     // e.g., CUP, PIECE
	ImageURL     string         `json:"image_url,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// ItemResponse represents an item returned from inventory-api.
// ItemVariantResponse is an item variation surfaced by inventory-api.
type ItemVariantResponse struct {
	ID         uuid.UUID         `json:"id"`
	SKU        string            `json:"sku"`
	Name       string            `json:"name"`
	Price      float64           `json:"price"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Barcode    string            `json:"barcode,omitempty"`
	IsActive   bool              `json:"is_active"`
}

type ItemResponse struct {
	ID             uuid.UUID             `json:"id"`
	SKU            string                `json:"sku"`
	Name           string                `json:"name"`
	Description    string                `json:"description,omitempty"`
	Type           string                `json:"type"`
	IsActive       bool                  `json:"is_active"`
	// NotForSale marks purchasable-only stock (e.g. ingredients bought for production):
	// such items must NEVER surface on any sales/storefront surface.
	NotForSale bool `json:"not_for_sale"`
	ImageURL       string                `json:"image_url,omitempty"`
	CategoryID     *uuid.UUID            `json:"category_id,omitempty"`
	CategoryName   string                `json:"category_name,omitempty"`
	BrandID        *uuid.UUID            `json:"brand_id,omitempty"`
	BrandName      string                `json:"brand_name,omitempty"`
	BrandCode      string                `json:"brand_code,omitempty"`
	Manufacturer   string                `json:"manufacturer,omitempty"`
	Model          string                `json:"model,omitempty"`
	Condition      string                `json:"condition,omitempty"` // NEW | REFURBISHED | USED | OPEN_BOX
	HasVariants    bool                  `json:"has_variants,omitempty"`
	Variants       []ItemVariantResponse `json:"variants,omitempty"`
	UnitID         *uuid.UUID            `json:"unit_id,omitempty"`
	Tags           []string              `json:"tags,omitempty"`
	Metadata       map[string]any        `json:"metadata,omitempty"`
	CostPrice      *float64              `json:"cost_price,omitempty"`
	SuggestedPrice *float64              `json:"suggested_price,omitempty"`
	// Recipe-costing fields (added 2026-06-01)
	SellingPrice   *float64 `json:"selling_price,omitempty"`
	FoodCostPct    *float64 `json:"food_cost_pct,omitempty"`
	FoodCostStatus string   `json:"status,omitempty"` // "OK - healthy" | "OK - above target FC%" | "LOSS"
	// Supplier / EP-cost fields
	PurchasePrice    *float64 `json:"purchase_price,omitempty"`
	PurchasePackSize *float64 `json:"purchase_pack_size,omitempty"`
	PurchaseUnit     string   `json:"purchase_unit,omitempty"`
	YieldPct         *float64 `json:"yield_pct,omitempty"`
	// Tax — enriched by inventory-api from treasury-api (source of truth)
	TaxCodeID    string   `json:"tax_code_id,omitempty"`
	TaxInclusive bool     `json:"tax_inclusive,omitempty"`
	TaxRate      *float64 `json:"tax_rate,omitempty"`
	NetPrice     *float64 `json:"net_price,omitempty"`
	TaxAmount    *float64 `json:"tax_amount,omitempty"`
	// Event fields — SERVICE type only
	TotalCapacity *int       `json:"total_capacity,omitempty"`
	EventStartAt  *time.Time `json:"event_start_at,omitempty"`
	EventEndAt    *time.Time `json:"event_end_at,omitempty"`
	EventVenue    string     `json:"event_venue,omitempty"`
}

// CreateItem creates a new item in inventory-api.
func (c *Client) CreateItem(ctx context.Context, tenantSlug string, req CreateItemRequest) (*ItemResponse, error) {
	path := fmt.Sprintf("/v1/%s/inventory/items", tenantSlug)
	resp, err := c.serviceClient.Post(ctx, path, req, c.headers(tenantSlug, ""))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}
	var result ItemResponse
	if err := resp.DecodeJSON(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

// ListItems returns items from inventory-api filtered by type, with server-side pagination.
// typeFilter: comma-separated types (e.g. "SERVICE", "GOODS,RECIPE"). Defaults to "GOODS,RECIPE".
// limit: page size; uses inventory-api default (20) when <= 0.
// page: 1-based page number; defaults to 1 when <= 0.
// sortParam is an opaque storefront sort key ("newest") mapped to inventory-api's own
// whitelisted ?sort=<column>&dir=asc|desc columns — kept as a translation choke point here
// so the storefront-facing API never has to know inventory-api's raw column names.
func sortParam(sort string) string {
	switch sort {
	case "newest":
		return "&sort=created_at&dir=desc"
	default:
		return ""
	}
}

func (c *Client) ListItems(ctx context.Context, tenantSlug string, typeFilter string, limit, page int, categoryID *uuid.UUID, sort string) ([]ItemResponse, int, error) {
	if typeFilter == "" {
		typeFilter = "GOODS,RECIPE"
	}
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	// Only finished, sellable, in-stock items belong on the storefront. category_id is applied
	// SERVER-SIDE so a selected category returns its items (and the correct total) instead of the
	// previous client-side post-pagination filter that returned an empty/null page.
	// include=variants so merged catalog items carry their sellable variations.
	path := fmt.Sprintf("/v1/%s/inventory/items?type=%s&status=active&limit=%d&page=%d&include=variants", tenantSlug, typeFilter, limit, page)
	if categoryID != nil {
		path += "&category_id=" + categoryID.String()
	}
	path += sortParam(sort)
	resp, err := c.serviceClient.Get(ctx, path, c.headers(tenantSlug, ""))
	if err != nil {
		return nil, 0, fmt.Errorf("execute request: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, 0, c.parseError(resp)
	}
	var listResp struct {
		Data  []ItemResponse `json:"data"`
		Total int            `json:"total"`
	}
	if err := resp.DecodeJSON(&listResp); err != nil {
		return nil, 0, fmt.Errorf("decode response: %w", err)
	}
	return listResp.Data, listResp.Total, nil
}

// GetOrCreateItem checks if an item exists by SKU, creates it if not.
func (c *Client) GetOrCreateItem(ctx context.Context, tenantSlug, sku string, req CreateItemRequest) (*ItemResponse, error) {
	// Try to get existing
	avail, err := c.GetStockAvailability(ctx, tenantSlug, sku)
	if err == nil {
		return &ItemResponse{
			ID:  avail.ItemID,
			SKU: avail.SKU,
		}, nil
	}
	// Item not found — create it
	req.SKU = sku
	return c.CreateItem(ctx, tenantSlug, req)
}

// HealthCheck checks if the inventory service is healthy.
func (c *Client) HealthCheck(ctx context.Context) error {
	resp, err := c.serviceClient.Get(ctx, "/healthz", nil)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}

	if !resp.IsSuccess() {
		return fmt.Errorf("inventory service unhealthy: status %d", resp.StatusCode)
	}

	return nil
}

// CategoryResponse represents a category from inventory-api.
type CategoryResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Code        string `json:"code"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	IsActive    bool   `json:"is_active"`
	// ParentID/Depth/Path/SortOrder mirror inventory-api's CategoryDTO materialized-path
	// hierarchy fields — inventory-api already populates these (its own doc comment names
	// this exact "Shop by Category" storefront flyout use case), ordering-backend just
	// never forwarded them until now.
	ParentID   *string `json:"parent_id,omitempty"`
	ParentName string  `json:"parent_name,omitempty"`
	Depth      int     `json:"depth"`
	Path       string  `json:"path,omitempty"`
	SortOrder  int     `json:"sort_order,omitempty"`
}

// ListCategories returns categories from inventory-api.
// inventory-api returns paginated: {"data": [...], "total": N}
// When hasItems is true, only categories with at least one item linked are returned.
func (c *Client) ListCategories(ctx context.Context, tenantSlug string, hasItems bool) ([]CategoryResponse, error) {
	path := fmt.Sprintf("/v1/%s/inventory/categories", tenantSlug)
	if hasItems {
		path += "?has_items=true"
	}
	resp, err := c.serviceClient.Get(ctx, path, c.headers(tenantSlug, ""))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}
	// inventory-api wraps results in {"data": [...], "total": N}
	var wrapper struct {
		Data  []CategoryResponse `json:"data"`
		Total int                `json:"total"`
	}
	if err := resp.DecodeJSON(&wrapper); err != nil {
		// Fallback: try direct array decode for backward compatibility
		var categories []CategoryResponse
		if err2 := resp.DecodeJSON(&categories); err2 != nil {
			return nil, fmt.Errorf("decode response: %w", err2)
		}
		return categories, nil
	}
	return wrapper.Data, nil
}

// BrandResponse represents an item brand from inventory-api.
type BrandResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Code        string `json:"code"`
	Description string `json:"description"`
	LogoURL     string `json:"logo_url"`
	IsActive    bool   `json:"is_active"`
	SortOrder   int    `json:"sort_order"`
}

// ListBrands returns item brands from inventory-api.
// inventory-api returns paginated: {"data": [...], "total": N}
func (c *Client) ListBrands(ctx context.Context, tenantSlug string) ([]BrandResponse, error) {
	path := fmt.Sprintf("/v1/%s/inventory/brands", tenantSlug)
	resp, err := c.serviceClient.Get(ctx, path, c.headers(tenantSlug, ""))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}
	// inventory-api wraps results in {"data": [...], "total": N}
	var wrapper struct {
		Data  []BrandResponse `json:"data"`
		Total int             `json:"total"`
	}
	if err := resp.DecodeJSON(&wrapper); err != nil {
		// Fallback: try direct array decode for backward compatibility
		var brands []BrandResponse
		if err2 := resp.DecodeJSON(&brands); err2 != nil {
			return nil, fmt.Errorf("decode response: %w", err2)
		}
		return brands, nil
	}
	return wrapper.Data, nil
}

// ServiceClient returns the underlying service client for direct API calls.
func (c *Client) ServiceClient() *serviceclient.Client {
	return c.serviceClient
}

// ── Event ticketing (public storefront) ─────────────────────────────────────

// EventResponse is an event Item (type=SERVICE) as needed for the public event page.
type EventResponse struct {
	ID             uuid.UUID      `json:"id"`
	SKU            string         `json:"sku"`
	Name           string         `json:"name"`
	Description    string         `json:"description,omitempty"`
	ImageURL       string         `json:"image_url,omitempty"`
	EventStartAt   *time.Time     `json:"event_start_at,omitempty"`
	EventEndAt     *time.Time     `json:"event_end_at,omitempty"`
	EventVenue     *string        `json:"event_venue,omitempty"`
	TotalCapacity  *int           `json:"total_capacity,omitempty"`
	BookedCapacity *int           `json:"booked_capacity,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// EventTierAvailability mirrors inventory tickets.TierAvailability.
type EventTierAvailability struct {
	TierID    string  `json:"tier_id"`
	Name      string  `json:"name"`
	Price     float64 `json:"price"`
	Capacity  int     `json:"capacity"`
	Issued    int     `json:"issued"`
	Remaining int     `json:"remaining"`
}

// EventAvailability mirrors inventory tickets.Availability.
type EventAvailability struct {
	EventItemID    uuid.UUID               `json:"event_item_id"`
	TotalCapacity  int                     `json:"total_capacity"`
	BookedCapacity int                     `json:"booked_capacity"`
	Remaining      int                     `json:"remaining"`
	Tiers          []EventTierAvailability `json:"tiers"`
}

// ListEvents fetches SERVICE-type event items for a tenant (public catalog browse).
func (c *Client) ListEvents(ctx context.Context, tenantSlug string, limit, page int) ([]EventResponse, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if page <= 0 {
		page = 1
	}
	path := fmt.Sprintf("/v1/%s/inventory/events?limit=%d&page=%d", tenantSlug, limit, page)
	resp, err := c.serviceClient.Get(ctx, path, c.headers(tenantSlug, ""))
	if err != nil {
		return nil, 0, fmt.Errorf("execute request: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, 0, c.parseError(resp)
	}
	var listResp struct {
		Data  []EventResponse `json:"data"`
		Total int             `json:"total"`
	}
	if err := resp.DecodeJSON(&listResp); err != nil {
		return nil, 0, fmt.Errorf("decode response: %w", err)
	}
	return listResp.Data, listResp.Total, nil
}

// GetEventAvailability fetches per-tier + total availability for an event.
func (c *Client) GetEventAvailability(ctx context.Context, tenantSlug, eventID string) (*EventAvailability, error) {
	path := fmt.Sprintf("/v1/%s/inventory/events/%s/availability", tenantSlug, url.PathEscape(eventID))
	resp, err := c.serviceClient.Get(ctx, path, c.headers(tenantSlug, ""))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}
	var av EventAvailability
	if err := resp.DecodeJSON(&av); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &av, nil
}
