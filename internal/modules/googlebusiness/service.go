package googlebusiness

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/googlebusinessconnection"
)

// stateTTL bounds how long a signed OAuth state remains valid.
const stateTTL = 15 * time.Minute

// Errors the handlers turn into a 409 with a plain message for the staff page.
var (
	ErrNotConnected = errors.New("google business profile is not connected")
	ErrNoLocation   = errors.New("choose a google business location first")
	// ErrReconnect means Google refused the stored refresh token (revoked access or a
	// changed password), so the business has to connect again.
	ErrReconnect = errors.New("google access expired, connect again")
	// ErrForeignReview is returned when a reply names a review outside the chosen location.
	ErrForeignReview = errors.New("review does not belong to the chosen location")
)

// Connection statuses stored on GoogleBusinessConnection.status.
const (
	statusDisconnected  = "disconnected"
	statusNeedsLocation = "needs_location"
	statusConnected     = "connected"
	statusReconnect     = "reconnect"
)

// StatusResult is the connection status surfaced to the staff integrations page.
type StatusResult struct {
	// Configured is true when the platform has set the Google OAuth client.
	Configured bool `json:"configured"`
	// Connected is true when a location is chosen and reviews can be read.
	Connected bool `json:"connected"`
	// Linked is true when Google tokens are stored, with or without a chosen location.
	Linked bool `json:"linked"`
	// Status is disconnected, needs_location, connected or reconnect.
	Status       string `json:"status"`
	LocationID   string `json:"location_id"`
	LocationName string `json:"location_name"`
	PlaceID      string `json:"place_id"`
	// ReviewLink is the link customers get, set by hand or by the connected location.
	ReviewLink ReviewLink `json:"review_link"`
}

// Service orchestrates the GBP connection lifecycle for a tenant.
type Service struct {
	client      *ent.Client
	oauth       OAuthConfig
	logger      *zap.Logger
	frontendURL string       // where the callback sends the browser; {tenant} becomes the slug
	keys        *KeyProvider // resolves AES keys (DB-first) for token encryption at rest
}

// NewService constructs the GBP service. It never fails: when OAuth env is unset the
// service simply reports IsConfigured()==false and the OAuth methods return ErrNotConfigured.
// The pasted review link works either way.
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

