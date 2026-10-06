package tracking

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// newBreakTracker devuelve un Tracker sobre SQLite con un reloj controlable.
func newBreakTracker(t *testing.T, now *int64) *Tracker {
	t.Helper()
	s, err := openSQLite(filepath.Join(t.TempDir(), "nexus.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.db.Close() })
	return NewTracker(s, func() time.Time { return time.Unix(*now, 0) })
}

func TestStartKindValidation(t *testing.T) {
	now := int64(1000)
	tracker := newBreakTracker(t, &now)
	if e, err := tracker.Start(StartInput{Title: "a"}); err != nil || e.Kind != KindWork {
		t.Fatalf("default kind = %+v, %v", e, err)
	}
	if e, err := tracker.Start(StartInput{Title: "b", Kind: KindBreak}); err != nil || e.Kind != KindBreak {
		t.Fatalf("break kind = %+v, %v", e, err)
	}
	if _, err := tracker.Start(StartInput{Title: "c", Kind: "nap"}); !errors.Is(err, ErrInvalidKind) {
		t.Fatalf("invalid kind error = %v", err)
	}
}

func TestWorkViewsExcludeBreaks(t *testing.T) {
	now := int64(1000)
	tracker := newBreakTracker(t, &now)
	if _, err := tracker.Start(StartInput{Title: "work", Project: "P"}); err != nil {
		t.Fatal(err)
	}
	brk, err := tracker.Start(StartInput{Title: "Break", Project: "Rest", Kind: KindBreak})
	if err != nil {
		t.Fatal(err)
	}
	now = 1600
	running, today, recent, err := tracker.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(running) != 1 || running[0].Kind != KindWork {
		t.Fatalf("running = %+v", running)
	}
	if today != 600 {
		t.Fatalf("today = %d, want 600", today)
	}
	if len(recent) != 2 || recent[0].ID != brk.ID || recent[0].Kind != KindBreak {
		t.Fatalf("recent = %+v", recent)
	}
	totals, err := tracker.Report(time.Unix(0, 0))
	if err != nil || len(totals) != 1 || totals[0].Project != "P" {
		t.Fatalf("Report = %+v, %v", totals, err)
	}
	projects := tracker.Projects("")
	if len(projects) != 1 || projects[0].Name != "P" {
		t.Fatalf("Projects = %+v", projects)
	}
}

func TestBreakSeconds(t *testing.T) {
	now := int64(1000)
	tracker := newBreakTracker(t, &now)
	if _, err := tracker.Start(StartInput{Title: "work"}); err != nil {
		t.Fatal(err)
	}
	first, _ := tracker.Start(StartInput{Title: "Break", Kind: KindBreak})
	now = 1300
	if err := tracker.Stop(first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := tracker.Start(StartInput{Title: "Break", Kind: KindBreak}); err != nil {
		t.Fatal(err)
	}
	now = 1500 // el segundo break sigue en curso: cuenta hasta ahora
	if got, err := tracker.BreakSeconds(time.Unix(0, 0)); err != nil || got != 500 {
		t.Fatalf("BreakSeconds = %d, %v; want 500", got, err)
	}
	if got, _ := tracker.BreakSeconds(time.Unix(1200, 0)); got != 100+200 {
		t.Fatalf("BreakSeconds since 1200 = %d, want 300", got)
	}
}

func TestResumeRejectsBreakEntry(t *testing.T) {
	now := int64(1000)
	tracker := newBreakTracker(t, &now)
	brk, err := tracker.Start(StartInput{Title: "Descanso", Kind: KindBreak})
	if err != nil {
		t.Fatal(err)
	}
	if err := tracker.Stop(brk.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := tracker.Resume(brk.ID); !errors.Is(err, ErrInvalidKind) {
		t.Fatalf("Resume de break = %v; quiero ErrInvalidKind", err)
	}
	if running, err := tracker.repository.Running(); err != nil || len(running) != 0 {
		t.Fatalf("sesiones en curso tras Resume de break = %+v, %v", running, err)
	}
}

func TestResumeCopiesFields(t *testing.T) {
	now := int64(1000)
	s, err := openSQLite(filepath.Join(t.TempDir(), "nexus.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	if _, err := s.db.Exec(`INSERT INTO projects(id, name) VALUES (7, 'Proj')`); err != nil {
		t.Fatal(err)
	}
	tracker := NewTracker(s, func() time.Time { return time.Unix(now, 0) })
	src, _ := s.Insert(Entry{Title: "Task", Description: "desc", Project: "Proj", ProjectID: 7, StartedAt: 900})
	if err := tracker.Stop(src.ID); err != nil {
		t.Fatal(err)
	}
	got, err := tracker.Resume(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID == src.ID || got.Kind != KindWork || got.Title != "Task" || got.Description != "desc" || got.Project != "Proj" || got.ProjectID != 7 || got.StartedAt != 1000 || got.EndedAt != nil {
		t.Fatalf("Resume = %+v", got)
	}
	stored, err := s.Get(got.ID)
	if err != nil || stored.ProjectID != 7 || stored.EndedAt != nil {
		t.Fatalf("stored = %+v, %v", stored, err)
	}
	if err := tracker.Delete(src.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := tracker.Resume(src.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted Resume error = %v", err)
	}
}

func TestStopManyIgnoresAlreadyStopped(t *testing.T) {
	now := int64(1000)
	tracker := newBreakTracker(t, &now)
	a, _ := tracker.Start(StartInput{Title: "a"})
	b, _ := tracker.Start(StartInput{Title: "b"})
	c, _ := tracker.Start(StartInput{Title: "c"})
	if err := tracker.Stop(a.ID); err != nil {
		t.Fatal(err)
	}
	if err := tracker.StopMany([]int64{a.ID, b.ID}); err != nil {
		t.Fatalf("StopMany = %v", err)
	}
	running, _, _, _ := tracker.Snapshot()
	if len(running) != 1 || running[0].ID != c.ID {
		t.Fatalf("running = %+v", running)
	}
}

func TestIsRunning(t *testing.T) {
	now := int64(1000)
	tracker := newBreakTracker(t, &now)
	e, _ := tracker.Start(StartInput{Title: "a"})
	if ok, err := tracker.IsRunning(e.ID); err != nil || !ok {
		t.Fatalf("running = %v, %v", ok, err)
	}
	_ = tracker.Stop(e.ID)
	if ok, _ := tracker.IsRunning(e.ID); ok {
		t.Fatal("stopped entry reported running")
	}
	if ok, err := tracker.IsRunning(999); err != nil || ok {
		t.Fatalf("missing entry = %v, %v", ok, err)
	}
}

func TestBreaksSinceReturnsOnlyLiveBreaksFromThatTime(t *testing.T) {
	now := int64(1000)
	tracker := newBreakTracker(t, &now)
	if _, err := tracker.Start(StartInput{Title: "work"}); err != nil {
		t.Fatal(err)
	}
	old, err := tracker.Start(StartInput{Title: "Viejo", Kind: KindBreak})
	if err != nil {
		t.Fatal(err)
	}
	now = 1100
	if err := tracker.Stop(old.ID); err != nil {
		t.Fatal(err)
	}
	now = 2000
	first, err := tracker.Start(StartInput{Title: "Uno", Kind: KindBreak})
	if err != nil {
		t.Fatal(err)
	}
	now = 2100
	if err := tracker.Stop(first.ID); err != nil {
		t.Fatal(err)
	}
	deleted, err := tracker.Start(StartInput{Title: "Borrado", Kind: KindBreak})
	if err != nil {
		t.Fatal(err)
	}
	if err := tracker.Delete(deleted.ID); err != nil {
		t.Fatal(err)
	}
	now = 2200
	second, err := tracker.Start(StartInput{Title: "Dos", Kind: KindBreak})
	if err != nil {
		t.Fatal(err)
	}
	got, err := tracker.BreaksSince(time.Unix(1500, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != first.ID || got[1].ID != second.ID {
		t.Fatalf("BreaksSince = %+v, want [Uno Dos] en orden de inicio", got)
	}
	if got[0].EndedAt == nil || got[1].EndedAt != nil {
		t.Fatalf("EndedAt = %v / %v", got[0].EndedAt, got[1].EndedAt)
	}
}

func TestGetReturnsEntryOrNotFound(t *testing.T) {
	now := int64(1000)
	tracker := newBreakTracker(t, &now)
	entry, err := tracker.Start(StartInput{Title: "work"})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := tracker.Get(entry.ID); err != nil || got.Title != "work" {
		t.Fatalf("Get = %+v, %v", got, err)
	}
	if _, err := tracker.Get(999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get inexistente = %v", err)
	}
}

func TestStopAllLeavesBreakRunning(t *testing.T) {
	now := int64(1000)
	tracker := newBreakTracker(t, &now)
	work, err := tracker.Start(StartInput{Title: "work"})
	if err != nil {
		t.Fatal(err)
	}
	brk, err := tracker.Start(StartInput{Title: "Break", Kind: KindBreak})
	if err != nil {
		t.Fatal(err)
	}
	now = 2000
	n, err := tracker.StopAll()
	if err != nil || n != 1 {
		t.Fatalf("StopAll = %d, %v; want 1, nil", n, err)
	}
	if running, _ := tracker.IsRunning(work.ID); running {
		t.Fatal("la entrada de trabajo debía quedar detenida")
	}
	if running, _ := tracker.IsRunning(brk.ID); !running {
		t.Fatal("StopAll no debe terminar la entrada del break")
	}
}
