package googlebusiness

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/serviceconfig"
)

// ReviewURLConfigKey is the tenant service-config key holding the "Review us on Google"
// link. The public review-url endpoint, the order pages and the order-completed message
// all read this one key, whether the business pasted the link or a connected Google
// location wrote it.
const ReviewURLConfigKey = "google_review_url"

// writeReviewBase is Google's standard write-a-review link for a Place ID.
const writeReviewBase = "https://search.google.com/local/writereview"

// ErrInvalidReviewLink is returned when a pasted value is neither a Place ID nor a Google link.
var ErrInvalidReviewLink = errors.New("enter a Google review link or a Place ID")

// placeIDPattern matches a Google Place ID. Real ones are 20 to 250 characters of
// letters, digits, underscores and dashes (most start with "ChIJ").
var placeIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{20,250}$`)

// googleReviewHosts are the hosts a pasted review link may point at: Google Search and
// Maps, and the short links Google hands out from a Business Profile ("Ask for reviews").
var googleReviewHosts = map[string]bool{
	"search.google.com": true,
	"www.google.com":    true,
	"google.com":        true,
	"maps.google.com":   true,
	"g.page":            true,
	"maps.app.goo.gl":   true,
	"goo.gl":            true,
	"g.co":              true,
}

// WriteReviewURL builds the write-a-review link for a Place ID.
func WriteReviewURL(placeID string) string {
	return writeReviewBase + "?placeid=" + url.QueryEscape(placeID)
}

// NormalizeReviewLink turns what a business pastes (a Place ID, or a review link copied
// from its Google Business Profile) into the link customers open. It returns the link and
// the Place ID when one is known. Only https links on Google hosts are accepted, so the
// button can never send customers somewhere else.
func NormalizeReviewLink(input string) (reviewURL, placeID string, err error) {
	in := strings.TrimSpace(input)
	if in == "" {
		return "", "", ErrInvalidReviewLink
	}
	if !strings.Contains(in, "/") && !strings.Contains(in, ".") {
		if placeIDPattern.MatchString(in) {
			return WriteReviewURL(in), in, nil
		}
		return "", "", ErrInvalidReviewLink
	}
	if !strings.Contains(in, "://") {
		in = "https://" + in
	}
	u, perr := url.Parse(in)
	if perr != nil || u.Host == "" {
		return "", "", ErrInvalidReviewLink
	}
	host := strings.ToLower(u.Hostname())
	if !googleReviewHosts[host] && !strings.HasSuffix(host, ".google.com") {
		return "", "", ErrInvalidReviewLink
	}
	if u.Scheme == "http" {
		u.Scheme = "https"
	}
	if u.Scheme != "https" || u.User != nil {
		return "", "", ErrInvalidReviewLink
	}
	// A link that already names a Place ID is rewritten to the standard form.
	q := u.Query()
	for _, k := range []string{"placeid", "place_id"} {
		if v := strings.TrimSpace(q.Get(k)); placeIDPattern.MatchString(v) {
			return WriteReviewURL(v), v, nil
		}
	}
	return u.String(), "", nil
}

// PlaceIDFromReviewURL returns the Place ID carried by a standard write-a-review link.
func PlaceIDFromReviewURL(reviewURL string) string {
	u, err := url.Parse(reviewURL)
	if err != nil {
		return ""
	}
	v := u.Query().Get("placeid")
	if placeIDPattern.MatchString(v) {
		return v
	}
	return ""
}

// LookupReviewURL reads a tenant's review link. It returns "" when the tenant has none.
// There is deliberately no platform-wide default: a shared link would send every
// tenant's customers to one business's Google profile.
func LookupReviewURL(ctx context.Context, client *ent.Client, tenantID uuid.UUID) string {
	if client == nil || tenantID == uuid.Nil {
		return ""
	}
	cfg, err := client.ServiceConfig.Query().
		Where(
			serviceconfig.ConfigKey(ReviewURLConfigKey),
			serviceconfig.TenantID(tenantID),
		).
		First(ctx)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cfg.ConfigValue)
}

// saveReviewURL writes (or, with an empty value, removes) the tenant's review link.
func saveReviewURL(ctx context.Context, client *ent.Client, tenantID uuid.UUID, reviewURL, description string) error {
	existing, err := client.ServiceConfig.Query().
		Where(
			serviceconfig.ConfigKey(ReviewURLConfigKey),
			serviceconfig.TenantID(tenantID),
		).
		First(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return err
	}
	if reviewURL == "" {
		if existing == nil {
			return nil
		}
		return client.ServiceConfig.DeleteOne(existing).Exec(ctx)
	}
	if existing != nil {
		_, uerr := existing.Update().SetConfigValue(reviewURL).SetDescription(description).Save(ctx)
		return uerr
	}
	_, cerr := client.ServiceConfig.Create().
		SetTenantID(tenantID).
		SetConfigKey(ReviewURLConfigKey).
		SetConfigValue(reviewURL).
		SetConfigType("string").
		SetDescription(description).
		Save(ctx)
	if cerr != nil {
		return fmt.Errorf("save review link: %w", cerr)
	}
	return nil
}

// Review link descriptions, stored with the config row so the source shows in the
// generic settings list and in the status response.
const (
	reviewURLFromGoogle = "Google review link from the connected Business Profile location"
	reviewURLPasted     = "Google review link entered by the business"
)

// ReviewLink is the tenant's current review link and where it came from.
type ReviewLink struct {
	ReviewURL string `json:"review_url"`
	PlaceID   string `json:"place_id"`
	// Source is "google" when a connected location wrote it, "manual" when pasted, "" when unset.
	Source string `json:"source"`
}

// GetReviewLink returns the tenant's review link.
func (s *Service) GetReviewLink(ctx context.Context, tenantID uuid.UUID) ReviewLink {
	cfg, err := s.client.ServiceConfig.Query().
		Where(
			serviceconfig.ConfigKey(ReviewURLConfigKey),
			serviceconfig.TenantID(tenantID),
		).
		First(ctx)
	if err != nil || strings.TrimSpace(cfg.ConfigValue) == "" {
		return ReviewLink{}
	}
	link := ReviewLink{ReviewURL: strings.TrimSpace(cfg.ConfigValue), Source: "manual"}
	if cfg.Description == reviewURLFromGoogle {
		link.Source = "google"
	}
	link.PlaceID = PlaceIDFromReviewURL(link.ReviewURL)
	return link
}

// SetReviewLink stores a pasted review link or Place ID. It works without the OAuth
// connection, so a business can collect reviews before Google approves API access.
func (s *Service) SetReviewLink(ctx context.Context, tenantID uuid.UUID, input string) (ReviewLink, error) {
	reviewURL, placeID, err := NormalizeReviewLink(input)
	if err != nil {
		return ReviewLink{}, err
	}
	if err := saveReviewURL(ctx, s.client, tenantID, reviewURL, reviewURLPasted); err != nil {
		return ReviewLink{}, err
	}
	return ReviewLink{ReviewURL: reviewURL, PlaceID: placeID, Source: "manual"}, nil
}

// ClearReviewLink removes the tenant's review link, which hides the customer button.
func (s *Service) ClearReviewLink(ctx context.Context, tenantID uuid.UUID) error {
	return saveReviewURL(ctx, s.client, tenantID, "", "")
}
