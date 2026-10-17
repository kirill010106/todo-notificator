// Package migrations provides the explicit release step and startup schema gate.
package migrations

import (
	"database/sql"
	"fmt"
	app "github.com/kirill010106/todo-notificator"
	"github.com/pressly/goose/v3"
	"math"
)

func Run(db *sql.DB, apply bool) error {
	goose.SetBaseFS(app.MigrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	files, err := goose.CollectMigrations("migrations", 0, math.MaxInt64)
	if err != nil {
		return err
	}
	if apply {
		// Migration 00021 checks copied legacy reminder counts atomically.
		if err := goose.Up(db, "migrations"); err != nil {
			return err
		}
	}
	version, err := goose.GetDBVersion(db)
	if err != nil {
		return err
	}
	latest := files[len(files)-1].Version
	if version != latest {
		return fmt.Errorf("database schema %d differs from required %d; run the backed-up release migration before startup", version, latest)
	}
	return nil
}
