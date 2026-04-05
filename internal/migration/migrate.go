// Package migration применяет миграции базы данных с помощью golang-migrate.
package migration

import (
	"embed"
	"errors"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // драйвер pgx/v5 — требует схему pgx5://
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/wb-go/wbf/logger"
)

//go:embed sql/*.sql
var migrationsFS embed.FS

// Run применяет все ожидающие миграции из встроенной директории sql/.
// Логирует результат: применены ли миграции или уже актуальны.
func Run(dsn string, log logger.Logger) error {
	src, err := iofs.New(migrationsFS, "sql")
	if err != nil {
		return fmt.Errorf("migration: create iofs source: %w", err)
	}

	// Драйвер pgx/v5 в golang-migrate зарегистрирован под схемой "pgx5".
	// Принимаем стандартный DSN postgres:// и преобразуем его внутри.
	migrateDSN := strings.ReplaceAll(dsn, "postgres://", "pgx5://")
	m, err := migrate.NewWithSourceInstance("iofs", src, migrateDSN)
	if err != nil {
		return fmt.Errorf("migration: create migrator: %w", err)
	}
	defer func() {
		srcErr, dbErr := m.Close()
		if srcErr != nil {
			log.Error("migration: close source", "error", srcErr)
		}

		if dbErr != nil {
			log.Error("migration: close db", "error", dbErr)
		}
	}()

	if err = m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			log.Info("migration: already up to date")

			return nil
		}

		return fmt.Errorf("migration: apply: %w", err)
	}

	log.Info("migration: migrations applied successfully")

	return nil
}
