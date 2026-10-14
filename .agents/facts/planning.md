# Planning Facts

- Board: Flashheart, project Froodbase, ticket key `FRB`. The backlog lives there; `docs/planning/04-implementation-plan.md` is the long-range plan and `docs/planning/07-local-mvp.md` the approved scope being built now.
- Implementation plans: `docs/dev/plans/<ticket-id>-short-kebab-title.md`, created for changes that touch more than one internal package, change the schema, or alter a product rule; deleted when the ticket closes.
- Decision log: `docs/dev/decisions.md`.
- Direction: the Direction section of `.agents/facts/product.md`.
- Ticket conventions: MVP tickets belong to the workstream of their slice (`slice-0` to `slice-5` tags); later work is tagged with its implementation-plan phase (`phase-1` to `phase-6`). A ticket that changes a safety rule or a `tools:` grant in the calling agents is a control change and needs explicit human review.
- Machines without the board (CI, other checkouts): skip board steps and say so in the PR description.
