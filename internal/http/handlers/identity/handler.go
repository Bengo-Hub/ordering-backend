package identityhandler

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/cafe-backend/internal/http/handlers"
	"github.com/bengobox/cafe-backend/internal/modules/identity"
)

// Handler exposes identity-related HTTP endpoints.
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
func (h *Handler) Register(r chi.Router, auth *Authenticator) {
	r.Route("/auth", func(authRouter chi.Router) {
		authRouter.Post("/login", h.Login)
		authRouter.Post("/google/start", h.BeginGoogleOAuth)
		authRouter.Post("/google/complete", h.CompleteGoogleOAuth)

		authRouter.With(auth.RequireAuth).Get("/me", h.Me)
		authRouter.With(auth.RequireAuth).Post("/logout", h.Logout)
		authRouter.Post("/refresh", h.Refresh)
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

// Login authenticates a user via email and password.
// @Summary Sign in with email and password
// @Description Authenticates a user with email/password credentials and issues session tokens scoped to the selected role.
// @Tags Auth
// @Accept json
// @Produce json
// @Param payload body LoginRequest true "Login request payload"
// @Success 200 {object} AuthResponsePayload
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Router /auth/login [post]
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	role := identity.Role(req.Role)
	meta := identity.RequestMeta{
		UserAgent: r.UserAgent(),
		IP:        clientIP(r),
	}

	result, err := h.service.LoginWithEmail(r.Context(), req.Email, req.Password, role, meta)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, toAuthResponsePayload(result))
}

// BeginGoogleOAuth starts the Google OAuth workflow.
// @Summary Start Google OAuth sign-in
// @Description Generates a Google OAuth consent URL for the requested role.
// @Tags Auth
// @Accept json
// @Produce json
// @Param payload body GoogleStartRequest true "Google OAuth start request"
// @Success 200 {object} GoogleOAuthURLResponse
// @Failure 400 {object} handlers.ErrorResponse
// @Router /auth/google/start [post]
func (h *Handler) BeginGoogleOAuth(w http.ResponseWriter, r *http.Request) {
	var req GoogleStartRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	role := identity.Role(req.Role)
	url, err := h.service.BeginGoogleOAuth(r.Context(), role, req.RedirectURI)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, GoogleOAuthURLResponse{URL: url})
}

// CompleteGoogleOAuth completes the OAuth workflow and issues tokens.
// @Summary Complete Google OAuth sign-in
// @Description Exchanges the Google authorization code for tokens, creates a user if required, and issues session credentials.
// @Tags Auth
// @Accept json
// @Produce json
// @Param payload body GoogleCompleteRequest true "Google OAuth completion request"
// @Success 200 {object} AuthResponsePayload
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Failure 403 {object} handlers.ErrorResponse
// @Router /auth/google/complete [post]
func (h *Handler) CompleteGoogleOAuth(w http.ResponseWriter, r *http.Request) {
	var req GoogleCompleteRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	meta := identity.RequestMeta{
		UserAgent: r.UserAgent(),
		IP:        clientIP(r),
	}

	result, err := h.service.CompleteGoogleOAuth(r.Context(), req.Code, req.State, meta)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, toAuthResponsePayload(result))
}

// Refresh issues new tokens based on refresh token.
// @Summary Refresh session tokens
// @Description Issues a fresh access token and refresh token pair using a valid refresh token.
// @Tags Auth
// @Accept json
// @Produce json
// @Param payload body RefreshRequest true "Refresh token request"
// @Success 200 {object} AuthResponsePayload
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Router /auth/refresh [post]
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	meta := identity.RequestMeta{
		UserAgent: r.UserAgent(),
		IP:        clientIP(r),
	}

	result, err := h.service.Refresh(r.Context(), req.RefreshToken, meta)
	if err != nil {
		h.handleError(w, err)
		return
	}
	handlers.RespondJSON(w, http.StatusOK, toAuthResponsePayload(result))
}

// Logout invalidates the current session.
// @Summary Sign out the current session
// @Description Revokes the active session belonging to the authenticated user.
// @Tags Auth
// @Security BearerAuth
// @Produce json
// @Success 200 {object} OperationStatusResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Router /auth/logout [post]
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	claims, ok := identity.ClaimsFromContext(r.Context())
	if !ok {
		handlers.RespondError(w, http.StatusUnauthorized, "missing session")
		return
	}

	sessionID, err := uuid.Parse(claims.SessionID)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "invalid session")
		return
	}

	if err := h.service.Logout(r.Context(), sessionID); err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, OperationStatusResponse{Status: "signed_out"})
}

