# Development

## Prerequisites

- Go matching the `go` directive in `go.mod`. Froodbase is pure Go: binaries are built with cgo disabled, and on macOS the tests run that way too, so a Mac never needs Xcode or its Command Line Tools for this repository. On Linux the race detector used by `make test` needs a C compiler (CI runners have one).
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

They are tagged `integration` and get their database from `internal/store/storetest`, which starts one `pgvector/pgvector:pg16` container per test binary, migrates a template database once, and clones it for each test (`storetest.Open`, giving an `Admin` pool as the owner and an `App` pool as the service role `froodbase_app`; `storetest.Pool` is the admin pool alone) or gives an empty one (`storetest.EmptyPool`). Row-level security applies to `App` and not to `Admin`, so a test of what a caller can see uses `App`. Nothing is shared between tests and nothing touches the Compose stack. CI runs them in the `integration` job on every pull request.

## Generated queries: sqlc

Hand-written SQL lives in `internal/store/queries/*.sql`; sqlc types it against the migrations and generates `internal/store/gen` (committed, exempt from the `doc.go` rule). After changing a query or a migration:

```bash
make sqlc
```

sqlc v1.30.0 runs through `go run` (pure Go, nothing to install); `make sqlc-check` fails when the committed output is stale, and CI runs it. Configuration is `internal/store/sqlc.yaml`: `uuid` columns are Go strings, `timestamptz` are `time.Time` (pointers when nullable).

## Running the local stack with Compose

`compose.yaml` at the repository root runs PostgreSQL 16 (image `pgvector/pgvector:pg16`, which ships pgvector; `pg_trgm` and `btree_gist` are contrib extensions in the same image), `froodbase-api` and `froodbase-dreamer`. The two server images are built from the root `Dockerfile` with the same version ldflags as `make build`. `froodbase-mcp` is not part of the stack; it runs on the host once slice 0 adds it.

| Target | What it does |
|---|---|
| `make up` | Creates `.env` from `.env.example` on first run (with a generated database password), builds the images, applies pending migrations through the one-shot `migrate` service, and starts the stack, returning once PostgreSQL, the API and the dreamer report healthy |
| `make down` | Stops and removes the containers; the database volume `froodbase_postgres-data` is kept |
| `make logs` | Follows the logs of all three services |
| `make migrate` | Applies pending schema migrations to the Compose database by rerunning the `migrate` service (`froodbase-api migrate`); safe to repeat |
| `make migrate-status` | Lists every migration with its version and whether it is applied, plus River's job queue schema version |

`make up` also generates `FROODBASE_APP_DB_PASSWORD` into `.env`: the password of `froodbase_app`, the service role the api and dreamer connect as. `local/postgres-init/10-app-role.sh` creates that role when the PostgreSQL volume is first initialised. A volume created before it existed gets the role from migration 2 without a login; either start again with `docker compose down -v` (the data is lost) or give it the login from `.env` once:

```bash
printf "ALTER ROLE froodbase_app LOGIN PASSWORD :'p';\n" | docker compose exec -T postgres psql -U froodbase -d froodbase -v p="$(grep '^FROODBASE_APP_DB_PASSWORD=' .env | cut -d= -f2-)"
``` The owner role `froodbase` is used only by the `migrate` service; `froodbase_app` owns nothing, so PostgreSQL's row-level security applies to every record it reads or writes and it cannot disable the policies (see "Records" below).

`make up` also creates `local/users.json` from `local/users.example.json` on first run: three sample users (alice, reviewer; bob, contributor; carol, reader) in the `demo` area, all with the password `froodbase-dev`. The file is ignored by git.

Checks after `make up`:

```bash
curl -s http://127.0.0.1:8080/healthz
```

returns `{"status":"ok","version":"...","min_client_version":"..."}`, and `curl -s http://127.0.0.1:8081/healthz` returns the dreamer's `{"status":"ok","version":"...","queue_depth":N,"index_total":N,"index_stale":N,"reconsider_waiting":N}`. `make logs` shows the dreamer's `froodbase-dreamer starting` line and a `heartbeat` line every minute. `psql postgres://froodbase:<password>@127.0.0.1:5432/froodbase` connects with the password from `.env`.

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

