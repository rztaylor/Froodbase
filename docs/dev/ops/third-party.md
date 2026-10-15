# Third-party dependencies

Direct Go dependencies with their licences, recorded when they are added (`.agents/facts/go.md`). Transitive dependencies are listed by `go mod graph`; none introduces a copyleft licence beyond those noted here.

| Module | Version | Licence | Used for |
|---|---|---|---|
| `github.com/jackc/pgx/v5` | v5.11.0 | MIT | PostgreSQL driver and connection pool (`internal/store`) |
| `github.com/pressly/goose/v3` | v3.29.0 | MIT | Forward-only schema migrations embedded in `froodbase-api` (`internal/store`) |
