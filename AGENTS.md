# Agent Instructions

Froodbase is a shared, remote knowledge service for people and agents: entities, relationships, evidenced assertions and lessons, with provenance, append-only history and bitemporal validity. It is written in Go as one module with three binaries: `froodbase-mcp` (local, per computer), `froodbase-api` and `froodbase-dreamer` (Azure).

Read these before changing anything durable:

1. `.agents/facts/*.md` for repo-local policy (product, architecture, docs, planning, testing, release, git, go).
2. `docs/planning/` for what is being built and why. The architecture design there is the reference for package boundaries until code catches up.
3. The `doc.go` of any package you touch, and of its adjacent owners.

Rules that outrank convenience:

- Every hand-written Go package has a `doc.go` boundary comment. Create it with the package; update it when ownership moves.
- `main` packages stay small: flags, wiring, exit codes. Behaviour lives in `internal/`.
- Validate with `scripts/validate.sh` (vet, lint, race tests, build) before handing back.
- Work on feature branches and pull requests; never commit to `main` directly.
- Record durable decisions in `docs/dev/decisions.md` and user-visible or release-impacting changes in `CHANGELOG.md` under `Unreleased`.
- Ticket work is tracked on the Flashheart board (see `.agents/facts/planning.md`); do not keep task lists in the repo.
- Product rules that the code must never violate are in `docs/planning/01-product-requirements.md`: validity is never inferred from recording time, contradictions become disputes rather than automatic invalidation, generated claims begin proposed and may be accepted only by a reviewer or an explicit, auditable knowledge-area policy grounded in sufficient independent evidence, corroboration counts distinct evidence roots, and permissions are enforced per human caller before retrieval.
- Never put secrets in code, config, tests or logs. Local configuration uses environment variables documented in `docs/dev/ops/development.md`.
