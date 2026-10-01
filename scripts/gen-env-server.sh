#!/usr/bin/env bash
# Builds .env.server (production) from .env.example: generates every secret,
# fills the domain-derived URLs and switches the app to production.
#
#   scripts/gen-env-server.sh --app olexfilms.app --realtime wss.olexfilms.app \
#       [--prefix olexfilms] [--mail-from noreply@olexfilms.app] \
#       [--smtp-host smtp.example.com --smtp-port 587 \
#        --smtp-user USER --smtp-pass PASS] [--out .env.server] \
#       [--rotate] [--rotate-encryption-key]
#
# Re-running keeps every value already in the output file (secrets, SMTP,
# migrator DSNs, anything entered by hand); only the prefix- and
# domain-derived keys are rewritten, so a re-run changes only the date line.
# --rotate regenerates the secrets (DB_PASSWORD and S3 keys too: update the
# running Postgres role / SeaweedFS identity yourself). It never touches
# APP_ENCRYPTION_KEY (data encrypted at rest) or WUZAPI_GLOBAL_ENCRYPTION_KEY
# (WhatsApp sessions); --rotate-encryption-key regenerates APP_ENCRYPTION_KEY
# only, which makes every stored encrypted value unreadable. CUSTOMER_PII_KEY
# (customer identity numbers) is generated once and never rotated here.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
EXAMPLE="$ROOT/.env.example"
OUT="$ROOT/.env.server"
APP_DOMAIN=""
RT_DOMAIN=""
PREFIX="olexfilms"
MAIL_FROM=""
SMTP_HOST=""
SMTP_PORT=""
SMTP_USER=""
SMTP_PASS=""
ROTATE=0
ROTATE_ENC=0

usage() { sed -n '2,18p' "$0" | sed 's/^# \{0,1\}//'; exit "${1:-0}"; }

while [ $# -gt 0 ]; do
  case "$1" in
    --app) APP_DOMAIN="${2:-}"; shift 2 ;;
    --realtime) RT_DOMAIN="${2:-}"; shift 2 ;;
    --prefix) PREFIX="${2:-}"; shift 2 ;;
    --mail-from) MAIL_FROM="${2:-}"; shift 2 ;;
    --smtp-host) SMTP_HOST="${2:-}"; shift 2 ;;
    --smtp-port) SMTP_PORT="${2:-}"; shift 2 ;;
    --smtp-user) SMTP_USER="${2:-}"; shift 2 ;;
    --smtp-pass) SMTP_PASS="${2:-}"; shift 2 ;;
    --out) OUT="${2:-}"; shift 2 ;;
    --rotate) ROTATE=1; shift ;;
    --rotate-encryption-key) ROTATE_ENC=1; shift ;;
    -h|--help) usage 0 ;;
    *) echo "unknown option: $1" >&2; usage 1 ;;
  esac
done

if [ -z "$APP_DOMAIN" ] || [ -z "$RT_DOMAIN" ]; then
  echo "--app and --realtime are required" >&2; usage 1
fi
[ -n "$PREFIX" ] || { echo "--prefix must not be empty" >&2; exit 1; }
command -v openssl >/dev/null || { echo "openssl is required" >&2; exit 1; }
[ -f "$EXAMPLE" ] || { echo "missing $EXAMPLE" >&2; exit 1; }
# Bare host names only (no scheme / path).
for d in "$APP_DOMAIN" "$RT_DOMAIN"; do
  case "$d" in *://*|*/*|*" "*) echo "give a bare host name, not a URL: $d" >&2; exit 1 ;; esac
done
# olexfilms.app → olexfilms.app; app.example.com → example.com
case "$APP_DOMAIN" in
  *.*.*) MAIL_DOMAIN="${APP_DOMAIN#*.}" ;;
  *) MAIL_DOMAIN="$APP_DOMAIN" ;;
esac
# Postgres role / database: no hyphens (olex-films → olex_films).
DB_IDENT="$(printf '%s' "$PREFIX" | tr '-' '_')"
NET="$PREFIX-app"

