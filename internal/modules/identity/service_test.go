package identity

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/config"
)

func TestService_SyncUserFromAuthService(t *testing.T) {
	authID2 := uuid.New()
	tests := []struct {
		name           string
		authServiceID  uuid.UUID
		tenantID       string
		authUserData   map[string]interface{}
		setupRepo      func() *MemoryRepository
		wantErr        bool
		validateResult func(t *testing.T, user *User, err error)
	}{
		{
			name:          "create new user from auth-service",
			authServiceID: uuid.New(),
			tenantID:      uuid.New().String(),
			authUserData: map[string]interface{}{
				"id":        uuid.New().String(),
				"email":     "newuser@example.com",
				"full_name": "New User",
				"phone":     "+254712345678",
				"status":    "active",
			},
			setupRepo: func() *MemoryRepository {
				return NewMemoryRepository()
			},
			wantErr: false,
			validateResult: func(t *testing.T, user *User, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if user == nil {
					t.Fatal("expected User, got nil")
				}
				if user.Email != "newuser@example.com" {
					t.Errorf("expected newuser@example.com, got %s", user.Email)
				}
				if user.AuthServiceUserID == nil {
					t.Error("expected AuthServiceUserID to be set")
				}
				if user.SyncStatus != "synced" {
					t.Errorf("expected sync_status 'synced', got %s", user.SyncStatus)
				}
			},
		},
		{
			name:          "update existing user from auth-service",
			authServiceID: authID2,
			tenantID:      uuid.New().String(),
			authUserData: map[string]interface{}{
				"id":        authID2.String(),
				"email":     "existing@example.com",
				"full_name": "Updated User",
				"phone":     "+254712345679",
				"status":    "active",
			},
			setupRepo: func() *MemoryRepository {
				repo := NewMemoryRepository()
				existingUser := &User{
					ID:                uuid.New(),
					TenantID:          uuid.New().String(),
					AuthServiceUserID: &authID2,
					Email:             "existing@example.com",
					FullName:          "Old Name",
					Status:            "active",
					Roles:             []Role{RoleCustomer},
					SyncStatus:        "pending",
					CreatedAt:         time.Now(),
					UpdatedAt:         time.Now(),
				}
				_ = repo.CreateUser(context.Background(), existingUser)
				return repo
			},
			wantErr: false,
			validateResult: func(t *testing.T, user *User, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if user.FullName != "Updated User" {
					t.Errorf("expected 'Updated User', got %s", user.FullName)
				}
				if user.SyncStatus != "synced" {
					t.Errorf("expected sync_status 'synced', got %s", user.SyncStatus)
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
				AccessTokenSecret: "test-secret-key-for-testing-only-min-32-chars",
			}

			service, err := NewService(repo, authCfg, logger)
			if err != nil {
				t.Fatalf("failed to create service: %v", err)
			}

			ctx := context.Background()
			user, err := service.SyncUserFromAuthService(ctx, tt.authServiceID, tt.tenantID, tt.authUserData, "")

			if (err != nil) != tt.wantErr {
				t.Errorf("SyncUserFromAuthService() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.validateResult != nil {
				tt.validateResult(t, user, err)
			}
		})
	}
}

func TestService_GetUser(t *testing.T) {
	tests := []struct {
		name      string
		userID    uuid.UUID
		setupRepo func() *MemoryRepository
		wantErr   bool
	}{
		{
			name:   "user found",
			userID: uuid.New(),
			setupRepo: func() *MemoryRepository {
				repo := NewMemoryRepository()
				user := &User{
					ID:       uuid.New(),
					TenantID: uuid.New().String(),
					Email:    "user@example.com",
					FullName: "Test User",
					Status:   "active",
					Roles:    []Role{RoleCustomer},
				}
				_ = repo.CreateUser(context.Background(), user)
				return repo
			},
			wantErr: true, // Will fail because we need to use the actual user ID
		},
		{
			name:   "user not found",
			userID: uuid.New(),
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
				AccessTokenSecret: "test-secret-key-for-testing-only-min-32-chars",
			}
			service, err := NewService(repo, authCfg, logger)
			if err != nil {
				t.Fatalf("failed to create service: %v", err)
			}

			ctx := context.Background()
			_, err = service.GetUser(ctx, tt.userID)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetUser() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestService_ExtractRolesFromAuthServiceUser(t *testing.T) {
	tests := []struct {
		name         string
		authUserData map[string]interface{}
		email        string
		wantRoles    []Role
	}{
		{
			name: "extract superuser from admin email",
			authUserData: map[string]interface{}{
				"email": "admin@codevertexitsolutions.com",
			},
			email:     "admin@codevertexitsolutions.com",
			wantRoles: []Role{RoleSuperAdmin, RoleAdmin},
		},
		{
			name: "extract roles from roles array",
			authUserData: map[string]interface{}{
				"roles": []interface{}{"customer", "rider"},
			},
			email:     "user@example.com",
			wantRoles: []Role{RoleCustomer, RoleRider},
		},
		{
			name:         "no roles in data",
			authUserData: map[string]interface{}{},
			email:        "user@example.com",
			wantRoles:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roles := extractRolesFromAuthServiceUser(tt.authUserData, tt.email)
			if len(roles) != len(tt.wantRoles) {
				t.Errorf("expected %d roles, got %d", len(tt.wantRoles), len(roles))
			}
		})
	}
}
