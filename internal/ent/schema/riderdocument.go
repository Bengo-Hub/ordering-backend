package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// RiderDocument holds the schema definition for the RiderDocument entity.
type RiderDocument struct {
	ent.Schema
}

// Fields of the RiderDocument.
func (RiderDocument) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.String("document_type").
			NotEmpty(),
		field.String("file_url").
			NotEmpty(),
		field.String("status").
			Default("pending"),
		field.Time("expires_at").
			Optional(),
		field.Time("verified_at").
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

// Edges of the RiderDocument.
func (RiderDocument) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("rider", RiderProfile.Type).
			Ref("documents").
			Required(),
		edge.From("reviewer", User.Type).
			Ref("reviewed_documents"),
	}
}
