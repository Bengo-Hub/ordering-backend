package events

import (
	"context"
	"encoding/json"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

// Subscriber handles subscribing to events from NATS.
type Subscriber struct {
	conn   *nats.Conn
	logger *zap.Logger
	subs   []*nats.Subscription
}

// NewSubscriber creates a new event subscriber.
func NewSubscriber(conn *nats.Conn, logger *zap.Logger) *Subscriber {
	return &Subscriber{
		conn:   conn,
		logger: logger.Named("events.subscriber"),
		subs:   make([]*nats.Subscription, 0),
	}
}

// EventHandler is a function that handles an event.
type EventHandler func(ctx context.Context, event Event) error

// Subscribe subscribes to a subject with the given handler.
func (s *Subscriber) Subscribe(subject string, handler EventHandler) error {
	if s.conn == nil {
		s.logger.Warn("NATS connection not available, skipping subscription",
			zap.String("subject", subject))
		return nil
	}

	sub, err := s.conn.Subscribe(subject, func(msg *nats.Msg) {
		var event Event
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			s.logger.Error("failed to unmarshal event",
				zap.Error(err),
				zap.String("subject", subject))
			return
		}

		ctx := context.Background()
		if err := handler(ctx, event); err != nil {
			s.logger.Error("failed to handle event",
				zap.Error(err),
				zap.String("subject", subject),
				zap.String("event_id", event.ID))
		}
	})

	if err != nil {
		return err
	}

	s.subs = append(s.subs, sub)
	s.logger.Info("subscribed to subject", zap.String("subject", subject))

	return nil
}

// QueueSubscribe subscribes to a subject with a queue group.
func (s *Subscriber) QueueSubscribe(subject string, queue string, handler EventHandler) error {
	if s.conn == nil {
		s.logger.Warn("NATS connection not available, skipping queue subscription",
			zap.String("subject", subject),
			zap.String("queue", queue))
		return nil
	}

	sub, err := s.conn.QueueSubscribe(subject, queue, func(msg *nats.Msg) {
		var event Event
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			s.logger.Error("failed to unmarshal event",
				zap.Error(err),
				zap.String("subject", subject))
			return
		}

		ctx := context.Background()
		if err := handler(ctx, event); err != nil {
			s.logger.Error("failed to handle event",
				zap.Error(err),
				zap.String("subject", subject),
				zap.String("event_id", event.ID))
		}
	})

	if err != nil {
		return err
	}

	s.subs = append(s.subs, sub)
	s.logger.Info("subscribed to subject with queue",
		zap.String("subject", subject),
		zap.String("queue", queue))

	return nil
}

// Unsubscribe unsubscribes from all subscriptions.
func (s *Subscriber) Unsubscribe() error {
	for _, sub := range s.subs {
		if err := sub.Unsubscribe(); err != nil {
			s.logger.Warn("failed to unsubscribe",
				zap.Error(err),
				zap.String("subject", sub.Subject))
		}
	}
	s.subs = nil
	return nil
}

// --- Event Subjects for Subscribing ---

const (
	// Inventory events
	SubjectInventoryStockUpdated         = "inventory.stock.updated"
	SubjectInventoryStockLow             = "inventory.stock.low"
	SubjectInventoryStockOut             = "inventory.stock.out"
	SubjectInventoryItemUpdated          = "inventory.item.updated"
	SubjectInventoryReservationConfirmed = "inventory.reservation.confirmed"

	// Notifications events
	SubjectNotificationsDeliveryCompleted = "notifications.delivery.completed"
	SubjectNotificationsDeliveryFailed    = "notifications.delivery.failed"

	// POS events
	SubjectPOSOrderReady      = "pos.order.ready"
	SubjectPOSPickupCompleted = "pos.pickup.completed"

	// Logistics events (already handled via webhooks, but can also subscribe)
	SubjectLogisticsTaskAssigned  = "logistics.task.assigned"
	SubjectLogisticsTaskAccepted  = "logistics.task.accepted"
	SubjectLogisticsTaskCompleted = "logistics.task.completed"
	SubjectLogisticsRouteUpdated  = "logistics.route.updated"
)
