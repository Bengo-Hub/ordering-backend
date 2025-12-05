package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// Tenant holds the schema definition for the Tenant entity.
type Tenant struct {
	ent.Schema
}

// Fields of the Tenant.
func (Tenant) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.String("slug").
			NotEmpty().
			Unique(),
		field.String("name").
			NotEmpty(),
		field.String("status").
			Default("active"),
		field.String("contact_email").
			Optional(),
		field.String("contact_phone").
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

// Edges of the Tenant.
func (Tenant) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("settings", TenantSetting.Type).
			Unique(),
		edge.To("users", User.Type),
		edge.To("sessions", Session.Type),
		// DEPRECATED: Rider profiles edge - all rider data owned by logistics-service
		// edge.To("rider_profiles", RiderProfile.Type),
		edge.To("sync_events", TenantSyncEvent.Type),
	}
}