hex() { openssl rand -hex "$1"; }
b64() { openssl rand -base64 "$1" | tr -d '\n'; }
alnum() { LC_ALL=C tr -dc 'A-Z0-9' </dev/urandom | head -c "$1" || true; }

# Existing value of KEY in the current output file.
existing() {
  [ -f "$OUT" ] || return 0
  grep -E "^$1=" "$OUT" | tail -n1 | cut -d= -f2- || true
}
# secret KEY generator-command... — keeps the current value unless --rotate.
secret() {
  local key="$1"; shift
  local cur=""; [ "$ROTATE" = 1 ] || cur="$(existing "$key")"
  if [ -n "$cur" ]; then printf '%s' "$cur"; else "$@"; fi
}
# stable KEY rotate-flag generator-command... — keeps the current value
# unless the given flag is 1 (encryption keys: --rotate does not apply).
stable() {
  local key="$1" flag="$2"; shift 2
  local cur=""; [ "$flag" = 1 ] || cur="$(existing "$key")"
  if [ -n "$cur" ]; then printf '%s' "$cur"; else "$@"; fi
}
# keep KEY default — hand-entered value wins, else the default.
keep() {
  local cur; cur="$(existing "$1")"
  if [ -n "$cur" ]; then printf '%s' "$cur"; else printf '%s' "$2"; fi
}

# Web Push VAPID pair (P-256): public = uncompressed point, private = scalar, base64url.
vapid_pair() {
  local pem pub priv
  pem="$(openssl ecparam -name prime256v1 -genkey -noout 2>/dev/null)"
  pub="$(printf '%s\n' "$pem" | openssl ec -pubout -outform DER 2>/dev/null | tail -c 65 | openssl base64 -A | tr '+/' '-_' | tr -d '=')"
  priv="$(printf '%s\n' "$pem" | openssl ec -outform DER 2>/dev/null | head -c 39 | tail -c 32 | openssl base64 -A | tr '+/' '-_' | tr -d '=')"
  printf '%s %s' "$pub" "$priv"
}
VAPID_PUB=""; VAPID_PRIV=""
if [ "$ROTATE" != 1 ]; then
  VAPID_PUB="$(existing VAPID_PUBLIC_KEY)"; VAPID_PRIV="$(existing VAPID_PRIVATE_KEY)"
fi
if [ -z "$VAPID_PUB" ] || [ -z "$VAPID_PRIV" ]; then
  read -r VAPID_PUB VAPID_PRIV <<<"$(vapid_pair)"
fi
[ -n "$MAIL_FROM" ] || MAIL_FROM="$(existing MAIL_FROM)"
[ -n "$MAIL_FROM" ] || MAIL_FROM="noreply@$MAIL_DOMAIN"

APP_URL="https://$APP_DOMAIN"
WS_URL="wss://$RT_DOMAIN/connection/websocket"

OVERRIDES="$(mktemp)"
TMP_OUT=""
trap 'rm -f "$OVERRIDES" ${TMP_OUT:+"$TMP_OUT"}' EXIT
set_kv() { printf '%s=%s\n' "$1" "$2" >>"$OVERRIDES"; }

