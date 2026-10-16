# FRB-45: claim evidence and acceptance policy

Status: active documentation change. Acceptance criteria and progress live on FRB-45.

## Scope

Replace the universal reviewer gate for generated inferences with explicit, versioned knowledge-area policy. Keep epistemic kind, evidential support and lifecycle disposition separate. Support changes and policy decisions must be attributable, append-only and visible to authorised reviewers. An inference accepted by policy remains an inference; conflict detection still creates disputes rather than silently invalidating knowledge.

This ticket changes planning contracts and affected implementation tickets, not runtime behaviour. The current local MVP has no claim acceptance implementation yet.

## Review and validation

1. Reconcile AGENTS.md, product facts, PR/TR requirements, architecture, explainer, phase plan and local MVP scope.
2. Ensure the schema, lifecycle, dreamer, retrieval and evaluation tickets use the same rule.
3. Review the interactive presentation for clear locations, responsibilities and evidence flow.
4. Check requirement references and Markdown links; run `scripts/validate.sh`; inspect the documentation diff for conflicting old rules.

Remove this plan when FRB-45 is closed; the durable rule remains in the requirements, architecture and decision log.
