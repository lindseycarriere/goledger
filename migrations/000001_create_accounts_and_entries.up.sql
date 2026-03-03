-- Accounts: id is business key; balance stored in micros.
CREATE TABLE accounts (
    id TEXT PRIMARY KEY,
    balance_micros BIGINT NOT NULL DEFAULT 0 CHECK (balance_micros >= 0)
);

-- Entries: immutable audit log of each debit/credit. amount_micros is signed (negative = debit, positive = credit).
CREATE TABLE entries (
    id BIGSERIAL PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id),
    amount_micros BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_entries_account_id ON entries(account_id);