# --- Identity / naming ---
set_kv NAME_PREFIX "$PREFIX"
set_kv COMPOSE_PROJECT_NAME "$PREFIX"
set_kv LOCAL_NETWORK_NAME "$PREFIX"
set_kv APP_NETWORK_NAME "$NET"
set_kv POSTGRES_VOLUME_NAME "$PREFIX-prod-postgres"
set_kv REDIS_VOLUME_NAME "$PREFIX-prod-redis"
set_kv SEAWEEDFS_VOLUME_NAME "$PREFIX-prod-seaweedfs"
set_kv MEILI_VOLUME_NAME "$PREFIX-prod-meilisearch"
set_kv WUZAPI_VOLUME_NAME "$PREFIX-prod-wuzapi"
set_kv BACKEND_IMAGE "$PREFIX-backend:prod"
set_kv FRONTEND_IMAGE "$PREFIX-frontend:prod"
# --- App ---
set_kv APP_NAME "$PREFIX"
set_kv APP_ENV production
set_kv LOG_LEVEL info
# --- Postgres (compose wires the host as <service>.<app network>) ---
set_kv DB_HOST "postgres.$NET"
set_kv DB_USER "$DB_IDENT"
set_kv DB_NAME "$DB_IDENT"
set_kv DB_PASSWORD "$(secret DB_PASSWORD hex 24)"
# --- Redis ---
set_kv REDIS_ADDR "redis.$NET:6379"
set_kv REDIS_PASSWORD "$(secret REDIS_PASSWORD hex 24)"
# --- SeaweedFS ---
set_kv S3_ENDPOINT "http://seaweedfs.$NET:8333"
set_kv S3_ACCESS_KEY "$(secret S3_ACCESS_KEY alnum 20)"
set_kv S3_SECRET_KEY "$(secret S3_SECRET_KEY hex 20)"
set_kv S3_BUCKET "$PREFIX"
set_kv SEAWEEDFS_JWT_WRITE_KEY "$(secret SEAWEEDFS_JWT_WRITE_KEY hex 32)"
set_kv SEAWEEDFS_JWT_READ_KEY "$(secret SEAWEEDFS_JWT_READ_KEY hex 32)"
# --- Centrifugo ---
set_kv CENTRIFUGO_API_URL "http://centrifugo.$NET:8000"
set_kv CENTRIFUGO_API_KEY "$(secret CENTRIFUGO_API_KEY hex 32)"
set_kv CENTRIFUGO_TOKEN_HMAC_SECRET "$(secret CENTRIFUGO_TOKEN_HMAC_SECRET hex 32)"
set_kv CENTRIFUGO_WS_URL "$WS_URL"
set_kv CENTRIFUGO_ALLOWED_ORIGINS "$APP_URL"
# --- JWT ---
set_kv JWT_ACCESS_SECRET "$(secret JWT_ACCESS_SECRET hex 48)"
set_kv JWT_REFRESH_SECRET "$(secret JWT_REFRESH_SECRET hex 48)"
# --- Mail ---
set_kv MAIL_FROM "$MAIL_FROM"
set_kv SMTP_FROM "$MAIL_FROM"
set_kv MAIL_FROM_NAME "$(keep MAIL_FROM_NAME Olexfilms)"
# SMTP: flags win; otherwise a hand-entered value stays and the local
# MailHog defaults from .env.example (127.0.0.1:1025) are cleared.
cur_host="$(existing SMTP_HOST)"; cur_port="$(existing SMTP_PORT)"
if [ -n "$SMTP_HOST" ]; then set_kv SMTP_HOST "$SMTP_HOST"
elif [ -z "$cur_host" ] || [ "$cur_host" = 127.0.0.1 ]; then set_kv SMTP_HOST ""; fi
if [ -n "$SMTP_PORT" ]; then set_kv SMTP_PORT "$SMTP_PORT"
elif [ -z "$cur_port" ] || [ "$cur_port" = 1025 ]; then set_kv SMTP_PORT 587; fi
if [ -n "$SMTP_USER" ]; then set_kv SMTP_USERNAME "$SMTP_USER"; fi
if [ -n "$SMTP_PASS" ]; then set_kv SMTP_PASSWORD "$SMTP_PASS"; fi
# --- Search ---
set_kv MEILI_HOST "http://meilisearch.$NET:7700"
set_kv MEILI_MASTER_KEY "$(secret MEILI_MASTER_KEY hex 32)"
set_kv MEILI_INDEX_PREFIX "$PREFIX"
# --- PDF (Gotenberg, private) ---
set_kv GOTENBERG_URL "http://gotenberg.$NET:3000"
set_kv PDF_FONTS "$(keep PDF_FONTS embedded)"
# --- WhatsApp gateway (wuzapi, private) ---
set_kv WUZAPI_URL "http://wuzapi.$NET:8080"
set_kv WUZAPI_ADMIN_TOKEN "$(secret WUZAPI_ADMIN_TOKEN hex 32)"
set_kv WUZAPI_WEBHOOK_SECRET "$(secret WUZAPI_WEBHOOK_SECRET hex 32)"
# Backend webhook on the private app network (wuzapi → backend).
set_kv WUZAPI_WEBHOOK_URL "$(keep WUZAPI_WEBHOOK_URL "http://backend.$NET:8080/hooks/wuzapi")"
# AES-256 key, exactly 32 characters. Never rotated by this script.
set_kv WUZAPI_GLOBAL_ENCRYPTION_KEY "$(stable WUZAPI_GLOBAL_ENCRYPTION_KEY 0 hex 16)"
# --- Queue: separate worker containers in production ---
set_kv QUEUE_WORKER_INPROCESS false
set_kv WORKER_QUEUES "$(keep WORKER_QUEUES critical,default,low)"
set_kv WORKER_DOCS_QUEUES "$(keep WORKER_DOCS_QUEUES docs)"
set_kv WORKER_DOCS_CONCURRENCY "$(keep WORKER_DOCS_CONCURRENCY 20)"
set_kv SCHEDULER_ENABLED true
# --- Public URLs ---
set_kv CORS_ALLOWED_ORIGINS "$APP_URL"
set_kv PUBLIC_API_URL "$APP_URL/api"
set_kv PUBLIC_FRONTEND_URL "$APP_URL"
set_kv SITE_URL "$APP_URL"
set_kv API_URL "http://backend.$NET:8080/v1"
# --- Encryption at rest (never rotate once data exists) ---
set_kv APP_ENCRYPTION_KEY "$(stable APP_ENCRYPTION_KEY "$ROTATE_ENC" b64 32)"
# Customer national ids / tax numbers (TEC-159): generated once, never rotated.
set_kv CUSTOMER_PII_KEY "$(stable CUSTOMER_PII_KEY 0 b64 32)"
# --- NextAuth ---
set_kv AUTH_SECRET "$(secret AUTH_SECRET hex 32)"
set_kv AUTH_URL "$APP_URL"
set_kv AUTH_ADAPTER_SECRET "$(secret AUTH_ADAPTER_SECRET hex 32)"
set_kv AUTH_WEBAUTHN_RP_ID "$APP_DOMAIN"
# --- Frontend public ---
set_kv NEXT_PUBLIC_CENTRIFUGO_URL "$WS_URL"
# --- Error tracking (technowide errortracking; DSNs are not secrets) ---
# Empty until the technowide source exists; a hand-entered DSN is kept.
set_kv SENTRY_DSN "$(keep SENTRY_DSN "")"
set_kv SENTRY_ENVIRONMENT "$(keep SENTRY_ENVIRONMENT production)"
set_kv SENTRY_RELEASE "$(keep SENTRY_RELEASE "")"
set_kv NEXT_PUBLIC_SENTRY_DSN "$(keep NEXT_PUBLIC_SENTRY_DSN "")"
set_kv NEXT_PUBLIC_SENTRY_ENVIRONMENT "$(keep NEXT_PUBLIC_SENTRY_ENVIRONMENT production)"
set_kv NEXT_PUBLIC_SENTRY_RELEASE "$(keep NEXT_PUBLIC_SENTRY_RELEASE "")"
# --- Web Push ---
set_kv VAPID_PUBLIC_KEY "$VAPID_PUB"
set_kv VAPID_PRIVATE_KEY "$VAPID_PRIV"
set_kv VAPID_SUBJECT "mailto:$MAIL_FROM"
# Everything else (migrator DSNs, hand-edited values)
# keeps its value from the existing output file; see the awk merge below.