// Me returns current user profile.
// @Summary Get current authenticated user
// @Description Returns the authenticated user's profile, preferences, and current session metadata.
// @Tags Auth
// @Security BearerAuth
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

	h.respondWithCurrentSession(w, r, user)
}

// UpdateProfile updates user profile information.
// @Summary Update profile information
// @Description Allows authenticated users to update their profile fields.
// @Tags Users
// @Security BearerAuth
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

	h.respondWithCurrentSession(w, r, user)
}

// UpdatePreferences updates user preferences.
// @Summary Update user preferences
// @Description Allows authenticated users to update theme, language, and notification preferences.
// @Tags Users
// @Security BearerAuth
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

	h.respondWithCurrentSession(w, r, user)
}

// UpdateSecurity toggles MFA.
// @Summary Update account security settings
// @Description Enables or disables two-factor authentication for the authenticated user.
// @Tags Users
// @Security BearerAuth
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

	h.respondWithCurrentSession(w, r, user)
}

// ListOrderSummary returns order summary for current user.
// @Summary List customer order summaries
// @Description Returns a collection of recent order summaries for the authenticated customer.
// @Tags Customers
// @Security BearerAuth
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
	h.log.Warn("identity request failed", zap.Error(err))
	switch {
	case errors.Is(err, identity.ErrInvalidCredentials):
		handlers.RespondError(w, http.StatusUnauthorized, "invalid credentials")
	case errors.Is(err, identity.ErrRoleNotPermitted):
		handlers.RespondError(w, http.StatusForbidden, "role not permitted")
	case errors.Is(err, identity.ErrUserNotFound):
		handlers.RespondError(w, http.StatusNotFound, "user not found")
	default:
		handlers.RespondError(w, http.StatusInternalServerError, "internal error")
	}
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dst)
}

func currentAccessToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
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

func (h *Handler) respondWithCurrentSession(w http.ResponseWriter, r *http.Request, user *identity.User) {
	session, ok := identity.SessionFromContext(r.Context())
	if !ok {
		handlers.RespondError(w, http.StatusUnauthorized, "session not found")
		return
	}

	resp := AuthResponsePayload{
		Session: SessionResponsePayload{
			AccessToken:  currentAccessToken(r),
			RefreshToken: session.RefreshToken,
			ExpiresAt:    session.ExpiresAt.Format(time.RFC3339),
			SessionID:    session.ID.String(),
		},
		User: toUserResponsePayload(user),
	}

	handlers.RespondJSON(w, http.StatusOK, resp)
}

// LoginRequest models email/password authentication input.
type LoginRequest struct {
	Email    string `json:"email" example:"customer@demo.com"`
	Password string `json:"password" example:"demo1234"`
	Role     string `json:"role" example:"customer"`
}

// GoogleStartRequest starts the OAuth workflow.
type GoogleStartRequest struct {
	Role        string `json:"role" example:"customer"`
	RedirectURI string `json:"redirectUri" example:"http://localhost:3000/auth/callback"`
}

// GoogleCompleteRequest completes OAuth.
type GoogleCompleteRequest struct {
	Code  string `json:"code" example:"auth-code"`
	State string `json:"state,omitempty" example:"customer"`
}

// RefreshRequest attempts to issue new tokens from a refresh token.
type RefreshRequest struct {
	RefreshToken string `json:"refreshToken" example:"refresh-token"`
}

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

// GoogleOAuthURLResponse contains the generated OAuth consent URL.
type GoogleOAuthURLResponse struct {
	URL string `json:"url"`
}

// AuthResponsePayload is returned after successful authentication or session refresh.
type AuthResponsePayload struct {
	Session SessionResponsePayload `json:"session"`
	User    UserResponsePayload    `json:"user"`
}

// SessionResponsePayload models session tokens.
type SessionResponsePayload struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt    string `json:"expiresAt"`
	SessionID    string `json:"sessionId"`
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

func toAuthResponsePayload(result *identity.AuthResult) AuthResponsePayload {
	return AuthResponsePayload{
		Session: SessionResponsePayload{
			AccessToken:  result.Session.AccessToken,
			RefreshToken: result.Session.RefreshToken,
			ExpiresAt:    result.Session.ExpiresAt.Format(time.RFC3339),
			SessionID:    result.Session.SessionID.String(),
		},
		User: toUserResponsePayload(result.User),
	}
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
