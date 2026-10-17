# Mobil premium arayüz: uygulama planı

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** iOS'ta Apple'a yakın premium görünüm: anlamsal ikon sistemi (SF Symbols / Lucide), eksik ikon ve placeholder'ların doldurulması, müşteriye yönelik animasyonlu tanıtım ve ülke seçicili, ikiye ayrılmış giriş.

**Architecture:** `src/ui/icons.ts` kavram adı → `{ sf, lucide }` sözlüğü olur; `Icon` iOS'ta `expo-symbols` `SymbolView`, Android'de Lucide çizer. Tanıtım `src/features/onboarding/` altında, `(public)/index` tanıtım görülmediyse onu, görüldüyse giriş ekranını gösterir. Giriş tek `SignInScreen` (mod: `phone` | `email`); telefon modunda `libphonenumber-js` ile ülkeye göre E.164.

**Tech Stack:** Expo SDK 58, expo-router (Stack.Protected), expo-symbols, expo-glass-effect, react-native-reanimated 4, uniwind, i18next, libphonenumber-js, Node 24 `node --test` (saf modüller için).

**Spec:** `docs/superpowers/specs/2026-10-17-mobile-premium-ui-design.md`

## Global Constraints

- Paket eklemek gerekirse yalnızca `npx expo install`; yeni native bağımlılık eklenmez (kurulu: expo-symbols, expo-glass-effect, reanimated).
- `ios/`, `android/` elle düzenlenmez, commit edilmez.
- Her yeni UI metni i18n anahtarı; tr ve en anahtar anahtar eşit (`npm run i18n:check`); yeni namespace sonrası `npm run i18n:gen`.
- Renkler yalnızca tema token'larından (`tone` / `useCSSVariable('--color-…')`); renk paleti değişmez.
- Tanıtım metinlerinde garanti süresi/kapsamı için rakam yok.
- Animasyon: ease-out, 300–600 ms; zıplama/yay (spring bounce), konfeti, maskot yok; `useReducedMotion()` true ise yalnızca opaklık geçişi.
- Cam: `GlassView` yalnızca `Platform.OS === 'ios' && isGlassEffectAPIAvailable() && isLiquidGlassAvailable()` iken; aksi halde `bg-card` düz yüzey.
- Öncelik iOS; Android'de Lucide (`strokeWidth` 1.75), düz yüzey, kırık görünüm yok.
- Kimlik doğrulama API'leri (`identify`, password, totp, phone OTP) değişmez.
- Her görev sonunda: `npm run lint`, `npm run typecheck`, `npm run i18n:check` (mobile/ içinde) yeşil.
- Commit: `TEC-<no>: …` + `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. İki PR: PR1 (Görev 1–4) CI yeşilse auto-merge; PR2 (Görev 5–9) ürün sahibi ekran görüntülerini onaylamadan merge edilmez.

## Review Focus

1. Ülke seçici TR'deyken `+44 7400 123456` yapıştırılırsa yapıştırılan ülke (GB) kazanır, E.164 `+447400123456` olur. → Görev 5 testi.
2. `0532 123 45 67`, `532-123-45-67`, `(532) 123 4567` TR'de geçerli → `+905321234567`. → Görev 5 testi.
3. iOS "Hareketi Azalt" açık → tanıtımda paralaks/kayma yok, yalnızca opaklık. → Görev 7 doğrulama adımı.
4. Liquid Glass yok (iOS < 26 ya da API yok) → çökme yok, düz kart. → Görev 7 doğrulama adımı.
5. Tanıtım ortasında uygulama kapatılırsa bir sonraki açılışta tanıtım yeniden gelir; çıkış yapınca (sign out) tanıtım geri gelmez. → Görev 6 + Görev 9 doğrulama adımı.

---

## PR1: ikon sistemi, eksik ikonlar, placeholder'lar

Başlarken Linear'da (P-TEC-4, takım Technowide-furkan) iş aç, dal `feat/tec-<no>-mobile-icon-system`, `docs/mobile-premium-ui-design` dalının üzerinden açılır; böylece spec ve plan PR1 ile main'e girer.

### Görev 1: Anlamsal ikon sözlüğü ve yeni `Icon`

**Files:**
- Modify: `mobile/src/ui/icons.ts` (Lucide yeniden-export listesi → sözlük)
- Modify: `mobile/src/ui/icon.tsx`

**Interfaces:**
- Produces: `export type IconName` (kavram adları birliği), `export const ICONS: Record<IconName, { sf: SFSymbol; lucide: LucideIcon }>` (`SFSymbol` tipi `expo-symbols`'tan), `Icon({ name, tone = 'foreground', size = 22, weight = 'regular' }: { name: IconName; tone?: Tone; size?: number; weight?: 'regular' | 'medium' | 'semibold' })`.
- Geçiş için Görev 2 bitene kadar `as?: LucideIcon` prop'u da kabul edilir; Görev 2 kaldırır.

- [ ] **Step 1:** `IconName`'i şu kavramları kapsayacak şekilde tanımla (bugünkü ~50 Lucide kullanımının kavramları + menü + giriş): `back, forward, chevronDown, close, check, add, delete, share, externalLink, download, search, scan, barcode, qr, camera, flashlight, keyboard, customer, customers, vehicle, service, warranty, warrantyClaim, appointment, calendar, order, cart, stock, package, packageIn, packageCheck, warehouse, truck, location, report, chart, accounting, task, announcement, lead, measure, gauge, tools, building, network, spaces, settings, phone, mail, password, shield, device, bluetooth, offline, info, alert, blocked, clock, target, inbox, globe`. Her biri için iOS SF Symbol adı ve mevcut Lucide karşılığı. SF adları `expo-symbols` SDK 58 tipinin kabul ettiği adlar olmalı (yanlış ad = typecheck hatası).
- [ ] **Step 2:** `Icon`: `Platform.OS === 'ios'` → `<SymbolView name={ICONS[name].sf} size={size} tintColor={color} weight={weight} fallback={<Lucide …/>} />`; diğer platformlar → Lucide `size`, `color`, `strokeWidth={1.75}`. RTL'de `back/forward` aynalanır (bugünkü `ChevronForward` davranışını koru).
- [ ] **Step 3:** `npm run typecheck && npm run lint` → yeşil.
- [ ] **Step 4:** Commit `TEC-<no>: Anlamsal ikon sözlüğü (SF Symbols / Lucide)`.

### Görev 2: Tüm ikon kullanımlarını sözlüğe taşı

**Files:**
- Modify: `mobile/src/ui/empty-state.tsx` (`icon: IconName`), `mobile/src/ui/list.tsx` (`ListRow`'a `icon?: IconName` → `leading` yoksa sol tarafta, `tone="primary"`, size 20), `mobile/src/ui/icon.tsx` (`as` prop'unu kaldır)
- Modify: `<Icon as={…}>` (~80) ve `icon={…}` (~48) kullanan tüm `mobile/src/features/**` dosyaları

**Interfaces:**
- Consumes: Görev 1 `IconName`, `Icon`.
- Produces: `ListRow` prop `icon?: IconName`; `EmptyState` prop `icon: IconName`.

- [ ] **Step 1:** Her kullanımı kavram adıyla değiştir (`as={Users}` → `name="customers"`).
- [ ] **Step 2:** Doğrula: `grep -rn "as={" src | grep -c "<Icon"` → `0`; `grep -rln "lucide-react-native" src` → yalnızca `src/ui/icons.ts`.
- [ ] **Step 3:** `npm run typecheck && npm run lint` → yeşil.
- [ ] **Step 4:** Commit `TEC-<no>: İkon kullanımları anlamsal adlara taşındı`.

### Görev 3: Eksik ikonlar (menü, depo yeni giriş)

**Files:**
- Modify: `mobile/src/features/menu/menu-screen.tsx` (`Item`'a `icon: IconName`; tüm `ITEMS` + arama/alanlar/ayarlar satırları ikonlu)
- Modify: `mobile/src/features/warehouse/entry-new-screen.tsx`

- [ ] **Step 1:** Menü eşlemesi: customers→`customers`, warranties→`warranty`, appointments→`appointment`, stock→`stock`, orders→`order`, warranty_claims→`warrantyClaim`, leads→`lead`, reports→`report`, accounting→`accounting`, tasks→`task`, announcements→`announcement`, search→`search`, spaces→`spaces`, settings→`settings`. Satırlar `ListRow icon=…`.
- [ ] **Step 2:** Depo yeni giriş ekranında alan/eylem ikonları (`scan`, `barcode`, `packageIn`, `warehouse`).
- [ ] **Step 3:** Ayarlar ekranı satırları da ikon alır (tema, birim, dil, alan değiştir, çıkış → `settings` bölümüne uygun adlar).
- [ ] **Step 4:** `npm run typecheck && npm run lint` → yeşil; commit `TEC-<no>: Menü, ayarlar ve depo girişine ikonlar`.

### Görev 4: Placeholder'lar + kalıcı kontrol

**Files:**
- Create: `mobile/scripts/check-placeholders.mjs`
- Modify: `mobile/package.json` (script `"placeholders:check": "node scripts/check-placeholders.mjs"`), `.github/workflows/ci.yml` (`mobile` işine `npm run placeholders:check` adımı, i18n adımından sonra)
- Modify: `<Input` içeren tüm `mobile/src/features/**` dosyaları, ilgili `mobile/src/core/i18n/locales/{tr,en}/*.json`

- [ ] **Step 1:** Script yaz: `src/**/*.tsx` içinde her `<Input` açılışından eşleşen `/>` ya da `>`'ya kadar olan bloğu tara; `placeholder=` yoksa `dosya:satır` yazdır ve çıkış kodu 1. Bugün yaklaşık 42 ihlal bulmalı (46 alanın ~4'ünde placeholder var).
- [ ] **Step 2:** `npm run placeholders:check` → FAIL (ihlaller listelenir).
- [ ] **Step 3:** Her alana anlamlı placeholder (i18n anahtarı, örn. `placeholder_email: "ornek@firma.com"`, `placeholder_phone: "532 123 45 67"`, `placeholder_plate: "34 ABC 123"`, `placeholder_vin: "17 karakter şasi no"`; tutar/miktar alanlarında birimli örnek). tr + en.
- [ ] **Step 4:** `npm run placeholders:check` → `0 ihlal`, çıkış 0; `npm run i18n:check` → `0 errors`.
- [ ] **Step 5:** Commit `TEC-<no>: Tüm form alanlarına placeholder + CI kontrolü`.

**PR1 kapanışı:** push, PR aç (kabul kriterleri: ikon sözlüğü, ikonsuz satır yok, placeholder kontrolü), `gh pr merge --auto --squash`. Telefonda (Metro + mevcut dev build, `EXPO_PUBLIC_API_URL=https://olexfilms.technowide.tr/api`) menü, ayarlar, bir liste ve bir form ekranının görüntüsünü al, PR'a ekle. Merge sonrası Linear işi Done.

