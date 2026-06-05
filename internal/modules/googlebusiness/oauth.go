package googlebusiness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrNotConfigured is returned by every entry point when the OAuth env vars are unset.
// Handlers translate this into an HTTP 503 "Google integration not configured".
var ErrNotConfigured = errors.New("google integration not configured")

// Google OAuth + token endpoints (Google Identity OAuth 2.0).
//   - https://developers.google.com/identity/protocols/oauth2/web-server
const (
	googleAuthEndpoint  = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenEndpoint = "https://oauth2.googleapis.com/token"

	// businessManageScope grants read/manage of the authenticated user's Business Profiles.
	// https://developers.google.com/my-business/content/basic-setup
	businessManageScope = "https://www.googleapis.com/auth/business.manage"
)

// OAuthConfig holds the operator-provided OAuth client credentials.
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// NewOAuthConfig builds an OAuthConfig from explicit values (sourced from app config/env).
func NewOAuthConfig(clientID, clientSecret, redirectURL string) OAuthConfig {
	return OAuthConfig{
		ClientID:     strings.TrimSpace(clientID),
		ClientSecret: strings.TrimSpace(clientSecret),
		RedirectURL:  strings.TrimSpace(redirectURL),
	}
}

// IsConfigured reports whether all three OAuth env values are present.
// When false the whole integration stays inert and endpoints return 503.
func (c OAuthConfig) IsConfigured() bool {
	return c.ClientID != "" && c.ClientSecret != "" && c.RedirectURL != ""
}

// AuthCodeURL builds the Google consent-screen URL. state binds the callback to a tenant.
// access_type=offline + prompt=consent ensures a refresh_token is returned.
func (c OAuthConfig) AuthCodeURL(state string) string {
	q := url.Values{}
	q.Set("client_id", c.ClientID)
	q.Set("redirect_uri", c.RedirectURL)
	q.Set("response_type", "code")
	q.Set("scope", businessManageScope)
	q.Set("access_type", "offline")
	q.Set("include_granted_scopes", "true")
	q.Set("prompt", "consent")
	q.Set("state", state)
	return googleAuthEndpoint + "?" + q.Encode()
}

// Token is the stored OAuth token payload. It is JSON-marshalled and then AES-256-GCM
// encrypted into GoogleBusinessConnection.encrypted_tokens.
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry"`
	Scope        string    `json:"scope"`
}

// Valid reports whether the access token is present and not (about to be) expired.
func (t *Token) Valid() bool {
	if t == nil || t.AccessToken == "" {
		return false
	}
	if t.Expiry.IsZero() {
		return true
	}
	// 1-minute leeway to avoid using a token that expires mid-request.
	return time.Now().Add(time.Minute).Before(t.Expiry)
}

// tokenResponse mirrors Google's token endpoint JSON.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

var oauthHTTPClient = &http.Client{Timeout: 30 * time.Second}

// Exchange swaps an authorization code for an access+refresh token.
func (c OAuthConfig) Exchange(ctx context.Context, code string) (*Token, error) {
	if !c.IsConfigured() {
		return nil, ErrNotConfigured
	}
	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", c.ClientID)
	form.Set("client_secret", c.ClientSecret)
	form.Set("redirect_uri", c.RedirectURL)
	form.Set("grant_type", "authorization_code")

	return c.doTokenRequest(ctx, form, "")
}

// Refresh obtains a fresh access token from a refresh token. Google does not return a
// new refresh_token on refresh, so the caller must retain the previous one.
func (c OAuthConfig) Refresh(ctx context.Context, refreshToken string) (*Token, error) {
	if !c.IsConfigured() {
		return nil, ErrNotConfigured
	}
	if refreshToken == "" {
		return nil, errors.New("no refresh token available")
	}
	form := url.Values{}
	form.Set("client_id", c.ClientID)
	form.Set("client_secret", c.ClientSecret)
	form.Set("refresh_token", refreshToken)
	form.Set("grant_type", "refresh_token")

	return c.doTokenRequest(ctx, form, refreshToken)
}

// doTokenRequest performs the POST to Google's token endpoint and maps the result.
// fallbackRefresh is preserved into the returned Token when Google omits refresh_token.
func (c OAuthConfig) doTokenRequest(ctx context.Context, form url.Values, fallbackRefresh string) (*Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || tr.Error != "" {
		msg := tr.Error
		if tr.ErrorDesc != "" {
			msg = tr.Error + ": " + tr.ErrorDesc
		}
		if msg == "" {
			msg = resp.Status
		}
		return nil, fmt.Errorf("google token endpoint: %s", msg)
	}

	refresh := tr.RefreshToken
	if refresh == "" {
		refresh = fallbackRefresh
	}

	expiry := time.Time{}
	if tr.ExpiresIn > 0 {
		expiry = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	}

	return &Token{
		AccessToken:  tr.AccessToken,
		RefreshToken: refresh,
		Expiry:       expiry,
		Scope:        tr.Scope,
	}, nil
}
