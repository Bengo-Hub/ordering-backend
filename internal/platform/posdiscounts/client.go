// Package posdiscounts is an S2S client for pos-api's discount/promotion banner endpoint.
//
// pos-api's Promotion + PromotionRule are the platform's discount source of truth (see its own
// handler doc comment). Storefront promotional banners are just another view over the same
// Promotion record (a `banner` object stored in Promotion.metadata) — ordering-backend does NOT
// own or duplicate discount data, it only reads the subset flagged `show_on_storefront: true`.
package posdiscounts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ErrDiscountsClientDisabled is returned by ApplyDiscount when INTERNAL_SERVICE_KEY is unset —
// callers use this to fall back to a legacy path, distinct from a real evaluation failure.
var ErrDiscountsClientDisabled = errors.New("posdiscounts: client disabled (no API key configured)")

// DefaultBaseURL is the production pos-api host, used when POS_API_URL is unset.
const DefaultBaseURL = "https://posapi.codevertexafrica.com"

// Banner is a single storefront-visible promotion, as returned by
// GET /api/v1/s2s/{tenant}/discounts/banners.
type Banner struct {
	PromoID        string  `json:"promo_id"`
	Name           string  `json:"name"`
	BannerTitle    string  `json:"banner_title"`
	BannerSubtitle string  `json:"banner_subtitle"`
	BannerImageURL string  `json:"banner_image_url"`
	CTALabel       string  `json:"cta_label"`
	CTALink        string  `json:"cta_link"`
	BannerColor    string  `json:"banner_color"`
	TextColor      string  `json:"text_color"`
	OutletID       *string `json:"outlet_id"`
	// IsFlashSale + EndAt let the storefront render a live countdown to the promotion's
	// real end_at instead of a static banner.
	IsFlashSale bool       `json:"is_flash_sale,omitempty"`
	EndAt       *time.Time `json:"end_at,omitempty"`
}

// Discount is one entry from GET /api/v1/s2s/{tenant}/discounts — a promotion paired with its
// discount rule, scoped down to what a storefront "Top Deals" grid needs to match against the
// items/categories it already has loaded. ScopeType/ScopeIDs mirror pos-api's PromotionRule
// (e.g. scope_type="item"|"category"|"all", scope_ids=the matching item/category IDs) — resolving
// which catalog rows a deal applies to is left to the caller (this package owns no catalog data).
type Discount struct {
	ID      string     `json:"id"`
	Name    string     `json:"name"`
	StartAt time.Time  `json:"start_at"`
	EndAt   *time.Time `json:"end_at"`
	Rule    *struct {
		ScopeType     string   `json:"scope_type"`
		ScopeIDs      []string `json:"scope_ids"`
		DiscountType  string   `json:"discount_type"`
		DiscountValue float64  `json:"discount_value"`
		MaxDiscount   float64  `json:"max_discount"`
	} `json:"rule"`
}

// Client calls pos-api's /api/v1/s2s/{tenant}/discounts/* endpoints with the shared
// INTERNAL_SERVICE_KEY sent as the X-API-Key header. Mirrors platform/posloyalty's Client shape.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	log        *zap.Logger
}

// NewClient builds a pos-api discounts S2S client. baseURL is POS_API_URL (falls back to
// DefaultBaseURL when empty); apiKey is the shared INTERNAL_SERVICE_KEY. When apiKey is empty the
// client is disabled (Enabled() == false) and ListBanners returns an empty slice, never an error —
// a missing/misconfigured promotions integration must never break the storefront homepage.
func NewClient(baseURL, apiKey string, log *zap.Logger) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 6 * time.Second},
		log:        log.Named("posdiscounts-client"),
	}
}

// Enabled reports whether the client is configured (API key set).
func (c *Client) Enabled() bool { return c != nil && c.apiKey != "" }

// ListBanners returns the active storefront-flagged promotions for a tenant, optionally filtered
// by outlet use_case. Best-effort: on any failure it logs and returns an empty slice (never an
// error) so a promotions-service hiccup never breaks the storefront homepage.
func (c *Client) ListBanners(ctx context.Context, tenantID uuid.UUID, useCase string) []Banner {
	if !c.Enabled() {
		return nil
	}
	u := fmt.Sprintf("%s/api/v1/s2s/%s/discounts/banners", c.baseURL, tenantID.String())
	if useCase = strings.TrimSpace(useCase); useCase != "" {
		u += "?use_case=" + url.QueryEscape(useCase)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		c.log.Warn("posdiscounts: build request failed", zap.Error(err))
		return nil
	}
	req.Header.Set("X-API-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.log.Warn("posdiscounts: request failed", zap.Error(err))
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		c.log.Warn("posdiscounts: unexpected status", zap.Int("status", resp.StatusCode))
		return nil
	}
	var out []Banner
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		c.log.Warn("posdiscounts: decode failed", zap.Error(err))
		return nil
	}
	return out
}