---

## PR2: tanıtım ve giriş

Linear'da ikinci iş, dal `feat/tec-<no>-mobile-onboarding-sign-in`, PR1 merge'ünden sonraki `origin/main`'den.

### Görev 5: Ülkeli telefon ayrıştırma (+ test altyapısı)

**Files:**
- Create: `mobile/src/features/auth/phone-country.ts`, `mobile/src/features/auth/phone-country.test.ts`
- Modify: `mobile/tsconfig.json` (`"exclude": ["node_modules", "**/*.test.ts"]`), `mobile/package.json` (`"test": "node --test \"src/**/*.test.ts\""`), `.github/workflows/ci.yml` (`mobile` işine `npm test`, typecheck'ten sonra)

**Interfaces:**
- Produces: `parsePhone(raw: string, country: CountryCode): string | null` (E.164 ya da null); `listCountries(locale: string): Country[]`; `type Country = { code: CountryCode; name: string; dial: string; flag: string }`; `DEFAULT_COUNTRY: CountryCode = 'TR'`. `CountryCode` `libphonenumber-js`'ten. Bu dosya yalnızca `libphonenumber-js/min` import eder (node testinin çalışması için `@/` alias'ı yok).

- [ ] **Step 1: Failing test** (`node:test`, `node:assert/strict`, import `./phone-country.ts`):
  - `parsePhone('0532 123 45 67','TR') === '+905321234567'`; `'532-123-45-67'` ve `'(532) 123 4567'` aynı sonuç.
  - `parsePhone('+44 7400 123456','TR') === '+447400123456'`.
  - `parsePhone('030 1234567','DE') === '+49301234567'`.
  - `parsePhone('123','TR') === null`; `parsePhone('','TR') === null`.
  - `listCountries('tr')[0].code === 'TR'`; `listCountries('tr').find(c => c.code === 'DE')!.name === 'Almanya'`; `dial === '+49'`; `flag === '🇩🇪'`; listede kod tekrarı yok.
