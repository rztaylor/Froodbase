-- +goose Up
-- Slice 1 record tables (docs/planning/03-architecture-design.md section 5):
-- knowledge areas, entities and their names, assertions, evidence, claim
-- evidence, contributions and their claims, revisions, audit events, support
-- events and support configuration. Every table carries area_id so a
-- row-level-security policy never needs a join (TR-27, TR-71).
--
-- Roles. The migration runs as the owning role (a superuser locally). The
-- service connects as froodbase_app, which owns nothing, so it is subject to
-- every policy and cannot disable them. Compose creates the role with LOGIN
-- and a password (local/postgres-init); elsewhere it is created here without
-- LOGIN and an operator adds the credential. Grants are the minimum: insert
-- and select everywhere, update on the two mutable record tables, delete on
-- the name projection only; revision, audit_event and support_event are
-- append-only at the grant level and by policy. River's queue tables, which
-- store.Migrate applies before the goose migrations, get full DML. Nothing
-- is granted by default: a later migration that adds a table grants what
-- the role needs, or the service fails closed with a permission error.

-- +goose StatementBegin
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'froodbase_app') THEN
    CREATE ROLE froodbase_app NOLOGIN;
  END IF;
END
$$;
-- +goose StatementEnd

-- The caller's areas, set per transaction by the service with
-- set_config('app.caller_areas', '<uuid>,<uuid>', true). Unset or empty
-- means no areas: every policy then matches nothing (fail closed).
-- +goose StatementBegin
CREATE FUNCTION app_caller_areas() RETURNS uuid[]
LANGUAGE sql STABLE PARALLEL SAFE AS $$
  SELECT COALESCE(string_to_array(NULLIF(current_setting('app.caller_areas', true), ''), ',')::uuid[], '{}'::uuid[])
$$;
-- +goose StatementEnd

CREATE TABLE knowledge_area (
  id          uuid PRIMARY KEY,
  key         text NOT NULL UNIQUE CHECK (key ~ '^[a-z0-9][a-z0-9-]{0,62}$'),
  name        text NOT NULL CHECK (btrim(name) <> ''),
  recorded_at timestamptz NOT NULL
);

-- Maps grant keys (as the users file and Entra groups name areas) to ids
-- before the caller's areas are set. SECURITY DEFINER so it reads past the
-- policy on knowledge_area; it reveals only the ids of keys the caller
-- already holds.
-- +goose StatementBegin
CREATE FUNCTION app_area_ids(keys text[]) RETURNS TABLE (key text, id uuid)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, public AS $$
  SELECT key, id FROM knowledge_area WHERE key = ANY (keys)
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION app_area_ids(text[]) FROM PUBLIC;

CREATE TABLE entity (
  id              uuid PRIMARY KEY,
  area_id         uuid NOT NULL REFERENCES knowledge_area (id),
  entity_type     text NOT NULL CHECK (entity_type IN ('system', 'service', 'component', 'team', 'environment', 'incident')),
  lifecycle_state text NOT NULL DEFAULT 'active' CHECK (lifecycle_state IN ('active', 'disputed', 'superseded', 'retracted')),
  revision        integer NOT NULL DEFAULT 1 CHECK (revision >= 1),
  recorded_at     timestamptz NOT NULL,
  updated_at      timestamptz NOT NULL
);
CREATE INDEX entity_area ON entity (area_id);

-- The current names of an entity (TR-20) in the order they were added;
-- history is in the entity's revisions. Rewritten whole when the entity
-- changes.
CREATE TABLE entity_name (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  entity_id  uuid NOT NULL REFERENCES entity (id),
  area_id    uuid NOT NULL REFERENCES knowledge_area (id),
  position   integer NOT NULL CHECK (position >= 0),
  name       text NOT NULL CHECK (btrim(name) <> ''),
  scope      jsonb NOT NULL DEFAULT '{}'::jsonb,
  valid_from timestamptz,
  valid_to   timestamptz,
  CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_from < valid_to)
);
CREATE UNIQUE INDEX entity_name_order ON entity_name (entity_id, position);
CREATE INDEX entity_name_trgm ON entity_name USING gin (name gin_trgm_ops);

