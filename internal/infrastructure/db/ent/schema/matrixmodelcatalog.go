package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type MatrixModelCatalog struct{ ent.Schema }

func (MatrixModelCatalog) Annotations() []entschema.Annotation {
	return []entschema.Annotation{entsql.Annotation{Table: "matrix_model_catalog"}}
}

func (MatrixModelCatalog) Mixin() []ent.Mixin { return []ent.Mixin{UUIDMixin{}, TimeMixin{}} }

func (MatrixModelCatalog) Fields() []ent.Field {
	return []ent.Field{
		field.String("model_id").NotEmpty().MaxLen(256).SchemaType(map[string]string{dialect.Postgres: "citext"}).Unique(),
		field.String("description").Optional().Nillable().MaxLen(4096),
		field.JSON("owned_by", []string{}).Default([]string{}),
		field.JSON("model_types", []string{}).Default([]string{}),
		field.JSON("supported_endpoint_types", []string{}).Default([]string{}),
		field.JSON("tags", []string{}).Default([]string{}),
		field.JSON("raw_entries", []map[string]any{}).Default([]map[string]any{}),
		field.Enum("status").Values("active", "inactive").Default("active"),
		field.Time("last_seen_at"),
		field.Time("synced_at"),
	}
}

func (MatrixModelCatalog) Indexes() []ent.Index {
	return []ent.Index{index.Fields("status", "model_id")}
}
