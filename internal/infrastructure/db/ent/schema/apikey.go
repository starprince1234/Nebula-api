package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type APIKey struct {
	ent.Schema
}

func (APIKey) Annotations() []entschema.Annotation {
	return []entschema.Annotation{entsql.Annotation{
		Table: "api_keys",
		Checks: map[string]string{
			"api_key_material_matches_status": `
				(status IN ('pending_mentor', 'pending_teacher', 'approved', 'rejected')
					AND key_hash IS NULL AND key_prefix IS NULL AND claimed_at IS NULL AND revoked_at IS NULL)
				OR (status = 'active'
					AND key_hash IS NOT NULL AND key_prefix IS NOT NULL AND claimed_at IS NOT NULL AND revoked_at IS NULL)
				OR (status = 'revoked'
					AND key_hash IS NOT NULL AND key_prefix IS NOT NULL AND claimed_at IS NOT NULL AND revoked_at IS NOT NULL)
			`,
		},
	}}
}

func (APIKey) Mixin() []ent.Mixin {
	return []ent.Mixin{UUIDMixin{}, TimeMixin{}}
}

func (APIKey) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("owner_user_id", uuid.UUID{}).Immutable(),
		field.UUID("project_id", uuid.UUID{}).Immutable(),
		field.String("name").
			NotEmpty().
			MaxLen(128).
			SchemaType(map[string]string{dialect.Postgres: "citext"}),
		field.Enum("status").
			Values("pending_mentor", "pending_teacher", "approved", "active", "rejected", "revoked").
			Default("pending_mentor"),
		field.Bytes("key_hash").Optional().Nillable().Sensitive().Unique(),
		field.String("key_prefix").Optional().Nillable().MaxLen(32),
		field.Time("claimed_at").Optional().Nillable(),
		field.Time("revoked_at").Optional().Nillable(),
		field.Int64("requested_monthly_credit_quota_milli").NonNegative().Max(1_000_000_000_000),
		field.Int64("mentor_monthly_credit_quota_milli").Optional().Nillable().NonNegative().Max(1_000_000_000_000),
		field.Int64("monthly_credit_quota_milli").Optional().Nillable().NonNegative().Max(1_000_000_000_000),
	}
}

func (APIKey) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("owner", User.Type).
			Ref("api_keys").
			Field("owner_user_id").
			Unique().
			Required().
			Immutable().
			Annotations(entsql.OnDelete(entsql.Restrict)),
		edge.From("project", Project.Type).
			Ref("api_keys").
			Field("project_id").
			Unique().
			Required().
			Immutable().
			Annotations(entsql.OnDelete(entsql.Restrict)),
		edge.To("models", APIKeyModel.Type),
		edge.To("audits", APIKeyAudit.Type),
		edge.To("credit_buckets", APIKeyMonthCreditBucket.Type),
	}
}

func (APIKey) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("owner_user_id", "name").
			Unique().
			Annotations(entsql.IndexWhere("status IN ('pending_mentor', 'pending_teacher', 'approved', 'active')")),
		index.Fields("owner_user_id", "created_at"),
		index.Fields("project_id", "status", "created_at"),
		index.Fields("status", "created_at"),
	}
}
