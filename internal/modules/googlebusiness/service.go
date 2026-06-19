package googlebusiness

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/googlebusinessconnection"
	"github.com/bengobox/ordering-backend/internal/ent/serviceconfig"
)

// googleReviewURLConfigKey mirrors the service-config key read by the public
// "Review us on Google" deep-link endpoint (handlers/config/service_config_handler.go).
const googleReviewURLConfigKey = "google_review_url"

// stateTTL bounds how long a signed OAuth state remains valid.
const stateTTL = 15 * time.Minute

// Status is the connection status surfaced to the admin UI.
type StatusResult struct {
	Configured   bool   `json:"configured"`
	Connected    bool   `json:"connected"`
	Status       string `json:"status"`
	LocationName string `json:"location_name"`
	PlaceID      string `json:"place_id"`
}

// Service orchestrates the GBP connection lifecycle for a tenant.
type Service struct {
	client      *ent.Client
	oauth       OAuthConfig
	logger      *zap.Logger
	frontendURL string       // where the callback redirects after storing tokens
	keys        *KeyProvider // resolves AES keys (DB-first) for token encryption at rest
}

// NewService constructs the GBP service. It never fails: when OAuth env is unset the
// service simply reports IsConfigured()==false and every method returns ErrNotConfigured.
//
// keys carries the credential-encryption KeyProvider. When nil, a provider bound to the
// same ent client is created so token encrypt/decrypt still works.
func NewService(client *ent.Client, oauth OAuthConfig, frontendURL string, logger *zap.Logger, keys *KeyProvider) *Service {
	if keys == nil {
		keys = NewKeyProvider(client)
	}
	return &Service{
		client:      client,
		oauth:       oauth,
		logger:      logger.Named("googlebusiness"),
		frontendURL: strings.TrimRight(frontendURL, "/"),
		keys:        keys,
	}
}

// Keys exposes the credential-encryption KeyProvider so platform handlers can read
// status/fingerprint and invalidate the cache after a key rotation.
func (s *Service) Keys() *KeyProvider { return s.keys }

// IsConfigured reports whether the OAuth client credentials are present.
func (s *Service) IsConfigured() bool { return s.oauth.IsConfigured() }

// FrontendRedirect returns the post-callback redirect URL for a tenant (slug-aware).
func (s *Service) FrontendRedirect(tenantSlug, status string) string {
	base := s.frontendURL
	if base == "" {
		base = "/"
	}
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	return fmt.Sprintf("%s%sintegration=google&status=%s", base, sep, status)
}

// ---- OAuth state signing ----------------------------------------------------

type statePayload struct {
	TenantID string `json:"tid"`
	Slug     string `json:"slug"`
	OutletID string `json:"oid,omitempty"`
	IssuedAt int64  `json:"iat"`
	Nonce    string `json:"n"`
}

