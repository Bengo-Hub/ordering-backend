package identityhandler

import (
	"net/http"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/food-delivery-backend/internal/http/handlers"
	"github.com/bengobox/food-delivery-backend/internal/modules/identity"
)

// Authenticator provides middleware helpers for RBAC-protected routes.
type Authenticator struct {
	log     *zap.Logger
	service *identity.Service
}

// NewAuthenticator constructs middleware helpers.
func NewAuthenticator(log *zap.Logger, service *identity.Service) *Authenticator {
	return &Authenticator{
		log:     log.Named("identity.Authenticator"),
		service: service,
	}
}

// RequireAuth enforces presence of a valid access token.
func (a *Authenticator) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r.Header.Get("Authorization"))
		if token == "" {
			handlers.RespondError(w, http.StatusUnauthorized, "missing authorization header")
			return
		}

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
func (a *Authenticator) RequireRoles(roles ...identity.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
func (a *Authenticator) RequirePermissions(perms ...identity.Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
