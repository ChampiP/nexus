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
	if err != nil || applied != 2 {
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

func TestMigrationNullsOrphanEntryReferences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "orphan.db")
	db, err := platformdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE entries (id INTEGER PRIMARY KEY); INSERT INTO entries(id) VALUES (7)`); err != nil {
		t.Fatal(err)
	}
	steps := Migrations()
	if _, _, err := platformdb.Migrate(db, path, steps[:1], time.Now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO countdowns(id,uid,started_at,ends_at,entry_id) VALUES
		(1,'orphan',10,20,999),(2,'valid',30,40,7)`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := platformdb.Migrate(db, path, steps, func() time.Time { return time.Now().Add(time.Minute) }); err != nil {
		t.Fatalf("Migrate with orphan entry reference: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM countdowns`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("countdowns = %d, %v; want 2 rows", count, err)
	}
	var orphanEntry, validEntry sql.NullInt64
	if err := db.QueryRow(`SELECT entry_id FROM countdowns WHERE uid='orphan'`).Scan(&orphanEntry); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT entry_id FROM countdowns WHERE uid='valid'`).Scan(&validEntry); err != nil {
		t.Fatal(err)
	}
	if orphanEntry.Valid {
		t.Fatalf("orphan entry_id = %d, want NULL", orphanEntry.Int64)
	}
	if !validEntry.Valid || validEntry.Int64 != 7 {
		t.Fatalf("valid entry_id = %v, want 7", validEntry)
	}
}

func TestIntegrityRulesAndMigrationRepair(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repair.db")
	db, err := platformdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE entries (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	steps := Migrations()
	if _, _, err := platformdb.Migrate(db, path, steps[:1], time.Now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO countdowns(id,uid,kind,started_at,ends_at) VALUES
		(1,'old','legacy',20,10),(2,'new','break',30,40)`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := platformdb.Migrate(db, path, steps, func() time.Time { return time.Now().Add(time.Minute) }); err != nil {
		t.Fatal(err)
	}
	var kind string
	var ends, finished int64
	if err := db.QueryRow(`SELECT kind,ends_at,finished_at FROM countdowns WHERE id=1`).Scan(&kind, &ends, &finished); err != nil {
		t.Fatal(err)
	}
	if kind != "break" || ends != 20 || finished != 10 {
		t.Fatalf("repaired row kind=%q ends=%d finished=%d", kind, ends, finished)
	}
	if _, err := db.Exec(`INSERT INTO countdowns(uid,kind,started_at,ends_at) VALUES('second','break',50,60)`); err == nil {
		t.Fatal("second active countdown accepted")
	}
	if _, err := db.Exec(`INSERT INTO countdowns(uid,kind,started_at,ends_at,finished_at) VALUES('bad','break',80,79,90)`); err == nil {
		t.Fatal("invalid interval accepted")
	}
}

func TestSQLiteActiveAcceptsEmptyResumeEntryIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nexus.db")
	db, err := platformdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE entries (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := platformdb.Migrate(db, path, Migrations(), time.Now); err != nil {
		t.Fatal(err)
	}
	s := NewSQLite(db)
	b, err := s.Insert(Break{Label: "x", StartedAt: 1, EndsAt: 2})
	if err != nil {
		t.Fatal(err)
	}
	for _, ids := range []string{"", " \t\n"} {
		if _, err := db.Exec(`UPDATE countdowns SET resume_entry_ids = ? WHERE id = ?`, ids, b.ID); err != nil {
			t.Fatal(err)
		}
		active, err := s.Active()
		if err != nil || active == nil || len(active.ResumeEntryIDs) != 0 {
			t.Fatalf("Active with resume_entry_ids %q = %+v, %v", ids, active, err)
		}
	}
}

func TestSQLiteDefaultsAndRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nexus.db")
	db, err := platformdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE entries (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := platformdb.Migrate(db, path, Migrations(), time.Now); err != nil {
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
