package notifications

import (
	"time"

	"github.com/google/uuid"
)

// NotificationChannel represents the delivery channel for notifications.
type NotificationChannel string

const (
	ChannelEmail  NotificationChannel = "email"
	ChannelSMS    NotificationChannel = "sms"
	ChannelPush   NotificationChannel = "push"
	ChannelInApp  NotificationChannel = "in_app"
)

// EventStatus represents the processing status of a notification event.
type EventStatus string

const (
	EventStatusPending   EventStatus = "pending"
	EventStatusQueued    EventStatus = "queued"
	EventStatusSent      EventStatus = "sent"
	EventStatusDelivered EventStatus = "delivered"
	EventStatusFailed    EventStatus = "failed"
	EventStatusSkipped   EventStatus = "skipped"
)

// NotificationEventKey represents standard notification event keys.
type NotificationEventKey string

const (
	// Order events
	EventOrderCreated       NotificationEventKey = "order.created"
	EventOrderConfirmed     NotificationEventKey = "order.confirmed"
	EventOrderReady         NotificationEventKey = "order.ready"
	EventOrderPickedUp      NotificationEventKey = "order.picked_up"
	EventOrderDelivered     NotificationEventKey = "order.delivered"
	EventOrderCancelled     NotificationEventKey = "order.cancelled"
	EventOrderStatusChanged NotificationEventKey = "order.status_changed"

	// Payment events
	EventPaymentSucceeded NotificationEventKey = "payment.succeeded"
	EventPaymentFailed    NotificationEventKey = "payment.failed"
	EventRefundProcessed  NotificationEventKey = "refund.processed"

	// Delivery events
	EventDriverAssigned   NotificationEventKey = "delivery.driver_assigned"
	EventDriverEnRoute    NotificationEventKey = "delivery.en_route"
	EventDeliveryETA      NotificationEventKey = "delivery.eta_updated"
	EventDeliveryComplete NotificationEventKey = "delivery.completed"

	// Loyalty events
	EventLoyaltyPointsAwarded  NotificationEventKey = "loyalty.points_awarded"
	EventLoyaltyPointsRedeemed NotificationEventKey = "loyalty.points_redeemed"
	EventLoyaltyTierChanged    NotificationEventKey = "loyalty.tier_changed"

	// Support events
	EventTicketCreated   NotificationEventKey = "support.ticket_created"
	EventTicketUpdated   NotificationEventKey = "support.ticket_updated"
	EventTicketResolved  NotificationEventKey = "support.ticket_resolved"
	EventTicketEscalated NotificationEventKey = "support.ticket_escalated"
)

// NotificationTemplate represents a notification template.
type NotificationTemplate struct {
	ID         uuid.UUID              `json:"id"`
	TenantID   uuid.UUID              `json:"tenant_id"`
	Channel    NotificationChannel    `json:"channel"`
	EventKey   string                 `json:"event_key"`
	Locale     string                 `json:"locale"`
	Subject    string                 `json:"subject,omitempty"`
	Body       string                 `json:"body"`
	DataSchema map[string]interface{} `json:"data_schema,omitempty"`
	IsActive   bool                   `json:"is_active"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
}

// NotificationEvent represents a notification event to be sent.
type NotificationEvent struct {
	ID            uuid.UUID              `json:"id"`
	TenantID      uuid.UUID              `json:"tenant_id"`
	UserID        *uuid.UUID             `json:"user_id,omitempty"`
	EventKey      string                 `json:"event_key"`
	Payload       map[string]interface{} `json:"payload"`
	OrderID       *uuid.UUID             `json:"order_id,omitempty"`
	Status        EventStatus            `json:"status"`
	Attempts      int                    `json:"attempts"`
	LastAttemptAt *time.Time             `json:"last_attempt_at,omitempty"`
	ErrorMessage  string                 `json:"error_message,omitempty"`
	ErrorCode     string                 `json:"error_code,omitempty"`
	ExternalID    string                 `json:"external_id,omitempty"`
	SentAt        *time.Time             `json:"sent_at,omitempty"`
	DeliveredAt   *time.Time             `json:"delivered_at,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
}

// NotificationSubscription represents a user's notification preference.
type NotificationSubscription struct {
	ID           uuid.UUID           `json:"id"`
	TenantID     uuid.UUID           `json:"tenant_id"`
	UserID       uuid.UUID           `json:"user_id"`
	Channel      NotificationChannel `json:"channel"`
	EventKey     string              `json:"event_key"`
	IsSubscribed bool                `json:"is_subscribed"`
	UpdatedAt    time.Time           `json:"updated_at"`
}

// UserPreferences represents aggregated notification preferences for a user.
type UserPreferences struct {
	UserID        uuid.UUID              `json:"user_id"`
	Email         bool                   `json:"email"`
	SMS           bool                   `json:"sms"`
	Push          bool                   `json:"push"`
	InApp         bool                   `json:"in_app"`
	EventTypes    []string               `json:"event_types,omitempty"`
	Subscriptions []SubscriptionSetting  `json:"subscriptions,omitempty"`
}

// SubscriptionSetting represents a specific event/channel subscription.
type SubscriptionSetting struct {
	EventKey     string `json:"event_key"`
	Channel      string `json:"channel"`
	IsSubscribed bool   `json:"is_subscribed"`
}

// CreateNotificationEventRequest represents a request to create a notification event.
type CreateNotificationEventRequest struct {
	TenantID uuid.UUID              `json:"tenant_id"`
	UserID   *uuid.UUID             `json:"user_id,omitempty"`
	EventKey string                 `json:"event_key"`
	Payload  map[string]interface{} `json:"payload"`
	OrderID  *uuid.UUID             `json:"order_id,omitempty"`
}

// CreateTemplateRequest represents a request to create a notification template.
type CreateTemplateRequest struct {
	TenantID   uuid.UUID              `json:"tenant_id"`
	Channel    NotificationChannel    `json:"channel"`
	EventKey   string                 `json:"event_key"`
	Locale     string                 `json:"locale"`
	Subject    string                 `json:"subject,omitempty"`
	Body       string                 `json:"body"`
	DataSchema map[string]interface{} `json:"data_schema,omitempty"`
}

// UpdateTemplateRequest represents a request to update a notification template.
type UpdateTemplateRequest struct {
	Subject    *string                `json:"subject,omitempty"`
	Body       *string                `json:"body,omitempty"`
	DataSchema map[string]interface{} `json:"data_schema,omitempty"`
	IsActive   *bool                  `json:"is_active,omitempty"`
}

// UpdatePreferencesRequest represents a request to update user notification preferences.
type UpdatePreferencesRequest struct {
	Email      *bool    `json:"email,omitempty"`
	SMS        *bool    `json:"sms,omitempty"`
	Push       *bool    `json:"push,omitempty"`
	InApp      *bool    `json:"in_app,omitempty"`
	EventTypes []string `json:"event_types,omitempty"`
}

// NotificationEventFilter defines filters for listing notification events.
type NotificationEventFilter struct {
	TenantID uuid.UUID
	UserID   *uuid.UUID
	EventKey *string
	Status   *EventStatus
	OrderID  *uuid.UUID
	DateFrom *time.Time
	DateTo   *time.Time
	Limit    int
	Offset   int
}

// TemplateFilter defines filters for listing notification templates.
type TemplateFilter struct {
	TenantID uuid.UUID
	Channel  *NotificationChannel
	EventKey *string
	Locale   *string
	IsActive *bool
	Limit    int
	Offset   int
}
