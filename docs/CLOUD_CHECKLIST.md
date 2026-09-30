# Cloud oturumu hazırlık checklist'i

Claude Code cloud oturumu başlatmadan önce Furkan'ın tamamlayacağı adımlar. Her madde "evet" olmadan kickoff prompt'u gönderilmez.

## A. GitHub

- [ ] `furkanmeclis/olexfilms-ecosystem` reposu cloud ortamında seçili ve **yazma** izni var (Claude GitHub App bu repoya yetkili).
- [ ] Referans repolar Claude GitHub App'e **okuma** izniyle açık: `otopoly-go`, `technowide-ecosystem`, `nextjs-go-boilerplate`, `olexfilms`, `olexfilms-warehouse`, `olexfilms-ai-layer-go`, `glorian`, `go-ubltr`. (Ajan bunları klonlamayacak, `gh api`/raw ile dosya çekecek; izin yoksa 404 alır.)
- [ ] Repo ayarları doğrulandı (bu oturumda yapıldı): auto-merge açık, yalnızca squash, merge sonrası branch silinir, `main` ruleset'i `ci` check'ini zorunlu kılar, review zorunlu değil.
- [ ] `gh` cloud ortamında kimlikli: oturumda `gh auth status` çalışıyor ve `gh pr create` / `gh pr merge --auto --squash` yetkisi var.
- [ ] GitHub Actions dakikaları yeterli (her PR'da Go + pnpm build koşacak).

## B. Linear MCP

- [ ] Cloud oturumunda Linear MCP bağlı; `list_projects` çağrısı **Olexfilms Tek App (P-TEC-4)** döndürüyor.
- [ ] `save_issue` ile bir test işinin durumu değiştirilebiliyor (yazma yetkisi).
- [ ] `save_comment` çalışıyor.
- [ ] Linear GitHub entegrasyonu açık: `TEC-<no>` içeren branch/PR merge olunca iş otomatik Done'a geçiyor. (Açık değilse ajan elle Done yapacak; AGENTS.md bunu kapsıyor.)
- [ ] Proje kaynaklarından Notion linki kaldırıldı (elle).

## C. Ortam ve secret'lar

- [ ] Cloud ortamında Docker yoksa ajan testleri servislerle koşamaz; bu durumda CI'daki Postgres/Redis servisleri tek doğrulama olur. Karar: kabul edildi / cloud'da Docker var.
- [ ] Ajanın ihtiyaç duyacağı hiçbir prod secret verilmez. Prod deploy Dokploy'da elle tetiklenir (tag).
- [ ] Anthropic API anahtarı yalnızca F4'te, lokal test için; repoya ve Linear'a girmez.

## D. Oturum ayarları

- [ ] Model: en güçlü model; uzun oturum için otomatik özetleme (compaction) açık.
- [ ] İzinler: dosya yazma, `git`, `gh`, `go`, `pnpm`, `docker` (varsa) için onaysız çalışma; `rm -rf`, `git push --force`, prod SSH yasak.
- [ ] Oturum başlangıcında `AGENTS.md` okunuyor (CLAUDE.md yönlendiriyor).
- [ ] Kickoff prompt'u hazır: `docs/KICKOFF_PROMPT.md`.

## E. Sonra

- [ ] İlk PR (F0-01) merge olunca `docs/design.md` ve Linear'daki kapı tanımlarıyla karşılaştır; sapma varsa Linear yorumuyla düzelt.
- [ ] Her faz kapısında (F0…F5) oturumu yeniden başlatıp kickoff prompt'unun "devam" varyantını gönder (KICKOFF_PROMPT.md içinde).
