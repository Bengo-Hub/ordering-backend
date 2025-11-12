package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// UserPreference holds the schema definition for the UserPreference entity.
type UserPreference struct {
	ent.Schema
}

// Fields of the UserPreference.
func (UserPreference) Fields() []ent.Field {
	return []ent.Field{
		field.String("theme").
			Default("system"),
		field.String("language").
			Default("en"),
		field.Bool("notify_email").
			Default(true),
		field.Bool("notify_sms").
			Default(false),
		field.Bool("notify_push").
			Default(true),
		field.String("timezone").
			Default("Africa/Nairobi"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the UserPreference.
func (UserPreference) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("preferences").
			Unique(),
	}
}
