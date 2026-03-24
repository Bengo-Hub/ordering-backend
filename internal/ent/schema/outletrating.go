package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// OutletRating holds the materialized rating aggregate for an outlet.
type OutletRating struct {
	ent.Schema
}

// Fields of the OutletRating.
func (OutletRating) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("tenant_id", uuid.UUID{}),
		field.UUID("outlet_id", uuid.UUID{}).Unique().Comment("One rating aggregate per outlet"),
		field.Float("average_rating").Default(0).Comment("Weighted average rating 1-5"),
		field.Int("total_ratings").Default(0).Comment("Total number of ratings received"),
		field.Int("total_reviews").Default(0).Comment("Total reviews with comments"),
		field.Int("five_star").Default(0),
		field.Int("four_star").Default(0),
		field.Int("three_star").Default(0),
		field.Int("two_star").Default(0),
		field.Int("one_star").Default(0),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

// Indexes of the OutletRating.
func (OutletRating) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "outlet_id").Unique(),
		index.Fields("average_rating"),
	}
}
