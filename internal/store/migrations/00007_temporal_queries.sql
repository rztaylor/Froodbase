-- +goose Up
-- Two clocks (slice 2, TR-26, TR-30 to TR-34, TR-53): the scope overlap,
-- validity, status, mode and stale rules of internal/domain as SQL
-- functions, so every retrieval channel filters with one definition, and the
-- per-area freshness policy the stale rule reads. The temporal columns and
-- scope of assertion and entity_name, and the GiST validity index on
-- assertion with lifecycle_state, exist since migration 2.
--
-- The functions answer to the same case tables as the Go functions
-- (internal/domain/domaintest), run by the store's integration tests. They
-- take the reference time ("now") as an argument so the tables run
-- deterministically; queries pass the service's clock. Validity is the
-- half-open range [valid_from, valid_to) with null bounds open; recorded_at
-- never stands in for it (TR-32, TR-34). Name keys compare without case and
-- surrounding ASCII whitespace; lower() and Go's EqualFold can differ for
-- some non-ASCII letters, a known gap while scopes are ASCII identifiers.

-- A version bound as comparable segments: an optional leading v or V, then
-- dot-separated whole numbers, trailing zero segments dropped so 2.4 equals
-- 2.4.0. Null when blank or not a version: such a bound is open.
-- +goose StatementBegin
CREATE FUNCTION froodbase_version(v text) RETURNS numeric[]
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
  SELECT CASE WHEN s ~ '^[0-9]+(\.[0-9]+)*$'
              THEN string_to_array(regexp_replace(s, '(\.0+)+$', ''), '.')::numeric[]
         END
  FROM (SELECT regexp_replace(btrim(coalesce(v, ''), E' \t\n\r\f\v'), '^[vV]', '') AS s) t
$$;
-- +goose StatementEnd

-- Whether two values of a name key can describe the same place: either is
-- unspecified (missing or blank), or they are equal ignoring case and
-- surrounding whitespace.
-- +goose StatementBegin
CREATE FUNCTION froodbase_scope_name_matches(a text, b text) RETURNS boolean
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
  SELECT x = '' OR y = '' OR lower(x) = lower(y)
  FROM (SELECT btrim(coalesce(a, ''), E' \t\n\r\f\v') AS x, btrim(coalesce(b, ''), E' \t\n\r\f\v') AS y) t
$$;
-- +goose StatementEnd

-- domain.Scope.Overlaps: every name key matches and neither version range
-- lies wholly above the other. A null or empty scope is unspecified.
-- +goose StatementBegin
CREATE FUNCTION froodbase_scope_overlaps(a jsonb, b jsonb) RETURNS boolean
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
  SELECT froodbase_scope_name_matches(a ->> 'environment', b ->> 'environment')
     AND froodbase_scope_name_matches(a ->> 'region', b ->> 'region')
     AND froodbase_scope_name_matches(a ->> 'cluster', b ->> 'cluster')
     AND froodbase_scope_name_matches(a ->> 'tenant', b ->> 'tenant')
     AND NOT coalesce(froodbase_version(a ->> 'version_min') > froodbase_version(b ->> 'version_max'), false)
     AND NOT coalesce(froodbase_version(b ->> 'version_min') > froodbase_version(a ->> 'version_max'), false)
$$;
-- +goose StatementEnd

-- domain.Temporal.ValidAt: the half-open window [valid_from, valid_to)
-- includes the time; a null bound is open, so unknown validity is not ruled
-- out. Null at is not a time and includes nothing.
-- +goose StatementBegin
CREATE FUNCTION froodbase_valid_at(valid_from timestamptz, valid_to timestamptz, at_time timestamptz) RETURNS boolean
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
  SELECT CASE WHEN at_time IS NULL OR valid_from >= valid_to THEN false
              ELSE tstzrange(valid_from, valid_to, '[)') @> at_time
         END
$$;
-- +goose StatementEnd

