package identity

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/config"
)

func TestEventHandler_HandleAuthUserCreated(t *testing.T) {
	tests := []struct {
		name           string
		event          *AuthUserCreatedEvent
		setupRepo      func() *MemoryRepository
		wantErr        bool
		validateResult func(t *testing.T, repo *MemoryRepository, err error)
	}{
		{
			name: "successful user creation from event",
			event: &AuthUserCreatedEvent{
				UserID:    uuid.New().String(),
				TenantID:  uuid.New().String(),
				Email:     "newuser@example.com",
				FullName:  "New User",
				Phone:     "+254712345678",
				Status:    "active",
				CreatedAt: time.Now(),
			},
			setupRepo: func() *MemoryRepository {
				return NewMemoryRepository()
			},
			wantErr: false,
			validateResult: func(t *testing.T, repo *MemoryRepository, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				// Verify user was created by finding by email
				user, findErr := repo.FindUserByEmail(context.Background(), "newuser@example.com")
				if findErr != nil {
					t.Fatalf("failed to find created user: %v", findErr)
				}
				if user.Email != "newuser@example.com" {
					t.Errorf("expected newuser@example.com, got %s", user.Email)
				}
			},
		},
		{
			name: "invalid user ID in event",
			event: &AuthUserCreatedEvent{
				UserID:   "invalid-uuid",
				TenantID: uuid.New().String(),
				Email:    "user@example.com",
				FullName: "Test User",
				Status:   "active",
			},
			setupRepo: func() *MemoryRepository {
				return NewMemoryRepository()
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.setupRepo()
			logger := zap.NewNop()

			authCfg := config.AuthConfig{
				ServiceURL:        "https://sso.codevertexitsolutions.com",
				Issuer:            "https://auth.bengobox.local",
				Audience:          "urban-cafe",
				AccessTokenSecret: "test-secret-key-for-testing-only-min-32-chars",
			}

			service, err := NewService(repo, authCfg, logger, nil)
			if err != nil {
				t.Fatalf("failed to create service: %v", err)
			}

			handler := NewEventHandler(service, logger)

			ctx := context.Background()
			err = handler.HandleAuthUserCreated(ctx, tt.event)

			if (err != nil) != tt.wantErr {
				t.Errorf("HandleAuthUserCreated() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.validateResult != nil {
				tt.validateResult(t, repo, err)
			}
		})
	}
}

func TestEventHandler_HandleAuthUserUpdated(t *testing.T) {
	authID := uuid.New()
	tests := []struct {
		name           string
		event          *AuthUserUpdatedEvent
		setupRepo      func() *MemoryRepository
		wantErr        bool
		validateResult func(t *testing.T, repo *MemoryRepository, eventUserID string, err error)
	}{
		{
			name: "successful user update from event",
			event: &AuthUserUpdatedEvent{
				UserID:    authID.String(),
				Email:     "updated@example.com",
				FullName:  "Updated User",
				Status:    "active",
				UpdatedAt: time.Now(),
			},
			setupRepo: func() *MemoryRepository {
				repo := NewMemoryRepository()
				authIDCopy := authID // Capture the closure variable
				user := &User{
					ID:                uuid.New(),
					TenantID:          uuid.New().String(),
					AuthServiceUserID: &authIDCopy,
					Email:             "old@example.com",
					FullName:          "Old User",
					Status:            "active",
					Roles:             []Role{RoleCustomer},
					SyncStatus:        "synced",
					CreatedAt:         time.Now(),
					UpdatedAt:         time.Now(),
				}
				_ = repo.CreateUser(context.Background(), user)
				return repo
			},
			wantErr: false,
			validateResult: func(t *testing.T, repo *MemoryRepository, eventUserID string, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				// Verify user was updated by finding by auth-service ID
				authIDParsed, _ := uuid.Parse(eventUserID)
				user, findErr := repo.FindUserByAuthServiceID(context.Background(), authIDParsed)
				if findErr != nil {
					t.Fatalf("failed to find updated user: %v", findErr)
				}
				if user.FullName != "Updated User" {
					t.Errorf("expected 'Updated User', got %s", user.FullName)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.setupRepo()
			logger := zap.NewNop()

			authCfg := config.AuthConfig{
				ServiceURL:        "https://sso.codevertexitsolutions.com",
				Issuer:            "https://auth.bengobox.local",
				Audience:          "urban-cafe",
				AccessTokenSecret: "test-secret-key-for-testing-only-min-32-chars",
			}

			service, err := NewService(repo, authCfg, logger, nil)
			if err != nil {
				t.Fatalf("failed to create service: %v", err)
			}

			handler := NewEventHandler(service, logger)

			ctx := context.Background()
			err = handler.HandleAuthUserUpdated(ctx, tt.event)

			if (err != nil) != tt.wantErr {
				t.Errorf("HandleAuthUserUpdated() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.validateResult != nil {
				tt.validateResult(t, repo, tt.event.UserID, err)
			}
		})
	}
}

func TestEventHandler_HandleAuthUserDeactivated(t *testing.T) {
	authID := uuid.New()
	tests := []struct {
		name           string
		event          *AuthUserDeactivatedEvent
		setupRepo      func() *MemoryRepository
		wantErr        bool
		validateResult func(t *testing.T, repo *MemoryRepository, err error)
	}{
		{
			name: "successful user deactivation from event",
			event: &AuthUserDeactivatedEvent{
				UserID:        authID.String(),
				DeactivatedAt: time.Now(),
			},
			setupRepo: func() *MemoryRepository {
				repo := NewMemoryRepository()
				user := &User{
					ID:                uuid.New(),
					TenantID:          uuid.New().String(),
					AuthServiceUserID: &authID,
					Email:             "user@example.com",
					FullName:          "Test User",
					Status:            "active",
					Roles:             []Role{RoleCustomer},
					CreatedAt:         time.Now(),
					UpdatedAt:         time.Now(),
				}
				_ = repo.CreateUser(context.Background(), user)
				return repo
			},
			wantErr: false,
			validateResult: func(t *testing.T, repo *MemoryRepository, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				// Verify user was deactivated by finding by auth-service ID
				authIDParsed, _ := uuid.Parse(authID.String())
				user, findErr := repo.FindUserByAuthServiceID(context.Background(), authIDParsed)
				if findErr != nil {
					t.Fatalf("failed to find deactivated user: %v", findErr)
				}
				if user.Status != "deactivated" {
					t.Errorf("expected status 'deactivated', got %s", user.Status)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.setupRepo()
			logger := zap.NewNop()

			authCfg := config.AuthConfig{
				ServiceURL:        "https://sso.codevertexitsolutions.com",
				Issuer:            "https://auth.bengobox.local",
				Audience:          "urban-cafe",
				AccessTokenSecret: "test-secret-key-for-testing-only-min-32-chars",
			}

			service, err := NewService(repo, authCfg, logger, nil)
			if err != nil {
				t.Fatalf("failed to create service: %v", err)
			}

			handler := NewEventHandler(service, logger)

			ctx := context.Background()
			err = handler.HandleAuthUserDeactivated(ctx, tt.event)

			if (err != nil) != tt.wantErr {
				t.Errorf("HandleAuthUserDeactivated() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.validateResult != nil {
				tt.validateResult(t, repo, err)
			}
		})
	}
}

func TestEventHandler_EventJSONUnmarshal(t *testing.T) {
	tests := []struct {
		name      string
		jsonData  string
		eventType string
		wantErr   bool
	}{
		{
			name:      "valid user created event",
			eventType: "auth.user.created",
			jsonData: `{
				"user_id": "` + uuid.New().String() + `",
				"tenant_id": "` + uuid.New().String() + `",
				"email": "user@example.com",
				"full_name": "Test User",
				"status": "active",
				"created_at": "2024-01-01T00:00:00Z"
			}`,
			wantErr: false,
		},
		{
			name:      "valid user updated event",
			eventType: "auth.user.updated",
			jsonData: `{
				"user_id": "` + uuid.New().String() + `",
				"email": "updated@example.com",
				"full_name": "Updated User",
				"updated_at": "2024-01-01T00:00:00Z"
			}`,
			wantErr: false,
		},
		{
			name:      "valid user deactivated event",
			eventType: "auth.user.deactivated",
			jsonData: `{
				"user_id": "` + uuid.New().String() + `",
				"deactivated_at": "2024-01-01T00:00:00Z"
			}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			switch tt.eventType {
			case "auth.user.created":
				var event AuthUserCreatedEvent
				err = json.Unmarshal([]byte(tt.jsonData), &event)
			case "auth.user.updated":
				var event AuthUserUpdatedEvent
				err = json.Unmarshal([]byte(tt.jsonData), &event)
			case "auth.user.deactivated":
				var event AuthUserDeactivatedEvent
				err = json.Unmarshal([]byte(tt.jsonData), &event)
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("JSON unmarshal error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
