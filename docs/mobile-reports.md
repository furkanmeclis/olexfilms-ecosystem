# Mobil rapor sözleşmesi (`/v1/reports`)

TEC-495 (F5-05f). Eski olexfilms mobil hub'ının 22 rapor ucu
(`/api/v1/mobile/reports/*`, `app/Services/MobileReports/*`) yeni tek sözleşmeye eşlendi.
Mobil uygulama (F5 sonrası) ve panel widget'ları (F5-05g) aynı uçları kullanır.
**Legacy alias yoktur** (F5 S22): `legacymobile` modülüne rapor eşlemesi eklenmez; mobil
uygulama bu sözleşmeye yazılır.

## 1. Uçlar

| Uç | Açıklama |
|---|---|
| `GET /v1/reports/catalog` | Çağıranın açabildiği raporlar (modül açık + izin + erişim), yerelleştirilmiş başlık, dönem/granülarite seçenekleri |
| `GET /v1/reports/overview` | Genel bakış kartları (`GET /v1/reports/overview` = `GET /v1/reports/{report}` ile `report=overview`) |
| `GET /v1/reports/{report}` | Tek rapor, ortak zarf |
| `GET /v1/reports/layout` | Kullanıcının aktif organizasyondaki widget düzeni; kayıt yoksa varsayılan (yalnız `overview`) |
| `PUT /v1/reports/layout` | Düzeni atomik değiştirir; tek geçersiz widget tüm isteği 400 ile reddeder, kayıtlı düzen değişmez |

Kimlik doğrulama F1 mobil API ilkeleriyle aynıdır: Bearer JWT (panel veya mobil oturum) ve aktif
organizasyon bağlamı (`RequireOrganization`). Rapor anahtarları:

| Anahtar | Tür | Modül | İzin | Dönem | Granülarite | limit |
|---|---|---|---|---|---|---|
| `overview` | summary | kart grubu başına | kart grubu başına | ✓ | – | – |
| `services.trend` | timeseries | services | services.read | ✓ | ✓ | – |
| `services.status_distribution` | distribution | services | services.read | ✓ | – | – |
| `services.top_brands` | ranking | services | services.read | ✓ | – | ✓ |
| `services.top_models` | ranking | services | services.read | ✓ | – | ✓ |
| `services.top_products` | ranking | services | services.read | ✓ | – | ✓ |
| `orders.trend` | timeseries | orders | orders.read | ✓ | ✓ | – |
| `orders.status_distribution` | distribution | orders | orders.read | ✓ | – | – |
| `customers.trend` | timeseries | customers | customers.read | ✓ | ✓ | – |
| `stock.summary` | summary | stock | stock.read | – (anlık) | – | – |
| `warranties.summary` | summary | services | warranties.read | ✓ | ✓ | – |
| `measurements.summary` | summary | measurements | measurements.read | ✓ | ✓ | – |
| `dealers.performance` | table | performance | performance.read | ✓ (range_to ayı) | – | ✓ |
| `dealers.top_by_warranty` | ranking | services | warranties.read (subtree/brand/all) | ✓ | – | ✓ |
| `activities.recent` | table | services | services.read | – (son N) | – | ✓ |

## 2. Ortak zarf

Her rapor aynı anahtarları döndürür (kullanılmayanlar `null` veya boş dizi):

```json
{
  "report": "services.trend",
  "kind": "timeseries",
  "title": "Hizmet trendi",
  "locale": "tr",
  "timezone": "Europe/Istanbul",
  "scope": "managed",
  "period": "30d",
  "range_from": "2026-09-10",
  "range_to": "2026-10-09",
  "granularity": "day",
  "generated_at": "2026-10-09T09:00:00Z",
  "series": [
    { "key": "created", "label": "Oluşturulan", "unit": "count",
      "points": [{ "key": "2026-09-10", "label": "2026-09-10", "value": 3 }] }
  ],
  "columns": [],
  "rows": []
}
```

* `series[].points[].key`: zaman serisinde kova başlangıç tarihi (hafta = Pazartesi, ay = ayın 1'i,
  boş kovalar 0 ile doldurulur), dağılımda durum anahtarı (bilinen tüm durumlar sabit sırada),
  sıralamada kaydın uuid'i, özet raporda kart anahtarı (`series.key = "cards"`).
* `columns` / `rows`: yalnız tablo ve sıralama raporlarında; `columns[].label` yerelleştirilmiş.
* `label`'lar 13 dilde (`locale` parametresi, yoksa K10 zinciri: kullanıcı → org → merkez →
  Accept-Language → tr).
* Dönem: `period` = `7d`, `30d` (varsayılan), `90d`, `12m` (bugün dahil, kullanıcı saat
  diliminde) veya `range_from` + `range_to` (dahil, en fazla 731 gün, `period = "custom"`).
  Granülarite varsayılanı: ≤31 gün `day`, ≤120 gün veya 90d `week`, üstü veya 12m `month`;
  `day` en fazla 366 kova. Raporun almadığı parametre 400 `VALIDATION_ERROR`.

## 3. Erişim (eski `ReportAccessResolver` → RBAC + scope)

