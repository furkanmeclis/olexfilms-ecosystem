# Mobil depo uçları eşleşmesi (TEC-232)

TEC-208'in "mobil uç eşleşmesi" maddesi. Mobil uygulama ertelendi; bu belge
yalnızca uçları eşler. Backend'de değişiklik yapılmadı.

Kaynaklar:

- F0-15 (TEC-91) mobil sözleşmesi: `docs/openapi.yaml`, `Mobile` etiketi.
- Eski depo mobil API'si: olexfilms-warehouse `docs/mobile-api.md` ve
  `docs/mobile-api/faz-*.md`.
- Yeni depo uçları: TEC-201..207 `/v1/warehouse/*` ve TEC-202
  `/v1/stock/*`.

## 1. Sözleşme kuralları (F0-15)

| Kural | Durum |
|---|---|
| Önek `/v1/mobile/*`, BFF üzerinden `/api/v1/mobile/*` Bearer geçişi | Var |
| `aud=mobile` token yalnız `/v1/mobile/*` için geçerli (`RealmAllows`, 403 `REALM_FORBIDDEN`) | Var |
| `X-Mobile-Api-Version` zorunlu, aralık dışı 426 | Var |
| Org bağlamı (`oid`): `POST /v1/mobile/auth/organization-context` | Var |
| Depo bağlamı (`whid`): sözleşmede "F1 için ayrıldı" | **Yok** |
| Push token, QR web girişi, ölçüm kabulü | Var (depo dışı) |

Sonuç: Mobil token bugün hiçbir `/v1/warehouse/*` ucunu çağıramaz. Depo
akışlarının mobilde çalışması için aşağıdaki uçların `/v1/mobile/warehouse/*`
altında yansıtılması gerekir. Önerilen yol: aynı handler'ları
`/v1/mobile/warehouse/...` yoluna da bağlamak (izin, modül kapısı ve org
kapsamı aynı kalır). Yeni iş kuralı gerekmez.

## 2. Eşleşme tablosu

Sütunlar:

- **Eski mobil (FAZ)**: olexfilms-warehouse ucu.
- **Yeni panel ucu**: mevcut backend ucu.
- **Mobil yansıma**: önerilen ve henüz olmayan `/v1/mobile` yolu.

### 2.1 Bağlam ve arama

| Eski mobil (FAZ) | Yeni panel ucu | Mobil yansıma |
|---|---|---|
| `GET /warehouse-context`, `POST /warehouse-context/switch` (3) | Uç yok; org bağlamı `organization-context` ile | **Eksik**: `whid` claim'i ya da depo tercih ucu |
| `GET /lookups/warehouses` (10), `GET /warehouses` (2) | `GET /v1/warehouse/warehouses` | `GET /v1/mobile/warehouse/warehouses` |
| `GET /warehouse-sites`, `GET /lookups/warehouse-sites` (10, 12) | `GET /v1/warehouse/warehouses/{uuid}/rooms` | `GET /v1/mobile/warehouse/warehouses/{uuid}/rooms` |
| `GET /locations/tree`, `GET /lookups/locations` (6) | `GET /v1/warehouse/rooms/{uuid}/locations` | `GET /v1/mobile/warehouse/rooms/{uuid}/locations` |
| `POST /scan/resolve` (13) | `POST /v1/warehouse/scan` | `POST /v1/mobile/warehouse/scan` |
| `GET /product-barcodes/{id}` (14) | `GET /v1/stock/units/by-barcode/{barcode}` | `GET /v1/mobile/stock/units/by-barcode/{barcode}` |
| `GET /products/{id}/stock-overview`, `stock-summary` (27, 31) | `GET /v1/stock/locations/{uuid}/products`, `GET /v1/stock/organizations/{uuid}/products` | Yansıma gerekir. Ürün bazlı, konum kırılımlı özet ucu yok (**eksik**) |

### 2.2 Stok girişi (TEC-204)

