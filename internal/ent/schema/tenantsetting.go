package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// TenantSetting holds the schema definition for the TenantSetting entity.
type TenantSetting struct {
	ent.Schema
}

// Fields of the TenantSetting.
func (TenantSetting) Fields() []ent.Field {
	return []ent.Field{
		field.JSON("brand_palette", map[string]any{}).
			Default(map[string]any{}),
		field.Strings("locales").
			Default([]string{"en"}),
		field.JSON("features", map[string]any{}).
			Default(map[string]any{}),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the TenantSetting.
func (TenantSetting) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("tenant", Tenant.Type).
			Ref("settings").
			Required().
			Unique(),
	}
}
