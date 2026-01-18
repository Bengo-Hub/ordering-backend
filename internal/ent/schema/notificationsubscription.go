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

// NotificationSubscription holds the schema definition for the NotificationSubscription entity.
// This stores user notification preferences/subscriptions per channel and event type.
type NotificationSubscription struct {
	ent.Schema
}

// Fields of the NotificationSubscription.
func (NotificationSubscription) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}).
			Comment("Reference to tenant"),
		field.UUID("user_id", uuid.UUID{}).
			Comment("Reference to user"),
		field.Enum("channel").
			Values("email", "sms", "push", "in_app").
			Comment("Notification channel"),
		field.String("event_key").
			MaxLen(100).
			NotEmpty().
			Comment("Event key to subscribe/unsubscribe from"),
		field.Bool("is_subscribed").
			Default(true).
			Comment("Whether user is subscribed to this notification"),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the NotificationSubscription.
func (NotificationSubscription) Edges() []ent.Edge {
	return []ent.Edge{}
}

// Annotations of the NotificationSubscription.
func (NotificationSubscription) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "notification_subscriptions",
		},
	}
}

// Indexes of the NotificationSubscription.
func (NotificationSubscription) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "user_id", "channel", "event_key").Unique(),
		index.Fields("tenant_id", "user_id"),
		index.Fields("is_subscribed"),
	}
}
