package googlebusiness

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const testPlaceID = "ChIJN1t_tDeuEmsRUsoyG83frY4"

func TestNormalizeReviewLink(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		wantURL   string
		wantPlace string
		wantErr   bool
	}{
		{name: "bare place id", in: "  " + testPlaceID + " ", wantURL: WriteReviewURL(testPlaceID), wantPlace: testPlaceID},
		{name: "writereview link", in: "https://search.google.com/local/writereview?placeid=" + testPlaceID, wantURL: WriteReviewURL(testPlaceID), wantPlace: testPlaceID},
		{name: "maps link with place_id", in: "https://www.google.com/maps/place/?q=place_id:x&place_id=" + testPlaceID, wantURL: WriteReviewURL(testPlaceID), wantPlace: testPlaceID},
		{name: "g.page short link kept", in: "https://g.page/r/CabcDEF123/review", wantURL: "https://g.page/r/CabcDEF123/review"},
		{name: "maps.app.goo.gl link kept", in: "https://maps.app.goo.gl/AbCdEf12345", wantURL: "https://maps.app.goo.gl/AbCdEf12345"},
		{name: "scheme added", in: "g.page/r/CabcDEF123/review", wantURL: "https://g.page/r/CabcDEF123/review"},
		{name: "http upgraded", in: "http://g.page/r/CabcDEF123/review", wantURL: "https://g.page/r/CabcDEF123/review"},
		{name: "other site refused", in: "https://evil.example.com/review", wantErr: true},
		{name: "lookalike host refused", in: "https://google.com.evil.example/review", wantErr: true},
		{name: "javascript refused", in: "javascript:alert(1)", wantErr: true},
		{name: "credentials refused", in: "https://user:pw@g.page/r/x", wantErr: true},
		{name: "short text refused", in: "hello", wantErr: true},
		{name: "empty refused", in: "   ", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotURL, gotPlace, err := NormalizeReviewLink(tc.in)
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidReviewLink) {
					t.Fatalf("want ErrInvalidReviewLink, got url=%q err=%v", gotURL, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotURL != tc.wantURL || gotPlace != tc.wantPlace {
				t.Fatalf("got (%q, %q), want (%q, %q)", gotURL, gotPlace, tc.wantURL, tc.wantPlace)
			}
		})
	}
}

func TestWriteReviewURLAndPlaceIDRoundTrip(t *testing.T) {
	u := WriteReviewURL(testPlaceID)
	if u != "https://search.google.com/local/writereview?placeid="+testPlaceID {
		t.Fatalf("unexpected review url %q", u)
	}
	if got := PlaceIDFromReviewURL(u); got != testPlaceID {
		t.Fatalf("place id = %q", got)
	}
	if got := PlaceIDFromReviewURL("https://g.page/r/abc/review"); got != "" {
		t.Fatalf("expected no place id, got %q", got)
	}
}

func TestFrontendRedirect(t *testing.T) {
	svc := NewService(nil, OAuthConfig{}, "https://ordering.example.com/{tenant}/dashboard/staff/integrations", zap.NewNop(), nil)
	got := svc.FrontendRedirect("codevertex-demo", "connected")
	want := "https://ordering.example.com/codevertex-demo/dashboard/staff/integrations?integration=google&status=connected"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	// A slug that is not a plain slug never reaches the URL.
	got = svc.FrontendRedirect("//evil.example.com", "error")
	if got != "https://ordering.example.com/?integration=google&status=error" {
		t.Fatalf("unsafe slug leaked: %q", got)
	}
	// A fixed URL without the placeholder is used as is.
	fixed := NewService(nil, OAuthConfig{}, "https://ordering.example.com/admin?tab=x", zap.NewNop(), nil)
	if got := fixed.FrontendRedirect("demo", "denied"); got != "https://ordering.example.com/admin?tab=x&integration=google&status=denied" {
		t.Fatalf("got %q", got)
	}
}

