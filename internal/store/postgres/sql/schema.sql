-- Matches migrations/000001. accounts and entries for double-entry ledger.
CREATE TABLE accounts (
    id TEXT PRIMARY KEY,
    balance_micros BIGINT NOT NULL DEFAULT 0 CHECK (balance_micros >= 0)
);

CREATE TABLE entries (
    id BIGSERIAL PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id),
    amount_micros BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_entries_account_id ON entries(account_id);
