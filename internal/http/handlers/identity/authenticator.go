package identityhandler

import (
	"net/http"
	"strings"

	"github.com/google/uuid"
	authclient "github.com/Bengo-Hub/shared-auth-client"
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
	// Check if "superuser" is in the scope or roles
	for _, scope := range claims.Scope {
		if scope == "superuser" || scope == "role:superuser" {
			return true
		}
	}
	return false
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

			// Load or sync user from local database
			user, err := a.service.GetUser(r.Context(), userID)
			if err != nil {
				// User might not exist locally yet, try to sync from auth-service
				a.log.Warn("user not found locally, attempting sync", zap.String("user_id", userID.String()), zap.Error(err))
				// For now, return error - user should be synced via events or on login
				handlers.RespondError(w, http.StatusUnauthorized, "user not found")
				return
			}

			// Store auth-service claims in context (authclient middleware already does this, but ensure it's there)
			ctx := r.Context()
			// Claims are already in context from authMiddleware, but ensure user is loaded
			ctx = identity.ContextWithUser(ctx, user)

			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Fallback to legacy token verification (deprecated)
		a.log.Warn("Using legacy token verification - auth-service validator not configured")
		claims, err := a.service.VerifyAccessToken(r.Context(), token)
		if err != nil {
			a.log.Warn("access token verification failed", zap.Error(err))
			handlers.RespondError(w, http.StatusUnauthorized, "invalid token")
			return
		}

		sessionID, err := uuid.Parse(claims.SessionID)
		if err != nil {
			handlers.RespondError(w, http.StatusUnauthorized, "invalid session")
			return
		}

		userID, err := uuid.Parse(claims.UserID)
		if err != nil {
			handlers.RespondError(w, http.StatusUnauthorized, "invalid subject")
			return
		}

		user, err := a.service.GetUser(r.Context(), userID)
		if err != nil {
			a.log.Warn("failed to load user", zap.Error(err))
			handlers.RespondError(w, http.StatusUnauthorized, "user not found")
			return
		}

		session, err := a.service.GetSession(r.Context(), sessionID)
		if err != nil {
			a.log.Warn("failed to load session", zap.Error(err))
			handlers.RespondError(w, http.StatusUnauthorized, "session not found")
			return
		}

		ctx := identity.ContextWithClaims(r.Context(), claims)
		ctx = identity.ContextWithUser(ctx, user)
		ctx = identity.ContextWithSession(ctx, session)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireRoles enforces that the authenticated user has at least one of the supplied roles.
// Superusers bypass this check.
func (a *Authenticator) RequireRoles(roles ...identity.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check for superuser from auth-service claims
			if claims, ok := authclient.ClaimsFromContext(r.Context()); ok {
				if IsSuperuser(claims) {
					// Superuser bypasses all role checks
					next.ServeHTTP(w, r)
					return
				}
			}

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

// RequirePermissions enforces that the user has all supplied permissions.
// Superusers bypass this check.
func (a *Authenticator) RequirePermissions(perms ...identity.Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check for superuser from auth-service claims
			if claims, ok := authclient.ClaimsFromContext(r.Context()); ok {
				if IsSuperuser(claims) {
					// Superuser bypasses all permission checks
					next.ServeHTTP(w, r)
					return
				}
			}

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
