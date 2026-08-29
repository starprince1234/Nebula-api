CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

ALTER TABLE projects ADD COLUMN monthly_credit_quota_milli bigint NOT NULL DEFAULT 20000000 CHECK(monthly_credit_quota_milli BETWEEN 0 AND 1000000000000);
ALTER TABLE models ADD COLUMN credit_multiplier_milli bigint CHECK(credit_multiplier_milli BETWEEN 0 AND 1000000000000);
UPDATE models SET credit_multiplier_milli=1000;
ALTER TABLE models ADD CONSTRAINT active_model_requires_credit_multiplier CHECK(status<>'active' OR credit_multiplier_milli IS NOT NULL);

ALTER TABLE api_keys ADD COLUMN requested_monthly_credit_quota_milli bigint NOT NULL DEFAULT 2000000 CHECK(requested_monthly_credit_quota_milli BETWEEN 0 AND 1000000000000);
ALTER TABLE api_keys ADD COLUMN mentor_monthly_credit_quota_milli bigint CHECK(mentor_monthly_credit_quota_milli BETWEEN 0 AND 1000000000000);
ALTER TABLE api_keys ADD COLUMN monthly_credit_quota_milli bigint CHECK(monthly_credit_quota_milli BETWEEN 0 AND 1000000000000);
UPDATE api_keys SET mentor_monthly_credit_quota_milli=2000000 WHERE status IN ('pending_teacher','approved','active','rejected','revoked');
UPDATE api_keys SET monthly_credit_quota_milli=2000000 WHERE status IN ('approved','active','revoked');

DO $$ DECLARE offenders text; BEGIN
 SELECT string_agg(project_id::text, ',') INTO offenders FROM (
   SELECT project_id FROM api_keys WHERE status IN ('approved','active') GROUP BY project_id HAVING count(*)*2000000>20000000
 ) x;
 IF offenders IS NOT NULL THEN RAISE EXCEPTION 'existing API key allocation exceeds project quota: %', offenders; END IF;
END $$;

CREATE TABLE project_month_credit_buckets(id uuid PRIMARY KEY,project_id uuid NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,month timestamptz NOT NULL,quota_milli bigint NOT NULL CHECK(quota_milli>=0),allocated_milli bigint NOT NULL DEFAULT 0 CHECK(allocated_milli>=0),charged_milli bigint NOT NULL DEFAULT 0 CHECK(charged_milli>=0),pending_milli bigint NOT NULL DEFAULT 0 CHECK(pending_milli>=0),created_at timestamptz NOT NULL,updated_at timestamptz NOT NULL,UNIQUE(project_id,month));
CREATE INDEX project_month_credit_buckets_month ON project_month_credit_buckets(month);
CREATE TABLE api_key_month_credit_buckets(id uuid PRIMARY KEY,api_key_id uuid NOT NULL REFERENCES api_keys(id) ON DELETE RESTRICT,project_id uuid NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,month timestamptz NOT NULL,quota_milli bigint NOT NULL CHECK(quota_milli>=0),allocation_active boolean NOT NULL DEFAULT true,charged_milli bigint NOT NULL DEFAULT 0 CHECK(charged_milli>=0),pending_milli bigint NOT NULL DEFAULT 0 CHECK(pending_milli>=0),created_at timestamptz NOT NULL,updated_at timestamptz NOT NULL,UNIQUE(api_key_id,month));
CREATE INDEX api_key_month_credit_buckets_project_month ON api_key_month_credit_buckets(project_id,month);
CREATE TABLE monthly_usage_cube(id uuid PRIMARY KEY,month timestamptz NOT NULL,organization_id uuid NOT NULL,project_id uuid NOT NULL,user_id uuid NOT NULL,api_key_id uuid NOT NULL,model_id uuid NOT NULL,organization_name varchar(128) NOT NULL,project_name varchar(128) NOT NULL,user_name varchar(64) NOT NULL,api_key_name varchar(128) NOT NULL,model_name varchar(256) NOT NULL,charged_milli bigint NOT NULL DEFAULT 0 CHECK(charged_milli>=0),charged_count bigint NOT NULL DEFAULT 0 CHECK(charged_count>=0),zero_cost_count bigint NOT NULL DEFAULT 0 CHECK(zero_cost_count>=0),created_at timestamptz NOT NULL,updated_at timestamptz NOT NULL,UNIQUE(month,project_id,user_id,api_key_id,model_id));
CREATE INDEX monthly_usage_cube_month_key ON monthly_usage_cube(month,api_key_id);
CREATE INDEX monthly_usage_cube_month_project ON monthly_usage_cube(month,project_id);

