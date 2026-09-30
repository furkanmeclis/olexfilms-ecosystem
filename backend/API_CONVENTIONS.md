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

## Plan limits

Tenant mutations may be refused by the organization's subscription (`internal/platform/entitlements`):

- `409 LIMIT_REACHED` — a hard limit is exhausted; `details` carries `feature`, `limit`, `used`, `tolerance`.
- `403 FEATURE_DISABLED` — the plan turns the feature / module off (also returned by `middleware.RequireFeature` route gates).

Handlers map `*entitlements.LimitError` → `middleware.WriteLimitReached` and `entitlements.ErrFeatureDisabled` → `FEATURE_DISABLED`.

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
