-- name: SweepReviewFlags :one
-- The review sweep of one area (migration 8): each assertion's due and
-- stale flags at now_at, through the same SQL functions as the live labels
-- (froodbase_stale under froodbase_freshness_policy, as search and
-- ClassifyAssertion use). Writes only review_flag, and only rows whose
-- flags changed; a record stale since an earlier sweep keeps its stale_at.
WITH flags AS (
  SELECT a.id, a.area_id,
         froodbase_due_for_review(a.lifecycle_state, a.review_due_at, sqlc.arg(now_at)::timestamptz) AS due,
         froodbase_stale(a.epistemic_kind, a.lifecycle_state, a.valid_from, a.valid_to, a.recorded_at, a.verified_at,
                         froodbase_freshness_policy(a.area_id), sqlc.arg(now_at)::timestamptz) AS stale
  FROM assertion a
  WHERE a.area_id = sqlc.arg(area_id)::uuid
), written AS (
  INSERT INTO review_flag AS r (record_id, area_id, due_for_review, stale_at, updated_at)
  SELECT f.id, f.area_id, f.due, CASE WHEN f.stale THEN sqlc.arg(now_at)::timestamptz END, sqlc.arg(now_at)::timestamptz
  FROM flags f
  WHERE f.due OR f.stale OR EXISTS (SELECT 1 FROM review_flag x WHERE x.record_id = f.id)
  ON CONFLICT (record_id) DO UPDATE
    SET due_for_review = EXCLUDED.due_for_review,
        stale_at = CASE WHEN EXCLUDED.stale_at IS NULL THEN NULL ELSE coalesce(r.stale_at, EXCLUDED.stale_at) END,
        updated_at = EXCLUDED.updated_at
    WHERE r.due_for_review <> EXCLUDED.due_for_review OR (r.stale_at IS NULL) <> (EXCLUDED.stale_at IS NULL)
  RETURNING r.record_id
)
SELECT count(*)::int AS records,
       (count(*) FILTER (WHERE f.due))::int AS due,
       (count(*) FILTER (WHERE f.stale))::int AS stale,
       (SELECT count(*) FROM written)::int AS changed
FROM flags f;

-- name: UpsertReviewSweep :exec
INSERT INTO review_sweep (area_id, swept_at, records, due_count, stale_count)
VALUES (sqlc.arg(area_id)::uuid, sqlc.arg(swept_at)::timestamptz, sqlc.arg(records)::int, sqlc.arg(due_count)::int, sqlc.arg(stale_count)::int)
ON CONFLICT (area_id) DO UPDATE
  SET swept_at = EXCLUDED.swept_at, records = EXCLUDED.records, due_count = EXCLUDED.due_count, stale_count = EXCLUDED.stale_count;

-- name: GetReviewFlags :one
-- The sweep's marks on one assertion and when its area was last swept;
-- nothing marked when the sweep has never flagged it.
SELECT coalesce(f.due_for_review, false)::boolean AS due_for_review, f.stale_at, s.swept_at
FROM assertion a
LEFT JOIN review_flag f ON f.record_id = a.id
LEFT JOIN review_sweep s ON s.area_id = a.area_id
WHERE a.id = sqlc.arg(id)::uuid;
