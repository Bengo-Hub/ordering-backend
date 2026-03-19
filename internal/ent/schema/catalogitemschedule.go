package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// CatalogItemSchedule holds the schema definition for the CatalogItemSchedule entity.
// Defines availability windows for catalog items (e.g., breakfast items only 6am-11am).
type CatalogItemSchedule struct {
	ent.Schema
}

// Fields of the CatalogItemSchedule.
func (CatalogItemSchedule) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("catalog_item_id", uuid.UUID{}).
			Comment("Reference to parent catalog item"),
		field.Int("day_of_week").
			Min(0).
			Max(6).
			Comment("Day of week: 0=Sunday, 6=Saturday"),
		field.String("time_start").
			NotEmpty().
			MaxLen(5).
			Comment("Start time in HH:MM format"),
		field.String("time_end").
			NotEmpty().
			MaxLen(5).
			Comment("End time in HH:MM format"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

// Edges of the CatalogItemSchedule.
func (CatalogItemSchedule) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("catalog_item", CatalogItem.Type).
			Ref("schedules").
			Field("catalog_item_id").
			Unique().
			Required(),
	}
}

// Indexes of the CatalogItemSchedule.
func (CatalogItemSchedule) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("catalog_item_id"),
		index.Fields("day_of_week"),
	}
}
