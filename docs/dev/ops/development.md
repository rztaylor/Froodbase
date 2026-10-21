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

You speak plainly; Claude structures the calls. The repository skill `.claude/skills/froodbase-contribute/SKILL.md` teaches Claude how: it searches before creating, picks the evidence and assertion kinds, records deductions with their sources and commands as command evidence, adds a new source to a claim already recorded instead of recording the claim again, records the findings in a document one by one with a citation to each section, mints the idempotency key, leaves out what the server infers, and reports the ids it wrote. It asks you something only when a claim has nothing checkable behind it.

1. Start the stack and build the binaries:

   ```bash
   make up && make build
   ```

2. Register `froodbase-mcp` in Claude Code once (see "Using Froodbase from Claude Code" above for the command; use bob, the contributor, as the user) and open a new Claude Code session in this folder. `/mcp` lists `froodbase` with whoami, contribute, search, get_record, get_as_of and get_history, and the skill loads from the checkout.
3. Say: "What does Froodbase know about me?" Claude calls `whoami`: bob, with the contributor role in `demo`.
4. Say: "Remember this for the import tool: callbacks on 2.3 fail because the new retry waits out the full 30 second timeout. It's in work item https://dev.azure.com/org/p/_workitems/edit/418, revision 12." Claude searches for the import tool, finds nothing, and contributes an entity, the work item as evidence and the observation, without asking for a kind, access class, ref or key. It reports each record's id and the observation's support (one root, threshold met).
5. Say: "So the retry change is the cause, that's my conclusion." Claude records an inference derived from the observation and reports it as an inference below its threshold: one root, inherited from the work item.
6. Say: "Work item https://dev.azure.com/org/p/_workitems/edit/419, revision 3, shows the same callback failure." Claude finds the observation by search and adds the work item to it under `links` instead of recording it again. It reports the observation under `linked` with `roots_added: 1`, two roots and still revision 1. Ask for the inference a few seconds later: the dreamer has passed the new root on, so it has two roots and meets its threshold.
7. Say: "I also know we raised the timeout to 30 seconds in September." That is your own knowledge, so Claude records an observation citing your statement (`person://bob`), which weighs less than an observed source.
8. Say: "Search Froodbase for Improt Tool." The entity comes back from the identifier channel as a candidate with its similarity; "callback" finds the observations from the full-text channel with their citations. Ask "show the history of the inference": `get_history` lists its one revision, its two `root_added` events (the second appended by `reconsider_derived`, inherited from the observation) and the observation it was derived from.
9. Say the finding from step 4 again: Claude finds it by search and tells you it is already recorded instead of writing a duplicate. Ask it to retry its first call with the same idempotency key: `replayed: true` and the same ids, nothing written twice.
10. Meanwhile `make logs` shows the dreamer's `index_record` lines for the records, and `curl -s http://127.0.0.1:8081/healthz` reports `index_stale: 0`.
11. Register a second server entry as carol (reader) or use curl with `-u carol:froodbase-dev`: `get_record` and `search` work, `contribute` is refused with "contributor role required" and the refusal is in `audit_event`.
12. Say: "Record the useful information in docs/dev/guides/sample-design-note.md." Claude cites the note at its commit, one section per finding, joins the import tool from step 4, and lists the open questions it did not record. "Recording the findings in a document" below walks through what it writes.
13. Say: "Check the callback timeout setting on the import tool's web app in Azure and remember it." This needs `az` signed in to a subscription with a web app you can read; name your own app, or read the example call under "Facts from commands and from people" below instead. Claude runs `az webapp config appsettings list --ids <ARM id> --query "[?name=='CALLBACK_TIMEOUT_SECONDS'].value" -o tsv`, checks the output for secrets, and contributes an observation citing the run as command evidence: the app as `azure:///<ARM id>`, the command as `locator`, the output as `excerpt` and the time it ran as `observed_at`. It reports one `command` root.
14. Say: "So callbacks fail because each retry waits out that timeout." Claude records an inference derived from the work-item observation of step 4 and the command observation of step 13, with no citation of its own. It inherits the roots of both: the two work items (step 6) and the command run, so it meets its threshold. Then ask "where does that conclusion come from?": `get_record` on the inference shows `derived: true` and the two observations in `derived_from`, and `get_record` on each shows its citations: the work items at their revisions, and the `az` command with the resource, the output and when it ran. `get_history` on the inference lists both derivations and one inherited `root_added` event per root.
15. Say: "The old investigation in work item https://dev.azure.com/org/p/_workitems/edit/77, revision 1, concluded callbacks on 2.3 failed from 2 September 2026 until the 2.4 deployment on 9 September." Claude records an inference with `valid_from` 2026-09-02, `valid_to` 2026-09-09 (exclusive) and scope `version_min` and `version_max` 2.3: the dates come from what you said, never from today. It is recorded today about an earlier time.
16. Say: "What is true about the import tool's callbacks now?" Claude searches in the default `current` mode. Step 15's inference is not there: its validity has ended. The step 4 observation comes back `status: current` with `validity_unknown: true`, which Claude reports as "not known to have ended", not as verified current. A record whose last check is older than its area's freshness policy (90 days for observations and inferences) would carry `stale: true` and still be returned.
17. Say: "What was true on 3 September?" Claude searches with `mode: as_of` and `as_of: 2026-09-03T00:00:00Z`: step 15's inference is found although it was recorded weeks later, labelled `historical`. Ask "was it still true on 1 October?": `get_as_of` answers `valid: false`, its validity ended on 9 September. Ask for "everything, past included": `mode: historical` returns every record, each labelled current, historical or upcoming. Add "on 2.4 only" and Claude passes `scope: {version_min: "2.4", version_max: "2.4"}`, which leaves out the 2.3 records.

