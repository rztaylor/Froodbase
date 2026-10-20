-- name: InsertAssertion :exec
INSERT INTO assertion (
  id, area_id, subject_entity_id, epistemic_kind, body_markdown, scope,
  valid_from, valid_to, recorded_at, verified_at, review_due_at,
  lifecycle_state, support_score, support_config_version, revision, updated_at
) VALUES (
  $1, $2, $3, $4, $5, $6,
  $7, $8, $9, $10, $11,
  $12, $13, $14, 1, $15
);

-- name: UpdateAssertion :execrows
UPDATE assertion
SET body_markdown = $3, scope = $4, valid_from = $5, valid_to = $6,
    verified_at = $7, review_due_at = $8,
    revision = revision + 1, updated_at = $9
WHERE id = $1 AND revision = $2;

-- name: UpdateAssertionSupport :execrows
UPDATE assertion
SET support_score = $2, support_config_version = $3, needs_reconsideration = $4
WHERE id = $1;

-- name: GetAssertion :one
SELECT id, area_id, subject_entity_id, epistemic_kind, body_markdown, scope,
       valid_from, valid_to, recorded_at, verified_at, review_due_at,
       lifecycle_state, support_score, support_config_version, needs_reconsideration, revision, updated_at
FROM assertion WHERE id = $1;

-- name: ListAssertionsForEntity :many
SELECT id, area_id, subject_entity_id, epistemic_kind, body_markdown, scope,
       valid_from, valid_to, recorded_at, verified_at, review_due_at,
       lifecycle_state, support_score, support_config_version, needs_reconsideration, revision, updated_at
FROM assertion WHERE subject_entity_id = $1 ORDER BY recorded_at, id;
