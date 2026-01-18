package superset

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/config"
)

// Client provides methods to interact with Apache Superset.
type Client struct {
	baseURL       string
	apiVersion    string
	username      string
	password      string
	httpClient    *http.Client
	logger        *zap.Logger
	guestTokenTTL int

	// Token management
	accessToken     string
	tokenExpiry     time.Time
	tokenMu         sync.RWMutex
	refreshToken    string
}

// NewClient creates a new Superset client.
func NewClient(cfg config.SupersetConfig, logger *zap.Logger) *Client {
	return &Client{
		baseURL:       cfg.BaseURL,
		apiVersion:    cfg.APIVersion,
		username:      cfg.AdminUsername,
		password:      cfg.AdminPassword,
		guestTokenTTL: cfg.GuestTokenTTLMinutes,
		httpClient: &http.Client{
			Timeout: cfg.RequestTimeout,
		},
		logger: logger.Named("superset.client"),
	}
}

// --- Authentication ---

// LoginResponse represents the login response from Superset.
type LoginResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

// Login authenticates with Superset and stores the access token.
func (c *Client) Login(ctx context.Context) error {
	url := fmt.Sprintf("%s/api/%s/security/login", c.baseURL, c.apiVersion)

	payload := map[string]interface{}{
		"username": c.username,
		"password": c.password,
		"provider": "db",
		"refresh":  true,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("superset: marshal login payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("superset: create login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("superset: login request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("superset: login failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var loginResp LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return fmt.Errorf("superset: decode login response: %w", err)
	}

	c.tokenMu.Lock()
	c.accessToken = loginResp.AccessToken
	c.refreshToken = loginResp.RefreshToken
	c.tokenExpiry = time.Now().Add(4 * time.Minute) // Refresh 1 minute before 5-minute expiry
	c.tokenMu.Unlock()

	c.logger.Info("Logged in to Superset successfully")
	return nil
}

// ensureAuthenticated ensures we have a valid access token.
func (c *Client) ensureAuthenticated(ctx context.Context) error {
	c.tokenMu.RLock()
	tokenValid := c.accessToken != "" && time.Now().Before(c.tokenExpiry)
	c.tokenMu.RUnlock()

	if tokenValid {
		return nil
	}

	return c.Login(ctx)
}

// getAccessToken returns the current access token.
func (c *Client) getAccessToken() string {
	c.tokenMu.RLock()
	defer c.tokenMu.RUnlock()
	return c.accessToken
}

// --- Guest Token ---

// GuestTokenRequest represents a request to generate a guest token.
type GuestTokenRequest struct {
	User      GuestUser `json:"user"`
	Resources []RLSRule `json:"resources"`
	RLS       []RLSRule `json:"rls"`
}

// GuestUser represents the guest user for token generation.
type GuestUser struct {
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// RLSRule represents a Row-Level Security rule.
type RLSRule struct {
	Type     string `json:"type"`
	ID       int    `json:"id,omitempty"`
	Clause   string `json:"clause,omitempty"`
}

// GuestTokenResponse represents the guest token response.
type GuestTokenResponse struct {
	Token string `json:"token"`
}

// GenerateGuestToken generates a guest token for embedding dashboards with RLS.
func (c *Client) GenerateGuestToken(ctx context.Context, dashboardID int, tenantID uuid.UUID, userID uuid.UUID) (string, error) {
	if err := c.ensureAuthenticated(ctx); err != nil {
		return "", fmt.Errorf("superset: authentication failed: %w", err)
	}

	url := fmt.Sprintf("%s/api/%s/security/guest_token/", c.baseURL, c.apiVersion)

	// Build RLS clause to filter by tenant_id
	rlsClause := fmt.Sprintf("tenant_id = '%s'", tenantID.String())

	payload := GuestTokenRequest{
		User: GuestUser{
			Username:  userID.String(),
			FirstName: "Guest",
			LastName:  "User",
		},
		Resources: []RLSRule{
			{
				Type: "dashboard",
				ID:   dashboardID,
			},
		},
		RLS: []RLSRule{
			{
				Clause: rlsClause,
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("superset: marshal guest token payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("superset: create guest token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.getAccessToken())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("superset: guest token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("superset: guest token failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var tokenResp GuestTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("superset: decode guest token response: %w", err)
	}

	return tokenResp.Token, nil
}

// --- Dashboard Operations ---

// Dashboard represents a Superset dashboard.
type Dashboard struct {
	ID              int                    `json:"id"`
	DashboardTitle  string                 `json:"dashboard_title"`
	Slug            string                 `json:"slug"`
	URL             string                 `json:"url"`
	Position        map[string]interface{} `json:"position_json,omitempty"`
	CSSClass        string                 `json:"css,omitempty"`
	PublishedStatus bool                   `json:"published"`
}

// GetDashboard retrieves a dashboard by ID.
func (c *Client) GetDashboard(ctx context.Context, dashboardID int) (*Dashboard, error) {
	if err := c.ensureAuthenticated(ctx); err != nil {
		return nil, fmt.Errorf("superset: authentication failed: %w", err)
	}

	url := fmt.Sprintf("%s/api/%s/dashboard/%d", c.baseURL, c.apiVersion, dashboardID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("superset: create dashboard request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.getAccessToken())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("superset: dashboard request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrDashboardNotFound
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("superset: get dashboard failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Result Dashboard `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("superset: decode dashboard response: %w", err)
	}

	return &result.Result, nil
}

// ListDashboards retrieves all dashboards.
func (c *Client) ListDashboards(ctx context.Context) ([]Dashboard, error) {
	if err := c.ensureAuthenticated(ctx); err != nil {
		return nil, fmt.Errorf("superset: authentication failed: %w", err)
	}

	url := fmt.Sprintf("%s/api/%s/dashboard/", c.baseURL, c.apiVersion)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("superset: create dashboards request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.getAccessToken())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("superset: dashboards request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("superset: list dashboards failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Result []Dashboard `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("superset: decode dashboards response: %w", err)
	}

	return result.Result, nil
}

// --- Embed URL Builder ---

// EmbedURL represents an embedded dashboard URL.
type EmbedURL struct {
	URL       string    `json:"url"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// GetEmbedURL generates an embed URL for a dashboard with RLS.
func (c *Client) GetEmbedURL(ctx context.Context, dashboardID int, tenantID, userID uuid.UUID) (*EmbedURL, error) {
	// Generate guest token with RLS
	token, err := c.GenerateGuestToken(ctx, dashboardID, tenantID, userID)
	if err != nil {
		return nil, err
	}

	// Build embed URL
	embedURL := fmt.Sprintf("%s/superset/embedded/%d/?guest_token=%s", c.baseURL, dashboardID, token)

	return &EmbedURL{
		URL:       embedURL,
		Token:     token,
		ExpiresAt: time.Now().Add(time.Duration(c.guestTokenTTL) * time.Minute),
	}, nil
}

// GetBaseURL returns the Superset base URL.
func (c *Client) GetBaseURL() string {
	return c.baseURL
}
