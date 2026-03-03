-- name: CreateAccount :exec
INSERT INTO accounts (id, balance_micros)
VALUES ($1, $2);

-- name: GetBalance :one
SELECT balance_micros FROM accounts WHERE id = $1;

-- name: GetAccountForUpdate :one
SELECT id, balance_micros FROM accounts WHERE id = $1 FOR UPDATE;

-- name: UpdateBalance :exec
UPDATE accounts SET balance_micros = $2 WHERE id = $1;

-- name: InsertEntry :one
INSERT INTO entries (account_id, amount_micros)
VALUES ($1, $2)
RETURNING id;
