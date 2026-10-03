# Son özet (handoff)

Tarih: 2026-10-03. Bu dosya, bulut oturumundaki orkestratör sürecinin devir notudur. Süreç buradan sonra kullanıcının kendi bilgisayarındaki Claude oturumlarında sürüyor.

Kaynaklar: Linear projesi "Olexfilms Tek App" (P-TEC-4), `AGENTS.md`, `docs/design.md`.

## 1. Nerede kaldık

| Faz | Durum |
|---|---|
| F0 Altyapı | Bitti (%100). |
| F1 Çekirdek iş | Kapı doğrulaması **geçti** (main `0aac6c0`, CI yeşil). Kod işleri bitti. Açık tek iş: TEC-226 (K7 kur kararı). |
| F2 Migrasyon ve geçiş | Başladı. 51 alt işe bölündü (TEC-233..283). 8'i bitti, kalanlar Backlog. |
| F3–F5 | Başlanmadı. |

### F1 kapı kanıtı
- `backend/internal/httpserver/f1_chain_integration_test.go` → `TestIntegrationF1ChainEURUAH`:
  - 100 barkod üretilir ve göze yerleştirilir.
  - Sipariş merkezden distribütöre (EUR), oradan bayiye (UAH) gider.
  - 2 birim hizmette tüketilir.
  - 2 garanti oluşur (120 ay, completed_at'ten başlar).
  - Üç org defterinde doğru kayıt düşer: kendi para biriminde, kur dondurulmuş, cari bakiye doğru.
- `f1_reverse_chain_integration_test.go` → `TestIntegrationF1ReverseChain`: itiraz, garanti void, tüketim düzeltme ve iade sonrası tüm cariler 0 olur, stok merkeze döner.
- Doğrulama yorumu Linear'da F1 kilometre taşında.

## 2. Migration durumu

- main'deki son migration: bu dosya merge edildiğinde **000075** olmalı (000074 TEC-252 migrator, 000075 TEC-266 Glorian şeması). `ls backend/migrations | tail -2` ile kontrol et.
- Sıradaki boş numara: **000076**.
- Kural: numarayı tek kişi (orkestratör) verir. Paralel PR'da aynı numara golang-migrate'i bozar. Migration'lı PR'larda auto-merge kapalı tutulur, numara sırasıyla merge edilir. Büyük numara önce merge edilirse küçük olan mevcut DB'lerde atlanır.

## 3. F2 sıradaki işler

Bağımlılıklar Linear'da "blocked by" ilişkisiyle işaretli.

**Bitenler:** 253 (eski şema fixture'ı), 267 (Glorian istemcisi ve fake sunucu), 238/239 (portal araç ve hizmet API'leri), 235 (mobil depo uçları), 247 (landing), 248 (/garanti tamamlama), 252 (migrator iskeleti), 266 (Glorian şeması).

**Migration'sız, hemen alınabilir:**
- 234 (mobil eski uç alias katmanı; 233'e bağlı)
- 241 (portal UI)
- 254 (migrator orgs/users)
- 256 (araç kataloğu/ürünler)
- 268–271 (Glorian pull/push/outbound)

**Migration gerektiren alt işler** (numarayı sırayla ver): 233 (minimal ölçüm saklama), 240 (bayi koordinatları + nearby), 244 (değerlendirme formu), 249 (kısa URL), 255 (müşteriler; telefonu/e-postası olmayanlar için şema değişikliği, K26), 263 (short URL + legacy_messages).

**Yalnız gerçek ortamda yapılabilen işler (ENV):** 265, 278, 237, 277, 279, 280, 281, 283. Bunlar en sona kalır. Adımları her işin açıklamasında; bitince Done yapılıp kontrol TEC-140'a yorum olarak eklenir.

## 4. Kullanıcı kararı bekleyenler

- **TEC-226 (K7):** Kurun değeri mi dondurulsun, yalnız kur günü mü? Testler şu an kurun sipariş onayında dondurulduğunu doğruluyor.
- **TEC-230:** Tamamlanmış hizmet iptal edilebilsin mi? Karar gelene kadar muhafazakâr varsayılan uygulandı: yalnız tüketim düzeltme ucu var (merkez, 24 saat, garantili kalem reddedilir, muhasebeye dokunmaz). "Evet" olursa iptal akışı ayrı iş olarak açılacak.
- **dealer_staff `stock.read`:** Bu role stok okuma yetkisi verilsin mi?

## 5. Bilinen açıklar ve notlar

- **Eski garanti kodları:** `chk_warranties_public_code` 12–32 karakter istiyor. Eski hub'ın 10 karakterlik kodları F2-01i migration'ı bu kısıtı gevşetene kadar 404 döner.
- **Migrator env adları değişti (TEC-252):** `MIGRATOR_*_DSN` yerine `LEGACY_HUB_DSN`, `LEGACY_WH_DSN`, `LEGACY_HUB_STORAGE_DIR` ve `MIGRATOR_ARGS`. Sunucudaki env dosyası buna göre güncellenmeli.
- **Migrator SQL guard'ı sıkı:** Kaynak tarafında yalnız tek SELECT/WITH kabul ediliyor; REPLACE, SET ve INTO kelimeleri reddediliyor. Step sorguları bu sınırda yazılmalı.
- **Glorian kimlik doğrulama:** İstemci hem `Authorization: Bearer` hem `X-Inventory-Api-Key` gönderiyor, mevcut hub kırılmasın diye. API anahtarı DB'de `api_key_enc` (SecretBox) olarak şifreli tutuluyor. `glorian.Store` henüz server'a bağlı değil.
- **Mobil depo:** 23 uç `/v1/mobile/warehouse/*` altında. Bağlanmayan uçların listesi `docs/mobile-warehouse-endpoints.md` §4'te.
- **Portal sahiplik kuralı:** Bir hizmeti, hizmetin müşterisi veya hizmetin garantilerinden birini tutan kullanıcı görür. Araç ise yalnız `vehicles.user_id` sahibine görünür.
- **Portal hizmet PDF'i:** Panel PDF'iyle aynı içeriği basıyor, kalem notları dahil. Notların müşteriye görünmesinin uygunluğu TEC-140'ta kontrol maddesi.
- **"Bayi bul" linki:** Landing'de `/portal/dealers`'a gidiyor; o sayfa TEC-242 ile gelecek, o zamana kadar 404.
- **Ortam doğrulamaları:** Gerçek ortam gerektiren tüm kontroller (Gotenberg PDF'leri, Glorian hub, migrator compose vb.) TEC-140'a yorum olarak ekli.
- **Repo görünürlüğü:** Actions dakikası bittiği için repo public yapıldı; hâlâ public. CI path-filter ile hafifletildi (image-build ve dr-drill yalnız main'de, `ci-full` etiketinde ya da `.github/**` değişikliğinde koşuyor).

## 6. Çalışma kuralları (alt ajanlara verilen ortak kurallar)

- **Okuma:** `docs/design.md`'yi tümüyle yükleme, `grep -n` ile bul ve dar aralık oku. Referans repolar (`/home/user/olexfilms`, `/home/user/olexfilms-warehouse` vb.) salt okunur.
- **HTTP kodları:** Doğrulama hatası her zaman **400 VALIDATION_ERROR**. 422 yalnız adı konmuş iş kuralı kodları için.
- **OpenAPI:** 3.1 kullanılıyor; `nullable` yerine `type: [x, "null"]` yaz. `docs/openapi.yaml` ile `backend/docs/openapi.yaml` eşit kalmalı. Sonra `make openapi-lint` ve `pnpm api:generate` çalıştır.
- **i18n:** Yeni metin 13 dilin hepsinde olmalı. `make check-i18n-translations && make check-i18n` çalıştır.
- **Yeni tablo:** migration + `sqlc generate` + iş tablosuysa `organization_id`/`brand_id`.
- **Merge hijyeni:**
  - Her push'tan önce `git merge origin/main`.
  - Paylaşılan dosyalara yalnız ekleme yapılır: rbac kataloğunda en sona (sort_order MAX+10), events kataloğunda, route'larda, i18n'de, OpenAPI'de.
  - Üretilen dosyaları (sqlc, api.d.ts, i18n gen) elle birleştirme, yeniden üret.
  - Formatlayıcıyı yalnız değiştirdiğin dosyalarda çalıştır.
- **DB testleri yalnız CI'da** (PG18, migration roundtrip). Yerelde Postgres açmaya çalışma, kimlik bilgisi arama. Sık düşülen hatalar:
  - users kaydında `chk_users_email_or_phone` kısıtı için benzersiz e-posta ya da telefon ver.
  - VARCHAR taşması 22001 döner.
  - Trigger'larla gelen kısıtları hesaba kat.
  - sqlc parametresini her kullanımda aynı tipe cast et.
  - Testler ledger'a doğrudan yazmasın, `ledger.Post` kullansın; yoksa ortak DB'de başka testlerin projeksiyon kontrolleri bozulur.
- **Güvenlik sınırları:**
  - Prod sunucuya bağlanma, eski sistem DB'lerine yazma, secret commit'leme, `git push --force` yok.
  - Kapsam dışı maddeler (design.md bölüm 9) yapılmaz.
  - Linear'da iş silme ya da kilometre taşı değiştirme yok.
- **Takılınca:** En muhafazakâr yolu seç ve nedenini Linear'a yaz. Aynı hata 3 denemede geçmezse işi Todo'ya al.

## 7. Karar kartı özeti

Tam metin `docs/design.md` bölüm 3 ve 6'da. Sık başvurulanlar:
- **K1/K20:** Depo marka-bağımsız; hizmet, garanti, müşteri ve sipariş domain markasıyla sınırlı.
- **K2:** Glorian taşınmaz, Inventory API ile senkron.
- **K4:** Distribütör kendi depoları; merkez depoya erişimi yok.
- **K7:** Org başına para birimi, kur dondurulur.
- **K9/K24:** Muhasebe tek yönlü; alt seviye itiraz eder, üst seviye ters kayıtla düzeltir.
- **K12:** Bayi basit stok.
- **K14:** Barkodu yalnız merkez üretir.
- **K26/K27/K29:** Migrator aynı telefonlu müşterileri birleştirir, tekrar çalıştırılabilir, telefonlar E.164.
