# Architecture Facts

Reference design: `docs/planning/03-architecture-design.md`. This file records what exists in code and the intended ownership boundaries.

## Current state

- One Go module, `github.com/rztaylor/froodbase`. Decision needed: confirm the module path and GitHub owner before the first external import.
- Entrypoints: `cmd/froodbase-mcp`, `cmd/froodbase-api`, `cmd/froodbase-dreamer`. All print a version and exit; no behaviour yet.
- `internal/version`: build identity shared by all binaries.

## Intended boundaries (create packages as the plan reaches them)

- `internal/domain`: records, scope, temporal rules, lifecycle state machine, conflict classification. No I/O. Everything depends on it; it depends on nothing internal.
- `internal/authz`: authorisation context from token claims, area and role mapping.
- `internal/store`: PostgreSQL access (pgx, sqlc), migrations, RLS session variable, River job insertion.
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

- sqlc output and migrations will live under `internal/store/`. Not yet created.
- Local build outputs in `bin/`, ignored by git.
