# Froodbase: product requirements

Status: draft for scoping review. Derived from the [Froodbase product pitch](../pitch/Froodbase%20product%20pitch.md), the background assessments and the [OpenBrain reuse assessment](08-reuse-research-openbrain.md). Requirements are numbered `PR-nn` so that the [technical requirements](02-technical-requirements.md) and [implementation plan](04-implementation-plan.md) can trace to them.

Date: 13 October 2026. Revised 16 October 2026 for living-source extraction, continuous review and OpenBrain findings; revised the same day to replace approval states with earned support and to state that reports are never an input.

## 1. Purpose

Froodbase is a shared, remote knowledge service that helps people and agents understand our systems, learn from investigations, and distinguish what is true now from what was true before. It is accessed through MCP, backed by managed PostgreSQL in Azure, and governed by existing Entra identities.

This document states what the product must do and for whom. It deliberately avoids implementation detail; that lives in the technical requirements and architecture design.

## 2. Problem statement

Investigations run by the DevOps Ticket, Support and Architecture Change agents, and by the skills they call, generate valuable knowledge about dependencies, ownership, behaviour, failure modes and remedies. That knowledge currently lives in three loosely connected places: the shipped architecture references packaged with `pxg-core`, configured external knowledge folders, and per-ticket investigation workspaces.

The consequences, observed directly in the repository assessment:

- Authority varies by source and no rule says which wins when they disagree.
- Useful findings are not reliably rediscovered; each investigation rebuilds context from scratch.
- Corrections have no common lifecycle, so an old incident keeps looking like a live problem and a proposed change can be mistaken for deployed behaviour.
- Identical names across environments hide material differences.
- The same claim repeated in a report, a summary and an architecture page looks like three confirmations.

## 3. Goals and non-goals

### Goals

| ID | Goal |
|---|---|
| G1 | A user or agent can obtain an answer supported by accessible evidence. |
| G2 | The answer makes clear whether it is current, historical, disputed or thinly supported. |
| G3 | Lessons from past investigations are reusable without confusing a resolved incident with an active defect. |
| G4 | Corrections and supersessions propagate to retrieval and to any derived summaries. |
| G5 | Concurrent contributions never silently overwrite one another. |
| G6 | Private investigation material stays private: reports, conversations and transcripts never enter Froodbase. |
| G7 | The existing investigation skills can retrieve from and contribute to Froodbase while still producing local reports. |
| G8 | Knowledge progresses from experience to well-supported fact through evidence and use, without a person approving each claim. |

### Non-goals (initial direction)

- A replacement ticketing system; Jira remains authoritative for ticket fields.
- A report editor, report-authoring platform or report store.
- A general-purpose wiki or a general-purpose agent memory platform.
- Ingestion of conversations, chat transcripts or whole documents.
- An autonomous agent that turns every conclusion into fact: support is earned from evidence and outcomes, never from fluency or repetition.
- A separate graph database or vector service unless a pilot demonstrates the need.

## 4. Users and roles

| Role | Who | Primary needs |
|---|---|---|
| Investigating agent | Copilot agents and skills in this toolkit (Support, DevOps Ticket, Architecture Change, system-map-discovery, triage skills) acting on behalf of a signed-in person | Retrieve relevant knowledge with evidence and temporal context; contribute individual findings with shareable evidence as the work proceeds; report what happened when a retrieved record was used |
| Engineer or support analyst | A person running an investigation or asking a direct question | Same as above, plus the ability to read knowledge views as Markdown and inspect why an answer was given |
| Reviewer or curator | A person responsible for a knowledge area (for example a platform team lead) | Resolve disputes, retract or supersede records, flag doubtful claims, merge duplicates; see what needs attention. Reviewers correct; they do not approve |
| Knowledge-area owner | Team that owns a system or environment | Control who can read and contribute within their area; set the support thresholds that decide when an answer is offered without a warning |
| Service operator | Platform or DevOps team | Operate, back up, audit and recover the service; manage group-to-role mappings |

Agent identities are distinct from human identities. An agent acts for a person, and Froodbase records both.

## 5. Questions Froodbase must answer

These are the acceptance-level user stories. Each must be answerable with sources, scope and temporal context attached.