- [ ] **Step 2:** `npm test` → FAIL (modül yok).
- [ ] **Step 3:** Uygula: `parsePhoneNumberFromString(raw, country)`, `isValid()`; `listCountries`: `getCountries()`, `getCountryCallingCode`, `new Intl.DisplayNames([locale], { type: 'region' })`, bölge bayrağı regional-indicator harfleri; `DEFAULT_COUNTRY` başta, kalanı `localeCompare(locale)` ile ada göre sıralı.
- [ ] **Step 4:** `npm test` → PASS; `npm run typecheck` → yeşil.
- [ ] **Step 5:** Commit `TEC-<no>: Ülkeye göre telefon ayrıştırma + node test altyapısı`.

### Görev 6: Tanıtım "görüldü" durumu

**Files:**
- Create: `mobile/src/features/onboarding/onboarding-state.ts`

**Interfaces:**
- Produces: `useOnboarding` (zustand): `{ seen: boolean; markSeen(): void }`; anahtar `'onboarding.v1'`, `readJSON/writeJSON` (`@/core/storage`). `signOut` bu anahtarı silmez (bugün yalnızca `ME_KEY` siliniyor; öyle kalmalı).

- [ ] **Step 1:** Uygula (`preferences.ts` kalıbı). `markSeen` yalnızca "Geç" ya da son sayfa eylemiyle çağrılır, sayfa açılışında değil.
- [ ] **Step 2:** `npm run typecheck` → yeşil; commit `TEC-<no>: Tanıtım görüldü durumu`.

