package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// NotificationEvent holds the schema definition for the NotificationEvent entity.
// This stores notification events that need to be sent to the notifications service.
type NotificationEvent struct {
	ent.Schema
}

// Fields of the NotificationEvent.
func (NotificationEvent) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}).
			Comment("Reference to tenant"),
		field.UUID("user_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("Target user for notification"),
		field.String("event_key").
			MaxLen(100).
			NotEmpty().
			Comment("Event key (e.g., order.created, order.ready)"),
		field.JSON("payload", map[string]any{}).
			Comment("Event payload with data for template rendering"),
		field.UUID("order_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("Related order if applicable"),
		field.Enum("status").
			Values("pending", "queued", "sent", "delivered", "failed", "skipped").
			Default("pending").
			Comment("Processing status"),
		field.Int("attempts").
			Default(0).
			Comment("Number of send attempts"),
		field.Time("last_attempt_at").
			Optional().
			Nillable().
			Comment("Last attempt timestamp"),
		field.Text("error_message").
			Optional().
			Comment("Error message if sending failed"),
		field.String("error_code").
			MaxLen(100).
			Optional().
			Comment("Error code if sending failed"),
		field.String("external_id").
			MaxLen(255).
			Optional().
			Comment("External reference from notifications service"),
		field.Time("sent_at").
			Optional().
			Nillable().
			Comment("When notification was sent"),
		field.Time("delivered_at").
			Optional().
			Nillable().
			Comment("When notification was delivered"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the NotificationEvent.
func (NotificationEvent) Edges() []ent.Edge {
	return []ent.Edge{}
}

// Annotations of the NotificationEvent.
func (NotificationEvent) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "notification_events",
		},
	}
}

// Indexes of the NotificationEvent.
func (NotificationEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "event_key"),
		index.Fields("tenant_id", "user_id"),
		index.Fields("tenant_id", "status"),
		// Composite index for retry queue queries
		index.Fields("status", "created_at"),
		index.Fields("status", "attempts", "last_attempt_at"),
		index.Fields("order_id"),
		index.Fields("external_id"),
		index.Fields("created_at"),
		index.Fields("sent_at"),
	}
}
