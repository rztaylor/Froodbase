# Testing Facts

- Full gate: `scripts/validate.sh` (go vet, golangci-lint v2, `go test -race -cover ./...`, `make build`).
- Quick: `go test ./...`.
- In sandboxes set `GOCACHE="$PWD/.cache/go-build"`; the script does this.
- Unit tests live beside the code as `_test.go`. Domain rules (temporal, scope overlap, conflict classification) must have table-driven tests before any store or API code depends on them. A rule implemented both in Go and in SQL keeps its cases in `internal/domain/domaintest`, and both the domain tests and the store's integration tests run every case.
- Integration tests needing PostgreSQL use `internal/store/storetest` (Testcontainers for Go, `pgvector/pgvector:pg16`, one container per test binary, a migrated database per test) and are tagged `integration`; run them with `make test-integration`, Docker is their only prerequisite, and CI runs them in the `integration` job. Slice exit scenarios live in `internal/apitest` (`TestSlice<N>Exit`), which starts the API in-process with a working dreamer and drives it through `internal/client`. The unit gate (`scripts/validate.sh`) stays Docker-free. `storetest.Open` gives an `Admin` pool (the owner, bypasses row-level security: set-up and inspection) and an `App` pool (the service role the policies apply to): anything that asserts what a caller can or cannot see runs on `App`.
- Generated sqlc code is checked, not tested: `make sqlc-check` (also in CI) fails when `internal/store/gen` is stale.
- The evaluation harness described in `docs/planning/02-technical-requirements.md` (TR-120 to TR-122) is a separate workstream and not part of the unit gate.
- No external service is required for the current gate.
