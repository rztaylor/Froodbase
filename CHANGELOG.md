# Changelog

All notable changes to Froodbase are recorded here. The format follows Keep a Changelog; versions follow SemVer once releases begin.

## Unreleased

### Added

- `froodbase-mcp` is a stdio MCP server for Claude Code: it forwards tools to `froodbase-api` with the person's credentials from `FROODBASE_API_URL`, `FROODBASE_USERNAME` and `FROODBASE_PASSWORD`, returns a structured `froodbase_unavailable` result instead of an error when the API cannot be reached, and sends its version on every call; `froodbase-api` publishes `min_client_version` on `/healthz` and warns about older clients.
- Basic auth for the local API: `froodbase-api` loads a static users file (`FROODBASE_USERS_FILE`, bcrypt hashes only) and serves MCP over Streamable HTTP at `/mcp` behind HTTP basic auth, with a `whoami` tool reporting the caller's identity and (area, role) grants. `froodbase-api hash-password` creates a hash from stdin; `make up` creates `local/users.json` with three sample users.
- Versioned schema migrations with goose, embedded in `froodbase-api`: `froodbase-api migrate` applies them and `froodbase-api migrate --status` lists them; `make migrate` and `make migrate-status` run them against the Compose database. The first migration enables pgvector, pg_trgm and btree_gist. `internal/store` owns the PostgreSQL pool, which both server binaries open at startup.
- Local stack in Docker Compose (`make up`, `make down`, `make logs`): PostgreSQL 16 with pgvector, `froodbase-api` and `froodbase-dreamer` built from the root `Dockerfile`; `.env.example` documents every `FROODBASE_` variable and `make up` generates the local database password into the ignored `.env`. `froodbase-api` answers `GET /healthz`; `froodbase-dreamer` logs a startup line. `internal/config` loads the `FROODBASE_` variables; a CI job builds the images, starts the stack and checks health.
- Repository foundation: Go module with the `froodbase-mcp`, `froodbase-api` and `froodbase-dreamer` entrypoints, shared `internal/version`, lint and validation gate, CI workflow, Dependabot, repo-local facts and developer docs.
- Planning documents moved from the AI.CloudToolkit reports folder and renamed to Froodbase.
- Local MVP scope (`docs/planning/07-local-mvp.md`): the brain built first in Docker Compose with basic auth, as six slices; decisions on Compose, basic auth behind the authorisation-context interface, goose and Testcontainers recorded.
