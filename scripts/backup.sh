#!/usr/bin/env bash
# Back up a production stack:
#   db.dump         app database (pg_dump custom format)
#   wuzapi.dump     wuzapi database (WhatsApp sessions, same Postgres)
#   seaweedfs.tar.gz  SeaweedFS data volume (contracts, invoices, PDFs, uploads)
#   meili.tar.gz    Meilisearch data volume (indexes; also rebuildable with
#                   make search-reindex)
#   wuzapi.tar.gz   wuzapi media volume
#
#   NAME_PREFIX=olexfilms BACKUP_DIR=/srv/backups scripts/backup.sh
#
# Keeps the newest $KEEP backups (default 14). Run it daily from cron on the
# Docker host and copy $BACKUP_DIR off the server (object storage, another host).
# SKIP_MEILI=1 / SKIP_WUZAPI=1 leave those parts out (dr-drill, partial stacks).
set -euo pipefail

PREFIX="${NAME_PREFIX:-app}"
DIR="${BACKUP_DIR:-./backups}"
KEEP="${KEEP:-14}"
STAMP="$(date +%Y%m%d-%H%M%S)"
PG="${PG_CONTAINER:-${PREFIX}-postgres}"
SEAWEED="${SEAWEED_CONTAINER:-${PREFIX}-seaweedfs}"
SEAWEED_VOLUME="${SEAWEEDFS_VOLUME_NAME:-${PREFIX}-prod-seaweedfs}"
MEILI="${MEILI_CONTAINER:-${PREFIX}-meilisearch}"
MEILI_VOLUME="${MEILI_VOLUME_NAME:-${PREFIX}-prod-meilisearch}"
WUZAPI="${WUZAPI_CONTAINER:-${PREFIX}-wuzapi}"
WUZAPI_VOLUME="${WUZAPI_VOLUME_NAME:-${PREFIX}-prod-wuzapi}"
WUZAPI_DB="${WUZAPI_DB_NAME:-wuzapi}"
SKIP_MEILI="${SKIP_MEILI:-0}"
SKIP_WUZAPI="${SKIP_WUZAPI:-0}"
OUT="${DIR}/${STAMP}"

mkdir -p "$OUT"
OUT_ABS="$(cd "$OUT" && pwd)"
FILES=(db.dump)

echo "database → ${OUT}/db.dump"
docker exec "$PG" sh -c 'pg_dump -U "${POSTGRES_USER:-postgres}" -d "$POSTGRES_DB" -Fc' > "${OUT}/db.dump"
# pg_dump writing nothing (wrong container, auth error in a pipe) must not
# pass as a backup.
[ -s "${OUT}/db.dump" ] || { echo "database dump is empty" >&2; exit 1; }

if [ "$SKIP_WUZAPI" != 1 ]; then
  echo "wuzapi database (${WUZAPI_DB}) → ${OUT}/wuzapi.dump"
  docker exec -e WUZAPI_DB="$WUZAPI_DB" "$PG" sh -c 'pg_dump -U "${POSTGRES_USER:-postgres}" -d "$WUZAPI_DB" -Fc' > "${OUT}/wuzapi.dump"
  [ -s "${OUT}/wuzapi.dump" ] || { echo "wuzapi dump is empty" >&2; exit 1; }
  FILES+=(wuzapi.dump)
fi

# volume_tar <container> <volume> <archive>: the container is paused for a
# consistent copy of its data directory.
volume_tar() {
  local container="$1" volume="$2" archive="$3"
  echo "volume ${volume} → ${OUT}/${archive}"
  docker pause "$container" >/dev/null
  # shellcheck disable=SC2064 # expand now: the trap must unpause this container
  trap "docker unpause '$container' >/dev/null 2>&1 || true" EXIT
  docker run --rm -v "${volume}:/data:ro" -v "${OUT_ABS}:/backup" alpine:3.22 \
    tar czf "/backup/${archive}" -C /data .
  docker unpause "$container" >/dev/null
  trap - EXIT
  FILES+=("$archive")
}

volume_tar "$SEAWEED" "$SEAWEED_VOLUME" seaweedfs.tar.gz
[ "$SKIP_MEILI" = 1 ] || volume_tar "$MEILI" "$MEILI_VOLUME" meili.tar.gz
[ "$SKIP_WUZAPI" = 1 ] || volume_tar "$WUZAPI" "$WUZAPI_VOLUME" wuzapi.tar.gz

if command -v sha256sum >/dev/null; then
  ( cd "$OUT" && sha256sum "${FILES[@]}" > SHA256SUMS )
else
  ( cd "$OUT" && shasum -a 256 "${FILES[@]}" > SHA256SUMS )
fi
echo "backup ready: ${OUT}"

# Retention (directory names are timestamps, so ls ordering is safe)
# shellcheck disable=SC2012
ls -1dt "${DIR}"/*/ 2>/dev/null | tail -n +"$((KEEP + 1))" | xargs -r rm -rf
