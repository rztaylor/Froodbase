# Froodbase

A shared, remote knowledge service that helps people and agents understand our systems, learn from investigations, and distinguish what is true now from what was true before. Written in Go on Azure Database for PostgreSQL, accessed through MCP.

Froodbase was scoped under the working name "The Brain"; the documents have been updated to the product name.

## Layout

| Path | Contents |
|---|---|
| `cmd/froodbase-mcp` | Local stdio MCP server for each engineer's computer |
| `cmd/froodbase-api` | Remote service: authorisation, retrieval, contributions, audit |
| `cmd/froodbase-dreamer` | Background module: indexing, review sweeps, consolidation, reflection, knowledge views |
| `internal/` | Shared packages; each has a `doc.go` stating what it owns |
| `docs/pitch/` | The original product pitch and the assessments that informed it |
| `docs/planning/` | Requirements, architecture, implementation plan, explainer, reuse research, and the local MVP scope being built now |
| `docs/dev/` | Decisions, development and release operations |
| `.agents/facts/` | Repo-local policy for agents and contributors |

## Building

```bash
scripts/validate.sh
```

See `docs/dev/ops/development.md` for prerequisites.

## Running

```bash
make up
```

Starts PostgreSQL, `froodbase-api` and `froodbase-dreamer` in Docker Compose; `make down` stops them. Details and configuration in `docs/dev/ops/development.md`.

## Reading order for a newcomer

The pitch, then `docs/planning/05-how-froodbase-works.md`, then the product requirements and architecture design. To contribute, read `docs/planning/07-local-mvp.md` for what is being built first and where it runs.
