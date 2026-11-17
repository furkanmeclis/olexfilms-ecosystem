# Çapraz kayıt referansları (Ref) ve kind sözlüğü

TEC-1215 / TEC-1216. Veritabanındaki her ilişki API yanıtında adıyla ve tıklanabilir
biçimde döner. Web (`entityHref(kind, uuid)`, `<EntityLink>`, `refColumn()`,
`<RelatedList>`) ve mobil (`useEntityHref(kind)`, `<RefRow>`) bu sözleşmeye dayanır.

## 1. Ref şekli

Go: `backend/internal/platform/refs` (`refs.Ref`). OpenAPI: `#/components/schemas/Ref`.

```json
{ "kind": "organization", "uuid": "6f1c2a52-6d0e-4a52-9a43-0f3f3a1b2c3d", "name": "Kadıköy Bayi", "sub": "dealer" }
```

| Alan | Kural |
|---|---|
| `kind` | Aşağıdaki sözlükten biri. İstemci detay rotasını bundan çözer. |
| `uuid` | Kaydın genel uuid'i. İç sayısal id **asla** dışarı çıkmaz. |
| `name` | Görünen ad (kullanıcı/müşteri: `ad soyad`, hizmet: hizmet no, ürün: ürün adı). Anonimleştirilmiş müşteride boş olabilir; yanıt bunu ayrıca işaretler (`customer_anonymized`). |
| `sub` | İsteğe bağlı ikinci satır; boşsa alan hiç gelmez. Organizasyonda tür (`center`, `distributor`, `dealer`), üründe SKU, depoda depo kodu. |

Yardımcılar: `refs.New`, `refs.WithSub`, `refs.Ptr` (uuid.Nil → nil), `refs.UserRef`,
`refs.UserRefPtr`, `refs.OrgRef(uuid, ad, tür)`, `refs.ProductRef(uuid, ad, sku)`,
`refs.CustomerRef`, `refs.FullName`.

Kural: Yeni ve dokunulan kodda yabancı anahtar yanıtta `Ref` (ya da en az `{uuid,name}`)
olarak döner. Modüllerdeki kopya ref struct'ları (`OrgRef`, `UserRef`, `ProductRef` …)
dokunuldukça bu pakete taşınır; mevcut alan adları ve JSON şekli geriye dönük korunur.

## 2. Kind sözlüğü

Rapor satırlarının `_link.kind` alanı aynı sözlüğü kullanır
(`reports/usecase/link_kinds_test.go` bunu denetler). `report` yalnız rapor
gezintisidir, kayıt türü değildir.

