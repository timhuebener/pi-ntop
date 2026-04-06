package db

import (
	"database/sql"
	"errors"
	"fmt"

	migrationsfs "pi-ntop/migrations"

	"github.com/golang-migrate/migrate/v4"
	sqlitemigrate "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

func RunMigrations(database *sql.DB) (err error) {
	sourceDriver, err := iofs.New(migrationsfs.FS, ".")
	if err != nil {
		return fmt.Errorf("create migration source: %w", err)
	}
	defer func() {
		err = errors.Join(err, sourceDriver.Close())
	}()

	databaseDriver, err := sqlitemigrate.WithInstance(database, &sqlitemigrate.Config{
		DatabaseName:    "pi-ntop",
		MigrationsTable: "schema_migrations",
	})
	if err != nil {
		return fmt.Errorf("create sqlite migration driver: %w", err)
	}

	migrator, err := migrate.NewWithInstance("iofs", sourceDriver, "sqlite", databaseDriver)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}

	if err := migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}

	return nil
}
