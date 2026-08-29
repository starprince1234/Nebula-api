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

type MonitoredInput struct{ ent.Schema }

func (MonitoredInput) Annotations() []entschema.Annotation {
	return []entschema.Annotation{entsql.Annotation{Table: "monitored_inputs"}}
}
func (MonitoredInput) Mixin() []ent.Mixin { return []ent.Mixin{UUIDMixin{}, CreatedAtMixin{}} }
func (MonitoredInput) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("call_id", uuid.UUID{}).Immutable().Unique(), field.UUID("project_id", uuid.UUID{}).Immutable(), field.UUID("user_id", uuid.UUID{}).Immutable(),
		field.String("source").MaxLen(64), field.Text("content").Sensitive(), field.Int64("content_bytes").NonNegative(), field.Bool("visible").Default(false),
	}
}
func (MonitoredInput) Edges() []ent.Edge {
	return []ent.Edge{edge.From("call", GatewayCall.Type).Ref("monitored_input").Field("call_id").Unique().Required().Immutable().Annotations(entsql.OnDelete(entsql.Restrict))}
}
func (MonitoredInput) Indexes() []ent.Index {
	return []ent.Index{index.Fields("project_id", "created_at"), index.Fields("visible", "created_at")}
}
