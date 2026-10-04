# Yerel geliştirme

Lokal paneli gerçekçi veriyle doldurmak için:

```sh
make seed-demo
```

Komut yalnız `APP_ENV!=production` ve yerel Postgres hostlarında çalışır. `TEST_DATABASE_URL`
tanımlıysa hedef olarak onu kullanır; aksi halde `.env` içindeki DB ayarlarını okur.
