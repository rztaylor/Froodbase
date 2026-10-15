# Third-party dependencies

Direct Go dependencies with their licences, recorded when they are added (`.agents/facts/go.md`). Transitive dependencies are listed by `go mod graph`; none introduces a copyleft licence beyond those noted here.

| Module | Version | Licence | Used for |
|---|---|---|---|
| `github.com/jackc/pgx/v5` | v5.11.0 | MIT | PostgreSQL driver and connection pool (`internal/store`) |
| `github.com/pressly/goose/v3` | v3.29.0 | MIT | Forward-only schema migrations embedded in `froodbase-api` (`internal/store`) |
| `github.com/modelcontextprotocol/go-sdk` | v1.8.0 | MIT (with Apache-2.0 for contributions after the licence change; see its LICENSE) | MCP server and client: tool definitions (`internal/mcpserver`), Streamable HTTP transport in `froodbase-api`, stdio in `froodbase-mcp` |
| `github.com/riverqueue/river` (with `riverdriver/riverpgxv5`) | v0.49.0 | MPL-2.0, used unmodified | Background job queue in PostgreSQL: transactional insertion from `internal/store`, workers and periodic jobs in `internal/dreamer`, schema via `rivermigrate` |
| `golang.org/x/crypto` | v0.58.0 | BSD-3-Clause | bcrypt password hashes for the static users file (`internal/authz/staticusers`) |
