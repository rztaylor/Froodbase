# Decisions

Durable product and architecture decisions. Design-level decisions D1 to D14 are tabulated in `docs/planning/03-architecture-design.md` section 11; this log records decisions made after that document, and repo-level ones.

| Date | Decision | Why | Consequence |
|---|---|---|---|
| 2026-10-14 | Implementation language is Go | Team's chosen language | Graphiti and Hindsight are design sources only; no Python dependencies |
| 2026-10-14 | One module, three binaries: `froodbase-mcp` (local), `froodbase-api`, `froodbase-dreamer` | Shared domain rules; local ingestion preview; simpler client sign-in | Per-machine binary needs a release and update path |
| 2026-10-14 | Product renamed from "The Brain" to Froodbase; repository created at `~/src/Froodbase` | Product naming | All documents and binary names use Froodbase |
| 2026-10-14 | Foundation follows the shared default project foundation (facts, docs/dev, changelog, feature-branch workflow) | Consistency with other repositories | Policies live in `.agents/facts/` |
| 2026-10-14 | Build a local MVP of the brain before any cloud infrastructure; Gate 0 of the implementation plan is taken as build | Judge the knowledge model, temporal rules, disputes, retrieval and dreamer jobs on one computer before paying to operate them | Scope in `docs/planning/07-local-mvp.md`; implementation plan phases 1 and 2 are localised, phase 0 organisational work waits for the cloud move |
| 2026-10-14 | Local runtime is Docker Compose: PostgreSQL 16 with pgvector, pg_trgm and btree_gist, `froodbase-api` and `froodbase-dreamer` as containers; `froodbase-mcp` runs on the host as a stdio server in Claude Code | Same images as the cloud; the three-binary split is kept so the move is a hosting change | `froodbase-mcp` never connects to PostgreSQL; `froodbase-api` is the only writer |
| 2026-10-14 | Authentication for the MVP is HTTP basic auth against a static user list, behind the `internal/authz` authorisation-context interface | No Entra tenant locally; the rest of the service must only see (area, role) grants | Entra becomes a second implementation of the same interface; no basic-auth type may appear in the interface signature |
| 2026-10-14 | Migrations use goose, versioned and forward-only; locally run by a `make` target, in the cloud only from the pipeline | Widely used, SQL-first, fits sqlc | TR-92 unchanged for cloud environments |
| 2026-10-14 | Integration tests use Testcontainers for Go with a disposable PostgreSQL, tagged `integration` | The test gate must not depend on a running Compose stack | Docker is a prerequisite for the integration suite; the unit gate stays Docker-free |
| 2026-10-14 | Flashheart ticket key is `FRB` | Short and recognisable | Ticket ids are `FRB-n`; PR titles start with the id |
| 2026-10-14 | Repository hosted at `github.com/rztaylor/Froodbase`; module path kept lowercase `github.com/rztaylor/froodbase` | Owner's choice of home; Go module paths are conventionally lowercase and GitHub resolves the name case-insensitively | Foundation landed as the first commit on `main`; all later changes arrive by pull request |
| 2026-10-14 | Work is organised as vertical slices, each advancing `froodbase-api`, `froodbase-mcp` and `froodbase-dreamer` together | The owner wants to see all three improve in lockstep, not a dark dreamer until the end | One workstream per slice on the board; every slice ends runnable in Claude with its own integration tests |
