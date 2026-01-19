package superset

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/bengobox/ordering-backend/internal/config"
)

func TestClient_Login(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		wantErr        bool
		errContains    string
	}{
		{
			name: "successful login",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/api/v1/security/login", r.URL.Path)
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				// Verify request body
				var req map[string]interface{}
				err := json.NewDecoder(r.Body).Decode(&req)
				require.NoError(t, err)
				assert.Equal(t, "testuser", req["username"])
				assert.Equal(t, "testpass", req["password"])
				assert.Equal(t, "db", req["provider"])
				assert.Equal(t, true, req["refresh"])

				// Send successful response
				w.WriteHeader(http.StatusOK)
				resp := LoginResponse{
					AccessToken:  "test-access-token",
					RefreshToken: "test-refresh-token",
				}
				json.NewEncoder(w).Encode(resp)
			},
			wantErr: false,
		},
		{
			name: "invalid credentials",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"message": "Invalid credentials"}`))
			},
			wantErr:     true,
			errContains: "login failed with status 401",
		},
		{
			name: "server error",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"message": "Internal server error"}`))
			},
			wantErr:     true,
			errContains: "login failed with status 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			client := NewClient(config.SupersetConfig{
				BaseURL:              server.URL,
				APIVersion:           "v1",
				AdminUsername:        "testuser",
				AdminPassword:        "testpass",
				RequestTimeout:       30 * time.Second,
				GuestTokenTTLMinutes: 5,
			}, zaptest.NewLogger(t))

			err := client.Login(context.Background())

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, "test-access-token", client.getAccessToken())
			}
		})
	}
}

