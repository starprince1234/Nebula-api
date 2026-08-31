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

type Model struct {
	ent.Schema
}

func (Model) Annotations() []entschema.Annotation {
	return []entschema.Annotation{entsql.Annotation{
		Table: "models",
		Checks: map[string]string{
			"active_model_requires_credit_multiplier": "status <> 'active' OR credit_multiplier_milli IS NOT NULL",
		},
	}}
}

func (Model) Mixin() []ent.Mixin {
	return []ent.Mixin{UUIDMixin{}, TimeMixin{}}
}

func (Model) Fields() []ent.Field {
	return []ent.Field{
		field.String("model_id").
			NotEmpty().
			MaxLen(256).
			SchemaType(map[string]string{dialect.Postgres: "citext"}).
			Unique(),
		field.String("display_name").NotEmpty().MaxLen(256),
		field.String("description").Optional().Nillable().MaxLen(2048),
		field.Enum("category").
			Values("text", "image", "audio", "video", "multimodal", "embedding", "rerank").
			Default("text"),
		field.JSON("capabilities", []string{}).Default([]string{}),
		field.JSON("input_modalities", []string{}).Default([]string{}),
		field.JSON("output_modalities", []string{}).Default([]string{}),
		field.Int("context_window").Optional().Nillable().Positive(),
		field.Int("max_input_tokens").Optional().Nillable().Positive(),
		field.Int("max_output_tokens").Optional().Nillable().Positive(),
		field.Bool("is_common").Default(false),
		field.Enum("status").
			Values("pending_configuration", "active", "inactive").
			Default("pending_configuration"),
		field.Int64("credit_multiplier_milli").Optional().Nillable().NonNegative().Max(1_000_000_000_000),
	}
}

func (Model) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("bindings", ModelBinding.Type),
		edge.To("api_key_models", APIKeyModel.Type),
	}
}

func (Model) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "is_common"),
		index.Fields("category", "status"),
	}
}
