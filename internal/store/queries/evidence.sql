-- name: InsertEvidence :exec
INSERT INTO evidence (
  id, area_id, source_kind, source_uri, source_revision, source_locator,
  excerpt, access_class, root_evidence_id, observed_at, recorded_at
) VALUES (
  $1, $2, $3, $4, $5, $6,
  $7, $8, $9, $10, $11
);

-- name: GetEvidence :one
SELECT id, area_id, source_kind, source_uri, source_revision, source_locator,
       excerpt, access_class, root_evidence_id, observed_at, recorded_at
FROM evidence WHERE id = $1;

-- name: InsertClaimEvidence :exec
INSERT INTO claim_evidence (claim_id, claim_type, evidence_id, area_id)
VALUES ($1, 'assertion', $2, $3)
ON CONFLICT (claim_id, evidence_id) DO NOTHING;

-- name: ListEvidenceForClaim :many
SELECT e.id, e.area_id, e.source_kind, e.source_uri, e.source_revision, e.source_locator,
       e.excerpt, e.access_class, e.root_evidence_id, e.observed_at, e.recorded_at
FROM claim_evidence ce JOIN evidence e ON e.id = ce.evidence_id
WHERE ce.claim_id = $1
ORDER BY e.recorded_at, e.id;

-- name: CountClaimEvidenceInArea :one
SELECT count(*)
FROM claim_evidence ce
JOIN evidence e ON e.id = ce.evidence_id
JOIN assertion a ON a.id = ce.claim_id
WHERE ce.area_id = $1;
