# Release Facts

- Maturity: pre-alpha; nothing is released. No tags exist.
- Versioning: SemVer once releases begin; tags `vMAJOR.MINOR.PATCH`. Pre-release tags `v0.y.z` carry no compatibility promise.
- Changelog: `CHANGELOG.md`, Keep a Changelog categories, curated entries, `Unreleased` section.
- Release governance: `docs/dev/ops/release-governance.md`.
- Default validation: `scripts/validate.sh`. Release-candidate validation: the same plus the integration and evaluation suites once they exist.
- External prerequisites for local validation: none. Skipped validation must be stated in the PR.
- Artifact targets (planned): `froodbase-mcp` for darwin/arm64, darwin/amd64, linux/amd64, windows/amd64 with SHA-256 checksums; `froodbase-api` and `froodbase-dreamer` as container images. Decision needed: signing and notarisation for the per-machine binary.
- Hosted release workflow: not yet created. Decision needed: GitHub Releases plus a container registry, triggered on tag push with manual approval.
- Rollback: redeploy the previous image tag; `froodbase-api` publishes the minimum compatible client version so older `froodbase-mcp` builds receive an update message.
