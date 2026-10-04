package tracking

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	platformdb "nexus/internal/platform/db"
)

// openSQLite opens and migrates a database at path and returns a SQLite repository over it.
// The projects table normally belongs to catalog, which tracking must not import, so a stub stands in.
func openSQLite(path string) (*SQLite, error) {
	db, err := platformdb.Open(path)
	if err != nil {
		return nil, err
	}
	stub := platformdb.Migration{Module: "catalog", Version: 1, Up: func(tx *sql.Tx) error {
		_, err := tx.Exec(`CREATE TABLE projects (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`)
		return err
	}}
	migrations := append(Migrations()[:1:1], stub)
	migrations = append(migrations, Migrations()[1:]...)
	if _, _, err := platformdb.Migrate(db, path, migrations, time.Now); err != nil {
		db.Close()
		return nil, err
	}
	return NewSQLite(db), nil
}

func TestInsertAssignsUIDAndKindAndReadsSkipDeleted(t *testing.T) {
	s, err := openSQLite(filepath.Join(t.TempDir(), "nexus.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	entry, err := s.Insert(Entry{Title: "a", StartedAt: 1})
	if err != nil || len(entry.UID) != 26 || entry.Kind != "work" {
		t.Fatalf("Insert = %+v, %v", entry, err)
	}
	if _, err := s.db.Exec(`UPDATE entries SET deleted_at = 5 WHERE id = ?`, entry.ID); err != nil {
		t.Fatal(err)
	}
	running, _ := s.Running()
	recent, _ := s.Recent(10)
	totals, _ := s.Totals(0, 100)
	projects, _ := s.Projects(100)
	if len(running)+len(recent)+len(totals)+len(projects) != 0 {
		t.Fatalf("deleted entry visible: %v %v %v %v", running, recent, totals, projects)
	}
}

type fakeResolver map[string]int64

func (f fakeResolver) EnsureProject(name string) (int64, error) { return f[name], nil }

func TestReconcileFillsUIDAndProjectID(t *testing.T) {
	s, err := openSQLite(filepath.Join(t.TempDir(), "nexus.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	if _, err := s.db.Exec(`INSERT INTO projects(id, name) VALUES (7, 'p')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO entries(title, project, started_at) VALUES ('x', 'p', 1), ('y', '', 2)`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := s.Reconcile(fakeResolver{"p": 7}); err != nil {
			t.Fatal(err)
		}
	}
	var linked, nulls int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM entries WHERE project_id = 7 AND project = 'p'`).Scan(&linked)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM entries WHERE uid IS NULL OR (project = '' AND project_id IS NOT NULL)`).Scan(&nulls)
	if linked != 1 || nulls != 0 {
		t.Fatalf("linked = %d, nulls = %d", linked, nulls)
	}
}

func TestOpenCreatesParentAndSupportsConcurrentTimers(t *testing.T) {
	s, err := openSQLite(filepath.Join(t.TempDir(), "nested", "nexus.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	first, err := s.Insert(Entry{Title: "first", Project: "work", Description: "details", StartedAt: 1_700_000_000})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Insert(Entry{Title: "second", Project: "home", StartedAt: 1_700_000_000})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatalf("duplicate ids: %d", first.ID)
	}
	running, err := s.Running()
	if err != nil {
		t.Fatal(err)
	}
	if len(running) != 2 || running[0].Title != "first" || running[0].Project != "work" || running[0].Description != "details" {
		t.Fatalf("Running() = %+v", running)
	}
}

func TestConcurrentIndependentHandlesStartAndStop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nexus.db")
	seed, err := openSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := seed.db.Close(); err != nil {
		t.Fatal(err)
	}

	const workers = 12
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			db, err := platformdb.Open(path)
			if err != nil {
				errs <- err
				return
			}
			defer db.Close()
			repo := NewSQLite(db)
			<-start
			entry, err := repo.Insert(Entry{Title: fmt.Sprintf("task-%d", i), StartedAt: int64(i + 1)})
			if err == nil {
				err = repo.Stop(entry.ID, int64(i+100))
			}
			if err != nil {
				errs <- err
			}
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("start/stop: %v", err)
	}
	db, err := platformdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM entries WHERE ended_at IS NOT NULL`).Scan(&rows); err != nil || rows != workers {
		t.Fatalf("stopped rows=%d err=%v", rows, err)
	}
}

func TestStopRequiresRunningEntryAndStopAll(t *testing.T) {
	s, err := openSQLite(filepath.Join(t.TempDir(), "nexus.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	first, err := s.Insert(Entry{Title: "first", StartedAt: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Stop(first.ID, 2); err != nil {
		t.Fatal(err)
	}
	if err := s.Stop(first.ID, 3); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("Stop() error = %v", err)
	}
	second, _ := s.Insert(Entry{Title: "second", StartedAt: 4})
	third, _ := s.Insert(Entry{Title: "third", StartedAt: 5})
	count, err := s.StopAll(100)
	if err != nil || count != 2 {
		t.Fatalf("StopAll() = %d, %v", count, err)
	}
	for _, id := range []int64{second.ID, third.ID} {
		if err := s.Stop(id, 101); !errors.Is(err, ErrNotRunning) {
			t.Errorf("Stop(%d) error = %v", id, err)
		}
	}
	running, err := s.Running()
	if err != nil || len(running) != 0 {
		t.Errorf("Running() = %v, %v", running, err)
	}
	if count, err := s.StopAll(200); err != nil || count != 0 {
		t.Errorf("empty StopAll() = %d, %v", count, err)
	}
}

func TestRecentTotalsAndProjectUsage(t *testing.T) {
	s, err := openSQLite(filepath.Join(t.TempDir(), "nexus.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	base := time.Date(2024, time.January, 2, 12, 0, 0, 0, time.Local).Unix()
	finished, _ := s.Insert(Entry{Title: "finished", Project: "alpha", StartedAt: base})
	if err := s.Stop(finished.ID, base+7200); err != nil {
		t.Fatal(err)
	}
	_, _ = s.Insert(Entry{Title: "running", Project: "alpha", StartedAt: base + 10800})
	_, _ = s.Insert(Entry{Title: "other", Project: "beta", StartedAt: base + 10800})
	totals, err := s.Totals(base+3600, base+10800)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for _, total := range totals {
		got[total.Project] = total.Seconds
	}
	if got["alpha"] != 3600 || got["beta"] != 0 {
		t.Errorf("Totals() = %#v", got)
	}
	recent, err := s.Recent(2)
	if err != nil || len(recent) != 2 || recent[0].Title != "other" || recent[1].Title != "running" {
		t.Errorf("Recent() = %+v, %v", recent, err)
	}
	projects, err := s.Projects(base + 10800)
	if err != nil || len(projects) != 2 {
		t.Fatalf("Projects() = %+v, %v", projects, err)
	}
	usage := map[string]ProjectUsage{}
	for _, p := range projects {
		usage[p.Name] = p
	}
	if usage["alpha"].Seconds != 7200 || usage["alpha"].LastUsed != base+10800 || usage["beta"].Seconds != 0 {
		t.Errorf("Projects() = %+v", projects)
	}
}

func TestStopUnknownID(t *testing.T) {
	s, err := openSQLite(filepath.Join(t.TempDir(), "nexus.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	if err := s.Stop(99, 100); !errors.Is(err, ErrNotRunning) {
		t.Errorf("Stop(unknown) error = %v", err)
	}
}

type testAmbigResolver struct{}

func (testAmbigResolver) EnsureProject(name string) (int64, error) {
	if name == "Ambiguo" {
		return 0, ErrAmbiguousProject
	}
	if name == "Unico" {
		return 42, nil
	}
	return 0, errors.New("desconocido")
}

func TestReconcileSkipsAmbiguousNames(t *testing.T) {
	// Comprueba que Reconcile no enlaza proyectos con nombres ambiguos (deja project_id NULL)
	// y nunca re-enlaza filas que ya tienen project_id asignado.
	s, err := openSQLite(filepath.Join(t.TempDir(), "nexus.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()

	if _, err := s.db.Exec(`
INSERT INTO projects(id, name) VALUES (42, 'Unico'), (99, 'Ambiguo');
INSERT INTO entries(id, uid, title, project, project_id, started_at, kind) VALUES
    (1, 'u1', 'Tarea 1', 'Ambiguo', NULL, 1000, 'work'),
    (2, 'u2', 'Tarea 2', 'Unico', NULL, 2000, 'work'),
    (3, 'u3', 'Tarea 3', 'Ambiguo', 99, 3000, 'work');
`); err != nil {
		t.Fatal(err)
	}

	if err := s.Reconcile(testAmbigResolver{}); err != nil {
		t.Fatalf("Reconcile con nombre ambiguo falló: %v", err)
	}

	type entryRow struct {
		id        int64
		project   string
		projectID sql.NullInt64
	}
	read := func(id int64) entryRow {
		var r entryRow
		if err := s.db.QueryRow(`SELECT id, project, project_id FROM entries WHERE id = ?`, id).Scan(&r.id, &r.project, &r.projectID); err != nil {
			t.Fatal(err)
		}
		return r
	}

	e1 := read(1)
	if e1.projectID.Valid {
		t.Fatalf("entrada 1 con nombre ambiguo debía tener project_id NULL, tiene %d", e1.projectID.Int64)
	}

	e2 := read(2)
	if !e2.projectID.Valid || e2.projectID.Int64 != 42 {
		t.Fatalf("entrada 2 con nombre único debía enlazarse al id 42, tiene %v", e2.projectID)
	}

	e3 := read(3)
	if !e3.projectID.Valid || e3.projectID.Int64 != 99 {
		t.Fatalf("entrada 3 ya enlazada debía conservar id 99, tiene %v", e3.projectID)
	}
}

func TestTotalsSeparateSameNamedProjectsOfDifferentClients(t *testing.T) {
	// Comprueba que Totals y Projects agregan por project_id cuando está definido
	// y separan proyectos con el mismo nombre de diferentes clientes.
	s, err := openSQLite(filepath.Join(t.TempDir(), "nexus.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()

	if _, err := s.db.Exec(`INSERT INTO projects(id, name) VALUES (10, 'Diseno'), (20, 'Diseno');`); err != nil {
		t.Fatal(err)
	}

	base := time.Date(2025, time.January, 1, 10, 0, 0, 0, time.Local).Unix()
	// Proyecto "Diseno" en cliente 1 (id 10) -> 1000 segundos
	if _, err := s.db.Exec(`INSERT INTO entries(uid, title, project, project_id, started_at, ended_at, kind) VALUES ('u10', 't1', 'Diseno', 10, ?, ?, 'work')`, base, base+1000); err != nil {
		t.Fatal(err)
	}
	// Proyecto "Diseno" en cliente 2 (id 20) -> 2000 segundos
	if _, err := s.db.Exec(`INSERT INTO entries(uid, title, project, project_id, started_at, ended_at, kind) VALUES ('u20', 't2', 'Diseno', 20, ?, ?, 'work')`, base, base+2000); err != nil {
		t.Fatal(err)
	}
	// Proyecto sin id histórico "SinID" -> 500 segundos
	if _, err := s.db.Exec(`INSERT INTO entries(uid, title, project, project_id, started_at, ended_at, kind) VALUES ('u30', 't3', 'SinID', NULL, ?, ?, 'work')`, base, base+500); err != nil {
		t.Fatal(err)
	}

	totals, err := s.Totals(base, base+3600)
	if err != nil {
		t.Fatalf("Totals(): %v", err)
	}
	if len(totals) != 3 {
		t.Fatalf("Totals esperaba 3 totales independientes, obtuvo %d: %#v", len(totals), totals)
	}

	totalsByID := map[int64]ProjectTotal{}
	for _, tot := range totals {
		if tot.ProjectID > 0 {
			totalsByID[tot.ProjectID] = tot
		}
	}
	if t10, ok := totalsByID[10]; !ok || t10.Seconds != 1000 || t10.Project != "Diseno" {
		t.Fatalf("total id 10 incorrecto: %+v", t10)
	}
	if t20, ok := totalsByID[20]; !ok || t20.Seconds != 2000 || t20.Project != "Diseno" {
		t.Fatalf("total id 20 incorrecto: %+v", t20)
	}

	projects, err := s.Projects(base + 3600)
	if err != nil {
		t.Fatalf("Projects(): %v", err)
	}
	if len(projects) != 3 {
		t.Fatalf("Projects esperaba 3 usos independientes, obtuvo %d: %#v", len(projects), projects)
	}

	usageByID := map[int64]ProjectUsage{}
	for _, p := range projects {
		if p.ProjectID > 0 {
			usageByID[p.ProjectID] = p
		}
	}
	if u10, ok := usageByID[10]; !ok || u10.Seconds != 1000 || u10.Name != "Diseno" {
		t.Fatalf("uso id 10 incorrecto: %+v", u10)
	}
	if u20, ok := usageByID[20]; !ok || u20.Seconds != 2000 || u20.Name != "Diseno" {
		t.Fatalf("uso id 20 incorrecto: %+v", u20)
	}
}
