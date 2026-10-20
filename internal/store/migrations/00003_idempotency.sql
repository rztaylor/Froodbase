-- +goose Up
-- Idempotency keys for contributions (TR-15): a client retrying a contribute
-- call with the same key gets the stored result and nothing is written
-- twice. Keyed per human caller so one person's key never replays another's
-- call. Rows older than the retention window are swept later; no row is
-- ever updated.
CREATE TABLE idempotency_key (
  caller_user_id text NOT NULL,
  key            text NOT NULL CHECK (btrim(key) <> ''),
  tool           text NOT NULL,
  request_hash   text NOT NULL,
  response       jsonb NOT NULL,
  created_at     timestamptz NOT NULL,
  PRIMARY KEY (caller_user_id, key)
);
CREATE INDEX idempotency_key_created ON idempotency_key (created_at);

ALTER TABLE idempotency_key ENABLE ROW LEVEL SECURITY;
CREATE POLICY any_read ON idempotency_key FOR SELECT USING (true);
CREATE POLICY any_append ON idempotency_key FOR INSERT WITH CHECK (true);
GRANT SELECT, INSERT ON idempotency_key TO froodbase_app;
