#!/bin/sh
# Backend image entrypoint. APP_ROLE picks the process:
#   server    HTTP API (default; AUTO_MIGRATE=true migrates first)
#   worker    Asynq worker (WORKER_QUEUES, SCHEDULER_ENABLED)
#   migrate   one-shot golang-migrate up (compose.prod.yml "migrate")
#   migrator  one-shot legacy data import (F2; profile "migrator")
#   create-super-admin  one-shot seed (profile "seed")
#   roles-sync  one-shot RBAC catalog reconcile (ROLES_SYNC_ARGS, e.g. -dry-run)
# PDFs render in the Gotenberg container (GOTENBERG_URL); the image ships no
# Chromium.
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
  migrator)
    # Legacy-system import (design K26/K27). The binary arrives in F2; until
    # then the profile is a no-op that says so instead of starting the API.
    if [ -x /app/migrator ]; then
      exec /app/migrator
    fi
    echo "migrator: not implemented yet (F2) — nothing was imported" >&2
    exit 1
    ;;
  create-super-admin)
    exec /app/create-super-admin \
      -email "${SA_EMAIL:-admin@example.com}" \
      -password "${SA_PASSWORD:-Password1}" \
      -name "${SA_NAME:-Platform}" \
      -surname "${SA_SURNAME:-Admin}"
    ;;
  roles-sync)
    # shellcheck disable=SC2086
    exec /app/roles-sync ${ROLES_SYNC_ARGS:-}
    ;;
  *)
    exec /app/server
    ;;
esac
