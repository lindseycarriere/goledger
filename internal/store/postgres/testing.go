//go:build integration

package postgres

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// runMigrations runs migrations from the repo migrations/ directory against the given DSN.
func runMigrations(dsn string) error {
	_, filename, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(filename), "..", "..", "..")
	migrationsFS := os.DirFS(repoRoot)
	source, err := iofs.New(migrationsFS, "migrations")
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
