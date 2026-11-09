# Revize 09.11 — durum takibi

Kaynak: [huseyin-revize.md](huseyin-revize.md). Linear etiketi: `revize-0911` (proje "Olexfilms Tek App").
Koordinatör gece çalışması, 2026-11-09. PR'lar merge EDİLMEDİ; merge sırası en altta.

Durum kısaltmaları: ⏳ çalışıyor · 🟢 CI yeşil · 🔴 CI kırık · ⏸ sırada · 🟡 karar bekliyor

## Migration numaraları (merkezi dağıtım)

| No | İş | Not |
| --- | --- | --- |
| 000166 | TEC-1047 / TEC-1052 | KULLANILMADI (boşluk) |
| 000167 | TEC-1050 | KULLANILMADI (boşluk) |
| 000168 | TEC-1052 | KULLANILMADI (boşluk) |
| 000169 | TEC-1062 | bekleme havuzu |
| 000170 | TEC-1060 | activity_events kısmi indeks |
| 000171 | TEC-1061 | products.dealer_orderable |
| 000172 | TEC-1070 | organizations fatura kolonları + iade kuyruğu |
| 000173 | TEC-1064 | bayi iskontosu, kalem iskonto/hediye |
| 000174 | TEC-1058 | order_items.plotter_cut |
| 000175 | TEC-1076 | yalnız gerekirse |
| 000176 | RV-Karar rol izinleri | center_sales rolü, dist. depo/muhasebe izinleri |
| 000177 | RV-Karar iade | stock_transfer_requests.order_id (planlı) |

Not: golang-migrate boşluklara izin verir; ancak PR'lar migration numarası **artan sırayla** merge edilmeli (yüksek numara önce uygulanırsa düşük numara prod'da atlanır).

## 1. Hata ve kritikler

