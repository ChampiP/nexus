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
