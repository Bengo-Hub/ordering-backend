package identityhandler

import (
	"net/http"
	"strings"

	authclient "github.com/Bengo-Hub/shared-auth-client"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/http/handlers"
	"github.com/bengobox/ordering-backend/internal/modules/identity"
)

// Authenticator provides middleware helpers for RBAC-protected routes.
type Authenticator struct {
	log           *zap.Logger
	service       *identity.Service
	authValidator *authclient.Validator // For auth-service JWT validation
}

// NewAuthenticator constructs middleware helpers.
func NewAuthenticator(log *zap.Logger, service *identity.Service, authValidator *authclient.Validator) *Authenticator {
	return &Authenticator{
		log:           log.Named("identity.Authenticator"),
		service:       service,
		authValidator: authValidator,
	}
}

// IsSuperuser checks if the user is a superuser from auth-service.
// Superusers bypass all RBAC checks.
func IsSuperuser(claims *authclient.Claims) bool {
	if claims == nil {
		return false
	}
	return claims.IsSuperuser()
}

// IsAdmin checks if the user has admin or superuser role from auth-service.
func IsAdmin(claims *authclient.Claims) bool {
	if claims == nil {
		return false
	}
	return claims.IsAdmin()
}

// claimsHasPermission checks if the JWT claims contain a specific permission code.
func claimsHasPermission(claims *authclient.Claims, perm identity.Permission) bool {
	if claims == nil {
		return false
	}
	permStr := string(perm)
	for _, p := range claims.Permissions {
		if p == permStr {
			return true
		}
	}
	return false
}

// claimsHasAllPermissions checks if the JWT claims contain all specified permission codes.
func claimsHasAllPermissions(claims *authclient.Claims, perms []identity.Permission) bool {
	for _, perm := range perms {
		if !claimsHasPermission(claims, perm) {
			return false
		}
	}
	return true
}

// RequireAuth enforces presence of a valid access token.
// Uses auth-service JWT validation if available, otherwise falls back to legacy token verification.
func (a *Authenticator) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r.Header.Get("Authorization"))
		if token == "" {
			handlers.RespondError(w, http.StatusUnauthorized, "missing authorization header")
			return
		}

		// If auth-service validator is available, use it (preferred)
		if a.authValidator != nil {
			authClaims, err := a.authValidator.ValidateToken(token)
			if err != nil {
				a.log.Warn("auth-service token validation failed", zap.Error(err))
				handlers.RespondError(w, http.StatusUnauthorized, "invalid token")
				return
			}

			// Extract user ID from auth-service claims
			userID, err := authClaims.UserID()
			if err != nil {
				handlers.RespondError(w, http.StatusUnauthorized, "invalid user ID in token")
				return
			}

			// Load or JIT-provision user from local database
			user, err := a.service.GetUser(r.Context(), userID)
			if err != nil {
				// JIT provisioning: valid token but user not in DB (e.g. NATS sync delayed)
				tenantSlug := chi.URLParam(r, "tenant")
				if tenantSlug == "" {
					tenantSlug = r.URL.Query().Get("tenant")
				}
				if tenantSlug == "" {
					tenantSlug = authClaims.GetTenantSlug()
				}
				authUserData := map[string]interface{}{"roles": authClaims.Roles, "permissions": authClaims.Permissions}
				if authClaims.Email != "" {
					authUserData["email"] = authClaims.Email
				}
				user, err = a.service.EnsureUserFromToken(r.Context(), userID, tenantSlug, authUserData)
				if err != nil {
					a.log.Warn("JIT provision failed", zap.String("user_id", userID.String()), zap.Error(err))
					handlers.RespondError(w, http.StatusUnauthorized, "user not found")
					return
				}
				a.log.Info("user JIT-provisioned", zap.String("user_id", userID.String()))
			}

			// Store auth-service claims in context (authclient middleware already does this, but ensure it's there)
			ctx := r.Context()
			// Claims are already in context from authMiddleware, but ensure user is loaded
			ctx = identity.ContextWithUser(ctx, user)

			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Require auth-service validator
		handlers.RespondError(w, http.StatusUnauthorized, "auth-service validator not configured")
	})
}