TMP_OUT="$(mktemp)"
{
  echo "# Generated by scripts/gen-env-server.sh on $(date -u +%Y-%m-%dT%H:%M:%SZ) — never commit."
  echo "# Site: $APP_URL   Realtime: $WS_URL"
  PREV="$OUT"; [ -f "$PREV" ] || PREV=/dev/null
  # Order: .env.example lines (with overrides / previous values), then
  # overrides missing from the example, then hand-added keys — all in file
  # order, so a re-run writes the same file.
  awk -F= '
    FILENAME==ARGV[1] { k=$1; sub(/^[^=]*=/, ""); if (!(k in ov)) ovorder[++m]=k; ov[k]=$0; next }
    FILENAME==ARGV[2] { if ($0 ~ /^[A-Za-z_][A-Za-z0-9_]*=/) { k=$1; sub(/^[^=]*=/, ""); if (!(k in prev)) order[++n]=k; prev[k]=$0 } ; next }
    FNR<=3 && /^#/ { next }
    /^[A-Za-z_][A-Za-z0-9_]*=/ {
      k=$1
      if (k in seen) next
      seen[k]=1
      if (k in ov) { print k "=" ov[k]; next }
      if (k in prev) { print k "=" prev[k]; next }
    }
    { print }
    END {
      for (i=1; i<=m; i++) { k=ovorder[i]; if (!(k in seen)) { print k "=" ov[k]; seen[k]=1 } }
      for (i=1; i<=n; i++) { k=order[i]; if (!(k in seen)) { print k "=" prev[k]; seen[k]=1 } }
    }
  ' "$OVERRIDES" "$PREV" "$EXAMPLE"
} >"$TMP_OUT"

