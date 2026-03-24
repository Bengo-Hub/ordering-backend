package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// Outlet holds the schema definition for the Outlet entity in ordering-backend.
// This is a projection of the outlet registry used across physical and digital channels.
type Outlet struct {
	ent.Schema
}

// Fields of the Outlet.
func (Outlet) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}).
			Comment("Reference to tenant"),
		field.String("name").
			NotEmpty().
			Comment("Display name of the outlet"),
		field.String("slug").
			NotEmpty().
			Comment("URL-safe identifier"),
		field.Text("description").
			Optional(),
		field.String("address").
			Optional(),
		field.String("phone").
			Optional(),
		field.String("email").
			Optional(),
		field.String("location").
			Optional().
			Comment("Geo-location or physical address string"),
		field.Float("latitude").
			Optional().
			Nillable(),
		field.Float("longitude").
			Optional().
			Nillable(),
		field.JSON("opening_hours", map[string]any{}).
			Optional(),
		field.String("image_url").
			Optional().
			Default("").
			Comment("URL or path to the outlet image"),
		field.String("use_case").
			Optional().
			Comment("Specific use case for this outlet (hospitality, retail, etc.)"),
		field.String("status").
			Default("active").
			Comment("active | inactive | closed"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the Outlet.
func (Outlet) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("tenant", Tenant.Type).
			Ref("outlets").
			Field("tenant_id").
			Unique().
			Required(),
		edge.To("orders", Order.Type),
	}
}

// Indexes of the Outlet.
func (Outlet) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "slug").Unique(),
		index.Fields("status"),
	}
}
