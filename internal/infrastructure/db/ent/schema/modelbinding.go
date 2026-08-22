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

type ModelBinding struct {
	ent.Schema
}

func (ModelBinding) Annotations() []entschema.Annotation {
	return []entschema.Annotation{entsql.Annotation{Table: "model_bindings"}}
}

func (ModelBinding) Mixin() []ent.Mixin {
	return []ent.Mixin{UUIDMixin{}, TimeMixin{}}
}

func (ModelBinding) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("model_id", uuid.UUID{}).Immutable(),
		field.UUID("provider_id", uuid.UUID{}).Immutable(),
		field.String("upstream_model_name").NotEmpty().MaxLen(256),
		field.Enum("adapter").Values(
			"openai_compatible",
			"openai_responses",
			"openai_embeddings",
			"openai_images",
			"openai_audio",
			"openai_video",
			"openai_realtime",
			"openai_moderations",
			"anthropic",
			"cohere_rerank_v2",
			"google_gemini_v1beta",
		),
		field.Int("priority").Default(100).NonNegative(),
		field.Enum("status").Values("active", "inactive").Default("active"),
	}
}

func (ModelBinding) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("model", Model.Type).
			Ref("bindings").
			Field("model_id").
			Unique().
			Required().
			Immutable().
			Annotations(entsql.OnDelete(entsql.Restrict)),
		edge.From("provider", Provider.Type).
			Ref("model_bindings").
			Field("provider_id").
			Unique().
			Required().
			Immutable().
			Annotations(entsql.OnDelete(entsql.Restrict)),
	}
}

func (ModelBinding) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("model_id", "provider_id", "adapter").Unique(),
		index.Fields("model_id", "status", "priority"),
		index.Fields("provider_id", "status"),
	}
}
