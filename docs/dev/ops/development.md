# Development

## Prerequisites

- Go matching the `go` directive in `go.mod`.
- golangci-lint v2 (`brew install golangci-lint`).
- Later phases: PostgreSQL 16 with pgvector for integration tests, Azure CLI for Entra app registration.

## Build and validate

```bash
scripts/validate.sh
```

Runs vet, lint, race tests and builds all three binaries into `bin/`. Individual targets: `make vet`, `make lint`, `make test`, `make build`.

## Running

The binaries currently accept `--version` only. Behaviour arrives with implementation plan phase 1 (`docs/planning/04-implementation-plan.md`).

## Configuration

Local configuration will use environment variables prefixed `FROODBASE_`; document each here when it is introduced. Secrets never go in files under version control.
