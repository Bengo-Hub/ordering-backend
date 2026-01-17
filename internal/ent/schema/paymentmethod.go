package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// PaymentMethod holds the schema definition for the PaymentMethod entity.
type PaymentMethod struct {
	ent.Schema
}

// Fields of the PaymentMethod.
func (PaymentMethod) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}).
			Comment("Reference to tenant"),
		field.UUID("user_id", uuid.UUID{}).
			Comment("Reference to user"),
		field.Enum("provider").
			Values("mpesa", "stripe", "paystack", "flutterwave", "manual").
			Comment("Payment provider"),
		field.Enum("type").
			Values("mobile_money", "card", "bank_account", "wallet", "cash").
			Comment("Payment method type"),
		field.String("mask").
			MaxLen(50).
			Optional().
			Comment("Masked identifier (last 4 digits, phone suffix, etc.)"),
		field.String("label").
			MaxLen(100).
			Optional().
			Comment("User-friendly label (e.g., 'My M-Pesa', 'Visa ending 4242')"),
		field.Int("exp_month").
			Optional().
			Nillable().
			Comment("Expiration month for cards"),
		field.Int("exp_year").
			Optional().
			Nillable().
			Comment("Expiration year for cards"),
		field.Bool("is_default").
			Default(false).
			Comment("Whether this is the default payment method"),
		field.String("fingerprint").
			MaxLen(255).
			Optional().
			Comment("Provider-specific fingerprint for deduplication"),
		field.String("provider_token").
			MaxLen(500).
			Optional().
			Sensitive().
			Comment("Tokenized payment method from provider"),
		field.JSON("metadata", map[string]any{}).
			Optional().
			Comment("Additional provider-specific metadata"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the PaymentMethod.
func (PaymentMethod) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("payment_methods").
			Field("user_id").
			Unique().
			Required(),
		edge.To("payment_intents", PaymentIntent.Type),
	}
}

// Indexes of the PaymentMethod.
func (PaymentMethod) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "user_id"),
		index.Fields("tenant_id", "fingerprint").
			Unique(),
		index.Fields("provider", "provider_token"),
	}
}
