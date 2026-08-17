package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type User struct {
	ent.Schema
}

func (User) Annotations() []entschema.Annotation {
	return []entschema.Annotation{entsql.Annotation{Table: "users"}}
}

func (User) Mixin() []ent.Mixin {
	return []ent.Mixin{UUIDMixin{}, TimeMixin{}}
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty().MaxLen(64),
		field.String("email").
			NotEmpty().
			MaxLen(254).
			SchemaType(map[string]string{dialect.Postgres: "citext"}).
			Unique(),
		field.String("password_hash").NotEmpty().MaxLen(255).Sensitive(),
		field.Enum("role").Values("student", "mentor", "teacher").Immutable(),
		field.Enum("status").
			Values("pending_invite", "active", "disabled").
			Default("active"),
	}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("organization_memberships", OrganizationMember.Type),
		edge.To("project_memberships", ProjectMember.Type),
		edge.To("mentor_project_applications", MentorProjectApplication.Type),
		edge.To("reviewed_project_applications", MentorProjectApplication.Type),
		edge.To("api_keys", APIKey.Type),
		edge.To("api_key_audits", APIKeyAudit.Type),
	}
}

func (User) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("role", "status"),
	}
}
