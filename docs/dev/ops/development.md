# Development

## Prerequisites

- Go matching the `go` directive in `go.mod`.
- golangci-lint v2 (`brew install golangci-lint`).
- Docker with Compose v2 (`docker compose`) for the local stack and, later, the Testcontainers integration suite. Not needed for `scripts/validate.sh`.
- Cloud move: Azure CLI for Entra app registration.

## Build and validate

```bash
scripts/validate.sh
```

Runs vet, lint, race tests and builds all three binaries into `bin/`. Individual targets: `make vet`, `make lint`, `make test`, `make build`.

## Running the local stack with Compose

`compose.yaml` at the repository root runs PostgreSQL 16 (image `pgvector/pgvector:pg16`, which ships pgvector; `pg_trgm` and `btree_gist` are contrib extensions in the same image), `froodbase-api` and `froodbase-dreamer`. The two server images are built from the root `Dockerfile` with the same version ldflags as `make build`. `froodbase-mcp` is not part of the stack; it runs on the host once slice 0 adds it.

| Target | What it does |
|---|---|
| `make up` | Creates `.env` from `.env.example` on first run (with a generated database password), builds the images and starts the stack, returning once PostgreSQL and the API report healthy |
| `make down` | Stops and removes the containers; the database volume `froodbase_postgres-data` is kept |
| `make logs` | Follows the logs of all three services |
| `make migrate` | Applies pending schema migrations to the Compose database by running `froodbase-api migrate` from the api image; safe to repeat |
| `make migrate-status` | Lists every migration with its version and whether it is applied |

`make up` also creates `local/users.json` from `local/users.example.json` on first run: three sample users (alice, reviewer; bob, contributor; carol, reader) in the `demo` area, all with the password `froodbase-dev`. The file is ignored by git.

Checks after `make up`:

```bash
curl -s http://127.0.0.1:8080/healthz
```

returns `{"status":"ok","version":"..."}`. `make logs` shows the dreamer's `froodbase-dreamer starting` line. `psql postgres://froodbase:<password>@127.0.0.1:5432/froodbase` connects with the password from `.env`.

To start again with an empty database, run `docker compose down -v`; the password in `.env` is applied when the volume is first created, so change it only together with removing the volume.

## Signing in: static users and basic auth

The local MVP authenticates with HTTP basic auth against a static user list (decision in `docs/dev/decisions.md`; the authorisation-context interface in `internal/authz` is what an Entra implementation replaces later). `froodbase-api` reads the file named by `FROODBASE_USERS_FILE` at startup and refuses to start if it is malformed or a `password_hash` is not a bcrypt hash, so a plaintext password can never be configured by accident.

```json
{
  "users": [
    {
      "username": "alice",
      "display_name": "Alice",
      "password_hash": "$2a$10$...",
      "grants": {"demo": ["reviewer"], "platform": ["reader"]}
    }
  ]
}
```

Roles per knowledge area are `reader`, `contributor`, `reviewer` (which includes contributor and reader) and `admin` (operators only, no implied roles). To add a user, generate a hash and paste it into `local/users.json`, then restart the api:

```bash
echo -n 'new-password' | docker compose run --rm --no-deps -T api hash-password
```

The MCP endpoint is `http://127.0.0.1:8080/mcp` (Streamable HTTP). Every request needs basic auth; without it the answer is 401 and no tool runs. Check your identity and grants with the `whoami` tool:

```bash
curl -s -u alice:froodbase-dev -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"whoami","arguments":{}}}' http://127.0.0.1:8080/mcp
```

The sample users and their shared password are for local development only; anything reachable beyond your computer needs its own users file.

## Migrations

Schema migrations are SQL files under `internal/store/migrations`, named `NNNNN_description.sql` with strictly increasing versions, applied by goose in version order and forward-only: a file has an Up section and no Down section, and a mistake is corrected by a later migration. They are embedded in `froodbase-api`, so `froodbase-api migrate` applies them wherever the binary runs and `froodbase-api migrate --status` lists them; `make migrate` and `make migrate-status` run those commands from the api image against the Compose database. Locally a developer runs them by hand after `make up`; in the cloud only the deployment pipeline runs them (`docs/planning/02-technical-requirements.md`, TR-92). Both server binaries open the database pool at startup and exit with a clear error if PostgreSQL is unreachable, so start the stack before running them from `bin/`.

Behaviour beyond the health endpoint and the dreamer's startup line arrives slice by slice per `docs/planning/07-local-mvp.md`. Running a binary from `bin/` outside Compose needs the two `FROODBASE_` variables the binaries read, below.

## Configuration

Local configuration is environment variables prefixed `FROODBASE_`. `.env.example` lists every one with a comment; `make up` copies it to `.env`, which git ignores. Secrets never go in files under version control.

Read by `compose.yaml` to assemble the stack:

| Variable | Default | Meaning |
|---|---|---|
| `FROODBASE_DB_USER` | `froodbase` | PostgreSQL role, also used in the connection URL |
| `FROODBASE_DB_NAME` | `froodbase` | PostgreSQL database |
| `FROODBASE_DB_PASSWORD` | generated by `make up` | PostgreSQL password; required, never committed |
| `FROODBASE_DB_PORT` | `5432` | Host port on 127.0.0.1 where PostgreSQL is published for `psql` |
| `FROODBASE_API_PORT` | `8080` | Host port on 127.0.0.1 where `froodbase-api` is published |

Read by the binaries (`internal/config`); Compose sets them for the containers:

| Variable | Default | Meaning |
|---|---|---|
| `FROODBASE_API_ADDR` | `:8080` | Listen address of `froodbase-api` |
| `FROODBASE_DATABASE_URL` | none, required | PostgreSQL connection URL (`postgres://user:password@host:port/db?sslmode=disable`) read by `froodbase-api` and `froodbase-dreamer`; its value is never logged |
| `FROODBASE_USERS_FILE` | none, required by `froodbase-api` | Path of the static users JSON file (Compose mounts `local/users.json` at `/etc/froodbase/users.json`) |