// ListDeals returns active, currently-in-window discounts for a "Top Deals" storefront grid.
// Best-effort, same posture as ListBanners: any failure returns an empty slice, never an error.
// S2SListDiscounts filters by DB status only (no time-window check, unlike the banners endpoint),
// so the in-window filter (start_at <= now <= end_at) is applied here.
func (c *Client) ListDeals(ctx context.Context, tenantID uuid.UUID) []Discount {
	if !c.Enabled() {
		return nil
	}
	u := fmt.Sprintf("%s/api/v1/s2s/%s/discounts?status=active", c.baseURL, tenantID.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		c.log.Warn("posdiscounts: build deals request failed", zap.Error(err))
		return nil
	}
	req.Header.Set("X-API-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.log.Warn("posdiscounts: deals request failed", zap.Error(err))
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		c.log.Warn("posdiscounts: deals unexpected status", zap.Int("status", resp.StatusCode))
		return nil
	}
	var envelope struct {
		Data []Discount `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		c.log.Warn("posdiscounts: deals decode failed", zap.Error(err))
		return nil
	}
	now := time.Now()
	out := make([]Discount, 0, len(envelope.Data))
	for _, d := range envelope.Data {
		if d.StartAt.After(now) {
			continue
		}
		if d.EndAt != nil && d.EndAt.Before(now) {
			continue
		}
		out = append(out, d)
	}
	return out
}

// ApplyLine is one cart line sent to ApplyDiscount — mirrors pos-api's applyPromoLineInput.
// Category is best-effort: ordering-backend's CartItem does not currently snapshot a category
// at add-time, so category-scoped discounts won't match at checkout until that's added (a
// separate follow-up) — item-scoped and storewide discounts are unaffected.
type ApplyLine struct {
	SKU       string  `json:"sku"`
	Category  string  `json:"category,omitempty"`
	Quantity  float64 `json:"quantity"`
	UnitPrice float64 `json:"unit_price"`
}

// ApplyResult is pos-api's response from POST /api/v1/s2s/{tenant}/discounts/apply — the same
// rule-based evaluation (schedule/meal_period/scope/BOGO) the POS terminal uses.
type ApplyResult struct {
	Valid          bool                       `json:"valid"`
	PromoCode      string                     `json:"promoCode"`
	PromoID        string                     `json:"promoId"`
	DiscountAmount string                     `json:"discountAmount"`
	PerSKU         map[string]json.RawMessage `json:"perSku,omitempty"`
	Reason         string                     `json:"reason,omitempty"`
}

// ApplyDiscount validates promoCode against the caller's REAL cart lines through pos-api's
// discount source-of-truth evaluator — the SAME schedule/meal_period/item-or-category scope/BOGO
// logic the POS terminal and Add Sale use, so a code behaves identically no matter which service
// applies it. Unlike ListBanners/ListDeals (best-effort, cosmetic), this is a real monetary
// decision: returns ErrDiscountsClientDisabled when unconfigured (caller's choice how to handle),
// and a real error on any transport/decode failure — NEVER a silently-zeroed discount, since that
// would be indistinguishable from a legitimately-invalid code.
func (c *Client) ApplyDiscount(ctx context.Context, tenantID uuid.UUID, outletID *uuid.UUID, code string, lines []ApplyLine) (*ApplyResult, error) {
	if !c.Enabled() {
		return nil, ErrDiscountsClientDisabled
	}
	body := struct {
		PromoCode string      `json:"promoCode"`
		OutletID  string      `json:"outlet_id,omitempty"`
		Lines     []ApplyLine `json:"lines"`
	}{PromoCode: code, Lines: lines}
	if outletID != nil {
		body.OutletID = outletID.String()
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("posdiscounts: encode apply request: %w", err)
	}

	u := fmt.Sprintf("%s/api/v1/s2s/%s/discounts/apply", c.baseURL, tenantID.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("posdiscounts: build apply request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("posdiscounts: apply request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("posdiscounts: apply unexpected status %d", resp.StatusCode)
	}
	var out ApplyResult
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("posdiscounts: decode apply response: %w", err)
	}
	return &out, nil
}
