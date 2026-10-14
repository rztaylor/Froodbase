# Froodbase: planning documents

Derived from [Froodbase product pitch](../pitch/Froodbase%20product%20pitch.md) (13 October 2026) and the two background assessments in [docs/pitch](../pitch/). All are drafts for scoping and architecture review; none is a delivery commitment. Froodbase is implemented in Go as one module with three binaries: `froodbase-mcp` on each computer, `froodbase-api` and `froodbase-dreamer` in Azure.

| Document | Purpose | Read it if you |
|---|---|---|
| [01 Product requirements](01-product-requirements.md) | What Froodbase must do, for whom, and how success is judged. Numbered `PR-nn`. | own scope or need to agree what "done" means |
| [02 Technical requirements](02-technical-requirements.md) | Platform, protocol, data, temporal, retrieval, security and operational requirements. Numbered `TR-nn`, traced to `PR-nn`. | are building or reviewing the service |
| [03 Architecture design](03-architecture-design.md) | Context, containers, components, data model, request flows, decisions and deployment. | need to challenge or implement a design choice |
| [04 Implementation plan](04-implementation-plan.md) | Phases with entry and exit criteria, two decision gates, roles and risks. No dates. | are planning or staffing the work |
| [05 How Froodbase works](05-how-froodbase-works.md) | Plain-language explanation of relationships, provenance, change history, temporal correctness, retrieval, learning and reflection, with analogies and diagrams. | are new to these concepts and want to understand the mechanism without reading code |
| [06 Reuse research: Graphiti and Hindsight](06-reuse-research-graphiti-hindsight.md) | What each project does, pinned to a revision with sources; which designs Froodbase adapts, which it refuses, and why neither is a service dependency. | want the evidence behind the architecture's reuse decisions |

Suggested reading order for a newcomer: 05, then 01, then 03.