### Görev 7: Tanıtım ekranı

**Files:**
- Create: `mobile/src/features/onboarding/onboarding-screen.tsx`, `mobile/src/features/onboarding/onboarding-page.tsx`, `mobile/src/core/i18n/locales/{tr,en}/onboarding.json` (diğer diller boş bırakılır → en)
- Modify: `mobile/src/core/i18n/resources.ts` (`npm run i18n:gen` ile)

**Interfaces:**
- Consumes: Görev 1 `Icon`/`ICONS`, Görev 6 `useOnboarding`.
- Produces: `OnboardingScreen({ mode }: { mode: 'first-run' | 'replay' })`. `first-run` son sayfa: birincil `Button` "Telefon numaramla devam et" → `markSeen()` + `router.replace({ pathname: '/sign-in', params: { mode: 'phone' } })`; ikincil metin düğmesi "Personel girişi" → aynı, `mode: 'email'`. `replay` son sayfa: tek "Tamam" → `router.back()`. "Geç" (her sayfada sağ üst, `first-run`'da) → `markSeen()` + phone moduna.

- [ ] **Step 1:** i18n `onboarding` namespace: `skip, next, done, cta_phone, cta_staff, p1_title, p1_body, p2_title, p2_body, p3_title, p3_body`. İçerik (tr; en karşılığı yazılır): s1 PPF ve cam filmi, aracın yüzeyini koruyan şeffaf katman; s2 yalnızca yetkili Olexfilms bayileri, ölçümle başlayan araca özel uygulama; s3 dijital garanti: araca, parçalara ve süreye bağlı kayıt, QR ile doğrulama, araç satılınca devir, kâğıt kart kaybolsa da telefonda. Rakam yok.
- [ ] **Step 2:** `OnboardingPage`: büyük SF Symbol kompozisyonu (s1 `vehicle` + `shield`, s2 `measure`/`tools` + onay rozeti, s3 `warranty` + `qr`), cam ya da `bg-card` yüzey üstünde; başlık (Outfit, `title` varyantı) + gövde (`callout`, `muted`).
- [ ] **Step 3:** `OnboardingScreen`: yatay `Animated.ScrollView` + `pagingEnabled`, `useAnimatedScrollHandler` ile sayfa ilerlemesi; görsel için hafif paralaks (translateX ±24 px) ve opaklık; nokta göstergesi (aktif nokta genişler, 300 ms). `useReducedMotion()` → paralaks yok. Güvenli alan, koyu/açık tema.
- [ ] **Step 4:** `npm run i18n:gen && npm run i18n:check && npm run typecheck && npm run lint` → yeşil.
- [ ] **Step 5:** Commit `TEC-<no>: Müşteri tanıtım ekranı`.

### Görev 8: Ayrılmış giriş ekranı ve ülke seçici

**Files:**
- Create: `mobile/src/features/auth/sign-in-screen.tsx`, `mobile/src/features/auth/country-picker.tsx`, `mobile/src/app/(public)/sign-in.tsx`
- Delete: `mobile/src/features/auth/identify-screen.tsx` (mantığı `sign-in-screen`'e taşınır); `parseIdentifier` kullanılmıyorsa `identifier.ts` de
- Modify: `mobile/src/features/auth/{password,phone,totp}-screen.tsx` (ikon, placeholder, başlık düzeni), `mobile/src/core/i18n/locales/{tr,en}/auth.json`

**Interfaces:**
- Consumes: Görev 5 `parsePhone`, `listCountries`, `DEFAULT_COUNTRY`; mevcut `identify(e164)`, `usePendingLogin`.
- Produces: `SignInScreen({ mode }: { mode: 'phone' | 'email' })` (route parametresinden); `CountryPicker({ value, onChange }: { value: CountryCode; onChange(c: CountryCode): void })` — dokununca sayfa (`formSheet`/modal) açılır, arama alanı + `FlashList`.

- [ ] **Step 1:** Telefon modu: `CountryPicker` (bayrak + `+90`) + numara alanı (`keyboardType="phone-pad"`, `textContentType="telephoneNumber"`, placeholder ülkeye göre örnek: `getExampleNumber` yoksa sabit `532 123 45 67`), "Devam" → `parsePhone` → `identify` → `/phone` (bugünkü parametrelerle). Geçersizse alan altı hata.
- [ ] **Step 2:** E-posta modu: e-posta alanı → bugünkü gibi `usePendingLogin.set({ mode: 'password', … })` + `/password`. `DevDemoAccounts` yalnızca bu modda.
- [ ] **Step 3:** Her modda altta geçiş bağlantısı: "E-posta ile giriş yap" / "Telefonla giriş yap" (`router.setParams({ mode })`).
- [ ] **Step 4:** Görünüm: üstte logo, SF Symbol'lü alan başlıkları, cam/düz kart içinde form, büyük birincil düğme; password/phone/totp ekranlarına aynı dil (ikon + placeholder).
- [ ] **Step 5:** `npm test && npm run i18n:check && npm run typecheck && npm run lint && npm run placeholders:check` → yeşil; commit `TEC-<no>: Telefon/e-posta olarak ayrılmış giriş + ülke seçici`.

### Görev 9: Yönlendirme, yeniden izleme, doğrulama

**Files:**
- Modify: `mobile/src/app/(public)/index.tsx` (`seen ? <Redirect href={{ pathname: '/sign-in', params: { mode: 'phone' } }} /> : <OnboardingScreen mode="first-run" />`), `mobile/src/app/(public)/_layout.tsx` (`sign-in` ekranı başlıksız)
- Create: `mobile/src/app/(app)/intro.tsx` (`<OnboardingScreen mode="replay" />`, modal sunum)
- Modify: `mobile/src/features/settings/*` (ayarlar listesine "Tanıtımı izle" satırı → `/intro`), `settings.json` tr/en

- [ ] **Step 1:** Uygula; `npm run typecheck && npm run lint && npm run i18n:check` → yeşil.
- [ ] **Step 2: Cihaz doğrulaması** (iPhone, dev build + Metro, demo API). Uygulamayı silip yeniden kur (`npx expo run:ios --device <UDID>`, önbellek env'leri ile) ve kontrol et:
  - İlk açılış tanıtım; "Geç" → telefonla giriş; geri dönüp yeniden açınca tanıtım yok.
  - Tanıtımın 2. sayfasında uygulamayı kapat → yeniden açınca tanıtım tekrar geliyor (Review Focus 5).
  - Telefonla giriş → çıkış yap → tanıtım gelmiyor, giriş ekranı geliyor (Review Focus 5).
  - Ayarlar > Hareket > Hareketi Azalt açık → paralaks yok (Review Focus 3).
  - Ülke seçicide Almanya, `030 1234567` → identify isteği `+49301234567` ile gidiyor (Metro log / ağ).
  - Ayarlar > "Tanıtımı izle" → son sayfada "Tamam".
  - Ekran görüntüleri: 3 tanıtım sayfası, telefon girişi, ülke seçici, e-posta girişi (açık + koyu).
- [ ] **Step 3:** Android emülatörde (varsa) tanıtım ve giriş için bir göz kontrolü; yoksa PR'da "Android görsel kontrol yapılmadı" diye belirt.
- [ ] **Step 4:** Commit; push; PR aç, ekran görüntülerini ekle. **Auto-merge açma**; ürün sahibine görüntüleri gönder (SendUserFile) ve tanıtım metni onayını iste. Onay + CI yeşil → merge, Linear Done.
