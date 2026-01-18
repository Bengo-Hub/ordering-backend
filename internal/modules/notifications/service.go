package notifications

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Service provides notification business logic.
type Service struct {
	repo   Repository
	logger *zap.Logger
}

// NewService creates a new notifications service.
func NewService(repo Repository, logger *zap.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger.Named("notifications"),
	}
}

// CreateEvent creates a new notification event.
func (s *Service) CreateEvent(ctx context.Context, req *CreateNotificationEventRequest) (*NotificationEvent, error) {
	if req.EventKey == "" {
		return nil, ErrInvalidEventKey
	}

	event := &NotificationEvent{
		ID:       uuid.New(),
		TenantID: req.TenantID,
		UserID:   req.UserID,
		EventKey: req.EventKey,
		Payload:  req.Payload,
		OrderID:  req.OrderID,
		Status:   EventStatusPending,
	}

	if err := s.repo.CreateEvent(ctx, req.TenantID, event); err != nil {
		s.logger.Error("failed to create notification event",
			zap.Error(err),
			zap.String("event_key", req.EventKey),
		)
		return nil, err
	}

	s.logger.Info("notification event created",
		zap.String("event_id", event.ID.String()),
		zap.String("event_key", event.EventKey),
	)

	return event, nil
}

// GetEvent retrieves a notification event by ID.
func (s *Service) GetEvent(ctx context.Context, tenantID uuid.UUID, eventID uuid.UUID) (*NotificationEvent, error) {
	return s.repo.GetEvent(ctx, tenantID, eventID)
}

// ListEvents lists notification events with optional filtering.
func (s *Service) ListEvents(ctx context.Context, tenantID uuid.UUID, filter NotificationEventFilter) ([]*NotificationEvent, int, error) {
	filter.TenantID = tenantID
	return s.repo.ListEvents(ctx, tenantID, filter)
}

// MarkEventSent marks a notification event as sent.
func (s *Service) MarkEventSent(ctx context.Context, tenantID uuid.UUID, eventID uuid.UUID, externalID string) error {
	if err := s.repo.UpdateEventSent(ctx, tenantID, eventID, externalID); err != nil {
		s.logger.Error("failed to mark event as sent",
			zap.Error(err),
			zap.String("event_id", eventID.String()),
		)
		return err
	}
	return nil
}

// MarkEventDelivered marks a notification event as delivered.
func (s *Service) MarkEventDelivered(ctx context.Context, tenantID uuid.UUID, eventID uuid.UUID) error {
	if err := s.repo.UpdateEventDelivered(ctx, tenantID, eventID); err != nil {
		s.logger.Error("failed to mark event as delivered",
			zap.Error(err),
			zap.String("event_id", eventID.String()),
		)
		return err
	}
	return nil
}

// MarkEventFailed marks a notification event as failed.
func (s *Service) MarkEventFailed(ctx context.Context, tenantID uuid.UUID, eventID uuid.UUID, errorMsg string, errorCode string) error {
	if err := s.repo.IncrementEventAttempts(ctx, tenantID, eventID); err != nil {
		s.logger.Warn("failed to increment event attempts",
			zap.Error(err),
			zap.String("event_id", eventID.String()),
		)
	}

	if err := s.repo.UpdateEventStatus(ctx, tenantID, eventID, EventStatusFailed, errorMsg, errorCode); err != nil {
		s.logger.Error("failed to mark event as failed",
			zap.Error(err),
			zap.String("event_id", eventID.String()),
		)
		return err
	}
	return nil
}

// Template operations

// CreateTemplate creates a new notification template.
func (s *Service) CreateTemplate(ctx context.Context, req *CreateTemplateRequest) (*NotificationTemplate, error) {
	if req.EventKey == "" {
		return nil, ErrInvalidEventKey
	}
	if req.Channel == "" {
		return nil, ErrInvalidChannel
	}

	locale := req.Locale
	if locale == "" {
		locale = "en"
	}

	template := &NotificationTemplate{
		ID:         uuid.New(),
		TenantID:   req.TenantID,
		Channel:    req.Channel,
		EventKey:   req.EventKey,
		Locale:     locale,
		Subject:    req.Subject,
		Body:       req.Body,
		DataSchema: req.DataSchema,
		IsActive:   true,
	}

	if err := s.repo.CreateTemplate(ctx, req.TenantID, template); err != nil {
		s.logger.Error("failed to create notification template",
			zap.Error(err),
			zap.String("event_key", req.EventKey),
			zap.String("channel", string(req.Channel)),
		)
		return nil, err
	}

	s.logger.Info("notification template created",
		zap.String("template_id", template.ID.String()),
		zap.String("event_key", template.EventKey),
		zap.String("channel", string(template.Channel)),
	)

	return template, nil
}

// GetTemplate retrieves a notification template by ID.
func (s *Service) GetTemplate(ctx context.Context, tenantID uuid.UUID, templateID uuid.UUID) (*NotificationTemplate, error) {
	return s.repo.GetTemplate(ctx, tenantID, templateID)
}

