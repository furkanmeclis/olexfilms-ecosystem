# Handoff: mobil premium arayüz (2026-10-17)

Kaynak: `docs/superpowers/specs/2026-10-17-mobile-premium-ui-design.md` (spec) ve `docs/superpowers/plans/2026-10-17-mobile-premium-ui.md` (plan). Uygulama yöntemi: subagent-driven (her göreve ayrı uygulayıcı + görev incelemesi).

## Durum

- **PR1 (TEC-778, Görev 1–4) bitti** ve bu PR ile main'e giriyor. İçerik: anlamsal ikon sözlüğü (`mobile/src/ui/icons.ts`, iOS SF Symbols / diğer Lucide), tüm ikon kullanımlarının taşınması, menü/ayarlar/depo girişi ikonları, tüm `<Input>`'lara i18n placeholder + CI `placeholders:check`.
- Telefonda (iPhone 16 Pro, dev build + Metro, demo API) paket yüklendi, ürün sahibi gördü ve PR'ı istedi.
- **Yapılmadı:** PR1 için tüm dal üzerinde final kod incelemesi (ürün sahibi cihaz için durdurdu, ardından PR istedi). CI kapısı geçerli; bir sonraki oturum isterse dal üzerinde final inceleme koşturabilir.
- **PR2 (Görev 5–9) başlamadı:** ülke seçicili telefon ayrıştırma + node test altyapısı, tanıtım "görüldü" durumu, tanıtım ekranı, ayrılmış giriş, yönlendirme/yeniden izleme/cihaz doğrulaması. Ürün sahibi bir sonraki aşamaya geçmemeyi istedi.

## Sonraki adım (PR2)

1. PR1 merge olduktan sonra `origin/main`'den `feat/tec-<no>-mobile-onboarding-sign-in` dalı; Linear'da yeni iş (P-TEC-4, takım Technowide-furkan).
2. Planın Görev 5–9'unu sırayla uygula. Görev 5 tek TDD görevi (`node --test`, Node 24 tip ayıklama; `@/` alias'ı kullanmayan saf modül).
3. PR2 **auto-merge olmadan** açılır; ürün sahibi tanıtım metinlerini ve ekran görüntülerini onaylamadan merge edilmez.

## Verdiğim kararlar (ürün sahibinin onayı yok, gözden geçirin)

- Görev 8, `identify-screen.tsx`'i silerken `(public)/index.tsx`'i geçici olarak `/sign-in?mode=phone`'a yönlendirir; Görev 9 tanıtım kapısını ekler. Yanlışsa: tek satırlık geçici kod.
- Görev 1–4 ve 7–9 için birim testi yok (mobilde RN bileşen test altyapısı yok); doğrulama typecheck/lint/i18n/placeholder + cihaz. Yanlışsa: görsel regresyonlar yalnızca cihazda yakalanır.
- Ayarlarda tek seçimli satırlara (dil, tema, birim) ikon konmadı; yalnızca gezinme/eylem satırları ikonlu (spec'in "premium, gürültü değil" hedefi plan metnine üstün tutuldu). Yanlışsa: üç satıra ikon geri eklemek.
- `globe` ve `measure` kavramları şu an kullanılmasa da sözlükte bırakıldı (plan listesinde; `measure` tanıtımda kullanılacak).

## Ertelenen küçük bulgular

- `on-media` ikon rengi `#ffffff` sabit (kamera üstü); token'a taşınabilir.
- Bazı SF Symbol'ler (checklist.checked, shippingbox.circle, macbook.and.iphone, photo.badge.plus, list.clipboard) iOS 16–17 gerektirebilir; eski iOS'ta Lucide yedeği çizilir. Dağıtım hedefi kontrol edilmeli.
- RTL'de SF `chevron.backward/forward` aynalanması cihazda (Arapça) kontrol edilmedi.
- Sayım ekranlarında `ClipboardList` → `order` (belki `task`), stok boş durumu `packageIn` (belki `stock`); `jobs-screen` içinde gereksiz `as IconName` cast'leri.
- Depo girişi ipucu ikonları dekoratif; çok satırda `items-center` hizası.
- `check-placeholders.mjs` yalnızca literal `<Input`'u tarar; `TextInput`/sarmalayıcılar, `placeholder=""`, `{...rest}` ve yorum içi tırnak durumları kapsam dışı.
- Şifre placeholder'ı ("Şifreni gir") talimat gibi; düzenleme sayfasındaki salt-okunur telefon alanının placeholder'ı hiç görünmez.

**Durum (TEC-961):** RTL chevron maddesi dışındakiler kapatıldı. on-media rengi `--color-on-media` token'ında; dağıtım hedefi iOS 16.4 (Expo modül podspec'leri) ve SF Symbols 5 isteyen `truck.box`, `gauge.with.dots.needle.67percent`, `photo.badge.plus` için `minIOS: 17` ile Lucide yedeği çizilir (expo-symbols iOS'ta bilinmeyen sembolde `fallback` çizmez, boş kalır; `src/ui/icons.test.ts` denetler). Listedeki diğer semboller iOS 16.1 ve öncesi. Sayım ikonu `task`, stok boş durumu `stock`; ipucu ikonları üst hizalı; placeholder kontrolü `TextInput`, boş placeholder, yorum ve spread'i kapsar. RTL chevron aynalaması yine cihazda (Arapça) doğrulanmalı.

## Cihazda çalıştırma notları

- iPhone UDID `00008140-000C28DA1AF1801C`, imza Team `N5T7KH96U3`; `ios/` gitignore'da, prebuild sonrası `DEVELOPMENT_TEAM` eklenir.
- RN/Hermes tarball'ları `~/Library/Caches/ReactNative/` içinde; `RCT_USE_LOCAL_RN_DEP=1`, `RCT_TESTONLY_RNCORE_TARBALL_PATH`, `HERMES_ENGINE_TARBALL_PATH`, `LANG=en_US.UTF-8`.
- `expo-dev-client` yok; Metro IP'si derlemeye gömülü (`10.22.178.29`). IP değişirse yeniden derleme gerekir. İlk açılışta Yerel Ağ izni şart.
- Native değişiklik yoksa yalnızca `npx expo start --lan --port 8081` yeterli. İş bitince Metro'yu kapat.
