package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

func TestAuthServiceClient_Login(t *testing.T) {
	tests := []struct {
		name           string
		request        AuthServiceLoginRequest
		serverResponse func() (int, interface{})
		wantErr        bool
		validateResult func(t *testing.T, resp *AuthServiceResponse, err error)
	}{
		{
			name: "successful login",
			request: AuthServiceLoginRequest{
				Email:      "user@example.com",
				Password:   "password123",
				TenantSlug: "cafe",
			},
			serverResponse: func() (int, interface{}) {
				return http.StatusOK, AuthServiceResponse{
					AccessToken:  "access_token_123",
					RefreshToken: "refresh_token_123",
					SessionID:    uuid.New().String(),
					ExpiresIn:    3600,
					User: map[string]interface{}{
						"id":        uuid.New().String(),
						"email":     "user@example.com",
						"full_name": "Test User",
						"status":    "active",
					},
					Tenant: map[string]interface{}{
						"id":   uuid.New().String(),
						"slug": "urban-cafe",
					},
				}
			},
			wantErr: false,
			validateResult: func(t *testing.T, resp *AuthServiceResponse, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if resp.AccessToken != "access_token_123" {
					t.Errorf("expected access_token_123, got %s", resp.AccessToken)
				}
				if resp.User["email"] != "user@example.com" {
					t.Errorf("expected user@example.com, got %v", resp.User["email"])
				}
			},
		},
		{
			name: "invalid credentials",
			request: AuthServiceLoginRequest{
				Email:      "user@example.com",
				Password:   "wrongpassword",
				TenantSlug: "urban-cafe",
			},
			serverResponse: func() (int, interface{}) {
				return http.StatusUnauthorized, AuthServiceError{
					ErrorField:       "invalid_credentials",
					ErrorDescription: "Invalid email or password",
				}
			},
			wantErr: true,
			validateResult: func(t *testing.T, resp *AuthServiceResponse, err error) {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if authErr, ok := err.(*AuthServiceError); !ok {
					t.Fatalf("expected AuthServiceError, got %T", err)
				} else if authErr.ErrorField != "invalid_credentials" {
					t.Errorf("expected invalid_credentials, got %s", authErr.ErrorField)
				}
			},
		},
		{
			name: "missing tenant_slug",
			request: AuthServiceLoginRequest{
				Email:    "user@example.com",
				Password: "password123",
			},
			serverResponse: func() (int, interface{}) {
				return http.StatusBadRequest, AuthServiceError{
					ErrorField:       "validation_error",
					ErrorDescription: "tenant_slug is required",
				}
			},
			wantErr: true,
			validateResult: func(t *testing.T, resp *AuthServiceResponse, err error) {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}
				if r.URL.Path != "/api/v1/auth/login" {
					t.Errorf("expected /api/v1/auth/login, got %s", r.URL.Path)
				}

				var req AuthServiceLoginRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Errorf("failed to decode request: %v", err)
				}

				status, response := tt.serverResponse()
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				if err := json.NewEncoder(w).Encode(response); err != nil {
					t.Errorf("failed to encode response: %v", err)
				}
			}))
			defer server.Close()

			// Create client
			logger := zap.NewNop()
			client := NewAuthServiceClient(server.URL, logger)

			// Execute
			ctx := context.Background()
			resp, err := client.Login(ctx, tt.request)

			// Validate
			if (err != nil) != tt.wantErr {
				t.Errorf("Login() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.validateResult != nil {
				tt.validateResult(t, resp, err)
			}
		})
	}
}

