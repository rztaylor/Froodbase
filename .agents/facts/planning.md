# Planning Facts

- Board: Flashheart. Decision needed: register the Froodbase project on the board and choose its ticket key (proposed `FRB`). Until then, the phase tables in `docs/planning/04-implementation-plan.md` are the only backlog.
- Implementation plans: `docs/dev/plans/<ticket-id>-short-kebab-title.md`, created for changes that touch more than one internal package, change the schema, or alter a product rule; deleted when the ticket closes.
- Decision log: `docs/dev/decisions.md`.
- Direction: the Direction section of `.agents/facts/product.md`.
- Ticket conventions: tag tickets with the implementation-plan phase (`phase-0` to `phase-6`); a ticket that changes a safety rule or a `tools:` grant in the calling agents is a control change and needs explicit human review.
- Machines without the board (CI, other checkouts): skip board steps and say so in the PR description.
