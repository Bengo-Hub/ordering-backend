package tenant

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/bengobox/ordering-backend/internal/ent"
	entoutlet "github.com/bengobox/ordering-backend/internal/ent/outlet"
	enttenant "github.com/bengobox/ordering-backend/internal/ent/tenant"
)

// Syncer handles dynamic syncing of tenant data from auth-api.
type Syncer struct {
	client  *ent.Client
	authURL string
}

// NewSyncer creates a new TenantSyncer.
// authURL is the base URL of the auth-api (e.g. from AUTH_SERVICE_URL config).
func NewSyncer(client *ent.Client, authURL string) *Syncer {
	return &Syncer{client: client, authURL: authURL}
}

// authAPITenantResponse is the minimal tenant JSON response from GET /api/v1/tenants/by-slug/{slug}.
// Only fields that this service persists locally are included.
type authAPITenantResponse struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Slug    string `json:"slug"`
	Status  string `json:"status"`
	UseCase string `json:"use_case"`
}

// s2sHTTPClient is used for all auth-api S2S calls. The Timeout bounds every
// request so a slow/hung auth-api cannot block the caller (incl. NATS event
// handlers and JIT-provisioning paths) indefinitely.
var s2sHTTPClient = &http.Client{Timeout: 15 * time.Second}

// SyncTenant fetches the tenant record from auth-api and persists the minimal
// reference in the local DB with the same UUID as auth-api. Used for JIT provisioning.
func (s *Syncer) SyncTenant(ctx context.Context, slug string) (uuid.UUID, error) {
	// Fast path: check if tenant already exists locally
	existingFast, err := s.client.Tenant.Query().Where(enttenant.SlugEQ(slug)).Only(ctx)
	if err == nil && existingFast != nil {
		return existingFast.ID, nil
	}

	authAPIURL := s.authURL
	if envURL := os.Getenv("AUTH_API_URL"); envURL != "" {
		authAPIURL = envURL
	}
	endpoint := strings.TrimRight(authAPIURL, "/") + "/api/v1/tenants/by-slug/" + slug

	log.Printf("  [tenant-sync] dynamically fetching %s from %s", slug, endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return uuid.Nil, fmt.Errorf("tenant.Syncer: build request: %w", err)
	}
	if svcKey := os.Getenv("INTERNAL_SERVICE_KEY"); svcKey != "" {
		req.Header.Set("X-API-Key", svcKey)
	}
	resp, err := s2sHTTPClient.Do(req)
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

	now := time.Now()

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

	if queryErr == nil && existing != nil {
		upd := existing.Update().
			SetName(remote.Name).
			SetStatus(remote.Status).
			SetNillableUseCase(nillableStr(remote.UseCase)).
			SetSyncStatus("synced").
			SetLastSyncAt(now)
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
		SetNillableUseCase(nillableStr(remote.UseCase)).
		SetSyncStatus("synced").
		SetLastSyncAt(now)

	created, createErr := create.Save(ctx)
	if createErr != nil {
		return uuid.Nil, fmt.Errorf("tenant.Syncer: create tenant: %w", createErr)
	}

	log.Printf("  [tenant-sync] dynamically created %s (UUID %s, synced from auth-api)", slug, created.ID)
	return created.ID, nil
}

// nillableStr returns a *string if non-empty, nil otherwise.
func nillableStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

type authAPIOutletResponse struct {
	ID      string `json:"id"`
	Code    string `json:"code"`
	Name    string `json:"name"`
	UseCase string `json:"use_case"`
	IsHQ    bool   `json:"is_hq"`
	Status  string `json:"status"`
	Address string `json:"address"`
}

// BootstrapOutlets fetches all active outlets for tenantSlug from auth-api and
// upserts them into the local outlets table. Intended for seed / startup when
// JetStream events may have aged out (72h TTL). Skips use-cases not applicable
// to ordering (e.g. pharmacy, salon).
func (s *Syncer) BootstrapOutlets(ctx context.Context, tenantSlug string, tenantID uuid.UUID) error {
	authAPIURL := s.authURL
	if envURL := os.Getenv("AUTH_API_URL"); envURL != "" {
		authAPIURL = envURL
	}
	svcKey := os.Getenv("INTERNAL_SERVICE_KEY")

	endpoint := strings.TrimRight(authAPIURL, "/") + "/api/v1/tenants/" + tenantSlug + "/outlets"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("outlet bootstrap: build request: %w", err)
	}
	if svcKey != "" {
		req.Header.Set("X-API-Key", svcKey)
	}

	resp, err := s2sHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("outlet bootstrap: GET %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("outlet bootstrap: auth-api HTTP %d", resp.StatusCode)
	}

	var outlets []authAPIOutletResponse
	if err := json.NewDecoder(resp.Body).Decode(&outlets); err != nil {
		return fmt.Errorf("outlet bootstrap: decode: %w", err)
	}

	synced, skipped := 0, 0
	for _, o := range outlets {
		if o.UseCase != "" && !orderingAcceptedUseCases[o.UseCase] {
			skipped++
			continue
		}
		outletID, parseErr := uuid.Parse(o.ID)
		if parseErr != nil {
			log.Printf("  [outlet-bootstrap] invalid outlet_id %q, skipping", o.ID)
			continue
		}
		slug := strings.ToLower(strings.ReplaceAll(o.Code, " ", "-"))
		if slug == "" {
			slug = o.ID[:8]
		}
		status := o.Status
		if status == "" {
			status = "active"
		}

		existing, getErr := s.client.Outlet.Get(ctx, outletID)
		if getErr != nil {
			// Create
			create := s.client.Outlet.Create().
				SetID(outletID).
				SetTenantID(tenantID).
				SetName(o.Name).
				SetSlug(slug).
				SetStatus(status).
				SetSupportsPickup(!o.IsHQ)
			if o.UseCase != "" {
				create = create.SetUseCase(o.UseCase)
			}
			if o.Address != "" {
				create = create.SetAddress(o.Address)
			}
			if _, createErr := create.Save(ctx); createErr != nil {
				log.Printf("  [outlet-bootstrap] create %s (%s): %v", o.Name, o.ID, createErr)
				continue
			}
		} else {
			// Update
			upd := s.client.Outlet.UpdateOne(existing).SetName(o.Name).SetStatus(status)
			if o.UseCase != "" {
				upd = upd.SetUseCase(o.UseCase)
			}
			if o.Address != "" {
				upd = upd.SetAddress(o.Address)
			}
			if _, updErr := upd.Save(ctx); updErr != nil {
				log.Printf("  [outlet-bootstrap] update %s (%s): %v", o.Name, o.ID, updErr)
				continue
			}
		}
		synced++
	}

	log.Printf("  [outlet-bootstrap] %s: synced %d outlet(s), skipped %d non-ordering use-case(s)", tenantSlug, synced, skipped)
	return nil
}

// BootstrapOutletsIfEmpty calls BootstrapOutlets only when the tenant has no outlets
// in the local DB. Safe to call on every seed run.
func (s *Syncer) BootstrapOutletsIfEmpty(ctx context.Context, tenantSlug string, tenantID uuid.UUID) error {
	count, err := s.client.Outlet.Query().Where(entoutlet.TenantID(tenantID)).Count(ctx)
	if err != nil {
		return fmt.Errorf("outlet bootstrap: count: %w", err)
	}
	if count > 0 {
		log.Printf("  [outlet-bootstrap] %s: %d outlet(s) already present, skipping bootstrap", tenantSlug, count)
		return nil
	}
	return s.BootstrapOutlets(ctx, tenantSlug, tenantID)
}
