-- name: InsertRevision :exec
INSERT INTO revision (record_id, record_type, revision, area_id, snapshot, actor_user_id, actor_agent_id, rationale, recorded_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: ListRevisions :many
SELECT record_id, record_type, revision, area_id, snapshot, actor_user_id, actor_agent_id, rationale, recorded_at
FROM revision WHERE record_id = $1 ORDER BY revision;

-- name: InsertAuditEvent :exec
INSERT INTO audit_event (area_id, event_type, actor_user_id, actor_agent_id, record_id, record_type, record_revision, detail, at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: ListAuditEventsForRecord :many
SELECT id, area_id, event_type, actor_user_id, actor_agent_id, record_id, record_type, record_revision, detail, at
FROM audit_event WHERE record_id = $1 ORDER BY at, id;

-- name: ListAuditEventsForArea :many
SELECT id, area_id, event_type, actor_user_id, actor_agent_id, record_id, record_type, record_revision, detail, at
FROM audit_event WHERE area_id = $1 ORDER BY at, id;
