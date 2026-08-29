package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type QuotaAdjustmentAudit struct{ ent.Schema }

func (QuotaAdjustmentAudit) Annotations() []entschema.Annotation {
	return []entschema.Annotation{entsql.Annotation{Table: "quota_adjustment_audits"}}
}
func (QuotaAdjustmentAudit) Mixin() []ent.Mixin { return []ent.Mixin{UUIDMixin{}, CreatedAtMixin{}} }
func (QuotaAdjustmentAudit) Fields() []ent.Field {
	return []ent.Field{field.Enum("owner_type").Values("project", "api_key").Immutable(), field.UUID("owner_id", uuid.UUID{}).Immutable(), field.UUID("actor_user_id", uuid.UUID{}).Immutable(), field.Int64("old_quota_milli").NonNegative().Immutable(), field.Int64("new_quota_milli").NonNegative().Immutable(), field.String("reason").NotEmpty().MaxLen(1000).Immutable()}
}
func (QuotaAdjustmentAudit) Indexes() []ent.Index {
	return []ent.Index{index.Fields("owner_type", "owner_id", "created_at"), index.Fields("actor_user_id", "created_at")}
}
