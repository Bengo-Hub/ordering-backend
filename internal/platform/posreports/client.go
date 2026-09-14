// Package posreports is an S2S client for pos-api's real sales-aggregation reports — used by
// the storefront's "Top Deals" rail to rank items by actual sales volume rather than by discount.
// Mirrors platform/posdiscounts' Client shape (Enabled/best-effort-empty-on-failure posture).
package posreports

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// DefaultBaseURL is the production pos-api host, used when POS_API_URL is unset.
const DefaultBaseURL = "https://posapi.codevertexafrica.com"

// SKURow is one entry from pos-api's S2S sales-by-sku aggregate.
type SKURow struct {
	SKU          string  `json:"sku"`
	Name         string  `json:"name"`
	QuantitySold float64 `json:"quantity_sold"`
	Revenue      float64 `json:"revenue"`
}

// Client calls pos-api's S2S reporting endpoints with the shared INTERNAL_SERVICE_KEY sent as
// the X-API-Key header. Mirrors platform/posdiscounts' Client shape.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	log        *zap.Logger
}

// NewClient builds a pos-api reports S2S client. baseURL is POS_API_URL (falls back to
// DefaultBaseURL when empty); apiKey is the shared INTERNAL_SERVICE_KEY. When apiKey is empty
// the client is disabled (Enabled() == false) and SalesBySKU returns an empty slice, never an
// error — a missing/misconfigured integration must never break the storefront homepage.
func NewClient(baseURL, apiKey string, log *zap.Logger) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 6 * time.Second},
		log:        log.Named("posreports-client"),
	}
}

// Enabled reports whether the client is configured (API key set).
func (c *Client) Enabled() bool { return c != nil && c.apiKey != "" }

// SalesBySKU returns units sold per SKU for completed POS orders between from/to (inclusive,
// "2006-01-02" format) — pos-api's GET /api/v1/s2s/{tenant}/pos/sales/by-sku, built explicitly
// for cross-service reuse (inventory-api's menu-engineering/variance reports already consume the
// same endpoint the same way). Best-effort: on any failure this logs and returns an empty slice,
// never an error, so a pos-api hiccup never breaks the storefront's "Top Deals" rail.
func (c *Client) SalesBySKU(ctx context.Context, tenantID uuid.UUID, from, to string) []SKURow {
	if !c.Enabled() {
		return nil
	}
	u := fmt.Sprintf("%s/api/v1/s2s/%s/pos/sales/by-sku?from=%s&to=%s", c.baseURL, tenantID.String(), from, to)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		c.log.Warn("posreports: build request failed", zap.Error(err))
		return nil
	}
	req.Header.Set("X-API-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.log.Warn("posreports: request failed", zap.Error(err))
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		c.log.Warn("posreports: unexpected status", zap.Int("status", resp.StatusCode))
		return nil
	}
	var envelope struct {
		Data []SKURow `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		c.log.Warn("posreports: decode failed", zap.Error(err))
		return nil
	}
	return envelope.Data
}
