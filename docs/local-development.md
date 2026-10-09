# Yerel geliştirme

Lokal paneli gerçekçi veriyle doldurmak için:

```sh
make seed-demo
```

Komut yalnız `APP_ENV!=production` ve yerel Postgres hostlarında çalışır. `TEST_DATABASE_URL`
tanımlıysa hedef olarak onu kullanır; aksi halde `.env` içindeki DB ayarlarını okur.

## Arama indekslerini yeniden kurma

Meilisearch indekslerinin hepsini (users, roles, ürünler, müşteriler, servisler, garantiler,
araçlar, organizasyonlar, siparişler, stok birimleri, leads) Postgres'ten yeniden doldurmak için:

```sh
make search-reindex
```

Hedef `backend/cmd/search-reindex`'i çalıştırır; bir spec başarısız olursa komut hata koduyla
çıkar. Spec listesi tek yerde tutulur: `backend/internal/modules/search/registry`. API sunucusu
(bootstrap), worker (`app:search:reindex` görevi) ve bu komut aynı listeyi kullanır; yeni bir
arama adapter'ı yalnız oraya eklenir, eklenmezse `TestAdaptersCoverEverySearchAdapter` kırılır.
Tek bir spec'i worker üzerinden yeniden kurmak için `app:search:reindex` görevi
`{"spec":"leads"}` yüküyle kuyruğa alınır; boş `spec` hepsini kurar.
