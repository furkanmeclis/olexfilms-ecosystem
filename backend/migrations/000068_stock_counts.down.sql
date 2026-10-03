-- Reverts TEC-206. Data loss: every stock count with its scans and lines
-- (the stock movements written by approvals stay: the ledger is
-- append-only).
DROP TABLE IF EXISTS stock_count_lines;
DROP TABLE IF EXISTS stock_count_scans;
DROP TABLE IF EXISTS stock_counts;