`TestSlice1Exit`, `TestPlainContribution`, `TestEvidenceForExistingClaim`, `TestDocumentFindings` and `TestSlice1bExit` in `internal/apitest` run the same calls through the client `froodbase-mcp` uses (`make test-integration`); `TestPlainContribution` sends only the fields a plain request gives, `TestEvidenceForExistingClaim` is step 6, `TestDocumentFindings` is step 12, and `TestSlice1bExit` is the slice 1b exit: steps 4, 13, 14 and 7, then step 6 with the dreamer strengthening the inference, then step 12, and every record read back with its sources through `get_record`, `get_history` and `search`. `TestTwoClocks` is steps 15 to 17: records with and without dates and scope, a superseded one, each mode, `get_as_of` before, during and after a record's validity, and the stale flag.

## Contributing knowledge and reading it back

Three tools arrive with slice 1, available in Claude through `froodbase-mcp` and over HTTP at `/mcp`:

| Tool | What it does |
|---|---|
| `contribute` | Writes one atomic change set to a knowledge area: entities, assertions about them (observations, decisions and inferences) and the shareable evidence behind them. A decision or inference may name the assertions it was derived from. `links` adds evidence to claims already recorded. Every record is active at once with the support its distinct evidence roots give it; there is no approval step. Needs the contributor role in the area; a refusal is audited. |
| `get_record` | Reads one record of any type by id with its kind, lifecycle state, support (score, configuration version, counts of roots, verifications and outcomes, threshold verdict), scope, validity, verification date and citations; a deduction also carries `derived: true` and the ids it was `derived_from`. |
| `get_history` | Reads a record's content revisions (actor, time, rationale, snapshot) and, for a claim, its support events and the assertions it was derived from (`derivations`). |
| `search` | Finds records in the areas you can read by an entity name, exact or near-miss (a near-miss is returned as a candidate with its similarity for you to confirm; it is never resolved for you), or by words in assertion bodies (PostgreSQL full text, websearch syntax). Identifier hits come first. Nothing is said about records outside your areas, not even a count. Optional `area` limits the search to one area key; `limit` defaults to 10, at most 50. `mode` is `current` (the default), `as_of` with an `as_of` time, or `historical`, and optional `scope` keeps records whose scope overlaps it (see "What is true now, what was true then" below). Reading evidence with the restricted access class, through search or get_record, writes a `restricted_read` audit event. |
| `get_as_of` | Reads one assertion or entity as valid at `as_of`, judged by its validity, not its recording time: `valid: true` with the record (an entity with only the names valid then), or `valid: false` with the reason. Evidence has no validity of its own and is refused. |

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

