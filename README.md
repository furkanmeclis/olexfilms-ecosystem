# Olexfilms Ecosystem

Olex Films'in garanti hub'ı, depo, AI katmanı ve müşteri portalını tek bir Go + Next.js uygulamasında birleştiren yeni sistem.

- Tasarım ve karar dokümanı: [docs/design.md](docs/design.md)
- Ajan çalışma kuralları: [AGENTS.md](AGENTS.md)
- İş takibi: Linear projesi **Olexfilms Tek App** (P-TEC-4), kilometre taşları F0–F5
- Domainler: `olexfilms.app`, `wss.olexfilms.app`

## Repo düzeni (hedef)

```
backend/     Go API, worker'lar, migrator (cmd/server, cmd/worker, cmd/migrator)
frontend/    Next.js panel, portal, public sayfalar, BFF
deploy/      centrifugo, nginx, scripts
scripts/     gen-env-server.sh, backup/restore/dr-drill
docs/        tasarım dokümanı, ADR'ler, API sözleşmeleri
compose.local.yml
compose.prod.yml
.env.example
```

## Taban

Kod tabanı `otopoly-go` altyapı katmanından türetilir; üzerine `technowide-ecosystem` düzeltmeleri (SeaweedFS, oturum, iOS push, compose düzeni, izin scope'ları, MCP) alınır. Ayrıntı ve referans yolları Linear F0 işlerinde.

## Yerel geliştirme

```bash
cp .env.example .env                 # secret'ları doldur; .env commit'lenmez
make local-dev                       # infra (docker) + migrate-up + backend (air) + frontend
make create-super-admin SA_EMAIL=admin@olexfilms.app SA_PASSWORD=...
```

- API sözleşmesi: `docs/openapi.yaml` (kanonik). Değiştirince `make api-generate`
  (backend'in gömdüğü kopyayı senkronlar ve frontend tiplerini üretir).
- Backend: `cd backend && go vet ./... && go test ./...`
- Frontend: `cd frontend && pnpm lint && pnpm typecheck && pnpm build`
