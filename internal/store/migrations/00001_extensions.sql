-- +goose Up
-- Extensions the schema relies on: pgvector for embeddings (used from slice
-- 6 onward), pg_trgm for near-miss name matching, btree_gist for validity
-- range indexes combined with lifecycle state. Migrations are forward-only;
-- there is no Down section by policy (docs/dev/decisions.md).
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS btree_gist;