CREATE TABLE gateway_calls(id uuid PRIMARY KEY,request_id varchar(128) NOT NULL,month timestamptz NOT NULL,organization_id uuid NOT NULL,project_id uuid NOT NULL,user_id uuid NOT NULL,api_key_id uuid NOT NULL,model_id uuid NOT NULL,provider_id uuid,organization_name varchar(128) NOT NULL,project_name varchar(128) NOT NULL,user_name varchar(64) NOT NULL,api_key_name varchar(128) NOT NULL,model_name varchar(256) NOT NULL,provider_name varchar(128),protocol varchar(64) NOT NULL,request_path varchar(512) NOT NULL,multiplier_milli bigint NOT NULL CHECK(multiplier_milli>=0),credit_milli bigint NOT NULL CHECK(credit_milli>=0),billing_state varchar(16) NOT NULL DEFAULT 'pending' CHECK(billing_state IN ('pending','charged','failed')),outcome varchar(32) NOT NULL DEFAULT 'pending' CHECK(outcome IN ('pending','succeeded','upstream_failed','quota_rejected','outcome_unknown')),error_category varchar(64),error_message varchar(1000),lease_expires_at timestamptz,sent_at timestamptz,finalized_at timestamptz,created_at timestamptz NOT NULL);
CREATE INDEX gateway_calls_project_created ON gateway_calls(project_id,created_at DESC);
CREATE INDEX gateway_calls_key_created ON gateway_calls(api_key_id,created_at DESC);
CREATE INDEX gateway_calls_state_lease ON gateway_calls(billing_state,lease_expires_at);
CREATE INDEX gateway_calls_cursor ON gateway_calls(created_at DESC,id DESC);
CREATE TABLE gateway_call_attempts(id uuid PRIMARY KEY,call_id uuid NOT NULL REFERENCES gateway_calls(id) ON DELETE RESTRICT,provider_id uuid NOT NULL,binding_id uuid NOT NULL,provider_name varchar(128) NOT NULL,status varchar(16) NOT NULL CHECK(status IN ('connecting','succeeded','failed')),http_status integer,error_category varchar(64),error_message varchar(1000),latency_ms bigint NOT NULL DEFAULT 0 CHECK(latency_ms>=0),completed_at timestamptz,created_at timestamptz NOT NULL);
CREATE INDEX gateway_call_attempts_call_created ON gateway_call_attempts(call_id,created_at);
CREATE TABLE monitored_inputs(id uuid PRIMARY KEY,call_id uuid NOT NULL UNIQUE REFERENCES gateway_calls(id) ON DELETE RESTRICT,project_id uuid NOT NULL,user_id uuid NOT NULL,source varchar(64) NOT NULL,content text NOT NULL,content_bytes bigint NOT NULL CHECK(content_bytes>=0),visible boolean NOT NULL DEFAULT false,created_at timestamptz NOT NULL);
CREATE INDEX monitored_inputs_project_created ON monitored_inputs(project_id,created_at DESC);
CREATE INDEX monitored_inputs_visible_created ON monitored_inputs(visible,created_at DESC);
CREATE INDEX monitored_inputs_content_trgm ON monitored_inputs USING gin(content gin_trgm_ops) WHERE visible;

CREATE TABLE prompt_access_audits(id uuid PRIMARY KEY,actor_user_id uuid NOT NULL,project_id uuid,monitored_input_id uuid,access_type varchar(32) NOT NULL,query_scope jsonb,result_ids jsonb,result_count integer NOT NULL DEFAULT 0 CHECK(result_count>=0),created_at timestamptz NOT NULL);
CREATE INDEX prompt_access_audits_actor_created ON prompt_access_audits(actor_user_id,created_at);
CREATE INDEX prompt_access_audits_input_created ON prompt_access_audits(monitored_input_id,created_at);
CREATE TABLE quota_adjustment_audits(id uuid PRIMARY KEY,owner_type varchar(16) NOT NULL CHECK(owner_type IN ('project','api_key')),owner_id uuid NOT NULL,actor_user_id uuid NOT NULL,old_quota_milli bigint NOT NULL CHECK(old_quota_milli>=0),new_quota_milli bigint NOT NULL CHECK(new_quota_milli>=0),reason varchar(1000) NOT NULL CHECK(btrim(reason)<>''),created_at timestamptz NOT NULL);
CREATE INDEX quota_adjustment_audits_owner_created ON quota_adjustment_audits(owner_type,owner_id,created_at);
CREATE TABLE model_multiplier_audits(id uuid PRIMARY KEY,model_id uuid NOT NULL,actor_user_id uuid NOT NULL,old_multiplier_milli bigint,new_multiplier_milli bigint NOT NULL CHECK(new_multiplier_milli>=0),reason varchar(1000) NOT NULL CHECK(btrim(reason)<>''),created_at timestamptz NOT NULL);
CREATE INDEX model_multiplier_audits_model_created ON model_multiplier_audits(model_id,created_at);
CREATE TABLE maintenance_state(id uuid PRIMARY KEY,name varchar(64) NOT NULL UNIQUE,last_success_at timestamptz,last_error varchar(1000),created_at timestamptz NOT NULL,updated_at timestamptz NOT NULL);

INSERT INTO project_month_credit_buckets(id,project_id,month,quota_milli,allocated_milli,created_at,updated_at)
SELECT gen_random_uuid(),p.id,date_trunc('month',now() AT TIME ZONE 'Asia/Shanghai') AT TIME ZONE 'Asia/Shanghai',p.monthly_credit_quota_milli,COALESCE(sum(k.monthly_credit_quota_milli) FILTER(WHERE k.status IN ('approved','active')),0),now(),now() FROM projects p LEFT JOIN api_keys k ON k.project_id=p.id GROUP BY p.id;
INSERT INTO api_key_month_credit_buckets(id,api_key_id,project_id,month,quota_milli,allocation_active,created_at,updated_at)
SELECT gen_random_uuid(),id,project_id,date_trunc('month',now() AT TIME ZONE 'Asia/Shanghai') AT TIME ZONE 'Asia/Shanghai',monthly_credit_quota_milli,true,now(),now() FROM api_keys WHERE status IN ('approved','active');
