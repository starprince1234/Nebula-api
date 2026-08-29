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

type GatewayCallAttempt struct{ ent.Schema }

func (GatewayCallAttempt) Annotations() []entschema.Annotation {
	return []entschema.Annotation{entsql.Annotation{Table: "gateway_call_attempts"}}
}
func (GatewayCallAttempt) Mixin() []ent.Mixin { return []ent.Mixin{UUIDMixin{}, CreatedAtMixin{}} }
func (GatewayCallAttempt) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("call_id", uuid.UUID{}).Immutable(), field.UUID("provider_id", uuid.UUID{}).Immutable(), field.UUID("binding_id", uuid.UUID{}).Immutable(),
		field.String("provider_name").MaxLen(128), field.Enum("status").Values("connecting", "succeeded", "failed"),
		field.Int("http_status").Optional().Nillable().NonNegative(), field.String("error_category").Optional().Nillable().MaxLen(64),
		field.String("error_message").Optional().Nillable().MaxLen(1000), field.Int64("latency_ms").NonNegative().Default(0), field.Time("completed_at").Optional().Nillable(),
	}
}
func (GatewayCallAttempt) Edges() []ent.Edge {
	return []ent.Edge{edge.From("call", GatewayCall.Type).Ref("attempts").Field("call_id").Unique().Required().Immutable().Annotations(entsql.OnDelete(entsql.Restrict))}
}
func (GatewayCallAttempt) Indexes() []ent.Index {
	return []ent.Index{index.Fields("call_id", "created_at")}
}
