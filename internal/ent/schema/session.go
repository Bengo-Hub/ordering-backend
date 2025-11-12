package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// Session holds the schema definition for the Session entity.
type Session struct {
	ent.Schema
}

// Fields of the Session.
func (Session) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}),
		field.UUID("user_id", uuid.UUID{}),
		field.String("refresh_token_hash").
			NotEmpty().
			Unique(),
		field.String("user_agent").
			Optional(),
		field.String("ip_address").
			Optional(),
		field.UUID("device_id", uuid.UUID{}).
			Optional(),
		field.Time("expires_at").
			Immutable(),
		field.Time("revoked_at").
			Optional(),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Indexes of the Session.
// Edges of the Session.
func (Session) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("tenant", Tenant.Type).
			Ref("sessions").
			Field("tenant_id").
			Required().
			Unique(),
		edge.From("user", User.Type).
			Ref("sessions").
			Field("user_id").
			Required().
			Unique(),
	}
}
