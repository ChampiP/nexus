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
	migrations := []Migration{{Module: "a", Version: 1, Up: createTable("t1")}, {Module: "b", Version: 1, Up: createTable("t2")}, {Module: "a", Version: 2, Up: createTable("t3")}}
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
	applied, backup, err := Migrate(db, path, []Migration{{Module: "a", Version: 1, Up: createTable("t1")}}, fixedNow)
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
		{Module: "a", Version: 1, Up: createTable("t1")},
		{Module: "a", Version: 2, Up: func(tx *sql.Tx) error {
			if _, err := tx.Exec(`CREATE TABLE t2 (id INTEGER)`); err != nil {
				return err
			}
			return boom
		}},
		{Module: "a", Version: 3, Up: createTable("t3")},
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
			_, _, err = Migrate(db, path, []Migration{{Module: "a", Version: 1, Up: createTable("migrated")}}, fixedNow)
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
	_, _, err = Migrate(db, path, []Migration{{Module: "a", Version: 2, Up: createTable("t1")}, {Module: "a", Version: 1, Up: createTable("t2")}}, fixedNow)
	if err == nil {
		t.Fatal("expected error")
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 't1'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("nothing may run on invalid input: %d, %v", n, err)
	}
}

func TestMigrateDisableForeignKeysRestoresPragmaEvenOnFailure(t *testing.T) {
	// Comprueba que las migraciones con DisableForeignKeys mantienen foreign_keys=ON después de ejecutarse,
	// incluso tras un paso que falle o provoque una infracción.
	path := filepath.Join(t.TempDir(), "n.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	getFK := func() int {
		var fk int
		if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil {
			t.Fatal(err)
		}
		return fk
	}

	if fk := getFK(); fk != 1 {
		t.Fatalf("foreign_keys inicial = %d, want 1", fk)
	}

	// Caso exitoso con DisableForeignKeys: true
	var fkInsideTx int
	migrations := []Migration{
		{
			Module:             "test",
			Version:            1,
			DisableForeignKeys: true,
			Up: func(tx *sql.Tx) error {
				if err := tx.QueryRow(`PRAGMA foreign_keys`).Scan(&fkInsideTx); err != nil {
					return err
				}
				_, err := tx.Exec(`CREATE TABLE parent (id INTEGER PRIMARY KEY); CREATE TABLE child (id INTEGER, parent_id INTEGER REFERENCES parent(id));`)
				return err
			},
		},
	}
	applied, _, err := Migrate(db, path, migrations, fixedNow)
	if err != nil || applied != 1 {
		t.Fatalf("Migrate exitoso = %d, %v", applied, err)
	}
	if fkInsideTx != 0 {
		t.Fatalf("foreign_keys dentro de tx = %d, want 0", fkInsideTx)
	}
	if fk := getFK(); fk != 1 {
		t.Fatalf("foreign_keys tras éxito = %d, want 1", fk)
	}

	// Caso fallido por error en Up
	boom := errors.New("boom")
	failMigrations := []Migration{
		{
			Module:             "test",
			Version:            2,
			DisableForeignKeys: true,
			Up: func(tx *sql.Tx) error {
				return boom
			},
		},
	}
	_, _, err = Migrate(db, path, failMigrations, fixedNow)
	if !errors.Is(err, boom) {
		t.Fatalf("Migrate con error esperado, obtuvo %v", err)
	}
	if fk := getFK(); fk != 1 {
		t.Fatalf("foreign_keys tras fallo en Up = %d, want 1", fk)
	}

	// Caso fallido por infracción en foreign_key_check
	fkViolationMigrations := []Migration{
		{
			Module:             "test",
			Version:            2,
			DisableForeignKeys: true,
			Up: func(tx *sql.Tx) error {
				// Inserta una fila con clave foránea rota
				_, err := tx.Exec(`INSERT INTO child(id, parent_id) VALUES (1, 999)`)
				return err
			},
		},
	}
	_, _, err = Migrate(db, path, fkViolationMigrations, fixedNow)
	if err == nil {
		t.Fatal("se esperaba error por infracción de clave foránea")
	}
	if fk := getFK(); fk != 1 {
		t.Fatalf("foreign_keys tras infracción FK = %d, want 1", fk)
	}
}