CREATE TABLE assertion (
  id                     uuid PRIMARY KEY,
  area_id                uuid NOT NULL REFERENCES knowledge_area (id),
  subject_entity_id      uuid NOT NULL REFERENCES entity (id),
  epistemic_kind         text NOT NULL CHECK (epistemic_kind IN ('observation', 'decision', 'inference')),
  body_markdown          text NOT NULL CHECK (btrim(body_markdown) <> ''),
  body_fts               tsvector GENERATED ALWAYS AS (to_tsvector('english', body_markdown)) STORED,
  scope                  jsonb NOT NULL DEFAULT '{}'::jsonb,
  valid_from             timestamptz,
  valid_to               timestamptz,
  recorded_at            timestamptz NOT NULL,
  verified_at            timestamptz,
  review_due_at          timestamptz,
  lifecycle_state        text NOT NULL DEFAULT 'active' CHECK (lifecycle_state IN ('active', 'disputed', 'superseded', 'retracted')),
  support_score          double precision NOT NULL DEFAULT 0 CHECK (support_score >= 0),
  support_config_version text NOT NULL,
  needs_reconsideration  boolean NOT NULL DEFAULT false,
  revision               integer NOT NULL DEFAULT 1 CHECK (revision >= 1),
  updated_at             timestamptz NOT NULL,
  CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_from < valid_to)
);
CREATE INDEX assertion_area_subject ON assertion (area_id, subject_entity_id);
CREATE INDEX assertion_fts ON assertion USING gin (body_fts);
CREATE INDEX assertion_validity ON assertion USING gist (tstzrange(valid_from, valid_to, '[)'), lifecycle_state);

-- A citation of a living source: where, which revision, where inside it,
-- at most a short excerpt. Never a copy of the source (PR-50, TR-24).
-- Immutable: no update or delete grant. One row per (uri, revision,
-- locator) in an area, so a source cited twice is one root, not two.
CREATE TABLE evidence (
  id               uuid PRIMARY KEY,
  area_id          uuid NOT NULL REFERENCES knowledge_area (id),
  source_kind      text NOT NULL CHECK (source_kind IN ('repository', 'ticket', 'telemetry_query', 'document')),
  source_uri       text NOT NULL CHECK (btrim(source_uri) <> ''),
  source_revision  text NOT NULL CHECK (btrim(source_revision) <> ''),
  source_locator   text NOT NULL DEFAULT '',
  excerpt          text,
  access_class     text NOT NULL CHECK (access_class IN ('restricted', 'area', 'organisation')),
  root_evidence_id uuid REFERENCES evidence (id),
  observed_at      timestamptz,
  recorded_at      timestamptz NOT NULL
);
CREATE UNIQUE INDEX evidence_source ON evidence (area_id, source_uri, source_revision, source_locator);

CREATE TABLE claim_evidence (
  claim_id    uuid NOT NULL REFERENCES assertion (id),
  claim_type  text NOT NULL DEFAULT 'assertion' CHECK (claim_type = 'assertion'),
  evidence_id uuid NOT NULL REFERENCES evidence (id),
  area_id     uuid NOT NULL REFERENCES knowledge_area (id),
  PRIMARY KEY (claim_id, evidence_id)
);
CREATE INDEX claim_evidence_evidence ON claim_evidence (evidence_id);

-- One contribute call: who, through which agent and client, from which
-- investigation, when Froodbase was told (PR-10).
CREATE TABLE contribution (
  id                uuid PRIMARY KEY,
  area_id           uuid NOT NULL REFERENCES knowledge_area (id),
  actor_user_id     text NOT NULL CHECK (btrim(actor_user_id) <> ''),
  actor_agent_id    text,
  client            text NOT NULL DEFAULT '',
  investigation_ref text NOT NULL DEFAULT '',
  recorded_at       timestamptz NOT NULL
);

CREATE TABLE contribution_claims (
  contribution_id uuid NOT NULL REFERENCES contribution (id),
  claim_id        uuid NOT NULL REFERENCES assertion (id),
  claim_type      text NOT NULL DEFAULT 'assertion' CHECK (claim_type = 'assertion'),
  area_id         uuid NOT NULL REFERENCES knowledge_area (id),
  PRIMARY KEY (contribution_id, claim_id)
);
CREATE INDEX contribution_claims_claim ON contribution_claims (claim_id);

-- Full snapshot of a record at each change (TR-35). Append-only.
CREATE TABLE revision (
  record_id      uuid NOT NULL,
  record_type    text NOT NULL CHECK (record_type IN ('entity', 'assertion', 'evidence')),
  revision       integer NOT NULL CHECK (revision >= 1),
  area_id        uuid NOT NULL REFERENCES knowledge_area (id),
  snapshot       jsonb NOT NULL,
  actor_user_id  text NOT NULL,
  actor_agent_id text,
  rationale      text NOT NULL DEFAULT '',
  recorded_at    timestamptz NOT NULL,
  PRIMARY KEY (record_id, revision)
);