## Five-minute walkthrough in Claude

You speak plainly; Claude structures the calls. The repository skill `.claude/skills/froodbase-contribute/SKILL.md` teaches Claude how: it searches before creating, picks the evidence and assertion kinds, records deductions with their sources and commands as command evidence, mints the idempotency key, leaves out what the server infers, and reports the ids it wrote. It asks you something only when a claim has nothing checkable behind it.

1. Start the stack and build the binaries:

   ```bash
   make up && make build
   ```

2. Register `froodbase-mcp` in Claude Code once (see "Using Froodbase from Claude Code" above for the command; use bob, the contributor, as the user) and open a new Claude Code session in this folder. `/mcp` lists `froodbase` with whoami, contribute, search, get_record and get_history, and the skill loads from the checkout.
3. Say: "What does Froodbase know about me?" Claude calls `whoami`: bob, with the contributor role in `demo`.
4. Say: "Remember this for the import tool: callbacks on 2.3 fail because the new retry waits out the full 30 second timeout. It's in work item https://dev.azure.com/org/p/_workitems/edit/418, revision 12." Claude searches for the import tool, finds nothing, and contributes an entity, the work item as evidence and the observation, without asking for a kind, access class, ref or key. It reports each record's id and the observation's support (one root, threshold met).
5. Say: "So the retry change is the cause, that's my conclusion." Claude records an inference derived from the observation and reports it as an inference below its threshold: one root, inherited from the work item.
6. Say: "I also know we raised the timeout to 30 seconds in September." That is your own knowledge, so Claude records an observation citing your statement (`person://bob`), which weighs less than an observed source.
7. Say: "Search Froodbase for Improt Tool." The entity comes back from the identifier channel as a candidate with its similarity; "callback" finds the observations from the full-text channel with their citations. Ask "show the history of the inference": `get_history` lists its revision, its `root_added` event and the observation it was derived from.
8. Say the finding from step 4 again: Claude finds it by search and tells you it is already recorded instead of writing a duplicate. Ask it to retry its first call with the same idempotency key: `replayed: true` and the same ids, nothing written twice.
9. Meanwhile `make logs` shows the dreamer's `index_record` lines for the records, and `curl -s http://127.0.0.1:8081/healthz` reports `index_stale: 0`.
10. Register a second server entry as carol (reader) or use curl with `-u carol:froodbase-dev`: `get_record` and `search` work, `contribute` is refused with "contributor role required" and the refusal is in `audit_event`.

`TestSlice1Exit` and `TestPlainContribution` in `internal/apitest` run the same calls through the client `froodbase-mcp` uses (`make test-integration`); `TestPlainContribution` sends only the fields a plain request gives.

## Contributing knowledge and reading it back

Three tools arrive with slice 1, available in Claude through `froodbase-mcp` and over HTTP at `/mcp`:

| Tool | What it does |
|---|---|
| `contribute` | Writes one atomic change set to a knowledge area: entities, assertions about them (observations, decisions and inferences) and the shareable evidence behind them. A decision or inference may name the assertions it was derived from. Every record is active at once with the support its distinct evidence roots give it; there is no approval step. Needs the contributor role in the area; a refusal is audited. |
| `get_record` | Reads one record of any type by id with its kind, lifecycle state, support (score, configuration version, counts of roots, verifications and outcomes, threshold verdict), scope, validity, verification date and citations; a deduction also carries `derived: true` and the ids it was `derived_from`. |
| `get_history` | Reads a record's content revisions (actor, time, rationale, snapshot) and, for a claim, its support events and the assertions it was derived from (`derivations`). |
| `search` | Finds records in the areas you can read by an entity name, exact or near-miss (a near-miss is returned as a candidate with its similarity for you to confirm; it is never resolved for you), or by words in assertion bodies (PostgreSQL full text, websearch syntax). Identifier hits come first. Nothing is said about records outside your areas, not even a count. Optional `area` limits the search to one area key; `limit` defaults to 10, at most 50. Reading evidence with the restricted access class, through search or get_record, writes a `restricted_read` audit event. |

