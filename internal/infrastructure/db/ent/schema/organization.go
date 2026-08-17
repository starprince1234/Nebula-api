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

type Organization struct {
	ent.Schema
}

func (Organization) Annotations() []entschema.Annotation {
	return []entschema.Annotation{entsql.Annotation{Table: "organizations"}}
}

func (Organization) Mixin() []ent.Mixin {
	return []ent.Mixin{UUIDMixin{}, TimeMixin{}}
}

func (Organization) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			NotEmpty().
			MaxLen(128).
			SchemaType(map[string]string{dialect.Postgres: "citext"}).
			Unique(),
		field.String("description").Optional().Nillable().MaxLen(1024),
		field.Enum("status").Values("active", "inactive").Default("active"),
	}
}

func (Organization) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("memberships", OrganizationMember.Type),
		edge.To("projects", Project.Type),
	}
}

func (Organization) Indexes() []ent.Index {
	return []ent.Index{index.Fields("status")}
}
