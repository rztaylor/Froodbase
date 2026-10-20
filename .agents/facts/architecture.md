# Architecture Facts

Reference design: `docs/planning/03-architecture-design.md`. This file records what exists in code and the intended ownership boundaries.

## Current state

- One Go module, `github.com/rztaylor/froodbase`, hosted at `github.com/rztaylor/Froodbase` (see `.agents/facts/git.md`).
- Entrypoints: `cmd/froodbase-mcp`, `cmd/froodbase-api`, `cmd/froodbase-dreamer`. `froodbase-mcp` serves MCP over stdio and forwards to the API through `internal/client`; `froodbase-api` serves `GET /healthz` and MCP at `/mcp` behind basic auth; `froodbase-dreamer` runs River workers (heartbeat every minute) and a health endpoint reporting queue depth.
- `internal/version`: build identity shared by all binaries.
- `internal/domain`: the knowledge model with no I/O, importing only the standard library: knowledge areas, entities (immutable id, type, names with validity windows), assertions (epistemic kind enum, subject, Markdown body, typed scope, the TR-30 temporal fields), evidence (area, source kind, URI, revision, locator, excerpt, access class, root), contributions, and the support model: the nine support event types of TR-29, `SupportConfig` with the service default and its bounds, and `SupportScore`, the deterministic projection of events to a score (TR-43). Owns evidence shareability (TR-61), access-class ordering, corroboration and distinct roots (TR-25). `NewObservation` sets `recorded_at` only. `internal/store` depends on it.
- `internal/config`: `FROODBASE_` environment variables for `froodbase-api` and `froodbase-dreamer`; validation only, no connections, never logs values.
- `internal/httpserver`: the HTTP listener of `froodbase-api` (router, health endpoint, graceful shutdown). MCP handlers and auth are mounted here from their own packages when they exist.
- `internal/authz`: `Context`, `Principal`, `Builder` interface and role implication; `internal/authz/staticusers`: JSON users file with bcrypt hashes, `Authenticate` and `Build`.
- `internal/wire`: the tool contract, input and output types of every tool, shared by `mcpserver`, the engines and `client`.
- `internal/contribute`: contribute calls into store records: ref resolution, role check with audited refusal, idempotency (per caller, request hash, result replayed), result shaping; `CallerFor` builds the store caller from the authz context. `internal/retrieval`: `get_record`, `get_history` and `search` (identifier channel: exact and trigram near-miss names as candidates; full-text channel over bodies; identifier hits first, no fusion yet); restricted evidence reads are audited.
- `internal/apihandlers`: the handlers `froodbase-api` serves, and the knowledge area directory (keys to ids, cached; areas created at start-up for the users file's grant keys). `froodbase-mcp` does not import it.
- `internal/client`: the SDK client `froodbase-mcp` uses against the API: basic auth, `Froodbase-Client-Version` header, connection and 5xx failures mapped to `mcpserver.UnavailableError`, 401/403 to `AuthError`.
- `internal/mcpserver`: tool definitions on the MCP Go SDK with caller-supplied handlers: `whoami`, `contribute`, `get_record`, `get_history`, `search`. An `UnavailableError` from a handler becomes an `Unavailable` result, not a tool error. `froodbase-api` mounts the Streamable HTTP handler at `/mcp` behind the basic-auth middleware in `internal/httpserver`.
- `internal/store`: pgx pool, embedded goose migrations (`internal/store/migrations`) plus River's schema, applied by `froodbase-api migrate` (the Compose `migrate` service runs it before the api and dreamer start); transactional job insertion (`Pool.Jobs().InsertTx`) and `QueueDepth`. Slice 1 records: the tables `knowledge_area`, `entity`, `entity_name`, `assertion`, `evidence`, `claim_evidence`, `contribution`, `contribution_claims`, `revision`, `audit_event`, `support_event`, `support_config`, every one with row-level security keyed on the `app.caller_areas` session variable (`Caller.AreaIDs`, set with `set_config(..., true)` per transaction) and a non-owning service role `froodbase_app` the policies apply to. The writer (`CreateArea`, `Contribute`, `UpdateEntity`, `UpdateAssertion`, `LinkEvidence`, `PutSupportConfig`) writes records, full-snapshot revisions, audit events, one `root_added` support event per distinct root with the score projected by `domain.SupportScore` under the area's configuration version, and the `index_record` job in one transaction; the reader (`GetEntity`, `GetAssertion`, `GetEvidence`, `ClaimEvidence`, `History`, `AuditEvents`, `SupportEvents`, `SupportConfig`, `RecomputeSupportScore`) runs under the same policies. Queries are sqlc-generated into `internal/store/gen` from `internal/store/queries`. `internal/store/jobs`: job argument types, one per kind (heartbeat, index_record). Both server binaries open the pool at startup as `froodbase_app`.
- `internal/dreamer`: River client with workers and periodic schedules (heartbeat, leader-only, unique per minute) and the health endpoint.

## Intended boundaries (create packages as the plan reaches them)

- `internal/domain`: records, scope, temporal rules, support projection, lifecycle state machine, conflict classification. No I/O. Exists with the slice 1 records and the support model; scope overlap, lifecycle and conflict classification arrive with later slices. Everything depends on it; it depends on nothing internal.
- `internal/authz`: the authorisation context (caller identity, acting agent, (area, role) grants) and the `Builder` interface from an authenticated `Principal`. Implementations: `authz/staticusers` (MVP, exists); Entra token claims and group mapping (cloud move). No credential type appears in the interface.
- `internal/store`: PostgreSQL access (pgx, sqlc), goose migrations, RLS session variable, the transactional writer, River job insertion. Exists.
- `internal/retrieval`: candidate channels, permission and mode filters, fusion, explanation trace.
- `internal/contribute`: evidence validation, entity resolution, conflict detection, support events and scoring against the area configuration, outcome recording, change-set writes. Exists with change-set translation, idempotency and the role check; resolution and conflicts follow.
- `internal/dreamer`: workers and schedules for indexing, support recomputation, quiet-use credits, sweeps, gap scans, consolidation, reflection, knowledge views. Exists with the heartbeat; job argument types live in `internal/store/jobs` so the API can enqueue them.
- `internal/mcpserver`: tool definitions on the MCP Go SDK with handlers supplied by the binary (API engines in `froodbase-api`, forwarding client in `froodbase-mcp`). Exists; grows a tool set per slice.
- `internal/client`: the HTTPS client `froodbase-mcp` uses to call `froodbase-api`. Exists; gains Entra sign-in with the cloud move.

Dependency direction: `cmd/*` -> `apihandlers`, `mcpserver`, `dreamer`, `client` -> `retrieval`, `contribute`, `authz` -> `store` (and `store/jobs`) -> `domain`; `wire` is a leaf everyone may import. Nothing imports `cmd/*`.

## Boundary documents

- Unit: every hand-written Go package. Document: `doc.go` with the package comment, per `document-code-boundaries` defaults (about 150 words, 2 KiB hard limit).
- Exemptions: generated code only. The sqlc output under `internal/store/gen` is exempt (`// Code generated ... DO NOT EDIT.`).

## Generated assets and data

- goose migrations live in `internal/store/migrations` (forward-only, `NNNNN_description.sql`, embedded). sqlc: config `internal/store/sqlc.yaml`, hand-written queries in `internal/store/queries/*.sql`, output committed under `internal/store/gen`, regenerated with `make sqlc` (sqlc v1.30.0 through `go run`, pure Go, nothing to install) and checked by `make sqlc-check` in CI. A new table or column is a migration plus, when queried, a query file change and a regeneration.
- Local runtime: Docker Compose (`compose.yaml` at the repository root) with PostgreSQL 16 (`pgvector/pgvector:pg16`, named volume), `froodbase-api` and `froodbase-dreamer` built from the root `Dockerfile`; `froodbase-mcp` runs on the host. Started with `make up`; variables in `.env.example`. The database has two roles: the owner `froodbase` runs migrations; the service role `froodbase_app` (created by `local/postgres-init` on first start, granted by the migrations, subject to row-level security) is what the api and dreamer connect as.
- Local build outputs in `bin/`, ignored by git.
