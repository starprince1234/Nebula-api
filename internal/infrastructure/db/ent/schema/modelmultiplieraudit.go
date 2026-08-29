package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type ModelMultiplierAudit struct{ ent.Schema }

func (ModelMultiplierAudit) Annotations() []entschema.Annotation {
	return []entschema.Annotation{entsql.Annotation{Table: "model_multiplier_audits"}}
}
func (ModelMultiplierAudit) Mixin() []ent.Mixin { return []ent.Mixin{UUIDMixin{}, CreatedAtMixin{}} }
func (ModelMultiplierAudit) Fields() []ent.Field {
	return []ent.Field{field.UUID("model_id", uuid.UUID{}).Immutable(), field.UUID("actor_user_id", uuid.UUID{}).Immutable(), field.Int64("old_multiplier_milli").Optional().Nillable().NonNegative().Immutable(), field.Int64("new_multiplier_milli").NonNegative().Immutable(), field.String("reason").NotEmpty().MaxLen(1000).Immutable()}
}
func (ModelMultiplierAudit) Indexes() []ent.Index {
	return []ent.Index{index.Fields("model_id", "created_at"), index.Fields("actor_user_id", "created_at")}
}
