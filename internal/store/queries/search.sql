-- name: SearchEntityNames :many
-- Identifier channel (TR-51): exact (case-insensitive) and near-miss names
-- by trigram similarity, best first. The caller's areas filter through RLS.
SELECT n.entity_id, n.area_id, n.name,
       similarity(n.name, sqlc.arg(query)::text)::float8 AS sim,
       (lower(n.name) = lower(sqlc.arg(query)::text)) AS exact
FROM entity_name n
WHERE lower(n.name) = lower(sqlc.arg(query)::text)
   OR similarity(n.name, sqlc.arg(query)::text) >= sqlc.arg(threshold)::float8
ORDER BY exact DESC, sim DESC, n.entity_id
LIMIT sqlc.arg(lim)::int;

-- name: SearchAssertionBodies :many
-- Full-text channel (TR-51): websearch syntax over the generated tsvector,
-- ranked by cover density.
SELECT a.id, a.area_id, ts_rank_cd(a.body_fts, q)::float8 AS rank
FROM assertion a, websearch_to_tsquery('english', sqlc.arg(query)::text) q
WHERE a.body_fts @@ q
ORDER BY rank DESC, a.id
LIMIT sqlc.arg(lim)::int;
