package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// CustomerAddress holds the schema definition for the CustomerAddress entity.
type CustomerAddress struct {
	ent.Schema
}

// Fields of the CustomerAddress.
func (CustomerAddress) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}).
			Comment("Reference to tenant"),
		field.UUID("user_id", uuid.UUID{}).
			Comment("Reference to user"),
		field.String("label").
			MaxLen(100).
			Comment("Address label (e.g., Home, Work, Office)"),
		field.String("address_line1").
			MaxLen(255).
			Comment("Street address"),
		field.String("address_line2").
			Optional().
			MaxLen(255).
			Comment("Apartment, suite, unit, etc."),
		field.String("city").
			MaxLen(100),
		field.String("county").
			Optional().
			MaxLen(100).
			Comment("County/Region"),
		field.String("postal_code").
			Optional().
			MaxLen(20),
		field.String("country").
			Default("KE").
			MaxLen(2).
			Comment("ISO 3166-1 alpha-2 country code"),
		field.Float("latitude").
			Optional().
			Nillable().
			Comment("GPS latitude"),
		field.Float("longitude").
			Optional().
			Nillable().
			Comment("GPS longitude"),
		field.String("plus_code").
			Optional().
			MaxLen(20).
			Comment("Google Plus Code"),
		field.Text("instructions").
			Optional().
			Comment("Delivery instructions"),
		field.String("contact_name").
			Optional().
			MaxLen(255).
			Comment("Recipient name if different from user"),
		field.String("contact_phone").
			Optional().
			MaxLen(20).
			Comment("Contact phone for delivery"),
		field.Bool("is_default").
			Default(false).
			Comment("Default address for user"),
		field.Bool("is_verified").
			Default(false).
			Comment("Address verified via geocoding"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the CustomerAddress.
func (CustomerAddress) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("addresses").
			Field("user_id").
			Unique().
			Required(),
		edge.To("orders", Order.Type),
	}
}

// Indexes of the CustomerAddress.
func (CustomerAddress) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "user_id"),
		index.Fields("user_id", "is_default"),
		index.Fields("latitude", "longitude"),
	}
}
