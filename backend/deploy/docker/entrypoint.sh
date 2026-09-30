#!/bin/sh
set -eu

ROLE="${APP_ROLE:-server}"

if [ "${ROLE}" = "server" ] && [ "${AUTO_MIGRATE:-true}" = "true" ]; then
  : "${DB_HOST:?DB_HOST is required for migrate}"
  : "${DB_PORT:?DB_PORT is required for migrate}"
  : "${DB_USER:?DB_USER is required for migrate}"
  : "${DB_PASSWORD:?DB_PASSWORD is required for migrate}"
  : "${DB_NAME:?DB_NAME is required for migrate}"
  DB_SSLMODE="${DB_SSLMODE:-disable}"

  DB_URL="postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=${DB_SSLMODE}"

  echo "migrate: applying migrations..."
  migrate -path /app/migrations -database "${DB_URL}" up
  echo "migrate: done"
fi

case "${ROLE}" in
  worker)
    exec /app/worker
    ;;
  migrate)
    : "${DATABASE_URL:?DATABASE_URL is required}"
    # If the DB is in a dirty state, force-clear it before running up.
    DIRTY_VERSION=$(migrate -path /app/migrations -database "${DATABASE_URL}" version 2>&1 | grep -oE '^[0-9]+' || true)
    IS_DIRTY=$(migrate -path /app/migrations -database "${DATABASE_URL}" version 2>&1 | grep -c 'dirty' || true)
    if [ "${IS_DIRTY}" -gt 0 ] && [ -n "${DIRTY_VERSION}" ]; then
      echo "migrate: dirty version ${DIRTY_VERSION} detected, forcing..."
      migrate -path /app/migrations -database "${DATABASE_URL}" force "${DIRTY_VERSION}"
    fi
    exec migrate -path /app/migrations -database "${DATABASE_URL}" up
    ;;
  create-super-admin)
    exec /app/create-super-admin \
      -email "${SA_EMAIL:-admin@example.com}" \
      -password "${SA_PASSWORD:-Password1}" \
      -name "${SA_NAME:-Platform}" \
      -surname "${SA_SURNAME:-Admin}"
    ;;
  *)
    exec /app/server
    ;;
esac
