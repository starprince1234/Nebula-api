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

type Provider struct {
	ent.Schema
}

func (Provider) Annotations() []entschema.Annotation {
	return []entschema.Annotation{entsql.Annotation{Table: "providers"}}
}

func (Provider) Mixin() []ent.Mixin {
	return []ent.Mixin{UUIDMixin{}, TimeMixin{}}
}

func (Provider) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			NotEmpty().
			MaxLen(128).
			SchemaType(map[string]string{dialect.Postgres: "citext"}).
			Unique(),
		field.String("base_url").NotEmpty().MaxLen(2048),
		field.Bytes("credential_ciphertext").NotEmpty().Sensitive(),
		field.Enum("status").Values("active", "inactive").Default("active"),
	}
}

func (Provider) Edges() []ent.Edge {
	return []ent.Edge{edge.To("model_bindings", ModelBinding.Type)}
}

func (Provider) Indexes() []ent.Index {
	return []ent.Index{index.Fields("status")}
}