Knowledge areas are created at API start-up for every area key the users file grants (the sample file grants `demo`), so the local stack needs no admin step; the cloud move adds an admin tool. Reports, conversations and transcripts are never sent: an agent contributes individual findings, each citing a source the area's audience can check (an https source at a revision, a command it ran, or the caller's own statement), and lists the returned ids in its report.

The server fills in what it can tell from the rest of the call, so neither you nor Claude needs to spell it out: a `ref` for any record nothing refers to (`entity-1`, `evidence-2`, `assertion-1`, `observation-1`, by position in its list); the `source_kind` of well-known links (Azure DevOps work items, Jira issues at `/browse/KEY-1` and GitHub issues are `ticket`; GitHub, GitLab and Bitbucket blob, tree and commit URLs and Azure DevOps `_git` URLs are `repository`; Confluence, SharePoint and Azure DevOps wiki pages are `document`; the `azure`, `k8s`, `sql` and `host` schemes are `command` and `person://` is `statement`); `access_class` as `area`; and the `source_revision` of a repository URL that pins a full commit SHA (in the path, or Azure DevOps `version=GC<sha>`). Anything else stays required and is refused with a message saying what to pass: a link it does not recognise needs its `source_kind`, and an https source that does not pin a commit needs its `source_revision`. A call that spells out what would be inferred is the same request.

Every `contribute` call carries an `idempotency_key`. Retrying with the same key and the same content returns the stored result (`replayed: true`) and writes nothing; the same key with different content is refused. Keys are per person. An observation must cite at least one piece of evidence (PR-43); evidence the area already holds is cited by id rather than recorded again, and recording the same source, revision and locator twice is refused with the message to cite the existing id.

A minimal call from the terminal (bob is a contributor in `demo`):

```bash
curl -s -u bob:froodbase-dev -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"contribute","arguments":{
    "area":"demo","idempotency_key":"demo-1","rationale":"seen during INC-1",
    "entities":[{"ref":"tool","type":"service","names":[{"name":"Import Tool"}]}],
    "evidence":[{"ref":"ticket","source_uri":"https://dev.azure.com/org/p/_workitems/edit/418","source_revision":"2026-10-20"}],
    "observations":[{"ref":"obs","subject":"tool","body_markdown":"Callback failure reproduced on version 2.3","scope":{"environment":"prod","version_min":"2.3","version_max":"2.3"},"evidence":["ticket"]}]}}}' \
  http://127.0.0.1:8080/mcp
```

The server infers the evidence's `source_kind` (`ticket`) and `access_class` (`area`). The result lists each record with its id; pass an id to `get_record` or `get_history` the same way (`"arguments":{"id":"..."}`).

### Deductions: decisions and inferences

`assertions` is the general form of the change set's claims; `observations` is the slice 1 form and is still accepted as assertions of kind `observation`. Each assertion has a `kind` that never changes: `observation` (what was seen), `decision` (what was approved or intended) or `inference` (what was concluded). An observation cites evidence and is never derived. A decision or inference cites evidence, names in `derived_from` the assertions it was concluded from (refs of assertions in the same call, in any order, or ids of assertions in the area), or both; it needs at least one.

A deduction starts with one `root_added` support event per distinct evidence root of its sources and of its own evidence: a root two sources share counts once, and an inherited root's event says which sources it came from (`inherited from <ids>`). With the service default configuration an inference needs a score of 2 to meet its threshold, so an inference whose sources rest on one root is stored, returned by `get_record` and `search` with `meets_threshold: false`, and earns more support only as any claim does: further distinct roots, verifications and outcomes. Reading it or recording the same conclusion again earns nothing. Deriving from an assertion in another area, from an unknown id, or in a loop is refused and nothing is written. When a source gains a new root later, the dreamer passes it on: every deduction derived from that source, directly or through other deductions, gains the root unless it already counts it, with a `root_added` event whose `job_name` is `reconsider_derived` and whose rationale names the source it was inherited from. The score changes, the revision does not. A source losing support (supersession, retraction, a changed source) does not reach its deductions yet (FRB-25).

