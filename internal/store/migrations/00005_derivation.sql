-- +goose Up
-- Derivation links (PR-13, docs/planning/03-architecture-design.md section
-- 5): a decision or inference names the assertions it was derived from, and
-- starts with the distinct evidence roots of those sources. A link is
-- written with the deduction and never changes, so the table is append-only
-- like revision and support_event. Both ends live in the link's area: the
-- composite foreign keys refuse a source in another area even for a caller
-- who holds both. Cycles cannot form across calls, because a link is only
-- ever written for a record created in the same transaction; within a call
-- the writer refuses them.

ALTER TABLE assertion ADD CONSTRAINT assertion_id_area UNIQUE (id, area_id);

CREATE TABLE derivation (
  derived_id  uuid NOT NULL,
  source_id   uuid NOT NULL,
  area_id     uuid NOT NULL REFERENCES knowledge_area (id),
  rationale   text NOT NULL DEFAULT '',
  recorded_at timestamptz NOT NULL,
  PRIMARY KEY (derived_id, source_id),
  FOREIGN KEY (derived_id, area_id) REFERENCES assertion (id, area_id),
  FOREIGN KEY (source_id, area_id) REFERENCES assertion (id, area_id),
  CHECK (derived_id <> source_id)
);
CREATE INDEX derivation_source ON derivation (source_id);

ALTER TABLE derivation ENABLE ROW LEVEL SECURITY;
CREATE POLICY area_read ON derivation FOR SELECT USING (area_id = ANY (app_caller_areas()));
CREATE POLICY area_append ON derivation FOR INSERT WITH CHECK (area_id = ANY (app_caller_areas()));

GRANT SELECT, INSERT ON derivation TO froodbase_app;
