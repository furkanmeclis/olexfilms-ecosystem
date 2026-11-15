# Rapor zarfı v2 (TEC-1210)

Bu belge `/v1/reports` uçlarının v2 veri sözleşmesidir. Web (TEC-1212) ve mobil (TEC-1213)
ekranları buna dayanır. Temel sözleşme `docs/mobile-reports.md` (TEC-495) içindedir. v2
geriye uyumludur: eski alanlar kalır, yeni alanlar her yanıtta gelir (değer yoksa `null` ya da
boş dizi). OpenAPI'de yeni alanlar `required` listesine girmedi. Sebep: istemcilerdeki eski
test fikstürleri kırılmasın. Backend bu alanları her zaman gönderir.

## Parametreler

`GET /v1/reports/{report}`, `GET /v1/reports/overview` ve `GET /v1/reports/{report}/export`:

* `period`, `range_from`, `range_to`, `granularity`, `limit`, `all_time`, `locale` aynen kalır.
* `compare=none|previous|year`. Varsayılan `none`.
  * `previous`: hemen önceki aynı gün sayısı. `12m` için önceki 12 ay.
    `dealers.performance` için bir önceki ay, çünkü rapor `range_to` ayını okur.
  * `year`: geçen yılın aynı tarihleri.
  * Dönemsiz raporlar (`stock.summary`, `activities.recent`) ve `all_time=true` bu parametreyi
    yok sayar. Bu durumda `period_info.compare` `none` olur. Geçersiz değer 400 `compare` döner.

## Zarfa eklenenler

* `period_info`: `{ from, to, granularity, compare, previous_from, previous_to }`. Dönemsiz
  raporda ve `all_time` ile `null` gelir. Granüler olmayan raporda `granularity`, KPI
  spark kovasıdır.
  **Sözleşmeden sapma:** Sözleşmede adı `period`. Ancak `period` eski dönem string'idir
  (`"30d"`) ve geriye uyumluluk için kaldı.
* `series[]`: `total` (noktaların toplamı; `cards` serisinde `null`), `previous_total`,
  `delta_pct` (bir ondalık; önceki `null` ya da 0 ise `null`).
* `series[].points[]`: `previous`. Zaman serisinde aynı sıradaki kovadır. Dağılım ve
  sıralamada aynı anahtardır; önceki dönemde olmayan kalem için 0 (sıralamalarda: önceki dönemin ilk `limit` kalemi
  içinde olmayan). `compare=none` ise `null`.
* `kpis[]`: `{ key, label, unit, value, previous, delta_pct, good, spark, link }`.
  * `unit`: `count | pieces | meters | hours`.
  * `good`: `up | down`.
  * `spark`: dönemin kova değerleri, en fazla son 30.
  * Anlık KPI'larda `previous` `null` ve `spark` boştur: açık hizmet/sipariş, toplam müşteri,
    aktif/biten/iptal/yaklaşan garanti, stok, bayi sayıları, açık/geciken sipariş.
  * `link`: `{ kind: "report", report: "<anahtar>" }` (detay raporu) ya da `null`. KPI'nın
    kendi raporuna link verilmez.
  * Overview ve özet raporları kartlarını `kpis[]`'e koyar. Eski `cards` serisi kalır.
  * Özet raporlarında durum dağılımı (`status`, `units`) ve zaman serisi ayrı serilerdir.
* `columns[]`:
  * `type`: `text | number | money | percent | date | datetime | duration | status | org | link`.
    Eski `string` ve `integer` artık gönderilmez; yerlerine `text` ve `number` gelir.
    `percent` değeri 0..1 oranıdır. `duration` saattir.
  * `align`: `left | right`.
  * `aggregate`: `sum | avg | none`.
  * `currency`: kolon para birimi. `null` ise satırın `currency` alanı kullanılır.
* `rows[]`:
  * `_link: { kind: service | order | customer | organization | warranty | measurement | product, uuid }`.
  * `_status: { <kolon anahtarı>: { label, tone: neutral | progress | done | problem } }`.
* Katalog öğesi: `compares` (dönemli raporda `[none, previous, year]`, dönemsizde `[none]`) ve
  `exportable`.

### Örnek (overview, `period=7d&compare=previous&locale=tr`)

```json
{
  "report": "overview", "kind": "summary", "period": "7d",
  "period_info": { "from": "2026-10-03", "to": "2026-10-09", "granularity": "day",
                   "compare": "previous", "previous_from": "2026-09-26", "previous_to": "2026-10-02" },
  "series": [{ "key": "cards", "label": "Özet", "unit": "count", "total": null,
               "previous_total": null, "delta_pct": null,
               "points": [{ "key": "orders_created", "label": "Yeni siparişler", "value": 6, "previous": 0 }] }],
  "kpis": [
    { "key": "orders_created", "label": "Yeni siparişler", "unit": "count", "value": 6, "previous": 0,
      "delta_pct": null, "good": "up", "spark": [0, 0, 0, 0, 6, 0, 0],
      "link": { "kind": "report", "report": "orders.trend" } },
    { "key": "services_open", "label": "Devam eden hizmetler", "unit": "count", "value": 0, "previous": null,
      "delta_pct": null, "good": "down", "spark": [],
      "link": { "kind": "report", "report": "services.status_distribution" } }
  ],
  "columns": [], "rows": []
}
```

### Örnek tablo satırı (`activities.recent`)

