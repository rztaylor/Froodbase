-- name: InsertEntity :exec
INSERT INTO entity (id, area_id, entity_type, lifecycle_state, revision, recorded_at, updated_at)
VALUES ($1, $2, $3, $4, 1, $5, $5);

-- name: UpdateEntity :execrows
UPDATE entity
SET revision = revision + 1, updated_at = $3
WHERE id = $1 AND revision = $2;

-- name: GetEntity :one
SELECT id, area_id, entity_type, lifecycle_state, revision, recorded_at, updated_at
FROM entity WHERE id = $1;

-- name: DeleteEntityNames :exec
DELETE FROM entity_name WHERE entity_id = $1;

-- name: InsertEntityName :exec
INSERT INTO entity_name (entity_id, area_id, position, name, scope, valid_from, valid_to)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListEntityNames :many
SELECT id, entity_id, area_id, position, name, scope, valid_from, valid_to
FROM entity_name WHERE entity_id = $1 ORDER BY position;

-- name: CountEntityNamesInArea :one
SELECT count(*) FROM entity_name n JOIN entity e ON e.id = n.entity_id WHERE n.area_id = $1;
