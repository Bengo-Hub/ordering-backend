package identityhandler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/http/handlers"
	"github.com/bengobox/ordering-backend/internal/modules/identity"
)

// Handler exposes identity-related HTTP endpoints.
// Authentication is handled entirely by the SSO (auth-service) via OIDC.
// This handler provides user profile management and sync confirmation endpoints.
type Handler struct {
	log     *zap.Logger
	service *identity.Service
}

// New constructs a Handler instance.
func New(log *zap.Logger, service *identity.Service) *Handler {
	return &Handler{
		log:     log.Named("identity.Handler"),
		service: service,
	}
}

// Register mounts identity routes on the supplied router, using the provided middleware.
// Login/register/OAuth endpoints have been removed - all authentication goes through SSO.
func (h *Handler) Register(r chi.Router, auth *Authenticator) {
	r.Route("/auth", func(authRouter chi.Router) {
		// GET /auth/me - Returns current user profile (confirms user sync after SSO login)
		authRouter.With(auth.RequireAuth).Get("/me", h.Me)
		// POST /auth/logout - Client-side cleanup (real logout happens at SSO)
		authRouter.With(auth.RequireAuth).Post("/logout", h.Logout)
	})

	r.Route("/users", func(usersRouter chi.Router) {
		usersRouter.With(auth.RequireAuth).Patch("/profile", h.UpdateProfile)
		usersRouter.With(auth.RequireAuth).Patch("/preferences", h.UpdatePreferences)
		usersRouter.With(auth.RequireAuth).Post("/security", h.UpdateSecurity)
	})

	r.Route("/customers", func(customersRouter chi.Router) {
		customersRouter.With(auth.RequireAuth, auth.RequireRoles(identity.RoleCustomer)).
			Get("/orders/summary", h.ListOrderSummary)
	})
}

// Logout handles server-side logout. The actual sign-out happens at the SSO level.
// This endpoint exists for backward compatibility and to allow any local cleanup.
// @Summary Sign out the current session
// @Description Acknowledges logout. The real sign-out happens at the SSO authorize endpoint.
// @Tags Auth
// @Security bearerAuth
// @Produce json
// @Success 200 {object} OperationStatusResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Router /auth/logout [post]
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	handlers.RespondJSON(w, http.StatusOK, OperationStatusResponse{Status: "signed_out"})
}

// Me returns current user profile.
// This is the primary endpoint called by frontends after SSO callback to confirm
// the user has been synced from auth-service via NATS events.
// @Summary Get current authenticated user
// @Description Returns the authenticated user's profile, preferences, and metadata.
// @Tags Auth
// @Security bearerAuth
// @Produce json
// @Success 200 {object} AuthResponsePayload
// @Failure 401 {object} handlers.ErrorResponse
// @Router /auth/me [get]
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	user, ok := identity.UserFromContext(r.Context())
	if !ok {
		handlers.RespondError(w, http.StatusUnauthorized, "missing user")
		return
	}

	h.respondWithUser(w, r, user)
}

// UpdateProfile updates user profile information.
// @Summary Update profile information
// @Description Allows authenticated users to update their profile fields.
// @Tags Users
// @Security bearerAuth
// @Accept json
// @Produce json
// @Param payload body UpdateProfileRequest true "Profile update payload"
// @Success 200 {object} AuthResponsePayload
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Router /users/profile [patch]
func (h *Handler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	var req UpdateProfileRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	userID := identity.MustUserID(r.Context())
	if userID == uuid.Nil {
		handlers.RespondError(w, http.StatusUnauthorized, "missing user")
		return
	}

	user, err := h.service.UpdateProfile(r.Context(), userID, identity.ProfileUpdateInput{
		FullName:  optionalString(req.FullName),
		Phone:     optionalString(req.Phone),
		AvatarURL: optionalString(req.AvatarURL),
	})
	if err != nil {
		h.handleError(w, err)
		return
	}

	h.respondWithUser(w, r, user)
}

// UpdatePreferences updates user preferences.
// @Summary Update user preferences
// @Description Allows authenticated users to update theme, language, and notification preferences.
// @Tags Users
// @Security bearerAuth
// @Accept json
// @Produce json
// @Param payload body UpdatePreferencesRequest true "Preferences update payload"
// @Success 200 {object} AuthResponsePayload
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Router /users/preferences [patch]
func (h *Handler) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	var req UpdatePreferencesRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	userID := identity.MustUserID(r.Context())
	if userID == uuid.Nil {
		handlers.RespondError(w, http.StatusUnauthorized, "missing user")
		return
	}

	var notifications *identity.NotificationPreferences
	if req.Notifications != nil {
		notifications = &identity.NotificationPreferences{
			Email: req.Notifications.Email,
			SMS:   req.Notifications.SMS,
			Push:  req.Notifications.Push,
		}
	}

	user, err := h.service.UpdatePreferences(r.Context(), userID, identity.PreferencesUpdateInput{
		Theme:         optionalString(req.Theme),
		Language:      optionalString(req.Language),
		Notifications: notifications,
	})
	if err != nil {
		h.handleError(w, err)
		return
	}

	h.respondWithUser(w, r, user)
}