# Every key compose.prod.yml requires must be filled.
REQUIRED="BACKEND_IMAGE FRONTEND_IMAGE DB_USER DB_PASSWORD DB_NAME REDIS_PASSWORD
S3_ACCESS_KEY S3_SECRET_KEY S3_BUCKET SEAWEEDFS_JWT_WRITE_KEY SEAWEEDFS_JWT_READ_KEY
CENTRIFUGO_API_KEY CENTRIFUGO_TOKEN_HMAC_SECRET CENTRIFUGO_WS_URL CENTRIFUGO_ALLOWED_ORIGINS
JWT_ACCESS_SECRET JWT_REFRESH_SECRET MEILI_MASTER_KEY APP_ENCRYPTION_KEY CUSTOMER_PII_KEY
AUTH_SECRET AUTH_URL AUTH_ADAPTER_SECRET CORS_ALLOWED_ORIGINS
VAPID_PUBLIC_KEY VAPID_PRIVATE_KEY GOTENBERG_URL WORKER_QUEUES
WUZAPI_ADMIN_TOKEN WUZAPI_WEBHOOK_SECRET WUZAPI_GLOBAL_ENCRYPTION_KEY"
missing=""
for k in $REQUIRED; do
  grep -qE "^$k=.+" "$TMP_OUT" || missing="$missing $k"
done
[ -z "$missing" ] || { echo "empty required keys:$missing" >&2; exit 1; }

umask 077
mv "$TMP_OUT" "$OUT"
TMP_OUT=""
chmod 600 "$OUT"

echo "wrote $OUT"
if [ "$ROTATE" = 1 ]; then
  echo "  ! --rotate: DB_PASSWORD / S3 keys changed; update the running Postgres role and SeaweedFS before redeploying"
fi
if [ "$ROTATE_ENC" = 1 ]; then
  echo "  ! --rotate-encryption-key: data encrypted with the old APP_ENCRYPTION_KEY is unreadable now"
fi
[ -n "$(grep -E '^SMTP_HOST=.' "$OUT" || true)" ] || echo "  ! SMTP_HOST empty: e-mail notifications will not be sent"
echo "  migrator DSNs (MIGRATOR_*_DSN) stay empty unless set by hand."
echo
echo "Traefik / Dokploy domains (only these two services are public):"
echo "  $APP_DOMAIN  → service frontend,   port 3000  ($APP_URL)"
echo "  $RT_DOMAIN  → service centrifugo, port 8000  ($WS_URL)"
echo "next: make prod-config"
