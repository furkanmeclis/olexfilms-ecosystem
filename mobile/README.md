# Olex Films mobile

One app for every role: customer, dealer, warehouse and admins (center and
distributors). Expo SDK 58, React Native 0.88, expo-router, Uniwind
(Tailwind 4), TanStack Query, i18next. It talks only to the mobile API of this
repo's backend (`/v1/mobile/*` in `docs/openapi.yaml`, `Mobile` tag); no
legacy API is used.

Screen plan, design system and speed budget: the "Olex Mobil Ekran Planı" page
(shared separately). Decisions so far are listed at the end of this file.

## Run

```bash
cd mobile
cp .env.example .env.local      # EXPO_PUBLIC_API_URL=<BFF origin>/api
pnpm install
pnpm ios                        # development build (Expo Go is not supported)
```

The app needs a development build: it uses native modules (MMKV, camera,
secure store, local authentication) that Expo Go does not ship.

## Commands

| Command | What it does |
|---|---|
| `pnpm typecheck` | `tsc --noEmit` |
| `pnpm lint` | `expo lint` |
| `pnpm api:generate` | Regenerates `src/core/api/schema.d.ts` from `../docs/openapi.yaml`. Run it whenever the OpenAPI file changes, like the web's `pnpm api:generate`. |
| `pnpm i18n:gen` | Regenerates `src/core/i18n/resources.ts` after adding a namespace JSON file |
| `pnpm i18n:check` | tr/en must match key for key; other languages may miss keys (falls back to en); `{{params}}` must match |
| `pnpm uniwind:types` | Regenerates `src/uniwind-types.d.ts` (Metro also does this on start) |
| `pnpm doctor` | `expo-doctor` |

## Structure

```
src/
  app/                    expo-router routes, one-line files
    (public)/             sign-in (one box: phone or e-mail), password, 2FA
    (app)/                signed in (Stack.Protected on the session)
      bayi/ depo/ admin/ musteri/   one space each: native tabs only
      spaces, search, settings, qr-login/[code], soon/[key]
    update-required.tsx   shown on 426
  core/                   no UI
    api/                  openapi-fetch client, generated schema, ApiError
    auth/                 tokens (SecureStore), session (zustand), device id
    i18n/                 13 languages, RTL, web error catalog
    spaces.ts             which spaces a session opens
    query.ts              TanStack Query, cache persisted in MMKV
    preferences.ts        theme, thickness unit (µm / mil)
  features/               screens and hooks, shared by every space
  ui/                     design system (tokens from the web theme)
```

Rules:

- A space only arranges tabs. Screens live in `features/` and are written once;
  what a user sees inside them depends on permissions, not on the space.
- Every screen opens with one request. If a screen needs three calls, the
  backend gets a summary endpoint for it.
- Cached data renders first, the network refreshes in the background
  (`staleTime` 30 s, cache kept 7 days in MMKV, cleared on a new app version).
- New UI text always goes through an i18n key, at least in tr and en.
- Icons are imported from `@/ui/icons` (one file per icon); the package root
  would ship every Lucide icon because Metro does not tree-shake.
- Colors only through theme classes (`bg-card`, `text-muted-foreground`, ...).
  Tokens in `src/global.css` mirror `frontend/src/styles/theme.css`.
- Logical spacing (`ms-`, `pe-`, `start-`) for RTL.

## Backend contract

Every request sends `X-Mobile-Api-Version: 1`, `X-App-Version`,
`Accept-Language` and `Authorization: Bearer` (except sign-in and refresh).

- Access tokens refresh 30 s before they expire; a 401 triggers one shared
  refresh and a single retry. A rejected refresh (`REFRESH_TOKEN_REUSED`,
  expiry) signs the device out.
- 426 (`UPDATE_REQUIRED` or `MOBILE_API_VERSION_UNSUPPORTED`) switches the app
  to the update screen with the store link from `data.store_urls`.
- Error messages come from `frontend/src/locales/<lang>/errors.json`
  (`codes.<ERROR_CODE>`), so web and mobile say the same thing.

Used today: `POST /v1/mobile/auth/login` (with `totp_code` on `MFA_REQUIRED`),
`/refresh`, `/me`, `/logout`, `/organization-context`,
`GET /v1/mobile/auth/qr/{code}` and `/approve`, `/reject`.

## Backend gaps

Needed before the matching screens can work; none of these exist in the mobile
realm yet.

1. **Phone sign-in** (`POST /v1/mobile/auth/identify` + OTP verify): the login
   box accepts phones, but the mobile API only has e-mail + password. Staff may
   also sign in by phone later.
2. **Customer accounts on mobile**: `/v1/mobile/auth/login` answers 403
   `NO_PANEL_ACCESS` for customer and fleet accounts, so the customer space is
   built but unreachable.
3. **`GET /v1/mobile/me/bootstrap`**: session, spaces, permissions, open
   modules, menu and home summary in one call (today: `/me` only).
4. **Mobile mirrors** of the measurement endpoints (body types list, part map,
   devices, list/detail, VIN), plate formats, global search, and the full
   warehouse set (setup included, the warehouse does not need the web).
5. **Warehouse context** (`whid` in the token): reserved for F1.

## Decisions (2026-10-16)

- One app: customer, dealer, warehouse, admins.
- Admin space: the same tabs for the center and distributors; the API scopes
  the data (center: whole network, distributor: its subtree).
- Warehouse: fully on mobile, no need to open the web.
- Measuring tab: for everyone who can create a service (`services.write`).
- Thickness unit: µm or mil, user preference; the backend stores µm.
- Speed budget: targets on the plan page; no CI budget check for now.
