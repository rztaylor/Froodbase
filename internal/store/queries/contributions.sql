-- name: InsertContribution :exec
INSERT INTO contribution (id, area_id, actor_user_id, actor_agent_id, client, investigation_ref, recorded_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: InsertContributionClaim :exec
INSERT INTO contribution_claims (contribution_id, claim_id, claim_type, area_id)
VALUES ($1, $2, 'assertion', $3);

-- name: GetContribution :one
SELECT id, area_id, actor_user_id, actor_agent_id, client, investigation_ref, recorded_at
FROM contribution WHERE id = $1;

-- name: ListContributionClaims :many
SELECT claim_id FROM contribution_claims WHERE contribution_id = $1 ORDER BY claim_id;
