# Mobil uygulamada müşteri oturumu

Müşteri ve filo hesapları (yalnızca `customer` / `fleet` rolü olan, organizasyon üyeliği olmayan hesaplar) panelde `NO_PANEL_ACCESS` alır. Mobil uygulamada bu hesaplar artık **müşteri oturumu** ile giriş yapar.

## Giriş

`POST /v1/mobile/auth/otp/verify` (telefon + OTP, `phone-sign-in.md`):

| Hesap | Sonuç |
| --- | --- |
| Personel | Personel oturumu (`session_kind: "staff"`), davranış değişmedi |
| Müşteri / filo | Müşteri oturumu (`session_kind: "customer"`) |
| Bilinmeyen telefon | Portal OTP kurallarıyla (K11, K26) yeni müşteri hesabı açılır, müşteri oturumu verilir |

- Yanıtta ve `GET /v1/mobile/auth/me` içinde `session_kind` alanı vardır.
- Müşteri oturumunda `organization_slug` yok sayılır (organizasyon bağlamı yok).
- Personelin 2FA politikası müşteriye uygulanmaz (portaldaki gibi); hesabın kendi açtığı authenticator yine istenir.
- İsteğe bağlı `locale`, yeni açılan hesabın dilidir (yoksa Accept-Language).

## Token

- `aud` yine `mobile`; müşteri oturumunda ek claim `mobile_kind: "customer"`. Personel token'ları değişmedi (claim yok).
- Tür, refresh satırında saklanmaz: her mobil token üretiminde (giriş, refresh, organizasyon değişimi) hesaptan türetilir. Böylece refresh türü korur; rol değişikliği bir sonraki refresh'te uygulanır. Migration yok.
- Legacy (eski hub) token'ı müşteri türü taşımaz.

## İzinli rotalar

Müşteri oturumu yalnızca şunları çağırabilir:

- `GET /v1/mobile/auth/me`, `POST /v1/mobile/auth/logout`, `POST /v1/mobile/auth/logout-all`, `POST /v1/mobile/auth/refresh`
- `PUT|DELETE /v1/mobile/push-token`
- Portal aynası: `/v1/mobile/portal/*`

Diğer tüm `/v1/mobile/*` rotaları (organizasyon bağlamı, QR onayı, depo, ölçüm, legacy, panel aynası) `403 REALM_FORBIDDEN` döner. Kural merkezi: `middleware.MobileCustomerRealm` (`Authenticate` içinde, `RealmAllows`'tan önce).

## Portal aynası

`/v1/mobile/portal/<yol>` aynı mux üzerinden `/v1/portal/<yol>` olarak servis edilir (aynı handler, aynı feature / salt okunur / yetki zinciri, aynı gövde ve hatalar).

- `X-Mobile-Api-Version` zorunlu (426), uygulama sürüm kapısı orijinal mobil yolda çalışır.
- Bearer yok / geçersiz: 401. Personel mobil, panel veya portal token'ı: 403 `REALM_FORBIDDEN`.
- Aynada oturum portal oturumu gibi davranır (`Realm = portal`), portal kontrolleri aynen geçerlidir.
- `oauth/*` ve `ai/*` aynalanmaz (404).
- Örnek: `GET /v1/mobile/portal/vehicles` = `GET /v1/portal/vehicles`.

Kod: `backend/internal/middleware/mobile_portal_mirror.go`, `backend/internal/modules/auth/usecase/mobile_customer.go`.

## Açık konular

- E-posta + şifre ile mobil giriş (`/v1/mobile/auth/login`) müşteri / filo hesaplarına hâlâ `NO_PANEL_ACCESS` verir; filo kullanıcıları için ayrı iş.
- Rolü olmayan mevcut hesap (telefonla bulunursa) eskisi gibi personel oturumu alır; K26 sahiplenmesi mobilde yalnızca yeni / yarış durumunda açılan hesaplara uygulanır.
