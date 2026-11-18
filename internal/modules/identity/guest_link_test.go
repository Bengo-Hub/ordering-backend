package identity

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/config"
)

// A new customer gets past guest orders only through a proven contact: the email when auth-api
// says it is verified (never a phone-only placeholder address), the phone when auth-api holds it
// as verified.
func TestHandleAuthUserCreated_LinksGuestOrdersByVerifiedContact(t *testing.T) {
	tenantID := uuid.New()
	cases := []struct {
		name          string
		payload       map[string]interface{}
		verifiedPhone string
		wantCalled    bool
		wantEmail     string
		wantPhone     string
	}{
		{
			name:       "verified email",
			payload:    map[string]interface{}{"email": "Wanjiku@Example.com", "email_verified": true},
			wantCalled: true, wantEmail: "wanjiku@example.com",
		},
		{
			name:       "unverified email, no phone",
			payload:    map[string]interface{}{"email": "someone@example.com", "email_verified": false},
			wantCalled: false,
		},
		{
			name:          "unverified email, verified phone",
			payload:       map[string]interface{}{"email": "someone@example.com"},
			verifiedPhone: "+254712345678",
			wantCalled:    true, wantPhone: "+254712345678",
		},
		{
			name:       "phone-only placeholder email",
			payload:    map[string]interface{}{"email": "p254712345678@placeholder.local", "email_verified": true},
			wantCalled: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			userID := uuid.New()
			payload := map[string]interface{}{"user_id": userID.String(), "status": "active"}
			for k, v := range tc.payload {
				payload[k] = v
			}
			service, err := NewService(NewMemoryRepository(), config.AuthConfig{
				AccessTokenSecret: "test-secret-key-for-testing-only-min-32-chars",
			}, zap.NewNop(), nil)
			if err != nil {
				t.Fatalf("service: %v", err)
			}
			h := NewEventHandler(service, zap.NewNop())
			h.VerifiedPhone = func(context.Context, string) string { return tc.verifiedPhone }
			called := false
			var gotEmail, gotPhone string
			var gotTenant uuid.UUID
			h.LinkGuestOrders = func(_ context.Context, tid, _ uuid.UUID, email, phone string) (int, error) {
				called, gotTenant, gotEmail, gotPhone = true, tid, email, phone
				return 1, nil
			}
			if err := h.HandleAuthUserCreated(context.Background(), makeUserEvent("created", tenantID, payload)); err != nil {
				t.Fatalf("HandleAuthUserCreated: %v", err)
			}
			if called != tc.wantCalled {
				t.Fatalf("linked = %v, want %v", called, tc.wantCalled)
			}
			if !called {
				return
			}
			if gotTenant != tenantID || gotEmail != tc.wantEmail || gotPhone != tc.wantPhone {
				t.Fatalf("linked tenant=%s email=%q phone=%q, want %s %q %q", gotTenant, gotEmail, gotPhone, tenantID, tc.wantEmail, tc.wantPhone)
			}
		})
	}
}