func TestClient_GenerateGuestToken(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	dashboardID := 123

	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		wantErr        bool
		errContains    string
		wantToken      string
	}{
		{
			name: "successful guest token generation",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/security/login" {
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(LoginResponse{
						AccessToken: "test-token",
					})
					return
				}

				if r.URL.Path == "/api/v1/security/guest_token/" {
					assert.Equal(t, http.MethodPost, r.Method)
					assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
					assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

					// Verify request body
					var req GuestTokenRequest
					err := json.NewDecoder(r.Body).Decode(&req)
					require.NoError(t, err)
					assert.Equal(t, userID.String(), req.User.Username)
					assert.Equal(t, "Guest", req.User.FirstName)
					assert.Equal(t, "User", req.User.LastName)
					assert.Len(t, req.Resources, 1)
					assert.Equal(t, "dashboard", req.Resources[0].Type)
					assert.Equal(t, dashboardID, req.Resources[0].ID)
					assert.Len(t, req.RLS, 1)
					assert.Contains(t, req.RLS[0].Clause, tenantID.String())

					// Send successful response
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(GuestTokenResponse{
						Token: "guest-token-123",
					})
					return
				}

				w.WriteHeader(http.StatusNotFound)
			},
			wantErr:   false,
			wantToken: "guest-token-123",
		},
		{
			name: "authentication failure",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			},
			wantErr:     true,
			errContains: "authentication failed",
		},
		{
			name: "guest token generation failure",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/security/login" {
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(LoginResponse{
						AccessToken: "test-token",
					})
					return
				}

				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"message": "Invalid dashboard ID"}`))
			},
			wantErr:     true,
			errContains: "guest token failed with status 400",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			client := NewClient(config.SupersetConfig{
				BaseURL:              server.URL,
				APIVersion:           "v1",
				AdminUsername:        "testuser",
				AdminPassword:        "testpass",
				RequestTimeout:       30 * time.Second,
				GuestTokenTTLMinutes: 5,
			}, zaptest.NewLogger(t))

			token, err := client.GenerateGuestToken(context.Background(), dashboardID, tenantID, userID)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantToken, token)
			}
		})
	}
}

func TestClient_GetDashboard(t *testing.T) {
	dashboardID := 123

	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		wantErr        bool
		errIs          error
		wantDashboard  *Dashboard
	}{
		{
			name: "successful dashboard retrieval",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/security/login" {
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(LoginResponse{
						AccessToken: "test-token",
					})
					return
				}

				if r.URL.Path == "/api/v1/dashboard/123" {
					assert.Equal(t, http.MethodGet, r.Method)
					assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(map[string]interface{}{
						"result": Dashboard{
							ID:              123,
							DashboardTitle:  "Test Dashboard",
							Slug:            "test-dashboard",
							URL:             "/superset/dashboard/test-dashboard/",
							PublishedStatus: true,
						},
					})
					return
				}

				w.WriteHeader(http.StatusNotFound)
			},
			wantErr: false,
			wantDashboard: &Dashboard{
				ID:              123,
				DashboardTitle:  "Test Dashboard",
				Slug:            "test-dashboard",
				URL:             "/superset/dashboard/test-dashboard/",
				PublishedStatus: true,
			},
		},
		{
			name: "dashboard not found",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/security/login" {
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(LoginResponse{
						AccessToken: "test-token",
					})
					return
				}

				w.WriteHeader(http.StatusNotFound)
			},
			wantErr: true,
			errIs:   ErrDashboardNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			client := NewClient(config.SupersetConfig{
				BaseURL:              server.URL,
				APIVersion:           "v1",
				AdminUsername:        "testuser",
				AdminPassword:        "testpass",
				RequestTimeout:       30 * time.Second,
				GuestTokenTTLMinutes: 5,
			}, zaptest.NewLogger(t))

			dashboard, err := client.GetDashboard(context.Background(), dashboardID)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errIs != nil {
					assert.ErrorIs(t, err, tt.errIs)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantDashboard, dashboard)
			}
		})
	}
}

func TestClient_ListDashboards(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		wantErr        bool
		wantCount      int
	}{
		{
			name: "successful dashboards list",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/security/login" {
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(LoginResponse{
						AccessToken: "test-token",
					})
					return
				}

				if r.URL.Path == "/api/v1/dashboard/" {
					assert.Equal(t, http.MethodGet, r.Method)
					assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(map[string]interface{}{
						"result": []Dashboard{
							{ID: 1, DashboardTitle: "Dashboard 1"},
							{ID: 2, DashboardTitle: "Dashboard 2"},
							{ID: 3, DashboardTitle: "Dashboard 3"},
						},
					})
					return
				}

				w.WriteHeader(http.StatusNotFound)
			},
			wantErr:   false,
			wantCount: 3,
		},
		{
			name: "empty dashboards list",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/security/login" {
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(LoginResponse{
						AccessToken: "test-token",
					})
					return
				}

				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"result": []Dashboard{},
				})
			},
			wantErr:   false,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			client := NewClient(config.SupersetConfig{
				BaseURL:              server.URL,
				APIVersion:           "v1",
				AdminUsername:        "testuser",
				AdminPassword:        "testpass",
				RequestTimeout:       30 * time.Second,
				GuestTokenTTLMinutes: 5,
			}, zaptest.NewLogger(t))

			dashboards, err := client.ListDashboards(context.Background())

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, dashboards, tt.wantCount)
			}
		})
	}
}

func TestClient_GetEmbedURL(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	dashboardID := 123

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/security/login" {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(LoginResponse{
				AccessToken: "test-token",
			})
			return
		}

		if r.URL.Path == "/api/v1/security/guest_token/" {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(GuestTokenResponse{
				Token: "guest-token-abc",
			})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(config.SupersetConfig{
		BaseURL:              server.URL,
		APIVersion:           "v1",
		AdminUsername:        "testuser",
		AdminPassword:        "testpass",
		RequestTimeout:       30 * time.Second,
		GuestTokenTTLMinutes: 5,
	}, zaptest.NewLogger(t))

	embedURL, err := client.GetEmbedURL(context.Background(), dashboardID, tenantID, userID)

	assert.NoError(t, err)
	assert.NotNil(t, embedURL)
	assert.Contains(t, embedURL.URL, server.URL)
	assert.Contains(t, embedURL.URL, "/superset/embedded/123/")
	assert.Contains(t, embedURL.URL, "guest_token=guest-token-abc")
	assert.Equal(t, "guest-token-abc", embedURL.Token)
	assert.True(t, embedURL.ExpiresAt.After(time.Now()))
}

func TestClient_EnsureAuthenticated(t *testing.T) {
	t.Run("reuses valid token", func(t *testing.T) {
		loginCalls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v1/security/login" {
				loginCalls++
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(LoginResponse{
					AccessToken: "test-token",
				})
				return
			}
		}))
		defer server.Close()

		client := NewClient(config.SupersetConfig{
			BaseURL:              server.URL,
			APIVersion:           "v1",
			AdminUsername:        "testuser",
			AdminPassword:        "testpass",
			RequestTimeout:       30 * time.Second,
			GuestTokenTTLMinutes: 5,
		}, zaptest.NewLogger(t))

		// First call should login
		err := client.ensureAuthenticated(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, 1, loginCalls)

		// Second call should reuse token
		err = client.ensureAuthenticated(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, 1, loginCalls)
	})

	t.Run("refreshes expired token", func(t *testing.T) {
		loginCalls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v1/security/login" {
				loginCalls++
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(LoginResponse{
					AccessToken: "test-token",
				})
				return
			}
		}))
		defer server.Close()

		client := NewClient(config.SupersetConfig{
			BaseURL:              server.URL,
			APIVersion:           "v1",
			AdminUsername:        "testuser",
			AdminPassword:        "testpass",
			RequestTimeout:       30 * time.Second,
			GuestTokenTTLMinutes: 5,
		}, zaptest.NewLogger(t))

		// First call should login
		err := client.ensureAuthenticated(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, 1, loginCalls)

		// Manually expire the token
		client.tokenMu.Lock()
		client.tokenExpiry = time.Now().Add(-1 * time.Minute)
		client.tokenMu.Unlock()

		// Second call should login again
		err = client.ensureAuthenticated(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, 2, loginCalls)
	})
}