```bash
curl -s -u bob:froodbase-dev -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"contribute","arguments":{
    "area":"demo","idempotency_key":"demo-2","rationale":"concluded during INC-1",
    "assertions":[{"ref":"cause","kind":"inference","subject":"<entity id>","body_markdown":"The callback failure is caused by the 2.3 retry change","derived_from":["<observation id>","<observation id>"]}]}}}' \
  http://127.0.0.1:8080/mcp
```

The same flow runs as `TestDeductionFromTwoObservations` in `internal/apitest`.

### Facts from commands and from people

Not every fact has a web link. Two source kinds cover the rest:

- `command`: a fact found by running a command or query. `source_uri` names what was observed: an Azure resource as `azure:///` followed by its ARM id (`azure:///subscriptions/<id>/resourceGroups/<rg>/providers/...`), a Kubernetes object as `k8s://<cluster>/<namespace>/<kind>/<name>` (or `k8s://<cluster>/<kind>/<name>` for a cluster-scoped one), a database as `sql://<server>/<database>`, or any other machine as `host://<name>`. `locator` is the exact command or query with secrets removed, `observed_at` when it ran, `excerpt` optionally the output relied on. `source_revision` defaults to `observed_at`, so running the same command again later is new evidence and a second root.
- `statement`: something the caller knows from experience. `source_uri` is `person://<your user id>`, filled in when left out; `excerpt` is what you said and `observed_at` when; `source_revision` defaults to `observed_at`. You can only record your own statements: cite someone else's through where they wrote it down (a ticket or document). A call with no person behind it cannot record a statement. One person's statements count as one root per claim, and a statement root weighs 0.5 under the service default support configuration (observed sources weigh 1), so a claim resting on a statement alone is stored with `meets_threshold: false` until something observed backs it.

Anything in `source_uri`, `locator` or `excerpt` that looks like a credential (a connection-string or URL password, a bearer token, a key, token or secret value, a JWT, a private key) is refused with the kind of secret it looks like; replace the value with `***` and contribute again. The check is a guard: scrub command output before you send it.

The support in `contribute` results and `get_record` lists `roots_by_source_kind`, each kind with its root count and per-root weight, and `get_record` returns command and statement citations like any other, with their `locator`, `excerpt` and `observed_at`.

```bash
curl -s -u bob:froodbase-dev -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"contribute","arguments":{
    "area":"demo","idempotency_key":"demo-3","rationale":"checked during INC-9",
    "entities":[{"ref":"app","type":"service","names":[{"name":"Import Tool web app"}]}],
    "evidence":[
      {"ref":"run","source_kind":"command","source_uri":"azure:///subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg-import/providers/Microsoft.Web/sites/import-tool",
       "locator":"az webapp config show --ids /subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg-import/providers/Microsoft.Web/sites/import-tool --query http20Enabled",
       "excerpt":"true","observed_at":"2026-10-21T09:00:00Z","access_class":"area"},
      {"ref":"said","source_kind":"statement","excerpt":"We turned HTTP/2 on for the import tool in September.","observed_at":"2026-10-21T09:15:00Z","access_class":"area"}],
    "assertions":[
      {"ref":"http2","kind":"observation","subject":"app","body_markdown":"HTTP/2 is enabled on the import tool web app","evidence":["run"]},
      {"ref":"since","kind":"observation","subject":"app","body_markdown":"HTTP/2 was turned on for the import tool in September 2026","evidence":["said"]}]}}}' \
  http://127.0.0.1:8080/mcp
```

The first observation has one `command` root weighing 1 and meets its threshold; the second has one `statement` root (`person://bob`) weighing 0.5 and does not. The same flow, with a second run of the command and the refusals, runs as `TestCommandObservationAndStatement` in `internal/apitest`.

## Migrations

Migration `00003_idempotency.sql` adds the per-caller idempotency keys of `contribute`.

Schema migrations are SQL files under `internal/store/migrations`, named `NNNNN_description.sql` with strictly increasing versions, applied by goose in version order and forward-only: a file has an Up section and no Down section, and a mistake is corrected by a later migration. Before them, `froodbase-api migrate` applies River's own job queue schema (the `river_job` and related tables), which River versions itself, so a goose migration can grant the service role access to River's tables. Nothing is granted to `froodbase_app` by default: a migration that adds a table also grants what the role needs, otherwise the service fails closed with a permission error at startup.

