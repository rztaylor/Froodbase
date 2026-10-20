# FRB-9 Records are stored with full history, audit and per-area access control

Short-lived implementation plan; deleted when the ticket closes. Scope and acceptance criteria are on the ticket.

## Design

- Migration `00002_records.sql`: `knowledge_area`, `entity`, `entity_name`, `assertion`, `evidence`, `claim_evidence`, `contribution`, `contribution_claims`, `revision`, `audit_event`, `support_event`, `support_config`. Every table carries `area_id` (denormalised on the join tables) so a policy never needs a join.
- Application role `froodbase_app` (created `NOLOGIN` by the migration if absent; Compose creates it with `LOGIN` and `FROODBASE_APP_DB_PASSWORD` in an init script). It owns nothing; it gets `SELECT`/`INSERT` everywhere, `UPDATE` on `entity` and `assertion`, `DELETE` on `entity_name` only. `revision`, `audit_event`, `support_event` are append-only at the grant level and by policy.
- RLS: `app_caller_areas()` parses `app.caller_areas` (comma-separated uuids set with `set_config(..., true)` per transaction); every policy is `area_id = ANY (app_caller_areas())`. Missing setting means no rows. `audit_event` accepts inserts for any area (denied reads are audited) and `support_config` exposes the service default (`area_id IS NULL`, version `service-1`) to everyone. `app_area_ids(keys text[])` is `SECURITY DEFINER` so the API can turn grant keys into ids before setting the variable.
- Support: `internal/domain` gains `SupportEventType`, `SupportEvent`, `SupportConfig` (defaults seeded by the migration), `SupportScore(events, cfg)` and `DistinctRoots`. The writer appends one `root_added` per new distinct root and stores the projected score with the config version.
- Writer: `store.Writer` with `Caller{UserID, AgentID, AreaIDs}`; `CreateArea`, `Contribute` (entities, assertions, evidence, claim links, support events, revision rows, audit events, `index_record` job) and `UpdateAssertion`/`UpdateEntity` (revision n+1 with optimistic concurrency). Snapshots are explicit JSON DTOs in `internal/store/snapshot.go`.
- sqlc: `internal/store/sqlc.yaml`, queries in `internal/store/queries/*.sql`, output in `internal/store/gen` (exempt from doc.go), `make sqlc`, CI runs `sqlc diff`.

## Validation

- Unit: snapshot round trips, support score projection (table-driven), config validation, job args.
- Integration (`make test-integration`): RLS isolation across two areas including joins; bypass attempts fail as the app role; atomicity by injecting a failure mid-transaction; revision numbering; audit per write; one `root_added` per distinct root; recomputed score equals the stored score; service default config seeded.
- `scripts/validate.sh`.
