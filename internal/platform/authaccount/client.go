// Package authaccount is ordering's S2S client for creating a customer account in auth-api from
// a guest order, and for reading a user's verified phone. auth-api owns accounts: ordering only
// passes the guest's own details through and keeps nothing of the password.
package authaccount

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"
)

// Errors the guest account flow maps to answers for the storefront.
var (
	// ErrAccountExists: an account already holds the email.
	ErrAccountExists = errors.New("authaccount: account exists")
	// ErrEmailNotVerified: auth-api wants the email proven by a code first.
	ErrEmailNotVerified = errors.New("authaccount: email not verified")
	// ErrCodeInvalid: the email code is wrong.
	ErrCodeInvalid = errors.New("authaccount: code invalid")
	// ErrCodeExpired: the email code expired or was never sent.
	ErrCodeExpired = errors.New("authaccount: code expired")
	// ErrTooManyAttempts: too many codes or wrong codes for this email.
	ErrTooManyAttempts = errors.New("authaccount: too many attempts")
	// ErrWeakPassword: the password fails auth-api's password policy.
	ErrWeakPassword = errors.New("authaccount: weak password")
	// ErrNotAllowed: the tenant does not let customers sign up through this app.
	ErrNotAllowed = errors.New("authaccount: sign-up not allowed")
	// ErrNotConfigured: no auth-api URL.
	ErrNotConfigured = errors.New("authaccount: not configured")
)

// RegisterInput is a customer account made from a guest order. Name and Phone come from the
// order, never from the request.
type RegisterInput struct {
	Email      string
	Password   string
	TenantSlug string
	Name       string
	Phone      string
}

// Client calls auth-api.
type Client struct {
	baseURL    string
	apiKey     string
	clientID   string
	httpClient *http.Client
	log        *zap.Logger
}

// NewClient builds the client. baseURL is AUTH_SERVICE_URL, apiKey the shared
// INTERNAL_SERVICE_KEY and clientID the storefront's SSO client (its self sign-up role makes the
// new account a customer of the tenant).
func NewClient(baseURL, apiKey, clientID string, log *zap.Logger) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		clientID:   clientID,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		log:        log.Named("authaccount-client"),
	}
}

// authError is auth-api's error body.
type authError struct {
	Code    string `json:"code"`
	Message string `json:"error"`
}

// SendSignupCode asks auth-api to email a sign-up code to email. ErrAccountExists when a
// verified account already holds it.
func (c *Client) SendSignupCode(ctx context.Context, email, tenantSlug string) error {
	status, body, err := c.post(ctx, "/api/v1/auth/email/send-code", map[string]any{
		"email":       email,
		"tenant_slug": tenantSlug,
	})
	if err != nil {
		return err
	}
	if status == http.StatusOK {
		return nil
	}
	return c.mapError("send-code", status, body)
}

// VerifySignupCode proves the email with the code the guest received.
func (c *Client) VerifySignupCode(ctx context.Context, email, code string) error {
	status, body, err := c.post(ctx, "/api/v1/auth/email/verify-code", map[string]any{
		"email": email,
		"code":  code,
	})
	if err != nil {
		return err
	}
	if status == http.StatusOK {
		return nil
	}
	return c.mapError("verify-code", status, body)
}

// Register creates the customer account. ErrEmailNotVerified when the email has no recent
// verified code, ErrAccountExists when the email is taken.
func (c *Client) Register(ctx context.Context, in RegisterInput) error {
	profile := map[string]any{}
	if in.Name != "" {
		profile["name"] = in.Name
	}
	if in.Phone != "" {
		profile["phone"] = in.Phone
	}
	status, body, err := c.post(ctx, "/api/v1/auth/register", map[string]any{
		"email":       in.Email,
		"password":    in.Password,
		"tenant_slug": in.TenantSlug,
		"client_id":   c.clientID,
		"service_id":  "ordering",
		"profile":     profile,
	})
	if err != nil {
		return err
	}
	if status == http.StatusCreated || status == http.StatusOK {
		return nil
	}
	return c.mapError("register", status, body)
}

// VerifiedPhone returns the user's verified phone, or "" when there is none or auth-api cannot
// be reached.
func (c *Client) VerifiedPhone(ctx context.Context, userID string) string {
	if c.baseURL == "" || c.apiKey == "" || userID == "" {
		return ""
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/api/v1/s2s/users/"+url.PathEscape(userID)+"/phone", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("X-API-Key", c.apiKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.log.Warn("verified phone lookup failed", zap.Error(err))
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var out struct {
		Phone         string `json:"phone"`
		PhoneVerified bool   `json:"phone_verified"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&out); err != nil || !out.PhoneVerified {
		return ""
	}
	return out.Phone
}

// post sends a JSON body. The body may hold a password, so it is never logged.
func (c *Client) post(ctx context.Context, path string, payload map[string]any) (int, []byte, error) {
	if c.baseURL == "" {
		return 0, nil, ErrNotConfigured
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, fmt.Errorf("authaccount: encode: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return 0, nil, fmt.Errorf("authaccount: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("authaccount: %s request failed", path)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	return resp.StatusCode, body, nil
}

func (c *Client) mapError(op string, status int, body []byte) error {
	var e authError
	_ = json.Unmarshal(body, &e)
	switch {
	case e.Code == "email_exists" || status == http.StatusConflict:
		return ErrAccountExists
	case e.Code == "email_not_verified":
		return ErrEmailNotVerified
	case e.Code == "code_invalid":
		return ErrCodeInvalid
	case e.Code == "code_expired":
		return ErrCodeExpired
	case e.Code == "code_locked" || e.Code == "rate_limited" || status == http.StatusTooManyRequests:
		return ErrTooManyAttempts
	case e.Code == "weak_password":
		return ErrWeakPassword
	case e.Code == "tenant_membership_required" || e.Code == "role_not_allowed":
		return ErrNotAllowed
	}
	// Only the status and auth-api's code: the request held the password.
	c.log.Warn("auth-api refused guest account step",
		zap.String("op", op), zap.Int("status", status), zap.String("code", e.Code))
	return fmt.Errorf("authaccount: %s answered %d", op, status)
}
