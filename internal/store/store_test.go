package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestOpenCreatesParentAndSupportsConcurrentTimers(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "nested", "nexus.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer s.Close()

	started := time.Unix(1_700_000_000, 0)
	s.now = func() time.Time { return started }
	first, err := s.Start("first", "work", "details")
	if err != nil {
		t.Fatalf("Start(first) error = %v", err)
	}
	second, err := s.Start("second", "home", "")
	if err != nil {
		t.Fatalf("Start(second) error = %v", err)
	}
	if first.ID == second.ID {
		t.Fatalf("concurrent timers share id %d", first.ID)
	}
	running := s.Running()
	if len(running) != 2 {
		t.Fatalf("Running() returned %d entries, want 2", len(running))
	}
	if running[0].Title != "first" || running[0].Project != "work" || running[0].Description != "details" {
		t.Errorf("first running entry = %+v", running[0])
	}
}

func TestStopRequiresRunningEntryAndStopAll(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "nexus.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Unix(1_700_000_000, 0)
	s.now = func() time.Time { return now }
	first, err := s.Start("first", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Stop(first.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Stop(first.ID); err == nil {
		t.Fatal("Stop() on an already stopped entry succeeded")
	}
	second, err := s.Start("second", "", "")
	if err != nil {
		t.Fatal(err)
	}
	third, err := s.Start("third", "", "")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(90 * time.Second)
	count, err := s.StopAll()
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("StopAll() count = %d, want 2", count)
	}
	if err := s.Stop(second.ID); err == nil {
		t.Error("Stop() succeeded after StopAll")
	}
	if err := s.Stop(third.ID); err == nil {
		t.Error("Stop() succeeded after StopAll")
	}
	if running := s.Running(); len(running) != 0 {
		t.Errorf("Running() = %v; want empty", running)
	}
	if count, err := s.StopAll(); err != nil || count != 0 {
		t.Errorf("empty StopAll() = %d, %v; want 0, nil", count, err)
	}
}

func TestRecentAndTotalsClipRunningEntriesToSince(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "nexus.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	base := time.Date(2024, time.January, 2, 12, 0, 0, 0, time.Local)
	now := base
	s.now = func() time.Time { return now }
	finished, err := s.Start("finished", "alpha", "")
	if err != nil {
		t.Fatal(err)
	}
	now = base.Add(2 * time.Hour)
	if err := s.Stop(finished.ID); err != nil {
		t.Fatal(err)
	}
	now = base.Add(3 * time.Hour)
	if _, err := s.Start("running", "alpha", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Start("other", "beta", ""); err != nil {
		t.Fatal(err)
	}

	totals := s.Totals(base.Add(time.Hour))
	got := map[string]int64{}
	for _, total := range totals {
		got[total.Project] = total.Seconds
	}
	if got["alpha"] != 3600 || got["beta"] != 0 {
		t.Errorf("Totals() = %#v, want alpha=3600 and beta=0", got)
	}
	recent := s.Recent(2)
	if len(recent) != 2 || recent[0].Title != "other" || recent[1].Title != "running" {
		t.Errorf("Recent(2) = %+v, want newest two entries", recent)
	}
}

func TestStopUnknownID(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "nexus.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Stop(99); err == nil {
		t.Error("Stop(unknown) succeeded")
	}
}
