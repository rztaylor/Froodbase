# Froodbase

## Initial product pitch

**A shared, remote knowledge service that helps people and agents understand our systems, learn from investigations, and distinguish what is true now from what was true before.**

Status: proposed product direction for scoping, planning and architecture. This is not an implementation specification or a commitment to a delivery schedule.

Date: 13 October 2026.

## The problem

Our agents and skills investigate incidents, answer support questions, review architecture and plan changes. They uncover useful knowledge about dependencies, ownership, behaviour, failure modes and remedies. Today, that knowledge is spread across shipped architecture documents, configured knowledge folders and individual ticket investigations.

The existing workflows contain strong investigation practices, but they do not provide a consistent shared memory. Authority varies between sources, useful findings are not reliably discovered again, and corrections do not have a common lifecycle. An old incident can continue to look like a current problem. A proposed change can be mistaken for deployed behaviour. Similar names can hide differences between environments.

The opportunity is to preserve and reuse what we learn without making users reconstruct that history on every investigation.

## The proposed product

Build a focused Froodbase service, accessible remotely through MCP, backed by managed PostgreSQL in Azure. People and agents should be able to retrieve relevant knowledge, inspect its evidence, contribute findings, and correct or supersede earlier understanding.

Froodbase should answer questions such as:

- What depends on this service, and who owns it?
- What was the architecture when this incident occurred?
- Have we seen this failure before, and what actually resolved it?
- Does that workaround still apply to this environment and version?
- Which facts are disputed, insufficiently verified or due for review?
- What have several investigations taught us that should change a runbook or design?

Answers should carry sources, scope and temporal context. When evidence is insufficient or contradictory, Froodbase should make that visible.

## Reports remain local

An investigation has two separate outputs:

| Output | Purpose | Default destination |
|---|---|---|
| Investigation report | Answer the user's particular questions, including context, evidence, conclusions and recommended actions | Local Markdown, under the user's control |
| Froodbase update | Preserve selected, shareable findings and lessons for future use | The shared Froodbase, within the contributor's permissions |

A report may contain personal context, sensitive details or material irrelevant to shared knowledge. Generating it must not automatically upload it or share its contents through a summary or embedding. Users can share reports through Teams or other existing channels independently.

Explicit report ingestion should be a Froodbase capability. It should identify the destination and material being shared, and distinguish evidence, hypotheses and proposed conclusions. Ingestion must not automatically promote everything in a report into accepted knowledge.

Routine Froodbase contributions should contain only knowledge authorised for the intended audience. Where the original report remains private, a contribution needs sufficient shareable evidence of its own; a private local file path is not a usable shared citation.

Report generation and management remain responsibilities of the investigation workflow. Froodbase does not need to become a report-authoring platform.

## Knowledge with structure, evidence and history

The core model should support:

- **Entities:** stable identities for systems, services, components, teams, environments and incidents.
- **Relationships:** dependencies, ownership and other meaningful links, with evidence and applicable scope.
- **Assertions:** observations, accepted decisions and inferred conclusions, clearly distinguished from one another.
- **Lessons:** symptoms, relevant conditions, unsuccessful approaches, successful remedies and limits of applicability.
- **Provenance:** where knowledge came from, who contributed it, and which source revision supports it.
- **Change history:** corrections, disputes, verification and supersession without silently rewriting the past.

Human-readable Markdown should remain available for explanations, lessons and knowledge views. Structured records should make the relationships and lifecycle explicit rather than requiring an agent to infer them repeatedly from prose.

Observed implementation and approved design intent must remain distinguishable. A deployment observation does not automatically repeal a decision, and an approved design does not prove that deployment matches it.

## Temporal correctness is foundational

“Newer wins” is not an adequate policy. Newly ingested material can describe old events; recent speculation should not override verified evidence.

Froodbase should distinguish when a statement applied, when it was recorded, when it was verified, and the environment or version to which it applies. Unknown dates should remain unknown. An assertion with no recorded end date is not necessarily verified indefinitely.

For example:

| Event | Correct interpretation |
|---|---|
| A failure is reproduced in production on version 2.3 | Evidence that the issue occurred in that scope |
| A fix is merged | Implementation changed; deployment and resolution remain unverified |
| Version 2.4 is deployed and remediation verified | Evidence of resolution for that deployment |
| An older investigation is imported later | Historical evidence; it does not reopen the issue |

A current query should present the verified resolution. A historical query should explain the earlier failure. A troubleshooting query may retrieve the old case as a useful analogue, clearly labelled as historical.

