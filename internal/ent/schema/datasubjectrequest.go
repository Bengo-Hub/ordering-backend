package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// DataSubjectRequest holds the schema definition for the DataSubjectRequest entity.
// This stores GDPR/DPA data subject requests for compliance tracking.
type DataSubjectRequest struct {
	ent.Schema
}

// Fields of the DataSubjectRequest.
func (DataSubjectRequest) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}).
			Comment("Reference to tenant"),
		field.UUID("user_id", uuid.UUID{}).
			Comment("Reference to user who submitted the request"),
		field.Enum("request_type").
			Values("export", "delete", "access", "rectification", "restriction", "portability", "objection").
			Comment("Type of data subject request"),
		field.Enum("status").
			Values("pending", "in_progress", "completed", "rejected", "cancelled").
			Default("pending").
			Comment("Current status of the request"),
		field.Text("description").
			Optional().
			Comment("User-provided description or reason"),
		field.Text("notes").
			Optional().
			Comment("Admin notes or processing comments"),
		field.Text("result_url").
			Optional().
			Comment("URL to download result (for export requests)"),
		field.Time("submitted_at").
			Default(time.Now).
			Comment("When the request was submitted"),
		field.Time("processed_at").
			Optional().
			Nillable().
			Comment("When the request was processed"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the DataSubjectRequest.
func (DataSubjectRequest) Edges() []ent.Edge {
	return []ent.Edge{}
}

// Annotations of the DataSubjectRequest.
func (DataSubjectRequest) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "data_subject_requests",
		},
	}
}

// Indexes of the DataSubjectRequest.
func (DataSubjectRequest) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "user_id"),
		index.Fields("tenant_id", "request_type"),
		index.Fields("tenant_id", "status"),
		index.Fields("user_id"),
		index.Fields("status"),
		index.Fields("submitted_at"),
	}
}
