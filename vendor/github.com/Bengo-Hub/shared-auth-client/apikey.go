package authclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// APIKeyValidator validates API keys by checking them against auth-service.
// Supports both service-to-service authentication and external API access.
type APIKeyValidator struct {
	authServiceURL string
	httpClient     *http.Client
	cache          map[string]*apiKeyInfo
	cacheTTL       time.Duration
}

type apiKeyInfo struct {
	clientID             string
	tenantID             string
	tenantSlug           string
	scopes               []string
	roles                []string
	service              string
	subscriptionPlan     string
	subscriptionFeatures []string
	subscriptionLimits   map[string]int
	subscriptionStatus   string
	expiresAt            time.Time
}

// APIKeyValidationResult contains the full result of API key validation.
type APIKeyValidationResult struct {
	ClientID             string         `json:"client_id"`
	TenantID             string         `json:"tenant_id"`
	TenantSlug           string         `json:"tenant_slug"`
	Scopes               []string       `json:"scopes"`
	Roles                []string       `json:"roles"`
	Service              string         `json:"service"`
	SubscriptionPlan     string         `json:"subscription_plan"`
	SubscriptionFeatures []string       `json:"subscription_features"`
	SubscriptionLimits   map[string]int `json:"subscription_limits"`
	SubscriptionStatus   string         `json:"subscription_status"`
}

// NewAPIKeyValidator creates a new API key validator.
func NewAPIKeyValidator(authServiceURL string, httpClient *http.Client) *APIKeyValidator {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &APIKeyValidator{
		authServiceURL: strings.TrimSuffix(authServiceURL, "/"),
		httpClient:     httpClient,
		cache:          make(map[string]*apiKeyInfo),
		cacheTTL:       5 * time.Minute,
	}
}

// ValidateAPIKey validates an API key by checking it against auth-service.
// Returns client_id, tenant_id, scopes, and service if valid.
// Deprecated: Use ValidateAPIKeyFull for complete subscription data.
func (v *APIKeyValidator) ValidateAPIKey(ctx context.Context, apiKey string) (clientID, tenantID string, scopes []string, service string, err error) {
	result, err := v.ValidateAPIKeyFull(ctx, apiKey)
	if err != nil {
		return "", "", nil, "", err
	}
	return result.ClientID, result.TenantID, result.Scopes, result.Service, nil
}

// ValidateAPIKeyFull validates an API key and returns complete information including subscription data.
func (v *APIKeyValidator) ValidateAPIKeyFull(ctx context.Context, apiKey string) (*APIKeyValidationResult, error) {
	// Check cache first
	if info, ok := v.cache[apiKey]; ok {
		if time.Now().Before(info.expiresAt) {
			return &APIKeyValidationResult{
				ClientID:             info.clientID,
				TenantID:             info.tenantID,
				TenantSlug:           info.tenantSlug,
				Scopes:               info.scopes,
				Roles:                info.roles,
				Service:              info.service,
				SubscriptionPlan:     info.subscriptionPlan,
				SubscriptionFeatures: info.subscriptionFeatures,
				SubscriptionLimits:   info.subscriptionLimits,
				SubscriptionStatus:   info.subscriptionStatus,
			}, nil
		}
		// Cache expired, remove it
		delete(v.cache, apiKey)
	}

	// Validate against auth-service
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/api/v1/admin/api-keys/validate", v.authServiceURL), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("X-API-Key", apiKey)

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("invalid API key: status %d", resp.StatusCode)
	}

	var result APIKeyValidationResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// Cache the result
	v.cache[apiKey] = &apiKeyInfo{
		clientID:             result.ClientID,
		tenantID:             result.TenantID,
		tenantSlug:           result.TenantSlug,
		scopes:               result.Scopes,
		roles:                result.Roles,
		service:              result.Service,
		subscriptionPlan:     result.SubscriptionPlan,
		subscriptionFeatures: result.SubscriptionFeatures,
		subscriptionLimits:   result.SubscriptionLimits,
		subscriptionStatus:   result.SubscriptionStatus,
		expiresAt:            time.Now().Add(v.cacheTTL),
	}

	return &result, nil
}

// ToClaims converts an API key validation result to Claims for consistent handling.
func (r *APIKeyValidationResult) ToClaims() *Claims {
	return &Claims{
		TenantID:             r.TenantID,
		TenantSlug:           r.TenantSlug,
		Scope:                r.Scopes,
		Roles:                r.Roles,
		ServiceName:          r.Service,
		IsService:            r.Service != "",
		SubscriptionPlan:     r.SubscriptionPlan,
		SubscriptionFeatures: r.SubscriptionFeatures,
		SubscriptionLimits:   r.SubscriptionLimits,
		SubscriptionStatus:   r.SubscriptionStatus,
	}
}
