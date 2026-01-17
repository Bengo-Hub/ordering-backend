package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// OrderEvent holds the schema definition for the OrderEvent entity.
type OrderEvent struct {
	ent.Schema
}

// Fields of the OrderEvent.
func (OrderEvent) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("order_id", uuid.UUID{}).
			Comment("Reference to order"),
		field.String("event_type").
			MaxLen(100).
			Comment("Event type (e.g., status_changed, payment_received)"),
		field.String("from_status").
			Optional().
			MaxLen(50).
			Comment("Previous status"),
		field.String("to_status").
			Optional().
			MaxLen(50).
			Comment("New status"),
		field.JSON("payload", map[string]any{}).
			Optional().
			Comment("Event payload data"),
		field.UUID("actor_user_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("User who triggered the event"),
		field.String("actor_type").
			Optional().
			MaxLen(50).
			Comment("Actor type (user, system, webhook)"),
		field.String("ip_address").
			Optional().
			MaxLen(45),
		field.Time("occurred_at").
			Default(time.Now).
			Immutable(),
	}
}

// Edges of the OrderEvent.
func (OrderEvent) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("order", Order.Type).
			Ref("events").
			Field("order_id").
			Unique().
			Required(),
	}
}

// Indexes of the OrderEvent.
func (OrderEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("order_id"),
		index.Fields("event_type"),
		index.Fields("occurred_at"),
	}
}
