package authclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// Client handles communication with the auth-service.
type Client struct {
	baseURL    string
	httpClient *http.Client
	logger     *zap.Logger
}

// NewClient creates a new auth-service client.
func NewClient(baseURL string, logger *zap.Logger) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger.Named("auth-service-client"),
	}
}

// LoginRequest represents a login request to auth-service.
type LoginRequest struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	TenantSlug string `json:"tenant_slug"`
}

// RegisterRequest represents a registration request to auth-service.
type RegisterRequest struct {
	Email      string                 `json:"email"`
	Password   string                 `json:"password"`
	TenantSlug string                 `json:"tenant_slug"`
	Profile    map[string]interface{} `json:"profile,omitempty"`
}

// RefreshRequest represents a token refresh request.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// AuthResponse represents the response from auth-service.
type AuthResponse struct {
	AccessToken      string                 `json:"access_token"`
	RefreshToken     string                 `json:"refresh_token"`
	SessionID        string                 `json:"session_id"`
	TokenType        string                 `json:"token_type"`
	ExpiresIn        int                    `json:"expires_in"`
	RefreshExpiresIn int                    `json:"refresh_expires_in"`
	Tenant           map[string]interface{} `json:"tenant"`
	User             map[string]interface{} `json:"user"`
}

// Error represents an error response from auth-service.
type Error struct {
	ErrorField       string `json:"error"`
	ErrorCode        string `json:"error_code,omitempty"`
	ErrorDescription string `json:"error_description,omitempty"`
	Message          string `json:"message,omitempty"`
}

func (e *Error) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.ErrorDescription != "" {
		return e.ErrorDescription
	}
	return e.ErrorField
}

