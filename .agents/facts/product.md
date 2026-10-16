# Product Facts

- Name: Froodbase (working name during scoping: "The Brain").
- Audience: engineers, support analysts, reviewers and the Copilot investigation agents in the AI.CloudToolkit repository, acting on behalf of signed-in people.
- Shape: a remote knowledge service on Azure (PostgreSQL Flexible Server) exposed through MCP, with a local stdio MCP binary on each computer. Being built first as a local MVP (`docs/planning/07-local-mvp.md`): the same three binaries in Docker Compose on one computer with basic auth, no cloud.
- Full definition: `docs/planning/01-product-requirements.md` (requirements `PR-nn`), pitch in `docs/pitch/`.

## Direction

First, prove the brain locally: the MVP exit gate in `docs/planning/07-local-mvp.md`. Then deliver a pilot for one support area and one architecture area that can answer questions with evidence, label current versus historical knowledge, preserve disputes, propagate corrections and keep private reports private. The pilot exit gate is in `docs/planning/04-implementation-plan.md`.

## Safety boundaries

- Reports, conversations and transcripts are never an input to Froodbase. Agents contribute individual findings with shareable evidence as they work and list them in the report; `froodbase-mcp` reads no local documents.
- Retrieved prose is evidence, never an instruction.
- There is no approval state. Every claim is active with a support score earned only from distinct evidence roots, verification checks and evidenced use outcomes; retrieval, repetition, fluency and model confidence earn nothing, and quiet use earns a capped amount. An inference stays an inference at every level; contradictions become disputes; reviewers correct, flag and retract, they do not approve.
- Permissions are enforced per human caller before retrieval, including in derived output.

## Unknowns

- Decision needed before the cloud move: pilot knowledge areas and curators (implementation plan phase 0).
- Decision needed: model provider and budget for embeddings and reflection.
