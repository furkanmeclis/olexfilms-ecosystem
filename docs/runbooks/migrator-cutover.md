# Migrator cutover runbook'u: tam aktarım, arama indeksleri ve preflight

Kapsam: design §7 kapatma sırasının (1) ve (2). adımları: migrator'ın ilk tam aktarımı (ve demo /
prova için her `--mode=full` çalıştırma), ardından delta ile kapanış ve go/no-go kontrolü
(`cutover-preflight`). Her adımda **Komut** ve **Doğrulama** vardır; bir adımın doğrulaması
geçmeden sonrakine geçilmez.

## 0. Değişkenler

Komutlar prod Docker host'unda, yeni app compose dizininde çalışır (`docs/deploy.md`):

```bash
dc() { docker compose --env-file .env.server -f compose.prod.yml "$@"; }
tool() { dc --profile migrator run --rm --entrypoint "/app/$1" migrator "${@:2}"; }
```

`tool` migrator servisinin env'iyle (DB, Meili, `LEGACY_*_DSN`) image'daki bir yardımcı komutu
çalıştırır.

## 1. Tam aktarım

**Komut:**

```bash
dc --profile migrator run --rm migrator run --profile=olex --mode=full
```

**Doğrulama:** `dc --profile migrator run --rm migrator report --profile=olex` 0 mismatch ile çıkar.

## 2. Tüm arama indekslerini yeniden kur (her full çalıştırmadan sonra zorunlu)

Migrator Postgres'e doğrudan yazar, arama indeksine iş kuyruğa almaz. Backend açılışta yalnız
**boş** indeksleri doldurur: içinde tek belge olan bir indeks (ör. super admin kaydı yüzünden
`users`) dolu sayılır ve aktarımdan sonra eksik kalır (TEC-525, demo provası). Bu yüzden ilk tam
aktarımdan ve **her** `migrator run --mode=full` sonrasında, delta'dan ve preflight'tan önce:

**Komut:**

```bash
tool search-reindex
```

Komut kayıtlı her spec'i (users, roles, ürünler, müşteriler, servisler, garantiler, araçlar,
organizasyonlar, siparişler, stok birimleri; TEC-524 sonrası leads de) Postgres'ten yeniden yükler.
Yerelde karşılığı `make search-reindex`. Uzun sürebilir; backend ve worker çalışırken güvenle
koşar (belgeler upsert edilir).

**Doğrulama:** `tool cutover-preflight` çıktısında `search_index_counts` satırı `PASS`.

`WARN` ise satır her farklı indeksi `spec: index N, database M` olarak yazar:

- `index < database` (boş/eksik indeks): `tool search-reindex` tekrar çalıştırılır; düzelmezse
  `search_reindex_*` log satırlarına bakılır.
- `index > database` (silinmiş kayıtların eski belgeleri): reindex yalnız upsert eder, eski belgeyi
  silmez. İndeksi Meilisearch'ten silip (`DELETE /indexes/<MEILI_INDEX_PREFIX>_<spec>`, master key
  ile) `tool search-reindex` çalıştırılır.

Tek bir spec'i worker üzerinden kurmak da mümkündür: `app:search:reindex` görevi `{"spec":"users"}`
yüküyle kuyruğa alınır (boş `spec` hepsini kurar).

## 3. Delta ile kapanış

Eski hub salt okunur yapıldıktan sonra:

```bash
dc --profile migrator run --rm migrator run --profile=olex --mode=delta
```

Delta yalnız değişen satırları yazar; delta sonrasında da `search_index_counts` `WARN` verirse
§2'deki `tool search-reindex` tekrarlanır.

## 4. Go/no-go: cutover-preflight

**Komut:**

```bash
tool inventory-reconcile   # aktif Glorian bağlantısı varsa, preflight'tan hemen önce
tool cutover-preflight
```

**Doğrulama:** çıkış kodu 0 ve tabloda `FAIL` yok. `WARN` cutover'ı durdurmaz ama açıklaması
okunur ve giderilir; `search_index_counts` için §2.

Sıra özeti: `migrator run --mode=full` → `migrator report` → `search-reindex` → (delta) →
`inventory-reconcile` → `cutover-preflight`.
