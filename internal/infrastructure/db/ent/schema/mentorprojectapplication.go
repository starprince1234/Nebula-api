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

type MentorProjectApplication struct {
	ent.Schema
}

func (MentorProjectApplication) Annotations() []entschema.Annotation {
	return []entschema.Annotation{entsql.Annotation{
		Table: "mentor_project_applications",
		Checks: map[string]string{
			"mentor_project_application_review_state": `
				(status IN ('pending', 'cancelled') AND reviewed_by IS NULL AND reviewed_at IS NULL)
				OR (status IN ('approved', 'rejected') AND reviewed_by IS NOT NULL AND reviewed_at IS NOT NULL)
			`,
			"mentor_project_application_rejection_reason": `
				status <> 'rejected' OR (review_comment IS NOT NULL AND btrim(review_comment) <> '')
			`,
		},
	}}
}

func (MentorProjectApplication) Mixin() []ent.Mixin {
	return []ent.Mixin{UUIDMixin{}, CreatedAtMixin{}}
}

func (MentorProjectApplication) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("project_id", uuid.UUID{}).Immutable(),
		field.UUID("mentor_id", uuid.UUID{}).Immutable(),
		field.Enum("status").
			Values("pending", "approved", "rejected", "cancelled").
			Default("pending"),
		field.UUID("reviewed_by", uuid.UUID{}).Optional().Nillable(),
		field.String("review_comment").Optional().Nillable().MaxLen(1000),
		field.Time("reviewed_at").Optional().Nillable(),
	}
}

func (MentorProjectApplication) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("project", Project.Type).
			Ref("mentor_applications").
			Field("project_id").
			Unique().
			Required().
			Immutable().
			Annotations(entsql.OnDelete(entsql.Restrict)),
		edge.From("mentor", User.Type).
			Ref("mentor_project_applications").
			Field("mentor_id").
			Unique().
			Required().
			Immutable().
			Annotations(entsql.OnDelete(entsql.Restrict)),
		edge.From("reviewer", User.Type).
			Ref("reviewed_project_applications").
			Field("reviewed_by").
			Unique().
			Annotations(entsql.OnDelete(entsql.Restrict)),
	}
}

func (MentorProjectApplication) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("project_id", "mentor_id").
			Unique().
			Annotations(entsql.IndexWhere("status = 'pending'")),
		index.Fields("status", "created_at"),
		index.Fields("mentor_id", "created_at"),
	}
}
