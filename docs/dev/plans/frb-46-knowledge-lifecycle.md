# FRB-46: knowledge lifecycle without approval states

Status: active documentation change. Acceptance criteria and progress live on FRB-46.

## Scope

Replace the proposed and accepted states, the acceptance policy and its sufficiency predicates with a support score that every claim earns from distinct evidence roots, verification checks and recorded use outcomes. Shrink the lifecycle to active, disputed, superseded and retracted. State the privacy rule as "reports are never an input" and remove the report-ingestion tool, local preview and source-copy retention from the design. Keep the two clocks, scope, evidence roots, disputes, append-only history and per-person permissions unchanged.

This ticket changes planning contracts, facts and affected board tickets, not runtime behaviour. The `report_revision` source kind in `internal/domain` is removed by a follow-up code ticket.

## Review and validation

1. Reconcile AGENTS.md, product and architecture facts, PR/TR requirements, architecture, explainer, implementation plan, local MVP scope and the two reuse notes.
2. Grep docs and facts for `proposed`, `accepted`, `acceptance policy`, `sufficiency`, `ingest_report`, `report_revision`, `source copy`.
3. Check requirement references and Markdown links; run `scripts/validate.sh`; read the diff for surviving old rules.
4. Align the board tickets listed on FRB-46.

Remove this plan when FRB-46 is closed; the durable rule remains in the requirements, architecture and decision log.
