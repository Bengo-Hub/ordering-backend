package notifications

import (
	"context"
	"time"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/notificationevent"
	"github.com/bengobox/ordering-backend/internal/ent/notificationsubscription"
	"github.com/bengobox/ordering-backend/internal/ent/notificationtemplate"
	"github.com/google/uuid"
)

// EntRepository implements Repository using Ent ORM.
type EntRepository struct {
	client *ent.Client
}

// NewEntRepository creates a new EntRepository.
func NewEntRepository(client *ent.Client) *EntRepository {
	return &EntRepository{client: client}
}

// Template operations

func (r *EntRepository) CreateTemplate(ctx context.Context, tenantID uuid.UUID, template *NotificationTemplate) error {
	created, err := r.client.NotificationTemplate.Create().
		SetID(template.ID).
		SetTenantID(tenantID).
		SetChannel(notificationtemplate.Channel(template.Channel)).
		SetEventKey(template.EventKey).
		SetLocale(template.Locale).
		SetNillableSubject(&template.Subject).
		SetBody(template.Body).
		SetDataSchema(template.DataSchema).
		SetIsActive(template.IsActive).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return ErrTemplateExists
		}
		return err
	}
	template.ID = created.ID
	template.CreatedAt = created.CreatedAt
	template.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *EntRepository) GetTemplate(ctx context.Context, tenantID uuid.UUID, templateID uuid.UUID) (*NotificationTemplate, error) {
	t, err := r.client.NotificationTemplate.Query().
		Where(
			notificationtemplate.ID(templateID),
			notificationtemplate.TenantID(tenantID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrTemplateNotFound
		}
		return nil, err
	}
	return entTemplateToModel(t), nil
}

func (r *EntRepository) GetTemplateByKey(ctx context.Context, tenantID uuid.UUID, channel NotificationChannel, eventKey string, locale string) (*NotificationTemplate, error) {
	t, err := r.client.NotificationTemplate.Query().
		Where(
			notificationtemplate.TenantID(tenantID),
			notificationtemplate.ChannelEQ(notificationtemplate.Channel(channel)),
			notificationtemplate.EventKey(eventKey),
			notificationtemplate.Locale(locale),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrTemplateNotFound
		}
		return nil, err
	}
	return entTemplateToModel(t), nil
}

func (r *EntRepository) ListTemplates(ctx context.Context, tenantID uuid.UUID, filter TemplateFilter) ([]*NotificationTemplate, int, error) {
	query := r.client.NotificationTemplate.Query().
		Where(notificationtemplate.TenantID(tenantID))

	if filter.Channel != nil {
		query = query.Where(notificationtemplate.ChannelEQ(notificationtemplate.Channel(*filter.Channel)))
	}
	if filter.EventKey != nil {
		query = query.Where(notificationtemplate.EventKey(*filter.EventKey))
	}
	if filter.Locale != nil {
		query = query.Where(notificationtemplate.Locale(*filter.Locale))
	}
	if filter.IsActive != nil {
		query = query.Where(notificationtemplate.IsActive(*filter.IsActive))
	}

	total, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}

	templates, err := query.Order(notificationtemplate.ByCreatedAt()).All(ctx)
	if err != nil {
		return nil, 0, err
	}

	result := make([]*NotificationTemplate, len(templates))
	for i, t := range templates {
		result[i] = entTemplateToModel(t)
	}
	return result, total, nil
}

func (r *EntRepository) UpdateTemplate(ctx context.Context, tenantID uuid.UUID, templateID uuid.UUID, updates *UpdateTemplateRequest) error {
	update := r.client.NotificationTemplate.Update().
		Where(
			notificationtemplate.ID(templateID),
			notificationtemplate.TenantID(tenantID),
		)

	if updates.Subject != nil {
		update = update.SetSubject(*updates.Subject)
	}
	if updates.Body != nil {
		update = update.SetBody(*updates.Body)
	}
	if updates.DataSchema != nil {
		update = update.SetDataSchema(updates.DataSchema)
	}
	if updates.IsActive != nil {
		update = update.SetIsActive(*updates.IsActive)
	}

	n, err := update.Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrTemplateNotFound
	}
	return nil
}