-- Operational audit log (TR-74). Append-only; any session may append (a
-- denied read is audited under the area it was denied in) and an area's
-- events are readable by that area's callers. area_id is null for events
-- outside any area, which only operators read.
CREATE TABLE audit_event (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  area_id         uuid REFERENCES knowledge_area (id),
  event_type      text NOT NULL CHECK (btrim(event_type) <> ''),
  actor_user_id   text NOT NULL,
  actor_agent_id  text,
  record_id       uuid,
  record_type     text,
  record_revision integer,
  detail          jsonb NOT NULL DEFAULT '{}'::jsonb,
  at              timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_event_area_at ON audit_event (area_id, at);
CREATE INDEX audit_event_record ON audit_event (record_id, at);

-- A claim's support history (TR-29). Append-only; the claim's support_score
-- is the projection of these rows under the named configuration version.
-- A root_added event names exactly one root and exists at most once per
-- (claim, root): repeated descendants of one root produce no event.
CREATE TABLE support_event (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  record_id         uuid NOT NULL,
  record_type       text NOT NULL DEFAULT 'assertion' CHECK (record_type = 'assertion'),
  area_id           uuid NOT NULL REFERENCES knowledge_area (id),
  event_type        text NOT NULL CHECK (event_type IN ('root_added', 'verified', 'outcome_succeeded', 'outcome_failed', 'use_without_challenge', 'source_changed', 'contradicted', 'flagged', 'support_retracted')),
  evidence_root_ids uuid[] NOT NULL DEFAULT '{}'::uuid[],
  scope             jsonb NOT NULL DEFAULT '{}'::jsonb,
  actor_user_id     text,
  actor_agent_id    text,
  job_name          text,
  config_version    text NOT NULL CHECK (btrim(config_version) <> ''),
  rationale         text NOT NULL DEFAULT '',
  at                timestamptz NOT NULL,
  CHECK (actor_user_id IS NOT NULL OR job_name IS NOT NULL),
  CHECK (event_type <> 'root_added' OR cardinality(evidence_root_ids) = 1)
);
CREATE INDEX support_event_record_at ON support_event (record_id, at);
CREATE UNIQUE INDEX support_event_one_root_added ON support_event (record_id, (evidence_root_ids[1])) WHERE event_type = 'root_added';

-- Versioned support configuration per area (TR-43). The row with a null
-- area_id is the service default, used by every area without its own and
-- readable by everyone; its values match domain.DefaultSupportConfig.
CREATE TABLE support_config (
  id                        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  area_id                   uuid REFERENCES knowledge_area (id),
  version                   text NOT NULL CHECK (btrim(version) <> ''),
  event_weights             jsonb NOT NULL,
  use_without_challenge_cap double precision NOT NULL CHECK (use_without_challenge_cap >= 0),
  threshold_by_kind         jsonb NOT NULL,
  allowed_contributors      jsonb NOT NULL,
  recorded_at               timestamptz NOT NULL DEFAULT now(),
  UNIQUE NULLS NOT DISTINCT (area_id, version)
);
INSERT INTO support_config (area_id, version, event_weights, use_without_challenge_cap, threshold_by_kind, allowed_contributors)
VALUES (
  NULL,
  'service-1',
  '{"root_added": 1, "verified": 1, "outcome_succeeded": 1.5, "outcome_failed": -1.5, "use_without_challenge": 0.1, "source_changed": -1, "contradicted": -1, "flagged": -1, "support_retracted": -1}'::jsonb,
  0.5,
  '{"observation": 1, "decision": 1, "inference": 2}'::jsonb,
  '{"classes": ["person", "agent"], "source_kinds": ["repository", "ticket", "telemetry_query", "document"]}'::jsonb
);

-- Row-level security on every record table, keyed on the caller's areas.
-- The owning role is not subject to the policies (it runs migrations only);
-- froodbase_app is, for every statement kind.
ALTER TABLE knowledge_area ENABLE ROW LEVEL SECURITY;
CREATE POLICY area_isolation ON knowledge_area FOR ALL
  USING (id = ANY (app_caller_areas())) WITH CHECK (id = ANY (app_caller_areas()));

ALTER TABLE entity ENABLE ROW LEVEL SECURITY;
CREATE POLICY area_isolation ON entity FOR ALL
  USING (area_id = ANY (app_caller_areas())) WITH CHECK (area_id = ANY (app_caller_areas()));

ALTER TABLE entity_name ENABLE ROW LEVEL SECURITY;
CREATE POLICY area_isolation ON entity_name FOR ALL
  USING (area_id = ANY (app_caller_areas())) WITH CHECK (area_id = ANY (app_caller_areas()));

ALTER TABLE assertion ENABLE ROW LEVEL SECURITY;
CREATE POLICY area_isolation ON assertion FOR ALL
  USING (area_id = ANY (app_caller_areas())) WITH CHECK (area_id = ANY (app_caller_areas()));

ALTER TABLE evidence ENABLE ROW LEVEL SECURITY;
CREATE POLICY area_isolation ON evidence FOR ALL
  USING (area_id = ANY (app_caller_areas())) WITH CHECK (area_id = ANY (app_caller_areas()));

ALTER TABLE claim_evidence ENABLE ROW LEVEL SECURITY;
CREATE POLICY area_isolation ON claim_evidence FOR ALL
  USING (area_id = ANY (app_caller_areas())) WITH CHECK (area_id = ANY (app_caller_areas()));

ALTER TABLE contribution ENABLE ROW LEVEL SECURITY;
CREATE POLICY area_isolation ON contribution FOR ALL
  USING (area_id = ANY (app_caller_areas())) WITH CHECK (area_id = ANY (app_caller_areas()));

ALTER TABLE contribution_claims ENABLE ROW LEVEL SECURITY;
CREATE POLICY area_isolation ON contribution_claims FOR ALL
  USING (area_id = ANY (app_caller_areas())) WITH CHECK (area_id = ANY (app_caller_areas()));

-- Append-only tables: a select and an insert policy, nothing for update or
-- delete, so those are denied even if a grant were ever added.
ALTER TABLE revision ENABLE ROW LEVEL SECURITY;
CREATE POLICY area_read ON revision FOR SELECT USING (area_id = ANY (app_caller_areas()));
CREATE POLICY area_append ON revision FOR INSERT WITH CHECK (area_id = ANY (app_caller_areas()));

ALTER TABLE audit_event ENABLE ROW LEVEL SECURITY;
CREATE POLICY area_read ON audit_event FOR SELECT USING (area_id = ANY (app_caller_areas()));
CREATE POLICY any_append ON audit_event FOR INSERT WITH CHECK (true);

ALTER TABLE support_event ENABLE ROW LEVEL SECURITY;
CREATE POLICY area_read ON support_event FOR SELECT USING (area_id = ANY (app_caller_areas()));
CREATE POLICY area_append ON support_event FOR INSERT WITH CHECK (area_id = ANY (app_caller_areas()));

ALTER TABLE support_config ENABLE ROW LEVEL SECURITY;
CREATE POLICY area_or_default_read ON support_config FOR SELECT USING (area_id IS NULL OR area_id = ANY (app_caller_areas()));
CREATE POLICY area_append ON support_config FOR INSERT WITH CHECK (area_id = ANY (app_caller_areas()));

-- Grants for the service role.
GRANT USAGE ON SCHEMA public TO froodbase_app;
GRANT SELECT, INSERT ON knowledge_area, entity, entity_name, assertion, evidence, claim_evidence, contribution, contribution_claims, revision, audit_event, support_event, support_config TO froodbase_app;
GRANT UPDATE ON entity, assertion TO froodbase_app;
GRANT DELETE ON entity_name TO froodbase_app;
GRANT EXECUTE ON FUNCTION app_caller_areas(), app_area_ids(text[]) TO froodbase_app;

-- River's queue (river_job, river_leader, river_queue, ...): the api enqueues
-- and the dreamer works jobs as the service role.
-- +goose StatementBegin
DO $$
DECLARE
  name text;
BEGIN
  FOR name IN SELECT tablename FROM pg_tables WHERE schemaname = 'public' AND tablename LIKE 'river\_%' LOOP
    EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON %I TO froodbase_app', name);
  END LOOP;
  FOR name IN SELECT sequencename FROM pg_sequences WHERE schemaname = 'public' AND sequencename LIKE 'river\_%' LOOP
    EXECUTE format('GRANT USAGE, SELECT ON SEQUENCE %I TO froodbase_app', name);
  END LOOP;
END
$$;
-- +goose StatementEnd
