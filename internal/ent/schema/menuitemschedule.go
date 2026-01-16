package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// MenuItemSchedule holds the schema definition for the MenuItemSchedule entity.
// Defines availability windows for menu items (e.g., breakfast items only 6am-11am).
type MenuItemSchedule struct {
	ent.Schema
}

// Fields of the MenuItemSchedule.
func (MenuItemSchedule) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("menu_item_id", uuid.UUID{}).
			Comment("Reference to parent menu item"),
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

// Edges of the MenuItemSchedule.
func (MenuItemSchedule) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("menu_item", MenuItem.Type).
			Ref("schedules").
			Field("menu_item_id").
			Unique().
			Required(),
	}
}

// Indexes of the MenuItemSchedule.
func (MenuItemSchedule) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("menu_item_id"),
		index.Fields("day_of_week"),
	}
}