-- domain.ModeRecord.StatusAt: current when in force (active or disputed)
-- and valid at now_at; upcoming when in force and valid only from later;
-- historical otherwise.
-- +goose StatementBegin
CREATE FUNCTION froodbase_temporal_status(state text, valid_from timestamptz, valid_to timestamptz, now_at timestamptz) RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
  SELECT CASE
    WHEN NOT coalesce(state IN ('active', 'disputed'), false) THEN 'historical'
    WHEN froodbase_valid_at(valid_from, valid_to, now_at) THEN 'current'
    WHEN valid_from IS NOT NULL AND now_at < valid_from THEN 'upcoming'
    ELSE 'historical'
  END
$$;
-- +goose StatementEnd

-- domain.ModeRecord.InMode: current keeps records current at now_at; as_of
-- keeps records valid at as_of by validity time, whenever recorded, unless
-- retracted (a superseded record keeps its validity, TR-33); historical
-- keeps everything. An unknown mode, or as_of without a time, keeps nothing.
-- +goose StatementBegin
CREATE FUNCTION froodbase_in_mode(state text, valid_from timestamptz, valid_to timestamptz, mode text, as_of timestamptz, now_at timestamptz) RETURNS boolean
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
  SELECT coalesce(CASE mode
    WHEN 'current' THEN froodbase_temporal_status(state, valid_from, valid_to, now_at) = 'current'
    WHEN 'as_of' THEN state IN ('active', 'disputed', 'superseded') AND froodbase_valid_at(valid_from, valid_to, as_of)
    WHEN 'historical' THEN true
    ELSE false
  END, false)
$$;
-- +goose StatementEnd

-- domain.Classify's stale flag: a record current at now_at whose last check
-- (verified_at, or recorded_at if never verified) is strictly older than its
-- kind's limit in the policy ({"observation": 90, ...}, whole days of 24
-- hours whatever the session's time zone). A kind absent or zero has no
-- limit. Stale flags a record; it never filters one out.
-- +goose StatementBegin
CREATE FUNCTION froodbase_stale(kind text, state text, valid_from timestamptz, valid_to timestamptz, recorded_at timestamptz, verified_at timestamptz, limits jsonb, now_at timestamptz) RETURNS boolean
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
  SELECT coalesce(
    froodbase_temporal_status(state, valid_from, valid_to, now_at) = 'current'
    AND days > 0
    AND now_at - coalesce(verified_at, recorded_at) > make_interval(hours => 24 * days),
    false)
  FROM (SELECT (limits ->> kind)::integer AS days) p
$$;
-- +goose StatementEnd

-- Freshness policy per area (TR-31, architecture design section 14): the
-- maximum age in days of a record's last check, by epistemic kind. Append-
-- only like support_config: a change is a new row and the newest row of the
-- area applies; an area without one uses the service default (the row with
-- a null area_id), which matches domain.DefaultFreshnessPolicy.
CREATE TABLE freshness_policy (
  id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  area_id       uuid REFERENCES knowledge_area (id),
  max_age_days  jsonb NOT NULL CHECK (jsonb_typeof(max_age_days) = 'object'),
  actor_user_id text,
  recorded_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX freshness_policy_area ON freshness_policy (area_id, id DESC);
INSERT INTO freshness_policy (area_id, max_age_days) VALUES (NULL, '{"observation": 90, "inference": 90}');

ALTER TABLE freshness_policy ENABLE ROW LEVEL SECURITY;
CREATE POLICY area_or_default_read ON freshness_policy FOR SELECT USING (area_id IS NULL OR area_id = ANY (app_caller_areas()));
CREATE POLICY area_append ON freshness_policy FOR INSERT WITH CHECK (area_id = ANY (app_caller_areas()));
GRANT SELECT, INSERT ON freshness_policy TO froodbase_app;

-- The policy that applies in an area, read under the caller's policies.
-- +goose StatementBegin
CREATE FUNCTION froodbase_freshness_policy(area uuid) RETURNS jsonb
LANGUAGE sql STABLE PARALLEL SAFE AS $$
  SELECT max_age_days FROM freshness_policy
  WHERE area_id = area OR area_id IS NULL
  ORDER BY (area_id IS NULL), id DESC
  LIMIT 1
$$;
-- +goose StatementEnd

-- Validity-time queries over entity names (TR-34); the assertion index is
-- assertion_validity from migration 2.
CREATE INDEX entity_name_validity ON entity_name USING gist (tstzrange(valid_from, valid_to, '[)'));