| ID | Question | Minimum answer content |
|---|---|---|
| Q1 | What depends on this service, and who owns it? | Relationship list with evidence, scope (environment, version) and validity, plus ownership with its source |
| Q2 | What was the architecture when this incident occurred? | Relationships and assertions valid at the incident time, labelled historical |
| Q3 | Have we seen this failure before, and what actually resolved it? | Matching lessons and incidents, with what was tried, what worked, and how well each remedy is supported |
| Q4 | Does that workaround still apply to this environment and version? | Lesson applicability limits compared with the requested scope; explicit "unknown" when the scope was never verified |
| Q5 | Which facts are disputed, thinly supported or due for review? | Filtered list by knowledge area with reasons |
| Q6 | What have several investigations taught us that should change a runbook or design? | Reflection output citing the contributing investigations, clearly marked as derived and showing its support |

## 6. Functional requirements

### 6.1 Knowledge model

| ID | Requirement |
|---|---|
| PR-01 | Froodbase holds **entities** with stable identities for systems, services, components, teams, environments and incidents. An entity's identity does not change when its name does. |
| PR-02 | Froodbase holds **relationships** between entities (dependency, ownership, deployment, containment and others added by curators) carrying evidence, applicable scope and validity. |
| PR-03 | Froodbase holds **assertions** of three distinguished kinds: observations (what was seen), decisions (what was approved or intended) and inferences (what was concluded). The kind is visible in every answer and never changes. |
| PR-04 | Observed implementation and approved design intent are never merged. A deployment observation does not repeal a decision; an approved design does not prove deployment matches it. |
| PR-05 | Froodbase holds **lessons** structured as symptoms, relevant conditions, unsuccessful approaches, successful remedies and limits of applicability. |
| PR-06 | Every entity, relationship, assertion and lesson is addressable by a stable identifier that can be cited from outside Froodbase. |
| PR-07 | Human-readable Markdown views remain available for explanations, lessons and knowledge summaries. Structured records, not prose, are the source of relationships and lifecycle state. |

### 6.2 Provenance, evidence and support

| ID | Requirement |
|---|---|
| PR-10 | Every assertion, relationship and lesson records where it came from (source URI and revision), who contributed it (person and, where applicable, agent) and when. |
| PR-11 | Evidence is held as references to sources that the intended audience can actually open. Living sources such as repositories and Confluence pages are identified by a stable location and source revision or snapshot; their documents are never copied into Froodbase beyond a short excerpt. A private local file path is not an acceptable shared citation. |
| PR-12 | A report, an observation extracted from it and a summary derived from both count as **one** line of evidence, not three. Froodbase tracks derivation so that corroboration is not inflated. |
| PR-13 | Derived knowledge (summaries, inferences, reflection output) is always marked as derived, cites its supporting records and has its support reassessed when any supporting record changes. |
| PR-14 | A later change to a living source does not silently rewrite a claim or its historical validity. When Froodbase detects that cited support has changed or can no longer be checked, it lowers the claim's support, makes that visible and flags it for review. |
| PR-15 | Every claim carries a **support** level, tracked separately from its kind and its lifecycle state. Support is an append-only history of events and a score computed from them. It rises with each distinct evidence root, each verification against fresh evidence on a known date, and each recorded **use outcome** in which a person or agent applied the claim and reports with evidence what was done and what was observed. Sustained use without any challenge adds a bounded amount that can never on its own carry an inference past its area's threshold. Support falls on contradiction, a failed outcome, a changed or vanished source, a reviewer's flag or the retraction of a supporting record. Being retrieved adds nothing. Copies, summaries and repeated mentions of one root add nothing. A well-supported inference remains an inference. |

### 6.3 Temporal correctness

| ID | Requirement |
|---|---|
| PR-20 | Each assertion and relationship distinguishes four things: when it applied (validity interval), when it was recorded, when it was last verified, and the scope (environment, version, region) it applies to. |
| PR-21 | Unknown dates stay unknown. Froodbase never infers a validity date from the recording date. |
| PR-22 | An assertion with no recorded end date is "not known to have ended", not "verified current". Answers distinguish the two. |
| PR-23 | A current-state query returns the knowledge valid now, applying supersession and validity rules rather than text recency. |
| PR-24 | A historical query (as-of a time) returns what was valid then and labels it historical. |
| PR-25 | A troubleshooting query may return historical cases as analogues, clearly labelled as historical and never presented as current state. |
| PR-26 | Importing older material later records it as historical evidence and does not reopen a resolved issue. |
| PR-27 | The worked example in the pitch (failure reproduced on 2.3, fix merged, 2.4 deployed and verified, old investigation imported later) is a mandatory acceptance test. |

### 6.4 Conflict handling and change history

