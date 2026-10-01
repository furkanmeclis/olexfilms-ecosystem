# Prod deploy (Dokploy + Traefik)

Prod tek bir Docker host'ta `compose.prod.yml` ile çalışır (tasarım §6, 13 container). Dokploy repo'yu `v0.N` tag'inden çeker, `.env.server` ile `docker compose up -d --build` çalıştırır. Bu ajanlar prod sunucuya bağlanmaz (AGENTS §4).

## 1. Env üretimi

```bash
scripts/gen-env-server.sh --app olexfilms.app --realtime wss.olexfilms.app
# ya da: make gen-env-server
make prod-config        # docker compose config: eksik zorunlu anahtar varsa burada hata verir
```

- Tüm secret'lar rastgele üretilir: DB, Redis, SeaweedFS (S3 + JWT), Centrifugo, JWT, Meili, `APP_ENCRYPTION_KEY`, NextAuth, VAPID, wuzapi (`WUZAPI_ADMIN_TOKEN`, `WUZAPI_WEBHOOK_SECRET`, `WUZAPI_GLOBAL_ENCRYPTION_KEY`).
- Tekrar çalıştırınca mevcut değerler korunur; fark yalnızca tarih satırıdır.
- `--rotate` secret'ları yeniler. `DB_PASSWORD` ve S3 anahtarları değişirse çalışan Postgres rolünü ve SeaweedFS kimliğini elle güncelleyin.
- `APP_ENCRYPTION_KEY` `--rotate` ile değişmez, yalnızca `--rotate-encryption-key` ile değişir. Değişirse şifreli saklanan veriler okunamaz.
- `WUZAPI_GLOBAL_ENCRYPTION_KEY` script tarafından hiç döndürülmez, çünkü değişirse WhatsApp oturumları kaybolur. Yenilemek için satırı silip script'i tekrar çalıştırın.
- Elle doldurulacaklar: `SMTP_*` (veya `--smtp-*`), `WUZAPI_WEBHOOK_URL`, `MIGRATOR_*_DSN` (F2), `SA_*`.

## 2. Domain eşlemesi (Traefik / Dokploy)

Dışa yalnızca iki servis açıktır; ikisi de `dokploy-network`'e bağlıdır. Hiçbir serviste host `ports` yoktur.

| Domain | Compose servisi | Container portu | Not |
| --- | --- | --- | --- |
| `olexfilms.app` | `frontend` | `3000` | Panel, portal, public sayfalar, `/api/v1` BFF |
| `wss.olexfilms.app` | `centrifugo` | `8000` | WebSocket: `wss://wss.olexfilms.app/connection/websocket` |

Dokploy → Compose uygulaması → **Domains** sekmesinde her satır için: host, service name (`frontend` / `centrifugo`), container port, HTTPS açık (Let's Encrypt). Host'ta bir kez çalıştırın: `docker network create dokploy-network`.

Backend dışa açık değildir. Tarayıcı Go'ya doğrudan gitmez; frontend `API_URL=http://backend.<prefix>-app:8080/v1` ile app ağı üzerinden çağırır. Backend `dokploy-network`'e bağlanmaz, çünkü o ağda `backend` adı başka stack'lerin backend'ine çözülebilir.

## 3. Container'lar ve başlangıç sırası

| # | Servis | Rol | Ağ |
| --- | --- | --- | --- |
| 1 | postgres | Ana DB + `wuzapi` DB (`deploy/postgres/initdb`) | app |
| 2 | redis | Kuyruk, cache, rate limit, scheduler lease | app |
| 3 | meilisearch | Arama | app |
| 4 | seaweedfs | S3 dosya deposu (private) | app |
| 5 | centrifugo | Realtime | app + dokploy-network |
| 6 | gotenberg | HTML→PDF | app |
| 7 | wuzapi | WhatsApp gateway (`asternic/wuzapi:1.0.9`) | app |
| 8 | migrate | `APP_ROLE=migrate`, tek seferlik | app |
| 9 | backend | `APP_ROLE=server`, `/healthz` | app |
| 10 | worker-core | `WORKER_QUEUES=critical,default,low`, `SCHEDULER_ENABLED=true` | app |
| 11 | worker-docs | `WORKER_QUEUES=docs` (PDF/export), scheduler kapalı | app |
| 12 | frontend | Next.js, `/health` | app + dokploy-network |
| 13 | migrator | `APP_ROLE=migrator`, `profiles: [migrator]`; F2'ye kadar "not implemented" | app |

Başlangıç sırası `depends_on` ile şöyledir: altyapı servisleri healthy olur, sonra migrate başarıyla tamamlanır, sonra backend healthy olur, ardından worker'lar başlar, en son frontend. Frontend worker'ları yalnızca `service_started` olarak bekler. Böylece sağlıksız bir worker siteyi düşürmez.

- İç URL'ler `<servis>.<prefix>-app` biçimindedir (ör. `postgres.olexfilms-app`, `gotenberg.olexfilms-app:3000`, `wuzapi.olexfilms-app:8080`).
- Kuyruk grupları: `critical` = notifications, `default` = default/imports/bulk/search, `low` = maintenance, `docs` = exports. Birden fazla `SCHEDULER_ENABLED` worker çalışırsa Redis `SET NX PX` lease'i tek bir scheduler lideri seçer.
- Worker healthcheck'i `/tmp/worker.health` dosyasının tazeliğine bakar. Worker bu dosyayı her başarılı Redis ping'inde (15 sn'de bir) yeniler.
- `stop_grace_period`: backend 30 sn, worker'lar 35 sn.
- Migrator'ı çalıştırmak için: `docker compose --env-file .env.server -f compose.prod.yml --profile migrator run --rm migrator`.

## 4. Yedek ve geri yükleme

```bash
NAME_PREFIX=olexfilms BACKUP_DIR=/srv/backups scripts/backup.sh   # cron ile günlük; dizini sunucu dışına kopyalayın
NAME_PREFIX=olexfilms scripts/restore.sh /srv/backups/<stamp>      # YIKICI, onay ister
scripts/dr-drill.sh                                                # geçici container'larda backup → silme → restore → doğrulama
```

Yedek şunları kapsar: app DB (`db.dump`), wuzapi DB (`wuzapi.dump`), SeaweedFS volume'ü, Meilisearch volume'ü ve wuzapi medya volume'ü. Her parça `SHA256SUMS` ile doğrulanır.

## 5. Lokal

`compose.local.yml` aynı altyapı servislerini (wuzapi dahil) ve ayrıca mailhog (1025/8025) ile adminer'ı (8081) içerir. Backend 8080'de, wuzapi 8082'de çalışır. Komut: `make local-dev`.
