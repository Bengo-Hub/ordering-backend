package marketflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Client is an S2S client for the MarketFlow CRM API. The CRM is the single source of truth for
// customer contact data (per shared-docs CROSS-SERVICE-DATA-OWNERSHIP.md); ordering stores only the
// returned crm_contact_id reference on the order, never customer PII.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	log        *zap.Logger
}

// NewClient creates a MarketFlow S2S client. baseURL is MARKETFLOW_API_URL; apiKey is the shared
// INTERNAL_SERVICE_KEY. When baseURL is empty the client is disabled (Enabled() == false) and all
// calls are graceful no-ops.
func NewClient(baseURL, apiKey string, log *zap.Logger) *Client {
	return &Client{
		baseURL:    baseURL,
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 8 * time.Second},
		log:        log.Named("marketflow-client"),
	}
}

// Enabled reports whether the client is configured (base URL set).
func (c *Client) Enabled() bool { return c != nil && c.baseURL != "" }

type upsertContactRequest struct {
	TenantID  string `json:"tenant_id"`
	Phone     string `json:"phone,omitempty"`
	Email     string `json:"email,omitempty"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type upsertContactResponse struct {
	ID string `json:"id"`
}

// UpsertContact creates or returns the existing MarketFlow contact, keyed by phone and/or email
// (the CRM's per-tenant unique keys). Sending both covers guests (phone) and authenticated/SSO
// customers (often email-only). Returns uuid.Nil on any error or when disabled — callers must treat
// the link as optional and never block the order on it.
func (c *Client) UpsertContact(ctx context.Context, tenantID uuid.UUID, phone, email, fullName string) uuid.UUID {
	if !c.Enabled() || (phone == "" && email == "") {
		return uuid.Nil
	}

	firstName, lastName := splitName(fullName)
	payload, _ := json.Marshal(upsertContactRequest{
		TenantID:  tenantID.String(),
		Phone:     phone,
		Email:     email,
		FirstName: firstName,
		LastName:  lastName,
	})

	url := fmt.Sprintf("%s/api/v1/internal/contacts/upsert", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		c.log.Warn("marketflow: build upsert request failed", zap.Error(err))
		return uuid.Nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.log.Warn("marketflow: upsert contact request failed", zap.Error(err))
		return uuid.Nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		c.log.Warn("marketflow: upsert contact unexpected status", zap.Int("status", resp.StatusCode))
		return uuid.Nil
	}

	var result upsertContactResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		c.log.Warn("marketflow: decode upsert response failed", zap.Error(err))
		return uuid.Nil
	}
	id, err := uuid.Parse(result.ID)
	if err != nil {
		return uuid.Nil
	}
	return id
}

// splitName splits "First Last" into (firstName, lastName); a single word is the first name.
func splitName(fullName string) (string, string) {
	for i, ch := range fullName {
		if ch == ' ' {
			return fullName[:i], fullName[i+1:]
		}
	}
	return fullName, ""
}
