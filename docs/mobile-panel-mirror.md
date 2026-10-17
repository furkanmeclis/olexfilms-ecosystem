# Mobil panel aynası (`/v1/mobile/panel/{path}`)

Mobil uygulama, izin verilen panel uçlarını `/v1/mobile/panel/{path}` üzerinden
çağırır. Örneğin `GET /v1/mobile/panel/services?limit=20`, mevcut
`GET /v1/services?limit=20` işleyicisini çalıştırır. BFF adresi
`/api/v1/mobile/panel/services?limit=20` olur.

## Sözleşme

* `Authorization: Bearer <aud=mobile JWT>` ve desteklenen
  `X-Mobile-Api-Version` zorunludur. Panel/portal token'ı 403
  `REALM_FORBIDDEN`, eksik veya desteklenmeyen API sürümü 426
  `MOBILE_API_VERSION_UNSUPPORTED` döndürür.
* Metot, query, gövde, yanıt ve izinler panel ucuyla aynıdır. Aktif organizasyon,
  marka, üyelik, feature flag ve step-up kontrolleri korunur; ayna ek yetki vermez.
  Step-up isteyen uçlar geçerli kullanıcı grant'i olmadan yine reddedilir.
* Uygulama sürüm kapısı (`X-App-Version`, TEC-236) yol değişmeden önce çalışır.
  Organizasyon seçimi ve oturum işlemleri mevcut `/v1/mobile/auth/*` uçlarındadır.
* OpenAPI'de panel ucunun şeması kullanılır; ayna yolları ayrı ayrı çoğaltılmaz.

## İzin verilen önekler

`services`, `service-pdfs`, `warranty-certificates`, `customers`, `vehicles`,
`warranties`, `warranty-claims`, `orders`, `quotes`, `stock`, `stock-transfers`,
`warehouse`, `measurements`, `reports`, `search`, `notifications`, `appointments`,
`catalog`, `vehicle-catalog`, `plate-formats`, `performance`, `tasks`, `leads`,
`pricing`, `certificates`, `photo-standard`, `service-catalog`, `geo`,
`announcements`, `stats` ve iki segmentli `tenant/organizations`, `tenant/modules`.

Önek ve alt yolları tam segment sınırında eşleşir (`ordersx` eşleşmez).
Diğer tüm önekler 404 `NOT_FOUND` döndürür; `auth`, `platform`, `portal`,
`public`, `mobile`, `oauth`, `realtime`, `ai`, `accounting` ve `impersonation`
aynaya açık değildir. Kodda tek liste `middleware.MobilePanelMirrorPrefixes` olur.
