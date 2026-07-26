// Package posloyalty is an S2S client for pos-api's loyalty endpoints.
//
// pos-api is the decided source of truth for loyalty balances (keyed on tenant + customer_phone).
// ordering-backend mirrors its earn/redeem operations here so pos-api holds the authoritative
// balance. All calls are best-effort: a failure is logged and swallowed and must NEVER block an
// order — ordering keeps its own (legacy) LoyaltyAccount as a fallback during this transition.
package posloyalty

import (
	"bytes"
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

// Client calls pos-api's /api/v1/s2s/{tenant}/loyalty/* endpoints with the shared
// INTERNAL_SERVICE_KEY sent as the X-API-Key header.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	log        *zap.Logger
}

// NewClient builds a pos-api loyalty S2S client. baseURL is POS_API_URL (falls back to
// DefaultBaseURL when empty); apiKey is the shared INTERNAL_SERVICE_KEY. When apiKey is empty the
// client is disabled (Enabled() == false) and all calls are graceful no-ops.
func NewClient(baseURL, apiKey string, log *zap.Logger) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 8 * time.Second},
		log:        log.Named("posloyalty-client"),
	}
}

// Enabled reports whether the client is configured (API key set). Without the shared service key
// pos-api would reject every call, so we disable the client entirely and no-op.
func (c *Client) Enabled() bool { return c != nil && c.apiKey != "" }

type earnRequest struct {
	CustomerPhone string  `json:"customer_phone"`
	CustomerName  string  `json:"customer_name,omitempty"`
	Points        int     `json:"points"`
	OrderID       *string `json:"order_id,omitempty"`
}

type redeemRequest struct {
	CustomerPhone string  `json:"customer_phone"`
	Points        int     `json:"points"`
	OrderID       *string `json:"order_id,omitempty"`
}

// balanceResponse is the subset of the S2S balance payload we consume.
type balanceResponse struct {
	Balance int  `json:"balance"`
	Exists  bool `json:"exists"`
}

// Earn credits points to the (tenant, phone) loyalty account on pos-api. Best-effort: returns an
// error for logging only — callers must not fail the order on it.
func (c *Client) Earn(ctx context.Context, tenantID uuid.UUID, phone, name string, points int, orderID *uuid.UUID) error {
	if !c.Enabled() {
		return nil
	}
	phone = strings.TrimSpace(phone)
	if phone == "" || points <= 0 {
		// No phone means pos-api can't key the account; skip silently (legacy local balance still applies).
		return nil
	}
	body := earnRequest{CustomerPhone: phone, CustomerName: name, Points: points, OrderID: orderIDPtr(orderID)}
	return c.post(ctx, tenantID, "earn", body)
}

// Redeem debits points from the (tenant, phone) loyalty account on pos-api. Best-effort.
func (c *Client) Redeem(ctx context.Context, tenantID uuid.UUID, phone string, points int, orderID *uuid.UUID) error {
	if !c.Enabled() {
		return nil
	}
	phone = strings.TrimSpace(phone)
	if phone == "" || points <= 0 {
		return nil
	}
	body := redeemRequest{CustomerPhone: phone, Points: points, OrderID: orderIDPtr(orderID)}
	return c.post(ctx, tenantID, "redeem", body)
}

// Balance returns the authoritative pos-api balance for the (tenant, phone) account, whether the
// account exists, and an error (best-effort).
func (c *Client) Balance(ctx context.Context, tenantID uuid.UUID, phone string) (points int, exists bool, err error) {
	if !c.Enabled() {
		return 0, false, nil
	}
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return 0, false, nil
	}
	url := fmt.Sprintf("%s/api/v1/s2s/%s/loyalty/balance?phone=%s", c.baseURL, tenantID.String(), phone)
	req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if reqErr != nil {
		return 0, false, reqErr
	}
	req.Header.Set("X-API-Key", c.apiKey)
	resp, doErr := c.httpClient.Do(req)
	if doErr != nil {
		c.log.Warn("posloyalty: balance request failed", zap.Error(doErr))
		return 0, false, doErr
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		c.log.Warn("posloyalty: balance unexpected status", zap.Int("status", resp.StatusCode))
		return 0, false, fmt.Errorf("posloyalty: unexpected status %d", resp.StatusCode)
	}
	var out balanceResponse
	if decErr := json.NewDecoder(resp.Body).Decode(&out); decErr != nil {
		return 0, false, decErr
	}
	return out.Balance, out.Exists, nil
}

// post sends a JSON body to /api/v1/s2s/{tenant}/loyalty/{action} with the service key header.
func (c *Client) post(ctx context.Context, tenantID uuid.UUID, action string, body any) error {
	payload, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/api/v1/s2s/%s/loyalty/%s", c.baseURL, tenantID.String(), action)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		c.log.Warn("posloyalty: build request failed", zap.String("action", action), zap.Error(err))
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.log.Warn("posloyalty: request failed", zap.String("action", action), zap.Error(err))
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		c.log.Warn("posloyalty: unexpected status", zap.String("action", action), zap.Int("status", resp.StatusCode))
		return fmt.Errorf("posloyalty: %s returned status %d", action, resp.StatusCode)
	}
	return nil
}

func orderIDPtr(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}