| ID | Requirement |
|---|---|
| PR-30 | Conflicts are classified as one of: change over time, different scope, genuine correction, or unresolved contradiction. The classification is visible. |
| PR-31 | When incompatible evidence overlaps in scope and time, Froodbase preserves the disagreement as a **dispute** and surfaces it. It does not silently choose. |
| PR-32 | Corrections, disputes, verifications, retractions and supersessions are appended as history. The previous state remains readable; nothing is silently rewritten. |
| PR-33 | Age alone neither invalidates a durable fact nor erases a lesson, and age alone never raises support. Staleness is expressed as "due for review", not as deletion. |
| PR-34 | Every change records who made it, when, on what base revision and why (a short rationale). |

### 6.5 Contribution lifecycle

| ID | Requirement |
|---|---|
| PR-40 | A contribution is **active** as soon as it is written, with the support its evidence gives it. The lifecycle states are active, disputed, superseded and retracted. A claim becomes disputed when contradicted (PR-31), superseded when a contributor or reviewer declares a correction with a reason, and retracted when its contributor or a reviewer withdraws it with a reason. Every transition is an append-only record of actor, time, base revision and rationale. There is no proposed or accepted state and no approval step. |
| PR-41 | Knowledge-area owners set, per kind of claim, the support threshold below which an answer carries the insufficient-evidence warning, and which contributor classes and source kinds may write to the area. A claim below the threshold is still stored, retrievable and able to earn support. |
| PR-42 | Fluent synthesis, repetition, a valid citation and model confidence never raise support by themselves. Reflection output and agent inferences enter active as inferences, labelled derived, with the support their roots give them, and earn further support exactly as any other claim does. |
| PR-43 | Contributions must contain only knowledge authorised for the intended audience, and must carry shareable evidence of their own; the investigation they came from is never the evidence. |
| PR-44 | Two contributors editing the same record concurrently cannot overwrite each other; the later writer is told the record changed and must rebase. |
| PR-45 | Retries of the same contribution do not create duplicates. |
| PR-46 | An authorised reviewer can inspect every claim in their knowledge areas at any time, with its source, contributor, support history, current state and complete decision history. They can flag a claim with a reason, which lowers its support and queues it, without changing its state; retract or supersede it; and resolve disputes. Mixed contributions are reviewed claim by claim. |

### 6.6 Contribution and privacy

| ID | Requirement |
|---|---|
| PR-50 | A report, conversation or transcript is never uploaded to Froodbase, summarised into it, embedded or stored as evidence, whether automatically or on request. Froodbase holds extracted claims with citations to shareable sources and, at most, a short excerpt from each source. |
| PR-51 | People and agents contribute individual findings, each with its own shareable evidence, as an investigation proceeds or at its end. The investigation report records the identifiers of what was contributed so the contributor can inspect or retract it. What an agent may contribute is governed by its configuration and the person's permissions, not by a per-claim approval step. |
| PR-52 | Extracting knowledge from any document, including an old report, produces the same evidenced records through the same contribution path, distinguishing observations, decisions and inferences. The document itself is not sent and is never retained. |
| PR-53 | A Froodbase contribution does not fulfil the obligation to deliver a report, and a report does not require a Froodbase contribution. |
| PR-54 | Withdrawn on 16 October 2026: originals are never retained (PR-50). The evidence excerpt is the only text copied from a source, and it carries the source's access classification. |

### 6.7 Retrieval

| ID | Requirement |
|---|---|
| PR-60 | Retrieval combines exact identifier lookup, full-text search, semantic similarity and explicit relationship traversal in one query interface. |
| PR-61 | Every result carries its kind (observation, decision, inference, lesson), lifecycle state, support (score and the roots, verifications and outcomes behind it), scope, validity, verification date and citations. |
| PR-62 | Results never include records the caller is not permitted to read, including through summaries, counts, embeddings or relationship edges. |
| PR-63 | When the best candidates are below their area's support threshold, disputed or contradictory, the answer says so instead of ranking the most plausible text first. |
| PR-64 | The caller can request current, as-of, or historical-inclusive modes. |
| PR-65 | A caller can report the **outcome** of using a retrieved record: what was done with it, in what scope, and with evidence, what was observed afterwards. A successful outcome with evidence is a new observation that raises the record's support in that scope; a failed outcome lowers it and, where it contradicts the record, raises a dispute; "inapplicable" or "not used", with a reason, is recorded for evaluation and raises nothing. |

### 6.8 Learning and reflection

