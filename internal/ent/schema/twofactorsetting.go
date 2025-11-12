package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// TwoFactorSetting holds the schema definition for the TwoFactorSetting entity.
type TwoFactorSetting struct {
	ent.Schema
}

// Fields of the TwoFactorSetting.
func (TwoFactorSetting) Fields() []ent.Field {
	return []ent.Field{
		field.String("method").
			Default("totp"),
		field.String("secret").
			Optional(),
		field.String("backup_phone").
			Optional(),
		field.Bool("enabled").
			Default(false),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the TwoFactorSetting.
func (TwoFactorSetting) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("user", User.Type).
			Unique().
			Required(),
	}
}
