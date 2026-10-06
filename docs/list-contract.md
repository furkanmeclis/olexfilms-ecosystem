# Liste sözleşmesi (backend ↔ DataTable)

TEC-363 (epic TEC-362). Sayfalı her liste ucu bu sözleşmeyi uygular. Frontend tarafı
(`EntityTable` + `useServerListState`) aynı parametreleri üretir. Zarf ve genel kurallar için
`backend/API_CONVENTIONS.md`'ye bakın.

## 1. Query parametreleri

| Parametre | Kural |
|---|---|
| `limit`, `offset` | Varsayılan 20, en çok 100 (`apiquery.Parse`). Yanıt `apiquery.NewPage` (`items`, `total`, `limit`, `offset`). |
| `sort` | Tek alan: `alan` (artan) veya `-alan` (azalan). Alan uç whitelist'inde değilse **400 VALIDATION_ERROR** (`field: sort`). Birden fazla alan gelirse hepsi doğrulanır, yalnız ilki uygulanır (geri uyumluluk). Sort yoksa ucun varsayılanı uygulanır; varsayılan `/meta` yanıtında `default_sort` olarak yayımlanır. Backend her zaman benzersiz tiebreak (`id`) ekler. |
| `q` | Global arama (ucun aranabilir alanları). |
| Çok değerli enum | Virgüllü liste veya tekrarlanan anahtar: `status=draft,approved` ya da `status=draft&status=approved`. Tek değer de geçerli. Geçersiz değer → 400 (`field: status`). |
| Tarih aralığı | `<önek>_from`, `<önek>_to`; `YYYY-MM-DD` (UTC gece yarısı) veya RFC3339. Tarih biçimindeki `_to` o günü **tümüyle** kapsar; RFC3339 `_to` o anı kapsar. `from > to` → 400. Mevcut adlar (`created_from/_to`, `date_from/_to`) korunur. |
| Sayı aralığı | `<önek>_min`, `<önek>_max` (ikisi de dahil). `min > max` → 400. |
| Boolean | `true` / `false`; başka değer → 400. |

## 2. `pkg/apiquery` yardımcıları

```go
// Uç başına sözleşme: whitelist (API alanı → güvenilir sort anahtarı) + varsayılan.
var ordersSortSpec = apiquery.SortSpec{
	Columns: apiquery.SortColumns{"number": "number", "created_at": "created_at", "total": "total"},
	Default: apiquery.SortField{Field: "created_at", Desc: true},
}

q := apiquery.Parse(r.URL.Query())
sort, err := apiquery.ResolveSort(q.Sort, ordersSortSpec) // → ResolvedSort{Key, Desc}; bilinmeyen alan → *ValidationError
statuses, err := apiquery.EnumList(r.URL.Query(), "status", model.OrderStatuses...) // nil = filtre yok
created, err := apiquery.DateRange(r.URL.Query(), "created") // created_from / created_to → TimeRange{From, Before}
total, err := apiquery.NumRange(r.URL.Query(), "total")      // total_min / total_max → NumberRange{Min, Max *float64}
pinned, err := apiquery.Bool(r.URL.Query(), "pinned")        // *bool
vals := apiquery.CSVValues(r.URL.Query(), "tag")              // doğrulamasız çok değerli liste
```

- Tüm hatalar `*apiquery.ValidationError`; handler'ın hata yazıcısı bunu `response.ValidationError` ile
  400'e çevirir (bkz. `organizations/handler.writeError`, `auth/handler.writeUsecaseError`).
- `TimeRange.Before` **hariç** üst sınırdır; SQL her zaman `col >= from AND col < before` yazar.
- `/meta` yanıtı spec'ten türetilir; elle liste yazılmaz:
  `DefaultSort: spec.DefaultString()`, `SortableFields: spec.Fields()`. Kolonda `Sortable: true`
  yalnız whitelist'teki alanlar için (`resourcemeta/sort_test.go` bunu dört platform kaynağı için denetler).
- Yeni uçların spec'i modülün kendi paketinde tutulur; `pkg/apiquery/resources.go` büyütülmez
  (ortak dosya çakışmalarını azaltır).

## 3. sqlc'de dinamik ORDER BY kalıbı

sqlc ORDER BY'a kolon adı parametre olarak alamaz; string birleştirme de tip güvenliğini ve SQL
injection korumasını bozar. Kalıp: sabit iki parametre (`sort_key text`, `sort_desc bool`) ve
**kolon tipine göre gruplanmış** `CASE` çiftleri. `sort_key` her zaman `ResolveSort`'un döndürdüğü
güvenilir anahtardır; kullanıcı girdisi SQL'e kolon adı olarak hiç ulaşmaz.