func TestStateSignVerifyAndSlug(t *testing.T) {
	svc := NewService(nil, OAuthConfig{}, "", zap.NewNop(), nil)
	tid := uuid.New()
	state, err := svc.SignState(tid, "demo", nil)
	if err != nil {
		t.Fatal(err)
	}
	p, err := svc.VerifyState(state)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if p.TenantID != tid.String() || p.Slug != "demo" {
		t.Fatalf("payload %+v", p)
	}
	if StateSlug(state) != "demo" {
		t.Fatalf("StateSlug = %q", StateSlug(state))
	}

	// Swapping the tenant in the body breaks the signature.
	body, sig, _ := strings.Cut(state, ".")
	raw, _ := base64.RawURLEncoding.DecodeString(body)
	var forged statePayload
	_ = json.Unmarshal(raw, &forged)
	forged.TenantID = uuid.NewString()
	fraw, _ := json.Marshal(forged)
	if _, err := svc.VerifyState(base64.RawURLEncoding.EncodeToString(fraw) + "." + sig); err == nil {
		t.Fatal("forged state verified")
	}

	// An expired state is refused.
	old := statePayload{TenantID: tid.String(), Slug: "demo", IssuedAt: time.Now().Add(-time.Hour).Unix(), Nonce: "n"}
	oraw, _ := json.Marshal(old)
	obody := base64.RawURLEncoding.EncodeToString(oraw)
	if _, err := svc.VerifyState(obody + "." + svc.signBytes([]byte(obody))); err == nil {
		t.Fatal("expired state verified")
	}
}

func TestHandleCallbackRefusesOtherTenant(t *testing.T) {
	oauth := NewOAuthConfig("client-id", "client-secret", "https://api.example.com/api/v1/integrations/google/callback")
	svc := NewService(nil, oauth, "", zap.NewNop(), nil)
	stateTenant := uuid.New()
	state, err := svc.SignState(stateTenant, "tenant-a", nil)
	if err != nil {
		t.Fatal(err)
	}
	other := uuid.New()
	// Fails before any call to Google or the database.
	if _, err := svc.HandleCallback(context.Background(), state, "code", &other); err == nil ||
		!strings.Contains(err.Error(), "another tenant") {
		t.Fatalf("want tenant mismatch error, got %v", err)
	}
}

func TestOAuthNotConfigured(t *testing.T) {
	svc := NewService(nil, NewOAuthConfig("id", "", "https://x"), "", zap.NewNop(), nil)
	if svc.IsConfigured() {
		t.Fatal("configured without a secret")
	}
	if _, err := svc.Connect(uuid.New(), "demo", nil); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("want ErrNotConfigured, got %v", err)
	}
}

func TestAuthCodeURLAsksForOfflineAccess(t *testing.T) {
	c := NewOAuthConfig("id", "secret", "https://api.example.com/api/v1/integrations/google/callback")
	u := c.AuthCodeURL("st")
	for _, want := range []string{"access_type=offline", "prompt=consent", "business.manage", "state=st",
		"redirect_uri=https%3A%2F%2Fapi.example.com%2Fapi%2Fv1%2Fintegrations%2Fgoogle%2Fcallback"} {
		if !strings.Contains(u, want) {
			t.Fatalf("auth url %q is missing %q", u, want)
		}
	}
}

func TestReviewResourceName(t *testing.T) {
	loc := "accounts/1/locations/2"
	if got, err := reviewResourceName(loc, "abc"); err != nil || got != loc+"/reviews/abc" {
		t.Fatalf("bare id: %q %v", got, err)
	}
	if got, err := reviewResourceName(loc, loc+"/reviews/abc"); err != nil || got != loc+"/reviews/abc" {
		t.Fatalf("full name: %q %v", got, err)
	}
	for _, bad := range []string{"accounts/9/locations/9/reviews/abc", loc + "/reviews/abc/extra", ""} {
		if _, err := reviewResourceName(loc, bad); !errors.Is(err, ErrForeignReview) {
			t.Fatalf("%q: want ErrForeignReview, got %v", bad, err)
		}
	}
	if _, err := reviewResourceName("", "abc"); !errors.Is(err, ErrNoLocation) {
		t.Fatalf("want ErrNoLocation, got %v", err)
	}
}

func TestTokenEncryptionRoundTrip(t *testing.T) {
	kp := NewKeyProvider(nil)
	enc, err := kp.Encrypt(context.Background(), `{"access_token":"a","refresh_token":"r"}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(enc, "refresh_token") {
		t.Fatal("ciphertext contains plaintext")
	}
	plain, err := kp.Decrypt(context.Background(), enc)
	if err != nil || !strings.Contains(plain, `"refresh_token":"r"`) {
		t.Fatalf("decrypt: %q %v", plain, err)
	}
}

func TestJoinLocationName(t *testing.T) {
	if got := joinLocationName("accounts/1", "locations/2"); got != "accounts/1/locations/2" {
		t.Fatalf("got %q", got)
	}
	if got := joinLocationName("accounts/1", "accounts/3/locations/4"); got != "accounts/3/locations/4" {
		t.Fatalf("got %q", got)
	}
}
