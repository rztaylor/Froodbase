# Froodbase: technical requirements

Status: draft for architecture review. Each requirement is numbered `TR-nn` and traces to the [product requirements](01-product-requirements.md) (`PR-nn`). The [architecture design](03-architecture-design.md) shows how these are satisfied.

Date: 13 October 2026. Revised 16 October 2026 for source extraction, continuous review and retrieval feedback.

## 1. Platform and hosting

| ID | Requirement | Traces to |
|---|---|---|
| TR-01 | The service is written in Go as one module producing three binaries: `froodbase-mcp` (local, per computer), `froodbase-api` and `froodbase-dreamer` (Azure Container Apps in the organisation's existing subscription and region). | PR-104 |
| TR-02 | Storage is Azure Database for PostgreSQL Flexible Server, version 16 or later, with the `pgvector`, `pg_trgm` and `btree_gist` extensions enabled. No other datastore is introduced in the pilot. | PR-60, non-goals |
| TR-03 | The service connects to PostgreSQL using its managed identity with Entra-only authentication. Password authentication is disabled on the server. | PR-81 |
| TR-04 | Infrastructure is defined in Terraform using the organisation's existing module conventions and is deployable to dev, staging and production environments. | PR-104 |
| TR-05 | Secrets (none expected beyond optional model API keys) live in Key Vault and are referenced, never embedded, in configuration. | safety rules |

## 2. Protocol and interface

| ID | Requirement | Traces to |
|---|---|---|
| TR-10 | The primary client interface is `froodbase-mcp`, a stdio MCP server on each computer built on the official MCP Go SDK, forwarding to `froodbase-api` over HTTPS. `froodbase-api` also exposes the same tools over Streamable HTTP for clients that support remote MCP. A thin REST surface for the same operations is acceptable for testing. | pitch |
| TR-11 | MCP authorisation follows the MCP authorisation specification with Entra ID as the OAuth 2.1 authorisation server. Access tokens carry the caller's object ID, group or role claims and, for agents, the acting-for identity via the on-behalf-of flow. | PR-80, PR-82 |
| TR-12 | The MCP tool surface is explicit and finite. No tool accepts arbitrary SQL, arbitrary filters compiled to SQL, or raw table access. | assessment |
| TR-13 | Minimum tool set for the pilot: `search`, `get_record`, `get_history`, `get_as_of`, `list_relationships`, `propose_contribution`, `review_contribution` (accept, dispute, reject, supersede), `verify_record`, `ingest_report`, `list_due_for_review`, `explain_answer`. | PR-60 to PR-64, PR-40, PR-51, PR-71, PR-100 |
| TR-14 | Every tool response includes lifecycle state, epistemic kind, current support assessment, scope, validity, verification date and citations for each record returned. An accepted inference is still labelled inference. A response never returns bare text without these fields. | PR-15, PR-61 |
| TR-15 | Tools are idempotent where a retry is possible: contributions and ingestions carry a client-supplied idempotency key held for at least 24 hours. | PR-45 |
| TR-16 | Writes require the base revision of any record they modify. A mismatch returns a conflict with the current revision; the server never merges silently. | PR-44 |
| TR-17 | Results are paginated and each page carries a stable cursor; the maximum page size is enforced server-side. | operability |

## 3. Data model

Full schema design is in the architecture document. These are the invariants the schema must satisfy.

| ID | Requirement | Traces to |
|---|---|---|
| TR-20 | Entities have an immutable surrogate identifier, a type, one or more names with validity intervals, and a knowledge-area assignment. Names are not identifiers. | PR-01, PR-06 |
| TR-21 | Relationships are first-class records between two entities with a type, scope, validity, lifecycle state and evidence links. They are not derived from text at query time. | PR-02, PR-07 |
| TR-22 | Assertions carry an epistemic kind (`observation`, `decision`, `inference`), a subject entity, a Markdown body, a scope, and the four temporal fields in TR-30. | PR-03, PR-20 |
| TR-23 | Lessons are structured records with sections for symptoms, conditions, unsuccessful approaches, remedies, and applicability limits, each section optionally linked to evidence. | PR-05 |
| TR-24 | Evidence is a record with a source URI, source kind (repository, ticket, telemetry query, document, report revision), revision or snapshot identifier, precise locator or excerpt, observation time, and an access classification. An evidence record can support many claims without storing the original document. | PR-10, PR-11, PR-14 |
| TR-25 | Derivation links record that one record was derived from others. A corroboration count is computed over distinct evidence roots, not over records. | PR-12, PR-13 |
| TR-26 | Scope is a structured value (environment, region, version range, cluster, tenant) with explicit "unspecified" for each dimension. Two scopes are comparable for overlap. | PR-20, PR-30 |
| TR-27 | Knowledge areas partition records for authorisation. Every record belongs to exactly one area. Cross-area relationships are allowed and are visible only to callers who can read both ends. | PR-62, PR-80 |
| TR-28 | The normal ingestion result is extracted records with source references, not a copy of the original report or document. If the contributor explicitly elects to retain a source copy for a stated benefit, it is stored as an immutable revision with its own access classification and kept separate from extracted claims. | PR-51 to PR-54 |
| TR-29 | Each claim has a separately inspectable, area-scoped support assessment derived from linked evidence: root identity, source authority, relevance, scope and time fit, sufficiency and challenges. Adding independent support or losing checkable support appends an assessment event without changing epistemic kind or inferring a validity date. Repeated descendants of one root cannot raise corroboration. | PR-12, PR-14, PR-15, PR-74 |

## 4. Temporal model

| ID | Requirement | Traces to |
|---|---|---|
| TR-30 | Every assertion and relationship carries: `valid_from` and `valid_to` (nullable; when the fact applied), `recorded_at` (system time, never null), `verified_at` (nullable), and `review_due_at` (nullable). | PR-20 |
| TR-31 | A null `valid_to` means "no known end", not "current". Current-state queries treat records with null `valid_to` as current only if they are not superseded and not disputed, and flag those whose `verified_at` is older than the area's freshness policy. | PR-22 |
| TR-32 | Validity dates are never populated from `recorded_at`. Where a contributor cannot supply them, they remain null and the record is marked "validity unknown". | PR-21 |
| TR-33 | Supersession is a directed link from a newer record to the one it replaces, with a reason. Superseding sets `valid_to` on the old record only when the contributor asserts an end date; otherwise the old record is marked superseded with validity unchanged. | PR-32 |
| TR-34 | As-of queries are evaluated against validity time, not recording time: a record recorded later but valid earlier is included in an as-of query for that earlier time. | PR-24, PR-26 |
| TR-35 | Record history is append-only. Each change creates a new revision; previous revisions remain readable through `get_history`. Physical deletion exists only for the redaction path in TR-72. | PR-32, PR-34 |

## 5. Conflict and lifecycle rules

| ID | Requirement | Traces to |
|---|---|---|
| TR-40 | Lifecycle states: `proposed`, `accepted`, `verified`, `disputed`, `superseded`, `rejected`. Transitions are restricted to authorised people or versioned knowledge-area policies and recorded with human actor or policy identifier, time, base revision, evidence set and rationale. `verified` records a fresh check; it does not turn an inference into an observation. | PR-40, PR-34 |
| TR-41 | On write, the service detects existing accepted or verified records with the same subject, type and overlapping scope and validity. It then classifies: non-overlapping time (change over time), non-overlapping scope (different scope), contributor-declared correction (supersession), or otherwise (contradiction). | PR-30 |
| TR-42 | A contradiction creates a dispute record linking both sides. Both remain retrievable and are labelled disputed until a reviewer resolves the dispute. | PR-31 |
| TR-43 | Acceptance policy is versioned per knowledge area and claim class. It identifies allowed contributor and source classes, minimum independent evidence roots, required source authority, relevance, scope/time and sufficiency predicates, conflict exclusions, and whether qualifying claims are auto-accepted, verified or queued for review. The unmatched fallback is proposed. Inferences and reflection output may auto-accept only when the policy's independently checkable sufficiency predicates pass; model confidence, prose fluency, valid citations and repeated descendants of one root cannot satisfy them alone. Every evaluation records the policy version, input revisions, result and rationale. | PR-41, PR-42, PR-74 |
| TR-44 | When a supporting source or record changes, the service recomputes affected support assessments and policy eligibility, appends the result and marks derived records `needs_reconsideration` where support is lost or ambiguous. It does not silently change historical validity or resolve a dispute. Eligible proposed claims can gain support or be accepted through TR-43 without a human action. | PR-13, PR-15, PR-101 |
| TR-45 | An authorised reviewer can flag any claim, including one already accepted, for reconsideration with a reason. The flag is recorded in its history and appears in the review queue; its lifecycle state and validity do not change until an authorised review action. Review decisions address individual claims within a contribution. | PR-32, PR-46 |

## 6. Retrieval

| ID | Requirement | Traces to |
|---|---|---|
| TR-50 | `search` accepts a query string, optional entity identifiers, optional scope, a mode (`current`, `as_of` with timestamp, `historical`) and a kind filter. | PR-64 |
| TR-51 | Candidate generation runs four channels: exact identifier and name match (including trigram similarity for near-miss names), PostgreSQL full-text search over Markdown bodies, optional vector similarity over `pgvector` embeddings, and relationship expansion from matched entities to a configurable depth (default 1). | PR-60 |
| TR-52 | Authorisation filtering is applied to candidates before fusion, ranking, snippet generation or any model call. Embedding vectors and relationship edges are filtered by the same rule. | PR-62, PR-83 |
| TR-53 | Mode filtering is applied after authorisation: `current` excludes superseded and rejected records and validity-expired records; `as_of` applies TR-34; `historical` includes everything but labels each record's status relative to now. | PR-23 to PR-25 |
| TR-54 | Ranking fuses channels (reciprocal rank fusion or equivalent). Recency of `recorded_at` is not a ranking signal in `current` mode; verification state and evidence corroboration are. | PR-23 |
| TR-55 | `explain_answer` returns, for a prior search, every authorised candidate that was considered and the rule that included or excluded it after authorisation. It does not reveal inaccessible candidate identities, contents or counts. | PR-100 |
| TR-56 | A search whose top candidates are all `proposed`, `disputed` or below the area's support threshold returns an explicit `insufficient_evidence` flag alongside the results. The threshold uses distinct roots and sufficiency, not mention counts or model confidence. | PR-15, PR-63 |
| TR-57 | Full-text and vector indexes are projections rebuildable from canonical tables by a documented job. Index staleness is observable per record. | assessment |
| TR-58 | Embedding generation, when enabled, runs asynchronously from a durable job queue in the same database (outbox pattern). A failed embedding leaves the record committed and searchable by full text. | assessment |
| TR-59 | Retrieval feedback links a request and returned record revision to the human caller, acting agent if any, and a used, ignored or inapplicable outcome with a reason. It is append-only and may inform evaluation or review selection, but cannot itself increase corroboration or change a claim's lifecycle state. | PR-65 |

## 7. Contribution and ingestion

| ID | Requirement | Traces to |
|---|---|---|
| TR-60 | `propose_contribution` accepts a batch of records (entities, relationships, assertions, lessons, evidence) as one atomic change set with a single idempotency key and a target knowledge area. Its result identifies each submitted claim, source reference, support and lifecycle state, and any area-policy decision so the contributor can inspect or correct it. | PR-40, PR-45, PR-46 |
| TR-61 | The service validates every evidence reference: the source kind is known, the URI matches an allowed pattern for shared sources, and the access classification is not more permissive than the source's. Local file paths are rejected. | PR-11, PR-43 |
| TR-62 | `ingest_report` is a two-step tool: `preview` runs inside `froodbase-mcp` on the user's machine and returns the destination area, the records that would be created and their classification (evidence, hypothesis, proposed conclusion), and whether a source copy is offered for retention. It sends only entity names and the area to `froodbase-api`; `commit` sends the approved change set and any separately approved source copy. The investigation workflow never calls `commit` without the user having seen the preview. | PR-50 to PR-54 |
| TR-63 | Ingested hypotheses and explanations are stored as `inference` assertions in `proposed` state linked to the contribution and its shareable evidence. Extraction alone cannot accept them; later independent evidence may strengthen or permit policy-governed acceptance under TR-43. A rejected explanation remains visibly rejected or challenged. These records link to a report revision if one was explicitly retained. | PR-42, PR-43, PR-52, PR-54 |
| TR-64 | Contribution records the human caller, the agent identity if any, the client application and the originating investigation identifier. | PR-82 |
| TR-65 | Importers record the upstream source identity and revision, precise locator, importer and extraction versions, and a fingerprint of extracted content. A digest of the input may be computed without retaining it. Where source material names an original speaker or author, record that attribution and its uncertainty separately from the authenticated contributor. Fingerprints identify reconciliation candidates, not independent evidence roots. | PR-10 to PR-12, PR-14 |
| TR-66 | When a later import or source check detects that a cited living source changed, disappeared or became inaccessible, record the observation and flag affected claims for review. Do not infer `valid_to`, verification or a new lifecycle state from source change alone. | PR-14, PR-21, PR-33 |

## 8. Authorisation and privacy

| ID | Requirement | Traces to |
|---|---|---|
| TR-70 | Three app roles: `Froodbase.Reader`, `Froodbase.Contributor`, `Froodbase.Reviewer`, assigned to Entra security groups. Area scoping uses a mapping table from group object ID to knowledge area and role. | PR-80 |
| TR-71 | Authorisation is enforced in the application layer on every operation and reinforced by PostgreSQL row-level security policies keyed on a session variable set per request. The service's database role is not a table owner and cannot bypass policies. | PR-81, PR-62 |
| TR-72 | Redaction: an administrator can physically remove an evidence excerpt or report revision body while retaining the record identifier and a redaction marker. Dependent derived records are flagged for reconsideration. This is the only physical delete. | PR-103, operability |
| TR-73 | Caches, exports and reflection outputs are keyed by the authorisation context they were produced for and are never served to a different context. | PR-83 |
| TR-74 | An audit event is written for every write, every state transition, every redaction and every read of a record with a restricted access classification. Audit events are retained for at least 400 days and exported to the organisation's log analytics workspace. | PR-103 |
| TR-75 | An area-scoped, authorised query path lists all contributions and their individual claims, regardless of lifecycle state, with source references, contributor and agent, extraction context, revisions, flags, reviewer decisions and reasons. Reviewers can filter and page through the history at any time. The interface may be an MCP tool, UI or controlled query; it must use the caller's permissions and not expose inaccessible claims through counts or joins. This knowledge history is distinct from the retention-limited operational audit events in TR-74. | PR-46, PR-62, PR-103 |
| TR-76 | Reviewers can traverse supporting evidence and dependent derived records from a claim with bounded depth and results. Permission filtering applies before traversal output, counts or truncation details reveal any node. | PR-62, PR-100 |

## 9. Reflection and model use

| ID | Requirement | Traces to |
|---|---|---|
| TR-80 | Reflection runs as a scheduled background job scoped to one knowledge area and one authorisation context at a time. | PR-72 |
| TR-81 | Reflection output is written as `inference` assertions or lesson candidates in `proposed` state with derivation links to every record consulted. | PR-42, PR-70 |
| TR-82 | Gap analysis (TR-13 `list_due_for_review`) is rule-based and needs no model: unowned entities, evidence-free relationships, lessons unverified in a requested scope, records past `review_due_at`, records marked `needs_reconsideration`, and claims explicitly flagged by a reviewer. | PR-46, PR-71, PR-73 |
| TR-83 | All model calls go through one adapter with a per-job token budget, a configured model identifier and full request logging (prompt hash, record identifiers, cost). The service must pass all pilot acceptance tests with model calls disabled, except the tests specific to semantic search and reflection synthesis. | PR-73 |
| TR-84 | Model hosting uses the organisation's approved provider. Prompts include only records the job's authorisation context can read. | PR-72 |
| TR-85 | Assessment of generated claims separately checks citation existence, topical relevance and evidential sufficiency. Structural checks are deterministic. Model assessments carry policy/prompt versions, source revisions and rationale but cannot alone satisfy an auto-acceptance predicate or resolve a dispute; only independently checkable TR-43 criteria can authorise an automatic transition. Ambiguous sufficiency is queued for human review. | PR-42, PR-74 |

## 10. Concurrency, durability and operations

| ID | Requirement | Traces to |
|---|---|---|
| TR-90 | Optimistic concurrency via per-record revision numbers (TR-16). Change sets are applied in one database transaction. | PR-44 |
| TR-91 | Point-in-time restore is enabled with at least 14 days retention; a documented restore drill is executed before pilot go-live. | PR-104 |
| TR-92 | Schema migrations are versioned, forward-only and run from the deployment pipeline, never from a developer workstation. | operability |
| TR-93 | Health, readiness, request latency per tool, job queue depth and index staleness are exported as metrics to the organisation's monitoring. Alerts exist for queue backlog and failed migrations. | PR-104 |
| TR-94 | Published service levels for the pilot: retrieval p95 under 2 seconds without model calls; correction visible in `current` search within 60 seconds; embedding backlog under 15 minutes. These are targets to measure, not commitments. | PR-101 |
| TR-95 | The service is stateless apart from the database; two instances can run concurrently behind the platform's load balancer. | operability |

## 11. Client integration

| ID | Requirement | Traces to |
|---|---|---|
| TR-100 | Investigation agents consume Froodbase as a configured MCP server. The agent's `tools:` grant names only Froodbase tools it needs; retrieval-only agents receive no write tools. | repository safety rules |
| TR-101 | Skills that retrieve from Froodbase treat returned prose as evidence. Nothing retrieved is interpreted as an instruction or as authorisation to execute a command. | assessment |
| TR-102 | When Froodbase is unreachable, the agent continues with packaged and local knowledge and records in its report that Froodbase was unavailable. | PR-102 |
| TR-103 | No skill or agent instructs local-state mutation on the user's machine as part of Froodbase integration. | repository safety rules |

## 12. Licensing and reuse

| ID | Requirement | Traces to |
|---|---|---|
| TR-110 | Any adopted code from Hindsight (MIT) or Graphiti (Apache-2.0) is pinned to a revision, recorded in a third-party notices file with its licence text, and reviewed for transitive dependencies before merge. | pitch |
| TR-111 | Hosted features of either project are not assumed to exist in the open-source package. Any dependency on a hosted service is a separate decision. | pitch |
| TR-112 | AGPL (Basic Memory engine) and BSL (Outline) components are evaluated as products, not as code sources. | assessment |

## 13. Evaluation harness

| ID | Requirement | Traces to |
|---|---|---|
| TR-120 | A repeatable evaluation harness loads a versioned corpus of synthetic and sanitised real investigations, runs the adjudicated question set, and reports the measures in section 8 of the product requirements. | PR success criteria |
| TR-121 | The harness runs in the deployment pipeline against a disposable database and blocks promotion on regressions in current/historical labelling, citation accuracy, access leakage and lost updates. | PR-101, PR-103 |
| TR-122 | Held-out questions and source revisions are maintained separately so that prompt or ranking changes are not tuned against the whole set. | assessment |

## 14. Traceability summary

| Product requirement group | Technical requirements |
|---|---|
| Knowledge model (PR-01 to PR-07) | TR-20 to TR-23, TR-27 |
| Provenance (PR-10 to PR-15) | TR-24, TR-25, TR-29, TR-44, TR-61, TR-65, TR-66 |
| Temporal (PR-20 to PR-27) | TR-30 to TR-35, TR-53 |
| Conflict and history (PR-30 to PR-34) | TR-40 to TR-42, TR-35 |
| Contribution (PR-40 to PR-46) | TR-15, TR-16, TR-43, TR-45, TR-60, TR-64, TR-75, TR-90 |
| Explicit ingestion (PR-50 to PR-54) | TR-28, TR-62 to TR-66 |
| Retrieval (PR-60 to PR-65) | TR-50 to TR-59, TR-76 |
| Reflection (PR-70 to PR-74) | TR-80 to TR-85 |
| Access (PR-80 to PR-83) | TR-11, TR-70 to TR-76 |
| Migration (PR-90 to PR-92) | TR-100 to TR-103, plan phases 3 and 4 |
| Quality (PR-100 to PR-104) | TR-55, TR-76, TR-91 to TR-95, TR-120 to TR-122 |
