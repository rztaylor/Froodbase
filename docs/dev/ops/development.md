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

Integration tests need Docker and are kept out of that gate:

```bash
make test-integration
```

They are tagged `integration` and get their database from `internal/store/storetest`, which starts one `pgvector/pgvector:pg16` container per test binary, migrates a template database once, and clones it for each test (`storetest.Pool`) or gives an empty one (`storetest.EmptyPool`). Nothing is shared between tests and nothing touches the Compose stack. CI runs them in the `integration` job on every pull request.

## Running the local stack with Compose

`compose.yaml` at the repository root runs PostgreSQL 16 (image `pgvector/pgvector:pg16`, which ships pgvector; `pg_trgm` and `btree_gist` are contrib extensions in the same image), `froodbase-api` and `froodbase-dreamer`. The two server images are built from the root `Dockerfile` with the same version ldflags as `make build`. `froodbase-mcp` is not part of the stack; it runs on the host once slice 0 adds it.

| Target | What it does |
|---|---|
| `make up` | Creates `.env` from `.env.example` on first run (with a generated database password), builds the images, applies pending migrations through the one-shot `migrate` service, and starts the stack, returning once PostgreSQL, the API and the dreamer report healthy |
| `make down` | Stops and removes the containers; the database volume `froodbase_postgres-data` is kept |
| `make logs` | Follows the logs of all three services |
| `make migrate` | Applies pending schema migrations to the Compose database by rerunning the `migrate` service (`froodbase-api migrate`); safe to repeat |
| `make migrate-status` | Lists every migration with its version and whether it is applied, plus River's job queue schema version |

`make up` also creates `local/users.json` from `local/users.example.json` on first run: three sample users (alice, reviewer; bob, contributor; carol, reader) in the `demo` area, all with the password `froodbase-dev`. The file is ignored by git.

Checks after `make up`:

```bash
curl -s http://127.0.0.1:8080/healthz
```

returns `{"status":"ok","version":"...","min_client_version":"..."}`, and `curl -s http://127.0.0.1:8081/healthz` returns the dreamer's `{"status":"ok","version":"...","queue_depth":N}`. `make logs` shows the dreamer's `froodbase-dreamer starting` line and a `heartbeat` line every minute. `psql postgres://froodbase:<password>@127.0.0.1:5432/froodbase` connects with the password from `.env`.

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

## Using Froodbase from Claude Code

`froodbase-mcp` is the stdio MCP server Claude Code launches on your computer. It forwards every tool call to `froodbase-api` with your credentials and never touches the database. Build it with `make build`, then register it once (replace the path with your checkout):

```bash
claude mcp add froodbase \
  -e FROODBASE_API_URL=http://127.0.0.1:8080/mcp \
  -e FROODBASE_USERNAME=alice \
  -e FROODBASE_PASSWORD=froodbase-dev \
  -- /Users/you/src/Froodbase/bin/froodbase-mcp
```

`froodbase-mcp` refuses to start if any of the three variables is missing and never prints the password. In Claude Code, `/mcp` lists the `froodbase` server and its tools; ask Claude to call `whoami` and it reports your user id, display name and grants per knowledge area.

When `froodbase-api` is not running (for example the stack is down), every tool returns a result with `"froodbase_unavailable": true`, the API address, the cause, and the instruction to continue with packaged and local knowledge and to record in the report that Froodbase could not be reached. It is a normal result, not an error, so an agent can carry on (TR-102). Wrong credentials are different: they produce a tool error naming the two variables to check.

Every call carries the client version in the `Froodbase-Client-Version` header. The API publishes its minimum compatible client version on `/healthz` (`min_client_version`) and logs a warning when a client is below it; development builds are always accepted.

Without Claude Code you can drive the binary over stdin; the stream must stay open until the answers arrive:

```bash
( printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"whoami","arguments":{}}}'; sleep 3 ) \
  | FROODBASE_API_URL=http://127.0.0.1:8080/mcp FROODBASE_USERNAME=alice FROODBASE_PASSWORD=froodbase-dev bin/froodbase-mcp 2>/dev/null
```

## Migrations

Schema migrations are SQL files under `internal/store/migrations`, named `NNNNN_description.sql` with strictly increasing versions, applied by goose in version order and forward-only: a file has an Up section and no Down section, and a mistake is corrected by a later migration. After them, `froodbase-api migrate` applies River's own job queue schema (the `river_job` and related tables), which River versions itself. The migrations are embedded in `froodbase-api`, so `froodbase-api migrate` applies them wherever the binary runs and `froodbase-api migrate --status` lists them. Locally the Compose `migrate` service runs them before the api and dreamer start, and `make migrate` reruns them after you add a migration; in the cloud only the deployment pipeline runs them (`docs/planning/02-technical-requirements.md`, TR-92). Both server binaries open the database pool at startup and exit with a clear error if PostgreSQL is unreachable, so start the stack before running them from `bin/`.

## Background jobs: the dreamer

`froodbase-dreamer` works background jobs from the River queue in PostgreSQL. Jobs are enqueued by `froodbase-api` inside the transaction that changes a record (the outbox pattern), so a job exists only if its record was committed; periodic jobs are inserted by the dreamer instance River elects as leader, so two instances never both schedule them. Argument types live in `internal/store/jobs`, one per job kind; workers live in `internal/dreamer`. Slice 0 has one kind, `heartbeat`, which runs every minute (and once at startup) and is unique per minute. Its completed rows in `river_job` are the proof the dreamer is alive:

```bash
docker compose exec -T postgres psql -U froodbase -d froodbase -c "select id, kind, state, finalized_at from river_job order by id desc limit 5"
```

The health endpoint on `FROODBASE_DREAMER_ADDR` reports the queue depth (jobs waiting or running) and answers 503 with `"status":"degraded"` when the database cannot be read.

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
| `FROODBASE_DREAMER_PORT` | `8081` | Host port on 127.0.0.1 where the dreamer's health endpoint is published |

Read by the binaries (`internal/config`); Compose sets them for the containers:

| Variable | Default | Meaning |
|---|---|---|
| `FROODBASE_API_ADDR` | `:8080` | Listen address of `froodbase-api` |
| `FROODBASE_DREAMER_ADDR` | `:8081` | Health listen address of `froodbase-dreamer` (queue depth) |
| `FROODBASE_DATABASE_URL` | none, required | PostgreSQL connection URL (`postgres://user:password@host:port/db?sslmode=disable`) read by `froodbase-api` and `froodbase-dreamer`; its value is never logged |
| `FROODBASE_USERS_FILE` | none, required by `froodbase-api` | Path of the static users JSON file (Compose mounts `local/users.json` at `/etc/froodbase/users.json`) |

Read by `froodbase-mcp` on the host, normally set in the Claude Code registration:

| Variable | Default | Meaning |
|---|---|---|
| `FROODBASE_API_URL` | none, required | The API's MCP endpoint, `http://127.0.0.1:8080/mcp` for the Compose stack |
| `FROODBASE_USERNAME` | none, required | A username from the users file |
| `FROODBASE_PASSWORD` | none, required | That user's password; never logged |
