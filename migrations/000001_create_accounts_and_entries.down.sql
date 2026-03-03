-- Called by golang-migrate when rolling back migrations.
DROP TABLE IF EXISTS entries;
DROP TABLE IF EXISTS accounts;
