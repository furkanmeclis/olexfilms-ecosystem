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
- Elle doldurulacaklar: `SMTP_*` (veya `--smtp-*`), `WUZAPI_WEBHOOK_URL`, `LEGACY_*_DSN` (F2), `SA_*`.

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
| 13 | migrator | `APP_ROLE=migrator`, `profiles: [migrator]`; tek seferlik aktarım ve cutover araçları (`cutover-preflight`, `search-reindex`, `inventory-reconcile`) | app |

Başlangıç sırası `depends_on` ile şöyledir: altyapı servisleri healthy olur, sonra migrate başarıyla tamamlanır, sonra backend healthy olur, ardından worker'lar başlar, en son frontend. Frontend worker'ları yalnızca `service_started` olarak bekler. Böylece sağlıksız bir worker siteyi düşürmez.

- İç URL'ler `<servis>.<prefix>-app` biçimindedir (ör. `postgres.olexfilms-app`, `gotenberg.olexfilms-app:3000`, `wuzapi.olexfilms-app:8080`).
- Kuyruk grupları: `critical` = notifications, `default` = default/imports/bulk/search, `low` = maintenance, `docs` = exports. Birden fazla `SCHEDULER_ENABLED` worker çalışırsa Redis `SET NX PX` lease'i tek bir scheduler lideri seçer.
- Worker healthcheck'i `/tmp/worker.health` dosyasının tazeliğine bakar. Worker bu dosyayı her başarılı Redis ping'inde (15 sn'de bir) yeniler.
- `stop_grace_period`: backend 30 sn, worker'lar 35 sn.
- Migrator'ı çalıştırmak için: `docker compose --env-file .env.server -f compose.prod.yml --profile migrator run --rm migrator`.
- İlk tam aktarımdan ve her `migrator run --mode=full` çalıştırmasından sonra tüm arama indeksleri yeniden kurulur; backend açılışta yalnız boş indeksleri doldurduğu için bu adım atlanırsa dolu sayılan indeksler (ör. super admin'li `users`) eksik kalır: `docker compose --env-file .env.server -f compose.prod.yml --profile migrator run --rm --entrypoint /app/search-reindex migrator`. Sonra `--entrypoint /app/cutover-preflight` ile preflight çalıştırılır; `search_index_counts` satırı `PASS` olmalı (`WARN` cutover'ı durdurmaz). Sıra ve ayrıntı: [`docs/runbooks/migrator-cutover.md`](runbooks/migrator-cutover.md).

## 4. Yedek ve geri yükleme

```bash
NAME_PREFIX=olexfilms BACKUP_DIR=/srv/backups scripts/backup.sh   # cron ile günlük; dizini sunucu dışına kopyalayın
NAME_PREFIX=olexfilms scripts/restore.sh /srv/backups/<stamp>      # YIKICI, onay ister
scripts/dr-drill.sh                                                # geçici container'larda backup → silme → restore → doğrulama
```

Yedek şunları kapsar: app DB (`db.dump`), wuzapi DB (`wuzapi.dump`), SeaweedFS volume'ü, Meilisearch volume'ü ve wuzapi medya volume'ü. Her parça `SHA256SUMS` ile doğrulanır.

## 5. Lokal

`compose.local.yml` aynı altyapı servislerini (wuzapi dahil) ve ayrıca mailhog (1025/8025) ile adminer'ı (8081) içerir. Backend 8080'de, wuzapi 8082'de çalışır. Komut: `make local-dev`.

## 6. CI ve sürüm

`.github/workflows/ci.yml` her PR'da ve `main`'e her push'ta çalışır. Aynı ref'te yeni bir PR çalışması eskisini iptal eder; `main` çalışmaları iptal edilmez.

| Job | Ne yapar |
| --- | --- |
| `backend` | Postgres 18 + Redis 8 servisleri; `sqlc generate` + diff; golangci-lint (sabit sürüm, `backend/.golangci.yml`); `go vet`; `migrate up`; `go test ./... -count=1` (`TEST_DATABASE_URL` tanımlı, entegrasyon testleri gerçekten koşar); `make openapi-lint` |
| `frontend` | pnpm install, lint, `format:check`, typecheck, test, build |
| `contracts` | `make api-generate` + `git diff --exit-code` (OpenAPI kopyası, `frontend/src/generated/`); `make check-i18n` |
| `image-build` | Sahte secret'lı env (`gen-env-server.sh --app ci.example`) ile `compose.prod.yml` backend + frontend image'larını buildx bake ile derler; push yok, gha cache |
| `dr-drill` | `scripts/dr-drill.sh`: backup → silme → restore → doğrulama |
| `ci` | Yukarıdakilerin hepsini bekler; biri bile başarılı değilse kırmızı. Branch protection / auto-merge zorunlu check'i budur |
| `release-tag` | Yalnızca `main` push'unda ve `ci` yeşilse: sıradaki `v0.N` tag'ini atar |

Tag akışı: PR squash-merge → `main` push → CI yeşil → `release-tag` en büyük `v0.N`'i bulur, `v0.(N+1)` atar (hiç yoksa `v0.1`); HEAD zaten `v0.N` ise bir şey yapmaz → GitHub tag push webhook'u → Dokploy `v0.*` tag'ini çeker ve `docker compose up -d --build` çalıştırır. Image registry'ye push yoktur; Dokploy sunucuda derler. `GITHUB_TOKEN` ile atılan tag başka workflow tetiklemez ama webhook yine gider.

Elle yapılacak iki ayar (bir kez, repo/Dokploy sahibi):

1. GitHub → repo **Settings → Actions → General → Workflow permissions**: **Read and write permissions**. Job seviyesinde `contents: write` istenir; repo ayarı salt okunursa tag push'u 403 alır.
2. Dokploy → Compose uygulaması → **General / Provider**: GitHub kaynağı, trigger türü **Tag**, desen `v0.*`; **Deployments → Webhook URL**'ini GitHub → repo **Settings → Webhooks**'a ekleyin (content type `application/json`, event: **Just the push event**). Dokploy GitHub App ile bağlıysa webhook otomatik kurulur; yalnızca tag trigger'ını seçin.

Bu repo ve ajanlar prod'a bağlanmaz, deploy tetiklemez; deploy yalnızca tag ile olur. Bir sürümü geri almak için Dokploy'da önceki tag'i seçip yeniden deploy edin.
