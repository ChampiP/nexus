package countdown

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	platformdb "nexus/internal/platform/db"
)

// fakeTimers registra las llamadas y simula el estado de los timers.
type fakeTimers struct {
	running  map[int64]bool
	stopped  [][]int64
	started  []int64 // ids copiados con StartLike
	nextID   int64
	breakErr error
}

func newFakeTimers(running ...int64) *fakeTimers {
	f := &fakeTimers{running: map[int64]bool{}, nextID: 100}
	for _, id := range running {
		f.running[id] = true
	}
	return f
}

func (f *fakeTimers) StopMany(ids []int64) error {
	f.stopped = append(f.stopped, ids)
	for _, id := range ids {
		f.running[id] = false
	}
	return nil
}
func (f *fakeTimers) StartBreak(label string) (int64, error) {
	if f.breakErr != nil {
		return 0, f.breakErr
	}
	f.nextID++
	f.running[f.nextID] = true
	return f.nextID, nil
}
func (f *fakeTimers) Stop(id int64) error            { f.running[id] = false; return nil }
func (f *fakeTimers) StartLike(id int64) error       { f.started = append(f.started, id); return nil }
func (f *fakeTimers) Running(id int64) (bool, error) { return f.running[id], nil }

// failingRepo hace fallar Insert para probar el rollback.
type failingRepo struct{ Repository }

func (failingRepo) Insert(Break) (Break, error) { return Break{}, errors.New("disk full") }

