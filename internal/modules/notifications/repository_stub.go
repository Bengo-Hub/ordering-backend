package notifications

import (
	"context"

	"github.com/google/uuid"
)

// StubRepository implements Repository with no local persistence.
// Notification events, templates, and subscriptions are owned by notifications-api;
// ordering-backend does not store them. Use this when notifications-api is the source of truth.
type StubRepository struct{}

// NewStubRepository returns a repository that returns empty data and no-ops writes.
func NewStubRepository() *StubRepository {
	return &StubRepository{}
}

// Template operations — no-op / not found (templates live in notifications-api).
func (r *StubRepository) CreateTemplate(ctx context.Context, tenantID uuid.UUID, template *NotificationTemplate) error {
	return nil
}
func (r *StubRepository) GetTemplate(ctx context.Context, tenantID uuid.UUID, templateID uuid.UUID) (*NotificationTemplate, error) {
	return nil, ErrTemplateNotFound
}
func (r *StubRepository) GetTemplateByKey(ctx context.Context, tenantID uuid.UUID, channel NotificationChannel, eventKey string, locale string) (*NotificationTemplate, error) {
	return nil, ErrTemplateNotFound
}
func (r *StubRepository) ListTemplates(ctx context.Context, tenantID uuid.UUID, filter TemplateFilter) ([]*NotificationTemplate, int, error) {
	return nil, 0, nil
}
func (r *StubRepository) UpdateTemplate(ctx context.Context, tenantID uuid.UUID, templateID uuid.UUID, updates *UpdateTemplateRequest) error {
	return nil
}
func (r *StubRepository) DeleteTemplate(ctx context.Context, tenantID uuid.UUID, templateID uuid.UUID) error {
	return nil
}

// Event operations — no-op / empty (events live in notifications-api).
func (r *StubRepository) CreateEvent(ctx context.Context, tenantID uuid.UUID, event *NotificationEvent) error {
	return nil
}
func (r *StubRepository) GetEvent(ctx context.Context, tenantID uuid.UUID, eventID uuid.UUID) (*NotificationEvent, error) {
	return nil, ErrEventNotFound
}
func (r *StubRepository) ListEvents(ctx context.Context, tenantID uuid.UUID, filter NotificationEventFilter) ([]*NotificationEvent, int, error) {
	return nil, 0, nil
}
func (r *StubRepository) UpdateEventStatus(ctx context.Context, tenantID uuid.UUID, eventID uuid.UUID, status EventStatus, errorMsg string, errorCode string) error {
	return nil
}
func (r *StubRepository) UpdateEventSent(ctx context.Context, tenantID uuid.UUID, eventID uuid.UUID, externalID string) error {
	return nil
}
func (r *StubRepository) UpdateEventDelivered(ctx context.Context, tenantID uuid.UUID, eventID uuid.UUID) error {
	return nil
}
func (r *StubRepository) GetPendingEvents(ctx context.Context, limit int) ([]*NotificationEvent, error) {
	return nil, nil
}
func (r *StubRepository) IncrementEventAttempts(ctx context.Context, tenantID uuid.UUID, eventID uuid.UUID) error {
	return nil
}

// Subscription operations — not found / no-op (subscriptions live in notifications-api).
func (r *StubRepository) GetSubscription(ctx context.Context, tenantID uuid.UUID, userID uuid.UUID, channel NotificationChannel, eventKey string) (*NotificationSubscription, error) {
	return nil, ErrSubscriptionNotFound
}
func (r *StubRepository) UpsertSubscription(ctx context.Context, tenantID uuid.UUID, subscription *NotificationSubscription) error {
	return nil
}
func (r *StubRepository) ListUserSubscriptions(ctx context.Context, tenantID uuid.UUID, userID uuid.UUID) ([]*NotificationSubscription, error) {
	return nil, nil
}
func (r *StubRepository) GetUserPreferences(ctx context.Context, tenantID uuid.UUID, userID uuid.UUID) (*UserPreferences, error) {
	return &UserPreferences{UserID: userID}, nil
}
func (r *StubRepository) UpdateUserPreferences(ctx context.Context, tenantID uuid.UUID, userID uuid.UUID, prefs *UpdatePreferencesRequest) error {
	return nil
}
func (r *StubRepository) IsUserSubscribed(ctx context.Context, tenantID uuid.UUID, userID uuid.UUID, channel NotificationChannel, eventKey string) (bool, error) {
	return true, nil
}
