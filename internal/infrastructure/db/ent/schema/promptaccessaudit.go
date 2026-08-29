package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type PromptAccessAudit struct{ ent.Schema }

func (PromptAccessAudit) Annotations() []entschema.Annotation {
	return []entschema.Annotation{entsql.Annotation{Table: "prompt_access_audits"}}
}
func (PromptAccessAudit) Mixin() []ent.Mixin { return []ent.Mixin{UUIDMixin{}, CreatedAtMixin{}} }
func (PromptAccessAudit) Fields() []ent.Field {
	return []ent.Field{field.UUID("actor_user_id", uuid.UUID{}).Immutable(), field.UUID("project_id", uuid.UUID{}).Optional().Nillable().Immutable(), field.UUID("monitored_input_id", uuid.UUID{}).Optional().Nillable().Immutable(), field.String("access_type").MaxLen(32).Immutable(), field.JSON("query_scope", map[string]any{}).Optional(), field.JSON("result_ids", []string{}).Optional(), field.Int("result_count").NonNegative().Default(0)}
}
func (PromptAccessAudit) Indexes() []ent.Index {
	return []ent.Index{index.Fields("actor_user_id", "created_at"), index.Fields("monitored_input_id", "created_at")}
}