| ID | Requirement |
|---|---|
| PR-70 | Froodbase can consolidate related evidence across investigations into candidate lessons and observations, citing each contributing record. |
| PR-71 | Reflection can identify gaps: entities with no owner, relationships with no evidence, lessons never verified in a given scope, assertions past their review date, claims below threshold that are in regular use. |
| PR-72 | Reflection respects access boundaries: it never combines evidence a given reader could not see into a summary that reader can. |
| PR-73 | The split of reasoning between the calling agent and Froodbase is configurable per operation; Froodbase must function with model-free retrieval alone. |
| PR-74 | Model-assisted assessment of whether a derived claim's cited evidence exists, is relevant and sufficiently supports it is a review aid: it prioritises what reviewers look at and may raise a challenge for them. It never raises support. Only evidence roots, verifications and outcomes do. |

### 6.9 Access and privacy

| ID | Requirement |
|---|---|
| PR-80 | Access uses existing Entra identities and security groups mapped to reader, contributor and reviewer roles, optionally scoped to knowledge areas. |
| PR-81 | The actual caller's identity, not the service's, is enforced on every read and write and recorded as author. |
| PR-82 | Agent identities are recorded alongside the person they act for. |
| PR-83 | Access-restricted evidence remains restricted in every derived form: summaries, embeddings, exports, caches and reflection output. |

### 6.10 Migration and existing knowledge

| ID | Requirement |
|---|---|
| PR-90 | The shipped architecture references and system map can be imported with repository commit and path provenance, entering as sourced claims with the support their evidence gives them (not inflated). |
| PR-91 | Duplicate and overlapping records found during import are reconciled explicitly, with the reconciliation recorded. |
| PR-92 | Existing investigation skills are rewritten to retrieve from and contribute to Froodbase while continuing to produce local reports. Their versioned workflows demonstrate contributing, reusing, reporting outcomes and correcting sourced knowledge, including an earlier false positive. A Markdown-only fallback remains available while Froodbase is unavailable. |

## 7. Product-level quality requirements

| ID | Requirement |
|---|---|
| PR-100 | **Explainability.** For any answer, a user can see every authorised candidate that contributed and why it was included or excluded (validity, scope, supersession, support). Explanations do not reveal the existence or count of records the caller cannot read. An authorised reviewer can trace a claim back to its evidence and see which derived records depend on it, subject to the same access rules. |
| PR-101 | **Correctability.** Any record can be retracted, corrected or superseded by an authorised role in one operation, and the correction is visible in subsequent retrieval within a bounded, published delay. |
| PR-102 | **Availability of the fallback.** If Froodbase is unavailable, investigations continue using local and packaged knowledge; nothing in the workflow hard-fails. |
| PR-103 | **Auditability.** Every read of restricted material and every write is attributable and retained for a defined period. |
| PR-104 | **Operability.** The service is owned by a named team with backup, recovery, deletion and incident procedures documented before pilot users are onboarded. |

## 8. Success criteria and evaluation

Success is evaluated against a representative corpus, not vendor benchmarks. The corpus is built in the first phase of the implementation plan and must include:

- Conflicting records for the same system and time.
- Delayed imports of older investigations.
- Fixes deployed to only some environments.
- Repeated incidents with different root causes.
- Access-restricted evidence.
- Identical service names in different environments.
- Proposed versus approved versus deployed changes.
- Duplicated summaries derived from one observation.
- A real citation that is relevant but insufficient to support the stated deduction.
- An inference whose support grows through independent roots and a recorded successful use until it crosses its area's threshold with no human action; the same inference with only duplicated echoes stays below it; a failed outcome lowers it; a contradiction disputes it.
- A claim retrieved many times but never used with an outcome, whose support does not change.
- A living source that changed after a claim was extracted, and a claim later flagged by a reviewer.
- Questions Froodbase should refuse to answer for lack of evidence.

Roughly 30 to 50 adjudicated questions are the target. Measures:

| Measure | Target for pilot exit |
|---|---|
| Correct current/historical labelling | No current-state answer cites a superseded record as current |
| Citation accuracy | Every cited source opens and supports the claim |
| False entity merges | None across environment boundaries |
| Abstention | Questions whose best candidates are below threshold receive "insufficient evidence", not a confident answer |
| Correction propagation | A correction is reflected in retrieval and lowers the support of derived records within the published delay |
| Concurrent writes | No lost update across the conflict test set |
| Access | No restricted record leaks in any test, including via reflection |
| Reviewer oversight | An authorised reviewer can find any claim in their area, inspect its support and decision history, and flag, retract or supersede it |
| Support learning | Distinct roots, verifications and evidenced outcomes raise support; echoes, retrievals and model confidence do not; a failed outcome or contradiction lowers it; an inference stays labelled inference at every level |
| Reuse outcome | At least one later investigation records an outcome for a retrieved record, and that outcome changed the record's support |
| Cost and latency | Recorded per query mode; no target until measured |

