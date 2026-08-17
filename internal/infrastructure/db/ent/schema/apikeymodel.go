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

type APIKeyModel struct {
	ent.Schema
}

func (APIKeyModel) Annotations() []entschema.Annotation {
	return []entschema.Annotation{entsql.Annotation{Table: "api_key_models"}}
}

func (APIKeyModel) Mixin() []ent.Mixin {
	return []ent.Mixin{UUIDMixin{}, CreatedAtMixin{}}
}

func (APIKeyModel) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("api_key_id", uuid.UUID{}).Immutable(),
		field.UUID("model_id", uuid.UUID{}).Immutable(),
	}
}

func (APIKeyModel) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("api_key", APIKey.Type).
			Ref("models").
			Field("api_key_id").
			Unique().
			Required().
			Immutable().
			Annotations(entsql.OnDelete(entsql.Restrict)),
		edge.From("model", Model.Type).
			Ref("api_key_models").
			Field("model_id").
			Unique().
			Required().
			Immutable().
			Annotations(entsql.OnDelete(entsql.Restrict)),
	}
}

func (APIKeyModel) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("api_key_id", "model_id").Unique(),
		index.Fields("model_id"),
	}
}