// UpdateSecurity toggles MFA.
// @Summary Update account security settings
// @Description Enables or disables two-factor authentication for the authenticated user.
// @Tags Users
// @Security bearerAuth
// @Accept json
// @Produce json
// @Param payload body UpdateSecurityRequest true "Security update payload"
// @Success 200 {object} AuthResponsePayload
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Router /users/security [post]
func (h *Handler) UpdateSecurity(w http.ResponseWriter, r *http.Request) {
	var req UpdateSecurityRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	userID := identity.MustUserID(r.Context())
	if userID == uuid.Nil {
		handlers.RespondError(w, http.StatusUnauthorized, "missing user")
		return
	}

	user, err := h.service.UpdateSecurity(r.Context(), userID, identity.SecurityUpdateInput{
		EnableTwoFactor:  req.EnableTwoFactor,
		DisableTwoFactor: req.DisableTwoFactor,
	})
	if err != nil {
		h.handleError(w, err)
		return
	}

	h.respondWithUser(w, r, user)
}

// ListOrderSummary returns order summary for current user.
// @Summary List customer order summaries
// @Description Returns a collection of recent order summaries for the authenticated customer.
// @Tags Customers
// @Security bearerAuth
// @Produce json
// @Success 200 {array} OrderSummaryResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Router /customers/orders/summary [get]
func (h *Handler) ListOrderSummary(w http.ResponseWriter, r *http.Request) {
	userID := identity.MustUserID(r.Context())
	if userID == uuid.Nil {
		handlers.RespondError(w, http.StatusUnauthorized, "missing user")
		return
	}

	orders, err := h.service.GetOrders(r.Context(), userID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	payload := make([]OrderSummaryResponse, 0, len(orders))
	for _, order := range orders {
		payload = append(payload, OrderSummaryResponse{
			ID:       order.ID.String(),
			Status:   order.Status,
			Total:    order.Total,
			PlacedAt: order.PlacedAt.Format(time.RFC3339),
			ETA:      formatOptionalTime(order.ETA),
		})
	}

	handlers.RespondJSON(w, http.StatusOK, payload)
}

func (h *Handler) handleError(w http.ResponseWriter, err error) {
	h.log.Error("identity request failed", zap.Error(err), zap.String("error_type", fmt.Sprintf("%T", err)))

	switch {
	case errors.Is(err, identity.ErrInvalidCredentials):
		handlers.RespondError(w, http.StatusUnauthorized, "invalid credentials")
	case errors.Is(err, identity.ErrRoleNotPermitted):
		handlers.RespondError(w, http.StatusForbidden, "role not permitted")
	case errors.Is(err, identity.ErrUserNotFound):
		handlers.RespondError(w, http.StatusNotFound, "user not found")
	case errors.Is(err, identity.ErrTwoFactorConflict):
		handlers.RespondError(w, http.StatusBadRequest, "two-factor conflict: cannot enable and disable simultaneously")
	default:
		handlers.RespondError(w, http.StatusInternalServerError, err.Error())
	}
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dst)
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// respondWithUser returns the user profile in the standard AuthResponsePayload format.
// With SSO, session tokens are managed by the SSO provider - the session fields
// are populated from the request's Bearer token for backward compatibility.
func (h *Handler) respondWithUser(w http.ResponseWriter, r *http.Request, user *identity.User) {
	// Try to get session from context (legacy auth path)
	session, hasSession := identity.SessionFromContext(r.Context())

	var sessionPayload SessionResponsePayload
	if hasSession && session != nil {
		sessionPayload = SessionResponsePayload{
			AccessToken:  currentAccessToken(r),
			RefreshToken: session.RefreshToken,
			ExpiresAt:    session.ExpiresAt.Format(time.RFC3339),
			SessionID:    session.ID.String(),
		}
	} else {
		// SSO auth path - return the bearer token as access token, no local session
		sessionPayload = SessionResponsePayload{
			AccessToken: currentAccessToken(r),
		}
	}

	resp := AuthResponsePayload{
		Session: sessionPayload,
		User:    toUserResponsePayload(user),
	}

	handlers.RespondJSON(w, http.StatusOK, resp)
}

func currentAccessToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// --- Request/Response types ---

// UpdateProfileRequest captures mutable profile fields.
type UpdateProfileRequest struct {
	FullName  string `json:"fullName" example:"Urban Café Guest"`
	Phone     string `json:"phone" example:"+254700000000"`
	AvatarURL string `json:"avatarUrl" example:"https://cdn.example.com/avatar.png"`
}

// UpdatePreferencesRequest captures mutable preference fields.
type UpdatePreferencesRequest struct {
	Theme         string                       `json:"theme" example:"dark"`
	Language      string                       `json:"language" example:"en"`
	Notifications *NotificationPreferenceInput `json:"notifications"`
}

// NotificationPreferenceInput represents channel toggles.
type NotificationPreferenceInput struct {
	Email bool `json:"email" example:"true"`
	SMS   bool `json:"sms" example:"false"`
	Push  bool `json:"push" example:"true"`
}

// UpdateSecurityRequest toggles MFA.
type UpdateSecurityRequest struct {
	EnableTwoFactor  bool `json:"enableTwoFactor"`
	DisableTwoFactor bool `json:"disableTwoFactor"`
}

// AuthResponsePayload is returned for authenticated user endpoints.
type AuthResponsePayload struct {
	Session SessionResponsePayload `json:"session"`
	User    UserResponsePayload    `json:"user"`
}

// SessionResponsePayload models session tokens.
type SessionResponsePayload struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken,omitempty"`
	ExpiresAt    string `json:"expiresAt,omitempty"`
	SessionID    string `json:"sessionId,omitempty"`
}

