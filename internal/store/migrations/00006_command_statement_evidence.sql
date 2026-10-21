-- +goose Up
-- Command observations and personal statements as evidence (PR-11, TR-24,
-- TR-61). A command observation cites what a command or query observed, the
-- command (its locator) and the time it ran (its revision); a statement cites
-- person://<user id>, what was said (its excerpt) and when (its revision).
-- domain.Evidence.Validate holds the URI patterns; this mirrors the kinds.

ALTER TABLE evidence DROP CONSTRAINT evidence_source_kind_check;
ALTER TABLE evidence ADD CONSTRAINT evidence_source_kind_check
  CHECK (source_kind IN ('repository', 'ticket', 'telemetry_query', 'document', 'command', 'statement'));

-- One row per (uri, revision, locator) in an area, as before, except that
-- two different things one person said at the same moment are two
-- statements: a statement is also keyed on what was said.
DROP INDEX evidence_source;
CREATE UNIQUE INDEX evidence_source ON evidence (area_id, source_uri, source_revision, source_locator)
  WHERE source_kind <> 'statement';
CREATE UNIQUE INDEX evidence_statement ON evidence (area_id, source_uri, source_revision, md5(coalesce(excerpt, '')))
  WHERE source_kind = 'statement';

-- The source kind of a root_added event's root, which the area's support
-- configuration weighs. Backfilled for every existing root from its evidence.
ALTER TABLE support_event ADD COLUMN source_kind text
  CHECK (source_kind IS NULL OR source_kind IN ('repository', 'ticket', 'telemetry_query', 'document', 'command', 'statement'));
UPDATE support_event se SET source_kind = e.source_kind
FROM evidence e
WHERE se.event_type = 'root_added' AND e.id = se.evidence_root_ids[1];

-- Root weight per source kind (TR-43). Every observed kind keeps the weight
-- every root had (1); a statement weighs 0.5. Existing area versions get the
-- same values, so no stored score changes. The service default keeps its
-- version: no claim under it has a command or statement root yet.
ALTER TABLE support_config ADD COLUMN root_weight_by_source_kind jsonb NOT NULL
  DEFAULT '{"repository": 1, "ticket": 1, "telemetry_query": 1, "document": 1, "command": 1, "statement": 0.5}'::jsonb;
UPDATE support_config
SET allowed_contributors = jsonb_set(allowed_contributors, '{source_kinds}', '["repository", "ticket", "telemetry_query", "document", "command", "statement"]'::jsonb)
WHERE area_id IS NULL AND version = 'service-1';
