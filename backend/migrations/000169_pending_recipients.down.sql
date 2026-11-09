-- Drops the waiting pool rows (data loss: kind/state/note of the
-- recipients); the recipient organizations stay (status 'pending', no
-- members).
DROP TABLE IF EXISTS pending_recipients;
