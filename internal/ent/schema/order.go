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

// Order holds the schema definition for the Order entity.
type Order struct {
	ent.Schema
}

// Fields of the Order.
func (Order) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}).
			Comment("Reference to tenant"),
		field.UUID("cafe_id", uuid.UUID{}).
			Comment("Reference to cafe/outlet"),
		field.UUID("customer_id", uuid.UUID{}).
			Comment("Reference to customer user"),
		field.UUID("cart_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("Reference to source cart"),
		field.String("order_number").
			MaxLen(50).
			Comment("Human-readable order number"),
		field.Enum("status").
			Values("pending", "confirmed", "preparing", "ready", "out_for_delivery", "delivered", "completed", "cancelled", "refunded").
			Default("pending"),
		field.Enum("payment_status").
			Values("pending", "authorized", "paid", "failed", "refunded", "partially_refunded").
			Default("pending"),
		field.String("currency").
			Default("KES").
			MaxLen(3),
		field.Float("subtotal").
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
		field.Float("tip_total").
			Default(0).
			Comment("Tip amount"),
		field.Float("grand_total").
			Comment("Final total amount"),
		field.Int("loyalty_points_earned").
			Default(0),
		field.Int("loyalty_points_redeemed").
			Default(0),
		field.UUID("delivery_address_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("Reference to delivery address"),
		field.UUID("promo_code_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("Applied promo code"),
		field.Text("instructions").
			Optional().
			Comment("Delivery/order instructions"),
		field.Enum("channel").
			Values("web", "mobile_app", "kiosk", "phone", "api").
			Default("web").
			Comment("Order channel"),
		field.String("source").
			Optional().
			MaxLen(100).
			Comment("Order source identifier"),
		field.String("idempotency_key").
			Optional().
			MaxLen(255).
			Comment("Idempotency key for duplicate prevention"),
		field.Time("placed_at").
			Optional().
			Nillable().
			Comment("When order was placed"),
		field.Time("confirmed_at").
			Optional().
			Nillable(),
		field.Time("ready_at").
			Optional().
			Nillable().
			Comment("When order is ready for pickup/delivery"),
		field.Time("delivered_at").
			Optional().
			Nillable(),
		field.Time("completed_at").
			Optional().
			Nillable(),
		field.Time("cancelled_at").
			Optional().
			Nillable(),
		field.Text("cancellation_reason").
			Optional(),
		field.JSON("metadata", map[string]any{}).
			Optional().
			Comment("Additional metadata"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the Order.
func (Order) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("items", OrderItem.Type),
		edge.To("events", OrderEvent.Type),
		edge.To("payment_intents", PaymentIntent.Type),
		edge.To("payments", Payment.Type),
		edge.To("assignments", OrderAssignment.Type),
		edge.From("customer", User.Type).
			Ref("orders").
			Field("customer_id").
			Unique().
			Required(),
		edge.From("delivery_address", CustomerAddress.Type).
			Ref("orders").
			Field("delivery_address_id").
			Unique(),
	}
}

// Annotations of the Order.
func (Order) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "orders",
		},
	}
}

// Indexes of the Order.
func (Order) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "cafe_id"),
		index.Fields("tenant_id", "customer_id"),
		index.Fields("tenant_id", "status"),
		index.Fields("order_number").
			Unique(),
		// Partial unique index: unique idempotency key when set
		index.Fields("idempotency_key").
			Annotations(entsql.IndexAnnotation{
				Where: "idempotency_key IS NOT NULL",
			}).
			Unique(),
		index.Fields("placed_at"),
		index.Fields("created_at"),
	}
}