Conflict handling should distinguish changes over time, different scopes, genuine corrections and unresolved contradictions. When incompatible evidence overlaps in scope and time, Froodbase should preserve the disagreement rather than silently select an answer. Age alone should neither invalidate a durable fact nor erase a useful lesson.

## Retrieval, learning and reflection

Retrieval should combine exact identifiers, full-text search, semantic similarity and explicit relationships. Current-state questions should use validity and supersession rules, not merely rank newer text above older text. Historical and investigative searches should retain access to the relevant past.

Borrow proven ideas from Graphiti for temporal relationships and provenance, and from Hindsight for experience recall, consolidation and reflection. Reflection should connect evidence across investigations, suggest lessons and identify gaps in understanding.

Derived interpretations must remain distinguishable from their supporting evidence. They should cite sources, respect access boundaries and be reconsidered when those sources change. A report, an extracted observation and a summary derived from the same event are not three independent confirmations.

The acceptance policy for derived knowledge needs to be designed. It should support useful automation without allowing fluent synthesis to become authoritative merely because it was generated.

## Azure approach and reuse

The preferred starting point is a small remote MCP/API service with Azure Database for PostgreSQL Flexible Server. Existing Entra identities and security groups should provide access, with groups mapped to suitable Froodbase permissions and knowledge areas. The service would normally connect to PostgreSQL using managed identity while enforcing the actual caller's permissions and recording their authorship.

PostgreSQL provides the proposed shared storage foundation, with full-text search and optional pgvector for semantic retrieval. Explicit relationships can initially live in relational tables. A separate graph database or vector service should require a demonstrated benefit.

Reuse maintained components for database access, identity, MCP and search. Own the domain-specific rules for knowledge, evidence, temporal changes and contribution. This is a proposal to build a focused knowledge service, not to recreate a general-purpose memory platform.

Hindsight is a potential MIT-licensed source of reusable functionality or an optional service dependency. Graphiti offers relevant design ideas and Apache-2.0-licensed code. Any code reuse needs licence and dependency checks at the chosen revision; hosted product features must not be assumed to exist in the open-source package.

## Product boundaries and success

The initial direction does not require a replacement ticketing system, a report editor, a general wiki, unrestricted ingestion of conversations, or an autonomous agent that turns every conclusion into fact. Existing investigation skills will need to be rewritten to retrieve from and contribute to Froodbase while continuing to produce local reports.

Success means that a user can obtain an answer supported by accessible evidence, tell whether it is current or historical, see unresolved uncertainty, and reuse lessons without confusing a resolved incident with an active defect. Corrections should propagate to retrieval and derived summaries. Concurrent contributions should not silently overwrite one another. Private material should remain private unless deliberately shared.

These outcomes should be evaluated against representative investigations, including conflicting records, delayed imports, fixes deployed to only some environments, repeated incidents and access-restricted evidence. No retrieval-quality or cost advantage has yet been demonstrated through a pilot.

## Questions for scoping and architecture

The next phases should resolve:

1. The initial users, knowledge domains and highest-value questions.
2. Canonical identities, source authority and boundaries between teams or environments.
3. Which contributions can be accepted automatically, which require review, and who can correct or supersede them.
4. The temporal model, evidence requirements and treatment of disputed or stale knowledge.
5. Entra group mappings, agent identities and permissions for derived knowledge.
6. The division of reasoning between the calling agent and Froodbase, including model hosting and operating cost.
7. The minimum retrieval and reflection capabilities justified by an evaluation corpus.
8. Migration of shipped architecture and existing knowledge, including provenance and duplicate reconciliation.
9. Concurrency, audit, deletion, recovery and the operational ownership of the service.
10. Whether any prebuilt component satisfies enough of this contract to be adopted intact.

The intended outcome is a shared organisational memory that remains explainable and correctable as our systems change, while leaving each user's investigation output under their control.

## Background

The earlier assessments informed this pitch. Where they suggest shared storage of investigation reports by default, this pitch supersedes that suggestion.

- [Repository knowledge assessment](Froodbase%20knowledge%20architecture%20assessment.md)
- [Tool selection and build-versus-buy assessment](Froodbase%20tool%20selection%20and%20reports.md)
- [Graphiti](https://github.com/getzep/graphiti)
- [Hindsight](https://github.com/vectorize-io/hindsight)
- [Azure PostgreSQL Entra authentication](https://learn.microsoft.com/en-us/azure/postgresql/flexible-server/concepts-azure-ad-authentication)
- [Azure PostgreSQL pgvector support](https://learn.microsoft.com/azure/postgresql/extensions/how-to-use-pgvector)
