-- name: InsertSupportEvent :exec
INSERT INTO support_event (record_id, record_type, area_id, event_type, evidence_root_ids, scope, actor_user_id, actor_agent_id, job_name, config_version, rationale, at, source_kind)
VALUES ($1, 'assertion', $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);

-- name: ListSupportEvents :many
SELECT id, record_id, record_type, area_id, event_type, evidence_root_ids, scope, actor_user_id, actor_agent_id, job_name, config_version, rationale, at, source_kind
FROM support_event WHERE record_id = $1 ORDER BY at, id;

-- name: ListSupportRoots :many
SELECT evidence_root_ids[1]::uuid AS root_id
FROM support_event WHERE record_id = $1 AND event_type = 'root_added' ORDER BY 1;

-- name: ListSupportRootSources :many
-- Each root a claim has, with its evidence's kind and source URI (empty if
-- the evidence cannot be read).
SELECT se.evidence_root_ids[1]::uuid AS root_id, coalesce(e.source_kind, '')::text AS source_kind, coalesce(e.source_uri, '')::text AS source_uri
FROM support_event se LEFT JOIN evidence e ON e.id = se.evidence_root_ids[1]
WHERE se.record_id = $1 AND se.event_type = 'root_added'
ORDER BY 1;

-- name: CurrentSupportConfig :one
-- The area's newest configuration, or the service default when the area has none.
SELECT id, area_id, version, event_weights, use_without_challenge_cap, threshold_by_kind, allowed_contributors, recorded_at, root_weight_by_source_kind
FROM support_config
WHERE area_id = sqlc.arg(area_id)::uuid OR area_id IS NULL
ORDER BY (area_id IS NULL), recorded_at DESC
LIMIT 1;

-- name: SupportConfigVersion :one
-- A named configuration version as the area saw it: the area's own row if it
-- has one of that version, else the service default of that version.
SELECT id, area_id, version, event_weights, use_without_challenge_cap, threshold_by_kind, allowed_contributors, recorded_at, root_weight_by_source_kind
FROM support_config
WHERE version = sqlc.arg(version) AND (area_id = sqlc.arg(area_id)::uuid OR area_id IS NULL)
ORDER BY (area_id IS NULL)
LIMIT 1;

-- name: InsertSupportConfig :exec
INSERT INTO support_config (area_id, version, event_weights, use_without_challenge_cap, threshold_by_kind, allowed_contributors, recorded_at, root_weight_by_source_kind)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);
