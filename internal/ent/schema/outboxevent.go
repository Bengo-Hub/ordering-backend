package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// OutboxEvent holds the schema definition for the transactional outbox pattern.
// Events are stored here atomically with domain operations, then published asynchronously.
type OutboxEvent struct {
	ent.Schema
}

// Fields of the OutboxEvent.
func (OutboxEvent) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}),
		field.String("aggregate_type").
			NotEmpty().
			Comment("Entity type: order, cart, payment, etc."),
		field.UUID("aggregate_id", uuid.UUID{}).
			Comment("ID of the entity this event relates to"),
		field.String("event_type").
			NotEmpty().
			Comment("Event name: order.created, payment.completed, etc."),
		field.Bytes("payload").
			Comment("JSON-encoded event payload"),
		field.Enum("status").
			Values("PENDING", "PUBLISHED", "FAILED").
			Default("PENDING"),
		field.Int("attempts").
			Default(0).
			Comment("Number of publish attempts"),
		field.Time("last_attempt_at").
			Optional().
			Nillable().
			Comment("Timestamp of last publish attempt"),
		field.Time("published_at").
			Optional().
			Nillable().
			Comment("Timestamp when successfully published"),
		field.String("error_message").
			Optional().
			Nillable().
			Comment("Last error message if failed"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

// Indexes of the OutboxEvent.
func (OutboxEvent) Indexes() []ent.Index {
	return []ent.Index{
		// Primary query: find pending events for publishing
		index.Fields("status", "created_at"),
		// Query by aggregate for debugging/replay
		index.Fields("aggregate_type", "aggregate_id"),
		// Tenant-scoped queries
		index.Fields("tenant_id", "status"),
	}
}