func (r *EntRepository) DeleteTemplate(ctx context.Context, tenantID uuid.UUID, templateID uuid.UUID) error {
	n, err := r.client.NotificationTemplate.Delete().
		Where(
			notificationtemplate.ID(templateID),
			notificationtemplate.TenantID(tenantID),
		).
		Exec(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrTemplateNotFound
	}
	return nil
}

// Event operations

func (r *EntRepository) CreateEvent(ctx context.Context, tenantID uuid.UUID, event *NotificationEvent) error {
	create := r.client.NotificationEvent.Create().
		SetID(event.ID).
		SetTenantID(tenantID).
		SetEventKey(event.EventKey).
		SetPayload(event.Payload).
		SetStatus(notificationevent.Status(event.Status))

	if event.UserID != nil {
		create = create.SetUserID(*event.UserID)
	}
	if event.OrderID != nil {
		create = create.SetOrderID(*event.OrderID)
	}

	created, err := create.Save(ctx)
	if err != nil {
		return err
	}
	event.ID = created.ID
	event.CreatedAt = created.CreatedAt
	event.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *EntRepository) GetEvent(ctx context.Context, tenantID uuid.UUID, eventID uuid.UUID) (*NotificationEvent, error) {
	e, err := r.client.NotificationEvent.Query().
		Where(
			notificationevent.ID(eventID),
			notificationevent.TenantID(tenantID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrEventNotFound
		}
		return nil, err
	}
	return entEventToModel(e), nil
}

func (r *EntRepository) ListEvents(ctx context.Context, tenantID uuid.UUID, filter NotificationEventFilter) ([]*NotificationEvent, int, error) {
	query := r.client.NotificationEvent.Query().
		Where(notificationevent.TenantID(tenantID))

	if filter.UserID != nil {
		query = query.Where(notificationevent.UserID(*filter.UserID))
	}
	if filter.EventKey != nil {
		query = query.Where(notificationevent.EventKey(*filter.EventKey))
	}
	if filter.Status != nil {
		query = query.Where(notificationevent.StatusEQ(notificationevent.Status(*filter.Status)))
	}
	if filter.OrderID != nil {
		query = query.Where(notificationevent.OrderID(*filter.OrderID))
	}
	if filter.DateFrom != nil {
		query = query.Where(notificationevent.CreatedAtGTE(*filter.DateFrom))
	}
	if filter.DateTo != nil {
		query = query.Where(notificationevent.CreatedAtLTE(*filter.DateTo))
	}

	total, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}

	events, err := query.Order(notificationevent.ByCreatedAt()).All(ctx)
	if err != nil {
		return nil, 0, err
	}

	result := make([]*NotificationEvent, len(events))
	for i, e := range events {
		result[i] = entEventToModel(e)
	}
	return result, total, nil
}

func (r *EntRepository) UpdateEventStatus(ctx context.Context, tenantID uuid.UUID, eventID uuid.UUID, status EventStatus, errorMsg string, errorCode string) error {
	update := r.client.NotificationEvent.Update().
		Where(
			notificationevent.ID(eventID),
			notificationevent.TenantID(tenantID),
		).
		SetStatus(notificationevent.Status(status))

	if errorMsg != "" {
		update = update.SetErrorMessage(errorMsg)
	}
	if errorCode != "" {
		update = update.SetErrorCode(errorCode)
	}

	n, err := update.Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrEventNotFound
	}
	return nil
}

func (r *EntRepository) UpdateEventSent(ctx context.Context, tenantID uuid.UUID, eventID uuid.UUID, externalID string) error {
	now := time.Now()
	n, err := r.client.NotificationEvent.Update().
		Where(
			notificationevent.ID(eventID),
			notificationevent.TenantID(tenantID),
		).
		SetStatus(notificationevent.StatusSent).
		SetSentAt(now).
		SetExternalID(externalID).
		Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrEventNotFound
	}
	return nil
}

