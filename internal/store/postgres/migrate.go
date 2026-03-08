package postgres

import (
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// RunMigrations runs migrations against the DB. dsn is the Postgres connection string,
// e.g. "postgres://user:pass@host:5432/dbname?sslmode=disable".
// Uses CWD-relative "migrations" (override with ENV var: LEDGER_MIGRATIONS_DIR).
func RunMigrations(dsn string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	dir := os.Getenv("LEDGER_MIGRATIONS_DIR")
	if dir == "" {
		dir = "migrations"
	}
	source, err := iofs.New(os.DirFS(cwd), dir)
	if err != nil {
		return err
	}
	m, err := migrate.NewWithSourceInstance("iofs", source, dsn)
	if err != nil {
		return err
	}
	defer m.Close()
	return m.Up()
}
