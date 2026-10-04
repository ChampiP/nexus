package tracking

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	platformdb "nexus/internal/platform/db"
)

// openSQLite opens a database at path and returns a SQLite repository over it.
func openSQLite(path string) (*SQLite, error) {
	db, err := platformdb.Open(path)
	if err != nil {
		return nil, err
	}
	s, err := NewSQLite(db)
	if err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
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
