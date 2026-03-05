-- Idempotency keys: deduplicate PostTransaction requests. key is client-provided UUID.
-- error_code '' = success; non-empty = domain error code for replay.
-- error_detail holds account id for account_not_found, or empty.
CREATE TABLE idempotency_keys (
    key TEXT PRIMARY KEY,
    error_code TEXT NOT NULL DEFAULT '',
    error_detail TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