| Madde | Linear | PR | CI | Migration | Durum / not |
| --- | --- | --- | --- | --- | --- |
| 7.5 Web+API sabit barkodlu ürün siparişe eklenemiyor | TEC-1047 | [#508](https://github.com/furkanmeclis/olexfilms-ecosystem/pull/508) | 🟢 | — | kök neden: hazırlamada adetsiz okutma 400, tekrar okutma 409, öneri yalnız konumlu adet; üçü düzeltildi |
| 5.2 + 5.10 Web+API depo seçimi, bildirim, aramasız ürün listesi, KALEMLER | TEC-1052 | [#510](https://github.com/furkanmeclis/olexfilms-ecosystem/pull/510) | 🟢 | — | #508 üstünde; `self_prepare` ("ben hazırlayacağım") eklendi; ürün popülerlik sıralaması üründe yok, ada göre |
| 5.4 Web+API depo fiyat görmesin | TEC-1050 | [#511](https://github.com/furkanmeclis/olexfilms-ecosystem/pull/511) | 🟢 | — | izin tabanlı `SeesPrices()`; sipariş, transfer, export, fiş; dealer_staff/center_service de artık fiyat görmüyor (soru) |
| 7 Mobil depo değişince birimler | TEC-1049 | [#509](https://github.com/furkanmeclis/olexfilms-ecosystem/pull/509) | 🟢 | — | 🟢 kök neden: depo seçimi hiçbir listeye gitmiyordu; sipariş listesi depo filtresi backend'de yok → TEC-1054 |
| 7.5 Mobil | TEC-1048 | [#509](https://github.com/furkanmeclis/olexfilms-ecosystem/pull/509) | 🟢 | — | 🟢 mobil UI engeli yok; depo hazırlamada adet/SKU okutma düzeltildi |
| 5.2.1 Mobil depo seçimi | TEC-1053 | [#509](https://github.com/furkanmeclis/olexfilms-ecosystem/pull/509) | 🟢 | — | 🟢 depo seçimi eklendi; "kendim hazırlayacağım" API'si TEC-1052'den sonra |
| 5.4 Mobil fiyat gizleme | TEC-1051 | [#509](https://github.com/furkanmeclis/olexfilms-ecosystem/pull/509) | 🟢 | — | 🟢 depo alanı/rolünde fiyat gizli |

## 2. Sipariş akışı

| Madde | Linear | PR | CI | Migration | Durum |
| --- | --- | --- | --- | --- | --- |
| 5 Web+API sipariş listesi alanları | TEC-1054 | [#514](https://github.com/furkanmeclis/olexfilms-ecosystem/pull/514) | 🟢 | — | #510 üstünde; depo, adet/metre, USD + anlık TL, ilgilenen; `seller_warehouse` filtresi |
| 5 + 5.1 Mobil | TEC-1055 | | | — | ⏳ mobil paket 2 (#518 + #509 üstünde) |
| 5.5 + 5.9 Web+API sipariş düzenleme | TEC-1056 | [#521](https://github.com/furkanmeclis/olexfilms-ecosystem/pull/521) | 🟢 | — | #518 üstünde; sevk öncesi satış+depo düzenler, atanmış altına 409 |
| 5.5 + 5.9 Mobil | TEC-1057 | | | | ⏸ |
| 5.6 Web+API plotter işareti + rapor | TEC-1058 | [#524](https://github.com/furkanmeclis/olexfilms-ecosystem/pull/524) | 🟢 | 000174 | #521 üstünde; kalem anahtarı + PATCH, aylık rapor `orders.plotter_cut` |
| 5.6 + 5.4.1 Mobil | TEC-1059 | | | — | ⏳ hareket geçmişi mobil paket 2'de; plotter TEC-1058 sonrası |
| 5.4.1 Web+API hareket geçmişi | TEC-1060 | [#518](https://github.com/furkanmeclis/olexfilms-ecosystem/pull/518) | 🟢 | 000170 | #514 üstünde; sipariş/transfer/stok girişi zaman çizelgesi (activity_events) |
| 5.7 Web+API siparişe açık ürün | TEC-1061 | [#517](https://github.com/furkanmeclis/olexfilms-ecosystem/pull/517) | 🟢 | 000171 | #510 üstünde; kapalı ürün merkez dışı kataloglarda gizli, bayi siparişinde PRODUCT_NOT_ORDERABLE |
| KRİTİK Web+API yeni teslim yeri / bekleme havuzu | TEC-1062 | [#515](https://github.com/furkanmeclis/olexfilms-ecosystem/pull/515) | 🟢 | 000169 | alıcı = pending dealer org + pending_recipients; Ayarlar > Hesaplar > Bekleme havuzu |
| KRİTİK Mobil | TEC-1063 | | | | ⏸ |

## 3. Fiyatlandırma

| Madde | Linear | PR | CI | Migration | Durum |
| --- | --- | --- | --- | --- | --- |
| 5.3 Web+API USD, iskonto, hediye | TEC-1064 | [#522](https://github.com/furkanmeclis/olexfilms-ecosystem/pull/522) | 🟢 | 000173 | #510 üstünde; bayi iskontoları sayfası, quote ucu, hediye kalem; USD geçişi veri/ayar (aşağıda) |
| 5.3 Mobil | TEC-1065 | | | | ⏸ |

## 4. Rol ekranları

| Madde | Linear | PR | CI | Migration | Durum |
| --- | --- | --- | --- | --- | --- |
| 7.1 Web depo ana sayfası | TEC-1066 | [#519](https://github.com/furkanmeclis/olexfilms-ecosystem/pull/519) | 🟢 | — | #512 üstünde; depo ana sayfası, salt okunur Bayiler sayfası, warehouse-summary ucu |
| 7.2 + 7.3 Mobil depo ana sayfa + kart | TEC-1067 | | | — | ⏳ mobil paket 2 (#518 + #509 üstünde) |
| 5.8 Mobil satış görevlisi | TEC-1068 | [#525](https://github.com/furkanmeclis/olexfilms-ecosystem/pull/525) | 🟢 | — | alt bar + satış panosu (istemci hesaplı, backend rapor ucu sonraki iş) |
| 6 Mobil stok depo filtresi | TEC-1069 | [#525](https://github.com/furkanmeclis/olexfilms-ecosystem/pull/525) | 🟢 | — | depo çipleri (1000 birim sınırı; backend toplam ucu sonraki iş) |
| 8 + 8.1 Web+API muhasebe | TEC-1070 | [#523](https://github.com/furkanmeclis/olexfilms-ecosystem/pull/523) | 🟢 | 000172 | #512 üstünde; muhasebe ana sayfası, kuyruk (faturalanacak/yolda/iadeler), cari fatura bilgileri |
| 8.1 Mobil muhasebe kuyruğu | TEC-1071 | | | | ⏸ |

## 5. Merkez ekranları

| Madde | Linear | PR | CI | Migration | Durum |
| --- | --- | --- | --- | --- | --- |
| 1 Web+API genel bakış | TEC-1072 | [#516](https://github.com/furkanmeclis/olexfilms-ecosystem/pull/516) | 🟢 | — | dönem seçici, en çok kullanılan ürün grafiği, distribütör sayısı |
| 1.1 + 3.1 Mobil | TEC-1073 | [#525](https://github.com/furkanmeclis/olexfilms-ecosystem/pull/525) | 🟢 | — | hizmet panosu, kartta plaka + bayi |
| 2 + 4 Web menü, Müşteriler ve Araçlar | TEC-1074 | [#512](https://github.com/furkanmeclis/olexfilms-ecosystem/pull/512) | 🟢 | — | legacy sıra; "açan bayi" = ilk müşteri-org bağlantısı (varsayım) |
| 3 Web+API hizmet listesi | TEC-1075 | [#513](https://github.com/furkanmeclis/olexfilms-ecosystem/pull/513) | 🟢 | — | stats ucu; defaultHidden kök nedeni useDataTable (genel düzeltme); "Olex Merkez" = TEC-1003 veri durumu, artık servis noktası adı |

## 6. Hizmet oluşturma

| Madde | Linear | PR | CI | Migration | Durum |
| --- | --- | --- | --- | --- | --- |
| 9 + 9.1 + 9.2 Web sihirbaz | TEC-1076 | | | 000175? | ⏳ |
| 9.3 Mobil gövde tekrar seçimi | TEC-1077 | [#525](https://github.com/furkanmeclis/olexfilms-ecosystem/pull/525) | 🟢 | — | bölge bir kez seçilir, özet görseli |

## 7. Metrajlı çıkış

| Madde | Linear | PR | CI | Migration | Durum |
| --- | --- | --- | --- | --- | --- |
| Metraj Web+API `KOD-3M` | TEC-1078 | [#520](https://github.com/furkanmeclis/olexfilms-ecosystem/pull/520) | 🟢 | — | #508 üstünde; `-3M`, `-2.5M`, `-3M-2`; birim geçmişinde parçalar, `family=` filtresi |
| Metraj Mobil | TEC-1079 | | | | ⏸ |

## Karar gerektirenler (kodlama yok)

| Madde | Linear | Durum |
| --- | --- | --- |
| Bayi/Distribütör, Olex Merkez = AFM Florya, Florya'dan AFM Florya'ya sipariş | TEC-1080 | 🟡 analiz Linear yorumunda, karar bekliyor |
| 5.7 distribütör bazlı | TEC-1081 | 🟡 analiz Linear yorumunda, karar bekliyor |
| 9.2 sözleşme ek fotoğraf | TEC-1082 | 🟡 analiz Linear yorumunda, karar bekliyor |
| 9.3? mobil araç kabul / sözleşme | TEC-1083 ([#525](https://github.com/furkanmeclis/olexfilms-ecosystem/pull/525): durum kartı, web'de imzalat + QR; mobil imza için contracts aynaya eklenmeli) | 🟡 analiz Linear yorumunda, karar bekliyor |
| Kodların kredili kullanımı | TEC-1084 | 🟡 analiz Linear yorumunda, karar bekliyor |
| İadelerin oluşturulması (kapsam) | TEC-1085 | 🟡 analiz Linear yorumunda, karar bekliyor |
| 5.1 dipnot metrajlı sipariş yolu | TEC-1086 | 🟡 analiz Linear yorumunda, karar bekliyor |
| 7.4 mobil Yeni Giriş (ertelendi) | TEC-1087 | 🟡 analiz Linear yorumunda, karar bekliyor |
| Kapatılabilecek modüller (sunucuda hiçbir şey kapatılmadı) | TEC-1088 | 🟡 analiz Linear yorumunda, karar bekliyor |

## Merge sonrası entegrasyon adımları

- #511 (fiyat gizleme) merge olduktan sonra: #514'ün eklediği `live_try_rate`/`live_total_try` alanları `OrderView.hidePrices()`'a eklenmeli, `orders-list-page.tsx`'teki yerel izin kontrolü `useCanSeePrices()`'a geçmeli. **O zamana kadar bu iki alan API'de fiyat izni olmayan kullanıcıya dönüyor.** Aynısı #517/TEC-1064 yeni fiyat alanları için.

## Kullanıcı yapacak (veri/ayar, sunucuda — ben yapmadım)

- USD baz fiyat (TEC-1064): `brands.currency` = USD (API ucu yok, DB güncellemesi); merkez fiyat listesine USD satırları; USD→TRY kuru tabloda olmalı (yoksa onay `RATE_NOT_FOUND`).
- AFM Florya kararı (TEC-1080) seçilirse 1b: yeni bayi org'u açılması.
- Modül kapatma (TEC-1088) kararı.

## Kararlar (kullanıcı 09.11 sabah kararları koordinatöre bıraktı — revizeye göre verildi)

| Konu | Karar | Uygulama |
| --- | --- | --- |
| K0 AFM Florya (TEC-1080) | **1b**: AFM Florya ayrı bayi org'u olarak açılır (merkeze bağlı, kendi carisi); eski hizmet/garanti merkezde tarihçe kalır; sipariş Florya deposundan AFM Florya bayisine açılır. TEC-1003 birleşmesi geri alınmaz. | Kod gerekmez. **Kullanıcı:** panelden yeni bayi aç (Ayarlar > Hesaplar), AFM Florya personelini bayi rollerine ekle. design.md güncellendi. |
| Metraj biçimi (TEC-1078) | `2.5M`, ikinci kesim `-3M-2` | #520'de olduğu gibi |
| Modüller (TEC-1088) | Bayi standardında kapatılması önerilen: leads, appointments, dealer_accounting, service_catalog, intake_contracts (mobil sözleşme gelene kadar); açık kalacak: dealer_transfers, announcements, warranty_claims, measurements (sekme rol bazlı gizlendi). Eklentiler kapalı kalsın. | **Sunucuda kapatma yapılmadı** (yasak); kullanıcı panelden Özellikler sayfasından kapatır. |
| İade (TEC-1085) | (b) uygulanır: merkez bayi adına iade açar, iade siparişe bağlanır, muhasebe kuyruğu İadeler sekmesinde. Faturalı iade ertelendi. | Kodlanıyor (000177) |
| Kredili kod (TEC-1084) | Açılış kapsamı dışı; revize "birlikte karar alacağız" diyor. | Yok |
| 9.3? mobil sözleşme (TEC-1083) | Seçenek 3: mobil detayda sözleşme durumu + imzalat; `CONTRACT_REQUIRED` anlaşılır mesaj; `intake_required` kapalı kalır. | Mobil paket 3a'ya eklendi |
| 9.2 ek fotoğraf (TEC-1082) | "Hasar / not fotoğrafları (isteğe bağlı)", katlanır alan + ipucu | TEC-1076 PR'ına eklendi |
| 5.7 distribütör bazlı (TEC-1081) | Şimdilik yok (TR'de distribütör yok); genel anahtar yeterli. Kapalı ürünler global aramadan da gizlenir. | Arama gizleme kodlanıyor |
| 7.4 (TEC-1087) | Mobil "Yeni giriş" kısayolu yalnız merkez yöneticisine | Mobil paket 2'ye eklendi |
| 5.4 staff fiyatı | dealer_staff / center_service fiyat görmez (design.md) | #511 olduğu gibi |
| 5.3 satış temsilcisi fiyatı | Yeni rol **center_sales**: fiyat okuma/yazma + iskonto; alış fiyatı yok | Kodlanıyor (000176). **Kullanıcı:** Zorbey'i center_sales rolüne ata |
| 5.3 K8 liste fiyatı | Merkeze bağlı bayi liste fiyatı = `sale_to_distributor_price`, iskonto bunun üzerine | #522 olduğu gibi |
| 5.3 ilk sipariş | Legacy siparişler sayılır (mevcut bayi ilk sipariş iskontosu almaz) | #522 olduğu gibi |
| 3 hizmet listesi | Merkezde varsayılan görünüm **Ağ** (bayi adları görünsün) | Kodlanıyor |
| KRİTİK cari | Üyesiz alıcıda (havuz/teslim yeri) merkez "teslim alındı"yı alıcı adına yapabilir → cari K9 ile oluşur | Kodlanıyor |
| KRİTİK seçiciler | Havuzdaki (pending) org'lar sipariş alıcı seçicisi dışındaki seçicilerde gizli | Kodlanıyor |
| 5.4.1 bayi zaman çizelgesi | Bayi depocu adını ve barkodları görebilir (şeffaflık) | #518 olduğu gibi |
| 5.6 plotter raporu | Tüm işaretli kalemler (ürün kırılımı yeterli) | #524 olduğu gibi |
| 7.1 dist. depo Bayiler | distributor_warehouse_staff'a organizations.read | Kodlanıyor (000176) |
| 8 dist. muhasebe | distributor_accounting'e einvoice.manage; "Faturalandı" sevk/teslim edilmiş siparişte de konabilir (legacy gibi); muhasebe ana menüsünde katalog gizli | Kodlanıyor (000176 + muhasebe takip) |
| 7.1 test hesabı | Depo hesabının rolü panelden kontrol edilmeli (center_warehouse müşteri görmez) | **Kullanıcı** |

## Eski soru listesi (yukarıda karara bağlandı)

- **K0 (TEC-1080):** AFM Florya ayrı bayi mi olsun? Öneri 1b (yeni bayi org'u aç, eski veri merkezde kalsın) veya 1a (org-merge undo). Kök neden: AFM Florya TEC-1003 ile merkeze birleşti; sipariş alıcısı olarak seçilebilecek org yok, alıcı=satıcı=merkez olunca cari oluşmuyor.
- **Metraj (TEC-1078):** ondalık metre `2.5M` mi `250CM` mi? Aynı metrede ikinci kesim `-3M-2` uygun mu? (Varsayım: 2.5M ve -3M-2.)
- **Modüller (TEC-1088):** hangi modüller geçici kapatılsın? Liste Linear'da; sunucuda hiçbir şey kapatılmadı.
- **İade (TEC-1085):** merkezin bayi adına iade açması + iadenin siparişe bağlanması açılıştan önce mi?
- **Kredi (TEC-1084):** "kredili kod" = veresiye kod kullanımı mı, kredi paketi mi?
- **9.3? (TEC-1083):** mobilde sözleşme — öneri: açılışta sonradan tamamlama, sonra tam adım.
- **5.7 dist. (TEC-1081):** distribütör bazlı görünürlük ilk geçişte gerekli mi?
- **5.4 (TEC-1050):** `dealer_staff` ve `center_service` (AFM Florya hizmet personeli) rollerinde `pricing.*` izni yok → artık sipariş tutarı görmüyorlar (design.md "staff fiyat görmez" ile uyumlu). Görmeleri gerekiyorsa rol paketine fiyat okuma izni eklenir (migration).
- **3 (TEC-1075):** Merkez hizmet listesi varsayılan "Kendi" görünümünde açılıyor; "Ağ" varsayılan olsun mu?
- **KRİTİK (TEC-1062):** Cari kaydı yalnız alıcı "teslim alındı" deyince oluşuyor; üyesiz perakende teslim yerinde bunu yapacak kimse yok → cari oluşmaz. Merkez "teslim alındı"yı alıcı adına yapabilsin mi?
- **KRİTİK (TEC-1062):** pending alıcılar diğer org seçicilerinde (görev, transfer) de görünüyor; filtrelensin mi?
- **5.7 (TEC-1061):** Siparişe kapalı ürünler global aramada (Meilisearch) hâlâ görünüyor; aramadan da gizlensin mi?
- **5.4.1 (TEC-1060):** Bayi kendi siparişinin zaman çizelgesinde depocunun adını ve okuttuğu barkodları görüyor; uygun mu?
- **5.3 (TEC-1064) KRİTİK:** Satış temsilcisi (`center_staff`) rolünde `pricing.sale.read/write` yok → Zorbey fiyat ve iskonto göremiyor. Revize görmesini istiyor: rol paketine fiyat izni eklensin mi (migration)?
- **5.3 (TEC-1064):** Merkeze doğrudan bağlı bayinin liste fiyatı = `sale_to_distributor_price` varsayıldı, iskonto bunun üzerine (K8 açık sorusu).
- **5.3 (TEC-1064):** "İlk sipariş" sayımına legacy siparişler dahil → mevcut bayiler ilk sipariş iskontosu almaz. Uygun mu?
- **7.1 (TEC-1066):** Revizede depo hesabı "Müşteriler"i görüyor; ama center_warehouse rolü müşteri okuyamıyor → test hesabı büyük ihtimalle center_staff/owner rolüyle açılmış, kontrol et.
- **7.1 (TEC-1066):** distributor_warehouse_staff `organizations.read` tutmuyor → Bayiler sayfasını göremez; izin eklensin mi (migration)?
- **8 (TEC-1070):** distributor_accounting `einvoice.manage` tutmuyor → "Faturalandı" butonu yok; izin eklensin mi? Muhasebe menüsünde katalog açık (fiyat yönetimi yetkisi); gizlensin mi? Faturalandı yalnız teslim alınmış siparişe (TEC-1030 kuralı) — legacy gibi sevkte de olsun mu?
- **7.4 (TEC-1087):** mobil "Yeni giriş" kısayolu yalnız merkez yöneticisine mi gösterilsin?

## Önerilen merge sırası

(Sabah raporunda kesinleşecek.)
