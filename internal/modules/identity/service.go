package identity

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/bengobox/food-delivery-backend/internal/config"
)

// Service coordinates identity workflows across persistence and token services.
type Service struct {
	repo        Repository
	authCfg     config.AuthConfig
	tokenSigner *TokenSigner
	googleCfg   *oauth2.Config
	logger      *zap.Logger
	now         func() time.Time
}

// AuthResult models the payload returned to clients after successful auth.
type AuthResult struct {
	Session SessionTokens
	User    *User
}

// SessionTokens captures issued access and refresh tokens.
type SessionTokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	SessionID    uuid.UUID
}

// NewService constructs the identity service with provided dependencies.
func NewService(repo Repository, authCfg config.AuthConfig, logger *zap.Logger) (*Service, error) {
	tokenSigner, err := NewTokenSigner(authCfg)
	if err != nil {
		return nil, fmt.Errorf("identity: token signer: %w", err)
	}

	var googleCfg *oauth2.Config
	if authCfg.GoogleClientID != "" && authCfg.GoogleClientSecret != "" {
		redirect := authCfg.GoogleRedirectBase
		if redirect == "" {
			redirect = "http://localhost:3000/auth/callback"
		}
		googleCfg = &oauth2.Config{
			ClientID:     authCfg.GoogleClientID,
			ClientSecret: authCfg.GoogleClientSecret,
			Endpoint:     google.Endpoint,
			RedirectURL:  redirect,
			Scopes: []string{
				"openid",
				"email",
				"profile",
			},
		}
	}

	svc := &Service{
		repo:        repo,
		authCfg:     authCfg,
		tokenSigner: tokenSigner,
		googleCfg:   googleCfg,
		logger:      logger.Named("identity.Service"),
		now:         time.Now,
	}

	if err := svc.seedDemoData(context.Background()); err != nil {
		logger.Warn("identity: demo seed failed", zap.Error(err))
	}

	return svc, nil
}

// LoginWithEmail authenticates a user via email/password combination.
func (s *Service) LoginWithEmail(ctx context.Context, email, password string, role Role, meta RequestMeta) (*AuthResult, error) {
	user, err := s.repo.FindUserByEmail(ctx, email)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	if !user.HasRole(role) {
		return nil, ErrRoleNotPermitted
	}

	return s.issueSession(ctx, user, meta)
}

// BeginGoogleOAuth returns an OAuth consent url for the requested role.
func (s *Service) BeginGoogleOAuth(ctx context.Context, role Role, redirectURI string) (string, error) {
	if s.googleCfg == nil {
		// Fallback to a generated demo URL.
		v := url.Values{
			"client_id":     {s.authCfg.GoogleClientID},
			"redirect_uri":  {redirectURI},
			"response_type": {"code"},
			"scope":         {"openid email profile"},
			"state":         {string(role)},
		}
		return fmt.Sprintf("https://accounts.google.com/o/oauth2/v2/auth?%s", v.Encode()), nil
	}

	state, err := s.tokenSigner.GenerateState(role)
	if err != nil {
		return "", err
	}

	redirect := redirectURI
	if redirect == "" {
		redirect = s.googleCfg.RedirectURL
	}

	return s.googleCfg.AuthCodeURL(state,
		oauth2.SetAuthURLParam("redirect_uri", redirect),
		oauth2.SetAuthURLParam("prompt", "select_account"),
	), nil
}

// CompleteGoogleOAuth validates the callback and issues an auth session.
func (s *Service) CompleteGoogleOAuth(ctx context.Context, code string, state string, meta RequestMeta) (*AuthResult, error) {
	if code == "" {
		return nil, fmt.Errorf("identity: oauth code required")
	}

	var role Role
	if state != "" {
		decodedRole, err := s.tokenSigner.ParseState(state)
		if err != nil {
			return nil, err
		}
		role = decodedRole
	}

	// Demo fallback if Google config disabled.
	if s.googleCfg == nil {
		user, err := s.pickDemoUserByRole(ctx, role)
		if err != nil {
			return nil, err
		}
		return s.issueSession(ctx, user, meta)
	}

	token, err := s.googleCfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("identity: google exchange: %w", err)
	}

	client := s.googleCfg.Client(ctx, token)
	profile, err := fetchGoogleProfile(client)
	if err != nil {
		return nil, err
	}

	user, err := s.repo.FindUserByEmail(ctx, profile.Email)
	if err != nil {
		// Create user automatically with default role.
		user, err = s.createGoogleUser(ctx, profile, role)
		if err != nil {
			return nil, err
		}
	}

	if role != "" && !user.HasRole(role) {
		return nil, ErrRoleNotPermitted
	}

	return s.issueSession(ctx, user, meta)
}