| Eski kural | Yeni karşılık |
|---|---|
| super_admin / center_staff tüm bayiler, `dealer_id` opsiyonel filtre | Merkez rolleri `brand` scope'u ile markanın tamamını okur. `dealer_id` filtresi yok: tek bayiye bakmak için panelde o organizasyona geçilir (aktif org bağlamı). |
| dealer_owner / dealer_staff yalnız kendi bayisi, `dealer_id` 422 | Bayi rolleri `managed` scope'u ile yalnız kendi organizasyonunu okur. |
| (yok) | Distribütör `subtree` scope'u ile kendi alt ağacını okur. |
| `dealers/performance` dealer_staff 403, dealer_owner tek satır | `performance.read` izni (dealer_staff'ta yok); bayi kendi satırını, merkez/distribütör sıralamayı görür. |
| `dealers/top-by-warranty` yalnız merkez | `warranties.read` + çoklu organizasyon erişimi (subtree/brand/all); bayi için 403. |
| `users/role-distribution` dealer_staff 403 | Kaldırıldı (aşağıda). |
| Overview'da bayi kapsamında `total_dealers` gizli | Bayi sayısı kartları yalnız `organizations.read` subtree/brand/all erişiminde. |
| (yok) | Raporun modülü kapalıysa 403 `FEATURE_DISABLED`; rapor `catalog`'da görünmez, layout GET'inden düşer, layout PUT'u reddeder. |

Own/assigned kapsamlı özel roller kendi oluşturdukları kayıtları görür (hizmet, sipariş, garanti,
ölçüm); bu kapsamı desteklemeyen raporlar (müşteri, stok, bayi raporları) 403 döner.
Müşteri ve filo oturumları raporları açamaz.

## 4. Parite tablosu (22 uç)

| # | Eski uç (`/api/v1/mobile/reports`) | Yeni uç | Durum / gerekçe |
|---|---|---|---|
| 1 | `GET` + `PUT /layout` | `GET` + `PUT /v1/reports/layout` | Eşlendi. `kind` → `report` (yeni anahtarlar), `version` = 1, atomik replace, varsayılan overview widget'ı aynı id ile. Erişilemeyen rapor, tekrar eden rapor/id, dönemsiz rapora dönem 400 (eskiden 422). Tablo: `report_layouts` (user × org). |
| 2 | `GET /overview` | `GET /v1/reports/overview` | Eşlendi. Kartlar alan bazında kapılı (modül + izin); özet grafikler ayrı widget'lara bırakıldı. |
| 3 | `GET /services/trend` | `services.trend` | Eşlendi; `created` + `completed` serileri. `group_by` → `granularity`. |
| 4 | `GET /services/status-distribution` | `services.status_distribution` | Eşlendi. |
| 5 | `GET /services/top-car-brands` | `services.top_brands` | Eşlendi (dönemde tamamlanan hizmetler). |
| 6 | `GET /services/top-car-models` | `services.top_models` | Eşlendi. |
| 7 | `GET /services/top-products` | `services.top_products` | Eşlendi. |
| 8 | `GET /orders/trend` | `orders.trend` | Eşlendi; `created` + `received` serileri. |
| 9 | `GET /orders/status-distribution` | `orders.status_distribution` | Eşlendi. |
| 10 | `GET /customers/trend` | `customers.trend` | Eşlendi; seriler müşteri tipine göre (`individual`, `corporate`). |
| 11 | `GET /customers/type-distribution` | `customers.trend` | Birleştirildi: tip kırılımı trend serileri olarak gelir; toplamlar seri toplamıdır. |
| 12 | `GET /stock/summary` | `stock.summary` | Eşlendi; anlık (dönemsiz) kartlar. |
| 13 | `GET /stock/status-distribution` | `stock.summary` | Birleştirildi: `units` serisi seri birim durum dağılımıdır. |
| 14 | `GET /warranty/summary` | `warranties.summary` | Eşlendi; kartlar + durum dağılımı + başlayan garanti trendi. |
| 15 | `GET /warranty/trend` | `warranties.summary` | Birleştirildi: `started` zaman serisi. |
| 16 | `GET /warranty/status-distribution` | `warranties.summary` | Birleştirildi: `status` serisi. |
| 17 | `GET /operations/monthly-trends` | `services.trend` + `orders.trend` + `customers.trend` | Birleştirildi: çok serili grafik istemci tarafında üç raporun `granularity=month` serilerinden çizilir; ayrı uç gereksiz. |
| 18 | `GET /activities/recent-services` | `activities.recent` | Eşlendi; tablo (`columns` + `rows`). |
| 19 | `GET /dealers/performance` | `dealers.performance` | Eşlendi; F5-05c performans projeksiyonundan okur (`performance` modülü). |
| 20 | `GET /dealers/top-by-warranty` | `dealers.top_by_warranty` | Eşlendi. |
| 21 | `GET /nexptg/summary` | `measurements.summary` | Eşlendi; NexPTG ölçümleri yeni sistemde `measurement_results` (ölçüm modülü). |
| 22 | `GET /users/role-distribution` | – | Kaldırıldı: üye/rol yönetimi ekranı (`members.read`) aynı bilgiyi filtreli listeyle verir; mobil ana ekran widget'ı olarak kullanım değeri düşük. |

Eski ortak query parametrelerinin karşılığı: `locale` → `locale`, `period` → `period`,
`date_from`/`date_to` → `range_from`/`range_to`, `group_by` → `granularity`, `limit` → `limit`,
`timezone` → kullanıcı/organizasyon saat dilimi (K10), `dealer_id` → aktif organizasyon (kaldırıldı).
Eski zarftaki `theme` (renk paleti) kaldırıldı: renkler istemcinin tasarım sistemindedir.
