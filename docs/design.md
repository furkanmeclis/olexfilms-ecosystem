# Olexfilms Tek App — Tasarım ve Karar Dokümanı

Tarih: 2026-09-30 · Hazırlayan: Furkan Meclis (Claude ile soru-cevap oturumu)
Claude Docs sürümü: https://claude.ai/code/artifact/2c449f59-00e0-443e-9509-34cf8f69f730

## 1. Amaç ve özet

Olex Films'in dört ayrı uygulaması (garanti hub'ı, depo, AI katmanı ve Glorian fork'u) tek bir Go + Next.js uygulamasında birleşiyor; yalnızca Olex markası taşınıyor, Glorian mevcut kurulumunda kalıp depo senkronu ile bağlı kalıyor. Mevcut sistem olduğu gibi taşınmıyor: veri modeli (organizasyon ağacı, ledger, marka izolasyonu, fiyat zinciri) yeniden kurgulanıyor ve 30'un üzerinde yeni modül ekleniyor.

**Neden tek app:** bugün ürün, bayi, stok ve sipariş üç ayrı veritabanında kopya olarak yaşıyor; senkron yanlış hedefe yazabiliyor (HYDRA rollback olayı: 1.349 kayıt geri alındı) ve her sistem kendi kullanıcı ve rol setini tutuyor. Tek DB ile senkron, reconcile ve connection katmanı sadece Glorian köprüsüne iner.

**Kapsam sınırı:** Web paneli (merkez, distribütör, bayi), müşteri portalı, herkese açık sayfalar, WhatsApp ve AI katmanı, MCP uçları, migrator. Mobil uygulama bu fazda yazılmıyor; API sözleşmesi ve auth ilk günden kuruluyor.

**Kapasite hedefi:** 5 yıl, yıllık %50 bileşik büyüme (7,6 kat), 9 yeni ülke (Almanya, Hollanda, Yunanistan, ABD, Ukrayna, Bulgaristan, Azerbaycan, Çin, Dubai), RTL desteği. Bugünkü veri (89 MB hub + 53 MB depo) bu katsayıyla bile tek Postgres ve iki worker'ı zorlamıyor.

**Referans repolar (her modülde ayrıca belirtilir):**

| Repo | GitHub | Yerel yol |
| --- | --- | --- |
| olexfilms (hub) | https://github.com/furkanmeclis/olexfilms | /Users/furkanmeclis/Documents/Projects/olexfilms |
| olexfilms-warehouse | https://github.com/furkanmeclis/olexfilms-warehouse | /Users/furkanmeclis/Documents/Projects/olexfilms-warehouse |
| olexfilms-ai-layer-go | https://github.com/furkanmeclis/olexfilms-ai-layer-go | /Users/furkanmeclis/Documents/Projects/olexfilms-ai-layer-go |
| glorian | https://github.com/furkanmeclis/glorian | /Users/furkanmeclis/Documents/Projects/glorian |
| otopoly-go | https://github.com/furkanmeclis/otopoly-go | /Users/furkanmeclis/Documents/Projects/otopoly-go |
| nextjs-go-boilerplate | https://github.com/furkanmeclis/nextjs-go-boilerplate | /Users/furkanmeclis/Documents/Projects/nextjs-go-boilerplate |
| technowide-ecosystem | https://github.com/furkanmeclis/technowide-ecosystem | /Users/furkanmeclis/Documents/Projects/technowide-ecosystem |
| go-ubltr | https://github.com/furkanmeclis/go-ubltr | /Users/furkanmeclis/Documents/Projects/go-ubltr |

## 2. Mevcut sistem tespitleri

Dört kod tabanı ve açık gereksinim listesi okundu (30.09.2026). Özet: hub olgun ama Laravel/Filament; depo en temiz kod tabanı (1.125 test); AI katmanı diğerleriyle bağsız; Glorian bir fork.

| Sistem | Ne yapıyor | Stack | Durum |
| --- | --- | --- | --- |
| olexfilms (hub) | Hizmet, garanti, bayi, müşteri, sipariş, NexPTG ölçüm, bildirim, SMS/WhatsApp, push kampanya, müşteri portalı, Inventory API, Chatbot M2M API, mobil API | Laravel 12, Filament v5, Inertia/React, MariaDB, DB queue | 437 commit, aktif |
| glorian | olexfilms'in Nisan 2026 fork'u, marka değiştirilmiş; push kampanya, WhatsApp log, passkey, Meilisearch yönetimi yok | Aynı | 27 commit, geride |
| olexfilms-warehouse | Konum ağacı (oda/koridor/raf/göz), barkod ve etiket, stok girişi, transfer, sayım, sipariş çıkışı, muhasebe belgesi, gün sonu, iki markaya Inventory API senkronu | Laravel 13, Inertia/React, MariaDB, Horizon, Reverb, Meilisearch | 1.125 test, operasyonel olarak yeni |
| olexfilms-ai-layer-go | WhatsApp botu (whatsmeow), hub'ın salt okunur M2M API'sinden veri, bildirim gönderme API'si | Go, Postgres, OpenAI/Gemini | Warehouse ve Glorian ile bağı yok |

**Entegrasyon bugün:** ortak DB yok, her şey REST. Depo hem Olex hem Glorian'a aynı `/api/v1/inventory` sözleşmesiyle bağlanıyor; veri sahipliği `olexfilms-warehouse/docs/architecture/source-of-truth.md` (D1–D12) ile tanımlı.

**Tek app'te ortadan kalkan çiftler:** 3 ayrı users tablosu ve rol seti; ürün/kategori/marka kopyaları; hub Dealer ile depo Customer + integration_links; hub `stock_items` ile depo `product_barcodes` + ledger; iki sipariş modeli; bildirim, export, activity log, 2FA/passkey, Meilisearch kopyaları; hub'daki kimliksiz MCP ucu ile Chatbot M2M API'nin aynı işi yapması; reconcile, sync run, connection yönetimi.

**Mevcut açık gereksinimler (yeni modele doğrudan giriyor):** tek aktif sahip + hareket defteri; metrajlı ürün; distribütör ve alt bayi; müşteriden bayiye yükseltme (tek kimlik); Olex↔Glorian ürün dönüşümü; güvenli toplu import + undo. Bekleyen kararlar: PDF motoru (→ Gotenberg), bildirimlerin WhatsApp'a taşınması (→ wuzapi tek numara), gövde SVG kapsamı (→ mevcut SVG'ler korunur).

**Kodda bulunan riskler (yeni sistemde ele alınır):**

