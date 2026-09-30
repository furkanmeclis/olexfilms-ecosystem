# AGENTS.md — Olexfilms Ecosystem çalışma kuralları

Bu repo bir Claude Code cloud oturumu tarafından sıfırdan inşa edilir. Tek kaynak: **Linear projesi "Olexfilms Tek App" (P-TEC-4)** ve bu repodaki `docs/design.md`. Notion veya başka bir iş takip aracı kullanılmaz.

## 1. İş akışı

1. Linear'da sıradaki işi al: kilometre taşı sırası F0 → F1 → F2 → F3 → F4 → F5; aynı faz içinde `blocked by` ilişkisi olmayan ve önceliği yüksek olan önce. `list_issues` ile `project=P-TEC-4`, `state=Backlog|Todo` filtrele.
2. İşi **In Progress**'e al (`save_issue` ile `state`). İşin açıklamasındaki **Kapsam**, **Kabul** ve **Referans** bölümlerini oku; referans repolardaki dosyaları GitHub üzerinden incele (yerel yollar bilgi amaçlı, cloud'da yok).
3. Branch: `feat/tec-<no>-<kisa-slug>` (Linear'ın verdiği `gitBranchName` de olur).
4. Küçük, sık commit. Commit mesajı: `TEC-<no>: <ne yapıldı>`. Linear commit'i otomatik bağlar.
5. İş bitince PR aç, başlıkta `TEC-<no>`; PR açıklamasına kabul kriterlerinin nasıl doğrulandığını yaz. **Auto-merge açık**: CI yeşilse PR kendiliğinden squash-merge olur, branch silinir. PR'ı bekleme, bir sonraki işe geç; merge olunca Linear işini **Done** yap (Linear GitHub entegrasyonu varsa otomatik).
6. Kabul kriteri sağlanamıyorsa işi Done yapma; Linear'a yorum yaz (`save_comment`), neyin eksik kaldığını ve nedenini belirt, işi **Blocked**/**Todo**'ya çek.
7. Tasarım kararıyla çelişen bir durum bulursan kodu tahminle değiştirme; Linear işine yorum yaz ve `docs/design.md`'deki ilgili karar numarasını (K1–K29) belirt.

## 2. Bağlam (context) hijyeni

- Her işe **temiz bağlamla** başla: önce `docs/design.md`'nin ilgili bölümünü ve Linear işini oku, tüm dokümanı her seferinde yükleme.
- Büyük dosyaları tamamen okuma; `grep`/`rg` ile ilgili kısmı bul, yalnızca gereken aralığı oku.
- Referans repolarda (otopoly-go, technowide-ecosystem, olexfilms, olexfilms-warehouse, olexfilms-ai-layer-go) ihtiyacın olan dosyayı `gh api` veya raw URL ile çek, repoyu klonlama.
- Uzun süren araştırmayı alt ajana (subagent) ver ve sadece özetini al.
- Bir iş 3 saatten uzun sürüyorsa böl: Linear'da alt iş (`parentId`) aç, PR'ı küçük tut.
- Test ve lint çıktısının yalnızca başarısız kısmını oku.
- Sohbet uzayınca `/compact` benzeri özetleme yap; "şu an hangi iş, hangi branch, ne kaldı" üç satırını korumak yeter.

## 3. Kod kuralları

- Backend: Go 1.26, `net/http` ServeMux, sqlc, golang-migrate, Asynq, Centrifugo, Meilisearch, SeaweedFS (S3), Gotenberg, wuzapi. Modül düzeni `backend/internal/modules/<ad>/{handler,usecase,repository,model}` + `routes.go`. Platform katmanı `backend/internal/platform/*`.
- Frontend: Next.js 16 App Router, React 19, Tailwind 4, shadcn, TanStack Query, NextAuth BFF. Tarayıcı Go'ya doğrudan gitmez; yalnızca `/api/v1` BFF'i. Özellik klasörü `frontend/src/features/<ad>`.
- Her iş tablosunda `organization_id` + `brand_id`. RLS yok; scope uygulama katmanında.
- Ledger tabloları append-only; güncel durum projeksiyonda. Modüller arası iletişim outbox + event bus.
- Feature flag'ler DB'de; `RequireFeature(module)` middleware'i.
- Telefonlar E.164. Para tutarları `NUMERIC(18,2)`, para birimi `CHAR(3)`, kur `rate_snapshot` ile dondurulur.
- i18n: 13 dil (tr, en, bg, de, el, uk, ru, fr, es, it, zh_CN, az, ar); RTL için logical properties. Yeni UI metni her zaman i18n anahtarıyla.
- Her modülde usecase testi; kritik akışlarda uçtan uca senaryo (F1-13); Playwright ana akışlar.
- OpenAPI (`docs/openapi.yaml`) değişince `pnpm api:generate` çalıştır ve üretilen tipleri commit'le.
- Secret'lar `.env`'de; `.env.example` güncel tutulur; `scripts/gen-env-server.sh` prod env'i üretir.

## 4. Sınırlar

- Prod sunucuya (technowide-spaceship) bağlanma, deploy tetikleme; Dokploy tag'den deploy eder.
- Eski sistemlerin veritabanlarına yazma; migrator yalnızca okur ve yeni DB'ye yazar.
- Linear'da iş silme, kilometre taşı değiştirme, yeni etiket açma yok; yorum ve durum güncellemesi serbest.
- Kapsam dışı ve ertelenen maddeleri (`docs/design.md` bölüm 9) yapma; ihtiyaç görürsen Linear'a "Backlog" işi olarak öner.

## 5. Her PR'da kontrol listesi

- [ ] Linear iş numarası başlıkta ve commit'lerde
- [ ] Kabul kriterleri PR açıklamasında tek tek işaretli
- [ ] `go vet`, `golangci-lint`, `go test ./...` yeşil
- [ ] `pnpm lint`, `pnpm typecheck`, `pnpm build` yeşil
- [ ] Yeni tablo → migration + sqlc + `organization_id`/`brand_id`
- [ ] Yeni UI metni → i18n anahtarı (en az tr + en)
- [ ] `.env.example` ve `docs/openapi.yaml` güncel