| Version | File | Creates |
|---|---|---|
| 1 | `00001_extensions.sql` | The `vector`, `pg_trgm` and `btree_gist` extensions |
| 2 | `00002_records.sql` | The slice 1 record tables, the service role `froodbase_app` and its grants, row-level security on every table, the seeded service default support configuration |
| 3 | `00003_idempotency.sql` | The per-caller idempotency keys of `contribute` |
| 4 | `00004_index_state.sql` | The `index_state` projection bookkeeping and the staleness function the dreamer reports |
| 5 | `00005_derivation.sql` | The append-only `derivation` table linking a deduction to its sources in the same area, with row-level security and its grants |
| 6 | `00006_command_statement_evidence.sql` | The `command` and `statement` source kinds (statements unique on what was said as well), the root's `source_kind` on support events (backfilled), and `root_weight_by_source_kind` on every support configuration |

The migrations are embedded in `froodbase-api`, so `froodbase-api migrate` applies them wherever the binary runs and `froodbase-api migrate --status` lists them. Locally the Compose `migrate` service runs them before the api and dreamer start, and `make migrate` reruns them after you add a migration; in the cloud only the deployment pipeline runs them (`docs/planning/02-technical-requirements.md`, TR-92). Both server binaries open the database pool at startup and exit with a clear error if PostgreSQL is unreachable, so start the stack before running them from `bin/`.

## Records: history, audit and per-area access control

Migration `00002_records.sql` creates the slice 1 tables (`knowledge_area`, `entity`, `entity_name`, `assertion`, `evidence`, `claim_evidence`, `contribution`, `contribution_claims`, `revision`, `audit_event`, `support_event`, `support_config`), the service role `froodbase_app` (without a login when it does not exist yet; Compose and `storetest` give it one) and its grants, and enables row-level security on every one of them. The policies compare each row's `area_id` with `app.caller_areas`, a comma-separated list of area ids the service sets per transaction with `set_config('app.caller_areas', ..., true)` from the caller's grants (`store.Caller`); unset means no rows. Migration `00005_derivation.sql` adds `derivation` (derived id, source id, area, rationale, recorded time) on the same pattern; composite foreign keys keep both ends in the link's area. `revision`, `audit_event`, `support_event` and `derivation` have insert and select policies only and no update or delete grant, so history is append-only at both levels. `support_config` holds the service default (`area_id` null, version `service-1`, the values of `domain.DefaultSupportConfig`) and each area's own versions.

Every write goes through `store.Pool`'s writer, which in one transaction inserts the records, a full JSON snapshot per record revision, an audit event per write, one `root_added` support event per distinct evidence root with the projected score, and the `index_record` job for the dreamer; a failure anywhere rolls all of it back. Record ids are UUIDs from `store.NewID`. Inspect what a write left behind from the Compose stack:

```bash
docker compose exec -T postgres psql -U froodbase -d froodbase -c "select record_type, record_id, revision, actor_user_id, rationale from revision order by recorded_at"
```

## Background jobs: the dreamer

`froodbase-dreamer` works background jobs from the River queue in PostgreSQL. Jobs are enqueued by `froodbase-api` inside the transaction that changes a record (the outbox pattern), so a job exists only if its record was committed; periodic jobs are inserted by the dreamer instance River elects as leader, so two instances never both schedule them. Argument types live in `internal/store/jobs`, one per job kind; workers live in `internal/dreamer`. Slice 0 has one kind, `heartbeat`, which runs every minute (and once at startup) and is unique per minute; slice 1 adds `index_record`, enqueued by the store's writer for every entity and assertion revision (its worker arrives with the slice 1 dreamer ticket). The heartbeat's completed rows in `river_job` are the proof the dreamer is alive:

```bash
docker compose exec -T postgres psql -U froodbase -d froodbase -c "select id, kind, state, finalized_at from river_job order by id desc limit 5"
```

