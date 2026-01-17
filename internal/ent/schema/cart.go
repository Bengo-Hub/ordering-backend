package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// Cart holds the schema definition for the Cart entity.
type Cart struct {
	ent.Schema
}

// Fields of the Cart.
func (Cart) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}).
			Comment("Reference to tenant"),
		field.UUID("cafe_id", uuid.UUID{}).
			Comment("Reference to cafe/outlet"),
		field.UUID("user_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("Reference to user (null for guest carts)"),
		field.String("session_id").
			Optional().
			MaxLen(255).
			Comment("Session ID for guest carts"),
		field.Enum("status").
			Values("active", "checked_out", "abandoned", "expired").
			Default("active"),
		field.String("currency").
			Default("KES").
			MaxLen(3),
		field.Float("subtotal").
			Default(0).
			Comment("Sum of item prices before discounts"),
		field.Float("discount_total").
			Default(0).
			Comment("Total discount amount"),
		field.Float("tax_total").
			Default(0).
			Comment("Total tax amount"),
		field.Float("delivery_fee").
			Default(0).
			Comment("Delivery fee"),
		field.Int("loyalty_points_redeemed").
			Default(0).
			Comment("Loyalty points redeemed"),
		field.UUID("promo_code_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("Applied promo code"),
		field.Time("expires_at").
			Optional().
			Nillable().
			Comment("Cart expiration time"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the Cart.
func (Cart) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("items", CartItem.Type),
		edge.From("user", User.Type).
			Ref("carts").
			Field("user_id").
			Unique(),
	}
}

// Annotations of the Cart.
func (Cart) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "carts",
		},
	}
}

// Indexes of the Cart.
func (Cart) Indexes() []ent.Index {
	return []ent.Index{
		// Partial unique index: one active cart per user per tenant
		index.Fields("tenant_id", "user_id", "status").
			Annotations(entsql.IndexAnnotation{
				Where: "status = 'active' AND user_id IS NOT NULL",
			}).
			Unique(),
		// Partial unique index: one active cart per session per tenant
		index.Fields("tenant_id", "session_id", "status").
			Annotations(entsql.IndexAnnotation{
				Where: "status = 'active' AND session_id IS NOT NULL",
			}).
			Unique(),
		index.Fields("status"),
		index.Fields("expires_at"),
	}
}
