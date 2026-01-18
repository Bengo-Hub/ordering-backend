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

// DataDeletionJob holds the schema definition for the DataDeletionJob entity.
// This stores data deletion jobs for GDPR right to be forgotten compliance.
type DataDeletionJob struct {
	ent.Schema
}

// Fields of the DataDeletionJob.
func (DataDeletionJob) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}).
			Comment("Reference to tenant"),
		field.UUID("user_id", uuid.UUID{}).
			Comment("Reference to user whose data is being deleted"),
		field.Enum("deletion_type").
			Values("soft", "permanent", "anonymize").
			Default("soft").
			Comment("Type of deletion: soft (deactivate), permanent (hard delete), or anonymize"),
		field.Enum("status").
			Values("pending", "scheduled", "in_progress", "completed", "failed", "cancelled").
			Default("pending").
			Comment("Current status of the deletion job"),
		field.Text("reason").
			Optional().
			Comment("User-provided reason for deletion"),
		field.Bool("confirmed").
			Default(false).
			Comment("User confirmed the deletion request"),
		field.Int("retention_days").
			Default(30).
			Comment("Days to wait before permanent deletion (for soft delete)"),
		field.Text("error_message").
			Optional().
			Comment("Error message if job failed"),
		field.JSON("deletion_summary", map[string]int{}).
			Optional().
			Comment("Summary of deleted records by type"),
		field.Time("requested_at").
			Default(time.Now).
			Comment("When the deletion was requested"),
		field.Time("scheduled_for").
			Optional().
			Nillable().
			Comment("When permanent deletion is scheduled (after retention period)"),
		field.Time("started_at").
			Optional().
			Nillable().
			Comment("When processing started"),
		field.Time("completed_at").
			Optional().
			Nillable().
			Comment("When processing completed"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the DataDeletionJob.
func (DataDeletionJob) Edges() []ent.Edge {
	return []ent.Edge{}
}

// Annotations of the DataDeletionJob.
func (DataDeletionJob) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "data_deletion_jobs",
		},
	}
}

// Indexes of the DataDeletionJob.
func (DataDeletionJob) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "user_id"),
		index.Fields("tenant_id", "status"),
		index.Fields("user_id"),
		index.Fields("status"),
		index.Fields("requested_at"),
		index.Fields("scheduled_for"),
	}
}
