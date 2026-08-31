ALTER TABLE models ADD COLUMN max_input_tokens integer CHECK(max_input_tokens>0);
UPDATE models SET capabilities='[]'::jsonb;

CREATE TABLE matrix_model_catalog(
  id uuid PRIMARY KEY,
  model_id citext NOT NULL UNIQUE,
  description varchar(4096),
  owned_by jsonb NOT NULL DEFAULT '[]'::jsonb,
  model_types jsonb NOT NULL DEFAULT '[]'::jsonb,
  supported_endpoint_types jsonb NOT NULL DEFAULT '[]'::jsonb,
  tags jsonb NOT NULL DEFAULT '[]'::jsonb,
  raw_entries jsonb NOT NULL DEFAULT '[]'::jsonb,
  status varchar(16) NOT NULL DEFAULT 'active' CHECK(status IN ('active','inactive')),
  last_seen_at timestamptz NOT NULL,
  synced_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);
CREATE INDEX matrix_model_catalog_status_model_id ON matrix_model_catalog(status,model_id);
