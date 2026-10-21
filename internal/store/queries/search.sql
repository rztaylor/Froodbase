-- name: SearchEntityNames :many
-- Identifier channel (TR-51): exact (case-insensitive) and near-miss names
-- by trigram similarity, best first. The caller's areas filter through RLS;
-- the mode filter (the entity's lifecycle state with the name's validity)
-- and the scope filter run before the limit (TR-53).
SELECT n.entity_id, n.area_id, n.name,
       similarity(n.name, sqlc.arg(query)::text)::float8 AS sim,
       (lower(n.name) = lower(sqlc.arg(query)::text)) AS exact,
       froodbase_temporal_status(e.lifecycle_state, n.valid_from, n.valid_to, sqlc.arg(now_at)::timestamptz)::text AS status
FROM entity_name n JOIN entity e ON e.id = n.entity_id
WHERE (lower(n.name) = lower(sqlc.arg(query)::text)
       OR similarity(n.name, sqlc.arg(query)::text) >= sqlc.arg(threshold)::float8)
  AND froodbase_in_mode(e.lifecycle_state, n.valid_from, n.valid_to, sqlc.arg(mode)::text, sqlc.narg(as_of)::timestamptz, sqlc.arg(now_at)::timestamptz)
  AND froodbase_scope_overlaps(n.scope, sqlc.arg(scope)::jsonb)
ORDER BY exact DESC, sim DESC, n.entity_id
LIMIT sqlc.arg(lim)::int;

-- name: SearchAssertionBodies :many
-- Full-text channel (TR-51): websearch syntax over the generated tsvector,
-- ranked by cover density, filtered by mode and scope before the limit
-- (TR-53), with each record's status relative to now and its stale flag
-- under the area's freshness policy (TR-31).
SELECT a.id, a.area_id, ts_rank_cd(a.body_fts, q)::float8 AS rank,
       froodbase_temporal_status(a.lifecycle_state, a.valid_from, a.valid_to, sqlc.arg(now_at)::timestamptz)::text AS status,
       froodbase_stale(a.epistemic_kind, a.lifecycle_state, a.valid_from, a.valid_to, a.recorded_at, a.verified_at,
                       froodbase_freshness_policy(a.area_id), sqlc.arg(now_at)::timestamptz)::boolean AS stale
FROM assertion a, websearch_to_tsquery('english', sqlc.arg(query)::text) q
WHERE a.body_fts @@ q
  AND froodbase_in_mode(a.lifecycle_state, a.valid_from, a.valid_to, sqlc.arg(mode)::text, sqlc.narg(as_of)::timestamptz, sqlc.arg(now_at)::timestamptz)
  AND froodbase_scope_overlaps(a.scope, sqlc.arg(scope)::jsonb)
ORDER BY rank DESC, a.id
LIMIT sqlc.arg(lim)::int;

-- name: ClassifyAssertion :one
-- One assertion through the mode predicate, status and stale rule: what
-- get_record labels and get_as_of asks.
SELECT froodbase_in_mode(a.lifecycle_state, a.valid_from, a.valid_to, sqlc.arg(mode)::text, sqlc.narg(as_of)::timestamptz, sqlc.arg(now_at)::timestamptz)::boolean AS included,
       froodbase_temporal_status(a.lifecycle_state, a.valid_from, a.valid_to, sqlc.arg(now_at)::timestamptz)::text AS status,
       froodbase_stale(a.epistemic_kind, a.lifecycle_state, a.valid_from, a.valid_to, a.recorded_at, a.verified_at,
                       froodbase_freshness_policy(a.area_id), sqlc.arg(now_at)::timestamptz)::boolean AS stale
FROM assertion a WHERE a.id = sqlc.arg(id)::uuid;

-- name: ClassifyEntityNames :many
-- Each name of an entity through the mode predicate with the entity's
-- lifecycle state, in name order.
SELECT n.position,
       froodbase_in_mode(e.lifecycle_state, n.valid_from, n.valid_to, sqlc.arg(mode)::text, sqlc.narg(as_of)::timestamptz, sqlc.arg(now_at)::timestamptz)::boolean AS included,
       froodbase_temporal_status(e.lifecycle_state, n.valid_from, n.valid_to, sqlc.arg(now_at)::timestamptz)::text AS status
FROM entity_name n JOIN entity e ON e.id = n.entity_id
WHERE n.entity_id = sqlc.arg(entity_id)::uuid
ORDER BY n.position;

-- name: CurrentFreshnessPolicy :one
-- The freshness policy that applies in the area: its newest, or the
-- service default.
SELECT froodbase_freshness_policy(sqlc.arg(area_id)::uuid)::jsonb AS max_age_days;

-- name: InsertFreshnessPolicy :exec
INSERT INTO freshness_policy (area_id, max_age_days, actor_user_id, recorded_at)
VALUES (sqlc.arg(area_id)::uuid, sqlc.arg(max_age_days)::jsonb, sqlc.narg(actor_user_id)::text, sqlc.arg(recorded_at)::timestamptz);
