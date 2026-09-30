# Frontend

Next.js BFF for the boilerplate. The browser talks to same-origin `/api/v1/*`; the BFF forwards to the Go API and keeps JWTs in **HttpOnly** cookies.

## Stack

- Next.js 16 App Router + React 19 + TypeScript
- Tailwind CSS 4 (sidebar, tables, entity forms, dialogs)
- TanStack Query + TanStack Table
- openapi-fetch + generated types from `../backend/docs/openapi.yaml`
- NextAuth (Go adapter) + Centrifugo client
- pnpm; `pnpm lint` / `pnpm typecheck` / `pnpm format`

## Panels

| Who                                          | Panel                                                                                                                    |
| -------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------ |
| `super_admin` or any `platform.*` permission | `/platform` — overview, users, roles, notifications, activity, logs, storage, imports, exports, settings, access, GitHub |
| Other authenticated users                    | `/` — CMS placeholder + profile                                                                                          |

Guest login: `/platform/login` (aliases at `/login`, `/forgot-password`, …).

There is no `/business` shell and no tenant/workspace switcher.

## Setup

Prefer `make frontend-dev` or `make local-dev` from the **repo root** so `API_URL` comes from root `.env`.

```sh
cd frontend
pnpm install
pnpm api:generate   # refresh types from ../backend/docs/openapi.yaml
pnpm typecheck && pnpm lint
pnpm dev            # http://localhost:3000
```

### Env (root `.env`)

| Variable                       | Purpose                                                     |
| ------------------------------ | ----------------------------------------------------------- |
| `API_URL`                      | Server-only Go upstream, default `http://127.0.0.1:8080/v1` |
| `AUTH_SECRET` / `AUTH_URL`     | NextAuth                                                    |
| `NEXT_PUBLIC_BFF_BASE_URL`     | Browser BFF base (`/api`) when set                          |
| `NEXT_PUBLIC_REALTIME_ENABLED` | `false` skips Centrifugo connect                            |
| `NEXT_PUBLIC_CENTRIFUGO_URL`   | Fallback WS URL if the token response omits `ws_url`        |

Cookie names: `app_access_token` / `app_refresh_token` (HttpOnly).

Go API listens on `APP_HTTP_ADDR` (default `:8080`). Paths are `/v1/...`.
Centrifugo HMAC stays server-side — never expose it to the browser.

## BFF

```
Browser  →  /api/v1/...              →  Next Route Handler  →  $API_URL/...
Browser  →  /api/auth/[...nextauth]  (session; no tokens in JS)
```

Login / refresh responses strip `access_token` / `refresh_token` before returning to the client; cookies are set server-side.

SSE / long-lived streams are proxied without buffering when `Accept: text/event-stream`.

## Features

| Feature             | Path                                                               |
| ------------------- | ------------------------------------------------------------------ |
| `auth`              | Login, forgot/reset, verify email                                  |
| `account`           | Profile, password, preferences, passkeys                           |
| `users`             | Platform user CRUD + export/import/bulk + impersonate              |
| `roles`             | Platform role CRUD + export/import/bulk                            |
| `notifications`     | Inbox + platform log + export                                      |
| `platform-overview` | `/platform` home KPIs                                              |
| `nav-engine`        | Sidebar catalog, accordion groups, badges, info popovers           |
| `io`                | Import/export jobs, letterhead settings, activity (shared toolbar) |
| `bulk-engine`       | Row selection, bulk menu, job rollback                             |
| `storage`           | Object explorer (folders, versions, shares, links)                 |
| `logs`              | Application logs + purge rules                                     |
| `step-up-engine`    | Re-auth gate/dialog; access policy under `/platform/access`        |
| `access`            | Step-up TTL / methods admin page                                   |
| `search-engine`     | Cmd+K palette (Meilisearch via BFF)                                |
| `integrations`      | GitHub App settings (`/platform/integrations/github`)              |

Shared infra: `components/ui`, `tables`, `entity`, `forms`, `layout`, `dialogs`, `lib/query`, `lib/i18n`, `lib/realtime`.

Brand tokens: `src/config/brand.ts`.

Media: never assemble MinIO/S3 URLs in the browser. Use `assertServiceMediaURL` in `src/lib/media/urls.ts`.

## Scripts

| Script              | Purpose                         |
| ------------------- | ------------------------------- |
| `pnpm dev`          | Next.js dev server              |
| `pnpm build`        | Production build                |
| `pnpm typecheck`    | `tsc --noEmit`                  |
| `pnpm lint`         | ESLint                          |
| `pnpm api:generate` | Regenerate OpenAPI client types |

`src/generated/api.d.ts` is generated. If it still lists deleted product paths, ignore unused ones and regenerate after the OpenAPI spec is trimmed.
