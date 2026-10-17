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

## Slices done (committed locally)

- Dealer: home (KPIs from /v1/reports/overview, recent services, quick actions), services list/detail/status transitions,
  create service (customer → vehicle → details, items added on the detail screen by search or barcode scan),
  customers list/detail, warranties list/detail with the public `/warranty/{code}` link (open + share).
- Warehouse: warehouse picker, continuous scan (POST /v1/mobile/warehouse/scan) with next actions,
  stock (products, units, locations, barcode history) for depo and bayi, jobs: transfers (create, scan lines,
  ship, receive/place, complete, cancel), counts (create, start, scan, complete, report, approve), stock entries
  (printed labels, with_existing), bin-to-bin moves; depo home with counters.
- Admin: home KPIs + services trend, network (tenant/organizations) list/detail, orders list/detail + simple
  transitions, reports catalog with generic renderers, approvals (orders, module requests, reclassifications).
- Measurement tab: list/detail in µm or mil, manual entry (POST /v1/mobile/measurements); SmartPTG BLE and
  plate recognition are placeholders behind flags (`features/measure/parts.ts`).
- Customer space: screens with empty states only (customer accounts cannot sign in on mobile yet).
- Settings: language (13, RTL restart for ar), theme, unit, API server, app version, logout.
- Sign-in: one box; e-mail → password (+TOTP); phone → WhatsApp code (PR #407).

## Backend PRs

- #406 Mobil API: panel uçlarının mobil aynası (/v1/mobile/panel/*) — open, not merged. The local backend runs from this branch.

- #407 Mobil API: telefonla giriş (identify + OTP doğrulama) — open, not merged, migration 000150.
- Local backend runs from `local/mobile-api-combined` (main + #406 + #407, never pushed) in
  `.claude/worktrees/mobile-api-local`; the app schema was generated from that branch's OpenAPI.

## Stubbed / gaps

- Customer and fleet accounts still cannot sign in on mobile (NO_PANEL_ACCESS); portal routes are not mirrored.
- Phone sign-in only for existing staff with a phone; locally the WhatsApp code is not readable (sandbox swallows it).
- No bootstrap endpoint; homes make 2-3 calls (dealer, warehouse, customer detail).
- Warehouse not built yet: order picking/preparation, distributor receipt placement, generate_new stock entries,
  label PDFs, warehouse/room/location setup (CRUD, generate, import), stock import, exports.
- Services: intake photos, contract signing, service PDF, warranty void, customer/vehicle editing.
- Warranty QR image (no QR lib in Expo Go; code + link shown).
- Warehouse context (`whid`) is device-side only.

### Backend check (04:00, local, PR #406 branch)

curl with the seed-demo accounts (password in `mobile/.demo-credentials`, gitignored):
login → organization-context → `/v1/mobile/panel/*`: dealer/center/distributor get 200 on
services, customers, warranties, orders, reports, tenant/organizations, measurements;
center_warehouse gets 200 on stock/warehouse and 403 on services (permissions work).
Login answers without an active organization, so the app now opens the last used
(or first) organization right after sign-in.
