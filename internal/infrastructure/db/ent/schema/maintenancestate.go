package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

type MaintenanceState struct{ ent.Schema }

func (MaintenanceState) Annotations() []entschema.Annotation {
	return []entschema.Annotation{entsql.Annotation{Table: "maintenance_state"}}
}
func (MaintenanceState) Mixin() []ent.Mixin { return []ent.Mixin{UUIDMixin{}, TimeMixin{}} }
func (MaintenanceState) Fields() []ent.Field {
	return []ent.Field{field.String("name").NotEmpty().MaxLen(64).Unique(), field.Time("last_success_at").Optional().Nillable(), field.String("last_error").Optional().Nillable().MaxLen(1000)}
}