func (r *EntRepository) UpdateEventDelivered(ctx context.Context, tenantID uuid.UUID, eventID uuid.UUID) error {
	now := time.Now()
	n, err := r.client.NotificationEvent.Update().
		Where(
			notificationevent.ID(eventID),
			notificationevent.TenantID(tenantID),
		).
		SetStatus(notificationevent.StatusDelivered).
		SetDeliveredAt(now).
		Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrEventNotFound
	}
	return nil
}

func (r *EntRepository) GetPendingEvents(ctx context.Context, limit int) ([]*NotificationEvent, error) {
	events, err := r.client.NotificationEvent.Query().
		Where(notificationevent.StatusEQ(notificationevent.StatusPending)).
		Order(notificationevent.ByCreatedAt()).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*NotificationEvent, len(events))
	for i, e := range events {
		result[i] = entEventToModel(e)
	}
	return result, nil
}

func (r *EntRepository) IncrementEventAttempts(ctx context.Context, tenantID uuid.UUID, eventID uuid.UUID) error {
	now := time.Now()
	n, err := r.client.NotificationEvent.Update().
		Where(
			notificationevent.ID(eventID),
			notificationevent.TenantID(tenantID),
		).
		AddAttempts(1).
		SetLastAttemptAt(now).
		Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrEventNotFound
	}
	return nil
}

// Subscription operations

func (r *EntRepository) GetSubscription(ctx context.Context, tenantID uuid.UUID, userID uuid.UUID, channel NotificationChannel, eventKey string) (*NotificationSubscription, error) {
	s, err := r.client.NotificationSubscription.Query().
		Where(
			notificationsubscription.TenantID(tenantID),
			notificationsubscription.UserID(userID),
			notificationsubscription.ChannelEQ(notificationsubscription.Channel(channel)),
			notificationsubscription.EventKey(eventKey),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrSubscriptionNotFound
		}
		return nil, err
	}
	return entSubscriptionToModel(s), nil
}

func (r *EntRepository) UpsertSubscription(ctx context.Context, tenantID uuid.UUID, subscription *NotificationSubscription) error {
	err := r.client.NotificationSubscription.Create().
		SetID(subscription.ID).
		SetTenantID(tenantID).
		SetUserID(subscription.UserID).
		SetChannel(notificationsubscription.Channel(subscription.Channel)).
		SetEventKey(subscription.EventKey).
		SetIsSubscribed(subscription.IsSubscribed).
		OnConflictColumns(
			notificationsubscription.FieldTenantID,
			notificationsubscription.FieldUserID,
			notificationsubscription.FieldChannel,
			notificationsubscription.FieldEventKey,
		).
		UpdateIsSubscribed().
		UpdateUpdatedAt().
		Exec(ctx)
	return err
}

func (r *EntRepository) ListUserSubscriptions(ctx context.Context, tenantID uuid.UUID, userID uuid.UUID) ([]*NotificationSubscription, error) {
	subs, err := r.client.NotificationSubscription.Query().
		Where(
			notificationsubscription.TenantID(tenantID),
			notificationsubscription.UserID(userID),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*NotificationSubscription, len(subs))
	for i, s := range subs {
		result[i] = entSubscriptionToModel(s)
	}
	return result, nil
}

func (r *EntRepository) GetUserPreferences(ctx context.Context, tenantID uuid.UUID, userID uuid.UUID) (*UserPreferences, error) {
	subs, err := r.ListUserSubscriptions(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}

	prefs := &UserPreferences{
		UserID:        userID,
		Email:         true, // defaults
		SMS:           true,
		Push:          true,
		InApp:         true,
		Subscriptions: make([]SubscriptionSetting, len(subs)),
	}

	eventTypes := make(map[string]bool)
	for i, sub := range subs {
		prefs.Subscriptions[i] = SubscriptionSetting{
			EventKey:     sub.EventKey,
			Channel:      string(sub.Channel),
			IsSubscribed: sub.IsSubscribed,
		}
		if sub.IsSubscribed {
			eventTypes[sub.EventKey] = true
		}
	}

	prefs.EventTypes = make([]string, 0, len(eventTypes))
	for k := range eventTypes {
		prefs.EventTypes = append(prefs.EventTypes, k)
	}

	return prefs, nil
}

