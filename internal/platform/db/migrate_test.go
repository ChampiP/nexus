package db

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

func fixedNow() time.Time { return time.Unix(1_700_000_000, 0) }

func exec(t *testing.T, db *sql.DB, query string) {
	t.Helper()
	if _, err := db.Exec(query); err != nil {
		t.Fatal(err)
	}
}

func createTable(name string) func(*sql.Tx) error {
	return func(tx *sql.Tx) error {
		_, err := tx.Exec(`CREATE TABLE ` + name + ` (id INTEGER)`)
		return err
	}
}

func TestOpenPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "n.db")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+"-wal", nil, 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for file, want := range map[string]os.FileMode{path: 0o600, path + "-wal": 0o600} {
		info, err := os.Stat(file)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != want {
			t.Errorf("%s mode = %v, want %v", file, info.Mode().Perm(), want)
		}
	}
}

func TestMigrateFreshAppliesAllWithoutBackupAndIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "n.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	migrations := []Migration{{"a", 1, createTable("t1")}, {"b", 1, createTable("t2")}, {"a", 2, createTable("t3")}}
	applied, backup, err := Migrate(db, path, migrations, fixedNow)
	if err != nil || applied != 3 || backup != "" {
		t.Fatalf("Migrate = %d, %q, %v", applied, backup, err)
	}
	applied, backup, err = Migrate(db, path, migrations, fixedNow)
	if err != nil || applied != 0 || backup != "" {
		t.Fatalf("second Migrate = %d, %q, %v", applied, backup, err)
	}
}

func TestMigrateBacksUpExistingDatabaseOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "n.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	exec(t, db, `CREATE TABLE legacy (id INTEGER); INSERT INTO legacy VALUES (42)`)
	applied, backup, err := Migrate(db, path, []Migration{{"a", 1, createTable("t1")}}, fixedNow)
	if err != nil || applied != 1 {
		t.Fatalf("Migrate = %d, %q, %v", applied, backup, err)
	}
	if want := path + ".bak-" + strconv.FormatInt(fixedNow().UnixNano(), 10) + "-" + strconv.Itoa(os.Getpid()); backup != want {
		t.Fatalf("backup = %q, want %q", backup, want)
	}
	info, err := os.Stat(backup)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("backup stat = %v, %v", info, err)
	}
	copyDB, err := sql.Open("sqlite", backup)
	if err != nil {
		t.Fatal(err)
	}
	defer copyDB.Close()
	var id int
	if err := copyDB.QueryRow(`SELECT id FROM legacy`).Scan(&id); err != nil || id != 42 {
		t.Fatalf("backup row = %d, %v", id, err)
	}
	var n int
	if err := copyDB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 't1'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("backup must predate migrations: %d, %v", n, err)
	}
}

func TestMigrateFailureRollsBackAndStops(t *testing.T) {
	path := filepath.Join(t.TempDir(), "n.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	boom := errors.New("boom")
	migrations := []Migration{
		{"a", 1, createTable("t1")},
		{"a", 2, func(tx *sql.Tx) error {
			if _, err := tx.Exec(`CREATE TABLE t2 (id INTEGER)`); err != nil {
				return err
			}
			return boom
		}},
		{"a", 3, createTable("t3")},
	}
	applied, _, err := Migrate(db, path, migrations, fixedNow)
	if !errors.Is(err, boom) || applied != 1 {
		t.Fatalf("Migrate = %d, %v", applied, err)
	}
	var tables, rows int
	exec(t, db, `SELECT 1`)
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name IN ('t2','t3')`).Scan(&tables); err != nil || tables != 0 {
		t.Fatalf("leftover tables = %d, %v", tables, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("schema_migrations rows = %d, %v", rows, err)
	}
}

func TestMigrateConcurrentLegacyDatabaseBacksUpOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "n.db")
	seed, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	exec(t, seed, `CREATE TABLE legacy (id INTEGER); INSERT INTO legacy VALUES (42)`)
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}

	const workers = 5
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			db, err := Open(path)
			if err != nil {
				errs <- err
				return
			}
			defer db.Close()
			<-start
			_, _, err = Migrate(db, path, []Migration{{"a", 1, createTable("migrated")}}, fixedNow)
			if err != nil {
				errs <- err
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("Migrate: %v", err)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE module='a' AND version=1`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("migration rows=%d err=%v", rows, err)
	}
	backups, err := filepath.Glob(path + ".bak-*")
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups=%v err=%v", backups, err)
	}
}

func TestMigrateRejectsNonIncreasingVersions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "n.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, _, err = Migrate(db, path, []Migration{{"a", 2, createTable("t1")}, {"a", 1, createTable("t2")}}, fixedNow)
	if err == nil {
		t.Fatal("expected error")
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 't1'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("nothing may run on invalid input: %d, %v", n, err)
	}
}