// UserResponsePayload models user profile output.
type UserResponsePayload struct {
	ID                   string                  `json:"id"`
	Email                string                  `json:"email"`
	FullName             string                  `json:"fullName"`
	Phone                string                  `json:"phone"`
	AvatarURL            string                  `json:"avatarUrl"`
	Roles                []identity.Role         `json:"roles"`
	Permissions          []identity.Permission   `json:"permissions"`
	LoyaltyPoints        int                     `json:"loyaltyPoints"`
	AvailableCoupons     int                     `json:"availableCoupons"`
	DefaultLocationLabel string                  `json:"defaultLocationLabel"`
	TwoFactorEnabled     bool                    `json:"twoFactorEnabled"`
	BackupCodesEnabled   bool                    `json:"backupCodesEnabled"`
	Preferences          UserPreferencesResponse `json:"preferences"`
	LastLoginAt          string                  `json:"lastLoginAt"`
	CreatedAt            string                  `json:"createdAt"`
	UpdatedAt            string                  `json:"updatedAt"`
}

// UserPreferencesResponse models preference output.
type UserPreferencesResponse struct {
	Theme         string                      `json:"theme"`
	Language      string                      `json:"language"`
	Notifications NotificationPreferenceInput `json:"notifications"`
}

// OrderSummaryResponse models lightweight order details.
type OrderSummaryResponse struct {
	ID       string  `json:"id"`
	Status   string  `json:"status"`
	Total    float64 `json:"total"`
	PlacedAt string  `json:"placedAt"`
	ETA      string  `json:"eta,omitempty"`
}

// OperationStatusResponse provides a generic status indicator.
type OperationStatusResponse struct {
	Status string `json:"status"`
}

func toUserResponsePayload(user *identity.User) UserResponsePayload {
	return UserResponsePayload{
		ID:                   user.ID.String(),
		Email:                user.Email,
		FullName:             user.FullName,
		Phone:                user.Phone,
		AvatarURL:            user.AvatarURL,
		Roles:                user.Roles,
		Permissions:          user.Permissions,
		LoyaltyPoints:        user.LoyaltyPoints,
		AvailableCoupons:     user.AvailableCoupons,
		DefaultLocationLabel: user.DefaultLocationLabel,
		TwoFactorEnabled:     user.TwoFactorEnabled,
		BackupCodesEnabled:   len(user.BackupCodes) > 0,
		Preferences: UserPreferencesResponse{
			Theme:    user.Preferences.Theme,
			Language: user.Preferences.Language,
			Notifications: NotificationPreferenceInput{
				Email: user.Preferences.Notifications.Email,
				SMS:   user.Preferences.Notifications.SMS,
				Push:  user.Preferences.Notifications.Push,
			},
		},
		LastLoginAt: formatOptionalTime(user.LastLoginAt),
		CreatedAt:   user.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   user.UpdatedAt.Format(time.RFC3339),
	}
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format(time.RFC3339)
}
