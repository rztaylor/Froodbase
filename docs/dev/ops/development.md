# Development

## Prerequisites

- Go matching the `go` directive in `go.mod`.
- golangci-lint v2 (`brew install golangci-lint`).
- From MVP slice 0: Docker (Compose for the local stack, Testcontainers for the integration suite).
- Cloud move: Azure CLI for Entra app registration.

## Build and validate

```bash
scripts/validate.sh
```

Runs vet, lint, race tests and builds all three binaries into `bin/`. Individual targets: `make vet`, `make lint`, `make test`, `make build`.

## Running

The binaries currently accept `--version` only. Behaviour arrives slice by slice per `docs/planning/07-local-mvp.md`; slice 0 adds the Compose stack and the Claude Code registration, documented here when it lands.

## Configuration

Local configuration will use environment variables prefixed `FROODBASE_`; document each here when it is introduced. Secrets never go in files under version control.
