-- +goose Up
-- Projection bookkeeping (TR-57): when each record was last indexed and at
-- which revision. It holds identifiers and revisions only, never content,
-- so it is readable without area scoping; the dreamer reports staleness
-- across every area from it. A record is stale when its revision is newer
-- than the indexed one or it has no row.
CREATE TABLE index_state (
  record_id   uuid PRIMARY KEY,
  record_type text NOT NULL CHECK (record_type IN ('entity', 'assertion')),
  area_id     uuid NOT NULL,
  revision    integer NOT NULL CHECK (revision >= 1),
  indexed_at  timestamptz NOT NULL
);
CREATE INDEX index_state_area ON index_state (area_id);
GRANT SELECT, INSERT, UPDATE ON index_state TO froodbase_app;

-- Staleness across all areas, for the dreamer's health endpoint. SECURITY
-- DEFINER so it may count records the caller's areas do not cover; it
-- returns counts only.
-- +goose StatementBegin
CREATE FUNCTION app_index_staleness() RETURNS TABLE (total bigint, stale bigint)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, public AS $$
  WITH records AS (
    SELECT id, revision FROM entity
    UNION ALL
    SELECT id, revision FROM assertion
  )
  SELECT count(*)::bigint AS total,
         count(*) FILTER (WHERE s.record_id IS NULL OR s.revision < r.revision)::bigint AS stale
  FROM records r LEFT JOIN index_state s ON s.record_id = r.id
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION app_index_staleness() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION app_index_staleness() TO froodbase_app;
