// Package posdiscounts is an S2S client for pos-api's discount/promotion banner endpoint.
//
// pos-api's Promotion + PromotionRule are the platform's discount source of truth (see its own
// handler doc comment). Storefront promotional banners are just another view over the same
// Promotion record (a `banner` object stored in Promotion.metadata) — ordering-backend does NOT
// own or duplicate discount data, it only reads the subset flagged `show_on_storefront: true`.
package posdiscounts

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

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