| Eski mobil (FAZ 15) | Yeni panel ucu | Mobil yansıma |
|---|---|---|
| `GET/POST /stock-entries`, `GET /stock-entries/{id}` | `GET/POST /v1/warehouse/stock-entries`, `GET .../{uuid}` | `/v1/mobile/warehouse/stock-entries[/{uuid}]` |
| `POST /stock-entries/{id}/attach-barcodes` | `POST .../{uuid}/lines` (`barcodes`) | `.../lines` |
| `POST /stock-entries/{id}/reserve-barcodes` | `POST .../{uuid}/lines` (`product_uuid`, `quantity`; yalnız merkez, K14) | `.../lines` |
| `POST /stock-entries/{id}/assign-location` | `POST .../{uuid}/place` | `.../place` |
| `POST /stock-entries/{id}/confirm`, `/cancel` | `POST .../{uuid}/confirm`, `/cancel` | `.../confirm`, `.../cancel` |
| `POST /stock-entries/{id}/mark-printed` | Uç yok; etiket PDF'i `label_url` / `labels_url` | Eşleşmez. "Basıldı" durumu yeni modelde ayrı tutulmuyor |

### 2.3 Transfer (TEC-205)

| Eski mobil (FAZ 16) | Yeni panel ucu | Mobil yansıma |
|---|---|---|
| `GET/POST /stock-transfers`, `GET/PATCH /stock-transfers/{id}` | `GET/POST /v1/warehouse/transfers`, `GET .../{uuid}` | `/v1/mobile/warehouse/transfers[/{uuid}]`. PATCH (not düzenleme) yok, **eksik** ama düşük öncelikli |
| `POST /stock-transfers/{id}/lines`, `DELETE .../lines/{line}` | `POST .../{uuid}/lines`, `DELETE .../lines/{line_uuid}` | aynı alt yollar |
| `POST /stock-transfers/{id}/ship` | `POST .../{uuid}/ship` | `.../ship` |
| `POST /stock-transfers/{id}/receive`, `/complete` | `POST .../{uuid}/place` + `POST .../{uuid}/complete` | `.../place`, `.../complete` |
| `POST /stock-transfers/{id}/cancel` | `POST .../{uuid}/cancel` | `.../cancel` |
| Göz → göz taşıma (FAZ 16 içinde aynı depo transferi) | `POST /v1/warehouse/moves` | `POST /v1/mobile/warehouse/moves` |
| Sipariş rafa kaldırma (FAZ 20, 32) | `POST /v1/warehouse/orders/{uuid}/place` | `POST /v1/mobile/warehouse/orders/{uuid}/place` |
| `GET /stock-transfers/{id}/report`, `GET /reports/stock-transfers/overview`, `POST /reports/stock-transfers/export` (18) | Uç yok | **Eksik**: transfer raporu ve dışa aktarma |

### 2.4 Sayım (TEC-206)

| Eski mobil (FAZ 17) | Yeni panel ucu | Mobil yansıma |
|---|---|---|
| `GET/POST /stock-counts`, `GET/PATCH /stock-counts/{id}` | `GET/POST /v1/warehouse/stock-counts`, `GET .../{uuid}` | `/v1/mobile/warehouse/stock-counts[/{uuid}]`. PATCH yok, **eksik** (düşük öncelik) |
| `POST /stock-counts/{id}/approve-start`, `/start` | `POST .../{uuid}/approve-start`, `/start` | aynı alt yollar |
| `POST /stock-counts/{id}/location-context` | Ayrı uç yok; konum QR'ı `POST .../{uuid}/scans` ile bağlamı kurar | `.../scans` (davranış eşdeğer) |
| `POST /stock-counts/{id}/scan` | `POST .../{uuid}/scans`; liste `GET .../scans`; silme `DELETE .../scans/{scan_uuid}` | aynı alt yollar |
| `POST /stock-counts/{id}/complete` | `POST .../{uuid}/complete` | `.../complete` |
| `GET /stock-counts/{id}/report` (17, 18) | `GET .../{uuid}/report`, CSV `GET .../{uuid}/export` | `.../report`, `.../export` |
| `POST /stock-counts/{id}/approve` | `POST .../{uuid}/approve` (`stock.adjust`) | `.../approve` |
| `POST /stock-counts/{id}/cancel` | `POST .../{uuid}/cancel` | `.../cancel` |
| `GET /reports/stock-counts/overview` (18) | Uç yok | **Eksik**: sayımlar arası özet raporu |
| `GET/PUT /reports/layout` (18) | Uç yok | Eşleşmez. Rapor düzeni istemci tercihi; sunucu ucu önerilmez |