func TestAuthServiceClient_Register(t *testing.T) {
	tests := []struct {
		name           string
		request        AuthServiceRegisterRequest
		serverResponse func() (int, interface{})
		wantErr        bool
		validateResult func(t *testing.T, resp *AuthServiceResponse, err error)
	}{
		{
			name: "successful registration",
			request: AuthServiceRegisterRequest{
				Email:      "newuser@example.com",
				Password:   "password123",
				TenantSlug: "urban-cafe",
				Profile: map[string]interface{}{
					"full_name": "New User",
				},
			},
			serverResponse: func() (int, interface{}) {
				userID := uuid.New()
				return http.StatusCreated, AuthServiceResponse{
					AccessToken:  "access_token_123",
					RefreshToken: "refresh_token_123",
					SessionID:    uuid.New().String(),
					ExpiresIn:    3600,
					User: map[string]interface{}{
						"id":        userID.String(),
						"email":     "newuser@example.com",
						"full_name": "New User",
						"status":    "active",
					},
					Tenant: map[string]interface{}{
						"id":   uuid.New().String(),
						"slug": "urban-cafe",
					},
				}
			},
			wantErr: false,
			validateResult: func(t *testing.T, resp *AuthServiceResponse, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if resp.User["email"] != "newuser@example.com" {
					t.Errorf("expected newuser@example.com, got %v", resp.User["email"])
				}
			},
		},
		{
			name: "duplicate email",
			request: AuthServiceRegisterRequest{
				Email:      "existing@example.com",
				Password:   "password123",
				TenantSlug: "urban-cafe",
			},
			serverResponse: func() (int, interface{}) {
				return http.StatusConflict, AuthServiceError{
					ErrorField:       "duplicate_email",
					ErrorDescription: "Email already registered",
				}
			},
			wantErr: true,
			validateResult: func(t *testing.T, resp *AuthServiceResponse, err error) {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/auth/register" {
					t.Errorf("expected /api/v1/auth/register, got %s", r.URL.Path)
				}

				status, response := tt.serverResponse()
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				if err := json.NewEncoder(w).Encode(response); err != nil {
					t.Errorf("failed to encode response: %v", err)
				}
			}))
			defer server.Close()

			// Create client
			logger := zap.NewNop()
			client := NewAuthServiceClient(server.URL, logger)

			// Execute
			ctx := context.Background()
			resp, err := client.Register(ctx, tt.request)

			// Validate
			if (err != nil) != tt.wantErr {
				t.Errorf("Register() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.validateResult != nil {
				tt.validateResult(t, resp, err)
			}
		})
	}
}

func TestAuthServiceClient_Refresh(t *testing.T) {
	tests := []struct {
		name           string
		refreshToken   string
		serverResponse func() (int, interface{})
		wantErr        bool
	}{
		{
			name:         "successful refresh",
			refreshToken: "valid_refresh_token",
			serverResponse: func() (int, interface{}) {
				return http.StatusOK, AuthServiceResponse{
					AccessToken:  "new_access_token",
					RefreshToken: "new_refresh_token",
					SessionID:    uuid.New().String(),
					ExpiresIn:    3600,
				}
			},
			wantErr: false,
		},
		{
			name:         "invalid refresh token",
			refreshToken: "invalid_token",
			serverResponse: func() (int, interface{}) {
				return http.StatusUnauthorized, AuthServiceError{
					ErrorField:       "invalid_token",
					ErrorDescription: "Refresh token is invalid or expired",
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/auth/refresh" {
					t.Errorf("expected /api/v1/auth/refresh, got %s", r.URL.Path)
				}

				status, response := tt.serverResponse()
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				if err := json.NewEncoder(w).Encode(response); err != nil {
					t.Errorf("failed to encode response: %v", err)
				}
			}))
			defer server.Close()

			// Create client
			logger := zap.NewNop()
			client := NewAuthServiceClient(server.URL, logger)

			// Execute
			ctx := context.Background()
			resp, err := client.Refresh(ctx, tt.refreshToken)

			// Validate
			if (err != nil) != tt.wantErr {
				t.Errorf("Refresh() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && resp == nil {
				t.Error("expected response, got nil")
			}
		})
	}
}

func TestAuthServiceClient_GetUser(t *testing.T) {
	tests := []struct {
		name           string
		userID         string
		accessToken    string
		serverResponse func() (int, interface{})
		wantErr        bool
	}{
		{
			name:        "successful get user",
			userID:      uuid.New().String(),
			accessToken: "valid_token",
			serverResponse: func() (int, interface{}) {
				return http.StatusOK, map[string]interface{}{
					"id":        uuid.New().String(),
					"email":     "user@example.com",
					"full_name": "Test User",
					"status":    "active",
				}
			},
			wantErr: false,
		},
		{
			name:        "user not found",
			userID:      uuid.New().String(),
			accessToken: "valid_token",
			serverResponse: func() (int, interface{}) {
				return http.StatusNotFound, AuthServiceError{
					ErrorField:       "user_not_found",
					ErrorDescription: "User does not exist",
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				expectedPath := "/api/v1/users/" + tt.userID
				if r.URL.Path != expectedPath {
					t.Errorf("expected %s, got %s", expectedPath, r.URL.Path)
				}
				if auth := r.Header.Get("Authorization"); auth != "Bearer "+tt.accessToken {
					t.Errorf("expected Bearer token, got %s", auth)
				}

				status, response := tt.serverResponse()
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				if err := json.NewEncoder(w).Encode(response); err != nil {
					t.Errorf("failed to encode response: %v", err)
				}
			}))
			defer server.Close()

			// Create client
			logger := zap.NewNop()
			client := NewAuthServiceClient(server.URL, logger)

			// Execute
			ctx := context.Background()
			user, err := client.GetUser(ctx, tt.userID, tt.accessToken)

			// Validate
			if (err != nil) != tt.wantErr {
				t.Errorf("GetUser() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && user == nil {
				t.Error("expected user data, got nil")
			}
		})
	}
}
