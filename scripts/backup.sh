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
#
# Off-server copy (TEC-1259): OFFSITE_TARGET copies the finished backup
# directory to <target>/<stamp> after the checksums are written:
#   OFFSITE_TARGET=s3://bucket/olexfilms   aws CLI (aws s3 cp --recursive)
#   OFFSITE_TARGET=remote:olexfilms        rclone (any rclone remote; use a
#                                          crypt remote for client-side encryption)
# OFFSITE_TOOL=aws|rclone forces the tool. A failed copy exits non-zero (cron
# mails it) and the local backup stays. The encryption keys are NOT in the
# backup: keep CUSTOMER_PII_KEY, APP_ENCRYPTION_KEY and
# WUZAPI_GLOBAL_ENCRYPTION_KEY in a separate vault (docs/deploy.md §4).
#
# Freshness check (monitoring cron, no backup taken):
#   BACKUP_DIR=/srv/backups scripts/backup.sh --check
# exits 1 when the newest backup is older than MAX_AGE_HOURS (default 26),
# has no SHA256SUMS or fails its checksums.
set -euo pipefail

# check_latest: age and integrity of the newest backup in $BACKUP_DIR.
check_latest() {
  local dir="${BACKUP_DIR:-./backups}" max_h="${MAX_AGE_HOURS:-26}" latest now mtime age_h
  # shellcheck disable=SC2012 # timestamp directory names
  latest="$(ls -1dt "${dir}"/*/ 2>/dev/null | head -n 1 || true)"
  if [ -z "$latest" ]; then
    echo "backup check: no backup in ${dir}" >&2
    return 1
  fi
  latest="${latest%/}"
  if [ ! -s "${latest}/SHA256SUMS" ]; then
    echo "backup check: ${latest} has no SHA256SUMS (incomplete backup)" >&2
    return 1
  fi
  now="$(date +%s)"
  mtime="$(stat -c %Y "${latest}/SHA256SUMS" 2>/dev/null || stat -f %m "${latest}/SHA256SUMS")"
  age_h=$(((now - mtime) / 3600))
  if [ "$age_h" -ge "$max_h" ]; then
    echo "backup check: newest backup ${latest} is ${age_h}h old (limit ${max_h}h)" >&2
    return 1
  fi
  if command -v sha256sum >/dev/null; then
    (cd "$latest" && sha256sum --quiet -c SHA256SUMS) || { echo "backup check: checksum mismatch in ${latest}" >&2; return 1; }
  else
    (cd "$latest" && shasum -a 256 -q -c SHA256SUMS) || { echo "backup check: checksum mismatch in ${latest}" >&2; return 1; }
  fi
  echo "backup check: ${latest} is ${age_h}h old, checksums ok"
}

if [ "${1:-}" = "--check" ]; then
  check_latest
  exit $?
fi

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

# offsite_copy: copies the finished backup directory off the server.
offsite_copy() {
  local target="${OFFSITE_TARGET%/}/${STAMP}" tool="${OFFSITE_TOOL:-}"
  if [ -z "$tool" ]; then
    case "$OFFSITE_TARGET" in
      s3://*) tool=aws ;;
      *) tool=rclone ;;
    esac
  fi
  echo "off-server copy (${tool}) → ${target}"
  case "$tool" in
    aws) aws s3 cp --recursive --only-show-errors "$OUT" "$target" ;;
    rclone) rclone copy "$OUT" "$target" ;;
    *) echo "OFFSITE_TOOL must be aws or rclone, got ${tool}" >&2; return 1 ;;
  esac
}

OFFSITE_FAILED=0
if [ -n "${OFFSITE_TARGET:-}" ]; then
  offsite_copy || { echo "off-server copy FAILED; local backup kept at ${OUT}" >&2; OFFSITE_FAILED=1; }
else
  echo "warning: OFFSITE_TARGET is not set; the backup exists only on this server" >&2
fi

# Retention (directory names are timestamps, so ls ordering is safe)
# shellcheck disable=SC2012
ls -1dt "${DIR}"/*/ 2>/dev/null | tail -n +"$((KEEP + 1))" | xargs -r rm -rf

[ "$OFFSITE_FAILED" = 0 ] || exit 1
