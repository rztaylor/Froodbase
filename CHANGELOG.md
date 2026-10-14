# Changelog

All notable changes to Froodbase are recorded here. The format follows Keep a Changelog; versions follow SemVer once releases begin.

## Unreleased

### Added

- Repository foundation: Go module with the `froodbase-mcp`, `froodbase-api` and `froodbase-dreamer` entrypoints, shared `internal/version`, lint and validation gate, CI workflow, Dependabot, repo-local facts and developer docs.
- Planning documents moved from the AI.CloudToolkit reports folder and renamed to Froodbase.
- Local MVP scope (`docs/planning/07-local-mvp.md`): the brain built first in Docker Compose with basic auth, as six slices; decisions on Compose, basic auth behind the authorisation-context interface, goose and Testcontainers recorded.
