-- name: InsertArea :exec
INSERT INTO knowledge_area (id, key, name, recorded_at)
VALUES ($1, $2, $3, $4);

-- name: GetArea :one
SELECT id, key, name, recorded_at FROM knowledge_area WHERE id = $1;

-- name: GetAreaByKey :one
SELECT id, key, name, recorded_at FROM knowledge_area WHERE key = $1;
