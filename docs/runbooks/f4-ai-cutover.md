# F4 AI geçiş runbook'u: eski AI katmanı ve Evolution API kapatma

Kapsam: design §7 kapatma sırasının (4). adımı. Eski AI katmanı (olexfilms-ai-layer-go: whatsmeow
WhatsApp botu, `notifications/send` API'si, hub M2M köprüsü) ve hub'ın kullandığı Evolution API
kapanır; WhatsApp tek numarada wuzapi + yeni app AI pipeline'ı (F4-02) ile çalışır. Uygulama işi
F4-05b (ENV) içindir; bu belge yalnız hazırlıktır. Her adımda **Komut**, **Doğrulama** ve
**Geri dönüş** vardır. Bir adımın doğrulaması geçmeden sonrakine geçilmez.

Kapsam dışı: AI katmanı konuşmaları ve bilgi tabanı taşınmaz (design §7 "Taşınmaz", QUESTIONS #19);
yalnız arşiv yedeği alınır (§2).

## 0. Değişkenler

Komutlar prod Docker host'unda çalışır (design §6, `docs/deploy.md`). Değerleri uygulama günü
doldurun:

```bash
NEW=/path/to/olexfilms-ecosystem            # yeni app compose dizini (.env.server burada)
AI_DIR=/path/to/olexfilms-ai-layer-go         # eski AI katmanı compose dizini
AI_PROJECT=olexfilms-ai                       # eski AI compose proje adı (COMPOSE_PROJECT_NAME)
ARCHIVE=/srv/backups/f4-ai-cutover-$(date +%Y%m%d)
dc()    { docker compose --env-file "$NEW/.env.server" -f "$NEW/compose.prod.yml" "$@"; }
appsql(){ dc exec -T postgres psql -U "$DB_USER" -d "$DB_NAME" -At -c "$1"; }
aisql() { docker compose -p "$AI_PROJECT" -f "$AI_DIR/docker-compose.yml" exec -T postgres \
            psql -U "${AI_DB_USER:-olexfilms}" -d "${AI_DB_NAME:-olexfilms_ai}" -At -c "$1"; }
```

`DB_USER`/`DB_NAME` yeni app'in `.env.server` değerleridir; `AI_DB_USER`/`AI_DB_NAME` eski
katmanın `.env` değerleridir (varsayılan `olexfilms` / `olexfilms_ai`).

API ile sağlık kontrolü (backend dışa açık değil; iç ağdan, panel alan adı `Host` başlığıyla,
super_admin access token'ı ile):

```bash
api() { docker run --rm --network olexfilms-app curlimages/curl -s \
          -H "Host: $PANEL_HOST" -H "Authorization: Bearer $TOKEN" "http://backend.olexfilms-app:8080$1"; }
api /v1/platform/whatsapp | jq '.data | {status, phone, ai_pipeline}'
```

`ai_pipeline` alanı (TEC-409) son 24 saatte başlayan AI çalıştırmalarını sayar: `runs`,
`completed`, `failed`, `skipped`, `running`, `error_rate` = failed / (completed + failed),
`last_run_at`, `last_failed_at`. Aynı sayılar DB'den:

```bash
appsql "SELECT status, count(*) FROM conversation_ai_runs
        WHERE created_at > now() - interval '24 hours' GROUP BY 1 ORDER BY 1"
```

Ekran: Platform → Entegrasyonlar → WhatsApp (`/platform/integrations/whatsapp`, bağlantı durumu ve
numara), Platform → Konuşmalar (`/platform/conversations`).

## 1. Ön koşullar

| # | Koşul | Doğrulama |
|---|---|---|
| 1.1 | F4-02 canlı: wuzapi bağlı, webhook geliyor, AI pipeline çalışıyor | `api /v1/platform/whatsapp` → `status = "connected"`, `logged_in = true`, `webhook_configured = true`; `ai_pipeline.runs > 0` ve `ai_pipeline.error_rate < 0.05` (son 24 sa) |
| 1.2 | ENV doğrulamaları geçti: F4-01k (asistan), F4-02h (WhatsApp), F4-03e (MCP), F4-04f (kampanya) | Linear'da dört iş **Done** |
| 1.3 | Parite testi yeşil (§8) | `cd backend && go test -count=1 -run TestParityTableNamesAreRegistered ./internal/modules/ai/tools/` → `ok` |
| 1.4 | Kalıntı taraması temiz (§9) | `cd backend && go test -count=1 -run TestNoLegacyAIReferences ./internal/modules/ai/tools/` → `ok` |
| 1.5 | Yeni app'in güncel yedeği alındı | `NAME_PREFIX=olexfilms BACKUP_DIR=/srv/backups scripts/backup.sh` → `SHA256SUMS` doğrulandı |
| 1.6 | Numara durumu belirlendi (QUESTIONS #18): eski AI katmanının numarası merkez numarasıyla **aynı** mı **farklı** mı | Eski panelde bağlı numara ↔ `api /v1/platform/whatsapp` `.phone`; karar bu belgenin uygulama kaydına yazılır |
| 1.7 | Bakım penceresi (düşük trafik, ör. Pazar 08:00–10:00) ve sorumlu kişi belli | — |

**Geri dönüş:** Bu adım hiçbir şeyi değiştirmez.

## 2. Arşiv yedeği (veri)

AI katmanı konuşmaları yeni app'e taşınmaz (design §7). Kapatmadan önce eski katmanın tamamı arşivlenir;
arşiv sunucu dışına şifreli kopyalanır ve KVKK saklama politikasına göre (öneri: 1 yıl) tutulup
sonra silinir.

**Komut**

```bash
mkdir -p "$ARCHIVE"
docker compose -p "$AI_PROJECT" -f "$AI_DIR/docker-compose.yml" exec -T postgres \
  pg_dump -U "${AI_DB_USER:-olexfilms}" -Fc "${AI_DB_NAME:-olexfilms_ai}" > "$ARCHIVE/olexfilms_ai.dump"
# whatsmeow oturumu (SQLite) ve dışa aktarım dosyaları Postgres'te değil:
docker run --rm -v olexfilms-whatsmeow-data:/v:ro -v "$ARCHIVE":/b alpine tar czf /b/whatsmeow-data.tgz -C /v .
docker run --rm -v olexfilms-exports-data:/v:ro   -v "$ARCHIVE":/b alpine tar czf /b/exports-data.tgz   -C /v .
(cd "$ARCHIVE" && sha256sum * > SHA256SUMS)
```

Volume adları eski `.env`'deki `WHATSMEOW_VOLUME_NAME` / `EXPORTS_VOLUME_NAME` ile doğrulanır
(`docker volume ls | grep olexfilms-`).

**Doğrulama**

```bash
(cd "$ARCHIVE" && sha256sum -c SHA256SUMS)
pg_restore --list "$ARCHIVE/olexfilms_ai.dump" | grep -E 'TABLE DATA public (conversations|messages|ai_runs|tool_executions|outbound_deliveries|api_keys)'
aisql "SELECT 'conversations', count(*) FROM conversations UNION ALL SELECT 'messages', count(*) FROM messages"
```

Dökümde altı tablo görünür; sayımlar uygulama kaydına yazılır (arşivin bütünlüğü için referans).

**Geri dönüş:** Gerekmez (salt okuma). Arşiv §6 sonrası geri dönüşte de kaynak olarak kullanılır.

## 3. `notifications/send` çağıranı yok kontrolü

Eski katmanın tek dış API'si `POST /api/v1/notifications/send` (API anahtarı `api_keys`, kayıt
`outbound_deliveries`). Referans taramasında çağıran yoktur: hub bu uca istek atmaz (ilişki ters
yöndedir, eski katman hub M2M ucunu çağırır); yeni app bildirimlerini kendi `notifications` modülü ile
wuzapi üzerinden gönderir. Eski katman başarılı istekleri loglamadığı için kontrol DB üzerinden yapılır.

**Komut / Doğrulama**

```bash
# Son 30 günde uca gelen istekler, anahtar sahibine göre. Beklenen: satır yok.
aisql "SELECT k.app_name, k.key_prefix, count(*), max(d.created_at)
       FROM outbound_deliveries d JOIN api_keys k ON k.id = d.api_key_id
       WHERE d.created_at > now() - interval '30 days' GROUP BY 1, 2 ORDER BY 4 DESC"
# Ters proxy erişim logu (Traefik / Dokploy). Beklenen: eşleşme yok.
docker logs --since 720h "$(docker ps -qf name=traefik)" 2>&1 | grep -c 'notifications/send'
```

Satır çıkarsa `app_name` sahibi belirlenir, çağrı yeni app'e (bildirim kataloğu) taşınana kadar
kapatma ertelenir.

Yumuşak kesme: aktif anahtarlar iptal edilir, 48 saat `api key auth failed` log satırı izlenir.

```bash
aisql "UPDATE api_keys SET revoked_at = now() WHERE revoked_at IS NULL RETURNING app_name, key_prefix"
docker compose -p "$AI_PROJECT" -f "$AI_DIR/docker-compose.yml" logs --since 48h api | grep -c 'api key auth failed'   # beklenen: 0
```

**Geri dönüş:** `aisql "UPDATE api_keys SET revoked_at = NULL WHERE revoked_at >= '<iptal zamanı>'"`.

## 4. Merkez numarasının wuzapi'ye eşlenmesi / whatsmeow oturumunun kapatılması

WhatsApp bir numaraya birden çok bağlı cihaz izin verir: eski whatsmeow ve wuzapi aynı numaraya
aynı anda bağlıysa her mesaja iki bot yanıt verir. Bu yüzden **önce eski bot susar, sonra wuzapi
eşlenir**.

### 4A. Numaralar aynı

1. Eski katmanı durdur (whatsmeow mesaj almayı bırakır):
   `docker compose -p "$AI_PROJECT" -f "$AI_DIR/docker-compose.yml" stop api`
2. whatsmeow cihazını numaradan çıkar: telefonda WhatsApp → Ayarlar → **Bağlı cihazlar** → eski
   cihaz → Çıkış yap. (Alternatif: durdurmadan hemen önce eski panelde
   `POST /api/v1/admin/whatsapp/logout`, admin oturumuyla.)
3. wuzapi'yi eşle: Platform → Entegrasyonlar → WhatsApp → **Bağlan** → QR'ı telefondan okut (ya da
   eşleme kodu: `POST /v1/platform/whatsapp/pair-phone`).

**Doğrulama**

- Telefonda Bağlı cihazlar listesinde yalnız wuzapi cihazı var.
- `api /v1/platform/whatsapp` → `status = "connected"`, `phone` = merkez numarası (E.164).
- Ekrandan **Test mesajı** (`POST /v1/platform/whatsapp/test-message`) test telefonuna ulaşır.
- Test telefonundan merkez numarasına "merhaba" yazılır: mesaj Platform → Konuşmalar'da görünür, AI
  yanıtı tek kez gelir; birkaç dakika içinde `ai_pipeline.runs` artar, `ai_pipeline.last_run_at` güncellenir.

**Geri dönüş:** Platform → WhatsApp → **Çıkış** (`POST /v1/platform/whatsapp/logout`);
`docker compose -p "$AI_PROJECT" -f "$AI_DIR/docker-compose.yml" start api`; eski panelden QR ile
yeniden eşle (çıkış yapılan whatsmeow oturumu geçersizdir, yeniden QR gerekir; §2 arşivindeki
`main.db` geri yüklenemez).

### 4B. Numaralar farklı

wuzapi zaten merkez numarasında bağlıdır (F4-02h). Eski numaraya yazan müşteriler bilgilendirilir.

1. Son 90 günde eski numaraya yazanları çıkar (kolon adlarını `aisql "\d conversations"` ile doğrula):
   `aisql "SELECT DISTINCT phone FROM conversations WHERE updated_at > now() - interval '90 days'" > "$ARCHIVE/notify.txt"`
2. Bilgilendirme mesajını **eski numaradan** gönder (müşteri aynı sohbette görür), eski katmanın
   kendi gönderim ucu ile; dakikada en çok 20 mesaj (kampanya hızıyla aynı), 21:00–09:00 arası gönderme:
   > Olex Films WhatsApp hattı değişti. Bundan sonra garanti, hizmet ve randevu işlemleriniz için
   > `<merkez numarası>` numarasına yazabilirsiniz. Bu numara yakında yanıt vermeyecek.
3. Eski numaranın WhatsApp profil "Hakkında" metnine yeni numarayı yaz.
4. Gönderim bitince 4A'nın 1. ve 2. adımı (durdur + Bağlı cihazlardan çıkış). SIM en az 90 gün
   aktif kalır.

**Doğrulama**

- `aisql "SELECT status, count(*) FROM outbound_deliveries WHERE created_at > now() - interval '1 day' GROUP BY 1"`
  → `sent` sayısı `notify.txt` satır sayısına eşit (`failed` olanlar elle tekrar).
- Eski numaranın Bağlı cihazlar listesi boş; `api /v1/platform/whatsapp` → merkez numarasında `connected`.

**Geri dönüş:** `start api` ve eski panelden QR ile yeniden eşle; gönderilen bilgilendirme geri
alınamaz (gerekirse ikinci bir düzeltme mesajı).

## 5. Evolution instance'ının silinmesi

Hub F2'den beri salt okunurdur. Evolution yalnız hub'ın bayi SMS → WhatsApp yönlendirmesinde
kullanılıyordu (`SendSmsJob`: bayi sahibi/personeli alıcıya SMS yerine Evolution `sendText`). Yeni
app'te bayi bildirimleri `notifications` modülünün WhatsApp kanalından (wuzapi) gider.

**Komut**

```bash
# Hub DB (salt okuma): instance adı ve son Evolution gönderimi.
SELECT payload FROM settings WHERE "group" = 'evolution' AND name = 'default_instance';
SELECT count(*), max(created_at) FROM sms_logs
 WHERE channel = 'whatsapp' AND description = 'evolution' AND created_at > now() - interval '7 days';  -- beklenen: 0
# Evolution API (v2): önce oturumu kapat, sonra instance'ı sil.
curl -s -X DELETE "$EVOLUTION_API_URL/instance/logout/$INSTANCE" -H "apikey: $EVOLUTION_API_KEY"
curl -s -X DELETE "$EVOLUTION_API_URL/instance/delete/$INSTANCE" -H "apikey: $EVOLUTION_API_KEY"
```

**Doğrulama**

- `curl -s "$EVOLUTION_API_URL/instance/fetchInstances" -H "apikey: $EVOLUTION_API_KEY" | jq '.[].name'` listesinde `$INSTANCE` yok.
- Evolution'ın bağlı olduğu numaranın telefonunda Bağlı cihazlar listesinde Evolution cihazı yok.
- Yeni app'te bir bayi bildirimi (ör. test siparişi onayı) WhatsApp'tan bayiye ulaşır.

**Geri dönüş:** Hub salt okunur olduğundan Evolution'a geri dönüş gerekmez; zorunlu olursa
`POST /instance/create` + QR ile yeni instance açılır ve hub'daki `evolution` ayarları güncellenir.
Evolution verisi taşınmaz (design §7).

## 6. Container'ların, DB'nin ve Evolution'ın kapatılması

Sıra: önce durdur ve izle (48 sa), sonra kaldır; volume'ler 30 gün sonra silinir.

**Komut**

```bash
docker compose -p "$AI_PROJECT" -f "$AI_DIR/docker-compose.yml" stop          # api, frontend, postgres, mailhog
# Evolution: Dokploy'da Evolution uygulaması → Stop (repo dışında barındırılıyor).
# Hub /mcp/olex ucu ve M2M chatbot ucu: hub'da CHATBOT_M2M_HASH boşaltılır (eski uçlar kapanır, docs/MCP.md §8).
# Dokploy → eski AI panel domain'i kaldırılır.
# 48 saat sorunsuz geçince:
docker compose -p "$AI_PROJECT" -f "$AI_DIR/docker-compose.yml" down           # -v YOK: volume'ler kalır
# 30 gün sonra (arşiv §2 doğrulandıysa):
docker volume rm olexfilms-postgres-data olexfilms-whatsmeow-data olexfilms-exports-data
```

`olexfilms-postgres-data` adını eski `.env`'deki `POSTGRES_VOLUME_NAME` ile doğrulayın; yeni app'in
`olexfilms-postgres` volume'üyle karıştırmayın.

**Doğrulama**

- `docker ps -a --filter "label=com.docker.compose.project=$AI_PROJECT" --format '{{.Names}} {{.Status}}'` → stop sonrası hepsi `Exited`, down sonrası boş.
- `docker ps --format '{{.Names}}' | grep -i evolution` → boş.
- Yeni app etkilenmedi: `dc ps` hepsi `healthy`; `api /v1/platform/whatsapp` → `connected`,
  `ai_pipeline.error_rate` önceki değerin üstüne çıkmadı; `api /v1/platform/whatsapp | jq .data.ai_pipeline.last_run_at` son saat içinde.

**Geri dönüş:** `down` öncesi `start`; `down` sonrası ve volume'ler duruyorken
`docker compose -p "$AI_PROJECT" -f "$AI_DIR/docker-compose.yml" up -d`; volume'ler silindiyse §2
arşivinden `pg_restore` + tar ile geri yükleme, ardından 4A geri dönüşü.

## 7. Geri dönüş planı (genel)

| Adım | Geri dönüş | Süre sınırı |
|---|---|---|
| §2 arşiv | — | — |
| §3 anahtar iptali | `revoked_at = NULL` | her zaman |
| §4 numara | wuzapi çıkış → eski `start api` → QR | eski container'lar silinene kadar |
| §5 Evolution | yeni instance + QR (yalnız zorunluysa) | her zaman |
| §6 kapatma | `start` / `up -d`; volume silindiyse arşivden geri yükleme | volume'ler 30 gün |

Geri dönüş kararı: §4 sonrası ilk 24 saatte `ai_pipeline.error_rate > 0.2` (en az 20 tamamlanmış
çalıştırma ile) bir saatten uzun sürerse, wuzapi `banned` / `logged_out` alarmı çözülemezse ya da
müşteri mesajları konuşma kutusuna düşmüyorsa. Geri dönüş sürecinde yeni app'te açılan konuşmalar
yeni app'te kalır; eski katmana aktarılmaz.

## 8. Parite (F4-01d)

Eski katmanın tüm tool'ları, hub `/mcp/olex` tool'ları ve M2M chatbot uçları yeni tool'lara eşlenmiştir
ya da gerekçesiyle "bilinçli olarak yok"tur; eksik tool kalmadığı için Linear'a Backlog önerisi
gerekmedi (TEC-409). Tablo `backend/internal/modules/ai/tools/parity_test.go`
(`TestParityTableNamesAreRegistered`) ile doğrulanır: işaretçiler arasındaki her satır okunur, "Yeni"
sütunundaki her ad kayıt defterinde olmalı, kaynak başına satır sayısı (eski-ai 21, hub-mcp 12, m2m 16)
tutmalıdır. Bilgi tabanı (`knowledge_documents`) taşınmaz; yerine `ai_settings.knowledge_text` (QUESTIONS #19).

Kaynaklar (`Kaynak` sütunu): `eski-ai` = olexfilms-ai-layer-go `api/internal/tools/catalog` (18 sabit + 3 koşullu tool),
`hub-mcp` = olexfilms hub `/mcp/olex` (`app/Mcp/Tools`, 12 tool), `m2m` = hub M2M chatbot uçları
(`routes/api.php`, `/api/v1/m2m/chatbot`, 16 uç). "Yeni" sütunundaki her ad kayıt defterinde
olmalıdır (`parity_test.go`); karşılığı olmayan satır "bilinçli olarak yok" der ve gerekçesini
yazar.

Genel kurallar: eski katmanda `phone` parametreyle gelirdi; yeni sistemde kimlik kanaldan
(portal oturumu, WhatsApp kimlik çözümü TEC-394, MCP OAuth) gelir, tool girdisi değildir. PDF
üreten eski tool'ların yerine yalnız mevcut pdfrender çıktısına kısa link verilir; CSV/XLSX
üretilmez, model panel liste ekranının dışa aktarımına (io-engine export) yönlendirir
(`ExportNotice`).

<!-- parity:start -->
| Kaynak | Eski | Yeni | Gerekçe / not |
|---|---|---|---|
| eski-ai | `change_language` | `change_language` | müşteri: `users.locale`; ziyaretçinin dili konuşmada otomatik tespit edilir (TEC-394) |
| eski-ai | `m2m_health` | bilinçli olarak yok | M2M köprüsü yok; tool'lar aynı uygulamanın usecase'lerini çağırır |
| eski-ai | `identity_lookup` | bilinçli olarak yok | kimlik çözümü kanal katmanında (WhatsApp TEC-394, portal oturumu, MCP OAuth); modele açılmaz |
| eski-ai | `list_dealers` | `find_nearest_dealers` | il/ilçe veya konum; yalnız aktif ve sözleşmesi geçerli |
| eski-ai | `dealer_services_count` | `service_activity_summary` | tarih aralığı, kapsamlı |
| eski-ai | `dealer_brand_breakdown` | `service_activity_summary` | araç markası kırılımı |
| eski-ai | `dealer_services_by_plate` | `search_services` | boşluklu plaka da |
| eski-ai | `dealer_products_top` | `service_activity_summary` | en çok kullanılan ürünler |
| eski-ai | `dealer_inventory` | `stock_summary`, `stock_units` | alış fiyatı yalnız izinle |
| eski-ai | `dealer_service_pdf` | `service_pdf_link` | garanti sertifikası PDF kısa linkleri; PDF üretilmez |
| eski-ai | `customer_services` | `my_services` | |
| eski-ai | `customer_service_detail` | `my_service_detail` | |
| eski-ai | `customer_service_pdf` | `my_service_pdf_link` | portal hizmet sayfası + sertifika PDF kısa linkleri |
| eski-ai | `customer_vehicles` | `my_vehicles` | |
| eski-ai | `customer_warranties` | `my_warranties` | kalan gün |
| eski-ai | `customer_profile_link` | `my_profile_link` | |
| eski-ai | `customer_review_link` | `dealer_review_link` | |
| eski-ai | `search_knowledge` | `search_knowledge` | bilgi tabanı yerine tek `knowledge_text` (Q19) |
| eski-ai | `generate_csv` | bilinçli olarak yok | dosya üretimi yok; panel liste ekranı io-engine export'u (`ExportNotice`) |
| eski-ai | `generate_xlsx` | bilinçli olarak yok | dosya üretimi yok; panel liste ekranı io-engine export'u (`ExportNotice`) |
| eski-ai | `send_whatsapp_document` | bilinçli olarak yok | tool dosya göndermez; linkler metinde gider, WhatsApp medya gönderimi giden kuyrukta (F4-02d) |
| hub-mcp | `PhoneLookupTool` | bilinçli olarak yok | telefonla herkese açık kimlik sorgusu güvenlik açığı; MCP kimliği OAuth ile (F4-03) |
| hub-mcp | `CustomerRecentServicesTool` | `my_services` | |
| hub-mcp | `CustomerVehiclesTool` | `my_vehicles` | |
| hub-mcp | `CustomerWarrantyDaysRemainingTool` | `my_warranties` | |
| hub-mcp | `CustomerProfileLinkTool` | `my_profile_link` | |
| hub-mcp | `CustomerDealerGoogleBusinessReviewLinkTool` | `dealer_review_link` | |
| hub-mcp | `CustomerServiceInfoTool` | `my_service_detail` | |
| hub-mcp | `DealerThisMonthServiceCountTool` | `service_activity_summary` | varsayılan bu ay |
| hub-mcp | `DealerThisMonthBrandServiceCountTool` | `service_activity_summary` | marka kırılımı |
| hub-mcp | `DealerTopProductsTodayTool` | `service_activity_summary` | `from`/`to` bugün |
| hub-mcp | `DealerProductInventoryTool` | `stock_summary`, `stock_units` | |
| hub-mcp | `DealersByCityTool` | `find_nearest_dealers` | |
| m2m | `GET /health` | bilinçli olarak yok | M2M köprüsü yok |
| m2m | `POST /identity/lookup` | bilinçli olarak yok | kimlik çözümü kanal katmanında (TEC-394) |
| m2m | `GET /dealers` | `find_nearest_dealers` | |
| m2m | `GET /dealer/services/count` | `service_activity_summary` | |
| m2m | `GET /dealer/services/brand-breakdown` | `service_activity_summary` | |
| m2m | `GET /dealer/services/by-plate` | `search_services` | |
| m2m | `GET /dealer/products/top` | `service_activity_summary` | |
| m2m | `GET /dealer/inventory` | `stock_summary`, `stock_units` | |
| m2m | `GET /customer/services` | `my_services` | |
| m2m | `GET /customer/services/{service_no}/pdf` | `my_service_pdf_link` | |
| m2m | `GET /customer/services/{service_no}` | `my_service_detail` | |
| m2m | `GET /dealer/services/{service_no}/pdf` | `service_pdf_link` | |
| m2m | `GET /customer/vehicles` | `my_vehicles` | |
| m2m | `GET /customer/warranties` | `my_warranties` | |
| m2m | `GET /customer/links/profile` | `my_profile_link` | |
| m2m | `GET /customer/links/review` | `dealer_review_link` | |
<!-- parity:end -->

Hub `/mcp/olex`'in `OlexAssistantFaqResource` kaynağı `search_knowledge` ile, `OlexCombinedAssistantPrompt`
istemi sohbetin sistem istemiyle (F4-01f) karşılanır; yeni MCP uçları F4-03c'dedir.

## 9. Kalıntı kontrolü

Repoda eski AI katmanı ve Evolution adı yalnız bu runbook'ta ve `docs/design.md`'de geçer
(`rg -i "evolution|ai-layer"`). Tek istisna, metin editörünün İngilizce otomatik tamamlama sözlüğüdür
(`frontend/src/components/editor/plugins/auto-complete-plugin.tsx`: "evolution", "revolution"
kelimeleri; servis adı değil). Kural `TestNoLegacyAIReferences`
(`backend/internal/modules/ai/tools/legacy_refs_test.go`) ile CI'da korunur.

- `.env.example` ve `compose.local.yml` / `compose.prod.yml`: eski AI veya Evolution değişkeni/servisi
  yok (`LEGACY_HUB_*` yalnız migrator'ın hub okumasıdır).
- errtrack modül eşlemesi: `/mcp/*` → `mcp`; `/mcp/olex` yeni app'te yoktur, MCP uçları yalnız
  `/mcp/dealer`, `/mcp/customer`, `/mcp/user` (`TestMCPResourcesHaveNoLegacyEndpoint`).
- `docs/CLOUD_CHECKLIST.md` ve `AGENTS.md` referans repo listesinden eski AI katmanı reposu çıkarıldı.
