-- name: InsertIdempotencyKey :exec
INSERT INTO idempotency_key (caller_user_id, key, tool, request_hash, response, created_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetIdempotencyKey :one
SELECT caller_user_id, key, tool, request_hash, response, created_at
FROM idempotency_key WHERE caller_user_id = $1 AND key = $2;
