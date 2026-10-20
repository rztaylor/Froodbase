-- name: InsertDerivation :exec
INSERT INTO derivation (derived_id, source_id, area_id, rationale, recorded_at)
VALUES ($1, $2, $3, $4, $5);

-- name: ListDerivationSources :many
SELECT derived_id, source_id, area_id, rationale, recorded_at
FROM derivation WHERE derived_id = $1 ORDER BY recorded_at, source_id;