Slice 1 adds `index_record`, enqueued in the same transaction as every entity or assertion write (the outbox pattern): the worker refreshes the record's projections at its current revision and records it in `index_state`, the projection bookkeeping (identifiers and revisions only, never content). The job is unique per record while it waits or runs, so a burst of writes to one record collapses into one job. `rebuild_index` re-indexes every record of one area on demand:

```bash
docker compose run --rm --no-deps dreamer rebuild-index <area-id>
```

The health endpoint on `FROODBASE_DREAMER_ADDR` reports the queue depth (jobs waiting or running), `index_total` (records across every area) and `index_stale` (records not indexed at their current revision), and answers 503 with `"status":"degraded"` when the database cannot be read. A write that commits in the moment between a running index job reading the record and finishing stays stale until the next write or a rebuild; the count makes that visible. Migration `00004_index_state.sql` adds the table and the counting function.

Slice 1b adds `reconsider_derived`, enqueued in the same transaction whenever a record that something is derived from gains an evidence root. It walks every record derived from it, transitively, sources first, each once even if links loop, and appends to each the roots its sources have and it has not counted; it stays in the record's area and writes support events and scores, never revisions. Several root additions to one record before the dreamer reaches it collapse into one job, which reads the sources' current state, and the job completes in the same transaction as the walk so a root added meanwhile is never lost. The health endpoint's `reconsider_waiting` counts the jobs waiting, and `make logs` shows a `reconsider_derived` line per walk with how many records it changed.

Behaviour beyond the health endpoint and the dreamer's startup line arrives slice by slice per `docs/planning/07-local-mvp.md`. Running a binary from `bin/` outside Compose needs the two `FROODBASE_` variables the binaries read, below.

## Configuration

Local configuration is environment variables prefixed `FROODBASE_`. `.env.example` lists every one with a comment; `make up` copies it to `.env`, which git ignores. Secrets never go in files under version control.

Read by `compose.yaml` to assemble the stack:

| Variable | Default | Meaning |
|---|---|---|
| `FROODBASE_DB_USER` | `froodbase` | PostgreSQL role, also used in the connection URL |
| `FROODBASE_DB_NAME` | `froodbase` | PostgreSQL database |
| `FROODBASE_DB_PASSWORD` | generated by `make up` | Password of the owner role, used by the `migrate` service; required, never committed |
| `FROODBASE_APP_DB_PASSWORD` | generated by `make up` | Password of the service role `froodbase_app`, used by the api and dreamer; required, never committed |
| `FROODBASE_DB_PORT` | `5432` | Host port on 127.0.0.1 where PostgreSQL is published for `psql` |
| `FROODBASE_API_PORT` | `8080` | Host port on 127.0.0.1 where `froodbase-api` is published |
| `FROODBASE_DREAMER_PORT` | `8081` | Host port on 127.0.0.1 where the dreamer's health endpoint is published |

Read by the binaries (`internal/config`); Compose sets them for the containers:

| Variable | Default | Meaning |
|---|---|---|
| `FROODBASE_API_ADDR` | `:8080` | Listen address of `froodbase-api` |
| `FROODBASE_DREAMER_ADDR` | `:8081` | Health listen address of `froodbase-dreamer` (queue depth) |
| `FROODBASE_DATABASE_URL` | none, required | PostgreSQL connection URL (`postgres://user:password@host:port/db?sslmode=disable`) read by `froodbase-api` and `froodbase-dreamer` (as `froodbase_app`) and by `froodbase-api migrate` (as the owner); its value is never logged |
| `FROODBASE_USERS_FILE` | none, required by `froodbase-api` | Path of the static users JSON file (Compose mounts `local/users.json` at `/etc/froodbase/users.json`) |

Read by `froodbase-mcp` on the host, normally set in the Claude Code registration:

| Variable | Default | Meaning |
|---|---|---|
| `FROODBASE_API_URL` | none, required | The API's MCP endpoint, `http://127.0.0.1:8080/mcp` for the Compose stack |
| `FROODBASE_USERNAME` | none, required | A username from the users file |
| `FROODBASE_PASSWORD` | none, required | That user's password; never logged |
