-- TEC-84: currencies and daily exchange rates. Rates travel as text so no
-- precision is lost between NUMERIC and Go.

-- name: ListCurrencies :many
SELECT * FROM currencies
WHERE (NOT sqlc.arg(active_only)::bool OR is_active)
ORDER BY sort_order ASC, code ASC;

-- name: UpsertExchangeRate :exec
INSERT INTO exchange_rates (rate_date, base, quote, rate, source, note, created_by_user_id, fetched_at)
VALUES (
    sqlc.arg(rate_date), sqlc.arg(base), sqlc.arg(quote), sqlc.arg(rate)::text::numeric,
    sqlc.arg(source), sqlc.narg(note), sqlc.narg(created_by_user_id), NOW()
)
ON CONFLICT (rate_date, base, quote, source) DO UPDATE SET
    rate = EXCLUDED.rate,
    note = EXCLUDED.note,
    created_by_user_id = EXCLUDED.created_by_user_id,
    fetched_at = NOW();

-- name: DeleteManualExchangeRate :execrows
DELETE FROM exchange_rates
WHERE rate_date = sqlc.arg(rate_date) AND base = sqlc.arg(base) AND quote = sqlc.arg(quote)
  AND source = 'manual';

-- name: FindPairRate :one
-- The rate of a pair (either direction) on the latest day within
-- [min_date, on_date]; on that day manual > tcmb > ecb, direct before inverse.
SELECT rate_date, base, quote, rate::text AS rate, source
FROM exchange_rates
WHERE rate_date <= sqlc.arg(on_date) AND rate_date >= sqlc.arg(min_date)
  AND (
    (base = sqlc.arg(base) AND quote = sqlc.arg(quote))
    OR (base = sqlc.arg(quote) AND quote = sqlc.arg(base))
  )
ORDER BY rate_date DESC,
         CASE source WHEN 'manual' THEN 0 WHEN 'tcmb' THEN 1 ELSE 2 END,
         (base = sqlc.arg(base)) DESC
LIMIT 1;

-- name: ListExchangeRatesByDate :many
SELECT rate_date, base, quote, rate::text AS rate, source, note, fetched_at, updated_at
FROM exchange_rates
WHERE rate_date = sqlc.arg(rate_date)
  AND (sqlc.narg(base)::text IS NULL OR base = sqlc.narg(base)::text)
ORDER BY base, quote, CASE source WHEN 'manual' THEN 0 WHEN 'tcmb' THEN 1 ELSE 2 END;

-- name: LatestExchangeRateDate :one
SELECT COALESCE(MAX(rate_date), CURRENT_DATE)::date AS rate_date
FROM exchange_rates
WHERE rate_date <= sqlc.arg(on_date);
