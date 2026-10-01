#!/bin/sh
# Runs once, when the Postgres volume is empty (docker-entrypoint-initdb.d).
# Creates the separate database wuzapi keeps its sessions in. The wuzapi
# container repeats this check on start, so existing volumes get it too.
set -eu

WUZAPI_DB="${WUZAPI_DB_NAME:-wuzapi}"

if ! psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d postgres -tAc \
  "SELECT 1 FROM pg_database WHERE datname = '${WUZAPI_DB}'" | grep -q 1; then
  createdb -U "$POSTGRES_USER" -O "$POSTGRES_USER" "$WUZAPI_DB"
  echo "initdb: created database ${WUZAPI_DB}"
fi
