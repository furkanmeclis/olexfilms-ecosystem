-- sqlc catalog stubs only (not applied by golang-migrate).
-- Lets queries reference Postgres system catalogs during codegen.

CREATE TABLE IF NOT EXISTS pg_extension (
  extname name NOT NULL
);
