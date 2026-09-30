# Development toolchain

Local tooling for the Go API under `backend/`.

## Tools

| Tool | Purpose |
|------|---------|
| Go | Language / runtime |
| Docker + Compose | Local infra (repo-root compose files) |
| golang-migrate | PostgreSQL migrations |
| sqlc | Type-safe SQL → Go |
| air | Hot reload (`make dev`) |
| golangci-lint | Lint |
| gofumpt | Stricter formatting |

## Install (macOS / Homebrew)

```bash
brew install golang-migrate sqlc golangci-lint gofumpt
go install github.com/air-verse/air@latest
```

Ensure `$(go env GOPATH)/bin` is on `PATH`.

## Common commands

From `backend/` (or `make -C backend <target>` from the repo root):

```bash
make tools           # print tool versions (incl. sqlc)
make migrate-up      # apply migrations
make migrate-down    # rollback one migration
make migrate-create NAME=description   # new seq migration pair
make sqlc            # sqlc generate
make run             # API once
make dev             # air hot reload
make worker          # standalone Asynq worker
make test            # go test ./...
make lint            # golangci-lint
make fmt             # gofumpt -w .
make health          # curl /healthz + /readyz
make openapi-lint    # Redocly lint
```

Infra is started from the **repo root**: `make infra` / `make local-dev`.

## Local URLs

| Service | URL |
|---------|-----|
| API (host) | `http://127.0.0.1:8080` |
| Scalar | `http://127.0.0.1:8080/docs/` |
| Health | `http://127.0.0.1:8080/healthz` |
| Ready | `http://127.0.0.1:8080/readyz` |
| Adminer | `http://127.0.0.1:8081` |
| MinIO API | `http://127.0.0.1:9000` |
| MinIO Console | `http://127.0.0.1:9001` |
| MailHog UI | `http://127.0.0.1:8025` |
| MailHog SMTP | `127.0.0.1:1025` |
| Postgres | `127.0.0.1:5432` |
| Redis | `127.0.0.1:6379` |
| Centrifugo WS | `ws://127.0.0.1:8000/connection/websocket` |
| Centrifugo API | `http://127.0.0.1:8000/api` |

## Notes

- Migrations: **golang-migrate** only; create with **`-seq`** via `make migrate-create`.
- SQL access: **sqlc** + pgx — see [`DATABASE_RULES.md`](DATABASE_RULES.md).
- Postgres image is **`postgres:18-alpine`**. `000001` enables `pgcrypto`.
- Queue: in-process by default (`QUEUE_WORKER_INPROCESS=true`) or `make worker`.
- Storage: MinIO via Compose (`STORAGE_DRIVER=minio`).
