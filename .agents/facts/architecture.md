# Architecture Facts

Reference design: `docs/planning/03-architecture-design.md`. This file records what exists in code and the intended ownership boundaries.

## Current state

- One Go module, `github.com/rztaylor/froodbase`, hosted at `github.com/rztaylor/Froodbase` (see `.agents/facts/git.md`).
- Entrypoints: `cmd/froodbase-mcp`, `cmd/froodbase-api`, `cmd/froodbase-dreamer`. `froodbase-mcp` prints a version and exits; `froodbase-api` serves `GET /healthz`; `froodbase-dreamer` logs a startup line and waits for a signal.
- `internal/version`: build identity shared by all binaries.
- `internal/config`: `FROODBASE_` environment variables for `froodbase-api` and `froodbase-dreamer`; validation only, no connections, never logs values.
- `internal/httpserver`: the HTTP listener of `froodbase-api` (router, health endpoint, graceful shutdown). MCP handlers and auth are mounted here from their own packages when they exist.

## Intended boundaries (create packages as the plan reaches them)

- `internal/domain`: records, scope, temporal rules, lifecycle state machine, conflict classification. No I/O. Everything depends on it; it depends on nothing internal.
- `internal/authz`: the authorisation context (caller identity, acting agent, (area, role) grants) and the interface that builds it from a request. MVP implementation: static users with basic auth. Later: Entra token claims and group mapping. No credential type appears in the interface.
- `internal/store`: PostgreSQL access (pgx, sqlc), goose migrations, RLS session variable, River job insertion.
- `internal/retrieval`: candidate channels, permission and mode filters, fusion, explanation trace.
- `internal/contribute`: evidence validation, entity resolution, conflict detection, acceptance policy, change-set writes.
- `internal/ingest`: local report parsing and preview (used by `froodbase-mcp` only).
- `internal/dreamer`: job definitions for indexing, reconsideration, sweeps, gap scans, consolidation, reflection, knowledge views.
- `internal/mcpserver`: tool definitions and handlers on the MCP Go SDK, shared by the stdio and Streamable HTTP binaries.
- `internal/client`: the HTTPS client `froodbase-mcp` uses to call `froodbase-api`.

Dependency direction: `cmd/*` -> `mcpserver`, `dreamer`, `client` -> `retrieval`, `contribute`, `ingest`, `authz` -> `store` -> `domain`. Nothing imports `cmd/*`.

## Boundary documents

- Unit: every hand-written Go package. Document: `doc.go` with the package comment, per `document-code-boundaries` defaults (about 150 words, 2 KiB hard limit).
- Exemptions: none declared. Generated code (sqlc output) will live under `internal/store/gen` and is exempt.

## Generated assets and data

- sqlc output and goose migrations will live under `internal/store/`. Not yet created.
- Local runtime: Docker Compose (`compose.yaml` at the repository root) with PostgreSQL 16 (`pgvector/pgvector:pg16`, named volume), `froodbase-api` and `froodbase-dreamer` built from the root `Dockerfile`; `froodbase-mcp` runs on the host. Started with `make up`; variables in `.env.example`.
- Local build outputs in `bin/`, ignored by git.
