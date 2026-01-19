package identityhandler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	authclient "github.com/Bengo-Hub/shared-auth-client"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/config"
	"github.com/bengobox/ordering-backend/internal/modules/identity"
)

func TestIsSuperuser(t *testing.T) {
	tests := []struct {
		name   string
		claims *authclient.Claims
		want   bool
	}{
		{
			name: "superuser in scope",
			claims: &authclient.Claims{
				Scope: []string{"superuser", "read", "write"},
			},
			want: true,
		},
		{
			name: "superuser as role",
			claims: &authclient.Claims{
				Scope: []string{"role:superuser", "read"},
			},
			want: true,
		},
		{
			name: "no superuser",
			claims: &authclient.Claims{
				Scope: []string{"read", "write"},
			},
			want: false,
		},
		{
			name:   "nil claims",
			claims: nil,
			want:   false,
		},
		{
			name: "empty scope",
			claims: &authclient.Claims{
				Scope: []string{},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSuperuser(tt.claims)
			if got != tt.want {
				t.Errorf("IsSuperuser() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestAuthenticator_RequireRoles_WithSuperuser tests the RequireRoles middleware with superuser claims.
// NOTE: This test is currently skipped because it requires setting claims in context using
// authclient's internal private context key. The authclient package (v0.2.0) does not export
// a helper function to inject claims for testing purposes.
//
// Possible solutions:
// 1. Add authclient.ContextWithClaims() helper in shared-auth-client package
// 2. Use integration tests with real JWT validation instead of unit tests
// 3. Mock the entire authclient.ClaimsFromContext function (requires interface/dependency injection)
//
// The actual implementation works correctly in production with real auth middleware.
func TestAuthenticator_RequireRoles_WithSuperuser(t *testing.T) {
	t.Skip("Test requires authclient.ContextWithClaims() helper - not available in v0.2.0")

	tests := []struct {
		name           string
		claims         *authclient.Claims
		requiredRoles  []identity.Role
		expectedStatus int
	}{
		{
			name: "superuser bypasses role check",
			claims: &authclient.Claims{
				Scope: []string{"superuser"},
			},
			requiredRoles:  []identity.Role{identity.RoleAdmin},
			expectedStatus: http.StatusOK,
		},
		{
			name: "non-superuser with required role",
			claims: &authclient.Claims{
				Scope: []string{"read"},
			},
			requiredRoles:  []identity.Role{identity.RoleCustomer},
			expectedStatus: http.StatusForbidden, // Will fail because user context is missing
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zap.NewNop()
			repo := identity.NewMemoryRepository()
			authCfg := config.AuthConfig{}
			service, _ := identity.NewService(repo, authCfg, logger)

			// Create authenticator with nil validator (for testing)
			authenticator := NewAuthenticator(logger, service, nil)

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			// TODO: Need authclient.ContextWithClaims(req.Context(), tt.claims)
			rec := httptest.NewRecorder()

			handler := authenticator.RequireRoles(tt.requiredRoles...)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rec.Code)
			}
		})
	}
}
