package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// TenantSyncEvent holds webhook delivery metadata to downstream services.
type TenantSyncEvent struct {
	ent.Schema
}

// Fields of the TenantSyncEvent.
func (TenantSyncEvent) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}),
		field.String("tenant_slug").
			NotEmpty().
			Immutable(),
		field.String("destination_service").
			NotEmpty(),
		field.JSON("payload", map[string]any{}).
			Default(map[string]any{}),
		field.String("status").
			Default("pending"),
		field.Int("attempts").
			Default(0),
		field.Time("synced_at").
			Optional(),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the TenantSyncEvent.
func (TenantSyncEvent) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("tenant", Tenant.Type).
			Ref("sync_events").
			Field("tenant_id").
			Required().
			Unique(),
	}
}

// Indexes of the TenantSyncEvent.
func (TenantSyncEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "destination_service").Unique(),
	}
}
