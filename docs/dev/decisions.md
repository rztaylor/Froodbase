# Decisions

Durable product and architecture decisions. Design-level decisions D1 to D14 are tabulated in `docs/planning/03-architecture-design.md` section 11; this log records decisions made after that document, and repo-level ones.

| Date | Decision | Why | Consequence |
|---|---|---|---|
| 2026-10-14 | Implementation language is Go | Team's chosen language | Graphiti and Hindsight are design sources only; no Python dependencies |
| 2026-10-14 | One module, three binaries: `froodbase-mcp` (local), `froodbase-api`, `froodbase-dreamer` | Shared domain rules; local ingestion preview; simpler client sign-in | Per-machine binary needs a release and update path |
| 2026-10-14 | Product renamed from "The Brain" to Froodbase; repository created at `~/src/Froodbase` | Product naming | All documents and binary names use Froodbase |
| 2026-10-14 | Foundation follows the shared default project foundation (facts, docs/dev, changelog, feature-branch workflow) | Consistency with other repositories | Policies live in `.agents/facts/` |