```json
{
  "columns": [{ "key": "status", "label": "Durum", "type": "status", "align": "left", "aggregate": "none", "currency": null }],
  "rows": [{ "uuid": "…", "service_no": "S-…", "status": "completed", "status_label": "Tamamlandı",
             "_link": { "kind": "service", "uuid": "…" },
             "_status": { "status": { "label": "Tamamlandı", "tone": "done" } } }]
}
```

## Mevcut tabloların zenginleşmesi

| Rapor | Kolon tipleri | `_link` | `_status` |
|---|---|---|---|
| `dealers.performance` | organization=org, oranlar=percent, order_volume=money (satırda `currency`) | organization | – |
| `activities.recent` | service_no=link, organization=org, status=status | service | status |
| `orders.inactive_buyers` | organization=org, last_order_at=datetime | organization | – |
| `orders.plotter_cut` | product=link | product | – |
| `dealers.top_by_warranty` | organization=org | organization | – |
| `orders.by_buyer`, `services.by_org` | organization=org | organization | – |

**Fiyat kuralı (TEC-1050):** `dealers.performance` raporunda `order_volume` kolonu ve satır
değeri yalnız `pricing.purchase.read` ya da `pricing.sale.read` izniyle gelir. Bu kural
`orders.by_buyer` raporundaki `total` alanıyla aynıdır.

## Yeni raporlar

| Anahtar | Tür | Modül | İzin | Dönem | Granülarite | limit |
|---|---|---|---|---|---|---|
| `orders.fulfillment` | summary | orders | orders.read | ✓ | ✓ | – |
| `network.summary` | summary | organizations | organizations.read (subtree/brand/all; bayi hariç) | ✓ | ✓ | ✓ |
| `stock.movement` | timeseries | stock | stock.read | ✓ | ✓ | ✓ |

* `orders.fulfillment` (depo): kapsamdaki kurumların **sattığı** siparişler
  (`organization_id` = satıcı).
  * Seriler: `cards`, `shipped` (`shipped_at`), `received` (`received_at`). Sevk sonrası
    iptal edilen siparişler sayılmaz.
  * KPI'lar:
    * `fulfillment_shipped`, `fulfillment_received`.
    * `fulfillment_prep_hours`: dönemde sevk edilenlerin onaydan sevke ortalama süresi,
      saat, bir ondalık.
    * `fulfillment_open`: approved, preparing, ready veya processing durumundaki siparişler.
    * `fulfillment_overdue`: açık siparişlerden 48 saatten önce onaylananlar.
  * Satırlar sevk deposu ve satıcı bazındadır: `warehouse_uuid` (depo atanmamışsa `null` ve
    ad "Depo atanmamış"), `warehouse`, `organization`, `shipped_count`, `received_count`,
    `open_count`, `overdue_count`, `avg_prep_hours`.
* `network.summary` (distribütör ve merkez): kapsam alt ağaçtır.
  * Seriler: `services` (tamamlanan), `orders` (oluşturulan; satıcı ya da alıcı kapsamda),
    `warranties` (başlayan).
  * KPI'lar: `network_services`, `network_orders`, `network_warranties` ve
    `network_active_dealers` (anlık).
  * Satırlar bayi bazındadır, en çok hizmetten aza sıralanır: `organization`, `city`,
    `status` (`_status`), `services_count`, `orders_count` (alıcı olarak), `warranties_count`.
* `stock.movement`: kapsamdaki depolara giren ve çıkan seri birim hareketleri.
  * Hareket bir depo konumuna giriyorsa "giriş", bir depo konumundan çıkıyorsa "çıkış"
    sayılır. Aynı depo içindeki yerleştirme sayılmaz. İki depo arası transfer birinden
    çıkış, ötekine giriştir.
  * Seriler: `in`, `out`.
  * Satırlar depo bazındadır: `warehouse`, `organization`, `in_count`, `out_count`,
    `net_count`.
  * Depo konumu olmayan bayi stoğu bu raporda görünmez; anlık stok için `stock.summary`
    kullanılır.

## Dışa aktarma

`GET /v1/reports/{report}/export?format=xlsx|csv` (varsayılan xlsx):

* Rapor ucu ile aynı parametreleri, izinleri, modül ve kapsam kontrollerini kullanır. Hata
  kodları da aynıdır.
* Ne yazılır:
  * Tablo raporu: kolonlar ve satırlar. Durum hücresine etiketi yazılır. Kolon para birimi
    olmayan para kolonunun yanına `currency` kolonu eklenir.
  * Zaman serisi: her kova bir satır, her seri bir kolon. `compare` verilmişse her serinin
    önceki dönem kolonu da eklenir.
  * Diğerleri: her KPI ve seri noktası bir satır. Kolonlar `series`, `item`, `value`,
    `previous`, `delta_pct`.
* Başlıklar istek dilindedir: önce `?locale=`, yoksa Accept-Language zinciri.
* Number, money, percent ve duration hücreleri XLSX'te sayı tipindedir. Money hücreleri
  `#,##0.00` biçimiyle yazılır. CSV'de bu değerler düz ondalıktır.
* Zamanlar UTC yazılır.
* Mobil panel aynasından da açıktır: `/v1/mobile/panel/reports/{report}/export`.

## Katalog erişimi

Her rapor kendi modül ve izin kontrolünü yapar. Katalog, kullanıcının açabildiği her raporu
listeler. Bunun için `performance.read` gerekmez; örneğin depo rolü `orders.fulfillment`,
`stock.movement`, `stock.summary` ve `network.summary` raporlarını görür.
`TestCatalogMatchesAccess` testi şunu doğrular: bir rapor katalogda listeleniyorsa açılır,
açılıyorsa katalogda listelenir.