- Hub: garanti süresi dolması cron yok (lazy), garanti çift kaydı riski, `WARRANTY_EXPIRING_SOON` ve stok bildirimleri hiç tetiklenmiyor, `/mcp/olex` kimliksiz, müşteri hash'i süresiz, Dockerfile'da gömülü APP_KEY, fuar dashboard'u açık, queue_monitors 109 bin satır çöp.
- Depo: backup schedule yok, "sipariscim" şablonundan artık config'ler, legacy `source` kolonları, activity_log 53 bin satır.
- AI: vision bağlanmamış, ses ve video yok, kuyruk yok, araç segmentasyonu spec'i commit edilmemiş, resmi olmayan WhatsApp istemcisi (ban riski).

**Sunucu (technowide-spaceship, salt okunur):** 8 vCPU, 15 GB RAM, load 1,8. Hub DB 89 MB: 6.484 hizmet, 6.463 müşteri, 11.968 stok birimi, 8.789 garanti, 7.081 ölçüm satırı, 1.393 sipariş, 151 bayi, 317 kullanıcı. Depo DB 53 MB: 13.525 barkod, 7 stok girişi.

## 3. Temel kararlar

| # | Karar | İçerik |
| --- | --- | --- |
| K1 | Tek app, tek DB, marka izolasyonu | Her ürün, bayi, stok birimi, hizmet ve kullanıcı bir `brand` altında. Olex kullanıcısı Glorian ürünü göremez ve kullanamaz. |
| K2 | Glorian bu kapsamda taşınmaz | Glorian mevcut kurulumunda kalır. Yeni app'in depo modülü `brand=glorian` fiziksel stoğunu tutmaya devam eder ve Glorian hub'ına Inventory API sözleşmesiyle senkron olur (katalog pull, barkod push, sipariş outbound, reconcile). Garanti/hizmet/müşteri tarafı Glorian'a kapalı. Kod marka-bilinçli; Glorian'ı ileride içeri almak veri import'udur. Migrator'da Glorian profili hazır, kapalı. |
| K3 | Giriş URL'i markayı belirler | Bu fazda tek domain: `olexfilms.app`. Glorian gelirse `warranty.glorianppf.com` üçüncü URL olur. |
| K4 | Distribütör modeli | Bayilere bayilik yapan hesap. Kendi depoları (birden fazla olabilir), kendi alt bayileri, merkez depoya erişimi yok. Katalog yalnızca merkezden; kendi katalog tanımlamaz, barkod üretmez. Kendisi de hizmet verebilir. Kayıtta "depo olarak tanımla" preset'i ile depo otomatik açılır. |
| K5 | Ülke/bölge veritabanı | Ülke > il > ilçe. Her distribütöre territory atanır; o bölgedeki bayiler distribütöre bağlıdır. Her ülke = bir distribütör varsayımı; çakışma kontrolü territory ataması üzerinden. Türkiye'de de distribütör var. |
| K6 | Admin bayiye doğrudan ürün satmaz | Ürün akışı merkez → distribütör → bayi. Hizmet satışı (ürün dışı) merkezden bayiye doğrudan olabilir. |
| K7 | Para birimi ve kur | Organizasyon başına para birimi; admin distribütör kaydında seçer. Sistemde kur tablosu: günlük otomatik çekim (TCMB, ECB) + manuel üzerine yazma; her sipariş ve muhasebe kaydında o anki kur dondurulur. |
| K8 | Fiyat zinciri | Admin ürün alış + satış fiyatı (distribütöre) belirler ve distribütör bazında özel fiyat verebilir. Distribütörün alışı = admin siparişindeki satış fiyatı; distribütör kendi satışını belirler. Bayinin alışı = distribütörün satışı; bayi son satış fiyatını kendisi belirler, satış başına kâr analizi. Merkez tavsiye satış fiyatı yayınlar. Fiyat görünürlüğü izin scope'u ile. |
| K9 | Muhasebe köprüsü hiyerarşik ve tek yönlü | Üst seviye (merkez veya distribütör) alt seviyeye sipariş/hizmet satışı işler → alt seviyenin defterine otomatik gider + üst seviyeye borç düşer; alt seviye bu kayıtları silemez, üst deftere yazamaz. Her org'un tedarikçisi = ağaçta bir üstü. |
| K10 | Lokalizasyon her yerde | Bayi, müşteri, distribütör ve kullanıcı kendi dil + saat dilimi tercihini tutar; UI, WhatsApp, e-posta, PDF buna göre. 12 mevcut dil + Arapça; RTL zorunlu. |
| K11 | Müşteri bir user'dır | Ayrı customer auth yok. Giriş yalnızca telefon OTP (WhatsApp), her OTP'de KVKK bilgilendirmesi. Müşteri global: birden fazla bayi ve distribütörden hizmet alabilir. Müşteriden bayiye yükseltme = rol değişimi, geçmiş korunur. Filo müşterisi e-posta + şifre ile girer. |
| K12 | Bayi = basit stok | Bayi elindeki birimleri görür, siparişi kabul eder, hizmette tüketir. Konum/sayım/göz yok. Tam depo modülü merkez ve distribütörde. |
| K13 | Bayiler arası transfer distribütör onaylı | Bayi A talep açar, distribütör onaylar, transfer fiyatı = A'nın alış fiyatı, muhasebede A alacak / B borç. |
| K14 | Barkod üretimi yalnızca merkez | Admin stok girişinde hedef depo seçer; kodlar o depoya tanımlanır, transferle dağılır. Distribütör stok girişi akışıyla kabul eder. |
| K15 | AI olmayan tahmin | Stok tükenme ve sipariş önerisi algoritmik (hareketli ortalama, mevsimsellik); admin genelinde minimum veri süresi dolmadan öneri üretilmez. |
| K16 | Tek WhatsApp numarası | Tüm OTP, bildirim, kampanya ve AI sohbet merkez numarasından; wuzapi tek instance. |
| K17 | Hata takibi | Sentry uyumlu SDK, DSN technowide-ecosystem errortracking alıcısına; hata kategorileri modül bazında kodda sabit. |
| K18 | Taksit yok | Muhasebe yalnızca kayıt tutar; taksit açıklama alanında izlenir. |
| K19 | KVKK/GDPR ilk fazda | Anonimleştirme aksiyonu (hizmet ve garanti kalır, kişi verisi maskelenir) + veri dışa aktarma. AI kullanımı için girişte "yönergeleri kabul ediyorum" onayı (varsayılan işaretsiz, bkz. K22), metin admin tarafından Markdown ile. |

**Gözden geçirme sonrası eklenen kararlar (30.09.2026):**

