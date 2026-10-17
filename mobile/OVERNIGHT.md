# Overnight build log (2026-10-17)

Goal: a working app in Expo Go by 09:00, against the local backend.

## Approach

- The app uses the existing panel endpoints through a new backend mirror:
  `/v1/mobile/panel/<path>` serves `/v1/<path>` for an allowlist of prefixes,
  mobile token only (backend PR "Mobil API: panel uçlarının mobil aynası").
  Screens call the typed panel path (`api.GET('/v1/services')`);
  `src/core/api/client.ts` rewrites it to the mirror.
- Warehouse uses the dedicated `/v1/mobile/warehouse/*` routes where they
  exist, the mirror for the rest.
- Local test: the app talks to Go directly (`EXPO_PUBLIC_API_URL=http://<LAN IP>:8080`
  in `mobile/.env.local`); in production it goes through the BFF (`https://<host>/api`),
  which already forwards `/api/v1/mobile/*`.

## Log

- MMKV replaced with expo-sqlite kv-store (Expo Go). Branch rebased on main, API schema regenerated.
- Panel mirror rewrite in the API client; `.env.example` documents the LAN setup.
- Menu rows open the built screens (customers, warranties, orders, reports).

## Backend PRs

- #406 Mobil API: panel uçlarının mobil aynası (/v1/mobile/panel/*) — open, not merged. The local backend runs from this branch.

## Stubbed / gaps

### Backend check (04:00, local, PR #406 branch)

curl with the seed-demo accounts (password in `mobile/.demo-credentials`, gitignored):
login → organization-context → `/v1/mobile/panel/*`: dealer/center/distributor get 200 on
services, customers, warranties, orders, reports, tenant/organizations, measurements;
center_warehouse gets 200 on stock/warehouse and 403 on services (permissions work).
Login answers without an active organization, so the app now opens the last used
(or first) organization right after sign-in.
