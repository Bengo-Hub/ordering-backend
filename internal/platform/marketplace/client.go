// Package marketplace is an S2S client for auth-api's public marketplace tenant-directory
// endpoint. It is the data source for the platform-level (no-tenant-slug) storefront landing
// page — ordering-backend does not own tenant data, this is a thin read-through proxy.
package marketplace

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// TenantSummary is one tenant row from auth-api's GET /api/v1/tenants/marketplace — only the
// public/safe fields a marketplace listing card needs.
type TenantSummary struct {
	ID      string `json:"id"`
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	LogoURL string `json:"logo_url,omitempty"`
	// BrandColors is free-form JSON on the tenant (hex strings in practice). Decoded loosely so
	// one tenant with a non-string value cannot fail the whole directory.
	BrandColors      map[string]any `json:"brand_colors,omitempty"`
	UseCase          string         `json:"use_case,omitempty"`
	UseCases         []string       `json:"use_cases,omitempty"`
	SubscriptionPlan string         `json:"subscription_plan,omitempty"`
	Country          string         `json:"country,omitempty"`
}

// Query narrows the directory. UseCase keeps one vertical; UseCases keeps tenants with any of
// them (auth-api matches both in SQL, so Total counts exactly the matching tenants).
type Query struct {
	UseCase  string
	UseCases []string
	Page     int
	Limit    int
}

// Page is one page of the directory with the full match count.
type Page struct {
	Data  []TenantSummary `json:"data"`
	Total int             `json:"total"`
}

// Client calls auth-api's public marketplace tenant-directory endpoint.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	log        *zap.Logger
}

// NewClient builds an auth-api marketplace client. baseURL is the auth-api service URL
// (AUTH_SERVICE_URL/AUTH_API_URL); apiKey is the shared INTERNAL_SERVICE_KEY (sent best-effort —
// the endpoint is public, but the header is harmless and future-proofs against it being locked
// down later).
func NewClient(baseURL, apiKey string, log *zap.Logger) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 8 * time.Second},
		log:        log.Named("marketplace-client"),
	}
}

// ListTenants fetches one page of marketplace-visible tenants (active, non-demo, ranked by plan
// tier by auth-api). An error means the directory could not be read; callers decide how to
// degrade (the landing page shows an empty state rather than a 500).
func (c *Client) ListTenants(ctx context.Context, q Query) (Page, error) {
	if c.baseURL == "" {
		return Page{Data: []TenantSummary{}}, nil
	}
	v := url.Values{}
	if uc := strings.TrimSpace(q.UseCase); uc != "" {
		v.Set("use_case", uc)
	}
	if len(q.UseCases) > 0 {
		v.Set("use_cases", strings.Join(q.UseCases, ","))
	}
	if q.Page > 0 {
		v.Set("page", strconv.Itoa(q.Page))
	}
	if q.Limit > 0 {
		v.Set("limit", strconv.Itoa(q.Limit))
	}
	u := fmt.Sprintf("%s/api/v1/tenants/marketplace?%s", c.baseURL, v.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Page{}, fmt.Errorf("marketplace: build request: %w", err)
	}
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Page{}, fmt.Errorf("marketplace: request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Page{}, fmt.Errorf("marketplace: auth-api answered %d", resp.StatusCode)
	}
	var page Page
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return Page{}, fmt.Errorf("marketplace: decode: %w", err)
	}
	if page.Data == nil {
		page.Data = []TenantSummary{}
	}
	return page, nil
}