| kind | Kayıt | Web detay (öneri) |
|---|---|---|
| `service` | Hizmet | `/services/{uuid}` |
| `warranty` | Garanti | `/warranties/{uuid}` |
| `warranty_claim` | Garanti talebi | `/warranty-claims/{uuid}` |
| `customer` | Müşteri (müşteri hesabının kullanıcı uuid'i) | `/customers/{uuid}` |
| `vehicle` | Araç | `/vehicles/{uuid}` |
| `order` | Sipariş | `/orders/{uuid}` |
| `product` | Ürün | `/catalog/products/{uuid}` |
| `stock_unit` | Stok birimi (barkod) | stok birimi geçmişi |
| `transfer` | Stok transferi | `/stock-transfers/{uuid}` |
| `organization` | Organizasyon (merkez/distribütör/bayi) | `/organizations/{uuid}` (özet: `GET /v1/tenant/organizations/{uuid}/summary`) |
| `user` | Personel kullanıcı | `/users/{uuid}` (özet: `GET /v1/tenant/users/{uuid}/summary`) |
| `lead` | Lead | `/leads/{uuid}` |
| `appointment` | Randevu | `GET /v1/appointments/{uuid}` |
| `task` | Görev | `/tasks/{uuid}` |
| `measurement` | Ölçüm | `/measurements/{uuid}` |
| `contract` | Sözleşme | `GET /v1/contracts/{uuid}` |
| `invoice` | E-fatura | `/einvoices/{uuid}` |
| `cari` | Cari hesap | `/accounting/cari/{uuid}` |
| `announcement` | Duyuru | `GET /v1/announcements/{uuid}` |
| `warehouse` | Depo | `GET /v1/warehouse/warehouses/{uuid}` |
| `subscription` | Hizmet aboneliği | `/service-subscriptions/{uuid}` |

Web rota eşlemesi web ajanınındır (TEC-1217); buradaki sütun yalnız öneridir.

## 3. Ters ilişki filtreleri (RelatedList)

Hepsi `docs/list-contract.md`'ye uyar; yalnız çağıranın kapsamındaki kümeyi daraltır
(kapsam dışı kayıt filtrelense de boş liste döner, 403 değil). Geçersiz uuid → 400
`VALIDATION_ERROR`. Bilinmeyen ama geçerli uuid → boş liste.

| Uç | Filtre | Anlam |
|---|---|---|
| `GET /v1/services` | `customer_uuid`, `vehicle_uuid`, `organization_uuid` (CSV) | mevcut |
| | `created_by_uuid` | hizmeti açan kullanıcı |
| | `performer_uuid` | uygulayan (`performed_by`, yoksa `created_by`) |
| | `product_uuid` | kalemlerinden biri bu üründen olan hizmetler |
| `GET /v1/warranties` | `customer_uuid` | garanti sahibi (araç devrinde yeni sahip) |
| | `product_uuid`, `organization_uuid` (CSV) | mevcut |
| `GET /v1/orders` | `organization_uuid` (CSV) | satıcı **veya** alıcı |
| | `product_uuid` | satırlarından biri bu ürün |
| | `created_by_uuid` | siparişi açan kullanıcı |
| `GET /v1/appointments` | `customer_uuid` | müşterinin randevuları; bu filtreyle `from`/`to` zorunlu değil |
| `GET /v1/leads` | `customer_uuid` | müşterinin lead'leri |
| `GET /v1/measurements` | `customer_uuid` | bağlı hizmetin müşterisi veya aracın sahibi |
| `GET /v1/contracts` (yeni liste) | `customer_uuid` | hizmet müşterisi veya müşteri imzacısı; ayrıca `status`, `kind` (CSV), `sort` |
| `GET /v1/tasks` | `organization_uuid` (CSV), `assignee_uuid` | `subject_organization_uuid` / `assignee_user_uuid` takma adları |
| `GET /v1/stock/organizations/{uuid}/units`, `GET /v1/stock/warehouses/{uuid}/units` | `product_uuid` | mevcut |

Not: `leads.read` bayi/distribütör sahiplerinde `managed` kapsamdır; distribütör alt
bayilerinin lead'lerini görmez (filtre bunu değiştirmez).

## 4. Detay uçları

| Uç | Kapsam | İçerik |
|---|---|---|
| `GET /v1/tenant/organizations/{uuid}/summary` | `organizations.read` | `organization` (mevcut görünüm), `ref`, `parent` (Ref), `counts` |
| `GET /v1/tenant/users/{uuid}/summary` | `members.read` | `user`, `ref`, `memberships[]` (organization Ref, rol, rol slug'ları, depolar Ref), `counts` |
| `GET /v1/contracts` | `contracts.read` | sözleşme listesi (Ref'li) |
| `GET /v1/appointments/{uuid}`, `GET /v1/announcements/{uuid}`, `GET /v1/contracts/{uuid}`, `GET /v1/warehouse/warehouses/{uuid}` | modül izinleri | mevcut tekil uçlar |

Sayaçlar (`counts`): çağıran ilgili modülün okuma iznine sahip değilse `null` döner
(ör. `services.read` yoksa `services: null`). Organizasyon sayaçları:
`members, children, customers, services, warranties, orders, open_tasks, leads, warehouses`.
Kullanıcı sayaçları (çağıranın kapsamındaki organizasyonlarda):
`services_created, services_performed, orders_created, open_tasks, leads_assigned`.

Mobil ayna (`/v1/mobile/panel/*`): `tenant/organizations`, `contracts`, `appointments`,
`announcements`, `warehouse`, `tasks` zaten açık; `tenant/users` TEC-1216 ile eklendi.
