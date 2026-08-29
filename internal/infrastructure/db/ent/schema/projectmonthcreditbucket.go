package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type ProjectMonthCreditBucket struct{ ent.Schema }

func (ProjectMonthCreditBucket) Annotations() []entschema.Annotation {
	return []entschema.Annotation{entsql.Annotation{Table: "project_month_credit_buckets"}}
}
func (ProjectMonthCreditBucket) Mixin() []ent.Mixin { return []ent.Mixin{UUIDMixin{}, TimeMixin{}} }
func (ProjectMonthCreditBucket) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("project_id", uuid.UUID{}).Immutable(), field.Time("month").Immutable(),
		field.Int64("quota_milli").NonNegative(), field.Int64("allocated_milli").NonNegative().Default(0),
		field.Int64("charged_milli").NonNegative().Default(0), field.Int64("pending_milli").NonNegative().Default(0),
	}
}
func (ProjectMonthCreditBucket) Edges() []ent.Edge {
	return []ent.Edge{edge.From("project", Project.Type).Ref("credit_buckets").Field("project_id").Unique().Required().Immutable().Annotations(entsql.OnDelete(entsql.Restrict))}
}
func (ProjectMonthCreditBucket) Indexes() []ent.Index {
	return []ent.Index{index.Fields("project_id", "month").Unique(), index.Fields("month")}
}
