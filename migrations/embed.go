package migrations

import (
	"embed"
	"io/fs"
)

//go:embed postgres/*.sql
var postgres embed.FS

//go:embed sqlite/*.sql
var sqlite embed.FS

// PostgresDir returns an fs.FS rooted at the postgres migration SQL files.
func PostgresDir() (fs.FS, error) {
	return fs.Sub(postgres, "postgres")
}

// SQLiteDir returns an fs.FS rooted at the sqlite migration SQL files.
func SQLiteDir() (fs.FS, error) {
	return fs.Sub(sqlite, "sqlite")
}
