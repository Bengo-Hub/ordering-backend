package authclient

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Claims represents JWT claims from auth-service.
// Supports both user authentication (JWT) and service authentication (API Key).
// Includes subscription data for feature gating without per-request lookups.
type Claims struct {
	// Core identity
	SessionID  string   `json:"sid"`
	TenantID   string   `json:"tenant_id,omitempty"`
	TenantSlug string   `json:"tenant_slug,omitempty"`
	Scope      []string `json:"scope,omitempty"`
	Email      string   `json:"email,omitempty"`
	IsPlatformOwner bool `json:"is_platform_owner,omitempty"`

	// RBAC - Global roles from auth-service
	Roles []string `json:"roles,omitempty"`

	// Subscription data - embedded at token issuance for zero-latency feature gating
	SubscriptionPlan     string         `json:"subscription_plan,omitempty"`     // e.g., "STARTER", "GROWTH", "PROFESSIONAL"
	SubscriptionFeatures []string       `json:"subscription_features,omitempty"` // enabled feature codes
	SubscriptionLimits   map[string]int `json:"subscription_limits,omitempty"`   // usage limits per metric
	SubscriptionStatus   string         `json:"subscription_status,omitempty"`   // "ACTIVE", "TRIAL", "EXPIRED", "CANCELLED"
	SubscriptionExpires  *time.Time     `json:"subscription_expires,omitempty"`  // current period end

	// Service account identification (for API Key auth)
	ServiceName string `json:"service_name,omitempty"` // e.g., "ordering-service", "logistics-service"
	IsService   bool   `json:"is_service,omitempty"`   // true if this is a service account, not a user

	jwt.RegisteredClaims
}

// UserID returns the user ID as UUID.
func (c *Claims) UserID() (uuid.UUID, error) {
	if c.Subject == "" {
		return uuid.Nil, jwt.ErrInvalidKey
	}
	return uuid.Parse(c.Subject)
}

// Valid implements jwt.Claims interface.
// In jwt/v5, validation is handled by the parser, so we just check basic requirements.
func (c *Claims) Valid() error {
	if c.Subject == "" {
		return jwt.ErrInvalidKey
	}
	// RegisteredClaims validation is handled by parser
	return nil
}

// TenantUUID returns the tenant ID as UUID if present.
func (c *Claims) TenantUUID() (*uuid.UUID, error) {
	if c.TenantID == "" {
		return nil, nil
	}
	id, err := uuid.Parse(c.TenantID)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// GetTenantSlug returns the tenant slug from claims, or empty string if not present.
func (c *Claims) GetTenantSlug() string {
	return c.TenantSlug
}

// HasScope checks if the token has a specific scope.
func (c *Claims) HasScope(scope string) bool {
	for _, s := range c.Scope {
		if s == scope {
			return true
		}
	}
	return false
}

// HasAnyScope checks if the token has any of the provided scopes.
func (c *Claims) HasAnyScope(scopes ...string) bool {
	for _, required := range scopes {
		if c.HasScope(required) {
			return true
		}
	}
	return false
}

// HasAllScopes checks if the token has all of the provided scopes.
func (c *Claims) HasAllScopes(scopes ...string) bool {
	for _, required := range scopes {
		if !c.HasScope(required) {
			return false
		}
	}
	return true
}

// ============================================================================
// RBAC Role Helpers
// ============================================================================

// HasRole checks if the token has a specific role.
func (c *Claims) HasRole(role string) bool {
	for _, r := range c.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// HasAnyRole checks if the token has any of the provided roles.
func (c *Claims) HasAnyRole(roles ...string) bool {
	for _, required := range roles {
		if c.HasRole(required) {
			return true
		}
	}
	return false
}

// IsSuperuser checks if the token has the superuser role (bypasses all RBAC).
func (c *Claims) IsSuperuser() bool {
	return c.HasRole("superuser")
}

// IsAdmin checks if the token has the admin role.
func (c *Claims) IsAdmin() bool {
	return c.HasRole("admin") || c.IsSuperuser()
}

// ============================================================================
// Subscription Feature Helpers
// ============================================================================

// HasFeature checks if the subscription includes a specific feature.
func (c *Claims) HasFeature(feature string) bool {
	for _, f := range c.SubscriptionFeatures {
		if f == feature {
			return true
		}
	}
	return false
}

// HasAnyFeature checks if the subscription includes any of the provided features.
func (c *Claims) HasAnyFeature(features ...string) bool {
	for _, required := range features {
		if c.HasFeature(required) {
			return true
		}
	}
	return false
}

// HasAllFeatures checks if the subscription includes all of the provided features.
func (c *Claims) HasAllFeatures(features ...string) bool {
	for _, required := range features {
		if !c.HasFeature(required) {
			return false
		}
	}
	return true
}

// GetLimit returns the usage limit for a metric. Returns 0 if not set (unlimited or N/A).
func (c *Claims) GetLimit(metric string) int {
	if c.SubscriptionLimits == nil {
		return 0
	}
	return c.SubscriptionLimits[metric]
}

// IsSubscriptionActive checks if the subscription is currently active.
func (c *Claims) IsSubscriptionActive() bool {
	switch c.SubscriptionStatus {
	case "ACTIVE", "TRIAL":
		return true
	case "EXPIRED", "CANCELLED", "SUSPENDED":
		return false
	default:
		// If no subscription status, assume active (free tier or legacy)
		return true
	}
}

// IsTrialSubscription checks if the subscription is in trial period.
func (c *Claims) IsTrialSubscription() bool {
	return c.SubscriptionStatus == "TRIAL"
}

// ============================================================================
// Plan Tier Helpers
// ============================================================================

// PlanTier returns the subscription plan tier for comparison.
// Higher values = higher tier plans.
func (c *Claims) PlanTier() int {
	switch c.SubscriptionPlan {
	case "STARTER", "FREE":
		return 1
	case "GROWTH", "BASIC":
		return 2
	case "PROFESSIONAL", "PRO":
		return 3
	case "ENTERPRISE":
		return 4
	default:
		return 0 // Unknown plan
	}
}

// IsAtLeastPlan checks if the subscription is at least the specified plan tier.
func (c *Claims) IsAtLeastPlan(plan string) bool {
	requiredTier := (&Claims{SubscriptionPlan: plan}).PlanTier()
	return c.PlanTier() >= requiredTier
}
