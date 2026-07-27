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
	ID               string            `json:"id"`
	Slug             string            `json:"slug"`
	Name             string            `json:"name"`
	LogoURL          string            `json:"logo_url,omitempty"`
	BrandColors      map[string]string `json:"brand_colors,omitempty"`
	UseCase          string            `json:"use_case,omitempty"`
	UseCases         []string          `json:"use_cases,omitempty"`
	SubscriptionPlan string            `json:"subscription_plan,omitempty"`
	Country          string            `json:"country,omitempty"`
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

// ListTenants fetches marketplace-visible tenants (active, non-demo), optionally filtered by
// use_case, paginated. Best-effort: on any failure it logs and returns an empty slice, never an
// error — a directory-service hiccup must never break the marketplace landing page (it should
// degrade to an empty/skeleton state, not a 500).
func (c *Client) ListTenants(ctx context.Context, useCase string, page, limit int) []TenantSummary {
	if c.baseURL == "" {
		return nil
	}
	q := url.Values{}
	if useCase = strings.TrimSpace(useCase); useCase != "" {
		q.Set("use_case", useCase)
	}
	if page > 0 {
		q.Set("page", strconv.Itoa(page))
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	u := fmt.Sprintf("%s/api/v1/tenants/marketplace", c.baseURL)
	if enc := q.Encode(); enc != "" {
		u += "?" + enc
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		c.log.Warn("marketplace: build request failed", zap.Error(err))
		return nil
	}
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.log.Warn("marketplace: request failed", zap.Error(err))
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		c.log.Warn("marketplace: unexpected status", zap.Int("status", resp.StatusCode))
		return nil
	}

	// Accept either a bare array or a {data: [...]} envelope — mirrors the tolerant decoding
	// already used elsewhere in this codebase for cross-service list endpoints.
	var raw json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		c.log.Warn("marketplace: decode failed", zap.Error(err))
		return nil
	}
	var direct []TenantSummary
	if err := json.Unmarshal(raw, &direct); err == nil {
		return direct
	}
	var enveloped struct {
		Data []TenantSummary `json:"data"`
	}
	if err := json.Unmarshal(raw, &enveloped); err == nil {
		return enveloped.Data
	}
	c.log.Warn("marketplace: response did not match array or {data:[]} envelope")
	return nil
}