func (r *EntRepository) UpdateUserPreferences(ctx context.Context, tenantID uuid.UUID, userID uuid.UUID, prefs *UpdatePreferencesRequest) error {
	// Update channel-level preferences by upserting subscriptions for all event types
	channels := []NotificationChannel{ChannelEmail, ChannelSMS, ChannelPush, ChannelInApp}
	channelEnabled := map[NotificationChannel]bool{
		ChannelEmail: true,
		ChannelSMS:   true,
		ChannelPush:  true,
		ChannelInApp: true,
	}

	if prefs.Email != nil {
		channelEnabled[ChannelEmail] = *prefs.Email
	}
	if prefs.SMS != nil {
		channelEnabled[ChannelSMS] = *prefs.SMS
	}
	if prefs.Push != nil {
		channelEnabled[ChannelPush] = *prefs.Push
	}
	if prefs.InApp != nil {
		channelEnabled[ChannelInApp] = *prefs.InApp
	}

	// If event types are specified, only update those
	if len(prefs.EventTypes) > 0 {
		for _, eventKey := range prefs.EventTypes {
			for _, channel := range channels {
				sub := &NotificationSubscription{
					ID:           uuid.New(),
					TenantID:     tenantID,
					UserID:       userID,
					Channel:      channel,
					EventKey:     eventKey,
					IsSubscribed: channelEnabled[channel],
				}
				if err := r.UpsertSubscription(ctx, tenantID, sub); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (r *EntRepository) IsUserSubscribed(ctx context.Context, tenantID uuid.UUID, userID uuid.UUID, channel NotificationChannel, eventKey string) (bool, error) {
	sub, err := r.GetSubscription(ctx, tenantID, userID, channel, eventKey)
	if err != nil {
		if err == ErrSubscriptionNotFound {
			// No explicit preference means subscribed by default
			return true, nil
		}
		return false, err
	}
	return sub.IsSubscribed, nil
}

// Converters

func entTemplateToModel(t *ent.NotificationTemplate) *NotificationTemplate {
	return &NotificationTemplate{
		ID:         t.ID,
		TenantID:   t.TenantID,
		Channel:    NotificationChannel(t.Channel),
		EventKey:   t.EventKey,
		Locale:     t.Locale,
		Subject:    t.Subject,
		Body:       t.Body,
		DataSchema: t.DataSchema,
		IsActive:   t.IsActive,
		CreatedAt:  t.CreatedAt,
		UpdatedAt:  t.UpdatedAt,
	}
}

func entEventToModel(e *ent.NotificationEvent) *NotificationEvent {
	return &NotificationEvent{
		ID:            e.ID,
		TenantID:      e.TenantID,
		UserID:        e.UserID,
		EventKey:      e.EventKey,
		Payload:       e.Payload,
		OrderID:       e.OrderID,
		Status:        EventStatus(e.Status),
		Attempts:      e.Attempts,
		LastAttemptAt: e.LastAttemptAt,
		ErrorMessage:  e.ErrorMessage,
		ErrorCode:     e.ErrorCode,
		ExternalID:    e.ExternalID,
		SentAt:        e.SentAt,
		DeliveredAt:   e.DeliveredAt,
		CreatedAt:     e.CreatedAt,
		UpdatedAt:     e.UpdatedAt,
	}
}

func entSubscriptionToModel(s *ent.NotificationSubscription) *NotificationSubscription {
	return &NotificationSubscription{
		ID:           s.ID,
		TenantID:     s.TenantID,
		UserID:       s.UserID,
		Channel:      NotificationChannel(s.Channel),
		EventKey:     s.EventKey,
		IsSubscribed: s.IsSubscribed,
		UpdatedAt:    s.UpdatedAt,
	}
}
