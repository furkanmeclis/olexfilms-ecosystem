# Olexfilms mobile

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
npm install
npx expo prebuild --platform ios --clean   # ios/ and android/ are generated (CNG), never committed
npx expo run:ios                # development build on a simulator or device
```

Expo Go from the App Store only supports SDK 57, so this SDK 58 app needs a
development build. `pod install` downloads ~100 MB of React Native/Hermes
tarballs each time; point CocoaPods at the cached copies to skip that:

```bash
export RCT_USE_LOCAL_RN_DEP=1
export RCT_TESTONLY_RNCORE_TARBALL_PATH=~/Library/Caches/ReactNative/<react-native-core>-debug.tar.gz
export HERMES_ENGINE_TARBALL_PATH=~/Library/Caches/ReactNative/<hermes>-debug.tar.gz
```

Later native modules (BLE, plate recognition) also need the development build.

## Commands

| Command | What it does |
|---|---|
| `npm run typecheck` | `tsc --noEmit` |
| `npm run lint` | `expo lint` |
| `npm run api:generate` | Regenerates `src/core/api/schema.d.ts` from `../docs/openapi.yaml`. Run it whenever the OpenAPI file changes, like the web's `pnpm api:generate`. |
| `npm run i18n:gen` | Regenerates `src/core/i18n/resources.ts` after adding a namespace JSON file |
| `npm run i18n:check` | tr/en must match key for key; other languages may miss keys (falls back to en); `{{params}}` must match |
| `npm run uniwind:types` | Regenerates `src/uniwind-types.d.ts` (Metro also does this on start) |
| `npm run doctor` | `expo-doctor` |

## OTA updates

JS-only changes ship over the air with `expo-updates` + EAS Update (config in
`app.json` `updates`/`runtimeVersion`, build profiles in `eas.json`). Nothing runs in dev
builds or Expo Go; release builds check on launch and on each foreground (at most every 15 min),
download in the background and show a "new version ready" bar (restart or later; otherwise the
update applies on the next cold start). Settings > About shows channel, runtime and update id.

- **Runtime version** uses the `fingerprint` policy: a hash of everything native (dependencies,
  config plugins, `app.json` native fields). An update only reaches builds with the same
  fingerprint, so a native change can never land on an incompatible binary. It suits CNG
  (`ios/`/`android/` are generated, not committed) because the hash covers the config instead of files.
- The native launch check is off (`checkAutomatically: NEVER`); only `core/updates.ts` checks, so the banner is reliable.
- `.fingerprintignore` skips JS/TS and tests under `modules/**` so they don't change the runtime; native
  folders stay hashed. Run `eas update` only from a clean checkout (no local edits or untracked native
  files), or the fingerprint may differ from the build's.
- **Channel ↔ build profile:** `development` → `development`, `preview` → `preview`,
  `production` → `production`. A build only receives updates published to its own channel.
- **Publish** (JS/asset change only):
  `npx eas-cli@latest update --channel preview -m "what changed"`, then `--channel production` once verified.
  Run it from `mobile/` after `eas login`.
- **Needs a new build** (not OTA): any native dependency added/upgraded, a config plugin or
  native `app.json` field change, an Expo SDK upgrade. The fingerprint changes, so the old
  builds simply get no update; run `eas build --profile <profile>` and publish updates on top of it.
  Check before publishing: `npx @expo/fingerprint@latest fingerprint:generate`.
- **Rollback:** `npx eas-cli@latest update:republish --channel production --group <previous-group-id>`
  (list groups with `eas update:list`), or `eas update:rollback-to-embedded` to fall back to the
  JS bundled in the binary.
- The backend's 426 "update required" gate is separate: it forces a store update for old
  *binaries*; OTA never replaces it.

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
    query.ts              TanStack Query, cache persisted in expo-sqlite kv-store
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
  (`staleTime` 30 s, cache kept 7 days on disk, cleared on a new app version).
- New UI text always goes through an i18n key, at least in tr and en.
- Icons are imported from `@/ui/icons` (one file per icon); the package root
  would ship every Lucide icon because Metro does not tree-shake.
- Colors only through theme classes (`bg-card`, `text-muted-foreground`, ...).
  Tokens in `src/global.css` mirror `frontend/src/styles/theme.css`.
- Logical spacing (`ms-`, `pe-`, `start-`) for RTL.
- Native permission texts (iOS `NS*UsageDescription`) are translated only in
  `assets/locales/{tr,en}.json` (app.json `locales`). Every key goes into both
  files: a key missing from `en.json` shows the plugin's Turkish base text to
  English users (en.lproj is the fallback for every other language).
- Optional native capabilities live in local Expo modules under `modules/`
  (e.g. `plate-ocr`) and are loaded with `requireOptionalNativeModule`, so
  builds without them (Expo Go, web) hide the feature instead of crashing.

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

Status 2026-10-17: merged PRs #406 (panel mirror `/v1/mobile/panel/*`), #407 (phone sign-in), #408 (customer
sessions + portal mirror `/v1/mobile/portal/*`) close gaps 1, 2 and most of 4. The list below is the original.

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
