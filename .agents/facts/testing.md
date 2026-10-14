# Testing Facts

- Full gate: `scripts/validate.sh` (go vet, golangci-lint v2, `go test -race -cover ./...`, `make build`).
- Quick: `go test ./...`.
- In sandboxes set `GOCACHE="$PWD/.cache/go-build"`; the script does this.
- Unit tests live beside the code as `_test.go`. Domain rules (temporal, scope overlap, conflict classification) must have table-driven tests before any store or API code depends on them.
- Integration tests needing PostgreSQL will use a disposable database and be tagged `integration`; they are not yet present. Decision needed: Testcontainers versus a CI service container.
- The evaluation harness described in `docs/planning/02-technical-requirements.md` (TR-120 to TR-122) is a separate workstream and not part of the unit gate.
- No external service is required for the current gate.
