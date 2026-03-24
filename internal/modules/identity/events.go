package identity

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

// EventHandler handles events from auth-service for user synchronization.
type EventHandler struct {
	service *Service
	logger  *zap.Logger
}

// NewEventHandler creates a new event handler.
func NewEventHandler(service *Service, logger *zap.Logger) *EventHandler {
	return &EventHandler{
		service: service,
		logger:  logger.Named("identity.EventHandler"),
	}
}

// AuthUserCreatedEvent represents an auth.user.created event.
type AuthUserCreatedEvent struct {
	UserID      string                 `json:"user_id"`
	TenantID    string                 `json:"tenant_id"`
	Email       string                 `json:"email"`
	FullName    string                 `json:"full_name"`
	Phone       string                 `json:"phone,omitempty"`
	Status      string                 `json:"status"`
	Roles       []string               `json:"roles,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
}

// AuthUserUpdatedEvent represents an auth.user.updated event.
type AuthUserUpdatedEvent struct {
	UserID    string                 `json:"user_id"`
	TenantID  string                 `json:"tenant_id,omitempty"`
	Email     string                 `json:"email,omitempty"`
	FullName  string                 `json:"full_name,omitempty"`
	Phone     string                 `json:"phone,omitempty"`
	Status    string                 `json:"status,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// AuthUserDeactivatedEvent represents an auth.user.deactivated event.
type AuthUserDeactivatedEvent struct {
	UserID        string    `json:"user_id"`
	TenantID      string    `json:"tenant_id,omitempty"`
	DeactivatedAt time.Time `json:"deactivated_at"`
}

// AuthTenantCreatedEvent represents an auth.tenant.created event.
type AuthTenantCreatedEvent struct {
	TenantID     string                 `json:"tenant_id"`
	Slug         string                 `json:"slug"`
	Name         string                 `json:"name"`
	Status       string                 `json:"status"`
	ContactEmail string                 `json:"contact_email,omitempty"`
	ContactPhone string                 `json:"contact_phone,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
}

// AuthTenantUpdatedEvent represents an auth.tenant.updated event.
type AuthTenantUpdatedEvent struct {
	TenantID     string                 `json:"tenant_id"`
	Slug         string                 `json:"slug,omitempty"`
	Name         string                 `json:"name,omitempty"`
	Status       string                 `json:"status,omitempty"`
	ContactEmail string                 `json:"contact_email,omitempty"`
	ContactPhone string                 `json:"contact_phone,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// HandleAuthUserCreated handles auth.user.created events.
func (h *EventHandler) HandleAuthUserCreated(ctx context.Context, event *AuthUserCreatedEvent) error {
	authServiceUserID, err := uuid.Parse(event.UserID)
	if err != nil {
		return fmt.Errorf("identity: invalid user_id in event: %w", err)
	}

	tenantID := event.TenantID
	if tenantID == "" {
		return fmt.Errorf("identity: tenant_id required in event")
	}

	// Convert event to auth-service user data format
	authUserData := map[string]interface{}{
		"id":        event.UserID,
		"email":     event.Email,
		"full_name": event.FullName,
		"phone":     event.Phone,
		"status":    event.Status,
		"roles":     event.Roles,
		"metadata":  event.Metadata,
	}

	// Create or sync user
	_, err = h.service.SyncUserFromAuthService(ctx, authServiceUserID, tenantID, authUserData, "")
	if err != nil {
		h.logger.Error("Failed to sync user from auth.user.created event",
			zap.String("user_id", event.UserID),
			zap.String("tenant_id", tenantID),
			zap.Error(err))
		return fmt.Errorf("identity: sync user from event: %w", err)
	}

	h.logger.Info("User synced from auth.user.created event",
		zap.String("user_id", event.UserID),
		zap.String("tenant_id", tenantID),
		zap.String("email", event.Email))

	return nil
}

// HandleAuthUserUpdated handles auth.user.updated events.
func (h *EventHandler) HandleAuthUserUpdated(ctx context.Context, event *AuthUserUpdatedEvent) error {
	authServiceUserID, err := uuid.Parse(event.UserID)
	if err != nil {
		return fmt.Errorf("identity: invalid user_id in event: %w", err)
	}

	// Find user by auth_service_user_id
	user, err := h.service.repo.FindUserByAuthServiceID(ctx, authServiceUserID)
	if err != nil {
		h.logger.Warn("User not found for auth.user.updated event",
			zap.String("user_id", event.UserID),
			zap.Error(err))
		// User might not exist yet, try to create if we have tenant_id
		if event.TenantID != "" {
			authUserData := map[string]interface{}{
				"id":        event.UserID,
				"email":     event.Email,
				"full_name": event.FullName,
				"phone":     event.Phone,
				"status":    event.Status,
				"metadata":  event.Metadata,
			}
			_, err = h.service.SyncUserFromAuthService(ctx, authServiceUserID, event.TenantID, authUserData, "")
			if err != nil {
				return fmt.Errorf("identity: create user from update event: %w", err)
			}
			return nil
		}
		return fmt.Errorf("identity: user not found and no tenant_id: %w", err)
	}

	// Update user with event data
	authUserData := map[string]interface{}{
		"id":        event.UserID,
		"email":     event.Email,
		"full_name": event.FullName,
		"phone":     event.Phone,
		"status":    event.Status,
		"metadata":  event.Metadata,
	}

	_, err = h.service.updateUserFromAuthService(ctx, user, authUserData)
	if err != nil {
		h.logger.Error("Failed to update user from auth.user.updated event",
			zap.String("user_id", event.UserID),
			zap.Error(err))
		return fmt.Errorf("identity: update user from event: %w", err)
	}

	h.logger.Info("User updated from auth.user.updated event",
		zap.String("user_id", event.UserID))

	return nil
}

// HandleAuthUserDeactivated handles auth.user.deactivated events.
func (h *EventHandler) HandleAuthUserDeactivated(ctx context.Context, event *AuthUserDeactivatedEvent) error {
	authServiceUserID, err := uuid.Parse(event.UserID)
	if err != nil {
		return fmt.Errorf("identity: invalid user_id in event: %w", err)
	}

	// Find user by auth_service_user_id
	user, err := h.service.repo.FindUserByAuthServiceID(ctx, authServiceUserID)
	if err != nil {
		h.logger.Warn("User not found for auth.user.deactivated event",
			zap.String("user_id", event.UserID),
			zap.Error(err))
		// User doesn't exist locally, nothing to deactivate
		return nil
	}

	// Deactivate user
	user.Status = "deactivated"
	now := time.Now()
	user.UpdatedAt = now
	user.SyncAt = &now
	user.SyncStatus = "synced"

	if err := h.service.repo.UpdateUser(ctx, user); err != nil {
		h.logger.Error("Failed to deactivate user from auth.user.deactivated event",
			zap.String("user_id", event.UserID),
			zap.Error(err))
		return fmt.Errorf("identity: deactivate user from event: %w", err)
	}

	h.logger.Info("User deactivated from auth.user.deactivated event",
		zap.String("user_id", event.UserID))

	return nil
}

// HandleAuthTenantCreated handles auth.tenant.created events.
func (h *EventHandler) HandleAuthTenantCreated(ctx context.Context, event *AuthTenantCreatedEvent) error {
	tenantID, err := uuid.Parse(event.TenantID)
	if err != nil {
		return fmt.Errorf("identity: invalid tenant_id in event: %w", err)
	}

	if event.Slug == "" {
		return fmt.Errorf("identity: slug required in tenant.created event")
	}

	tenant := &Tenant{
		ID:     tenantID,
		Slug:   event.Slug,
		Name:   event.Name,
		Status: event.Status,
	}

	if tenant.Status == "" {
		tenant.Status = "active"
	}

	if err := h.service.repo.UpsertTenant(ctx, tenant); err != nil {
		h.logger.Error("Failed to create tenant from auth.tenant.created event",
			zap.String("tenant_id", event.TenantID),
			zap.String("slug", event.Slug),
			zap.Error(err))
		return fmt.Errorf("identity: create tenant from event: %w", err)
	}

	h.logger.Info("Tenant created from auth.tenant.created event",
		zap.String("tenant_id", event.TenantID),
		zap.String("slug", event.Slug),
		zap.String("name", event.Name))

	return nil
}

// HandleAuthTenantUpdated handles auth.tenant.updated events.
func (h *EventHandler) HandleAuthTenantUpdated(ctx context.Context, event *AuthTenantUpdatedEvent) error {
	tenantID, err := uuid.Parse(event.TenantID)
	if err != nil {
		return fmt.Errorf("identity: invalid tenant_id in event: %w", err)
	}

	// Find existing tenant
	tenant, err := h.service.repo.FindTenantByID(ctx, tenantID)
	if err != nil {
		h.logger.Warn("Tenant not found for auth.tenant.updated event, creating new tenant",
			zap.String("tenant_id", event.TenantID),
			zap.Error(err))

		// Tenant might not exist yet, create it if we have the slug
		if event.Slug == "" {
			return fmt.Errorf("identity: tenant not found and no slug provided: %w", err)
		}

		tenant = &Tenant{
			ID:     tenantID,
			Slug:   event.Slug,
			Name:   event.Name,
			Status: event.Status,
		}
		if tenant.Status == "" {
			tenant.Status = "active"
		}
	} else {
		// Update existing tenant fields (only if provided in event)
		if event.Slug != "" {
			tenant.Slug = event.Slug
		}
		if event.Name != "" {
			tenant.Name = event.Name
		}
		if event.Status != "" {
			tenant.Status = event.Status
		}
	}

	if err := h.service.repo.UpsertTenant(ctx, tenant); err != nil {
		h.logger.Error("Failed to update tenant from auth.tenant.updated event",
			zap.String("tenant_id", event.TenantID),
			zap.Error(err))
		return fmt.Errorf("identity: update tenant from event: %w", err)
	}

	h.logger.Info("Tenant updated from auth.tenant.updated event",
		zap.String("tenant_id", event.TenantID),
		zap.String("slug", tenant.Slug))

	return nil
}

// SubscribeToAuthEvents subscribes to auth-service events via NATS.
func (h *EventHandler) SubscribeToAuthEvents(nc *nats.Conn) error {
	// Subscribe to auth.user.created
	_, err := nc.Subscribe("auth.user.created", func(msg *nats.Msg) {
		var event AuthUserCreatedEvent
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			h.logger.Error("Failed to unmarshal auth.user.created event", zap.Error(err))
			return
		}

		ctx := context.Background()
		if err := h.HandleAuthUserCreated(ctx, &event); err != nil {
			h.logger.Error("Failed to handle auth.user.created event", zap.Error(err))
			// Don't ack the message so it can be retried
			return
		}

		// Ack the message
		_ = msg.Ack()
	})
	if err != nil {
		return fmt.Errorf("identity: subscribe to auth.user.created: %w", err)
	}

	// Subscribe to auth.user.updated
	_, err = nc.Subscribe("auth.user.updated", func(msg *nats.Msg) {
		var event AuthUserUpdatedEvent
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			h.logger.Error("Failed to unmarshal auth.user.updated event", zap.Error(err))
			return
		}

		ctx := context.Background()
		if err := h.HandleAuthUserUpdated(ctx, &event); err != nil {
			h.logger.Error("Failed to handle auth.user.updated event", zap.Error(err))
			// Don't ack the message so it can be retried
			return
		}

		// Ack the message
		_ = msg.Ack()
	})
	if err != nil {
		return fmt.Errorf("identity: subscribe to auth.user.updated: %w", err)
	}

	// Subscribe to auth.user.deactivated
	_, err = nc.Subscribe("auth.user.deactivated", func(msg *nats.Msg) {
		var event AuthUserDeactivatedEvent
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			h.logger.Error("Failed to unmarshal auth.user.deactivated event", zap.Error(err))
			return
		}

		ctx := context.Background()
		if err := h.HandleAuthUserDeactivated(ctx, &event); err != nil {
			h.logger.Error("Failed to handle auth.user.deactivated event", zap.Error(err))
			// Don't ack the message so it can be retried
			return
		}

		// Ack the message
		_ = msg.Ack()
	})
	if err != nil {
		return fmt.Errorf("identity: subscribe to auth.user.deactivated: %w", err)
	}

	// Subscribe to auth.tenant.created
	_, err = nc.Subscribe("auth.tenant.created", func(msg *nats.Msg) {
		var event AuthTenantCreatedEvent
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			h.logger.Error("Failed to unmarshal auth.tenant.created event", zap.Error(err))
			return
		}

		ctx := context.Background()
		if err := h.HandleAuthTenantCreated(ctx, &event); err != nil {
			h.logger.Error("Failed to handle auth.tenant.created event", zap.Error(err))
			// Don't ack the message so it can be retried
			return
		}

		// Ack the message
		_ = msg.Ack()
	})
	if err != nil {
		return fmt.Errorf("identity: subscribe to auth.tenant.created: %w", err)
	}

	// Subscribe to auth.tenant.updated
	_, err = nc.Subscribe("auth.tenant.updated", func(msg *nats.Msg) {
		var event AuthTenantUpdatedEvent
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			h.logger.Error("Failed to unmarshal auth.tenant.updated event", zap.Error(err))
			return
		}

		ctx := context.Background()
		if err := h.HandleAuthTenantUpdated(ctx, &event); err != nil {
			h.logger.Error("Failed to handle auth.tenant.updated event", zap.Error(err))
			// Don't ack the message so it can be retried
			return
		}

		// Ack the message
		_ = msg.Ack()
	})
	if err != nil {
		return fmt.Errorf("identity: subscribe to auth.tenant.updated: %w", err)
	}

	h.logger.Info("Subscribed to auth-service events",
		zap.Strings("events", []string{
			"auth.user.created", "auth.user.updated", "auth.user.deactivated",
			"auth.tenant.created", "auth.tenant.updated",
		}))

	return nil
}

