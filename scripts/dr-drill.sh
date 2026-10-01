#!/usr/bin/env bash
# Disaster-recovery drill: proves a backup can be restored, not only taken.
#
#   scripts/dr-drill.sh            (or: make dr-drill)
#
# Everything runs in throw-away containers (prefix "drill-<pid>") on free
# ports; no existing database, volume or container is touched. Steps:
#
#   1. start Postgres, SeaweedFS, Redis, Meilisearch and a wuzapi stand-in;
#      migrate; create an admin; put two files in object storage, a document
#      in Meilisearch, a table in the wuzapi database and a wuzapi media file
#   2. fingerprint every table (app + wuzapi DB), the files, the search
#      index and the wuzapi media
#   3. scripts/backup.sh  (the production backup, unchanged)
#   4. disaster: drop both database schemas and wipe every data volume
#   5. scripts/restore.sh (the production restore, non-interactive)
#   6. compare fingerprints, check migrations are at head, start the API on the
#      restored data and sign in
#   7. remove every drill container, volume and backup
#
# Exit code 0 means the backup restored byte-for-byte and the API came back.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PREFIX="drill-$$"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/olex-dr-drill.XXXXXX")"
PG_PORT="${DRILL_PG_PORT:-$((20000 + RANDOM % 20000))}"
S3_PORT="$((PG_PORT + 1))"
REDIS_PORT="$((PG_PORT + 2))"
API_PORT="$((PG_PORT + 3))"
MEILI_PORT="$((PG_PORT + 4))"
DB_URL="postgres://app:app@127.0.0.1:${PG_PORT}/app?sslmode=disable"
S3_KEY=drill-s3-key
S3_SECRET=drill-s3-secret-000000
MEILI_KEY=drill-meili-master-key-0000000000
API_PID=""
CONTAINERS="${PREFIX}-postgres ${PREFIX}-seaweedfs ${PREFIX}-redis ${PREFIX}-meilisearch ${PREFIX}-wuzapi"
VOLUMES="${PREFIX}-seaweedfs ${PREFIX}-postgres ${PREFIX}-meilisearch ${PREFIX}-wuzapi"

log() { printf '\n==> %s\n' "$*"; }
sha() { if command -v sha256sum >/dev/null; then sha256sum; else shasum -a 256; fi; } # stdin only

