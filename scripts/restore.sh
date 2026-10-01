#!/usr/bin/env bash
# Restore a backup made by scripts/backup.sh into a running production stack.
# DESTRUCTIVE: replaces the current databases (app + wuzapi), files, search
# indexes and wuzapi media. Parts missing from the backup are skipped.
#
#   NAME_PREFIX=olexfilms scripts/restore.sh /srv/backups/20260101-030000
set -euo pipefail

SRC="${1:?usage: scripts/restore.sh <backup directory>}"
PREFIX="${NAME_PREFIX:-app}"
PG="${PG_CONTAINER:-${PREFIX}-postgres}"
SEAWEED="${SEAWEED_CONTAINER:-${PREFIX}-seaweedfs}"
SEAWEED_VOLUME="${SEAWEEDFS_VOLUME_NAME:-${PREFIX}-prod-seaweedfs}"
MEILI="${MEILI_CONTAINER:-${PREFIX}-meilisearch}"
MEILI_VOLUME="${MEILI_VOLUME_NAME:-${PREFIX}-prod-meilisearch}"
WUZAPI="${WUZAPI_CONTAINER:-${PREFIX}-wuzapi}"
WUZAPI_VOLUME="${WUZAPI_VOLUME_NAME:-${PREFIX}-prod-wuzapi}"
WUZAPI_DB="${WUZAPI_DB_NAME:-wuzapi}"
# Containers that write to the data being replaced.
APP_CONTAINERS="${APP_CONTAINERS:-${PREFIX}-backend ${PREFIX}-worker-core ${PREFIX}-worker-docs}"
SRC_ABS="$(cd "$SRC" && pwd)"

if command -v sha256sum >/dev/null; then ( cd "$SRC" && sha256sum -c SHA256SUMS ); else ( cd "$SRC" && shasum -a 256 -c SHA256SUMS ); fi
# RESTORE_CONFIRM=<prefix> answers the prompt (automation, scripts/dr-drill.sh).
answer="${RESTORE_CONFIRM:-}"
if [ -z "$answer" ]; then
  read -r -p "This replaces the databases and files of '${PREFIX}'. Type the prefix to continue: " answer
fi
[ "$answer" = "$PREFIX" ] || { echo "aborted"; exit 1; }

echo "stopping API, workers and wuzapi"
# shellcheck disable=SC2086 # word-split the container list
docker stop $APP_CONTAINERS >/dev/null 2>&1 || true
if [ -f "${SRC}/wuzapi.dump" ] || [ -f "${SRC}/wuzapi.tar.gz" ]; then
  docker stop "$WUZAPI" >/dev/null 2>&1 || true
fi

echo "restoring database"
# One transaction: a failed restore leaves the database as it was, never half
# replaced.
docker exec -i "$PG" sh -c 'pg_restore -U "${POSTGRES_USER:-postgres}" -d "$POSTGRES_DB" --clean --if-exists --no-owner --single-transaction --exit-on-error' < "${SRC}/db.dump"

if [ -f "${SRC}/wuzapi.dump" ]; then
  echo "restoring wuzapi database (${WUZAPI_DB})"
  # Fails harmlessly when the database already exists.
  docker exec -e WUZAPI_DB="$WUZAPI_DB" "$PG" sh -c 'createdb -U "${POSTGRES_USER:-postgres}" "$WUZAPI_DB" 2>/dev/null' || true
  docker exec -i -e WUZAPI_DB="$WUZAPI_DB" "$PG" sh -c 'pg_restore -U "${POSTGRES_USER:-postgres}" -d "$WUZAPI_DB" --clean --if-exists --no-owner --single-transaction --exit-on-error' < "${SRC}/wuzapi.dump"
fi

# volume_restore <container> <volume> <archive>
volume_restore() {
  local container="$1" volume="$2" archive="$3"
  [ -f "${SRC}/${archive}" ] || { echo "skip ${archive} (not in backup)"; return 0; }
  echo "restoring ${volume}"
  docker stop "$container" >/dev/null 2>&1 || true
  docker run --rm -v "${volume}:/data" -v "${SRC_ABS}:/backup:ro" alpine:3.22 \
    sh -c "find /data -mindepth 1 -delete && tar xzf /backup/${archive} -C /data"
  docker start "$container" >/dev/null
}

volume_restore "$SEAWEED" "$SEAWEED_VOLUME" seaweedfs.tar.gz
volume_restore "$MEILI" "$MEILI_VOLUME" meili.tar.gz
volume_restore "$WUZAPI" "$WUZAPI_VOLUME" wuzapi.tar.gz
if [ -f "${SRC}/wuzapi.dump" ] && [ ! -f "${SRC}/wuzapi.tar.gz" ]; then
  docker start "$WUZAPI" >/dev/null 2>&1 || true
fi

echo "starting API and workers"
# shellcheck disable=SC2086 # word-split the container list
docker start $APP_CONTAINERS >/dev/null 2>&1 || echo "start the API and workers again (docker compose up -d)"
echo "restore done; check the app. Without meili.tar.gz, rebuild search: make search-reindex"
