// Package migrate runs embedded SQL schema migrations (Goose) for SQLite and PostgreSQL.
package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"io/fs"

	"github.com/pressly/goose/v3"
	embedded "github.com/wallissonmarinho/GoAnimes/migrations"
)

// RunMigrations applies embedded goose migrations for Postgres or SQLite.
func RunMigrations(ctx context.Context, db *sql.DB, pg bool) error {
	var (
		fsys fs.FS
		err  error
		d    goose.Dialect
	)
	if pg {
		fsys, err = embedded.PostgresDir()
		d = goose.DialectPostgres
	} else {
		fsys, err = embedded.SQLiteDir()
		d = goose.DialectSQLite3
	}
	if err != nil {
		return err
	}
	p, err := goose.NewProvider(d, db, fsys)
	if err != nil {
		return fmt.Errorf("goose NewProvider: %w", err)
	}
	_, err = p.Up(ctx)
	return err
}

// NewMigrateProvider builds a goose Provider for CLI operations.
func NewMigrateProvider(db *sql.DB, pg bool) (*goose.Provider, error) {
	var (
		fsys fs.FS
		err  error
		d    goose.Dialect
	)
	if pg {
		fsys, err = embedded.PostgresDir()
		d = goose.DialectPostgres
	} else {
		fsys, err = embedded.SQLiteDir()
		d = goose.DialectSQLite3
	}
	if err != nil {
		return nil, err
	}
	return goose.NewProvider(d, db, fsys)
}

// MigrateDownSteps runs goose Down N times (N >= 1).
func MigrateDownSteps(ctx context.Context, p *goose.Provider, n int) error {
	if n < 1 {
		return fmt.Errorf("down steps must be >= 1, got %d", n)
	}
	for i := 0; i < n; i++ {
		if _, err := p.Down(ctx); err != nil {
			return err
		}
	}
	return nil
}

// PrintMigrateStatus writes human-readable migration status to w.
func PrintMigrateStatus(ctx context.Context, p *goose.Provider, w io.Writer) error {
	st, err := p.Status(ctx)
	if err != nil {
		return err
	}
	for _, row := range st {
		state := "pending"
		if row.State == goose.StateApplied {
			state = "applied"
		}
		path := row.Source.Path
		if path == "" {
			path = "(go)"
		}
		_, err := fmt.Fprintf(w, "%d\t%s\t%s\n", row.Source.Version, state, path)
		if err != nil {
			return err
		}
	}
	return nil
}