No retrieval-quality or cost advantage has yet been demonstrated. The pilot exists to establish one.

## 9. Scope by phase

| Phase | Included | Excluded |
|---|---|---|
| Pilot | One or two knowledge areas; entities, relationships, assertions, lessons; provenance; temporal model; support and lifecycle with reviewer inspection of all claims; hybrid retrieval over PostgreSQL; agents contributing evidenced findings during investigations; use-outcome reporting; Entra roles; import of shipped architecture for the chosen areas; rewrite of one investigation agent | Reflection beyond simple consolidation; vector search unless the corpus shows full-text is insufficient; Hindsight, Graphiti or OB1 as dependencies |
| Expansion | Remaining knowledge areas and agents; reflection with gap analysis; review dashboards | Separate graph or vector services unless justified |
| Later | Optional Hindsight recall integration for support experience, if measured need | General wiki, report editing or storage |

## 10. Open questions for scoping

These map to the ten questions in the pitch. Each has a proposed owner and the phase in which it must be resolved.

| # | Question | Proposed resolution path | Resolve by |
|---|---|---|---|
| 1 | Initial users, domains and highest-value questions | Pick one support domain (candidate: Import Tool triage) and one architecture domain (candidate: TMF clusters) with their owning teams | Phase 0 |
| 2 | Canonical identities and source authority | Curators define entity identity rules per area; authority order: well-supported Froodbase record, then packaged architecture, then external map | Phase 0 |
| 3 | How certain is certain enough | Owners set per-area support thresholds per kind from the evaluation corpus; the service ships defaults; no approval step exists | Phase 2 |
| 4 | Temporal model and disputed/stale treatment | As specified in section 6.3 and 6.4; review intervals set per record type | Phase 1 |
| 5 | Entra mappings and agent identities | Three app roles, area scoping via group claims; agents use on-behalf-of flow | Phase 1 |
| 6 | Division of reasoning and model cost | Froodbase does retrieval and rule-based filtering; calling agent synthesises; reflection is a batch job with its own budget | Phase 0, revisit after pilot |
| 7 | Minimum retrieval and reflection capabilities | Determined by the evaluation corpus | Phase 0 and pilot exit |
| 8 | Migration of existing knowledge | Import with provenance; reconcile duplicates in the packaged master | Phase 3 |
| 9 | Concurrency, audit, deletion, recovery, ownership | Specified in technical requirements; operating team named before pilot | Phase 1 |
| 10 | Adopt a prebuilt component intact | Evaluate Basic Memory Cloud against the pilot acceptance tests before building retrieval | Phase 0 |

## 11. Assumptions and risks

Assumptions:

- An Azure subscription, Entra tenant and the ability to create app registrations and a Flexible Server are available to the owning team.
- The investigation agents can be modified; they are in this repository.
- Curators will handle disputes, flags and corrections and set thresholds. They approve nothing.

Risks:

| Risk | Mitigation |
|---|---|
| Fluent generated content becomes de facto authority | Support comes only from roots, verifications and evidenced outcomes; generated claims start with the support of their roots and stay labelled derived and inferred (PR-13, PR-42) |
| A wrong claim strengthens through use | Outcomes must carry evidence; use without an outcome is bounded and cannot carry an inference past the threshold; a failed outcome or contradiction lowers support and raises a dispute (PR-15, PR-65) |
| Entity merge errors across environments | Identity rules per area; no fuzzy merge without curator approval (PR-01, success criteria) |
| Private material leaks through contribution | Reports are never an input; every claim needs shareable evidence of its own; the report lists what was contributed so it can be retracted (PR-50 to PR-52) |
| Operating cost of model calls | Model-free baseline must work; reflection budgeted separately (PR-73) |
| No one curates | Disputes, flags and due-for-review reporting make the backlog visible; knowledge-area owners are named before go-live; knowledge still flows while the queue waits |
| Pilot shows no advantage over improved Markdown search | Markdown-only fallback is retained throughout; the go/no-go gate is explicit in the plan |