| # | Karar | İçerik |
| --- | --- | --- |
| K20 | Marka izolasyonu iki katmanlı | Depo modülü marka-bağımsız: merkez depo rolleri deposuna giren her markayı yönetir. Hizmet, garanti, müşteri ve sipariş tarafı giriş yapılan domain'in markasıyla sınırlı (merkez dahil). |
| K21 | WhatsApp sağlayıcı soyutlaması | Mesajlaşma katmanı WhatsApp Cloud API uyumlu bir arayüz üzerinden yazılır; wuzapi ilk sağlayıcı, ileride resmi Cloud API'ye geçilir. SMS yedeği admin tarafından açılabilir. |
| K22 | AI onayı işaretsiz | Yönerge kabul kutusu varsayılan işaretsiz; ilk girişte bir kez sorulur; onay tarihi ve metin sürümü kaydedilir (GDPR). |
| K23 | Sözleşme onboarding'i bloklar | Distribütörün açtığı bayi, admin sözleşme yükleyene kadar salt okunur kalır; grace uygulanmaz. |
| K24 | Muhasebede itiraz | Alt seviye üstten düşen kaydı silemez ama açıklamayla itiraz edebilir; üst seviye ters kayıtla düzeltir; itirazlar üst panelde listelenir. |
| K25 | Distribütör değişikliği yalnızca admin | Bayinin distribütörünü (re-parenting) yalnızca admin değiştirir; açık cari için devir kaydı üretilir. |
| K26 | Müşteri birleştirme | Migrator aynı telefondaki müşteri kayıtlarını tek user'da birleştirir; telefonsuz kayıtlar "doğrulanmamış" taşınır, ilk OTP girişinde sahiplenilir. |
| K27 | Migrator geçiş dönemi aracı | Sunum ve geçiş süresince tekrar çalıştırılır; final cutover sonrası kaldırılır. |
| K28 | Ölçüm kabul ucu mobil sözleşmesinde | Ölçümler mobil uygulamadan gelir; kabul ucu F1'deki mobil API sözleşmesine girer. F2 geçiş şartı: mevcut mobil uygulamanın ölçüm ucu yeni API'ye yönlendirilir. |
| K29 | Telefon numarası standardı | Tüm telefonlar DB'de E.164 biçiminde tutulur (`+905551234567`; ülke kodu zorunlu, boşluk/ayraç yok). Girişte libphonenumber ile ayrıştırma ve doğrulama; kullanıcının org/lokal ülkesi varsayılan ülke kodu olur. Ayrıca `phone_country` (ISO 3166-1 alpha-2) ve `phone_national` (yerel gösterim) türetilmiş alan olarak saklanır. OTP, WhatsApp, arama ve migrator hep E.164 anahtarıyla çalışır; eski kayıtlar (`0(555)...`, `905...`, `5...`) migrator'da normalize edilir, çözülemeyenler "doğrulanmamış" işaretlenir. Referans: olexfilms `docs/chatbot-m2m-api.md` telefon normalize kuralları (10 hane, sadece TR) → uluslararasıya genişletilir. |

## 4. Organizasyon ve yetki modeli

Sistem bir organizasyon ağacı üzerine kurulu: merkez → distribütör → bayi. Ürün, fiyat, muhasebe köprüsü, modül izinleri ve kampanya onayı hep bu ağacı izler. Müşteri ağacın dışında, global bir user'dır.

```
Merkez (Olex)
 ├─ Distribütör TR  ── depo(lar) ── bayiler (il/ilçe territory)
 ├─ Distribütör DE  ── depo(lar) ── bayiler
 ├─ Distribütör NL  ── ...
 └─ ...
Müşteri (global user) ── hizmet alır: herhangi bir bayi/distribütör
```