### 2.5 Gün sonu (TEC-207)

| Eski mobil (FAZ 29) | Yeni panel ucu | Mobil yansıma |
|---|---|---|
| `GET /end-of-day-reports` | `GET /v1/warehouse/eod-reports` | `GET /v1/mobile/warehouse/eod-reports` |
| `GET /end-of-day-reports/{id}` | `GET /v1/warehouse/eod-reports/{uuid}` | `.../eod-reports/{uuid}` |
| `POST /end-of-day-reports` | `POST /v1/warehouse/eod-reports` | `POST .../eod-reports` |
| PDF (FAZ 29 PDF URL) | `POST .../eod-reports/{uuid}/pdf`, `GET /v1/warehouse/eod-report-pdfs/{uuid}[/download]` | `/v1/mobile/warehouse/eod-reports/{uuid}/pdf`, `/v1/mobile/warehouse/eod-report-pdfs/{uuid}[/download]` |

### 2.6 Etiket

| Eski mobil | Yeni panel ucu | Mobil yansıma |
|---|---|---|
| Konum etiketi (FAZ 6) | `GET /v1/warehouse/labels/locations.pdf` | `GET /v1/mobile/warehouse/labels/locations.pdf` |
| Birim / parti etiketi (FAZ 14, 15) | `GET /v1/stock/labels/units.pdf`, `GET /v1/stock/barcodes/{uuid}/labels.pdf` | `/v1/mobile/stock/labels/...` |

## 3. Eksik uçlar özeti

Backend'e dokunulmadı. Aşağıdakiler ayrı bir iş olarak açılmalı:

1. **Mobil yansıma (ana eksik):** §2'deki tüm "Mobil yansıma" yolları.
   `RealmAllows` mobil token'ı yalnız `/v1/mobile/*` için kabul ediyor.
   Bu yüzden `/v1/warehouse/*`, `/v1/stock/labels/*` ve
   `/v1/stock/units/by-barcode/*` mobil yolu altında da bağlanmalı. Aynı
   izinler (`warehouse.read` / `warehouse.write` / `stock.adjust`), aynı
   `RequireFeature(warehouse)` ve aynı org kapsamı geçerli kalmalı.
2. **Depo bağlamı (`whid`):** F0-15'te ayrılan claim ya da kullanıcı başına
   varsayılan depo ucu. Eski FAZ 3 `warehouse-context` karşılığıdır. Panelde
   depo her istekte gövdeyle seçildiği için web tarafı bunu gerektirmiyor.
3. **Transfer raporu ve dışa aktarma** (eski FAZ 18). Transfer listesi ve
   detayı var. Özet rapor ve Excel/CSV yok.
4. **Sayım özet raporu** (`reports/stock-counts/overview`). Tek sayımın
   raporu ve CSV'si var.
5. **Ürün stok özeti, konum kırılımı** (eski FAZ 27/31). Konum → ürün ve
   org → ürün listeleri var. Ürün → konumlar görünümü yok.
6. **Transfer / sayım notu düzenleme** (PATCH). Düşük öncelik. Not, yalnız
   oluşturmada veriliyor.

Bilerek eşleşmeyenler: `mark-printed` (yeni modelde basım durumu yok),
`reports/layout` (istemci tercihi).
