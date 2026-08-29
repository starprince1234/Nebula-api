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

type APIKeyMonthCreditBucket struct{ ent.Schema }

func (APIKeyMonthCreditBucket) Annotations() []entschema.Annotation {
	return []entschema.Annotation{entsql.Annotation{Table: "api_key_month_credit_buckets"}}
}
func (APIKeyMonthCreditBucket) Mixin() []ent.Mixin { return []ent.Mixin{UUIDMixin{}, TimeMixin{}} }
func (APIKeyMonthCreditBucket) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("api_key_id", uuid.UUID{}).Immutable(), field.UUID("project_id", uuid.UUID{}).Immutable(), field.Time("month").Immutable(),
		field.Int64("quota_milli").NonNegative(), field.Bool("allocation_active").Default(true),
		field.Int64("charged_milli").NonNegative().Default(0), field.Int64("pending_milli").NonNegative().Default(0),
	}
}
func (APIKeyMonthCreditBucket) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("api_key", APIKey.Type).Ref("credit_buckets").Field("api_key_id").Unique().Required().Immutable().Annotations(entsql.OnDelete(entsql.Restrict)),
		edge.From("project", Project.Type).Ref("api_key_credit_buckets").Field("project_id").Unique().Required().Immutable().Annotations(entsql.OnDelete(entsql.Restrict)),
	}
}
func (APIKeyMonthCreditBucket) Indexes() []ent.Index {
	return []ent.Index{index.Fields("api_key_id", "month").Unique(), index.Fields("project_id", "month")}
}