// OptionalAuth attempts to authenticate the user but does not fail if missing or invalid.
func (a *Authenticator) OptionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r.Header.Get("Authorization"))
		if token == "" {
			next.ServeHTTP(w, r)
			return
		}

		if a.authValidator != nil {
			authClaims, err := a.authValidator.ValidateToken(token)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			userID, err := authClaims.UserID()
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			user, err := a.service.GetUser(r.Context(), userID)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			ctx := identity.ContextWithUser(r.Context(), user)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Fallback to anonymous
		next.ServeHTTP(w, r)
	})
}

// RequireRoles enforces that the authenticated user has at least one of the supplied roles.
// Superusers and admins bypass this check.
func (a *Authenticator) RequireRoles(roles ...identity.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check superuser/admin from auth-service JWT claims first (source of truth)
			if claims, ok := authclient.ClaimsFromContext(r.Context()); ok {
				if IsSuperuser(claims) || IsAdmin(claims) {
					next.ServeHTTP(w, r)
					return
				}
				// Check roles from JWT claims directly
				for _, required := range roles {
					if claims.HasRole(string(required)) {
						next.ServeHTTP(w, r)
						return
					}
				}
			}

			// Fall back to local user roles
			user, ok := identity.UserFromContext(r.Context())
			if !ok {
				handlers.RespondError(w, http.StatusForbidden, "user missing from context")
				return
			}
			if !userHasRole(user, roles) {
				handlers.RespondError(w, http.StatusForbidden, "insufficient role")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireSuperuser enforces that the authenticated user is a platform superuser.
// Unlike RequirePermissions/RequireRoles, this does NOT honor the admin role — a
// tenant admin must not pass. Used to gate platform-owner-only endpoints
// (e.g. platform service-config defaults and unmasked secrets).
func (a *Authenticator) RequireSuperuser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := authclient.ClaimsFromContext(r.Context())
		if !ok {
			handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if !IsSuperuser(claims) {
			handlers.RespondError(w, http.StatusForbidden, "superuser access required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequirePermissions enforces that the user has all supplied permissions.
// Priority: (1) superuser/admin bypass → (2) JWT claims permissions → (3) local DB user permissions.
// JWT claims are the source of truth and are always checked before local DB to avoid stale data.
func (a *Authenticator) RequirePermissions(perms ...identity.Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if claims, ok := authclient.ClaimsFromContext(r.Context()); ok {
				// Superuser/admin bypass all permission checks
				if IsSuperuser(claims) || IsAdmin(claims) {
					next.ServeHTTP(w, r)
					return
				}
				// Check permissions directly from JWT claims (avoids stale local DB)
				if len(claims.Permissions) > 0 {
					if claimsHasAllPermissions(claims, perms) {
						next.ServeHTTP(w, r)
						return
					}
					// JWT claims exist but permissions not satisfied — deny early
					handlers.RespondError(w, http.StatusForbidden, "insufficient permissions")
					return
				}
			}

			// Fall back to local DB user permissions when JWT has no Permissions field
			user, ok := identity.UserFromContext(r.Context())
			if !ok {
				handlers.RespondError(w, http.StatusForbidden, "user missing from context")
				return
			}
			if !userHasPermissions(user, perms) {
				handlers.RespondError(w, http.StatusForbidden, "insufficient permissions")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func bearerToken(header string) string {
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func userHasRole(user *identity.User, roles []identity.Role) bool {
	if len(roles) == 0 {
		return true
	}
	for _, candidate := range roles {
		if user.HasRole(candidate) {
			return true
		}
	}
	return false
}

func userHasPermissions(user *identity.User, perms []identity.Permission) bool {
	if len(perms) == 0 {
		return true
	}
	for _, perm := range perms {
		if !user.HasPermission(perm) {
			return false
		}
	}
	return true
}
