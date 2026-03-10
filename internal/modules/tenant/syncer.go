package tenant

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/google/uuid"

	"github.com/bengobox/ordering-backend/internal/ent"
	enttenant "github.com/bengobox/ordering-backend/internal/ent/tenant"
)

// Syncer handles dynamic syncing of tenant data from auth-api.
type Syncer struct {
	client *ent.Client
}

// NewSyncer creates a new TenantSyncer.
func NewSyncer(client *ent.Client) *Syncer {
	return &Syncer{client: client}
}

// authAPITenantResponse is the full tenant JSON response from GET /api/v1/tenants/by-slug/{slug}.
type authAPITenantResponse struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	Slug                string         `json:"slug"`
	Status              string         `json:"status"`
	ContactEmail        string         `json:"contact_email"`
	ContactPhone        string         `json:"contact_phone"`
	LogoURL             string         `json:"logo_url"`
	Website             string         `json:"website"`
	Country             string         `json:"country"`
	Timezone            string         `json:"timezone"`
	BrandColors         map[string]any `json:"brand_colors"`
	OrgSize             string         `json:"org_size"`
	UseCase             string         `json:"use_case"`
	SubscriptionPlan    string         `json:"subscription_plan"`
	SubscriptionStatus  string         `json:"subscription_status"`
	TierLimits          map[string]any `json:"tier_limits"`
	Metadata            map[string]any `json:"metadata"`
}

// SyncTenant fetches the FULL tenant record from auth-api and persists it
// in the local DB with the same UUID as auth-api. Used for JIT provisioning.
func (s *Syncer) SyncTenant(ctx context.Context, slug string) (uuid.UUID, error) {
	// Fast path: check if tenant already exists locally
	existingFast, err := s.client.Tenant.Query().Where(enttenant.SlugEQ(slug)).Only(ctx)
	if err == nil && existingFast != nil {
		return existingFast.ID, nil
	}

	authAPIURL := os.Getenv("AUTH_API_URL")
	if authAPIURL == "" {
		authAPIURL = "https://sso.codevertexitsolutions.com"
	}
	endpoint := strings.TrimRight(authAPIURL, "/") + "/api/v1/tenants/by-slug/" + slug

	log.Printf("  [tenant-sync] dynamically fetching %s from %s", slug, endpoint)
	resp, err := http.Get(endpoint) //nolint:noctx
	if err != nil {
		return uuid.Nil, fmt.Errorf("tenant.Syncer: GET %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return uuid.Nil, fmt.Errorf("tenant.Syncer: tenant %q not found (404)", slug)
	}
	if resp.StatusCode != http.StatusOK {
		return uuid.Nil, fmt.Errorf("tenant.Syncer: auth-api HTTP %d for %q", resp.StatusCode, slug)
	}

	var remote authAPITenantResponse
	if err := json.NewDecoder(resp.Body).Decode(&remote); err != nil {
		return uuid.Nil, fmt.Errorf("tenant.Syncer: decode response: %w", err)
	}
	realID, err := uuid.Parse(remote.ID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("tenant.Syncer: invalid UUID %q: %w", remote.ID, err)
	}

	// Make a transaction.
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("tenant.Syncer: start tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
			return
		}
		_ = tx.Commit()
	}()

	existing, queryErr := tx.Tenant.Query().Where(enttenant.IDEQ(realID)).Only(ctx)

	extMeta := map[string]any{}
	for k, v := range remote.Metadata {
		extMeta[k] = v
	}
	if remote.LogoURL != "" {
		extMeta["logo_url"] = remote.LogoURL
	}
	if remote.Website != "" {
		extMeta["website"] = remote.Website
	}
	if remote.Country != "" {
		extMeta["country"] = remote.Country
	}
	if remote.Timezone != "" {
		extMeta["timezone"] = remote.Timezone
	}
	if len(remote.BrandColors) > 0 {
		extMeta["brand_colors"] = remote.BrandColors
	}
	if remote.OrgSize != "" {
		extMeta["org_size"] = remote.OrgSize
	}
	if remote.UseCase != "" {
		extMeta["use_case"] = remote.UseCase
	}
	if remote.SubscriptionPlan != "" {
		extMeta["subscription_plan"] = remote.SubscriptionPlan
	}
	if remote.SubscriptionStatus != "" {
		extMeta["subscription_status"] = remote.SubscriptionStatus
	}
	if len(remote.TierLimits) > 0 {
		extMeta["tier_limits"] = remote.TierLimits
	}

	if queryErr == nil && existing != nil {
		upd := existing.Update().
			SetName(remote.Name).
			SetStatus(remote.Status).
			SetContactEmail(remote.ContactEmail).
			SetContactPhone(remote.ContactPhone).
			SetLogoURL(remote.LogoURL).
			SetWebsite(remote.Website).
			SetCountry(remote.Country).
			SetTimezone(remote.Timezone).
			SetBrandColors(remote.BrandColors).
			SetOrgSize(remote.OrgSize).
			SetUseCase(remote.UseCase).
			SetSubscriptionPlan(remote.SubscriptionPlan).
			SetSubscriptionStatus(remote.SubscriptionStatus).
			SetTierLimits(remote.TierLimits).
			SetMetadata(extMeta)
		if _, err := upd.Save(ctx); err != nil {
			return uuid.Nil, fmt.Errorf("tenant.Syncer: update tenant: %w", err)
		}
		log.Printf("  [tenant-sync] updated %s (UUID %s) from auth-api", slug, realID)
		return realID, nil
	}

	if !ent.IsNotFound(queryErr) {
		return uuid.Nil, fmt.Errorf("tenant.Syncer: query existing: %w", queryErr)
	}

	bySlug, _ := tx.Tenant.Query().Where(enttenant.SlugEQ(slug)).Only(ctx)
	if bySlug != nil && bySlug.ID != realID {
		log.Printf("  [WARN] tenant %q exists locally with UUID %s but auth-api says %s", slug, bySlug.ID, realID)
		return bySlug.ID, nil
	}

	create := tx.Tenant.Create().
		SetID(realID).
		SetSlug(remote.Slug).
		SetName(remote.Name).
		SetStatus(remote.Status).
		SetContactEmail(remote.ContactEmail).
		SetContactPhone(remote.ContactPhone).
		SetLogoURL(remote.LogoURL).
		SetWebsite(remote.Website).
		SetCountry(remote.Country).
		SetTimezone(remote.Timezone).
		SetBrandColors(remote.BrandColors).
		SetOrgSize(remote.OrgSize).
		SetUseCase(remote.UseCase).
		SetSubscriptionPlan(remote.SubscriptionPlan).
		SetSubscriptionStatus(remote.SubscriptionStatus).
		SetTierLimits(remote.TierLimits).
		SetMetadata(extMeta)

	created, createErr := create.Save(ctx)
	if createErr != nil {
		return uuid.Nil, fmt.Errorf("tenant.Syncer: create tenant: %w", createErr)
	}

	log.Printf("  [tenant-sync] dynamically created %s (UUID %s, synced from auth-api)", slug, created.ID)
	return created.ID, nil
}
