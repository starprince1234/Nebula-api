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

type Project struct {
	ent.Schema
}

func (Project) Annotations() []entschema.Annotation {
	return []entschema.Annotation{entsql.Annotation{Table: "projects"}}
}

func (Project) Mixin() []ent.Mixin {
	return []ent.Mixin{UUIDMixin{}, TimeMixin{}}
}

func (Project) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("organization_id", uuid.UUID{}).Immutable(),
		field.String("name").
			NotEmpty().
			MaxLen(128).
			SchemaType(map[string]string{dialect.Postgres: "citext"}),
		field.String("description").Optional().Nillable().MaxLen(1024),
		field.Enum("status").Values("active", "inactive").Default("active"),
	}
}

func (Project) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("organization", Organization.Type).
			Ref("projects").
			Field("organization_id").
			Unique().
			Required().
			Immutable().
			Annotations(entsql.OnDelete(entsql.Restrict)),
		edge.To("memberships", ProjectMember.Type),
		edge.To("mentor_applications", MentorProjectApplication.Type),
		edge.To("api_keys", APIKey.Type),
	}
}

func (Project) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("organization_id", "name").Unique(),
		index.Fields("organization_id", "status"),
	}
}
