package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// RiderProfile holds the schema definition for the RiderProfile entity.
type RiderProfile struct {
	ent.Schema
}

// Fields of the RiderProfile.
func (RiderProfile) Fields() []ent.Field {
	return []ent.Field{
		field.String("national_id").
			Optional(),
		field.String("license_number").
			Optional(),
		field.String("vehicle_type").
			Optional(),
		field.String("status").
			Default("pending"),
		field.Float("rating").
			Default(5.0),
		field.Time("onboarded_at").
			Optional(),
		field.Time("suspended_at").
			Optional(),
		field.JSON("metadata", map[string]any{}).
			Default(map[string]any{}),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the RiderProfile.
func (RiderProfile) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("rider_profile").
			Required().
			Unique(),
		edge.From("tenant", Tenant.Type).
			Ref("rider_profiles").
			Required(),
		edge.To("documents", RiderDocument.Type),
	}
}

// Indexes of the RiderProfile.
func (RiderProfile) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("license_number").Unique(),
	}
}