```sql
-- name: ListThings :many
-- Sort: docs/list-contract.md, keys from <modül>.thingsSortSpec.
SELECT t.*
FROM things t
WHERE t.organization_id = sqlc.arg(organization_id)
  AND (
    COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0
    OR t.status = ANY (sqlc.narg(statuses)::text[])
  )
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR t.created_at >= sqlc.narg(created_from))
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR t.created_at < sqlc.narg(created_before))
ORDER BY
  -- metin kolonları (NOT NULL)
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'name' THEN t.name WHEN 'status' THEN t.status END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'name' THEN t.name WHEN 'status' THEN t.status END
  END DESC,
  -- zaman kolonları (NOT NULL)
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN t.created_at WHEN 'updated_at' THEN t.updated_at END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN t.created_at WHEN 'updated_at' THEN t.updated_at END
  END DESC,
  -- NULL olabilen kolon: kendi çifti + NULLS LAST (iki yönde de boşlar sonda)
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'due_at' THEN t.due_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'due_at' THEN t.due_at END DESC NULLS LAST,
  -- benzersiz tiebreak, birincil yönle aynı yönde
  CASE WHEN sqlc.arg(sort_desc)::bool THEN t.id END DESC,
  t.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);
```

Go tarafı:

```go
rows, err := q.ListThings(ctx, db.ListThingsParams{
	OrganizationID: orgID, Statuses: statuses,          // nil slice → NULL → filtre yok
	CreatedFrom: tsNarg(created.From), CreatedBefore: tsNarg(created.Before),
	SortKey: sort.Key, SortDesc: sort.Desc,
	LimitCount: q.Limit, OffsetCount: q.Offset,
})
```

Kurallar:
- Bir `CASE` içindeki tüm dallar aynı tipte olmalı; bu yüzden gruplama tipe göredir (metin, zaman,
  sayı, …). Yeni tip → yeni ASC/DESC çifti.
- Enum benzeri alanlar alfabetik değil anlamlı sırayla sıralanacaksa sıra değeri üreten iç `CASE`
  kullanılır (ör. `priority`: low < normal < high < critical; log `level`: debug < warn < error).
- `NULLS LAST` yalnız NULL olabilen kolonların çiftine yazılır. NOT NULL kolonlara eklenmez: `DESC`
  indeksi `NULLS FIRST` sıralıdır, `NULLS LAST` eklemek indeks sırasını kullanılmaz kılar.
- Tiebreak her zaman en sonda (`id`); join'li sorgularda ana tablonun `id`'si. `SELECT DISTINCT`
  ile `CASE` sıralaması birlikte kullanılamaz (Postgres ORDER BY ifadesini select listesinde ister);
  çoğaltan join'ler (`LEFT JOIN roles` gibi) `EXISTS` alt sorgusuna çevrilir (bkz. `ListUsersFiltered`).
- `sort_key` boş bırakılırsa yalnız `id ASC` kalır; handler/usecase her zaman `ResolveSort` sonucunu geçmeli.
- Count sorgusu ORDER BY/LIMIT almaz ama filtre bloğu liste sorgusuyla birebir aynı olmalı.
- Performans: pgx sorguyu hazırlanmış ifade olarak çalıştırır; Postgres ilk çalıştırmalarda parametre
  değerleriyle özel plan kurar ve sabit `CASE` dallarını katlar (`created_at DESC` varsayılanı
  `app_logs_created_idx` indeksini + incremental sort'u kullanır, doğrulandı). Genel plan tam sıralama
  yapar ve büyük tablolarda maliyeti yüksek çıktığı için planlayıcı özel planda kalır. Çok büyük bir tabloda
  bu yetmezse sıralama için ayrı sorgu (ör. varsayılan sıra için sabit ORDER BY'lı ikinci sorgu) düşünülür;
  yeni indeks ihtiyacı koordinatöre bildirilir.

## 4. Uç kontrol listesi

1. Modülde `SortSpec` (whitelist + varsayılan); `/meta` varsa `DefaultSort`/`SortableFields` spec'ten.
2. Handler: `Parse` → `ResolveSort` → filtre yardımcıları → usecase/sqlc'ye `SortKey`/`SortDesc`.
3. sqlc: yukarıdaki ORDER BY kalıbı + `id` tiebreak; çok değerli filtreler `text[]` + `ANY`.
4. Export/bulk "filtreye uyan tümü" sorguları aynı filtre parametrelerini (CSV) aynı anlamla okur.
5. OpenAPI: `$ref: '#/components/parameters/Sort'`, filtre parametreleri (çok değerli olanlar
   `style: form, explode: false` dizi), `make api-generate`.
6. Test: asc/desc gerçekten sıralıyor, eşit değerlerde `id` tiebreak kararlı, bilinmeyen sort → 400,
   çok değerli filtre çalışıyor (örnek: `internal/httpserver/list_sort_contract_integration_test.go`).