// Logout revokes a session.
func (s *Service) Logout(ctx context.Context, sessionID uuid.UUID) error {
	session, err := s.repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		return err
	}
	now := s.now()
	session.RevokedAt = &now
	return s.repo.UpdateSession(ctx, session)
}

// Refresh issues new access tokens from a refresh token.
func (s *Service) Refresh(ctx context.Context, refreshToken string, meta RequestMeta) (*AuthResult, error) {
	session, err := s.repo.FindSessionByToken(ctx, refreshToken)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	if session.RevokedAt != nil || session.ExpiresAt.Before(s.now()) {
		return nil, ErrInvalidCredentials
	}

	user, err := s.repo.FindUserByID(ctx, session.UserID)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	return s.issueSession(ctx, user, meta)
}

// GetUser returns a user by identifier.
func (s *Service) GetUser(ctx context.Context, id uuid.UUID) (*User, error) {
	return s.repo.FindUserByID(ctx, id)
}

// GetSession returns a session by identifier.
func (s *Service) GetSession(ctx context.Context, id uuid.UUID) (*Session, error) {
	return s.repo.FindSessionByID(ctx, id)
}

// GetOrders returns order summaries.
func (s *Service) GetOrders(ctx context.Context, userID uuid.UUID) ([]*OrderSummary, error) {
	return s.repo.ListOrdersByUser(ctx, userID)
}

