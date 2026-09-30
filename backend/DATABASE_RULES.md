# Database rules

## Engine

- **PostgreSQL** — primary system of record
- **Redis** — cache, queue (Asynq), ephemeral state — not authoritative business data
- **Object binary** — MinIO / S3; metadata may live in Postgres

## Access path

```
UseCase → Repository Interface → Repository Impl (sqlc / pgx) → PostgreSQL
```

### Forbidden

- SQL inside handlers / controllers
- Direct `*pgxpool.Pool` use in use cases (outside repositories)
- ORM-managed schema (GORM, etc.)
- Manual production schema changes outside migrations

## Migrations (mandatory)

- Tool: **golang-migrate**
- Location: `migrations/`
- Naming: **sequential** — `00000N_description.up.sql` / `.down.sql`
  - Create with: `make migrate-create NAME=<description>` (uses `-seq`)
  - Do **not** use timestamp naming
- Every schema change ships as a migration
- Both `up` and `down` are required (`down` data-loss risk must be obvious from the SQL)

### Workflow

1. `make migrate-create NAME=...`
2. Write up + down SQL
3. `make migrate-up` (local)
4. `make sqlc`
5. Update repository / tests
6. Include migration + generated `db` package in the PR

### Baseline squash

Historical migrations may be squashed into a clean `000001`–`000019` baseline that reflects the **current** schema (no create-then-alter patches). After a squash, existing local databases must be reset — not migrated incrementally from old version numbers:

```bash
# drops all tables + schema_migrations; then re-applies migrations
psql "$DB_URL" -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
make migrate-up
make create-super-admin
```

## SQLC

- Query sources: `internal/database/queries/*.sql`
- Generated package: `internal/database/db` (committed; **do not edit by hand**)
- Catalog stubs for codegen only: `internal/database/schema/` (not applied by migrate)
- Config: `sqlc.yaml`
- Regenerate: `make sqlc`

## Schema design principles

| Concern | Rule |
|---------|------|
| Internal PK | `BIGSERIAL` column `id` |
| Public id | `uuid` with `gen_random_uuid()` (pgcrypto) where exposed to API/clients |
| Timestamps | `timestamptz` — `created_at`, `updated_at` |
| Auto `updated_at` | `BEFORE UPDATE … EXECUTE FUNCTION set_updated_at()` (function from `000002`) |
| Soft delete | `deleted_at timestamptz NULL` when needed |
| RBAC | Global users; roles via `user_roles`; permissions via `role_permissions`. System roles (`is_system=true`) are immutable. `super_admin` is assigned by CLI, not a user column. |
| Money | `numeric` — never `float` / `double precision` |
| Enums | Postgres enum or `text` + check; keep app constants in sync |
| Indexes | Add for real query paths; avoid premature indexes |
| Foreign keys | Required unless explicitly justified |

## Naming

| Object | Convention |
|--------|------------|
| Tables | `snake_case`, plural (`users`, `roles`, `notifications`) |
| Columns | `snake_case` |
| PK | `id` |
| FK | `<singular>_id` (`user_id`, `role_id`) |
| Unique indexes | `uq_<table>_<cols>` |
| Indexes | `idx_<table>_<cols>` |

## Transactions

- Multi-write use cases own the transaction boundary
- Prefer single-responsibility repository methods
- Pass transactions via `context` / `WithTx`

## Migration safety

- Expand/Contract for breaking drops (two-step)
- Plan lock-heavy `ALTER` carefully for production
- Backfills may be separate migrations or jobs
- Seeds in migrations only for reference/system data — no demo users

## Redis rules

- Not the business source of truth
- Cache / queue / rate-limit / ephemeral session-style data
- Key prefix: `app:<env>:...`
- TTL required for cache/session-like keys

## Review checklist

- [ ] up + down migration present?
- [ ] FK / indexes correct?
- [ ] `make sqlc` regenerated and committed?
- [ ] Sensitive columns handled correctly?
- [ ] Rollback impact understood?