cleanup() {
  if [ -n "$API_PID" ]; then kill "$API_PID" 2>/dev/null || true; fi
  # shellcheck disable=SC2086 # word-split the name lists
  docker rm -f $CONTAINERS >/dev/null 2>&1 || true
  # shellcheck disable=SC2086
  docker volume rm $VOLUMES >/dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap cleanup EXIT

psql_q() { docker exec -i "${PREFIX}-postgres" psql -U app -d app -v ON_ERROR_STOP=1 -At "$@"; }
psql_w() { docker exec -i "${PREFIX}-postgres" psql -U app -d wuzapi -v ON_ERROR_STOP=1 -At "$@"; }

wait_for() { # wait_for <description> <command...>
  local what="$1"; shift
  for _ in $(seq 1 60); do "$@" >/dev/null 2>&1 && return 0; sleep 1; done
  echo "timed out waiting for $what" >&2; return 1
}

s3() { # s3 <method> <key> [curl args]
  local method="$1" key="$2"; shift 2
  curl -fsS --aws-sigv4 "aws:amz:us-east-1:s3" --user "${S3_KEY}:${S3_SECRET}" \
    -X "$method" "http://127.0.0.1:${S3_PORT}/app/${key}" "$@"
}
meili() { # meili <method> <path> [curl args]
  local method="$1" path="$2"; shift 2
  curl -fsS -X "$method" -H "Authorization: Bearer ${MEILI_KEY}" -H 'Content-Type: application/json' \
    "http://127.0.0.1:${MEILI_PORT}${path}" "$@"
}

table_fingerprint() { # table_fingerprint <psql function>
  local q="$1" tables
  tables="$($q -c "SELECT tablename FROM pg_tables WHERE schemaname='public' AND tablename <> 'schema_migrations' ORDER BY 1")"
  for t in $tables; do
    $q -c "SELECT '${t}', COUNT(*), COALESCE(md5(string_agg(x::text, '|' ORDER BY x::text)), '-') FROM \"${t}\" x"
  done
}

fingerprint() { # tables, files, search document, wuzapi media
  table_fingerprint psql_q
  table_fingerprint psql_w | sed 's/^/wuzapi:/'
  for k in drill/contract.pdf drill/nested/photo.bin; do
    printf 'file:%s %s\n' "$k" "$(s3 GET "$k" | sha | cut -d' ' -f1)"
  done
  printf 'meili:%s\n' "$(meili GET /indexes/drill/documents/1 | sha | cut -d' ' -f1)"
  printf 'wuzapi-media:%s\n' "$(docker exec "${PREFIX}-wuzapi" cat /app/files/user_1/media.bin | sha | cut -d' ' -f1)"
}

log "building binaries"
( cd "$ROOT/backend" && for c in server create-super-admin; do go build -o "$WORK/$c" "./cmd/$c"; done )

log "starting throw-away Postgres, SeaweedFS, Redis, Meilisearch, wuzapi stand-in (${PREFIX})"
docker run -d --name "${PREFIX}-postgres" -e POSTGRES_USER=app -e POSTGRES_PASSWORD=app -e POSTGRES_DB=app \
  -v "${PREFIX}-postgres:/var/lib/postgresql" -v "$ROOT/deploy/postgres/initdb:/docker-entrypoint-initdb.d:ro" \
  -p "${PG_PORT}:5432" postgres:18-alpine >/dev/null
docker run -d --name "${PREFIX}-seaweedfs" -e AWS_ACCESS_KEY_ID="$S3_KEY" -e AWS_SECRET_ACCESS_KEY="$S3_SECRET" \
  -v "${PREFIX}-seaweedfs:/data" -p "${S3_PORT}:8333" chrislusf/seaweedfs:4.47 \
  mini -dir=/data -ip=127.0.0.1 -ip.bind=0.0.0.0 -bucket=app -s3.port=8333 -s3.port.iceberg=0 \
  -s3.port.lance=0 -webdav=false -admin.ui=false -master.telemetry=false >/dev/null
docker run -d --name "${PREFIX}-redis" -p "${REDIS_PORT}:6379" redis:8-alpine >/dev/null
docker run -d --name "${PREFIX}-meilisearch" -e MEILI_ENV=development -e MEILI_MASTER_KEY="$MEILI_KEY" \
  -v "${PREFIX}-meilisearch:/meili_data" -p "${MEILI_PORT}:7700" getmeili/meilisearch:v1.11 >/dev/null
# wuzapi itself needs a WhatsApp login; the drill only needs its media volume
# and its database, so a stand-in container holds the volume.
docker run -d --name "${PREFIX}-wuzapi" -v "${PREFIX}-wuzapi:/app/files" alpine:3.22 sleep infinity >/dev/null
wait_for postgres docker exec "${PREFIX}-postgres" pg_isready -U app -d app
wait_for seaweedfs curl -fsS "http://127.0.0.1:${S3_PORT}/healthz"
wait_for meilisearch curl -fsS "http://127.0.0.1:${MEILI_PORT}/health"
sleep 2 # Postgres restarts once after initdb
wait_for "wuzapi database (initdb)" psql_w -c 'SELECT 1'

export APP_ENV=test DB_HOST=127.0.0.1 DB_PORT="$PG_PORT" DB_USER=app DB_PASSWORD=app DB_NAME=app DB_SSLMODE=disable \
  REDIS_ADDR="127.0.0.1:${REDIS_PORT}" STORAGE_DRIVER=s3 S3_ENDPOINT="http://127.0.0.1:${S3_PORT}" \
  S3_ACCESS_KEY="$S3_KEY" S3_SECRET_KEY="$S3_SECRET" S3_BUCKET=app S3_REGION=us-east-1 S3_USE_PATH_STYLE=true \
  SEARCH_ENABLED=false CENTRIFUGO_ENABLED=false QUEUE_WORKER_INPROCESS=false MAIL_DRIVER=log \
  APP_HTTP_ADDR=":${API_PORT}" LOG_LEVEL=warn

log "migrating and loading data"
if command -v migrate >/dev/null; then
  migrate -path "$ROOT/backend/migrations" -database "$DB_URL" up >/dev/null
else
  docker run --rm --network host -v "$ROOT/backend/migrations:/m:ro" migrate/migrate:v4.19.1 -path /m -database "$DB_URL" up >/dev/null
fi
HEAD_VERSION="$(psql_q -c 'SELECT version FROM schema_migrations')"
"$WORK/create-super-admin" -email drill@example.test -password 'Drill1234x' -name Drill -surname Admin >/dev/null
head -c 300000 /dev/urandom > "$WORK/contract.pdf"
head -c 70000 /dev/urandom > "$WORK/photo.bin"
s3 PUT drill/contract.pdf -T "$WORK/contract.pdf" >/dev/null
s3 PUT drill/nested/photo.bin -T "$WORK/photo.bin" >/dev/null
TASK="$(meili POST /indexes/drill/documents -d '[{"id":1,"title":"drill","body":"backup restore"}]' | sed -E 's/.*"taskUid":([0-9]+).*/\1/')"
wait_for "meili indexing" sh -c "curl -fsS -H 'Authorization: Bearer ${MEILI_KEY}' http://127.0.0.1:${MEILI_PORT}/tasks/${TASK} | grep -q '\"status\":\"succeeded\"'"
psql_w -c "CREATE TABLE drill_sessions (id int PRIMARY KEY, jid text); INSERT INTO drill_sessions VALUES (1, '905551234567@s.whatsapp.net');" >/dev/null
docker exec "${PREFIX}-wuzapi" sh -c 'mkdir -p /app/files/user_1 && head -c 50000 /dev/urandom > /app/files/user_1/media.bin'

log "fingerprinting"
fingerprint > "$WORK/before.txt"
echo "$(grep -c '|' "$WORK/before.txt") tables, 2 files, 1 search document, 1 wuzapi media file; migration version ${HEAD_VERSION}"

DRILL_ENV=(NAME_PREFIX="$PREFIX" PG_CONTAINER="${PREFIX}-postgres"
  SEAWEED_CONTAINER="${PREFIX}-seaweedfs" SEAWEEDFS_VOLUME_NAME="${PREFIX}-seaweedfs"
  MEILI_CONTAINER="${PREFIX}-meilisearch" MEILI_VOLUME_NAME="${PREFIX}-meilisearch"
  WUZAPI_CONTAINER="${PREFIX}-wuzapi" WUZAPI_VOLUME_NAME="${PREFIX}-wuzapi"
  APP_CONTAINERS="${PREFIX}-none")

log "backup (scripts/backup.sh)"
env "${DRILL_ENV[@]}" BACKUP_DIR="$WORK/backups" "$ROOT/scripts/backup.sh"
BACKUP="$(find "$WORK/backups" -mindepth 1 -maxdepth 1 -type d | head -1)"
ls -la "$BACKUP"

log "disaster: dropping both database schemas and wiping every data volume"
psql_q -c 'SET client_min_messages = warning; DROP SCHEMA public CASCADE; CREATE SCHEMA public;' >/dev/null
psql_w -c 'SET client_min_messages = warning; DROP SCHEMA public CASCADE; CREATE SCHEMA public;' >/dev/null
for svc in seaweedfs meilisearch wuzapi; do
  mount=/data; [ "$svc" = meilisearch ] && mount=/meili_data; [ "$svc" = wuzapi ] && mount=/app/files
  docker stop "${PREFIX}-${svc}" >/dev/null
  docker run --rm -v "${PREFIX}-${svc}:${mount}" alpine:3.22 sh -c "find ${mount} -mindepth 1 -delete"
  docker start "${PREFIX}-${svc}" >/dev/null
done
wait_for seaweedfs curl -fsS "http://127.0.0.1:${S3_PORT}/healthz"
wait_for meilisearch curl -fsS "http://127.0.0.1:${MEILI_PORT}/health"
if s3 GET drill/contract.pdf -o /dev/null 2>/dev/null; then echo "files survived the wipe?" >&2; exit 1; fi
if meili GET /indexes/drill/documents/1 >/dev/null 2>&1; then echo "search index survived the wipe?" >&2; exit 1; fi
[ "$(psql_q -c "SELECT COUNT(*) FROM pg_tables WHERE schemaname='public'")" = "0" ] || { echo "schema not empty" >&2; exit 1; }

log "restore (scripts/restore.sh)"
env "${DRILL_ENV[@]}" RESTORE_CONFIRM="$PREFIX" "$ROOT/scripts/restore.sh" "$BACKUP"
wait_for seaweedfs curl -fsS "http://127.0.0.1:${S3_PORT}/healthz"
wait_for "restored files" s3 GET drill/contract.pdf -o /dev/null
wait_for "restored search index" meili GET /indexes/drill/documents/1

log "integrity check"
fingerprint > "$WORK/after.txt"
if ! diff -u "$WORK/before.txt" "$WORK/after.txt"; then
  echo "FAIL: restored data differs from the backup" >&2
  exit 1
fi
[ "$(psql_q -c 'SELECT version FROM schema_migrations')" = "$HEAD_VERSION" ] || { echo "FAIL: migration version changed" >&2; exit 1; }
[ "$(psql_q -c 'SELECT dirty FROM schema_migrations')" = "f" ] || { echo "FAIL: migrations dirty" >&2; exit 1; }
echo "every table, file, search document and wuzapi media file is identical to the backup"

log "starting the API on the restored data"
"$WORK/server" > "$WORK/api.log" 2>&1 &
API_PID=$!
wait_for api curl -fsS "http://127.0.0.1:${API_PORT}/readyz" || { tail -20 "$WORK/api.log"; exit 1; }
curl -fsS "http://127.0.0.1:${API_PORT}/readyz"; echo
curl -fsS -X POST "http://127.0.0.1:${API_PORT}/v1/auth/login" -H 'Content-Type: application/json' \
  -d '{"email":"drill@example.test","password":"Drill1234x"}' | grep -q '"access_token"' \
  || { echo "FAIL: cannot sign in after restore" >&2; exit 1; }
echo "signed in as the restored admin"

log "DR drill passed: backup → wipe → restore → integrity → API up"
