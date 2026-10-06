package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/aligh5331/personal-budget-manger/internal/storage/sqlite"
)

func openRaw(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sqlite.OpenRaw(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func applied(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
	}
	return out
}

func TestMigrateAppliesInOrderOnce(t *testing.T) {
	db := openRaw(t)
	fsys := fstest.MapFS{
		"0002_b.sql": {Data: []byte(`INSERT INTO a (id) VALUES (1);`)},
		"0001_a.sql": {Data: []byte(`CREATE TABLE a (id INTEGER PRIMARY KEY);`)},
		"README.txt": {Data: []byte(`not a migration`)},
	}
	ctx := context.Background()
	if err := sqlite.Migrate(ctx, db, fsys); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.Migrate(ctx, db, fsys); err != nil {
		t.Fatalf("second run: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM a`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("rows in a = %d, want 1 (migration ran twice?)", n)
	}
	got := applied(t, db)
	if len(got) != 2 || got[0] != "0001_a" || got[1] != "0002_b" {
		t.Fatalf("applied = %v", got)
	}
}

func TestMigrateAppliesLateLowerNumber(t *testing.T) {
	// Parallel tickets number migrations by issue number, so a lower number
	// can land after a higher one is already applied.
	db := openRaw(t)
	ctx := context.Background()
	if err := sqlite.Migrate(ctx, db, fstest.MapFS{
		"0040_x.sql": {Data: []byte(`CREATE TABLE x (id INTEGER PRIMARY KEY);`)},
	}); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.Migrate(ctx, db, fstest.MapFS{
		"0035_y.sql": {Data: []byte(`CREATE TABLE y (id INTEGER PRIMARY KEY);`)},
		"0040_x.sql": {Data: []byte(`CREATE TABLE x (id INTEGER PRIMARY KEY);`)},
	}); err != nil {
		t.Fatal(err)
	}
	if got := applied(t, db); len(got) != 2 {
		t.Fatalf("applied = %v", got)
	}
}

func TestMigrateRollsBackFailedMigration(t *testing.T) {
	db := openRaw(t)
	fsys := fstest.MapFS{
		"0001_bad.sql": {Data: []byte(`CREATE TABLE ok (id INTEGER PRIMARY KEY); THIS IS NOT SQL;`)},
	}
	if err := sqlite.Migrate(context.Background(), db, fsys); err == nil {
		t.Fatal("want error")
	}
	if got := applied(t, db); len(got) != 0 {
		t.Fatalf("applied = %v, want none", got)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'ok'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("table from failed migration survived")
	}
}

func TestMigrateRejectsBadName(t *testing.T) {
	db := openRaw(t)
	err := sqlite.Migrate(context.Background(), db, fstest.MapFS{"1_x.sql": {Data: []byte(`SELECT 1;`)}})
	if err == nil {
		t.Fatal("want error for badly named migration")
	}
}

func TestOpenAppliesEmbeddedMigrations(t *testing.T) {
	st, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "sub", "bot.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}
