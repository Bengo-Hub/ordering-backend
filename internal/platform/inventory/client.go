package inventory

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/google/uuid"
	serviceclient "github.com/Bengo-Hub/shared-service-client"
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
	ItemID      uuid.UUID `json:"item_id"`
	SKU         string    `json:"sku"`
	WarehouseID uuid.UUID `json:"warehouse_id"`
	OnHand      int       `json:"on_hand"`
	Available   int       `json:"available"`
	Reserved    int       `json:"reserved"`
	UnitOfMeasure string  `json:"unit_of_measure"`
	UpdatedAt   time.Time `json:"updated_at"`
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
}

// ReservationResponse represents a stock reservation response.
type ReservationResponse struct {
	ID              uuid.UUID           `json:"id"`
	TenantID        uuid.UUID           `json:"tenant_id"`
	OrderID         uuid.UUID           `json:"order_id"`
	Status          string              `json:"status"` // pending, confirmed, released, consumed
	Items           []ReservedItem      `json:"items"`
	ExpiresAt       *time.Time          `json:"expires_at,omitempty"`
	ConfirmedAt     *time.Time          `json:"confirmed_at,omitempty"`
	CreatedAt       time.Time           `json:"created_at"`
}

// ReservedItem represents a reserved item.
type ReservedItem struct {
	SKU              string `json:"sku"`
	RequestedQty     int    `json:"requested_qty"`
	ReservedQty      int    `json:"reserved_qty"`
	AvailableQty     int    `json:"available_qty"`
	IsFullyReserved  bool   `json:"is_fully_reserved"`
}

// RecipeResponse represents a recipe/BOM from inventory.
type RecipeResponse struct {
	ID          uuid.UUID         `json:"id"`
	TenantID    uuid.UUID         `json:"tenant_id"`
	Name        string            `json:"name"`
	SKU         string            `json:"sku"`
	OutputQty   float64           `json:"output_qty"`
	Ingredients []RecipeIngredient `json:"ingredients"`
	IsActive    bool              `json:"is_active"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// RecipeIngredient represents an ingredient in a recipe.
type RecipeIngredient struct {
	ItemID        uuid.UUID `json:"item_id"`
	SKU           string    `json:"sku"`
	Name          string    `json:"name"`
	Quantity      float64   `json:"quantity"`
	UnitOfMeasure string    `json:"unit_of_measure"`
}

// ConsumptionRequest represents a request to record stock consumption.
type ConsumptionRequest struct {
	TenantID       uuid.UUID          `json:"tenant_id"`
	OrderID        uuid.UUID          `json:"order_id"`
	WarehouseID    uuid.UUID          `json:"warehouse_id,omitempty"`
	Items          []ConsumptionItem  `json:"items"`
	Reason         string             `json:"reason,omitempty"` // sale, waste, adjustment
	IdempotencyKey string             `json:"idempotency_key,omitempty"`
}

// ConsumptionItem represents an item to consume.
type ConsumptionItem struct {
	SKU      string  `json:"sku"`
	Quantity float64 `json:"quantity"`
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

// headers returns common headers for requests.
func (c *Client) headers(idempotencyKey string) map[string]string {
	h := map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}

	if c.apiKey != "" {
		h["X-API-Key"] = c.apiKey
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

	resp, err := c.serviceClient.Get(ctx, path, c.headers(""))
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

	resp, err := c.serviceClient.Post(ctx, path, reqBody, c.headers(""))
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

	resp, err := c.serviceClient.Post(ctx, path, req, c.headers(req.IdempotencyKey))
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

	resp, err := c.serviceClient.Get(ctx, path, c.headers(""))
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

	resp, err := c.serviceClient.Get(ctx, path, c.headers(""))
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

	resp, err := c.serviceClient.Post(ctx, path, reqBody, c.headers(""))
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

	resp, err := c.serviceClient.Post(ctx, path, nil, c.headers(""))
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

	resp, err := c.serviceClient.Post(ctx, path, req, c.headers(req.IdempotencyKey))
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

	resp, err := c.serviceClient.Get(ctx, path, c.headers(""))
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

	resp, err := c.serviceClient.Get(ctx, path, c.headers(""))
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
	Type         string         `json:"type"`          // GOODS, SERVICE, RECIPE, INGREDIENT
	CategorySlug string         `json:"category_slug,omitempty"` // inventory category
	UnitName     string         `json:"unit_name,omitempty"`     // e.g., CUP, PIECE
	ImageURL     string         `json:"image_url,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// ItemResponse represents an item returned from inventory-api.
type ItemResponse struct {
	ID           uuid.UUID      `json:"id"`
	SKU          string         `json:"sku"`
	Name         string         `json:"name"`
	Description  string         `json:"description,omitempty"`
	Type         string         `json:"type"`
	IsActive     bool           `json:"is_active"`
	ImageURL     string         `json:"image_url,omitempty"`
	CategoryID   *uuid.UUID     `json:"category_id,omitempty"`
	CategoryName string         `json:"category_name,omitempty"`
	UnitID       *uuid.UUID     `json:"unit_id,omitempty"`
	Tags         []string       `json:"tags,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// CreateItem creates a new item in inventory-api.
func (c *Client) CreateItem(ctx context.Context, tenantSlug string, req CreateItemRequest) (*ItemResponse, error) {
	path := fmt.Sprintf("/v1/%s/inventory/items", tenantSlug)
	resp, err := c.serviceClient.Post(ctx, path, req, c.headers(""))
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

// ListItems returns active items from inventory-api, optionally filtered by type.
// If no typeFilter is given, defaults to GOODS,RECIPE (the orderable catalog — SERVICE items are
// fetched only when explicitly requested, e.g. item_type=SERVICE for the events endpoint).
func (c *Client) ListItems(ctx context.Context, tenantSlug string, typeFilter ...string) ([]ItemResponse, error) {
	typeParam := "GOODS,RECIPE"
	if len(typeFilter) > 0 && typeFilter[0] != "" {
		typeParam = typeFilter[0]
	}
	path := fmt.Sprintf("/v1/%s/inventory/items?type=%s", tenantSlug, typeParam)
	resp, err := c.serviceClient.Get(ctx, path, c.headers(""))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}
	var listResp struct {
		Data []ItemResponse `json:"data"`
	}
	if err := resp.DecodeJSON(&listResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return listResp.Data, nil
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
}

// ListCategories returns categories from inventory-api.
// inventory-api returns paginated: {"data": [...], "total": N}
func (c *Client) ListCategories(ctx context.Context, tenantSlug string) ([]CategoryResponse, error) {
	path := fmt.Sprintf("/v1/%s/inventory/categories", tenantSlug)
	resp, err := c.serviceClient.Get(ctx, path, c.headers(""))
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

// ServiceClient returns the underlying service client for direct API calls.
func (c *Client) ServiceClient() *serviceclient.Client {
	return c.serviceClient
}
