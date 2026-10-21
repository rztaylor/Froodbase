-- +goose Up
-- The review sweep (slice 2, PR-33, TR-31, architecture design section 9):
-- the dreamer periodically marks records due for review and records stale
-- under their area's freshness policy. Age never deletes knowledge or
-- changes it; it makes it due for review. The marks are projections beside
-- the records, not revisions: the sweep writes only the two tables below,
-- never assertion, revision or support_event, so history stays the record
-- of what people and evidence said.

-- domain.ModeRecord.DueForReview: in force (active or disputed) and the
-- review date has come. A null date is never due; validity plays no part.
-- +goose StatementBegin
CREATE FUNCTION froodbase_due_for_review(state text, review_due_at timestamptz, now_at timestamptz) RETURNS boolean
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
  SELECT coalesce(state IN ('active', 'disputed') AND review_due_at <= now_at, false)
$$;
-- +goose StatementEnd

-- One row per assertion the sweep has ever marked: due_for_review as of the
-- area's last sweep, and stale_at, when a sweep first found the record
-- stale (null when it is not stale now). Rows are updated only when a flag
-- changes; a record never marked has no row. No delete grant: a cleared
-- flag is a row with nothing marked.
CREATE TABLE review_flag (
  record_id      uuid PRIMARY KEY REFERENCES assertion (id),
  area_id        uuid NOT NULL REFERENCES knowledge_area (id),
  due_for_review boolean NOT NULL DEFAULT false,
  stale_at       timestamptz,
  updated_at     timestamptz NOT NULL
);
CREATE INDEX review_flag_area ON review_flag (area_id);

-- The last sweep of each area: the reference time the flags were computed
-- at, and what it found.
CREATE TABLE review_sweep (
  area_id     uuid PRIMARY KEY REFERENCES knowledge_area (id),
  swept_at    timestamptz NOT NULL,
  records     integer NOT NULL CHECK (records >= 0),
  due_count   integer NOT NULL CHECK (due_count >= 0),
  stale_count integer NOT NULL CHECK (stale_count >= 0)
);

ALTER TABLE review_flag ENABLE ROW LEVEL SECURITY;
CREATE POLICY area_isolation ON review_flag FOR ALL
  USING (area_id = ANY (app_caller_areas())) WITH CHECK (area_id = ANY (app_caller_areas()));
ALTER TABLE review_sweep ENABLE ROW LEVEL SECURITY;
CREATE POLICY area_isolation ON review_sweep FOR ALL
  USING (area_id = ANY (app_caller_areas())) WITH CHECK (area_id = ANY (app_caller_areas()));
GRANT SELECT, INSERT, UPDATE ON review_flag, review_sweep TO froodbase_app;

-- Every area id, for the periodic sweep's fan-out: the dreamer then sweeps
-- each area with a caller holding that area alone. SECURITY DEFINER so it
-- reads past the policy on knowledge_area; it returns ids only.
-- +goose StatementBegin
CREATE FUNCTION app_all_area_ids() RETURNS SETOF uuid
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, public AS $$
  SELECT id FROM knowledge_area ORDER BY id
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION app_all_area_ids() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION app_all_area_ids() TO froodbase_app;
