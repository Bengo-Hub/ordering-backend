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

// NotificationTemplate holds the schema definition for the NotificationTemplate entity.
// This stores notification templates for different channels and event types.
type NotificationTemplate struct {
	ent.Schema
}

// Fields of the NotificationTemplate.
func (NotificationTemplate) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}).
			Comment("Reference to tenant"),
		field.Enum("channel").
			Values("email", "sms", "push", "in_app").
			Comment("Notification channel"),
		field.String("event_key").
			MaxLen(100).
			NotEmpty().
			Comment("Event key that triggers this template (e.g., order.created, order.ready)"),
		field.String("locale").
			MaxLen(10).
			Default("en").
			Comment("Locale/language code (e.g., en, sw, fr)"),
		field.String("subject").
			MaxLen(255).
			Optional().
			Comment("Subject line for email/push notifications"),
		field.Text("body").
			NotEmpty().
			Comment("Template body with placeholders (e.g., {{.OrderID}})"),
		field.JSON("data_schema", map[string]any{}).
			Optional().
			Comment("JSON schema for template data validation"),
		field.Bool("is_active").
			Default(true).
			Comment("Whether this template is active"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the NotificationTemplate.
func (NotificationTemplate) Edges() []ent.Edge {
	return []ent.Edge{}
}

// Annotations of the NotificationTemplate.
func (NotificationTemplate) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "notification_templates",
		},
	}
}

// Indexes of the NotificationTemplate.
func (NotificationTemplate) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "channel", "event_key", "locale").Unique(),
		index.Fields("tenant_id", "event_key"),
		index.Fields("is_active"),
	}
}