// GetTemplateByKey retrieves a notification template by channel, event key, and locale.
func (s *Service) GetTemplateByKey(ctx context.Context, tenantID uuid.UUID, channel NotificationChannel, eventKey string, locale string) (*NotificationTemplate, error) {
	return s.repo.GetTemplateByKey(ctx, tenantID, channel, eventKey, locale)
}

// ListTemplates lists notification templates with optional filtering.
func (s *Service) ListTemplates(ctx context.Context, tenantID uuid.UUID, filter TemplateFilter) ([]*NotificationTemplate, int, error) {
	filter.TenantID = tenantID
	return s.repo.ListTemplates(ctx, tenantID, filter)
}

// UpdateTemplate updates a notification template.
func (s *Service) UpdateTemplate(ctx context.Context, tenantID uuid.UUID, templateID uuid.UUID, req *UpdateTemplateRequest) error {
	if err := s.repo.UpdateTemplate(ctx, tenantID, templateID, req); err != nil {
		s.logger.Error("failed to update notification template",
			zap.Error(err),
			zap.String("template_id", templateID.String()),
		)
		return err
	}
	return nil
}

// DeleteTemplate deletes a notification template.
func (s *Service) DeleteTemplate(ctx context.Context, tenantID uuid.UUID, templateID uuid.UUID) error {
	if err := s.repo.DeleteTemplate(ctx, tenantID, templateID); err != nil {
		s.logger.Error("failed to delete notification template",
			zap.Error(err),
			zap.String("template_id", templateID.String()),
		)
		return err
	}
	return nil
}

// Preference operations

// GetUserPreferences retrieves a user's notification preferences.
func (s *Service) GetUserPreferences(ctx context.Context, tenantID uuid.UUID, userID uuid.UUID) (*UserPreferences, error) {
	return s.repo.GetUserPreferences(ctx, tenantID, userID)
}

// UpdateUserPreferences updates a user's notification preferences.
func (s *Service) UpdateUserPreferences(ctx context.Context, tenantID uuid.UUID, userID uuid.UUID, req *UpdatePreferencesRequest) error {
	if err := s.repo.UpdateUserPreferences(ctx, tenantID, userID, req); err != nil {
		s.logger.Error("failed to update user preferences",
			zap.Error(err),
			zap.String("user_id", userID.String()),
		)
		return err
	}

	s.logger.Info("user notification preferences updated",
		zap.String("user_id", userID.String()),
	)

	return nil
}

// IsUserSubscribed checks if a user is subscribed to a notification channel/event.
func (s *Service) IsUserSubscribed(ctx context.Context, tenantID uuid.UUID, userID uuid.UUID, channel NotificationChannel, eventKey string) (bool, error) {
	return s.repo.IsUserSubscribed(ctx, tenantID, userID, channel, eventKey)
}

// PublishOrderCreated publishes an order.created notification event.
func (s *Service) PublishOrderCreated(ctx context.Context, tenantID uuid.UUID, userID uuid.UUID, orderID uuid.UUID, payload map[string]interface{}) error {
	_, err := s.CreateEvent(ctx, &CreateNotificationEventRequest{
		TenantID: tenantID,
		UserID:   &userID,
		EventKey: string(EventOrderCreated),
		Payload:  payload,
		OrderID:  &orderID,
	})
	return err
}

// PublishOrderReady publishes an order.ready notification event.
func (s *Service) PublishOrderReady(ctx context.Context, tenantID uuid.UUID, userID uuid.UUID, orderID uuid.UUID, payload map[string]interface{}) error {
	_, err := s.CreateEvent(ctx, &CreateNotificationEventRequest{
		TenantID: tenantID,
		UserID:   &userID,
		EventKey: string(EventOrderReady),
		Payload:  payload,
		OrderID:  &orderID,
	})
	return err
}

// PublishDriverAssigned publishes a delivery.driver_assigned notification event.
func (s *Service) PublishDriverAssigned(ctx context.Context, tenantID uuid.UUID, userID uuid.UUID, orderID uuid.UUID, payload map[string]interface{}) error {
	_, err := s.CreateEvent(ctx, &CreateNotificationEventRequest{
		TenantID: tenantID,
		UserID:   &userID,
		EventKey: string(EventDriverAssigned),
		Payload:  payload,
		OrderID:  &orderID,
	})
	return err
}

// PublishDeliveryComplete publishes a delivery.completed notification event.
func (s *Service) PublishDeliveryComplete(ctx context.Context, tenantID uuid.UUID, userID uuid.UUID, orderID uuid.UUID, payload map[string]interface{}) error {
	_, err := s.CreateEvent(ctx, &CreateNotificationEventRequest{
		TenantID: tenantID,
		UserID:   &userID,
		EventKey: string(EventDeliveryComplete),
		Payload:  payload,
		OrderID:  &orderID,
	})
	return err
}
