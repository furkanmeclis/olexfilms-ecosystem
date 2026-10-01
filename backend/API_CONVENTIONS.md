# API conventions

HTTP contract for `github.com/furkanmeclis/olexfilms-ecosystem/backend`. Paths use `/v1/...` (no `/api` prefix).

## Response envelope

All `/v1/*` handlers return:

```json
{
  "success": true,
  "data": {},
  "meta": { "request_id": "uuid" }
}
```

Errors:

```json
{
  "success": false,
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "...",
    "details": [{ "field": "email", "message": "...", "code": "invalid" }]
  },
  "meta": { "request_id": "uuid" }
}
```

- Success: `response.JSON(w, r, status, data)`
- Errors: `Unauthorized` / `Forbidden` / `NotFound` / `Conflict` / `ValidationError` / `Internal`
- Branch on `error.code`, not message text
- Correlation: middleware sets `X-Request-ID` (generate if missing) and mirrors into `meta.request_id`
- **Outside envelope:** `GET /healthz`, `GET /readyz`

## Lists

```json
{
  "success": true,
  "data": { "items": [], "total": 0, "limit": 20, "offset": 0 },
  "meta": { "request_id": "..." }
}
```

Parse with `pkg/apiquery`:

| Param | Notes |
|-------|--------|
| `limit` | default 20, max 100 |
| `offset` | default 0 |
| `q` | search text |
| `sort` | e.g. `-created_at,name` |
| `include` / `fields` | sparse / expand (when supported) |

**Forbidden query names:** `page`, `page_size`, `per_page`, `search` (use `q` only). Unknown `sort` → `400 VALIDATION_ERROR` with details.

## Resource meta

Primary list resources expose `GET /v1/{resource}/meta` via `internal/platform/resourcemeta`:

- `GET /v1/platform/users/meta`
- `GET /v1/platform/roles/meta`
- `GET /v1/notifications/meta`
- `GET /v1/platform/notifications/meta`
- `GET /v1/platform/activity/meta`
- `GET /v1/platform/storage/meta`
- `GET /v1/platform/logs/meta`
- `GET /v1/platform/log-rules/meta`

Register `/meta` **before** `/{uuid}` on `ServeMux`. Capabilities must match live routes (do not advertise unfinished bulk/export).

## Auth

- Bearer JWT on protected routes
- Global users; no tenant (`tid`) or workspace (`wid`) claims
- User fields: `name` / `surname`
- Platform routes require the matching `platform.*` permission (`super_admin` bypasses `HasPermission`)

## Module flags (feature flags)

Modules are switched per organization (`internal/platform/features`, TEC-86); there are no plan
or subscription limits. The Go catalog (`features.Modules`) is the single source of module keys:
core (always on), standard (on by default) and add-on (off by default).

- `403 FEATURE_DISABLED` — `middleware.RequireFeature(checker, key)` (after `RequireOrganization`)
  when the module is off for the active organization. Unknown keys are off.
- `409 FEATURE_DISABLED` — a write tried to switch a module on below a level where it is off
  (closed system wide, or not enabled for the distributor).
- `422 MODULE_CORE` — core modules cannot be switched off.
- `409 MODULE_ADMIN_OVERRIDE` — the platform admin set the dealer's value; the distributor cannot change it.

Values are resolved on read (live inheritance) and cached in Redis for 30 s; every write invalidates
the affected snapshots (system changes bump a generation counter), so changes apply at once.
`LIMIT_REACHED` stays reserved; no endpoint emits it.

## Notifications

Domain / auth use cases call `notifications.Service.Enqueue(...)` only. HTTP handlers never call SMTP/SMS inline.

- Channels: `inapp` | `email` | `realtime` | `sms` | `push` (sms/push = noop unless VAPID is configured)
- Delivery: Asynq task `app:notification:deliver` on queue `notifications`
- **Security emails** (`auth.password_reset`, `auth.email_verification`) ignore `email_enabled` preference
- Realtime channel hint: `user:{user_uuid}`

Cross-module signals use the platform event bus (`internal/platform/events`). Use cases `Publish`; bus handlers may enqueue notifications — never SMTP.

## OpenAPI

Contract: `docs/openapi.yaml` (OpenAPI 3.1) + Scalar at `/docs/`. Run `make openapi-lint` after path/schema changes.

Live routes are registered in `internal/modules/**/routes.go`. If the YAML still describes a stripped product path, the Go router wins until the spec is trimmed.