// SignState builds a tamper-proof state string binding the callback to a tenant.
func (s *Service) SignState(tenantID uuid.UUID, slug string, outletID *uuid.UUID) (string, error) {
	p := statePayload{
		TenantID: tenantID.String(),
		Slug:     slug,
		IssuedAt: time.Now().Unix(),
		Nonce:    uuid.NewString(),
	}
	if outletID != nil {
		p.OutletID = outletID.String()
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	body := base64.RawURLEncoding.EncodeToString(raw)
	sig := s.signBytes([]byte(body))
	return body + "." + sig, nil
}

// VerifyState validates the signature + TTL and returns the bound tenant payload.
func (s *Service) VerifyState(state string) (statePayload, error) {
	var p statePayload
	parts := strings.SplitN(state, ".", 2)
	if len(parts) != 2 {
		return p, errors.New("malformed state")
	}
	expected := s.signBytes([]byte(parts[0]))
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return p, errors.New("invalid state signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return p, fmt.Errorf("decode state: %w", err)
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return p, fmt.Errorf("unmarshal state: %w", err)
	}
	if time.Since(time.Unix(p.IssuedAt, 0)) > stateTTL {
		return p, errors.New("state expired")
	}
	return p, nil
}

func (s *Service) signBytes(b []byte) string {
	key, _ := s.keys.PrimaryKey(context.Background())
	mac := hmac.New(sha256.New, key)
	mac.Write(b)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// ---- Connection lifecycle ---------------------------------------------------

// Connect returns the Google consent-screen URL for a tenant.
func (s *Service) Connect(tenantID uuid.UUID, slug string, outletID *uuid.UUID) (string, error) {
	if !s.IsConfigured() {
		return "", ErrNotConfigured
	}
	state, err := s.SignState(tenantID, slug, outletID)
	if err != nil {
		return "", err
	}
	return s.oauth.AuthCodeURL(state), nil
}

// HandleCallback exchanges the code, stores encrypted tokens, and best-effort
// auto-discovers the account + first location + placeId.
func (s *Service) HandleCallback(ctx context.Context, state, code string) (statePayload, error) {
	if !s.IsConfigured() {
		return statePayload{}, ErrNotConfigured
	}
	p, err := s.VerifyState(state)
	if err != nil {
		return p, err
	}
	tenantID, err := uuid.Parse(p.TenantID)
	if err != nil {
		return p, fmt.Errorf("invalid tenant in state: %w", err)
	}
	var outletID *uuid.UUID
	if p.OutletID != "" {
		if oid, perr := uuid.Parse(p.OutletID); perr == nil {
			outletID = &oid
		}
	}

	token, err := s.oauth.Exchange(ctx, code)
	if err != nil {
		return p, fmt.Errorf("exchange code: %w", err)
	}

	conn, err := s.upsertTokens(ctx, tenantID, outletID, token)
	if err != nil {
		return p, err
	}

	// Best-effort discovery of account + first location. Failures here do NOT fail the
	// connection — the operator can pick a location later via SelectLocation.
	gclient := NewClient(token.AccessToken)
	account, locations, derr := gclient.ListAccountsLocations(ctx)
	if derr != nil {
		s.logger.Warn("google location discovery failed (non-fatal)", zap.Error(derr))
		_, _ = conn.Update().SetStatus("needs_location").Save(ctx)
		return p, nil
	}

	upd := conn.Update().SetStatus("needs_location")
	if account != "" {
		upd = upd.SetAccountID(account)
	}
	if len(locations) == 1 {
		loc := locations[0]
		fullName := joinLocationName(account, loc.Name)
		upd = upd.SetLocationID(fullName).SetLocationName(loc.Title).SetStatus("connected")
		if loc.Metadata.PlaceID != "" {
			upd = upd.SetPlaceID(loc.Metadata.PlaceID)
		}
		if saved, serr := upd.Save(ctx); serr == nil && loc.Metadata.PlaceID != "" {
			_ = s.writeReviewURL(ctx, tenantID, loc.Metadata.PlaceID)
			_ = saved
		}
		return p, nil
	}
	if _, serr := upd.Save(ctx); serr != nil {
		return p, serr
	}
	return p, nil
}

// GetStatus returns the configured/connected status for a tenant connection.
func (s *Service) GetStatus(ctx context.Context, tenantID uuid.UUID, outletID *uuid.UUID) (StatusResult, error) {
	res := StatusResult{Configured: s.IsConfigured()}
	conn, err := s.findConnection(ctx, tenantID, outletID)
	if err != nil || conn == nil {
		res.Status = "disconnected"
		return res, nil
	}
	res.Connected = conn.Status == "connected"
	res.Status = conn.Status
	res.LocationName = conn.LocationName
	res.PlaceID = conn.PlaceID
	return res, nil
}

// ListLocations returns the locations available on the connected Google account.
func (s *Service) ListLocations(ctx context.Context, tenantID uuid.UUID, outletID *uuid.UUID) ([]Location, error) {
	if !s.IsConfigured() {
		return nil, ErrNotConfigured
	}
	conn, token, err := s.validToken(ctx, tenantID, outletID)
	if err != nil {
		return nil, err
	}
	gclient := NewClient(token.AccessToken)
	account := conn.AccountID
	if account == "" {
		acc, locs, derr := gclient.ListAccountsLocations(ctx)
		if derr != nil {
			return nil, derr
		}
		if acc != "" {
			_, _ = conn.Update().SetAccountID(acc).Save(ctx)
		}
		return locs, nil
	}
	return gclient.ListLocations(ctx, account)
}

// SelectLocation stores the chosen location + placeId and writes the public review URL.
func (s *Service) SelectLocation(ctx context.Context, tenantID uuid.UUID, outletID *uuid.UUID, locationName, placeID, displayName string) error {
	if !s.IsConfigured() {
		return ErrNotConfigured
	}
	conn, err := s.findConnection(ctx, tenantID, outletID)
	if err != nil || conn == nil {
		return errors.New("not connected")
	}
	full := joinLocationName(conn.AccountID, locationName)
	upd := conn.Update().SetLocationID(full).SetStatus("connected")
	if placeID != "" {
		upd = upd.SetPlaceID(placeID)
	}
	if displayName != "" {
		upd = upd.SetLocationName(displayName)
	}
	if _, err := upd.Save(ctx); err != nil {
		return err
	}
	if placeID != "" {
		if err := s.writeReviewURL(ctx, tenantID, placeID); err != nil {
			s.logger.Warn("failed to write google_review_url service config", zap.Error(err))
		}
	}
	return nil
}

// GetReviews returns the location's reviews.
func (s *Service) GetReviews(ctx context.Context, tenantID uuid.UUID, outletID *uuid.UUID) ([]Review, error) {
	if !s.IsConfigured() {
		return nil, ErrNotConfigured
	}
	conn, token, err := s.validToken(ctx, tenantID, outletID)
	if err != nil {
		return nil, err
	}
	if conn.LocationID == "" {
		return nil, errors.New("no location selected")
	}
	return NewClient(token.AccessToken).ListReviews(ctx, conn.LocationID)
}

// Reply posts an owner reply to a review. reviewID may be a bare id or full resource name.
func (s *Service) Reply(ctx context.Context, tenantID uuid.UUID, outletID *uuid.UUID, reviewID, comment string) error {
	if !s.IsConfigured() {
		return ErrNotConfigured
	}
	conn, token, err := s.validToken(ctx, tenantID, outletID)
	if err != nil {
		return err
	}
	reviewName := reviewID
	if !strings.Contains(reviewID, "/") {
		if conn.LocationID == "" {
			return errors.New("no location selected")
		}
		reviewName = fmt.Sprintf("%s/reviews/%s", strings.Trim(conn.LocationID, "/"), reviewID)
	}
	return NewClient(token.AccessToken).ReplyToReview(ctx, reviewName, comment)
}

// Disconnect clears the stored tokens and marks the connection disconnected.
func (s *Service) Disconnect(ctx context.Context, tenantID uuid.UUID, outletID *uuid.UUID) error {
	conn, err := s.findConnection(ctx, tenantID, outletID)
	if err != nil || conn == nil {
		return nil // already disconnected — idempotent
	}
	_, err = conn.Update().
		SetEncryptedTokens("").
		ClearTokenExpiry().
		SetStatus("disconnected").
		Save(ctx)
	return err
}

// ---- internals --------------------------------------------------------------

// validToken loads the connection, decrypts the token, refreshes it if expired
// (persisting the re-encrypted token), and returns both.
func (s *Service) validToken(ctx context.Context, tenantID uuid.UUID, outletID *uuid.UUID) (*ent.GoogleBusinessConnection, *Token, error) {
	conn, err := s.findConnection(ctx, tenantID, outletID)
	if err != nil || conn == nil {
		return nil, nil, errors.New("not connected")
	}
	token, err := s.decodeToken(ctx, conn.EncryptedTokens)
	if err != nil {
		return nil, nil, err
	}
	if token.Valid() {
		return conn, token, nil
	}
	// Refresh transparently.
	refreshed, err := s.oauth.Refresh(ctx, token.RefreshToken)
	if err != nil {
		return nil, nil, fmt.Errorf("refresh token: %w", err)
	}
	saved, err := s.upsertTokens(ctx, tenantID, outletID, refreshed)
	if err != nil {
		return nil, nil, err
	}
	return saved, refreshed, nil
}

func (s *Service) decodeToken(ctx context.Context, encrypted string) (*Token, error) {
	if encrypted == "" {
		return nil, errors.New("no stored token")
	}
	plain, err := s.keys.Decrypt(ctx, encrypted)
	if err != nil {
		return nil, fmt.Errorf("decrypt token: %w", err)
	}
	var t Token
	if err := json.Unmarshal([]byte(plain), &t); err != nil {
		return nil, fmt.Errorf("unmarshal token: %w", err)
	}
	return &t, nil
}

// upsertTokens encrypts and stores the token JSON, creating the connection row if needed.
func (s *Service) upsertTokens(ctx context.Context, tenantID uuid.UUID, outletID *uuid.UUID, token *Token) (*ent.GoogleBusinessConnection, error) {
	raw, err := json.Marshal(token)
	if err != nil {
		return nil, fmt.Errorf("marshal token: %w", err)
	}
	enc, err := s.keys.Encrypt(ctx, string(raw))
	if err != nil {
		return nil, fmt.Errorf("encrypt token: %w", err)
	}

	conn, err := s.findConnection(ctx, tenantID, outletID)
	if err != nil {
		return nil, err
	}
	if conn != nil {
		upd := conn.Update().SetEncryptedTokens(enc)
		if !token.Expiry.IsZero() {
			upd = upd.SetTokenExpiry(token.Expiry)
		}
		return upd.Save(ctx)
	}

	create := s.client.GoogleBusinessConnection.Create().
		SetTenantID(tenantID).
		SetEncryptedTokens(enc).
		SetStatus("needs_location").
		SetConnectedAt(time.Now())
	if outletID != nil {
		create = create.SetOutletID(*outletID)
	}
	if !token.Expiry.IsZero() {
		create = create.SetTokenExpiry(token.Expiry)
	}
	return create.Save(ctx)
}

// findConnection returns the tenant/outlet connection, or nil when none exists.
func (s *Service) findConnection(ctx context.Context, tenantID uuid.UUID, outletID *uuid.UUID) (*ent.GoogleBusinessConnection, error) {
	q := s.client.GoogleBusinessConnection.Query().
		Where(googlebusinessconnection.TenantID(tenantID))
	if outletID != nil {
		q = q.Where(googlebusinessconnection.OutletID(*outletID))
	} else {
		q = q.Where(googlebusinessconnection.OutletIDIsNil())
	}
	conn, err := q.First(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	return conn, err
}

// writeReviewURL upserts the tenant's google_review_url service-config key so Phase 1's
// public deep link auto-populates from the selected placeId.
func (s *Service) writeReviewURL(ctx context.Context, tenantID uuid.UUID, placeID string) error {
	reviewURL := fmt.Sprintf("https://search.google.com/local/writereview?placeid=%s", placeID)

	existing, err := s.client.ServiceConfig.Query().
		Where(
			serviceconfig.ConfigKey(googleReviewURLConfigKey),
			serviceconfig.TenantID(tenantID),
		).
		First(ctx)
	if err == nil {
		_, uerr := existing.Update().SetConfigValue(reviewURL).Save(ctx)
		return uerr
	}
	if !ent.IsNotFound(err) {
		return err
	}
	_, cerr := s.client.ServiceConfig.Create().
		SetTenantID(tenantID).
		SetConfigKey(googleReviewURLConfigKey).
		SetConfigValue(reviewURL).
		SetConfigType("string").
		SetDescription("Auto-generated Google review deep link from connected Business Profile").
		Save(ctx)
	return cerr
}

// joinLocationName combines an account resource name with a (possibly relative)
// location name into the v4 "accounts/*/locations/*" form expected by the reviews API.
func joinLocationName(account, location string) string {
	location = strings.Trim(location, "/")
	if location == "" {
		return ""
	}
	if strings.HasPrefix(location, "accounts/") {
		return location
	}
	account = strings.Trim(account, "/")
	if account == "" || !strings.HasPrefix(location, "locations/") {
		return location
	}
	return account + "/" + location
}
