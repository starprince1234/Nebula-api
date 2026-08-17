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

type APIKeyAudit struct {
	ent.Schema
}

func (APIKeyAudit) Annotations() []entschema.Annotation {
	return []entschema.Annotation{entsql.Annotation{
		Table: "api_key_audits",
		Checks: map[string]string{
			"api_key_audit_reason_required": `
				action NOT IN ('mentor_rejected', 'teacher_rejected', 'mentor_revoked')
				OR (comment IS NOT NULL AND btrim(comment) <> '')
			`,
		},
	}}
}

func (APIKeyAudit) Mixin() []ent.Mixin {
	return []ent.Mixin{UUIDMixin{}, CreatedAtMixin{}}
}

func (APIKeyAudit) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("api_key_id", uuid.UUID{}).Immutable(),
		field.UUID("actor_user_id", uuid.UUID{}).Immutable(),
		field.Enum("action").
			Values("mentor_approved", "mentor_rejected", "teacher_approved", "teacher_rejected", "mentor_revoked").
			Immutable(),
		field.String("comment").Optional().Nillable().MaxLen(1000).Immutable(),
	}
}

func (APIKeyAudit) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("api_key", APIKey.Type).
			Ref("audits").
			Field("api_key_id").
			Unique().
			Required().
			Immutable().
			Annotations(entsql.OnDelete(entsql.Restrict)),
		edge.From("actor", User.Type).
			Ref("api_key_audits").
			Field("actor_user_id").
			Unique().
			Required().
			Immutable().
			Annotations(entsql.OnDelete(entsql.Restrict)),
	}
}

func (APIKeyAudit) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("api_key_id", "created_at"),
		index.Fields("actor_user_id", "created_at"),
	}
}