// UpdateProfile mutates user profile fields.
func (s *Service) UpdateProfile(ctx context.Context, id uuid.UUID, input ProfileUpdateInput) (*User, error) {
	user, err := s.repo.FindUserByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if input.FullName != nil {
		user.FullName = *input.FullName
	}
	if input.Phone != nil {
		user.Phone = *input.Phone
	}
	if input.AvatarURL != nil {
		user.AvatarURL = *input.AvatarURL
	}

	user.UpdatedAt = s.now()

	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

// UpdatePreferences mutates user preferences.
func (s *Service) UpdatePreferences(ctx context.Context, id uuid.UUID, input PreferencesUpdateInput) (*User, error) {
	user, err := s.repo.FindUserByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if input.Theme != nil {
		user.Preferences.Theme = *input.Theme
	}
	if input.Language != nil {
		user.Preferences.Language = *input.Language
	}
	if input.Notifications != nil {
		user.Preferences.Notifications = *input.Notifications
	}

	user.UpdatedAt = s.now()

	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// UpdateSecurity toggles MFA configuration.
func (s *Service) UpdateSecurity(ctx context.Context, id uuid.UUID, input SecurityUpdateInput) (*User, error) {
	user, err := s.repo.FindUserByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if input.EnableTwoFactor && input.DisableTwoFactor {
		return nil, ErrTwoFactorConflict
	}

	if input.EnableTwoFactor {
		user.TwoFactorEnabled = true
	} else if input.DisableTwoFactor {
		user.TwoFactorEnabled = false
	}

	user.UpdatedAt = s.now()

	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// VerifyAccessToken validates the JWT access token.
func (s *Service) VerifyAccessToken(ctx context.Context, token string) (*Claims, error) {
	claims, err := s.tokenSigner.VerifyAccessToken(token)
	if err != nil {
		return nil, err
	}

	// Confirm session still valid.
	sessionID, err := uuid.Parse(claims.SessionID)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	session, err := s.repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	if session.RevokedAt != nil || session.ExpiresAt.Before(s.now()) {
		return nil, ErrInvalidCredentials
	}

	return claims, nil
}

// RequestMeta captures HTTP metadata for session logging.
type RequestMeta struct {
	UserAgent string
	IP        string
}

// ProfileUpdateInput for updating profile fields.
type ProfileUpdateInput struct {
	FullName  *string
	Phone     *string
	AvatarURL *string
}

// PreferencesUpdateInput for updating preferences.
type PreferencesUpdateInput struct {
	Theme         *string
	Language      *string
	Notifications *NotificationPreferences
}

// SecurityUpdateInput toggles 2FA configuration.
type SecurityUpdateInput struct {
	EnableTwoFactor  bool
	DisableTwoFactor bool
}

func (s *Service) issueSession(ctx context.Context, user *User, meta RequestMeta) (*AuthResult, error) {
	sessionID := uuid.New()
	refreshToken := uuid.NewString()
	now := s.now()

	session := &Session{
		ID:           sessionID,
		UserID:       user.ID,
		RefreshToken: refreshToken,
		UserAgent:    meta.UserAgent,
		IP:           meta.IP,
		ExpiresAt:    now.Add(s.authCfg.RefreshTokenTTL),
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.repo.CreateSession(ctx, session); err != nil {
		return nil, err
	}

	accessToken, err := s.tokenSigner.GenerateAccessToken(&TokenPayload{
		SessionID:   sessionID,
		UserID:      user.ID,
		TenantID:    user.TenantID,
		Roles:       user.Roles,
		Permissions: user.Permissions,
	})
	if err != nil {
		return nil, err
	}

	return &AuthResult{
		Session: SessionTokens{
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
			ExpiresAt:    now.Add(s.authCfg.AccessTokenTTL),
			SessionID:    sessionID,
		},
		User: user,
	}, nil
}

func (s *Service) pickDemoUserByRole(ctx context.Context, role Role) (*User, error) {
	users, err := s.repo.ListUsers(ctx)
	if err != nil {
		return nil, err
	}

	for _, user := range users {
		if role == "" || user.HasRole(role) {
			return user, nil
		}
	}

	return nil, ErrUserNotFound
}

func (s *Service) createGoogleUser(ctx context.Context, profile googleProfile, role Role) (*User, error) {
	now := s.now()
	if role == "" {
		role = RoleCustomer
	}

	user := &User{
		ID:               uuid.New(),
		TenantID:         "urban-cafe",
		Email:            strings.ToLower(profile.Email),
		FullName:         profile.Name,
		AvatarURL:        profile.Picture,
		Roles:            []Role{role},
		Permissions:      DefaultPermissions(role),
		LoyaltyPoints:    150,
		AvailableCoupons: 2,
		Preferences: Preferences{
			Theme:    "system",
			Language: "en",
			Notifications: NotificationPreferences{
				Email: true,
				SMS:   false,
				Push:  true,
			},
		},
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

func (s *Service) seedDemoData(ctx context.Context) error {
	users := []*User{}

	demoUsers := []struct {
		email    string
		password string
		role     Role
		fullName string
	}{
		{"customer@demo.com", "demo1234", RoleCustomer, "Urban Café Guest"},
		{"rider@demo.com", "demo1234", RoleRider, "Swift Rider"},
		{"staff@demo.com", "demo1234", RoleStaff, "Cafe Staff"},
		{"admin@demo.com", "demo1234", RoleAdmin, "Cafe Admin"},
	}

	now := s.now()

	for _, demo := range demoUsers {
		hash, err := bcrypt.GenerateFromPassword([]byte(demo.password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}

		userID := uuid.New()
		user := &User{
			ID:                   userID,
			TenantID:             "urban-cafe",
			Email:                strings.ToLower(demo.email),
			PasswordHash:         string(hash),
			FullName:             demo.fullName,
			Roles:                []Role{demo.role},
			Permissions:          DefaultPermissions(demo.role),
			LoyaltyPoints:        870,
			AvailableCoupons:     3,
			DefaultLocationLabel: "Busia township",
			Preferences: Preferences{
				Theme:    "system",
				Language: "en",
				Notifications: NotificationPreferences{
					Email: true,
					SMS:   true,
					Push:  true,
				},
			},
			Status:    "active",
			CreatedAt: now.Add(-24 * time.Hour),
			UpdatedAt: now,
		}

		users = append(users, user)
	}

	orders := []*OrderSummary{}
	for _, user := range users {
		if user.HasRole(RoleCustomer) {
			orderID := uuid.New()
			eta := now.Add(45 * time.Minute)
			orders = append(orders, &OrderSummary{
				ID:       orderID,
				UserID:   user.ID,
				Status:   "delivered",
				Total:    1450,
				PlacedAt: now.Add(-6 * time.Hour),
				ETA:      &eta,
			})
		}
	}

	return s.repo.Seed(ctx, users, nil, orders)
}

// googleProfile minimal userinfo payload.
type googleProfile struct {
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

func fetchGoogleProfile(client *http.Client) (googleProfile, error) {
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return googleProfile{}, fmt.Errorf("identity: google userinfo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return googleProfile{}, fmt.Errorf("identity: google userinfo status %d", resp.StatusCode)
	}

	var profile googleProfile
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return googleProfile{}, fmt.Errorf("identity: google userinfo decode: %w", err)
	}

	return profile, nil
}
