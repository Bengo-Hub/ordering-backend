package identity

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMemoryRepository_FindUserByAuthServiceID(t *testing.T) {
	authID := uuid.New()
	tests := []struct {
		name           string
		authServiceID  uuid.UUID
		setupRepo      func() *MemoryRepository
		wantErr        bool
		validateResult func(t *testing.T, user *User, err error)
	}{
		{
			name:          "user found by auth-service ID",
			authServiceID: authID,
			setupRepo: func() *MemoryRepository {
				repo := NewMemoryRepository()
				authIDCopy := authID // Capture the closure variable
				user := &User{
					ID:                uuid.New(),
					TenantID:          uuid.New().String(),
					AuthServiceUserID: &authIDCopy,
					Email:             "user@example.com",
					FullName:          "Test User",
					Status:            "active",
					Roles:             []Role{RoleCustomer},
					SyncStatus:        "synced",
					SyncAt:            timePtr(time.Now()),
					CreatedAt:         time.Now(),
					UpdatedAt:         time.Now(),
				}
				_ = repo.CreateUser(context.Background(), user)
				return repo
			},
			wantErr: false,
			validateResult: func(t *testing.T, user *User, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if user == nil {
					t.Fatal("expected User, got nil")
				}
				if user.AuthServiceUserID == nil {
					t.Error("expected AuthServiceUserID to be set")
				}
			},
		},
		{
			name:          "user not found",
			authServiceID: uuid.New(),
			setupRepo: func() *MemoryRepository {
				return NewMemoryRepository()
			},
			wantErr: true,
			validateResult: func(t *testing.T, user *User, err error) {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if user != nil {
					t.Error("expected nil user, got user")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.setupRepo()

			// Update MemoryRepository to support FindUserByAuthServiceID
			// For now, we'll need to add this method
			ctx := context.Background()

			// Use the actual FindUserByAuthServiceID method
			foundUser, foundErr := repo.FindUserByAuthServiceID(ctx, tt.authServiceID)

			if (foundErr != nil) != tt.wantErr {
				t.Errorf("FindUserByAuthServiceID() error = %v, wantErr %v", foundErr, tt.wantErr)
				return
			}

			if tt.validateResult != nil {
				tt.validateResult(t, foundUser, foundErr)
			}
		})
	}
}

func TestMemoryRepository_CreateUser_WithAuthServiceID(t *testing.T) {
	tests := []struct {
		name           string
		user           *User
		wantErr        bool
		validateResult func(t *testing.T, err error)
	}{
		{
			name: "create user with auth-service ID",
			user: &User{
				ID:                uuid.New(),
				TenantID:          uuid.New().String(),
				AuthServiceUserID: uuidPtr(uuid.New()),
				Email:             "user@example.com",
				FullName:          "Test User",
				Status:            "active",
				Roles:             []Role{RoleCustomer},
				SyncStatus:        "synced",
				SyncAt:            timePtr(time.Now()),
				CreatedAt:         time.Now(),
				UpdatedAt:         time.Now(),
			},
			wantErr: false,
			validateResult: func(t *testing.T, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			},
		},
		{
			name: "create user without auth-service ID",
			user: &User{
				ID:        uuid.New(),
				TenantID:  uuid.New().String(),
				Email:     "user2@example.com",
				FullName:  "Test User 2",
				Status:    "active",
				Roles:     []Role{RoleCustomer},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			wantErr: false,
			validateResult: func(t *testing.T, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewMemoryRepository()
			ctx := context.Background()

			err := repo.CreateUser(ctx, tt.user)

			if (err != nil) != tt.wantErr {
				t.Errorf("CreateUser() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.validateResult != nil {
				tt.validateResult(t, err)
			}

			// Verify user was created
			if err == nil {
				found, err := repo.FindUserByID(ctx, tt.user.ID)
				if err != nil {
					t.Fatalf("failed to find created user: %v", err)
				}
				if found.Email != tt.user.Email {
					t.Errorf("expected email %s, got %s", tt.user.Email, found.Email)
				}
			}
		})
	}
}

func TestMemoryRepository_UpdateUser_WithSyncFields(t *testing.T) {
	tests := []struct {
		name           string
		initialUser    *User
		updatedUser    *User
		wantErr        bool
		validateResult func(t *testing.T, user *User, err error)
	}{
		{
			name: "update sync status and sync_at",
			initialUser: &User{
				ID:                uuid.New(),
				TenantID:          uuid.New().String(),
				AuthServiceUserID: uuidPtr(uuid.New()),
				Email:             "user@example.com",
				FullName:          "Test User",
				Status:            "active",
				Roles:             []Role{RoleCustomer},
				SyncStatus:        "pending",
				CreatedAt:         time.Now(),
				UpdatedAt:         time.Now(),
			},
			updatedUser: &User{
				ID:                uuid.New(), // Will be set from initial
				TenantID:          uuid.New().String(),
				AuthServiceUserID: uuidPtr(uuid.New()),
				Email:             "user@example.com",
				FullName:          "Updated User",
				Status:            "active",
				Roles:             []Role{RoleCustomer},
				SyncStatus:        "synced",
				SyncAt:            timePtr(time.Now()),
				CreatedAt:         time.Now(),
				UpdatedAt:         time.Now(),
			},
			wantErr: false,
			validateResult: func(t *testing.T, user *User, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if user.SyncStatus != "synced" {
					t.Errorf("expected sync_status 'synced', got %s", user.SyncStatus)
				}
				if user.SyncAt == nil {
					t.Error("expected SyncAt to be set")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewMemoryRepository()
			ctx := context.Background()

			// Create initial user
			tt.updatedUser.ID = tt.initialUser.ID
			tt.updatedUser.TenantID = tt.initialUser.TenantID
			if err := repo.CreateUser(ctx, tt.initialUser); err != nil {
				t.Fatalf("failed to create initial user: %v", err)
			}

			// Update user
			err := repo.UpdateUser(ctx, tt.updatedUser)

			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateUser() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Verify update
			if err == nil {
				updated, err := repo.FindUserByID(ctx, tt.initialUser.ID)
				if err != nil {
					t.Fatalf("failed to find updated user: %v", err)
				}
				if tt.validateResult != nil {
					tt.validateResult(t, updated, nil)
				}
			}
		})
	}
}

// Helper functions
func uuidPtr(u uuid.UUID) *uuid.UUID {
	return &u
}

func timePtr(t time.Time) *time.Time {
	return &t
}
