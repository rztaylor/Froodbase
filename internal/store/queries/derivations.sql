-- name: InsertDerivation :exec
INSERT INTO derivation (derived_id, source_id, area_id, rationale, recorded_at)
VALUES ($1, $2, $3, $4, $5);

-- name: ListDerivationSources :many
SELECT derived_id, source_id, area_id, rationale, recorded_at
FROM derivation WHERE derived_id = $1 ORDER BY recorded_at, source_id;

-- name: ListDerivedFrom :many
-- The links naming any of the given records as their source: one step of
-- the walk from a changed record to everything derived from it.
SELECT derived_id, source_id, area_id, rationale, recorded_at
FROM derivation WHERE source_id = ANY(sqlc.arg(source_ids)::uuid[]) ORDER BY derived_id, source_id;

-- name: HasDependants :one
SELECT EXISTS (SELECT 1 FROM derivation WHERE source_id = $1);
