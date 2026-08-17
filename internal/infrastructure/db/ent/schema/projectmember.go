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

type ProjectMember struct {
	ent.Schema
}

func (ProjectMember) Annotations() []entschema.Annotation {
	return []entschema.Annotation{entsql.Annotation{Table: "project_members"}}
}

func (ProjectMember) Mixin() []ent.Mixin {
	return []ent.Mixin{UUIDMixin{}, CreatedAtMixin{}}
}

func (ProjectMember) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("project_id", uuid.UUID{}).Immutable(),
		field.UUID("user_id", uuid.UUID{}).Immutable(),
	}
}

func (ProjectMember) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("project", Project.Type).
			Ref("memberships").
			Field("project_id").
			Unique().
			Required().
			Immutable().
			Annotations(entsql.OnDelete(entsql.Restrict)),
		edge.From("user", User.Type).
			Ref("project_memberships").
			Field("user_id").
			Unique().
			Required().
			Immutable().
			Annotations(entsql.OnDelete(entsql.Restrict)),
	}
}

func (ProjectMember) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("project_id", "user_id").Unique(),
		index.Fields("user_id"),
	}
}
