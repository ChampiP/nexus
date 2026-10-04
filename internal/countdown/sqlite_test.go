package countdown

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	platformdb "nexus/internal/platform/db"
)

func TestMigrationKeepsLegacyDatabaseRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nexus.db")
	db, err := platformdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// Base de la fase 1: solo la tabla entries original, con datos y sin schema_migrations.
	if _, err := db.Exec(`CREATE TABLE entries (id INTEGER PRIMARY KEY, title TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', project TEXT NOT NULL DEFAULT '', started_at INTEGER NOT NULL, ended_at INTEGER NULL);
INSERT INTO entries(title, project, started_at, ended_at) VALUES ('old', 'p', 10, 20)`); err != nil {
		t.Fatal(err)
	}
	applied, _, err := platformdb.Migrate(db, path, Migrations(), time.Now)
	if err != nil || applied != 1 {
		t.Fatalf("Migrate = %d, %v", applied, err)
	}
	var title string
	var started int64
	if err := db.QueryRow(`SELECT title, started_at FROM entries`).Scan(&title, &started); err != nil || title != "old" || started != 10 {
		t.Fatalf("legacy row = %q %d, %v", title, started, err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM countdowns`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("countdowns = %d, %v", n, err)
	}
	// Reaplicar no cambia nada.
	if applied, _, err := platformdb.Migrate(db, path, Migrations(), time.Now); err != nil || applied != 0 {
		t.Fatalf("re-Migrate = %d, %v", applied, err)
	}
}

func TestSQLiteDefaultsAndRoundTrip(t *testing.T) {
	db, err := platformdb.Open(filepath.Join(t.TempDir(), "nexus.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, _, err := platformdb.Migrate(db, "", Migrations(), time.Now); err != nil {
		t.Fatal(err)
	}
	s := NewSQLite(db)
	b, err := s.Insert(Break{Label: "x", StartedAt: 1, EndsAt: 2})
	if err != nil || len(b.UID) != 26 {
		t.Fatalf("Insert = %+v, %v", b, err)
	}
	var kind, ids string
	var entry sql.NullInt64
	if err := db.QueryRow(`SELECT kind, resume_entry_ids, entry_id FROM countdowns`).Scan(&kind, &ids, &entry); err != nil || kind != "break" || ids != "[]" || entry.Valid {
		t.Fatalf("row = %q %q %v, %v", kind, ids, entry, err)
	}
}
