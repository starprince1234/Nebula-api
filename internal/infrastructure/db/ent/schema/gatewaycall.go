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

type GatewayCall struct{ ent.Schema }

func (GatewayCall) Annotations() []entschema.Annotation {
	return []entschema.Annotation{entsql.Annotation{Table: "gateway_calls"}}
}
func (GatewayCall) Mixin() []ent.Mixin { return []ent.Mixin{UUIDMixin{}, CreatedAtMixin{}} }
func (GatewayCall) Fields() []ent.Field {
	return []ent.Field{
		field.String("request_id").MaxLen(128), field.Time("month").Immutable(), field.UUID("organization_id", uuid.UUID{}), field.UUID("project_id", uuid.UUID{}),
		field.UUID("user_id", uuid.UUID{}), field.UUID("api_key_id", uuid.UUID{}), field.UUID("model_id", uuid.UUID{}), field.UUID("provider_id", uuid.UUID{}).Optional().Nillable(),
		field.String("organization_name").MaxLen(128), field.String("project_name").MaxLen(128), field.String("user_name").MaxLen(64), field.String("api_key_name").MaxLen(128),
		field.String("model_name").MaxLen(256), field.String("provider_name").Optional().Nillable().MaxLen(128), field.String("protocol").MaxLen(64), field.String("request_path").MaxLen(512),
		field.Int64("multiplier_milli").NonNegative(), field.Int64("credit_milli").NonNegative(),
		field.Enum("billing_state").Values("pending", "charged", "failed").Default("pending"),
		field.Enum("outcome").Values("pending", "succeeded", "upstream_failed", "quota_rejected", "outcome_unknown").Default("pending"),
		field.String("error_category").Optional().Nillable().MaxLen(64), field.String("error_message").Optional().Nillable().MaxLen(1000),
		field.Time("lease_expires_at").Optional().Nillable(), field.Time("sent_at").Optional().Nillable(), field.Time("finalized_at").Optional().Nillable(),
	}
}
func (GatewayCall) Edges() []ent.Edge {
	return []ent.Edge{edge.To("attempts", GatewayCallAttempt.Type), edge.To("monitored_input", MonitoredInput.Type).Unique()}
}
func (GatewayCall) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("project_id", "created_at"), index.Fields("api_key_id", "created_at"), index.Fields("billing_state", "lease_expires_at"), index.Fields("created_at", "id"),
	}
}
