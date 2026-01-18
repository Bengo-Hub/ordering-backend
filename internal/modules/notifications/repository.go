package notifications

import (
	"context"

	"github.com/google/uuid"
)

// Repository abstracts persistence for notification entities.
type Repository interface {
	// Template operations
	CreateTemplate(ctx context.Context, tenantID uuid.UUID, template *NotificationTemplate) error
	GetTemplate(ctx context.Context, tenantID uuid.UUID, templateID uuid.UUID) (*NotificationTemplate, error)
	GetTemplateByKey(ctx context.Context, tenantID uuid.UUID, channel NotificationChannel, eventKey string, locale string) (*NotificationTemplate, error)
	ListTemplates(ctx context.Context, tenantID uuid.UUID, filter TemplateFilter) ([]*NotificationTemplate, int, error)
	UpdateTemplate(ctx context.Context, tenantID uuid.UUID, templateID uuid.UUID, updates *UpdateTemplateRequest) error
	DeleteTemplate(ctx context.Context, tenantID uuid.UUID, templateID uuid.UUID) error

	// Event operations
	CreateEvent(ctx context.Context, tenantID uuid.UUID, event *NotificationEvent) error
	GetEvent(ctx context.Context, tenantID uuid.UUID, eventID uuid.UUID) (*NotificationEvent, error)
	ListEvents(ctx context.Context, tenantID uuid.UUID, filter NotificationEventFilter) ([]*NotificationEvent, int, error)
	UpdateEventStatus(ctx context.Context, tenantID uuid.UUID, eventID uuid.UUID, status EventStatus, errorMsg string, errorCode string) error
	UpdateEventSent(ctx context.Context, tenantID uuid.UUID, eventID uuid.UUID, externalID string) error
	UpdateEventDelivered(ctx context.Context, tenantID uuid.UUID, eventID uuid.UUID) error
	GetPendingEvents(ctx context.Context, limit int) ([]*NotificationEvent, error)
	IncrementEventAttempts(ctx context.Context, tenantID uuid.UUID, eventID uuid.UUID) error

	// Subscription/Preference operations
	GetSubscription(ctx context.Context, tenantID uuid.UUID, userID uuid.UUID, channel NotificationChannel, eventKey string) (*NotificationSubscription, error)
	UpsertSubscription(ctx context.Context, tenantID uuid.UUID, subscription *NotificationSubscription) error
	ListUserSubscriptions(ctx context.Context, tenantID uuid.UUID, userID uuid.UUID) ([]*NotificationSubscription, error)
	GetUserPreferences(ctx context.Context, tenantID uuid.UUID, userID uuid.UUID) (*UserPreferences, error)
	UpdateUserPreferences(ctx context.Context, tenantID uuid.UUID, userID uuid.UUID, prefs *UpdatePreferencesRequest) error
	IsUserSubscribed(ctx context.Context, tenantID uuid.UUID, userID uuid.UUID, channel NotificationChannel, eventKey string) (bool, error)
}