// slugPattern limits the slug placed into the post-callback redirect, so a tampered
// state can never turn the redirect into a link to another site.
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// FrontendRedirect returns where the browser goes after the OAuth callback. A
// "{tenant}" placeholder in the configured URL is replaced with the tenant slug, so one
// setting serves every tenant's staff integrations page.
func (s *Service) FrontendRedirect(tenantSlug, status string) string {
	base := s.frontendURL
	if base == "" {
		base = "/"
	}
	slug := strings.ToLower(strings.TrimSpace(tenantSlug))
	if !slugPattern.MatchString(slug) {
		slug = ""
	}
	if strings.Contains(base, "{tenant}") {
		if slug == "" {
			// Without a slug the tenant page is unknown; land on the app root.
			if u, err := url.Parse(strings.ReplaceAll(base, "{tenant}", "x")); err == nil && u.Host != "" {
				base = u.Scheme + "://" + u.Host + "/"
			} else {
				base = "/"
			}
		} else {
			base = strings.ReplaceAll(base, "{tenant}", slug)
		}
	}
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	return fmt.Sprintf("%s%sintegration=google&status=%s", base, sep, url.QueryEscape(status))
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

// StateSlug reads the tenant slug from a state string WITHOUT trusting it. It is used
// only to pick the redirect page when the callback fails before the state is verified;
// FrontendRedirect still restricts it to a plain slug.
func StateSlug(state string) string {
	body, _, ok := strings.Cut(state, ".")
	if !ok {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return ""
	}
	var p statePayload
	if json.Unmarshal(raw, &p) != nil {
		return ""
	}
	return p.Slug
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
//
// The tenant comes only from the signed state. When the callback arrives on a tenant URL,
// expectedTenant is that tenant and must match the state, so a consent started for one
// tenant can never store tokens on another.
func (s *Service) HandleCallback(ctx context.Context, state, code string, expectedTenant *uuid.UUID) (statePayload, error) {
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
	if expectedTenant != nil && *expectedTenant != uuid.Nil && *expectedTenant != tenantID {
		return p, errors.New("state belongs to another tenant")
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
	// connection; the business can pick a location afterwards.
	gclient := NewClient(token.AccessToken)
	account, locations, derr := gclient.ListAccountsLocations(ctx)
	if derr != nil {
		s.logger.Warn("google location discovery failed (non-fatal)", zap.Error(derr))
		_, _ = conn.Update().SetStatus(keptStatus(conn)).Save(ctx)
		return p, nil
	}

	upd := conn.Update().SetStatus(keptStatus(conn))
	if account != "" {
		upd = upd.SetAccountID(account)
	}
	if len(locations) == 1 {
		loc := locations[0]
		upd = upd.SetLocationID(joinLocationName(account, loc.Name)).
			SetLocationName(loc.Title).
			SetPlaceID(loc.Metadata.PlaceID).
			SetStatus(statusConnected)
		if _, serr := upd.Save(ctx); serr != nil {
			return p, serr
		}
		s.storeLocationReviewURL(ctx, tenantID, loc.Metadata.PlaceID, loc.Metadata.NewReviewURI)
		return p, nil
	}
	if _, serr := upd.Save(ctx); serr != nil {
		return p, serr
	}
	return p, nil
}

// keptStatus is the status after a (re)connect: a connection that already had a location
// stays connected to it, otherwise the business still has to pick one.
func keptStatus(conn *ent.GoogleBusinessConnection) string {
	if conn.LocationID != "" {
		return statusConnected
	}
	return statusNeedsLocation
}

// GetStatus returns the configured/connected status and the review link for a tenant.
func (s *Service) GetStatus(ctx context.Context, tenantID uuid.UUID, outletID *uuid.UUID) (StatusResult, error) {
	res := StatusResult{Configured: s.IsConfigured(), Status: statusDisconnected}
	res.ReviewLink = s.GetReviewLink(ctx, tenantID)
	conn, err := s.findConnection(ctx, tenantID, outletID)
	if err != nil {
		return res, err
	}
	if conn == nil {
		return res, nil
	}
	res.Status = conn.Status
	res.Linked = conn.EncryptedTokens != "" && conn.Status != statusDisconnected
	res.Connected = res.Linked && conn.Status == statusConnected && conn.LocationID != ""
	res.LocationID = conn.LocationID
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

// SelectLocation stores the chosen location and writes the customer review link from
// its Place ID (or Google's own new-review link when the location has no Place ID).
func (s *Service) SelectLocation(ctx context.Context, tenantID uuid.UUID, outletID *uuid.UUID, locationName, placeID, displayName, newReviewURI string) error {
	if !s.IsConfigured() {
		return ErrNotConfigured
	}
	conn, err := s.findConnection(ctx, tenantID, outletID)
	if err != nil {
		return err
	}
	if conn == nil || conn.EncryptedTokens == "" || conn.Status == statusDisconnected {
		return ErrNotConnected
	}
	full := joinLocationName(conn.AccountID, locationName)
	if full == "" {
		return ErrNoLocation
	}
	upd := conn.Update().
		SetLocationID(full).
		SetStatus(statusConnected).
		SetPlaceID(strings.TrimSpace(placeID)).
		SetLocationName(strings.TrimSpace(displayName))
	if _, err := upd.Save(ctx); err != nil {
		return err
	}
	s.storeLocationReviewURL(ctx, tenantID, strings.TrimSpace(placeID), newReviewURI)
	return nil
}

// storeLocationReviewURL writes the review link for a connected location. Failures are
// logged, not returned: the connection itself is already saved.
func (s *Service) storeLocationReviewURL(ctx context.Context, tenantID uuid.UUID, placeID, newReviewURI string) {
	reviewURL := ""
	if placeID != "" && placeIDPattern.MatchString(placeID) {
		reviewURL = WriteReviewURL(placeID)
	} else if newReviewURI != "" {
		if u, _, err := NormalizeReviewLink(newReviewURI); err == nil {
			reviewURL = u
		}
	}
	if reviewURL == "" {
		return
	}
	if err := saveReviewURL(ctx, s.client, tenantID, reviewURL, reviewURLFromGoogle); err != nil {
		s.logger.Warn("failed to save google review link", zap.Error(err))
	}
}

// GetReviews returns one page of the chosen location's reviews.
func (s *Service) GetReviews(ctx context.Context, tenantID uuid.UUID, outletID *uuid.UUID, pageToken string) (ReviewPage, error) {
	if !s.IsConfigured() {
		return ReviewPage{}, ErrNotConfigured
	}
	conn, token, err := s.validToken(ctx, tenantID, outletID)
	if err != nil {
		return ReviewPage{}, err
	}
	if conn.LocationID == "" {
		return ReviewPage{}, ErrNoLocation
	}
	return NewClient(token.AccessToken).ListReviews(ctx, conn.LocationID, pageToken)
}

// Reply posts an owner reply to a review. reviewID may be a bare id or a full resource
// name, but it must belong to the location this tenant chose.
func (s *Service) Reply(ctx context.Context, tenantID uuid.UUID, outletID *uuid.UUID, reviewID, comment string) error {
	if !s.IsConfigured() {
		return ErrNotConfigured
	}
	conn, token, err := s.validToken(ctx, tenantID, outletID)
	if err != nil {
		return err
	}
	reviewName, err := reviewResourceName(conn.LocationID, reviewID)
	if err != nil {
		return err
	}
	return NewClient(token.AccessToken).ReplyToReview(ctx, reviewName, comment)
}

// reviewResourceName builds the full review name under the chosen location and refuses
// any name that points at a different location.
func reviewResourceName(locationID, reviewID string) (string, error) {
	loc := strings.Trim(locationID, "/")
	if loc == "" {
		return "", ErrNoLocation
	}
	id := strings.Trim(reviewID, "/")
	if id == "" {
		return "", ErrForeignReview
	}
	if !strings.Contains(id, "/") {
		return loc + "/reviews/" + id, nil
	}
	prefix := loc + "/reviews/"
	if !strings.HasPrefix(id, prefix) || strings.Contains(strings.TrimPrefix(id, prefix), "/") {
		return "", ErrForeignReview
	}
	return id, nil
}

// Disconnect clears the stored tokens and the chosen location. The review link stays,
// so customers keep getting the button after a disconnect.
func (s *Service) Disconnect(ctx context.Context, tenantID uuid.UUID, outletID *uuid.UUID) error {
	conn, err := s.findConnection(ctx, tenantID, outletID)
	if err != nil || conn == nil {
		return err // nil when already disconnected
	}
	_, err = conn.Update().
		SetEncryptedTokens("").
		ClearTokenExpiry().
		SetAccountID("").
		SetLocationID("").
		SetLocationName("").
		SetPlaceID("").
		SetStatus(statusDisconnected).
		Save(ctx)
	return err
}

// ---- internals --------------------------------------------------------------

// validToken loads the connection, decrypts the token, refreshes it if expired
// (persisting the re-encrypted token), and returns both.
func (s *Service) validToken(ctx context.Context, tenantID uuid.UUID, outletID *uuid.UUID) (*ent.GoogleBusinessConnection, *Token, error) {
	conn, err := s.findConnection(ctx, tenantID, outletID)
	if err != nil {
		return nil, nil, err
	}
	if conn == nil || conn.EncryptedTokens == "" || conn.Status == statusDisconnected {
		return nil, nil, ErrNotConnected
	}
	if conn.Status == statusReconnect {
		return nil, nil, ErrReconnect
	}
	token, err := s.decodeToken(ctx, conn.EncryptedTokens)
	if err != nil {
		return nil, nil, err
	}
	if token.Valid() {
		return conn, token, nil
	}
	if token.RefreshToken == "" {
		_, _ = conn.Update().SetStatus(statusReconnect).Save(ctx)
		return nil, nil, ErrReconnect
	}
	refreshed, err := s.oauth.Refresh(ctx, token.RefreshToken)
	if err != nil {
		if errors.Is(err, errInvalidGrant) {
			// Google will not refresh this token again; ask the business to reconnect.
			_, _ = conn.Update().SetStatus(statusReconnect).Save(ctx)
			return nil, nil, ErrReconnect
		}
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
		return nil, ErrNotConnected
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
		if conn.Status == statusDisconnected || conn.Status == statusReconnect {
			// A fresh consent; the callback settles the final status after discovery.
			upd = upd.SetStatus(keptStatus(conn)).SetConnectedAt(time.Now())
		}
		if !token.Expiry.IsZero() {
			upd = upd.SetTokenExpiry(token.Expiry)
		}
		return upd.Save(ctx)
	}

	create := s.client.GoogleBusinessConnection.Create().
		SetTenantID(tenantID).
		SetEncryptedTokens(enc).
		SetStatus(statusNeedsLocation).
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
