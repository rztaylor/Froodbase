# Documentation Facts

- `README.md`: orientation, building and running.
- `docs/pitch/`: the original product pitch and assessments. Historical; do not edit except to fix links.
- `docs/planning/`: product and technical requirements, architecture design, implementation plan, explainer, reuse research. Living design documents; update when a decision changes them and record the decision in `docs/dev/decisions.md`.
- `docs/dev/decisions.md`: durable product and architecture decisions.
- `docs/dev/ops/`: development environment, validation, release governance.
- `docs/dev/specs/`: durable feature and API specs once the tool contract is implemented.
- `docs/dev/guides/`: contributor guides.
- `docs/dev/guides/sample-design-note.md`: a fictional document the development guide's worked example records findings from, cited at a commit; change it only together with that example and `TestDocumentFindings`.
- `docs/dev/plans/`: active implementation plans only, named after their ticket, deleted at closure.
- `docs/user/`: not yet needed.
- Style: British English, sentence-case headings, Mermaid for diagrams, requirement ids `PR-nn` and `TR-nn` where traceability matters. Keep diagrams parseable by Mermaid 11 (no semicolons in sequence messages).
