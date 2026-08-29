package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type MonthlyUsageCube struct{ ent.Schema }

func (MonthlyUsageCube) Annotations() []entschema.Annotation {
	return []entschema.Annotation{entsql.Annotation{Table: "monthly_usage_cube"}}
}
func (MonthlyUsageCube) Mixin() []ent.Mixin { return []ent.Mixin{UUIDMixin{}, TimeMixin{}} }
func (MonthlyUsageCube) Fields() []ent.Field {
	return []ent.Field{
		field.Time("month").Immutable(), field.UUID("organization_id", uuid.UUID{}).Immutable(), field.UUID("project_id", uuid.UUID{}).Immutable(),
		field.UUID("user_id", uuid.UUID{}).Immutable(), field.UUID("api_key_id", uuid.UUID{}).Immutable(), field.UUID("model_id", uuid.UUID{}).Immutable(),
		field.String("organization_name").MaxLen(128), field.String("project_name").MaxLen(128), field.String("user_name").MaxLen(64),
		field.String("api_key_name").MaxLen(128), field.String("model_name").MaxLen(256),
		field.Int64("charged_milli").NonNegative().Default(0), field.Int64("charged_count").NonNegative().Default(0), field.Int64("zero_cost_count").NonNegative().Default(0),
	}
}
func (MonthlyUsageCube) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("month", "project_id", "user_id", "api_key_id", "model_id").Unique(), index.Fields("month", "api_key_id"), index.Fields("month", "project_id"),
	}
}
