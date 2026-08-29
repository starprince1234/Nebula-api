CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE users (
  id uuid PRIMARY KEY, name varchar(64) NOT NULL, email citext NOT NULL UNIQUE,
  password_hash varchar(255) NOT NULL, role varchar(16) NOT NULL CHECK (role IN ('student','mentor','teacher')),
  status varchar(32) NOT NULL DEFAULT 'active' CHECK (status IN ('pending_invite','active','disabled')),
  created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL
);
CREATE INDEX users_role_status ON users(role,status);

CREATE TABLE organizations (
  id uuid PRIMARY KEY, name citext NOT NULL UNIQUE, description varchar(1024),
  status varchar(16) NOT NULL DEFAULT 'active' CHECK (status IN ('active','inactive')),
  created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL
);
CREATE INDEX organizations_status ON organizations(status);

CREATE TABLE organization_members (
  id uuid PRIMARY KEY, organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT, created_at timestamptz NOT NULL,
  UNIQUE(organization_id,user_id)
);
CREATE INDEX organization_members_user_id ON organization_members(user_id);

CREATE TABLE projects (
  id uuid PRIMARY KEY, organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
  name citext NOT NULL, description varchar(1024), status varchar(16) NOT NULL DEFAULT 'active' CHECK (status IN ('active','inactive')),
  created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL, UNIQUE(organization_id,name)
);
CREATE INDEX projects_organization_id_status ON projects(organization_id,status);

CREATE TABLE project_members (
  id uuid PRIMARY KEY, project_id uuid NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT, created_at timestamptz NOT NULL,
  UNIQUE(project_id,user_id)
);
CREATE INDEX project_members_user_id ON project_members(user_id);

CREATE TABLE mentor_project_applications (
  id uuid PRIMARY KEY, project_id uuid NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
  mentor_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT, status varchar(16) NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','approved','rejected','cancelled')),
  reviewed_by uuid REFERENCES users(id) ON DELETE RESTRICT, review_comment varchar(1000), reviewed_at timestamptz,
  created_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX mentor_project_applications_pending ON mentor_project_applications(project_id,mentor_id) WHERE status='pending';
CREATE INDEX mentor_project_applications_status_created_at ON mentor_project_applications(status,created_at);
CREATE INDEX mentor_project_applications_mentor_id_created_at ON mentor_project_applications(mentor_id,created_at);

CREATE TABLE providers (
  id uuid PRIMARY KEY, name citext NOT NULL UNIQUE, base_url varchar(2048) NOT NULL,
  credential_ciphertext bytea NOT NULL, status varchar(16) NOT NULL DEFAULT 'active' CHECK(status IN ('active','inactive')),
  created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL
);
CREATE INDEX providers_status ON providers(status);

CREATE TABLE models (
  id uuid PRIMARY KEY, model_id citext NOT NULL UNIQUE, display_name varchar(256) NOT NULL, description varchar(2048),
  category varchar(32) NOT NULL DEFAULT 'text' CHECK(category IN ('text','image','audio','video','multimodal','embedding','rerank')),
  capabilities jsonb NOT NULL DEFAULT '[]', input_modalities jsonb NOT NULL DEFAULT '[]', output_modalities jsonb NOT NULL DEFAULT '[]',
  context_window integer CHECK(context_window>0), max_output_tokens integer CHECK(max_output_tokens>0), is_common boolean NOT NULL DEFAULT false,
  status varchar(32) NOT NULL DEFAULT 'pending_configuration' CHECK(status IN ('pending_configuration','active','inactive')),
  created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL
);
CREATE INDEX models_status_is_common ON models(status,is_common);
CREATE INDEX models_category_status ON models(category,status);

CREATE TABLE model_bindings (
  id uuid PRIMARY KEY, model_id uuid NOT NULL REFERENCES models(id) ON DELETE RESTRICT,
  provider_id uuid NOT NULL REFERENCES providers(id) ON DELETE RESTRICT, upstream_model_name varchar(256) NOT NULL,
  adapter varchar(64) NOT NULL CHECK(adapter IN ('openai_compatible','openai_responses','openai_embeddings','openai_images','openai_audio','openai_video','openai_realtime','openai_moderations','anthropic','cohere_rerank_v2','google_gemini_v1beta')),
  priority integer NOT NULL DEFAULT 100 CHECK(priority>=0), status varchar(16) NOT NULL DEFAULT 'active' CHECK(status IN ('active','inactive')),
  created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL, UNIQUE(model_id,provider_id,adapter)
);
CREATE INDEX model_bindings_model_id_status_priority ON model_bindings(model_id,status,priority);
CREATE INDEX model_bindings_provider_id_status ON model_bindings(provider_id,status);

CREATE TABLE api_keys (
  id uuid PRIMARY KEY, owner_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  project_id uuid NOT NULL REFERENCES projects(id) ON DELETE RESTRICT, name citext NOT NULL,
  status varchar(32) NOT NULL DEFAULT 'pending_mentor' CHECK(status IN ('pending_mentor','pending_teacher','approved','active','rejected','revoked')),
  key_hash bytea UNIQUE, key_prefix varchar(32), claimed_at timestamptz, revoked_at timestamptz,
  created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX api_keys_owner_name_live ON api_keys(owner_user_id,name) WHERE status IN ('pending_mentor','pending_teacher','approved','active');
CREATE INDEX api_keys_owner_created ON api_keys(owner_user_id,created_at);
CREATE INDEX api_keys_project_status_created ON api_keys(project_id,status,created_at);
CREATE INDEX api_keys_status_created ON api_keys(status,created_at);

CREATE TABLE api_key_models (
  id uuid PRIMARY KEY, api_key_id uuid NOT NULL REFERENCES api_keys(id) ON DELETE RESTRICT,
  model_id uuid NOT NULL REFERENCES models(id) ON DELETE RESTRICT, created_at timestamptz NOT NULL,
  UNIQUE(api_key_id,model_id)
);
CREATE INDEX api_key_models_model_id ON api_key_models(model_id);

CREATE TABLE api_key_audits (
  id uuid PRIMARY KEY, api_key_id uuid NOT NULL REFERENCES api_keys(id) ON DELETE RESTRICT,
  actor_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  action varchar(32) NOT NULL CHECK(action IN ('mentor_approved','mentor_rejected','teacher_approved','teacher_rejected','mentor_revoked')),
  comment varchar(1000), created_at timestamptz NOT NULL,
  CONSTRAINT api_key_audit_reason_required CHECK(action NOT IN ('mentor_rejected','teacher_rejected','mentor_revoked') OR (comment IS NOT NULL AND btrim(comment)<>''))
);
CREATE INDEX api_key_audits_key_created ON api_key_audits(api_key_id,created_at);
CREATE INDEX api_key_audits_actor_created ON api_key_audits(actor_user_id,created_at);