### Adding evidence to something already recorded

Corroboration found later goes to the claim already recorded, not into a second copy of it: recording the same claim again earns nothing. `links` lists claims the area already holds by id, each with the evidence it should now cite: refs of evidence in the same call or ids of evidence in the area. The claim gains one `root_added` event per distinct root it did not count yet, its score is re-projected and an `evidence_linked` audit event names the contribution. Its content and revision do not change. Deductions derived from it then gain the root through the dreamer's `reconsider_derived` job, as above. Evidence the claim already counts (the same evidence, a restatement of a root it cites, a root it inherits, another statement from the same person) adds nothing. Links are written before the call's new assertions, so a deduction in the same call that is derived from a linked claim inherits the new roots at once.

A link to an unknown id, to something that is not an assertion, or to a claim or evidence in another area is refused, as is a link naming a ref from the same call (a new claim cites its evidence in its own `evidence` list), and nothing is written. The result lists the new evidence under `records` as usual and each linked claim under `linked` with its kind, its unchanged `revision`, the `evidence` ids linked, `roots_added` and its `support` after the link. A retry with the same idempotency key replays it.

```bash
curl -s -u bob:froodbase-dev -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"contribute","arguments":{
    "area":"demo","idempotency_key":"demo-4","rationale":"the same failure in another work item",
    "evidence":[{"ref":"wi","source_uri":"https://dev.azure.com/org/p/_workitems/edit/419","source_revision":"3"}],
    "links":[{"claim":"<observation id>","evidence":["wi"]}]}}}' \
  http://127.0.0.1:8080/mcp
```

The same flow, with the dreamer strengthening an inference derived from the observation, a replay and the refusals, runs as `TestEvidenceForExistingClaim` in `internal/apitest`.

### Recording the findings in a document

A person can point Claude at a document and ask it to record what is useful in it. Froodbase never receives the document: reports and private files are never an input (PR-50 to PR-52), and `froodbase-mcp` reads no local files. Claude reads the document with its own tools and contributes its findings one by one, each citing where in the document it came from. The skill section "Recording the findings in a document" teaches it how.

The worked example uses [`docs/dev/guides/sample-design-note.md`](../guides/sample-design-note.md), a short fictional design note about the import tool from the walkthrough. Run walkthrough step 4 first, so the area already knows the import tool and the work item's finding, then say in Claude Code: "Record the useful information in docs/dev/guides/sample-design-note.md."

1. **The citation.** The note is committed, so the note is the source, cited at the last commit that changed it: Claude runs `git log -1 --format=%H -- docs/dev/guides/sample-design-note.md` and checks the commit is pushed (`git branch -r --contains <sha>`). The URL is `https://github.com/rztaylor/Froodbase/blob/<sha>/docs/dev/guides/sample-design-note.md`. The server infers `repository` and the revision from the SHA. If the file has uncommitted changes, or the commit is not pushed, nobody else could check that version, so Claude treats it as a local file (below).
2. **Search first.** `search` for "Import Tool" finds the entity from step 4, so every finding about the tool uses its id. "Callback Gateway" finds nothing, so the change set creates it. "callbacks timeout" finds the work item's observation: the note's Background section only restates that work item, so it adds nothing and is not recorded again.
3. **One change set.** The note is recorded once without a locator, followed by one evidence record per section cited (`Measurements`, `Analysis`, `Decision`), each naming the note as its `root`. Then the claims:

   | Section | Claim | Kind | Rests on | Support |
   |---|---|---|---|---|
   | Measurements | Callbacks on 2.3.0 took 30.2 s in staging, against 1.1 s on 2.2.4 | observation | the section | 1 root, meets threshold |
   | Measurements | The callback gateway gives up after three failed attempts | observation | the section | 1 root, meets threshold |
   | Analysis | The 2.3 retry waits out the full timeout before each retry | inference | the recorded work-item observation and the measurement, plus the section | 2 roots (the work item and the note), meets threshold |
   | Analysis | Slow retries probably make the gateway drop callbacks under load | inference | the measurement and the attempt limit, plus the section | 1 root (the note), below threshold |
   | Decision | From 2.3.2 the timeout is capped at 5 s, retries back off from 1 s | decision | the section, derived from the cause; `valid_from` 2026-10-14, the date the note states | 2 roots, meets threshold |

   Because every section names the note as its root, the note counts once: the gateway deduction rests on two of the note's own findings and still has one root. It needs a source outside the note to meet its threshold.
