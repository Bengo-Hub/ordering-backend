package identity

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	sharedevents "github.com/Bengo-Hub/shared-events"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/config"
)

// makeUserEvent builds a sharedevents.Event that mimics an auth.user.* envelope.
func makeUserEvent(eventType string, tenantID uuid.UUID, payload map[string]interface{}) *sharedevents.Event {
	return &sharedevents.Event{
		ID:            uuid.New(),
		EventType:     eventType,
		AggregateType: "auth.user",
		AggregateID:   uuid.New(),
		TenantID:      tenantID,
		Payload:       payload,
		Timestamp:     time.Now().UTC(),
		Version:       "1.0",
	}
}

func TestEventHandler_HandleAuthUserCreated(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()

	tests := []struct {
		name           string
		evt            *sharedevents.Event
		setupRepo      func() *MemoryRepository
		wantErr        bool
		validateResult func(t *testing.T, repo *MemoryRepository, err error)
	}{
		{
			name: "successful user creation from event",
			evt: makeUserEvent("created", tenantID, map[string]interface{}{
				"user_id":   userID.String(),
				"email":     "newuser@example.com",
				"full_name": "New User",
				"phone":     "+254712345678",
				"status":    "active",
			}),
			setupRepo: func() *MemoryRepository { return NewMemoryRepository() },
			wantErr:   false,
			validateResult: func(t *testing.T, repo *MemoryRepository, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
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
			evt: makeUserEvent("created", tenantID, map[string]interface{}{
				"user_id":   "invalid-uuid",
				"email":     "user@example.com",
				"full_name": "Test User",
				"status":    "active",
			}),
			setupRepo: func() *MemoryRepository { return NewMemoryRepository() },
			wantErr:   true,
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
			err = handler.HandleAuthUserCreated(ctx, tt.evt)

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
	tenantID := uuid.New()

	tests := []struct {
		name           string
		evt            *sharedevents.Event
		setupRepo      func() *MemoryRepository
		wantErr        bool
		validateResult func(t *testing.T, repo *MemoryRepository, err error)
	}{
		{
			name: "successful user update from event",
			evt: makeUserEvent("updated", tenantID, map[string]interface{}{
				"user_id":   authID.String(),
				"email":     "updated@example.com",
				"full_name": "Updated User",
				"status":    "active",
			}),
			setupRepo: func() *MemoryRepository {
				repo := NewMemoryRepository()
				authIDCopy := authID
				user := &User{
					ID:                uuid.New(),
					TenantID:          tenantID.String(),
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
			validateResult: func(t *testing.T, repo *MemoryRepository, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				user, findErr := repo.FindUserByAuthServiceID(context.Background(), authID)
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
			err = handler.HandleAuthUserUpdated(ctx, tt.evt)

			if (err != nil) != tt.wantErr {
				t.Errorf("HandleAuthUserUpdated() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.validateResult != nil {
				tt.validateResult(t, repo, err)
			}
		})
	}
}

func TestEventHandler_HandleAuthUserDeactivated(t *testing.T) {
	authID := uuid.New()
	tenantID := uuid.New()

	tests := []struct {
		name           string
		evt            *sharedevents.Event
		setupRepo      func() *MemoryRepository
		wantErr        bool
		validateResult func(t *testing.T, repo *MemoryRepository, err error)
	}{
		{
			name: "successful user deactivation from event",
			evt: makeUserEvent("deactivated", tenantID, map[string]interface{}{
				"user_id": authID.String(),
			}),
			setupRepo: func() *MemoryRepository {
				repo := NewMemoryRepository()
				user := &User{
					ID:                uuid.New(),
					TenantID:          tenantID.String(),
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
				user, findErr := repo.FindUserByAuthServiceID(context.Background(), authID)
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
			err = handler.HandleAuthUserDeactivated(ctx, tt.evt)

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

func TestEventHandler_EventEnvelopeParsing(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()

	// Verify that sharedevents.FromJSON correctly parses the envelope and
	// that our handlers can extract fields from evt.Payload.
	tests := []struct {
		name      string
		jsonData  string
		eventType string
		wantErr   bool
	}{
		{
			name:      "valid user created envelope",
			eventType: "auth.user.created",
			jsonData: `{
				"id": "` + uuid.New().String() + `",
				"event_type": "created",
				"aggregate_type": "auth.user",
				"aggregate_id": "` + uuid.New().String() + `",
				"tenant_id": "` + tenantID.String() + `",
				"payload": {
					"user_id": "` + userID.String() + `",
					"email": "user@example.com",
					"full_name": "Test User",
					"status": "active"
				},
				"timestamp": "2024-01-01T00:00:00Z",
				"version": "1.0"
			}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evt, err := sharedevents.FromJSON([]byte(tt.jsonData))
			if (err != nil) != tt.wantErr {
				t.Errorf("FromJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				if evt.TenantID != tenantID {
					t.Errorf("expected tenant_id %s, got %s", tenantID, evt.TenantID)
				}
				if emailVal, _ := evt.Payload["email"].(string); emailVal != "user@example.com" {
					t.Errorf("expected email user@example.com in payload, got %q", emailVal)
				}
			}
		})
	}
}
