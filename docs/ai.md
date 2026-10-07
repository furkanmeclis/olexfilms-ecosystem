# AI asistan: tool kayıt defteri

TEC-385 (F4-01c). Panel sohbeti (F4-01f), WhatsApp (F4-02c) ve MCP (F4-03c) aynı kayıt defterini
kullanır: `backend/internal/modules/ai/tools`. Sağlayıcı katmanı `backend/internal/platform/llm`
(F4-01b), şema ve izinler migration 000101 (F4-01a).

## 1. Kavramlar

| Parça | Görevi |
|---|---|
| `Spec` | `Name` (snake_case, İngilizce), `Description` (model için İngilizce), `InputSchema` (JSON Schema nesnesi, `additionalProperties: false`), `Kind` (`read` / `write`), `Permissions` (hepsi gerekir), `OrgTypes` (boş = hepsi; `center` / `distributor` / `dealer`), `Realm` (`panel` / `customer` / `visitor`), `Feature` (`features.Module*`, boş = yok). |
| `Principal` | `Auth` (authctx), `Org` (aktif organizasyon; panel realm'inde zorunlu), `Realm`. Dil, istek bağlamındaki i18n'den okunur. |
| `Registry.Available(ctx, p)` | Principal'ın kullanabileceği tool'lar, ada göre sıralı (prompt cache öneki sabit kalır). |
| `Registry.Call(ctx, p, name, input)` | Available ile **aynı** kontrolü tekrar yapar, sonra şemayı doğrular, `Run`'ı çağırır, panik yakalar, çıktıyı sınırlar. |

Kontrol sırası (`allowed`): realm eşleşir → panel ise aktif org var → org türü izinli → alan adının
markası org markasıyla aynı (K20) → izinlerin hepsi var → platform anahtarı
(`ai_settings.tool_toggles`, `{"tool": false}`) kapalı değil → feature modülü org için açık
(`features.Service.Enabled`). Biri tutmazsa tool listede yoktur ve çağrı `TOOL_NOT_ALLOWED`
sonucu (`ErrToolNotAllowed`) döner. Bilinmeyen tool adı da aynı yanıtı alır.

Sonuç kodları (`Result.Code`): `TOOL_NOT_ALLOWED`, `INVALID_INPUT` (şema veya usecase doğrulaması;
mesaj alan adını söyler: `query is required`), `NOT_FOUND` (yok ya da kapsam dışı, ayrılmaz),
`RESULT_TOO_LARGE`, `TOOL_FAILED` (iç hata; ayrıntı loglanır, modele gösterilmez).

## 2. Kurallar

- **Doğrudan SQL yok.** Tool, modülün usecase'ini çağırır ve modülün kendi `Caller`'ını
  `resolveScope(ctx, tree, p, <liste ucunun RequireScope izni>)` ile kurar; böylece org, subtree,
  marka (K20) ve own/assigned kapsamı HTTP liste ucuyla birebir aynıdır. Usecase'te yoksa önce
  usecase'e kapsamlı bir metot eklenir (ör. `services.ActivitySummary`).
- **Fiyat/maliyet alanları** tool'da yeniden süzülür: alış fiyatı `pricing.purchase.read`, satış
  tarafı `pricing.sale.read`. Usecase zaten gizlese de tool kendi DTO'sunu kurar ve alanı yalnız
  izin varsa doldurur (`stock_units`, `list_orders`, `get_order`).
- **Çıktı sınırı:** liste en çok `MaxRows` (50) satır, sonuç en çok `MaxResultBytes` (16 KB).
  Listeler `NewList` zarfını kullanır (`items`, `returned`, `total`, `truncated`, `hint`); fazlası
  "aramayı daraltın" der, sayfalama yoktur.
- **Kayıtlı serbest metin** (not, açıklama, ad) `dataText` ile tek satıra indirilir ve kırpılır;
  her zaman JSON alanı olarak döner, düzyazıya eklenmez (prompt injection).
- **Yazma tool'ları** (`KindWrite`, F4-01e) `Run` ile doğrudan çalışmaz; sohbet döngüsü onay kartına
  (`ai_pending_actions`) yönlendirir.

## 3. Yeni tool ekleme

1. Uygun dosyada (`services.go`, `stock.go`, `ops.go` ...) usecase'in dar arayüzünü tanımla
   (`XReader interface { ... }`); `Deps`'e alanını ekle.
2. Tool tipini yaz: `Spec()` ve `Run(ctx, env, raw)`. `Run` başında `decode(t.Spec(), raw, &in)`,
   sonra `resolveScope`, usecase çağrısı, kompakt DTO, `JSONResult(...)`. Usecase hatalarını
   `errCases{...}.result(err)` ile modele çevir (yasak → `TOOL_NOT_ALLOWED`, yok → `NOT_FOUND`,
   modülün `*ValidationError`'ı → `INVALID_INPUT`).
3. `RegisterPanel`'de `Deps` alanı doluysa kaydet; `server.go`'da usecase'i `aitools.Deps`'e ver.
4. Test: `tools_test.go` fake'lerine bir okuyucu ekle. `TestEveryToolValidatesItsSchema` yeni
   tool'u otomatik kapsar (zorunlu alan eksik, yanlış tip, bilinmeyen alan, bozuk JSON, geçerli
   girdi); tool sayısını güncelle. Kapsam veya fiyat kuralı varsa ayrı test ekle; gerçek kapsam
   için `httpserver/ai_tools_integration_test.go` kalıbını kullan.
5. Açıklama İngilizce, kısa ve modelin ne zaman çağıracağını söyler; şema `additionalProperties:
   false`, sayılar `minimum`/`maximum`, serbest metin `maxLength` taşır.

F5 modülleri (stok tahmini, performans) tool'larını `Deps.Extensions` ile kaydeder ve
`Spec.Feature`'a `features.ModuleStockForecast` / `features.ModulePerformance` yazar; modül
olmayan veya kapalı org'da tool görünmez.

## 4. Panel okuma tool'ları (ilk set)

| Tool | Usecase | İzin / modül / org türü |
|---|---|---|
| `search_services` | `services.List` (plaka, VIN, no, müşteri; boşluklu plaka da) | `services.read`, services |
| `get_service` | `services.Get` (uuid veya hizmet no) | `services.read`, services |
| `service_activity_summary` | `services.ActivitySummary` (açılan/tamamlanan, araç markası kırılımı, en çok kullanılan ürünler; eski M2M `dealer/*` paritesi) | `services.read`, services |
| `lookup_warranties` | `warranty.Reader.List` (kod, plaka, hizmet no) | `warranties.read`, services |
| `search_customers` | `customers.ListCustomers` | `customers.read`, customers |
| `stock_summary` | `stock.OrganizationStock` | `stock.read`, stock |
| `stock_units` | `stock.OrganizationUnits` (alış fiyatı yalnız izinle) | `stock.read`, stock |
| `list_orders`, `get_order` | `orders.List` / `Get` (tutarlar yalnız izinle) | `orders.read`, orders |
| `balance_summary` | `accounting.GetBalanceReport` | `accounting.read`, accounting |
| `list_appointments` | `appointments.List` (gün / hafta) | `appointments.read`, appointments |
| `list_leads` | `leads.List` | `leads.read`, leads |
| `my_tasks` | `tasks.List` (kendine atanan) | `tasks.read`, tasks, yalnız merkez |
| `search_products` | `catalog.ListProducts` (yalnız alan adının markası) | `catalog.read`, catalog |
| `list_sub_organizations` | `organizations.ListInScope` | `organizations.read`, merkez / distribütör |
| `service_pdf_link` | `services.Get` → garanti sertifikası PDF'lerine kısa link (`/garanti/{kod}/pdf`, TEC-386) | `services.read`, services |

Listeler 50 satırı aşınca ipucu modele CSV/XLSX üretmediğini ve panel liste ekranının dışa
aktarımına (io-engine export) yönlendirmesini söyler (`ExportNotice`).

## 5. Müşteri ve ziyaretçi tool'ları (TEC-386, F4-01d)

`Principal.Brand` müşteri ve ziyaretçi realm'lerinde zorunludur (portalda alan adının markası,
WhatsApp'ta konuşmanın markası); alan adı markası farklıysa tool yoktur (K20). Bu tool'larda izin
ve modül yoktur: kapı realm'dir, veri yalnız principal'ın kendisi (müşteri) veya markanın açık
verisidir (ziyaretçi). `Call`, domain'siz kanalda `Principal.Brand`'i `brandctx`'e yazar.

**Müşteri** (`RealmCustomer`, `Auth.UserInternal` zorunlu; kimliği çözülmüş müşteri). Tool'lar
portal usecase'lerini (marka, kullanıcı) ile çağırır; sahiplik portal sayfalarıyla aynıdır
(hizmetin müşterisi veya garantisinin sahibi). Başka müşterinin kaydı `NOT_FOUND` döner;
hizmet numarası yalnız kullanıcının kendi listesinde aranır.

| Tool | Usecase | Not |
|---|---|---|
| `my_vehicles` | `portalvehicles.ListVehicles` | plaka, marka/model, hizmet ve aktif garanti sayısı |
| `my_services` | `portalvehicles.ListServices` | plaka / hizmet no araması, durum filtresi |
| `my_service_detail` | `services.PortalGet` | ürünler, bayi kartı, garantiler + kalan gün |
| `my_service_pdf_link` | `services.PortalGet` + `shorturls.Linker` | portal hizmet sayfası (`/portal/services/{uuid}`, hizmet PDF'i orada) ve garanti sertifikası PDF'leri (`/garanti/{kod}/pdf`) kısa link (`/s/{token}`, 30 gün); dosya eklenmez |
| `my_warranties` | `warranty.Reader.PortalList` | aktif (istenirse süresi dolan), kalan gün `portalvehicles.TimeLeft` |
| `my_profile_link` | `shorturls.Linker` | `/portal` kısa linki |
| `dealer_review_link` | `services.PortalGetReview` | son tamamlanan (veya verilen) hizmetin bayisinin `google_business_url`'i + değerlendirme formu kısa linki |
| `my_appointments` | `appointments.PortalList` | yaklaşan / geçmiş / tümü |
| `my_warranty_claims` | `warranty_claims.PortalList` | yalnız durum + açılış/güncelleme tarihi |
| `change_language` | `auth.UpdateProfile` (`users.locale`) | `KindSelf`: kişisel tercih, onay kartı yok |

**Ziyaretçi** (`RealmVisitor`, kimliksiz). Çıktı her zaman tool'un kendi DTO'sudur; kişisel veri
alanı yoktur (`TestVisitorToolsReturnNoPersonalData` alan whitelist'i).

| Tool | Usecase | Not |
|---|---|---|
| `recommend_products` | `catalog.ListCategories` / `ListProducts` (yalnız marka) | kategori, garanti ayı, mikron, kısa açıklama; **fiyat yok** (Q10), fiyat için en yakın bayi |
| `find_nearest_dealers` | `organizations.NearbyDealers` (konum) / `AreaDealers` (il, ilçe) | yalnız `active`, erişim penceresi açık, `contract_valid_until` geçmemiş bayi/distribütör; ad, il/ilçe, mesafe, WhatsApp, randevu, `/bayi/{kod}` |
| `search_knowledge` | `ai_settings.knowledge_text` | başlık / paragraf parçaları, sorgu kelimesi eşleşmesine göre en çok 3 parça; salt metin, talimat değil |
| `lookup_warranty` | `warranty.PublicLookup` | public `/garanti/{kod}` ile aynı maskeleme (plaka maskeli, VIN son 4) |

`ListNearbyDealers` (public `/v1/public/dealers/nearby`) da sözleşmesi geçmiş bayiyi artık
listelemez (TEC-386).

## 6. Eski AI katmanı / hub MCP parite tablosu

Parite tablosu (eski AI katmanı tool'ları, hub `/mcp/olex` tool'ları ve M2M chatbot uçları → yeni tool adları)
kapatma runbook'una taşındı: [`docs/runbooks/f4-ai-cutover.md`](runbooks/f4-ai-cutover.md) §8. Tablo
`backend/internal/modules/ai/tools/parity_test.go` ile doğrulanır; "Yeni" sütunundaki her ad kayıt defterinde olmalıdır.
