package events

import (
	"context"
	"fmt"

	sharedevents "github.com/Bengo-Hub/shared-events"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

// OutboxPublisher adapts the JetStream context to the outbox.EventPublisher interface.
// This allows the outbox publisher to publish events to NATS JetStream using the
// shared-events Event type, ensuring durability and replay for downstream consumers.
type OutboxPublisher struct {
	js     nats.JetStreamContext
	logger *zap.Logger
}

// NewOutboxPublisher creates a new outbox publisher adapter using JetStream.
func NewOutboxPublisher(js nats.JetStreamContext, logger *zap.Logger) *OutboxPublisher {
	return &OutboxPublisher{
		js:     js,
		logger: logger.Named("outbox.nats"),
	}
}

// Publish publishes an event from the outbox to NATS JetStream.
// Implements the outbox.EventPublisher interface.
func (p *OutboxPublisher) Publish(ctx context.Context, event *sharedevents.Event) error {
	if p.js == nil {
		p.logger.Warn("JetStream not available, skipping outbox event publish",
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

	// Build NATS message with headers for event metadata
	headers := make(nats.Header)
	headers.Set("event-id", event.ID.String())
	headers.Set("event-type", event.EventType)
	headers.Set("aggregate-type", event.AggregateType)
	headers.Set("aggregate-id", event.AggregateID.String())
	headers.Set("tenant-id", event.TenantID.String())
	headers.Set("event-version", event.Version)

	msg := nats.NewMsg(subject)
	msg.Data = data
	msg.Header = headers

	// Publish to JetStream (durable, with ack)
	if _, err := p.js.PublishMsg(msg); err != nil {
		p.logger.Error("failed to publish outbox event to JetStream",
			zap.Error(err),
			zap.String("subject", subject),
			zap.String("event_id", event.ID.String()))
		return fmt.Errorf("publish event: %w", err)
	}

	p.logger.Debug("outbox event published to JetStream",
		zap.String("subject", subject),
		zap.String("event_type", event.EventType),
		zap.String("event_id", event.ID.String()),
		zap.String("tenant_id", event.TenantID.String()))

	return nil
}