4. **Not recorded.** The two open questions are not findings. Claude lists them in its report with the Background finding (already recorded as the work item's observation) and the ids it wrote.

The call Claude builds (ids in angle brackets come from the search results):

```json
{
  "area": "demo", "idempotency_key": "bob-sample-design-note", "rationale": "findings of the sample design note",
  "entities": [{"ref": "gateway", "type": "service", "names": [{"name": "Callback Gateway"}]}],
  "evidence": [
    {"ref": "doc", "source_uri": "https://github.com/rztaylor/Froodbase/blob/<sha>/docs/dev/guides/sample-design-note.md"},
    {"ref": "measurements", "source_uri": "<the same URL>", "locator": "Measurements", "root": "doc"},
    {"ref": "analysis", "source_uri": "<the same URL>", "locator": "Analysis", "root": "doc"},
    {"ref": "decision", "source_uri": "<the same URL>", "locator": "Decision", "root": "doc", "excerpt": "From version 2.3.2 the callback timeout is capped at 5 seconds"}
  ],
  "assertions": [
    {"ref": "measured", "kind": "observation", "subject": "<import tool id>", "evidence": ["measurements"], "scope": {"environment": "staging", "version_min": "2.3.0", "version_max": "2.3.0"},
     "body_markdown": "In staging, callbacks on 2.3.0 took 30.2 seconds on average over 200 imports, against 1.1 seconds on 2.2.4"},
    {"ref": "attempts", "kind": "observation", "subject": "gateway", "evidence": ["measurements"], "body_markdown": "The callback gateway gives up on a callback after three failed attempts"},
    {"ref": "cause", "kind": "inference", "subject": "<import tool id>", "evidence": ["analysis"], "derived_from": ["<work-item observation id>", "measured"],
     "body_markdown": "The 2.3 retry waits out the full 30 second timeout before each retry"},
    {"ref": "drops", "kind": "inference", "subject": "gateway", "evidence": ["analysis"], "derived_from": ["measured", "attempts"],
     "body_markdown": "Slow retries on import tool 2.3 probably make the callback gateway drop callbacks under load"},
    {"kind": "decision", "subject": "<import tool id>", "evidence": ["decision"], "derived_from": ["cause"], "valid_from": "2026-10-14T00:00:00Z", "scope": {"version_min": "2.3.2"},
     "body_markdown": "From 2.3.2 the callback timeout is capped at 5 seconds and retries back off from 1 second"}
  ]
}
```

`get_record` on any of the claims returns its citation with the note's URL, the commit as `source_revision`, the section as `locator` and the note's evidence id as `root_evidence_id`; `get_history` on the gateway deduction shows a single `root_added` event for the note.

The same document kept only on your computer (a copy outside the repository, a download, an unpushed draft) cannot be cited: a `file://` URL or a local path is refused, because nobody else can open it, and nothing is written. Claude cites what the document relied on instead: the work item it names, a command it quotes (run again and cited as command evidence), and, if you agree to state it yourself, your own knowledge as a statement. For example, notes naming work item 419 about dropped callbacks give the gateway deduction a second root under `links`, and it meets its threshold. Anything in the file with nothing checkable behind it is listed back to you and never stored with an invented citation.

`TestDocumentFindings` in `internal/apitest` runs this example through the client, with a stand-in commit SHA, followed by the local-notes call and the refused `file://` citation.

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

## What is true now, what was true then

Every assertion and entity name keeps two clocks apart: when it applied (`valid_from`, `valid_to`, the half-open window `[valid_from, valid_to)`) and when Froodbase learned it (`recorded_at`). Give validity dates and `scope` in `contribute` only when the source says them; a date nobody gave stays unknown and the record is returned with `validity_unknown: true`. Scope has six keys, `environment`, `region`, `version_min`, `version_max`, `cluster` and `tenant`; a key left out matches anything, names compare without case, and versions (such as `2.3` or `v2.4.1`) compare numerically, so give both bounds for one version. A version that is not one, or a window that ends before it starts, is refused.

`search` asks one of three questions:

- `current` (the default): records in force (active or disputed) whose validity includes now. Superseded, retracted and ended records are left out.
- `as_of` with `as_of`: records whose validity includes that time, whenever they were recorded, so an investigation imported today about last month is found. Superseded records are kept; retracted ones are not.
- `historical`: everything.

In every mode each result carries `status` relative to now (`current`, `historical` or `upcoming`) and a current claim not checked (`verified_at`, or `recorded_at` if never verified) within its area's freshness policy carries `stale: true`; it is still returned. The service default policy is 90 days for observations and inferences and no limit for decisions, stored in the `freshness_policy` table; an area's own row (written with `store.PutFreshnessPolicy` until curator tools arrive) replaces it. An entity found by name is filtered and labelled by that name's validity, so a former name is found only in `as_of` or `historical`. The rules are SQL functions (migration 7) applied inside each query after row-level security and before the limit, and checked against the same case tables as the Go rules (`internal/domain/domaintest`). Supersession links arrive with slice 4; until then the `superseded` state alone is the signal.

```bash
curl -s -u bob:froodbase-dev -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search","arguments":{"query":"callback","mode":"as_of","as_of":"2026-09-03T00:00:00Z","scope":{"environment":"prod"}}}}' \
  http://127.0.0.1:8080/mcp
```

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
| 7 | `00007_temporal_queries.sql` | The temporal rules as SQL functions (`froodbase_scope_overlaps`, `froodbase_valid_at`, `froodbase_temporal_status`, `froodbase_in_mode`, `froodbase_stale`, `froodbase_freshness_policy`), the append-only `freshness_policy` table with the service default and row-level security, and a validity index on `entity_name` |

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

Slice 2 adds `review_sweep`, a periodic job like the heartbeat: the leader inserts one at start and then every `FROODBASE_REVIEW_SWEEP_INTERVAL` (default one hour), unique per slot of that interval. It sweeps every area in turn, each with a caller holding that area alone, and marks beside each assertion whether it is due for review (in force and past its `review_due_at`) and, in `stale_at`, when a sweep first found it stale under the area's freshness policy (`freshness_policy`, the same rule as the live `stale` flag). It writes only `review_flag` and `review_sweep` (migration `00008_review_sweep.sql`), never a record, revision or support event, and only rows whose flags changed; `make logs` shows a `review_sweep` line per run and one per area with how many records are due and stale. `get_record` shows the marks as `due_for_review`, `stale_at` and `review_swept_at`. Run a sweep now, or inspect the marks:

```bash
docker compose run --rm --no-deps dreamer review-sweep
docker compose exec -T postgres psql -U froodbase -d froodbase -c "select area_id, swept_at, records, due_count, stale_count from review_sweep"
```

No tool sets a review date or an area's freshness policy yet (slice 3's review tools will); to try the sweep locally, set them in `psql` as the owner, for example `update assertion set review_due_at = now() - interval '1 day' where id = '<id>'`, then run a sweep and read the record with `get_record`.

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
| `FROODBASE_REVIEW_SWEEP_INTERVAL` | `1h` | How often `froodbase-dreamer` runs the review sweep, as a Go duration (`30m`, `6h`) from `1m` to `24h`; it also runs once at start. Compose passes it through from `.env` |
| `FROODBASE_DATABASE_URL` | none, required | PostgreSQL connection URL (`postgres://user:password@host:port/db?sslmode=disable`) read by `froodbase-api` and `froodbase-dreamer` (as `froodbase_app`) and by `froodbase-api migrate` (as the owner); its value is never logged |
| `FROODBASE_USERS_FILE` | none, required by `froodbase-api` | Path of the static users JSON file (Compose mounts `local/users.json` at `/etc/froodbase/users.json`) |

Read by `froodbase-mcp` on the host, normally set in the Claude Code registration:

| Variable | Default | Meaning |
|---|---|---|
| `FROODBASE_API_URL` | none, required | The API's MCP endpoint, `http://127.0.0.1:8080/mcp` for the Compose stack |
| `FROODBASE_USERNAME` | none, required | A username from the users file |
| `FROODBASE_PASSWORD` | none, required | That user's password; never logged |
