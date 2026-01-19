package events

import (
	"context"
	"fmt"

	sharedevents "github.com/Bengo-Hub/shared-events"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

// OutboxPublisher adapts the NATS connection to the outbox.EventPublisher interface.
// This allows the outbox publisher to publish events to NATS using the shared-events Event type.
type OutboxPublisher struct {
	conn   *nats.Conn
	logger *zap.Logger
}

// NewOutboxPublisher creates a new outbox publisher adapter.
func NewOutboxPublisher(conn *nats.Conn, logger *zap.Logger) *OutboxPublisher {
	return &OutboxPublisher{
		conn:   conn,
		logger: logger.Named("outbox.nats"),
	}
}

// Publish publishes an event from the outbox to NATS.
// Implements the outbox.EventPublisher interface.
func (p *OutboxPublisher) Publish(ctx context.Context, event *sharedevents.Event) error {
	if p.conn == nil {
		p.logger.Warn("NATS connection not available, skipping outbox event publish",
			zap.String("event_type", event.EventType),
			zap.String("aggregate_type", event.AggregateType))
		return nil
	}

	// Serialize the event to JSON
	data, err := event.ToJSON()
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	// Build subject from aggregate_type.event_type
	subject := event.Subject()

	// Publish to NATS
	if err := p.conn.Publish(subject, data); err != nil {
		p.logger.Error("failed to publish outbox event",
			zap.Error(err),
			zap.String("subject", subject),
			zap.String("event_id", event.ID.String()))
		return fmt.Errorf("publish event: %w", err)
	}

	p.logger.Debug("outbox event published",
		zap.String("subject", subject),
		zap.String("event_type", event.EventType),
		zap.String("event_id", event.ID.String()),
		zap.String("tenant_id", event.TenantID.String()))

	return nil
}
