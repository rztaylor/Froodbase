# Testing Facts

- Full gate: `scripts/validate.sh` (go vet, golangci-lint v2, `go test -race -cover ./...`, `make build`).
- Quick: `go test ./...`.
- In sandboxes set `GOCACHE="$PWD/.cache/go-build"`; the script does this.
- Unit tests live beside the code as `_test.go`. Domain rules (temporal, scope overlap, conflict classification) must have table-driven tests before any store or API code depends on them.
- Integration tests needing PostgreSQL use Testcontainers for Go with a disposable PostgreSQL 16 container (pgvector, pg_trgm, btree_gist) and are tagged `integration`; Docker is their only prerequisite. Not yet present; they arrive with MVP slice 0. The unit gate stays Docker-free.
- The evaluation harness described in `docs/planning/02-technical-requirements.md` (TR-120 to TR-122) is a separate workstream and not part of the unit gate.
- No external service is required for the current gate.
