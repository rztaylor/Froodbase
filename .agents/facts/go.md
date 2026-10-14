# Go Facts

- Toolchain: the `go` directive in `go.mod` (currently 1.26). CI uses `go-version-file`.
- Layout: `cmd/<binary>/` entrypoints, `internal/` packages, no `pkg/`.
- Every hand-written package has `doc.go` (see `.agents/facts/architecture.md`).
- Formatting: `gofmt` and `goimports` via golangci-lint formatters.
- Lint: golangci-lint v2, config in `.golangci.yml` (standard set plus `misspell` with UK locale and `revive`).
- Build: `make build` injects version, commit and date into `internal/version` with `-ldflags`.
- Planned dependencies (add when the plan reaches them, record licences in `docs/dev/ops/third-party.md` at that point): MCP Go SDK, pgx v5, sqlc, pgvector-go, River, azidentity, go-oidc v3, OpenTelemetry.
