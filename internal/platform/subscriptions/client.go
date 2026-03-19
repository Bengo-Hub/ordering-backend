package subscriptions

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	serviceclient "github.com/Bengo-Hub/shared-service-client"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/config"
)

// SubscriptionStatus represents the tenant's subscription response from subscriptions-api.
type SubscriptionStatus struct {
	ID                 uuid.UUID  `json:"id"`
	TenantID           uuid.UUID  `json:"tenant_id"`
	PlanCode           string     `json:"plan_code"`
	PlanName           string     `json:"plan_name"`
	Status             string     `json:"status"`
	CurrentPeriodEnd   time.Time  `json:"current_period_end"`
}

// IsActive returns true when the subscription status allows service usage.
func (s *SubscriptionStatus) IsActive() bool {
	return s.Status == "ACTIVE" || s.Status == "TRIAL" ||
		s.Status == "active" || s.Status == "trial"
}

// Client interacts with the subscriptions service.
type Client struct {
	baseURL       string
	apiKey        string
	serviceClient *serviceclient.Client
	httpClient    *http.Client
	logger        *zap.Logger
}

// NewClient creates a new subscriptions service client.
func NewClient(cfg config.SubscriptionsConfig, logger *zap.Logger) *Client {
	scCfg := serviceclient.DefaultConfig(
		cfg.ServiceURL,
		"ordering-service",
		logger.Named("subscriptions.client"),
	)
	scCfg.Timeout = cfg.RequestTimeout

	return &Client{
		baseURL:       cfg.ServiceURL,
		apiKey:        cfg.APIKey,
		serviceClient: serviceclient.New(scCfg),
		httpClient:    &http.Client{Timeout: cfg.RequestTimeout},
		logger:        logger.Named("subscriptions.client"),
	}
}

// GetSubscription fetches the current subscription for a tenant.
// Returns ErrNotFound if no subscription record exists.
func (c *Client) GetSubscription(ctx context.Context, tenantID uuid.UUID, bearerToken string) (*SubscriptionStatus, error) {
	url := fmt.Sprintf("%s/api/v1/subscription", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("subscriptions: build request: %w", err)
	}
	req.Header.Set("X-Tenant-ID", tenantID.String())
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	} else if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("subscriptions: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // no subscription — treat as inactive
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("subscriptions: unexpected status %d", resp.StatusCode)
	}

	var sub SubscriptionStatus
	if err := json.NewDecoder(resp.Body).Decode(&sub); err != nil {
		return nil, fmt.Errorf("subscriptions: decode response: %w", err)
	}
	return &sub, nil
}

// IsSubscriptionActive checks whether a tenant's subscription is active or in trial.
// Returns true (allowing order creation) if the subscription-api is unreachable — fail open
// to avoid blocking orders due to subscriptions-api downtime.
func (c *Client) IsSubscriptionActive(ctx context.Context, tenantID uuid.UUID, bearerToken string) bool {
	sub, err := c.GetSubscription(ctx, tenantID, bearerToken)
	if err != nil {
		c.logger.Warn("subscriptions check failed, failing open", zap.String("tenant_id", tenantID.String()), zap.Error(err))
		return true // fail open on connectivity errors
	}
	if sub == nil {
		return false // no subscription record
	}
	return sub.IsActive()
}
