# Release governance

Status: pre-alpha. This document is the policy; `.agents/facts/release.md` is the machine-readable summary.

## Versioning

SemVer. Tags `vMAJOR.MINOR.PATCH`. Until v1.0.0, minor versions may change the MCP tool contract; the changelog says so explicitly.

## Release candidates

A release candidate passes `scripts/validate.sh`, the integration suite, and the evaluation harness regression gate (current/historical labelling, citation accuracy, access leakage, lost updates) once those exist.

## Compatibility and support

- `froodbase-api` publishes a minimum compatible `froodbase-mcp` version; clients below it receive an update message rather than a protocol error.
- Database migrations are forward-only and run from the pipeline.
- Only the latest minor version is supported until otherwise stated.

## Changelog

`CHANGELOG.md`, curated, Keep a Changelog categories. Every user-facing, safety, compatibility, packaging or validation change gets an entry under `Unreleased` in the PR that makes it.

## Release blockers

- Any failing gate in the release-candidate validation.
- Any open dispute about a product rule in `docs/planning/01-product-requirements.md` sections 6.3 to 6.6.
- Unreviewed changes to authorisation or redaction code.

## Hosted workflow

Not yet created. See `.agents/facts/release.md` for the pending decisions.
