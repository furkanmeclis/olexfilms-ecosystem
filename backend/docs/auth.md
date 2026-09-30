# Auth & RBAC

Users are global. Permissions come from assigned roles (union). **Organizations** (tenants) add a second access layer: membership in `organization_members` and organization `status` / `access_ends_at` for business screens under `/t/{slug}`.

## Roles

| Role | Type | Notes |
|------|------|-------|
| `super_admin` | system (`is_system=true`) | All permissions; cannot delete slug or demote last member |
| `organization_user` | system (`is_system=true`) | `auth.session`, `notifications.read` — assigned to organization owners/staff |
| Custom roles | platform-managed | Created via `/v1/platform/roles`; permissions from DB catalog only |

`users.is_super_admin` column is removed. JWT and `/me` still expose `is_super_admin` when the user has the `super_admin` role.

## Permissions (seed)

| Permission | Purpose |
|------------|---------|
| `platform.users.read` / `.write` / `.export` / `.import` | Platform user CRUD + I/O |
| `platform.users.impersonate` | Impersonate another user from platform admin (step-up) |
| `platform.users.bulk.disable` / `.bulk.enable` | User bulk actions |
| `platform.roles.read` / `.write` / `.export` / `.import` | Role CRUD + I/O |
| `platform.roles.bulk.delete` | Role bulk delete |
| `platform.bulk.read` | Bulk job list / rollback |
| `platform.notifications.read` / `.read_all` / `.export` | Platform notification log (`.read_all` unlocks other users' rows) |
| `platform.settings.read` / `.write` | Letterhead / export branding |
| `platform.activity.read` | Audit log |
| `platform.imports.read` / `platform.exports.read` | I/O job lists |
| `platform.storage.read` / `.write` | S3/MinIO file browser |
| `platform.logs.read` / `.write` | Application logs + purge |
| `platform.access.read` / `.write` | Step-up policy |
| `platform.auth.settings.read` / `.write` | Registration policy (also requires super admin) |
| `platform.integrations.github.read` / `.write` | GitHub App settings (also requires super admin) |
| `platform.integrations.google.read` / `.write` | Google OAuth settings (also requires super admin) |
| `platform.integrations.facebook.read` / `.write` | Facebook OAuth settings (also requires super admin) |
| `platform.integrations.apple.read` / `.write` | Apple OAuth settings (also requires super admin) |
| `platform.organizations.read` / `.write` | Organization list, access management, logo, staff assignment |
| `auth.session` | Sign-in / tenant access |
| `tenant.finance.read` | Tenant finance read (accounts, categories, transactions, summary) |
| `tenant.finance.write` | Tenant finance write (owner only at HTTP layer) |
| `tenant.finance.export` | Tenant finance export (letterhead from organization) |
| `tenant.finance.import` | Tenant finance import (owner; accounts and categories) |
| `tenant.settings.read` / `.write` | Tenant export letterhead (owner) |
| `tenant.imports.read` | Tenant import jobs (owner) |
| `notifications.read` / `notifications.manage` | User notifications |

Super admin bypasses permission checks in `HasPermission`. Most platform HTTP routes use `RequirePermission` only. Auth settings and OAuth integration routes also use `RequireSuperAdmin`.

## Login & registration

- Password / passkey / OAuth availability is controlled by `auth_settings` + per-provider flags (`GET /v1/app/config` → `auth_methods`).
- Global `registration_enabled` must be on, plus the method’s `register` flag, for platform self-registration (`/platform/register`).
- **Business self-register** uses `POST /v1/public/organizations/register` (no platform permission). Creates owner membership and 14-day trial access.
- Password register assigns the configured **default role** when set; otherwise no roles.
- **OAuth**: linked accounts can sign in when `login` is enabled. Unlinked accounts may self-register only when register is allowed for that provider.
- **Tenant login**: `POST /v1/auth/login` accepts optional `organization_slug`. When present, the user must be a member and the organization must not be suspended or past `access_ends_at`. Success adds `oid` (organization UUID) to the access token.
- **Switch tenant context**: `POST /v1/auth/organization-context` with `{ organization_slug }` re-issues tokens with `oid` for an already authenticated user (e.g. after platform login before visiting `/t/{slug}`).
- Secrets for OAuth apps are encrypted with `APP_ENCRYPTION_KEY`.

## Linked identities

| Method | Path | Auth |
|--------|------|------|
| GET | `/v1/auth/identities` | Bearer |
| DELETE | `/v1/auth/identities/{provider}` | Bearer (`github`, `google`, `facebook`, `apple`) |

GitHub: `GET/PATCH /v1/platform/integrations/github`. Google / Facebook / Apple: `/v1/platform/integrations/{provider}`. Auth policy: `GET/PATCH /v1/platform/auth/settings`.

Internal NextAuth adapter routes (not in OpenAPI): `/v1/internal/auth/oauth/{provider}`, `/v1/internal/auth/accounts`, `POST /v1/internal/auth/users`.

## JWT

HS256 claims: `sub`, `roles`, `is_super_admin`, `exp`, optional `imp` (impersonator user UUID), optional `sid` (refresh-session UUID), optional `oid` (organization UUID when tenant context is active). No workspace `wid`.

## Sessions

Refresh tokens are listed as devices. `sid` on the access token marks the current row.

| Method | Path | Auth |
|--------|------|------|
| GET | `/v1/auth/sessions` | Bearer |
| DELETE | `/v1/auth/sessions/{uuid}` | Bearer (own session) |
| POST | `/v1/auth/sessions/revoke-others` | Bearer (keeps current `sid`) |

## Rate limits

Redis INCR, 15-minute window, fail-open if Redis is down. `429 RATE_LIMITED` + `Retry-After`.

| Endpoint | Key | Max |
|----------|-----|-----|
| Login | IP + email | 10 |
| Register | IP | 5 |
| Forgot password | IP + email | 5 |
| Reset password | IP | 10 |
| Verify email (`/auth/email/verify`) | IP | 10 |
| Verification email request | user | 5 |
| Business register (`/public/organizations/register`) | IP | 5 |

The BFF forwards the browser IP as `X-Forwarded-For`; the Go API must not be reachable directly from clients or the header could be spoofed.

## `/v1/auth/me`

Returns `user`, `roles[]`, `permissions[]`, `organizations[]` (tenant memberships), self-service links, and realtime hints.

Each `organizations[]` item includes `uuid`, `slug`, `name`, `role` (`owner` | `staff`), optional `logo_url`, `status`, and optional `access_ends_at`.

## Organizations (platform)

| Method | Path | Permission |
|--------|------|------------|
| GET | `/v1/platform/organizations/meta` | `platform.organizations.read` |
| GET | `/v1/platform/organizations` | `platform.organizations.read` |
| POST | `/v1/platform/organizations` | `platform.organizations.write` — body: `name`, contact fields, `owner_user_uuid` |
| GET | `/v1/platform/organizations/{uuid}` | `platform.organizations.read` — includes `members[]` |
| PATCH | `/v1/platform/organizations/{uuid}` | `platform.organizations.write` — `status`, `plan_code`, `access_ends_at`, contact fields |
| PUT | `/v1/platform/organizations/{uuid}/logo` | `platform.organizations.write` |
| DELETE | `/v1/platform/organizations/{uuid}/logo` | `platform.organizations.write` |
| POST | `/v1/platform/organizations/{uuid}/members` | `platform.organizations.write` — body: `user_uuid`, optional `role` |

Public (no auth):

| Method | Path | Notes |
|--------|------|-------|
| POST | `/v1/public/organizations/register` | Business signup + owner user + tokens |
| GET | `/v1/public/organizations/by-slug/{slug}` | Login branding; `access_ok` flag |
| GET | `/v1/public/organizations/logo/{uuid}` | Logo stream |

Login error codes for tenant context: `NO_TENANT_MEMBERSHIP`, `ORGANIZATION_ACCESS_EXPIRED`.

## Platform users

| Method | Path | Permission |
|--------|------|------------|
| GET | `/v1/platform/users/meta` | `platform.users.read` |
| GET | `/v1/platform/users` | `platform.users.read` — query: `limit`,`offset`,`q`,`sort`,`status`,`role` |
| POST | `/v1/platform/users` | `platform.users.write` — body includes `role_uuids[]` |
| GET | `/v1/platform/users/{uuid}` | `platform.users.read` — includes `roles[]` |
| PATCH | `/v1/platform/users/{uuid}` | `platform.users.write` — optional `role_uuids[]` |
| POST | `/v1/platform/users/{uuid}/password` | `platform.users.write` |
| POST | `/v1/platform/users/{uuid}/impersonate` | `platform.users.impersonate` (step-up) |
| POST | `/v1/auth/impersonation/stop` | Bearer (active impersonation session) |

Last super admin cannot be demoted via role removal or disable.

## Platform roles

| Method | Path | Permission |
|--------|------|------------|
| GET | `/v1/platform/roles/meta` | `platform.roles.read` |
| GET | `/v1/platform/roles` | `platform.roles.read` |
| POST | `/v1/platform/roles` | `platform.roles.write` |
| GET | `/v1/platform/roles/{uuid}` | `platform.roles.read` |
| PATCH | `/v1/platform/roles/{uuid}` | `platform.roles.write` — system role slug immutable |
| DELETE | `/v1/platform/roles/{uuid}` | `platform.roles.write` — system roles → `409` |
| GET | `/v1/platform/permissions` | `platform.roles.read` — read-only catalog |

New permission slugs are added via migrations/seeds only (no permission CRUD API).

## CLI

`make create-super-admin` upserts the user and assigns the `super_admin` role.
