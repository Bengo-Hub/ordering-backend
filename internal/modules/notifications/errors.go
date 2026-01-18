package notifications

import "errors"

var (
	// ErrTemplateNotFound is returned when a notification template is not found.
	ErrTemplateNotFound = errors.New("notifications: template not found")

	// ErrEventNotFound is returned when a notification event is not found.
	ErrEventNotFound = errors.New("notifications: event not found")

	// ErrSubscriptionNotFound is returned when a subscription is not found.
	ErrSubscriptionNotFound = errors.New("notifications: subscription not found")

	// ErrInvalidEventKey is returned when an event key is invalid.
	ErrInvalidEventKey = errors.New("notifications: invalid event key")

	// ErrInvalidChannel is returned when a channel is invalid.
	ErrInvalidChannel = errors.New("notifications: invalid channel")

	// ErrTemplateExists is returned when a template already exists.
	ErrTemplateExists = errors.New("notifications: template already exists")

	// ErrSendFailed is returned when notification sending fails.
	ErrSendFailed = errors.New("notifications: failed to send notification")

	// ErrUserNotSubscribed is returned when user has unsubscribed from notification.
	ErrUserNotSubscribed = errors.New("notifications: user not subscribed to this notification")
)