func newTestService(t *testing.T, timers Timers, now *int64) *Service {
	t.Helper()
	db, err := platformdb.Open(filepath.Join(t.TempDir(), "nexus.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, _, err := platformdb.Migrate(db, "", Migrations(), time.Now); err != nil {
		t.Fatal(err)
	}
	return NewService(NewSQLite(db), timers, func() time.Time { return time.Unix(*now, 0) })
}

func TestStartBreakStopsChosenTimersAndRemembersThem(t *testing.T) {
	now := int64(1000)
	timers := newFakeTimers(1, 2, 3)
	s := newTestService(t, timers, &now)
	b, err := s.StartBreak(30*time.Minute, "", []int64{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	if b.Label != "Break" || b.EndsAt != 1000+1800 || b.EntryID != 101 || !reflect.DeepEqual(b.ResumeEntryIDs, []int64{1, 2}) {
		t.Fatalf("break = %+v", b)
	}
	if timers.running[1] || timers.running[2] || !timers.running[3] {
		t.Fatalf("running = %v", timers.running)
	}
	active, err := s.Active()
	if err != nil || active == nil || active.ID != b.ID || !reflect.DeepEqual(active.ResumeEntryIDs, []int64{1, 2}) {
		t.Fatalf("Active = %+v, %v", active, err)
	}
}

func TestStartBreakSingleActiveAndDurationLimits(t *testing.T) {
	now := int64(1000)
	timers := newFakeTimers()
	s := newTestService(t, timers, &now)
	for _, d := range []time.Duration{0, 59 * time.Second, 8*time.Hour + time.Second} {
		if _, err := s.StartBreak(d, "x", nil); !errors.Is(err, ErrInvalidDuration) {
			t.Fatalf("duration %v error = %v", d, err)
		}
	}
	if _, err := s.StartBreak(time.Minute, "Café", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartBreak(time.Hour, "otro", nil); !errors.Is(err, ErrBreakActive) {
		t.Fatalf("second break error = %v", err)
	}
	if b, _ := s.Active(); b.Label != "Café" {
		t.Fatalf("label = %q", b.Label)
	}
	if _, err := s.StartBreak(8*time.Hour, "x", nil); !errors.Is(err, ErrBreakActive) {
		t.Fatal("8h must be a valid duration, only blocked by the active break")
	}
}

func TestStartBreakRollsBackBreakEntryWhenInsertFails(t *testing.T) {
	now := int64(1000)
	timers := newFakeTimers(1)
	s := newTestService(t, timers, &now)
	s.repo = failingRepo{s.repo}
	if _, err := s.StartBreak(time.Hour, "", []int64{1}); err == nil {
		t.Fatal("expected error")
	}
	if timers.running[101] {
		t.Fatal("orphan break entry still running")
	}
}

func TestStartBreakEntryFailureLeavesNoCountdown(t *testing.T) {
	now := int64(1000)
	timers := newFakeTimers(1)
	timers.breakErr = errors.New("boom")
	s := newTestService(t, timers, &now)
	if _, err := s.StartBreak(time.Hour, "", []int64{1}); err == nil {
		t.Fatal("expected error")
	}
	if b, _ := s.Active(); b != nil {
		t.Fatalf("countdown left behind: %+v", b)
	}
}

func TestRemainingAndOverdue(t *testing.T) {
	b := Break{EndsAt: 1000}
	at := func(sec int64) time.Time { return time.Unix(sec, 0) }
	if b.Remaining(at(940)) != time.Minute || b.Overdue(at(940)) != 0 {
		t.Fatal("before deadline")
	}
	if b.Remaining(at(1000)) != 0 || b.Overdue(at(1000)) != 0 {
		t.Fatal("at deadline")
	}
	if b.Remaining(at(1720)) != 0 || b.Overdue(at(1720)) != 12*time.Minute {
		t.Fatal("after deadline")
	}
}

func TestExtendFromFutureAndPastDeadline(t *testing.T) {
	now := int64(1000)
	s := newTestService(t, newFakeTimers(), &now)
	if err := s.Extend(time.Minute); !errors.Is(err, ErrNoActiveBreak) {
		t.Fatalf("Extend without break = %v", err)
	}
	if _, err := s.StartBreak(10*time.Minute, "", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Extend(10 * time.Minute); err != nil {
		t.Fatal(err)
	}
	if b, _ := s.Active(); b.EndsAt != 1000+1200 {
		t.Fatalf("future extend ends_at = %d", b.EndsAt)
	}
	now = 5000 // vencido
	b, _ := s.Active()
	if err := s.MarkNotified(b.ID, time.Unix(now, 0)); err != nil {
		t.Fatal(err)
	}
	if err := s.Extend(10 * time.Minute); err != nil {
		t.Fatal(err)
	}
	b, _ = s.Active()
	if b.EndsAt != 5600 || b.NotifiedAt != nil {
		t.Fatalf("past extend = %+v", b)
	}
}

func TestEndWithAndWithoutResume(t *testing.T) {
	now := int64(1000)
	timers := newFakeTimers(1, 2)
	s := newTestService(t, timers, &now)
	if _, err := s.End(true); !errors.Is(err, ErrNoActiveBreak) {
		t.Fatalf("End without break = %v", err)
	}
	if _, err := s.StartBreak(time.Hour, "", []int64{1, 2}); err != nil {
		t.Fatal(err)
	}
	timers.running[2] = true // el usuario ya retomó el 2 por su cuenta
	n, err := s.End(true)
	if err != nil || n != 1 || !reflect.DeepEqual(timers.started, []int64{1}) {
		t.Fatalf("End(true) = %d, %v, started %v", n, err, timers.started)
	}
	if timers.running[101] {
		t.Fatal("break entry still running")
	}
	if b, _ := s.Active(); b != nil {
		t.Fatalf("break still active: %+v", b)
	}

	timers.started = nil
	if _, err := s.StartBreak(time.Hour, "", []int64{1}); err != nil {
		t.Fatal(err)
	}
	if n, err := s.End(false); err != nil || n != 0 || len(timers.started) != 0 {
		t.Fatalf("End(false) = %d, %v, started %v", n, err, timers.started)
	}
}

func TestDueNotificationCadence(t *testing.T) {
	now := int64(1000)
	s := newTestService(t, newFakeTimers(), &now)
	if _, ok := s.DueNotification(time.Unix(now, 0)); ok {
		t.Fatal("due without break")
	}
	b, _ := s.StartBreak(10*time.Minute, "", nil) // vence en 1600
	at := func(sec int64) time.Time { return time.Unix(sec, 0) }
	if _, ok := s.DueNotification(at(1599)); ok {
		t.Fatal("due before deadline")
	}
	if got, ok := s.DueNotification(at(1600)); !ok || got.ID != b.ID {
		t.Fatal("first overdue notification missing")
	}
	if err := s.MarkNotified(b.ID, at(1600)); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.DueNotification(at(1600 + 599)); ok {
		t.Fatal("renotified before 10 minutes")
	}
	if _, ok := s.DueNotification(at(1600 + 600)); !ok {
		t.Fatal("not renotified after 10 minutes")
	}
}