**Roller (izinler scope'lu RBAC ile verilir; roller hazır paket):**

| Organizasyon | Roller | Not |
| --- | --- | --- |
| Merkez | `super_admin`, `center_staff`, `center_warehouse`, `center_accounting`, `center_social` | center_staff modül bazında izin paketi alır. center_social lead, kampanya ve sosyal modüllere erişir, tüm sistem verisini görür; gelecek sosyal modüller için rezerve. |
| Distribütör | `owner`, `staff`, `warehouse_staff`, `accounting` | subtree scope: kendi ağacındaki bayileri görür |
| Bayi | `owner`, `staff`, `accounting` | staff hizmet açar, fiyat ve muhasebe görmez |
| Müşteri | `customer` | telefon OTP |
| Filo | `fleet` | e-posta + şifre, salt okunur |

- İzin scope'ları: `own`, `assigned`, `managed`, `subtree` (technowide-ecosystem `backend/migrations/000021_permission_scopes` referans; `subtree` yeni).
- Bir user birden fazla org'da üye olabilir; org değiştirici üstte. Impersonation yalnızca super_admin.
- Fiyat görünürlüğü role değil izne bağlı: `pricing.purchase.read`, `pricing.sale.write`.

**Sözleşme süresi:** her bayi ve distribütör merkezle sözleşmelidir; PDF ve geçerlilik tarihi yalnızca admin yükler (distribütör süre ekleyemez). Bitiş öncesi 30/7 gün uyarı; bitişte salt okunur (yeni hizmet, sipariş, garanti başlatılamaz; mevcut kayıtlar ve portal görünür); grace süresi admin ayarı. Distribütör süresi dolarsa alt bayiler etkilenmez.

**Modül paketleri (feature flag hiyerarşisi):**

- Üç seviye: **Çekirdek** (kapatılamaz), **Standart** (varsayılan açık, kapatılabilir), **Eklenti** (varsayılan kapalı, açılır). Liste bölüm 5'te.
- Zincir: admin sistem genelinde açar/kapar ve distribütöre tanımlar; distribütör bayi bazında veya "bayi standardı" ile toplu açar/kapar; yeni bayi standardı miras alır. Üstte kapalıysa altta açılamaz.
- Admin bir bayiye distribütörden bağımsız modül açabilir; hizmet kaydı gerekmez.
- Her org panelinde "Özellikler" sayfası: tüm modüller (açık/kapalı, ücretli/ücretsiz), liste üst seviyenin erişimi kadar; talep için üst seviyeyle iletişim.
- Varsayılan açık olanlar ücretsizdir, hizmet kaydı gerektirmez, tekrarlamaz; bu ibareler sayfada yazar. Admin sistem varsayılanlarını değiştirebilir.
- Eklenti, hizmet kataloğunda "modül paketi" tipli hizmet olarak tanımlanabilir: tanımlanınca açılır, bitiş veya iptalde kapanır, cayma bedeli aynı mekanizma.
- Flag'ler DB'de (JWT'de değil), Redis 30 sn cache; değişiklik anında etkili.

**Referans:** otopoly-go `backend/internal/modules/billing` (plan, entitlement, hard/soft limit) ve `RequireFeature` middleware'i (`backend/internal/httpserver`), https://github.com/furkanmeclis/otopoly-go, /Users/furkanmeclis/Documents/Projects/otopoly-go.

## 5. Modül envanteri

Her modül için: seviye (Ç = çekirdek, S = standart, E = eklenti), kapsam ve referans alınan repo + yol. Yerel yollar `/Users/furkanmeclis/Documents/Projects/` altındadır; tabloda kısaltılmıştır.

### 5.1 Çekirdek (kapatılamaz)

| Modül | Kapsam | Referans repo ve yol |
| --- | --- | --- |
| Organizasyon ve kullanıcı | Org ağacı (merkez/distribütör/bayi), user çoklu üyelik, roller, scope'lu izinler, impersonation, 2FA, passkey, step-up, org değiştirici, sözleşme PDF + geçerlilik + grace, "Özellikler" sayfası | otopoly-go `backend/internal/modules/{auth,organizations}`, `migrations/000021,000022,000043`; technowide-ecosystem `backend/migrations/000021_permission_scopes`, `backend/internal/platform/rbac`; olexfilms `app/Policies/*`, `app/Enums/UserRoleEnum.php` |
| Ülke/bölge ve territory | Ülke > il > ilçe, distribütör territory ataması, çakışma kontrolü, plaka formatları (regex + görsel meta), kur tablosu | olexfilms `dealer-locations.json`, `app/Console/Commands/ImportDealerCoordinates.php`; plaka: olexfilms `services.plate_country` alanı |
| Marka ve katalog | Brand, ürün, kategori, `available_parts`, garanti süresi, mikron, metraj/rulo tanımı, sabit barkodlu ürün, ürün görselleri, tavsiye satış fiyatı, distribütör bazlı özel fiyat | olexfilms `app/Models/{Product,ProductCategory}.php`; warehouse `app/Models/{Product,Brand,ProductBarcodeTemplate}.php`, `app/Support/Catalog/SyncedCatalogFields.php` |
| Stok ve ledger | Fiziksel birim (barkod/lot), tek aktif sahip, append-only `stock_movements`, projeksiyon tabloları, metrajlı tüketim, reclassification (barkod korunarak ürün değişimi), güvenli toplu import (staging, dry-run, batch ID, undo) | warehouse `app/Models/{ProductBarcode,StockMovement,BinProductStock}.php`, `app/Actions/StockEntry/*`, `docs/architecture/source-of-truth.md`; olexfilms `app/Support/Inventory/OrderStockReservation.php` |
| Depo (tam, merkez + distribütör) | Depo, oda, konum ağacı (koridor/raf/göz), QR etiket, evrensel tarama, stok girişi (hedef depo seçimi), transfer (göz↔göz, depo↔depo), sayım (çok yöntemli, blind/guided), gün sonu raporu, distribütör preset'i ile otomatik depo | warehouse `app/Actions/{StockTransfer,StockCount,WarehouseLocation}/*`, `app/Actions/Scan/ResolveScanAction.php`, `DEGISIKLIKLER.md` |
| Sipariş ve fiyat zinciri | Merkez→distribütör, distribütör→bayi siparişi; alış = üstün satış fiyatı; kur dondurma; hazırlık/sevk/teslim/kabul/iptal; barkod atama; bayiler arası transfer (distribütör onaylı); depolar arası sipariş | olexfilms `app/Filament/Resources/Orders`, `app/Support/Inventory/OrderStockReservation.php`; warehouse `app/Actions/Order/*`, `app/Models/{Order,OrderOutbound,AccountingDocument}.php` |
| Hizmet ve garanti | Araç kabul akışı (ölçüm var mı? → rapor seç → VIN zorunlu), hizmet sihirbazı, araç parça seçici (yerleşimi ayarlanabilir), hızlı seçim preset'leri, stok tüketimi, garanti başlatma, garanti süresi cron'u, garanti bitiş bildirimi, hizmet PDF'i, QR, araç devri (çift kodlu), müşteri değerlendirme isteği | olexfilms `app/Filament/Resources/Services/*`, `app/Observers/ServiceObserver.php`, `app/Listeners/ServiceStatusUpdatedListener.php`, `docs/car-part-picker.md`, `resources/js/panel/panel.tsx`, `app/Models/ServiceCustomerTransfer.php` |
| Müşteri | Müşteri = user, telefon OTP + KVKK, global (çoklu bayi), müşteriden bayiye yükseltme, araçlar, KVKK anonimleştirme + veri dışa aktarma | olexfilms `app/Services/CustomerPortal/{CustomerOtpService,CustomerHashService}.php`; otopoly-go `backend/internal/modules/contracts/usecase/otp.go` (OTP + KVKK metni) |
| Bildirim merkezi | Kanallar: WhatsApp, SMS (varsayılan kapalı), e-posta, web push, Expo push, in-app. Event × rol şablonları (yalnızca admin), dil bazlı içerik, kullanıcı tercihleri, tüm roller için | olexfilms `app/Services/NotificationService.php`, `app/Enums/NotificationEventEnum.php`, `notification_settings` tablosu; otopoly-go `backend/internal/modules/notifycenter`; nextjs-go-boilerplate `internal/modules/notifications` |
| Muhasebe çekirdeği | Kasa/banka hesapları, hiyerarşik kategoriler, gelir/gider/transfer, cari (borç/tahsilat/düzeltme), kaynak bazlı otomatik kayıt ve iptal cascade'i, hiyerarşik köprü (üst → alt otomatik gider), çok para birimi + kur, rapor. Bayi tarafında çekirdekte **salt okunur** | otopoly-go `backend/internal/modules/{finance,cari,reports}`, `migrations/000025,000033`, `finance/usecase/source.go` (`PostIncomeFromSourceTx`, `VoidBySourceTx`); technowide-ecosystem `backend/internal/modules/{ledger,payments,invoices}` |
| Arama | Meilisearch her listede, org filtreli indeks, Cmd+K global arama | nextjs-go-boilerplate `internal/platform/searchengine`; warehouse `app/Support/Search/*` (Searcher altyapısı) |
| Import / Export | I/O engine (CSV/XLSX/PDF/JSON), rollback, kolonlar org'a göre filtreli, merkezi import/export listesi (org bazlı), bulk işlemler (undo) | nextjs-go-boilerplate `internal/platform/{ioengine,bulkengine}`; warehouse `app/BulkOperations/*`, `config/exports.php` |
| Görevler (merkez) | Konu = bayi/distribütör, sorumlu = merkez çalışanı, performans panelinden otomatik görev | otopoly-go `todos` tablosu (`migrations/000050`), `backend/internal/modules/ai/tools/todo_tools.go` |
| Sistem ayarları | Plaka formatları, kur, minimum tahmin süresi, grace, fotoğraf standardı aç/kapa, AI yönerge metni (Markdown), modül varsayılanları | olexfilms `app/Filament/Pages/Manage*Settings.php`; nextjs-go-boilerplate settings altyapısı |

### 5.2 Standart (varsayılan açık, kapatılabilir)

| Modül | Kapsam | Referans repo ve yol |
| --- | --- | --- |
| Araç kabul sözleşmesi | Şablon modülü (Lexical editör), admin tek default şablon, değişkenler yalnızca müşteri/bayi verisi, imza tarafları sabit (müşteri + işlemi yapan personel), canvas imza, WhatsApp OTP + KVKK, görsel ekleme, Gotenberg PDF, sha256 kanıt; zorunluluk admin ayarı; hizmet satışı için de kullanılır | otopoly-go `backend/internal/modules/contracts/*`, `migrations/000040,000041,000047`, `frontend/src/features/contracts/*`, `docs/superpowers/specs/2026-09-17-messaging-contracts-ux-design.md` |
| Ölçüm sonuçları | `nexptg_reports` → `measurement_results`; VIN zorunlu; NexPTG cihaz kaydı; mobil Bluetooth ile kabul API'si; VIN bazlı otomatik öncesi/sonrası eşleme; parça bazlı mikron fark tablosu; mevcut SVG'ler korunur; ölçüm başına zaman damgalı PDF; elle seçilen raporlar panelde, müşteriye kapalı | olexfilms `app/Services/Nexptg/*`, `app/Services/NexptgSyncService.php`, `app/Models/Nexptg*.php`, `docs/nexptg/*` |
| Lead ve teklif | Her org kendi lead'i; `target_type = customer / dealer_candidate / distributor_candidate`; kaynak, sıcaklık, takip tarihi, sorumlu, zaman çizelgesi; teklif kalemleri ürün + hizmet kataloğundan; WhatsApp ile gönderim; hatırlatma; kazanılınca hizmet veya org oluşturma; admin tümünü görür; public bayi başvuru formu → territory'ye göre distribütöre, yoksa merkeze | otopoly-go `backend/internal/modules/{leads,quotes,salesflow}`, `migrations/000054,000055` |
| Randevu ve kapasite | Bayi günlük kapasite, portal/asistan üzerinden randevu, hatırlatma, gelmeyen → lead, merkezde ağ doluluğu | Yeni; lead ve bildirim modüllerine bağlanır |
| Duyuru ve doküman merkezi | Merkezden ağa duyuru, kılavuz ve materyal kütüphanesi, okundu takibi, dil bazlı sürüm | technowide-ecosystem `backend/internal/modules/announcements` |
| Garanti talebi | Bayi/distribütör müşteri adına açar; fotoğraf, parça, açıklama; AI ilk triyaj; merkez onay/red; onaylanınca yeniden uygulama hizmeti + garanti gideri; ürün/lot hata oranı raporu | Yeni; hizmet ve muhasebe modüllerine bağlanır |
| Bayi muhasebesi (tam) | Kendi gelir/gider, hizmet başına gelir kaydı, sipariş → otomatik gider, cari, sabit barkodlu üründe kendi satış fiyatı ve kâr analizi, personel kartı + maaş/avans, prim (bayi → çalışan, hedef bazlı) | otopoly-go finance/cari (yukarıdaki); `product_sales`, `purchases`, `suppliers` (`migrations/000037,000038`) |
| Hizmet kataloğu (ürün dışı) | Yalnızca merkezde; tek seferlik/aylık/yıllık, bitiş tarihi zorunlu; merkez → bayi doğrudan; distribütör kâr marjı olmadan alt bayiye tanımlar; distribütör bazlı özel fiyat; erken iptal bayi talep + admin onay; statik cayma bedeli; opsiyonel sözleşme; stok hareketi yok, sadece muhasebe; "modül paketi" tipi | otopoly-go `backend/internal/modules/billing` (plan/abonelik yaşam döngüsü referans) |
| Bayiler arası transfer | Distribütör onaylı; ledger ve muhasebe kaydı | olexfilms `app/Models/StockTransfer.php` (mevcut bayi-bayi transfer) |

### 5.3 Eklenti (varsayılan kapalı, üst seviye açar, ücretli olabilir)

| Modül | Kapsam | Referans repo ve yol |
| --- | --- | --- |
| AI asistan | Role özel tool kaydı, yazma tool'larında onay kartı, org başına aylık token kotası, SSE streaming, Anthropic SDK; stok tahmini ve performans verisini salt okur; WhatsApp'ta aynı yetkinlik; non-auth ziyaretçiye ürün tavsiyesi + en yakın bayi; sohbetler panelde + panelden mesaj | otopoly-go `backend/internal/modules/ai/{provider,tools,usecase,handler}`, `docs/AI.md`, `.agents/skills/ai-assistant/SKILL.md`, `migrations/000048-000050`; olexfilms-ai-layer-go `api/internal/ai/pipeline/engine.go`, `api/internal/tools/catalog/*`, `api/internal/ai/prompt/*` (tool kataloğu ve prompt kuralları) |
| WhatsApp gateway | wuzapi tek instance, tek numara, webhook → conversation → AI pipeline (kuyrukta); dosya ve görsel gönderimi; panelden yazma `sender_type=staff` | olexfilms-ai-layer-go `api/internal/channels/whatsapp/*`, `api/internal/notifications/*` (idempotent gönderim API'si); otopoly-go `backend/internal/modules/messaging/*` |
| MCP sunucuları | `/mcp/dealer`, `/mcp/customer`, `/mcp/user`; tek kod tabanı; tool listesi role göre dinamik; OAuth 2.1 + PKCE + DCR, Streamable HTTP; yazma tool'larında onay; org başına istek limiti | technowide-ecosystem `backend/internal/modules/mcp`, `docs/MCP.md`; olexfilms `app/Mcp/Servers/OlexSupportServer.php` (12 tool, kimliksiz, kaldırılacak) |
| Bayi vitrini ve lead formu | `olexfilms.app/bayi/{kod}`: harita, hizmetler, Google puanı, WhatsApp butonu, teklif formu → lead; "bayi bul" aynı veri | olexfilms `app/Http/Controllers/Api/V1/DealerController.php` (directory/nearby), `dealer-locations.json` |
| Filo müşterileri | Filo hesabı (e-posta + şifre), araçlar, toplu hizmet planı, dönemsel raporlar, kısmi muhasebe salt okunur | Yeni; müşteri ve hizmet modüllerine bağlanır |
| Sertifika | Sözleşme altyapısı gibi tanım + PDF yükleme + geçerlilik; kategori ve/veya ürünlere bağlanır; sertifikasız kayıt engellenmez, politika: müşteriye bildirim, statü değişiminde admin onayı; eğitim içeriği yok | otopoly-go contracts altyapısı (şablon/kayıt/geçerlilik deseni) |
| Stok tahmini ve sipariş önerisi | 30/90 gün tüketim + mevsimsellik, tükenme uyarısı (eşik org + ürün), tek tık sipariş taslağı, metrajda kalan metre, merkezde ağ talebi; algoritmik, minimum veri süresi | Yeni; ledger üzerinden |
| Performans ve hedef paneli | Hizmet sayısı, garanti başlatma oranı, ölçüm oranı, değerlendirme ortalaması, stok devir hızı, sözleşme durumu, cari gecikmesi; sözleşmeyle işlem hacmi hedefi; bayi aylık bireysel hedef; zayıf bayiye otomatik görev; bölge haritası ve bayi yoğunluğu | olexfilms `app/Filament/Widgets/{Admin,Dealer}/*`, `app/Services/MobileReports/*`, `docs/mobile-api/reports/*` |
| Verimlilik ve fire analizi | Parça başına metraj tüketimi, fire oranı, bayi karşılaştırması | Yeni; metraj ledger'ı üzerinden |
| Kampanya gönderimi | Filtreli kitleye zamanlanmış push/WhatsApp/e-posta; onay zinciri (bayi → distribütör; merkez serbest); zorunlu lokalizasyon (kitle taranır, diller listelenir, her dile içerik + dosya/görsel); SMS yok | olexfilms `app/Services/Push/*`, `app/Jobs/SendPushCampaign*.php`, `docs/mobile-api/push-campaigns.md`, `app/Filament/Resources/BulkSms` |
| Fotoğraf standardı | Araç kabulde zorunlu açılar, eksikse kabul tamamlanmaz, sözleşme PDF'ine gömülür; admin aç/kapa (varsayılan kapalı) | otopoly-go contracts `contract_media` (görsel ekleme deseni) |
| Değerlendirme | Google değerlendirme isteği (mevcut akış) + platform ve ürün kalitesi formu, işlenir ve raporlanır | olexfilms `app/Console/Commands` `services:send-review-request-sms`, `review_request_sms_sent_at` |
| E-fatura altyapısı (UBL-TR) | Admin tarafı UI'a dökülebilir; entegratör bağlantısı yok; bayi tarafında backend mapper hazır | go-ubltr (`builder`, `render`); otopoly-go `backend/internal/modules/billing/invoice/{mapper,render}.go`; technowide-ecosystem `backend/internal/modules/invoices`, `docs/UBLTR_PLAN.md` |
| Kısa URL | `olexfilms.app/s/{token}` | olexfilms `app/Models/ShortUrl.php` |

## 6. Teknik mimari

Taban kod otopoly-go'nun altyapı katmanıdır (organization = bayi/distribütör, platform = merkez); üzerine technowide-ecosystem'den SeaweedFS, env üretici, iOS push düzeltmesi, compose prod düzeni, izin scope'ları ve MCP modülü alınır. nextjs-go-boilerplate tek başına yeterli değildir: MinIO, oturum ve iOS push hataları orada hâlâ durur. (otopoly'nin ilk commit'i boilerplate HEAD ile birebir aynıdır; düzeltmeler sonraki commit'lerdedir.)

**Stack:** Go 1.26 `net/http` + sqlc + golang-migrate; Postgres 18; Redis; Asynq; Centrifugo v5; Meilisearch; SeaweedFS (S3); Gotenberg 8; wuzapi; Next.js 16 + React 19 + Tailwind 4 + shadcn (RTL logical properties); NextAuth BFF (tek uçuşlu refresh, refresh ömründe çerez); Anthropic SDK; sentry-go ve @sentry/nextjs.

**Alınan düzeltmeler:**

| Konu | Kaynak |
| --- | --- |
| SeaweedFS, private bucket, dosyalar API stream'inden | technowide-ecosystem commit `6ec2aa2`, `compose.prod.yml`, `backend/internal/platform/storage/s3.go` |
| iOS Safari'de panel açılmama (Notification yoksa web push atlanır) | technowide-ecosystem `8b40fd0`, `frontend/src/hooks/use-web-push-sync.ts` |
| Oturum: tek kullanımlık refresh + BFF single-flight + 120 sn yeniden kullanım | otopoly-go `b958a5d`, `b507024`, `frontend/src/lib/server/refresh-single-flight.ts`; technowide-ecosystem `e21711e`, `86ed3f9` |
| Compose prod: `<servis>.<ağ>` iç URL'ler, sadece frontend + centrifugo dokploy ağında, `/healthz`, build-gate | technowide-ecosystem `compose.prod.yml`, `54a3d98` |
| `scripts/gen-env-server.sh` aynen | technowide-ecosystem `bd64b9d` |
| BFF güvenliği (CSRF origin, path allowlist, Content-Length) | otopoly-go `57933d9`, `0f18173` |
| Backup / restore / dr-drill | technowide-ecosystem `scripts/{backup,restore,dr-drill}.sh` |
| CI: main yeşilse `v0.N` tag, Dokploy tag'den deploy | technowide-ecosystem `.github/workflows` |

**Prod compose container'ları (13):**

| # | Container | Görev | Port | Dışa açık |
| --- | --- | --- | --- | --- |
| 1 | postgres | Ana DB | 5432 | hayır |
| 2 | redis | Kuyruk, cache, rate limit, flag cache | 6379 | hayır |
| 3 | meilisearch | Arama indeksleri | 7700 | hayır |
| 4 | seaweedfs | S3 dosya deposu | 8333 | hayır |
| 5 | centrifugo | Realtime | 8000 | `wss.olexfilms.app` |
| 6 | gotenberg | HTML→PDF | 3000 | hayır |
| 7 | wuzapi | WhatsApp gateway | 8080 | hayır |
| 8 | migrate | DB migration, tek seferlik | — | hayır |
| 9 | backend | Go API, MCP, webhook alıcı | 8080 | hayır |
| 10 | worker-core | critical/default/low kuyrukları, scheduler lideri | — | hayır |
| 11 | worker-docs | `docs` kuyruğu: tüm PDF ve export; import girmez | — | hayır |
| 12 | frontend | Next.js panel, portal, public, BFF | 3000 | `olexfilms.app` |
| 13 | migrator | Eski DB'lerden aktarım; profil ile, varsayılan durur | — | hayır |

Dış ağa yalnızca frontend ve centrifugo bağlı. Lokal compose aynı liste + mailhog + adminer. Eski hub, depo ve AI katmanının 20'nin üzerindeki container'ı kapanır.

**Path yapısı (tek domain):**

| Path | Kim |
| --- | --- |
| `/`, `/bayi/{kod}`, `/garanti/{no}` | herkese açık |
| `/s/{token}` | kısa URL |
| `/panel` | merkez, distribütör, bayi (tek panel, org değiştirici) |
| `/portal` | müşteri (telefon OTP), filo (e-posta + şifre) |
| `/api/v1/...` | BFF → Go |
| `/mcp/dealer`, `/mcp/customer`, `/mcp/user` | MCP |
| `/hooks/wuzapi` | WhatsApp webhook |

**Veri modeli ilkeleri:**

- Tek Postgres, satır bazında `organization_id` + `brand_id`; RLS yok, scope uygulama katmanında; Meilisearch indeksleri org filtreli.
- Ledger append-only + projeksiyon: `stock_movements` ve `journal_entries` yazılır, güncel sahiplik/miktar/bakiye ayrı tablolarda türetilir.
- Modüller arası iletişim outbox + event bus ile zorunlu; Glorian senkronu ve migrator aynı olayları tüketir.
- Feature flag'ler DB'de, Redis 30 sn cache.
- Ledger ve muhasebe tablolarında `organization_id + created_at` indeksi; bildirim ve aktivite loglarında 90 gün otomatik temizlik.
- PDF şablonları DB'de HTML + değişken, Lexical ile düzenlenir; hizmet, ölçüm, sözleşme, sipariş fişi ve fatura görünümü aynı motor; isteğe bağlı ön üretim.
- WhatsApp: wuzapi webhook → conversation → AI pipeline kuyrukta; panelden mesaj `sender_type=staff`.
- Mobil API sözleşmesi ilk günden: Bearer JWT, org bağlamı, Expo push token, ölçüm kabul ucu, QR ile web giriş onayı. Referans: olexfilms `docs/mobile-api.md`, warehouse `docs/mobile-api/faz-1..32.md`.
- Test: modül usecase testleri, sipariş→stok→muhasebe uçtan uca senaryo, Playwright ile 5-6 ana akış.

**Kapasite:** 5 yıl %50 bileşik büyüme ile hizmet ~50 bin, stok birimi ~90 bin, ledger ~500 bin satır. Tek Postgres, tek Meili, iki worker yeterli; partition gerekmez. Worker'lar aynı imaj, kuyruk seti env ile; ikinci replika kod değişikliği gerektirmez.

## 7. Migrasyon planı

Migrator ayrı bir container; eski hub ve depo veritabanlarına (MariaDB) bağlanıp yeni Postgres'e aktarır, istenildiğinde ve tekrar çalıştırılabilir (idempotent, kaynak id eşleme tablosu). Olex profili aktif, Glorian profili kodda hazır ve kapalı.

| Taşınır | Kaynak | Not |
| --- | --- | --- |
| Bayiler, kullanıcılar, roller | hub `dealers`, `users`, `model_has_roles` | 151 bayi, 317 user; hepsi Türkiye distribütörü altına |
| Müşteriler | hub `customers` | user'a dönüşür; telefon normalize |
| Araç markası/modeli | hub `car_brands`, `car_models`, `car_brands.json` | 372 marka, 3.702 model, logo dosyaları S3'e |
| Ürün ve kategori | hub `products`, `product_categories`; depo `products`, `brands` | 121 ürün, brand ile eşleşir |
| Stok birimleri ve sahiplik | hub `stock_items` (11.968); depo `product_barcodes` (13.525) | Tek `units` tablosu; hub `dealer` konumu = bayi sahipliği, depo `placed` = merkez depo |
| Stok hareketleri | hub `stock_movements` (13.910); depo `stock_movements` | Ledger açılışı; sahiplik projeksiyonu bunlardan türetilir |
| Hizmet, kalem, görsel, durum logu | hub `services`, `service_items`, `service_images`, `service_status_logs` | 6.484 hizmet, 14.700 log |
| Garanti | hub `warranties` (8.789) | çift kayıtlar birleştirilir |
| Ölçüm raporları | hub `nexptg_reports` (122), `nexptg_report_measurements` (7.081), pivot | `measurement_results`; VIN eksikse "tamamlanacak" işaretli |
| Siparişler | hub `orders`, `order_items`, `order_item_stock` (1.393); depo `orders` | external_reference ile eşleşenler tek kayıt |
| Depo yapısı | depo `warehouses`, `warehouse_sites`, `warehouse_locations`, `bin_product_stocks` | 2 depo, 5 konum |
| Araç devirleri, kısa URL'ler, cihaz kayıtları | hub `service_customer_transfers`, `short_urls`, `nexptg_api_users` | |
| Arşiv (salt okunur tablo) | hub `sms_logs`, `notifications`, WhatsApp logları | yeni bildirim merkezine girmez |
| Taşınmaz | `queue_monitors`, `activity_log`, push kampanyaları, `cache`, `sessions`, fuar ayarları, Evolution API verisi, AI katmanı konuşmaları | |

**Glorian senkronu (yeni app depo modülünde yeniden yazılır):** `integration_connections` (key=glorian), katalog ve bayi pull (`updated_since`), barkod bulk upsert push, sipariş outbound (`external_reference` idempotency, ship/deliver/receive/cancel), `external_status` aynası, `inventory:reconcile` eşdeğeri drift raporu, `X-Inventory-Api-Version: 1`. Referans: warehouse `app/Services/Inventory/InventoryApiClient.php`, `InventoryClientResolver`, `app/Console/Commands/Sync*.php`, `docs/plans/inventory-incremental-sync.md`; hub `docs/inventory-api.md`, `docs/inventory-api-port-blueprint.md`.

**Kapatma sırası:** (1) yeni app prod'a çıkar, migrator ilk tam aktarımı yapar; (2) eski hub salt okunur, yeni app canlı, migrator fark aktarımı (delta) ile kapanış; (3) eski depo kapanır, Glorian connection yeni app'e yönlendirilir; (4) AI katmanı ve Evolution API kapanır, wuzapi tek numaraya geçer; (5) phpMyAdmin, eski Meili/Redis kopyaları silinir.

## 8. Fazlar

Altı faz; her faz kendi başına canlıya alınabilir. Süre tahmini verilmedi; sıralama bağımlılığa göredir.

```
F0 Altyapı ─◆─ F1 Çekirdek ─◆─ F2 Geçiş ─◆─ F3 Standart ─◆─ F4 AI/MCP ─◆─ F5 Eklenti
      OTP girişi     3 defter doğru   eski hub kapalı  araç kabul uçtan uca  eski AI kapalı
```

| Faz | İçerik | Kapı (bitmiş sayma şartı) |
| --- | --- | --- |
| F0 Altyapı | otopoly taban + ecosystem düzeltmeleri, compose (13 container), gen-env, CI/tag, Sentry, Centrifugo, Gotenberg, wuzapi bağlantısı, brand + org ağacı + territory + scope'lu RBAC, feature flag altyapısı, bildirim merkezi iskeleti, i18n + RTL | Boş panelde merkez/distribütör/bayi girişi, org değiştirici, WhatsApp OTP çalışıyor |
| F1 Çekirdek iş | Katalog, stok + ledger, tam depo, sipariş + fiyat zinciri + kur, hizmet ve garanti (araç kabul akışı, parça seçici, PDF), müşteri = user, muhasebe çekirdeği + hiyerarşik köprü, arama, import/export, görevler, sistem ayarları, mobil API sözleşmesi | Bir sipariş merkez→distribütör→bayi gidip hizmette tüketiliyor, garanti başlıyor, üç defterde doğru kayıt düşüyor |
| F2 Migrasyon ve geçiş | Migrator (Olex profili), Glorian senkron katmanı, müşteri portalı (araçlar, garanti, devir, bayi bul), public sayfalar, kısa URL, KVKK anonimleştirme | Eski hub salt okunur, yeni app canlı, Glorian depo senkronu çalışıyor, eski depo kapalı |
| F3 Standart modüller | Araç kabul sözleşmesi, ölçüm sonuçları (Bluetooth kabul, eşleme, fark tablosu, PDF), lead ve teklif + başvuru formu, randevu, duyuru, garanti talebi, tam bayi muhasebesi (personel, prim), hizmet kataloğu + modül paketi, bayiler arası transfer, değerlendirme formu | Bayi bir araç kabulünü sözleşme + ölçüm + hizmet olarak uçtan uca kapatıyor |
| F4 AI, WhatsApp, MCP | AI asistan (rol tool'ları, kota, onay), wuzapi konuşmaları + panelden mesaj, non-auth ziyaretçi akışı, üç MCP ucu (OAuth 2.1), kampanya gönderimi (onay zinciri, lokalizasyon), AI yönerge onayı | Eski AI katmanı ve Evolution kapalı |
| F5 Eklentiler | Bayi vitrini, filo, sertifika, stok tahmini, performans ve hedef (harita), verimlilik/fire, fotoğraf standardı, UBL-TR altyapısı, tavsiye fiyat | "Özellikler" sayfasında tüm eklentiler açılıp kapanabiliyor |

Mobil uygulama F5 sonrası ayrı proje; F1'deki API sözleşmesi değişmez.

## 9. Ertelenenler ve kapsam dışı

| Madde | Durum | Not |
| --- | --- | --- |
| Glorian'ın yeni app'e taşınması | Ertelendi | Kod marka-bilinçli, migrator profili hazır ve kapalı; senkron devam |
| Mobil uygulama | Ertelendi | Bu fazda yalnızca API sözleşmesi + auth; referans olexfilms `docs/mobile-api.md`, warehouse `docs/mobile-api/faz-*.md` |
| Garanti kartı Apple/Google Wallet | Ertelendi | |
| Ölçüm 2. faz: boya geçmişi, cihaz sağlığı | Ertelendi | |
| UBL-TR entegratör gönderimi | Ertelendi | Altyapı ve mapper hazır |
| Sesli asistan (Speaches) | Ertelendi | 14. container olur; otopoly-go `backend/internal/modules/ai/voice` |
| Vitrinde online randevu | Ertelendi | Randevu modülü kurulunca küçük iş |
| Gelecek sosyal modüller | Ertelendi | `center_social` rolü rezerve |
| Ürün orijinallik doğrulama sayfası | Kapsam dışı | |
| Açık API / webhook (3. taraf) | Kapsam dışı | |
| Taksit / vade planı | Kapsam dışı | Açıklama alanında takip |
| Fuar ekranı, Umami, NexPTG cihaz senkron API'si, eski import komutları | Kaldırılır | |
| Eğitim içerik kütüphanesi | Kapsam dışı | Sertifika yalnızca PDF + geçerlilik |
| Çok bölgeli deploy | Kapsam dışı | Tek sunucu |
| SMS kampanya kanalı | Kapsam dışı | SMS yalnızca bildirim kanalı, varsayılan kapalı |

## 10. Referans repolar ve kaynaklar

| Kaynak | GitHub | Yerel yol | Bu dokümanda kullanıldığı yer |
| --- | --- | --- | --- |
| olexfilms (hub) | https://github.com/furkanmeclis/olexfilms | /Users/furkanmeclis/Documents/Projects/olexfilms | Hizmet/garanti, müşteri, bildirim, push kampanya, araç parça seçici, NexPTG, Inventory + Chatbot M2M API, mobil API, kısa URL |
| olexfilms-warehouse | https://github.com/furkanmeclis/olexfilms-warehouse | /Users/furkanmeclis/Documents/Projects/olexfilms-warehouse | Depo, ledger, barkod, transfer, sayım, sipariş outbound, Glorian senkron, source-of-truth (D1–D12), mobil API fazları |
| olexfilms-ai-layer-go | https://github.com/furkanmeclis/olexfilms-ai-layer-go | /Users/furkanmeclis/Documents/Projects/olexfilms-ai-layer-go | AI pipeline, tool kataloğu, prompt kuralları, bildirim gönderim API'si (kaldırılıp içeri alınır) |
| glorian | https://github.com/furkanmeclis/glorian | /Users/furkanmeclis/Documents/Projects/glorian | Fork tespiti; senkron karşı tarafı |
| otopoly-go | https://github.com/furkanmeclis/otopoly-go | /Users/furkanmeclis/Documents/Projects/otopoly-go | Taban altyapı, organizations, oturum düzeltmesi, sözleşmeler, muhasebe (finance/cari), AI asistan, lead/teklif, todos, billing/entitlement, Gotenberg, messaging |
| nextjs-go-boilerplate | https://github.com/furkanmeclis/nextjs-go-boilerplate | /Users/furkanmeclis/Documents/Projects/nextjs-go-boilerplate | RBAC, I/O engine, bulk, searchengine, notifications, storage soyutlaması, modül konvansiyonu (`docs/EXTENDING.md`) |
| technowide-ecosystem | https://github.com/furkanmeclis/technowide-ecosystem | /Users/furkanmeclis/Documents/Projects/technowide-ecosystem | SeaweedFS, compose prod, gen-env-server.sh, iOS push fix, izin scope'ları, MCP, ledger/payments/invoices, errortracking alıcısı, backup scriptleri, CI tag |
| go-ubltr | https://github.com/furkanmeclis/go-ubltr | /Users/furkanmeclis/Documents/Projects/go-ubltr | UBL-TR e-fatura altyapısı |
| Sunucu | ssh technowide-spaceship | — | Veri boyutları ve yük (30.09.2026, salt okunur) |
| Bu doküman | — | olexfilms-ecosystem/docs/design.md | Ajanın okuduğu sürüm |
