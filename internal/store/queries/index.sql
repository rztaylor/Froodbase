-- name: UpsertIndexState :exec
INSERT INTO index_state (record_id, record_type, area_id, revision, indexed_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (record_id) DO UPDATE
  SET revision = GREATEST(index_state.revision, EXCLUDED.revision), indexed_at = EXCLUDED.indexed_at;

-- name: GetIndexState :one
SELECT record_id, record_type, area_id, revision, indexed_at FROM index_state WHERE record_id = $1;

-- name: ListAreaRecordRevisions :many
SELECT r.id, r.record_type, r.revision FROM (
  SELECT e.id, 'entity'::text AS record_type, e.revision FROM entity e WHERE e.area_id = sqlc.arg(area_id)::uuid
  UNION ALL
  SELECT a.id, 'assertion'::text AS record_type, a.revision FROM assertion a WHERE a.area_id = sqlc.arg(area_id)::uuid
) r ORDER BY r.id;

-- name: GetRecordRevision :one
SELECT r.revision FROM (
  SELECT e.id, e.revision FROM entity e WHERE e.id = sqlc.arg(record_id)::uuid
  UNION ALL
  SELECT a.id, a.revision FROM assertion a WHERE a.id = sqlc.arg(record_id)::uuid
) r;
