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

// DataExportJob holds the schema definition for the DataExportJob entity.
// This stores data export jobs for GDPR data portability compliance.
type DataExportJob struct {
	ent.Schema
}

// Fields of the DataExportJob.
func (DataExportJob) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}).
			Comment("Reference to tenant"),
		field.UUID("user_id", uuid.UUID{}).
			Comment("Reference to user whose data is being exported"),
		field.Enum("format").
			Values("json", "csv").
			Default("json").
			Comment("Export format"),
		field.Enum("status").
			Values("pending", "in_progress", "completed", "failed", "expired").
			Default("pending").
			Comment("Current status of the export job"),
		field.JSON("included_data", []string{}).
			Optional().
			Comment("List of data types to include (orders, addresses, etc.)"),
		field.Text("storage_url").
			Optional().
			Comment("URL to download the exported data"),
		field.Text("error_message").
			Optional().
			Comment("Error message if job failed"),
		field.Int("file_size_bytes").
			Optional().
			Nillable().
			Comment("Size of the exported file in bytes"),
		field.Int("records_exported").
			Optional().
			Nillable().
			Comment("Number of records exported"),
		field.Time("requested_at").
			Default(time.Now).
			Comment("When the export was requested"),
		field.Time("started_at").
			Optional().
			Nillable().
			Comment("When processing started"),
		field.Time("completed_at").
			Optional().
			Nillable().
			Comment("When processing completed"),
		field.Time("expires_at").
			Optional().
			Nillable().
			Comment("When the download link expires"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the DataExportJob.
func (DataExportJob) Edges() []ent.Edge {
	return []ent.Edge{}
}

// Annotations of the DataExportJob.
func (DataExportJob) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "data_export_jobs",
		},
	}
}

// Indexes of the DataExportJob.
func (DataExportJob) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "user_id"),
		index.Fields("tenant_id", "status"),
		index.Fields("user_id"),
		index.Fields("status"),
		index.Fields("requested_at"),
		index.Fields("expires_at"),
	}
}
