#!/bin/sh
# Runs once, when the PostgreSQL volume is first initialised (the image's
# docker-entrypoint-initdb.d hook). Creates the service role the api and
# dreamer connect as: it owns nothing, so row-level security applies to it
# (internal/store/migrations/00002_records.sql grants it what it needs).
# The migrations run as the owning role ${POSTGRES_USER}.
set -eu
: "${FROODBASE_APP_DB_PASSWORD:?set FROODBASE_APP_DB_PASSWORD in .env (make up generates one)}"
psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v app_password="$FROODBASE_APP_DB_PASSWORD" <<'SQL'
CREATE ROLE froodbase_app LOGIN PASSWORD :'app_password';
SQL