// Login authenticates a user via auth-service.
func (c *Client) Login(ctx context.Context, req LoginRequest) (*AuthResponse, error) {
	url := fmt.Sprintf("%s/api/v1/auth/login", c.baseURL)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("auth-service: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("auth-service: create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.logger.Error("auth-service: login request failed", zap.Error(err), zap.String("url", url), zap.String("email", req.Email))
		return nil, fmt.Errorf("auth-service: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.Error("auth-service: failed to read login response", zap.Error(err), zap.Int("status", resp.StatusCode))
		return nil, fmt.Errorf("auth-service: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		c.logger.Warn("auth-service: login failed",
			zap.Int("status", resp.StatusCode),
			zap.String("response", string(respBody)),
			zap.String("url", url),
			zap.String("email", req.Email))
		var authErr Error
		if err := json.Unmarshal(respBody, &authErr); err == nil {
			return nil, &authErr
		}
		return nil, fmt.Errorf("auth-service: login failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var authResp AuthResponse
	if err := json.Unmarshal(respBody, &authResp); err != nil {
		return nil, fmt.Errorf("auth-service: unmarshal response: %w", err)
	}

	return &authResp, nil
}

// Register registers a new user via auth-service.
func (c *Client) Register(ctx context.Context, req RegisterRequest) (*AuthResponse, error) {
	url := fmt.Sprintf("%s/api/v1/auth/register", c.baseURL)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("auth-service: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("auth-service: create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.logger.Error("auth-service: register request failed", zap.Error(err), zap.String("url", url))
		return nil, fmt.Errorf("auth-service: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.Error("auth-service: failed to read register response", zap.Error(err), zap.Int("status", resp.StatusCode))
		return nil, fmt.Errorf("auth-service: read response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		c.logger.Warn("auth-service: register failed",
			zap.Int("status", resp.StatusCode),
			zap.String("response", string(respBody)),
			zap.String("url", url))
		var authErr Error
		if err := json.Unmarshal(respBody, &authErr); err == nil {
			return nil, &authErr
		}
		return nil, fmt.Errorf("auth-service: register failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var authResp AuthResponse
	if err := json.Unmarshal(respBody, &authResp); err != nil {
		return nil, fmt.Errorf("auth-service: unmarshal response: %w", err)
	}

	return &authResp, nil
}

// Refresh refreshes an access token via auth-service.
func (c *Client) Refresh(ctx context.Context, refreshToken string) (*AuthResponse, error) {
	url := fmt.Sprintf("%s/api/v1/auth/refresh", c.baseURL)

	req := RefreshRequest{
		RefreshToken: refreshToken,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("auth-service: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("auth-service: create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("auth-service: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("auth-service: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var authErr Error
		if err := json.Unmarshal(respBody, &authErr); err == nil {
			return nil, &authErr
		}
		return nil, fmt.Errorf("auth-service: refresh failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var authResp AuthResponse
	if err := json.Unmarshal(respBody, &authResp); err != nil {
		return nil, fmt.Errorf("auth-service: unmarshal response: %w", err)
	}

	return &authResp, nil
}

// GetUser retrieves user details from auth-service.
func (c *Client) GetUser(ctx context.Context, userID string, accessToken string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/users/%s", c.baseURL, userID)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("auth-service: create request: %w", err)
	}

	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("auth-service: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("auth-service: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var authErr Error
		if err := json.Unmarshal(respBody, &authErr); err == nil {
			return nil, &authErr
		}
		return nil, fmt.Errorf("auth-service: get user failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var userData map[string]interface{}
	if err := json.Unmarshal(respBody, &userData); err != nil {
		return nil, fmt.Errorf("auth-service: unmarshal response: %w", err)
	}

	return userData, nil
}

// TenantRequest represents a tenant creation request to auth-service.
type TenantRequest struct {
	ID           string                 `json:"id,omitempty"` // Tenant UUID - must match across all services
	Slug         string                 `json:"slug"`
	Name         string                 `json:"name,omitempty"`
	ContactEmail string                 `json:"contact_email,omitempty"`
	ContactPhone string                 `json:"contact_phone,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// TenantResponse represents a tenant response from auth-service.
type TenantResponse struct {
	ID           string                 `json:"id"`
	Slug         string                 `json:"slug"`
	Name         string                 `json:"name"`
	Status       string                 `json:"status"`
	ContactEmail string                 `json:"contact_email,omitempty"`
	ContactPhone string                 `json:"contact_phone,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt    string                 `json:"created_at"`
	UpdatedAt    string                 `json:"updated_at"`
}

// SyncUserRequest represents the request to sync a user with auth-service.
type SyncUserRequest struct {
	Email      string                 `json:"email"`
	Password   string                 `json:"password,omitempty"`
	TenantSlug string                 `json:"tenant_slug"`
	Profile    map[string]interface{} `json:"profile,omitempty"`
	Service    string                 `json:"service,omitempty"`
}

// SyncUserResponse represents the response from auth-service.
type SyncUserResponse struct {
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	TenantID string `json:"tenant_id"`
	Created  bool   `json:"created"`
	Message  string `json:"message"`
}

// SyncUser syncs a user with auth-service SSO using an API Key.
func (c *Client) SyncUser(ctx context.Context, req SyncUserRequest, apiKey string) (*SyncUserResponse, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("auth-service: API key required for user sync")
	}

	url := fmt.Sprintf("%s/api/v1/admin/users/sync", c.baseURL)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("auth-service: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("auth-service: create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-API-Key", apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.logger.Error("auth-service: sync user request failed", zap.Error(err), zap.String("url", url), zap.String("email", req.Email))
		return nil, fmt.Errorf("auth-service: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.Error("auth-service: failed to read sync response", zap.Error(err), zap.Int("status", resp.StatusCode))
		return nil, fmt.Errorf("auth-service: read response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		c.logger.Warn("auth-service: user sync failed",
			zap.Int("status", resp.StatusCode),
			zap.String("response", string(respBody)),
			zap.String("email", req.Email))

		var errResp map[string]interface{}
		if err := json.Unmarshal(respBody, &errResp); err == nil {
			// Log parsed error for easier debugging
			c.logger.Debug("auth-service: sync error details", zap.Any("error_response", errResp))
		}

		return nil, fmt.Errorf("auth-service: user sync failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var syncResp SyncUserResponse
	if err := json.NewDecoder(bytes.NewReader(respBody)).Decode(&syncResp); err != nil {
		return nil, fmt.Errorf("auth-service: decode sync response: %w", err)
	}

	c.logger.Info("auth-service: user synced",
		zap.String("user_id", syncResp.UserID),
		zap.String("email", syncResp.Email),
		zap.Bool("created", syncResp.Created),
	)

	return &syncResp, nil
}

// CheckTenantExists checks if a tenant exists in auth-service by slug.
// Returns true if tenant exists, false if not found, error for other failures.
func (c *Client) CheckTenantExists(ctx context.Context, tenantSlug string) (bool, error) {
	url := fmt.Sprintf("%s/api/v1/tenants/by-slug/%s", c.baseURL, tenantSlug)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("auth-service: create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/json")
	// Note: Tenant check endpoint should be public (no auth required)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.logger.Error("auth-service: tenant check request failed", zap.Error(err), zap.String("url", url), zap.String("tenant_slug", tenantSlug))
		return false, fmt.Errorf("auth-service: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.Error("auth-service: failed to read tenant check response", zap.Error(err), zap.Int("status", resp.StatusCode))
		return false, fmt.Errorf("auth-service: read response: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return false, nil // Tenant doesn't exist
	}

	if resp.StatusCode != http.StatusOK {
		c.logger.Warn("auth-service: tenant check failed",
			zap.Int("status", resp.StatusCode),
			zap.String("response", string(respBody)),
			zap.String("url", url),
			zap.String("tenant_slug", tenantSlug))
		var authErr Error
		if err := json.Unmarshal(respBody, &authErr); err == nil {
			return false, &authErr
		}
		return false, fmt.Errorf("auth-service: tenant check failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	// Tenant exists
	return true, nil
}

// CreateTenant creates a new tenant in auth-service.
// Note: This endpoint should not require authentication (public endpoint for tenant auto-discovery).
func (c *Client) CreateTenant(ctx context.Context, req TenantRequest) (*TenantResponse, error) {
	url := fmt.Sprintf("%s/api/v1/tenants", c.baseURL)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("auth-service: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("auth-service: create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	// Note: Tenant creation endpoint should be public (no auth required for auto-discovery)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.logger.Error("auth-service: create tenant request failed", zap.Error(err), zap.String("url", url), zap.String("tenant_slug", req.Slug))
		return nil, fmt.Errorf("auth-service: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.Error("auth-service: failed to read create tenant response", zap.Error(err), zap.Int("status", resp.StatusCode))
		return nil, fmt.Errorf("auth-service: read response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		c.logger.Warn("auth-service: create tenant failed",
			zap.Int("status", resp.StatusCode),
			zap.String("response", string(respBody)),
			zap.String("url", url),
			zap.String("tenant_slug", req.Slug))
		var authErr Error
		if err := json.Unmarshal(respBody, &authErr); err == nil {
			return nil, &authErr
		}
		return nil, fmt.Errorf("auth-service: create tenant failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var tenantResp TenantResponse
	if err := json.Unmarshal(respBody, &tenantResp); err != nil {
		return nil, fmt.Errorf("auth-service: unmarshal response: %w", err)
	}

	c.logger.Info("auth-service: tenant created successfully", zap.String("tenant_slug", req.Slug), zap.String("tenant_id", tenantResp.ID))
	return &tenantResp, nil
}
