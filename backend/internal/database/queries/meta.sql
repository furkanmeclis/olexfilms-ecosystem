-- name: PingDB :one
SELECT 1::int AS ok;

-- name: ExtensionExists :one
SELECT EXISTS(
  SELECT 1 FROM pg_extension WHERE extname = $1
) AS exists;
