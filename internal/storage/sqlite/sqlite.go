// Package sqlite implements the storage interfaces on SQLite through the
// pure-Go modernc.org/sqlite driver. SQL stays in the portable subset
// (INTEGER ids, TEXT, BIGINT amounts and unix seconds, 0/1 booleans) so a
// Postgres implementation can replace it later.
//
// Each aggregate lives in its own file (settings.go, transactions.go, ...)
// and its schema in migrations/NNNN_name.sql.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"

	"github.com/aligh5331/personal-budget-manger/internal/storage"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrations returns the embedded migration files.
func Migrations() fs.FS {
	sub, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		panic(err)
	}
	return sub
}

// Store implements every storage interface.
type Store struct {
	db *sql.DB
}

var _ storage.Settings = (*Store)(nil)

// OpenRaw opens (creating if needed) the SQLite file at path with the bot's
// pragmas, without running migrations.
func OpenRaw(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "synchronous(NORMAL)")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	return db, nil
}

// Open opens the database at path and applies pending migrations.
func Open(ctx context.Context, path string) (*Store, error) {
	db, err := OpenRaw(path)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	if err := Migrate(ctx, db, Migrations()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// Ping checks the database answers.
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// DB exposes the handle for maintenance jobs such as VACUUM INTO backups.
func (s *Store) DB() *sql.DB { return s.db }
