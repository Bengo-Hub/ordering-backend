package identity

import (
	"context"
	"fmt"
	"time"

	sharedevents "github.com/Bengo-Hub/shared-events"
	"github.com/google/uuid"
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

// strFromPayload extracts a string from the payload map, returning "" if absent or wrong type.
func strFromPayload(payload map[string]interface{}, key string) string {
	v, _ := payload[key].(string)
	return v
}

// HandleAuthUserCreated handles auth.user.created events.
func (h *EventHandler) HandleAuthUserCreated(ctx context.Context, evt *sharedevents.Event) error {
	userID := strFromPayload(evt.Payload, "user_id")
	authServiceUserID, err := uuid.Parse(userID)
	if err != nil {
		return fmt.Errorf("identity: invalid user_id in event: %w", err)
	}

	tenantID := evt.TenantID.String()
	if evt.TenantID == uuid.Nil {
		return fmt.Errorf("identity: tenant_id required in event")
	}

	// Convert roles []interface{} → []string
	var roles []string
	if raw, ok := evt.Payload["roles"].([]interface{}); ok {
		for _, r := range raw {
			if s, ok := r.(string); ok {
				roles = append(roles, s)
			}
		}
	}

	authUserData := map[string]interface{}{
		"id":        userID,
		"email":     strFromPayload(evt.Payload, "email"),
		"full_name": strFromPayload(evt.Payload, "full_name"),
		"phone":     strFromPayload(evt.Payload, "phone"),
		"status":    strFromPayload(evt.Payload, "status"),
		"roles":     roles,
		"metadata":  evt.Payload["metadata"],
	}

	_, err = h.service.SyncUserFromAuthService(ctx, authServiceUserID, tenantID, authUserData, "")
	if err != nil {
		h.logger.Error("Failed to sync user from auth.user.created event",
			zap.String("user_id", userID),
			zap.String("tenant_id", tenantID),
			zap.Error(err))
		return fmt.Errorf("identity: sync user from event: %w", err)
	}

	h.logger.Info("User synced from auth.user.created event",
		zap.String("user_id", userID),
		zap.String("tenant_id", tenantID),
		zap.String("email", strFromPayload(evt.Payload, "email")))
	return nil
}

// HandleAuthUserUpdated handles auth.user.updated events.
func (h *EventHandler) HandleAuthUserUpdated(ctx context.Context, evt *sharedevents.Event) error {
	userID := strFromPayload(evt.Payload, "user_id")
	authServiceUserID, err := uuid.Parse(userID)
	if err != nil {
		return fmt.Errorf("identity: invalid user_id in event: %w", err)
	}

	// Find user by auth_service_user_id
	user, err := h.service.repo.FindUserByAuthServiceID(ctx, authServiceUserID)
	if err != nil {
		h.logger.Warn("User not found for auth.user.updated event",
			zap.String("user_id", userID),
			zap.Error(err))
		tenantID := evt.TenantID.String()
		if evt.TenantID != uuid.Nil {
			authUserData := map[string]interface{}{
				"id":        userID,
				"email":     strFromPayload(evt.Payload, "email"),
				"full_name": strFromPayload(evt.Payload, "full_name"),
				"phone":     strFromPayload(evt.Payload, "phone"),
				"status":    strFromPayload(evt.Payload, "status"),
				"metadata":  evt.Payload["metadata"],
			}
			_, err = h.service.SyncUserFromAuthService(ctx, authServiceUserID, tenantID, authUserData, "")
			if err != nil {
				return fmt.Errorf("identity: create user from update event: %w", err)
			}
			return nil
		}
		return fmt.Errorf("identity: user not found and no tenant_id: %w", err)
	}

	authUserData := map[string]interface{}{
		"id":        userID,
		"email":     strFromPayload(evt.Payload, "email"),
		"full_name": strFromPayload(evt.Payload, "full_name"),
		"phone":     strFromPayload(evt.Payload, "phone"),
		"status":    strFromPayload(evt.Payload, "status"),
		"metadata":  evt.Payload["metadata"],
	}

	_, err = h.service.updateUserFromAuthService(ctx, user, authUserData)
	if err != nil {
		h.logger.Error("Failed to update user from auth.user.updated event",
			zap.String("user_id", userID),
			zap.Error(err))
		return fmt.Errorf("identity: update user from event: %w", err)
	}

	h.logger.Info("User updated from auth.user.updated event",
		zap.String("user_id", userID))
	return nil
}

// HandleAuthUserDeactivated handles auth.user.deactivated events.
func (h *EventHandler) HandleAuthUserDeactivated(ctx context.Context, evt *sharedevents.Event) error {
	userID := strFromPayload(evt.Payload, "user_id")
	authServiceUserID, err := uuid.Parse(userID)
	if err != nil {
		return fmt.Errorf("identity: invalid user_id in event: %w", err)
	}

	user, err := h.service.repo.FindUserByAuthServiceID(ctx, authServiceUserID)
	if err != nil {
		h.logger.Warn("User not found for auth.user.deactivated event",
			zap.String("user_id", userID),
			zap.Error(err))
		return nil
	}

	user.Status = "deactivated"
	now := time.Now()
	user.UpdatedAt = now
	user.SyncAt = &now
	user.SyncStatus = "synced"

	if err := h.service.repo.UpdateUser(ctx, user); err != nil {
		h.logger.Error("Failed to deactivate user from auth.user.deactivated event",
			zap.String("user_id", userID),
			zap.Error(err))
		return fmt.Errorf("identity: deactivate user from event: %w", err)
	}

	h.logger.Info("User deactivated from auth.user.deactivated event",
		zap.String("user_id", userID))
	return nil
}

// HandleAuthUserDeleted handles auth.user.deleted events — auth-api's real hard-delete
// (AdminPurgeUser). Delegates to Service.HardDeleteUser; see that method's doc comment
// (and EntRepository.HardDeleteUser's) for the full per-table hard-delete-vs-sever policy.
func (h *EventHandler) HandleAuthUserDeleted(ctx context.Context, evt *sharedevents.Event) error {
	userID := strFromPayload(evt.Payload, "user_id")
	authServiceUserID, err := uuid.Parse(userID)
	if err != nil {
		return fmt.Errorf("identity: invalid user_id in event: %w", err)
	}

	deleted, err := h.service.HardDeleteUser(ctx, authServiceUserID)
	if err != nil {
		h.logger.Error("Failed to hard-delete user from auth.user.deleted event",
			zap.String("user_id", userID), zap.Error(err))
		return fmt.Errorf("identity: hard-delete user from event: %w", err)
	}
	if deleted {
		h.logger.Info("User hard-deleted from auth.user.deleted event", zap.String("user_id", userID))
	}
	return nil
}

// HandleAuthTenantCreated handles auth.tenant.created events.
func (h *EventHandler) HandleAuthTenantCreated(ctx context.Context, evt *sharedevents.Event) error {
	tenantID := evt.TenantID
	if tenantID == uuid.Nil {
		return fmt.Errorf("identity: invalid tenant_id in event")
	}

	slug := strFromPayload(evt.Payload, "slug")
	if slug == "" {
		return fmt.Errorf("identity: slug required in tenant.created event")
	}

	status := strFromPayload(evt.Payload, "status")
	if status == "" {
		status = "active"
	}

	t := &Tenant{
		ID:     tenantID,
		Slug:   slug,
		Name:   strFromPayload(evt.Payload, "name"),
		Status: status,
	}

	if err := h.service.repo.UpsertTenant(ctx, t); err != nil {
		h.logger.Error("Failed to create tenant from auth.tenant.created event",
			zap.String("tenant_id", tenantID.String()),
			zap.String("slug", slug),
			zap.Error(err))
		return fmt.Errorf("identity: create tenant from event: %w", err)
	}

	h.logger.Info("Tenant created from auth.tenant.created event",
		zap.String("tenant_id", tenantID.String()),
		zap.String("slug", slug),
		zap.String("name", strFromPayload(evt.Payload, "name")))
	return nil
}

// HandleAuthTenantUpdated handles auth.tenant.updated events.
func (h *EventHandler) HandleAuthTenantUpdated(ctx context.Context, evt *sharedevents.Event) error {
	tenantID := evt.TenantID
	if tenantID == uuid.Nil {
		return fmt.Errorf("identity: invalid tenant_id in event")
	}

	slug := strFromPayload(evt.Payload, "slug")
	name := strFromPayload(evt.Payload, "name")
	status := strFromPayload(evt.Payload, "status")

	existing, err := h.service.repo.FindTenantByID(ctx, tenantID)
	if err != nil {
		h.logger.Warn("Tenant not found for auth.tenant.updated event, creating new tenant",
			zap.String("tenant_id", tenantID.String()),
			zap.Error(err))
		if slug == "" {
			return fmt.Errorf("identity: tenant not found and no slug provided: %w", err)
		}
		if status == "" {
			status = "active"
		}
		existing = &Tenant{
			ID:     tenantID,
			Slug:   slug,
			Name:   name,
			Status: status,
		}
	} else {
		if slug != "" {
			existing.Slug = slug
		}
		if name != "" {
			existing.Name = name
		}
		if status != "" {
			existing.Status = status
		}
	}

	if err := h.service.repo.UpsertTenant(ctx, existing); err != nil {
		h.logger.Error("Failed to update tenant from auth.tenant.updated event",
			zap.String("tenant_id", tenantID.String()),
			zap.Error(err))
		return fmt.Errorf("identity: update tenant from event: %w", err)
	}

	h.logger.Info("Tenant updated from auth.tenant.updated event",
		zap.String("tenant_id", tenantID.String()),
		zap.String("slug", existing.Slug))
	return nil
}
