package wellbeing

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	platformdb "nexus/internal/platform/db"
)

func TestLegacyCountersBecomeEventsAndAreRemoved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := platformdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	steps := Migrations()
	if _, _, err := platformdb.Migrate(db, path, steps[:1], time.Now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO wellbeing_settings(key,value) VALUES('shown:2025-01-02','2'),('done:2025-01-02','1'),('enabled','true')`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := platformdb.Migrate(db, path, steps, time.Now); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2025, 1, 2, 12, 0, 0, 0, time.Local).Unix()
	var shown, done int
	if err := db.QueryRow(`SELECT COUNT(*) FROM wellbeing_events WHERE at=? AND event='shown'`, at).Scan(&shown); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM wellbeing_events WHERE at=? AND event='done'`, at).Scan(&done); err != nil {
		t.Fatal(err)
	}
	var old int
	if err := db.QueryRow(`SELECT COUNT(*) FROM wellbeing_settings WHERE key LIKE '%:%'`).Scan(&old); err != nil {
		t.Fatal(err)
	}
	if shown != 2 || done != 1 || old != 0 {
		t.Fatalf("shown=%d done=%d legacy keys=%d", shown, done, old)
	}
}

func TestConcurrentActionsUseSeparateSQLiteHandles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nexus.db")
	setup, err := platformdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := platformdb.Migrate(setup, path, Migrations(), time.Now); err != nil {
		t.Fatal(err)
	}
	if err := setup.Close(); err != nil {
		t.Fatal(err)
	}

	const each = 5
	start := make(chan struct{})
	errs := make(chan error, each*2)
	var wg sync.WaitGroup
	for i := 0; i < each*2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			db, err := platformdb.Open(path)
			if err != nil {
				errs <- err
				return
			}
			defer db.Close()
			<-start
			s := NewService(NewSQLite(db), &workState{active: true}, time.Now)
			now := time.Now()
			if i%2 == 0 {
				err = s.Skip(now)
			} else {
				err = s.Snooze(now, time.Minute)
			}
			if err != nil {
				errs <- fmt.Errorf("action %d: %w", i, err)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("action: %v", err)
	}
	db, err := platformdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var total, skipped, snoozed int
	if err := db.QueryRow(`SELECT COUNT(*), SUM(event='skipped'), SUM(event='snoozed') FROM wellbeing_events`).Scan(&total, &skipped, &snoozed); err != nil {
		t.Fatal(err)
	}
	if total != 10 || skipped != 5 || snoozed != 5 {
		t.Fatalf("events=%d skipped=%d snoozed=%d", total, skipped, snoozed)
	}
	st, err := NewService(NewSQLite(db), &workState{active: true}, time.Now).Status(time.Now())
	if err != nil || st.Counters.Snoozed != 5 {
		t.Fatalf("snoozed counter=%d err=%v", st.Counters.Snoozed, err)
	}
}

func TestSQLiteMigrationPersistsSettingsAndCounters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nexus.db")
	db, e := platformdb.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if _, _, e = platformdb.Migrate(db, path, Migrations(), time.Now); e != nil {
		t.Fatal(e)
	}
	repo := NewSQLite(db)
	w := &workState{active: true}
	s := NewService(repo, w, time.Now)
	cfg := DefaultSettings()
	cfg.Every = 20 * time.Minute
	if e = s.UpdateSettings(cfg); e != nil {
		t.Fatal(e)
	}
	now := time.Now()
	if e = s.MarkShown(now); e != nil {
		t.Fatal(e)
	}
	if e = s.Skip(now); e != nil {
		t.Fatal(e)
	}
	if e = s.Done(now); e != nil {
		t.Fatal(e)
	}
	if e = s.Snooze(now, 10*time.Minute); e != nil {
		t.Fatal(e)
	}
	again := NewService(repo, w, time.Now)
	st, e := again.Status(now)
	if e != nil || st.Settings.Every != 20*time.Minute || st.Counters != (Counters{Shown: 1, Done: 1, Skipped: 1, Snoozed: 1}) {
		t.Fatalf("status=%+v err=%v", st, e)
	}
	var events int
	if e = db.QueryRow(`SELECT COUNT(*) FROM wellbeing_events WHERE at=?`, now.Unix()).Scan(&events); e != nil || events != 4 {
		t.Fatalf("events=%d err=%v", events, e)
	}
	if e = again.DND(now, time.Hour); e != nil {
		t.Fatal(e)
	}
	if e = again.ClearDND(); e != nil {
		t.Fatal(e)
	}
	st, e = again.Status(now)
	if e != nil || st.DNDUntil != nil {
		t.Fatalf("DND not cleared: %+v %v", st, e)
	}
}

func newTestSQLite(t *testing.T) *SQLite {
	t.Helper()
	path := filepath.Join(t.TempDir(), "t.db")
	db, err := platformdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, _, err := platformdb.Migrate(db, path, Migrations(), time.Now); err != nil {
		t.Fatal(err)
	}
	return NewSQLite(db)
}

func TestDueFiresAfterInactivityWithRealRepo(t *testing.T) {
	repo := newTestSQLite(t)
	w := &workState{active: true}
	s := NewService(repo, w, nil)
	now := time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)
	_, _ = s.Due(now)
	w.active = false
	_, _ = s.Due(now.Add(time.Minute))
	w.active = true
	resume := now.Add(10 * time.Minute)
	_, _ = s.Due(resume)
	if v, _ := repo.Load(); v["inactive_since"] != "" {
		t.Fatalf("inactive_since persisted after activity: %q", v["inactive_since"])
	}
	// Tras volver a la actividad, un tramo continuo de cfg.Every debe disparar el aviso.
	if _, ok := s.Due(resume.Add(30 * time.Minute)); !ok {
		t.Fatal("Due never fires after an inactivity period")
	}
}

func TestDeleteRemovesKeyAndPartialSavePreservesOthers(t *testing.T) {
	repo := newTestSQLite(t)
	if err := repo.Save(map[string]string{"a": "1", "b": "2"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(map[string]string{"a": "3"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete("a"); err != nil {
		t.Fatal(err)
	}
	v, _ := repo.Load()
	if _, ok := v["a"]; ok || v["b"] != "2" {
		t.Fatalf("unexpected values: %v", v)
	}
}
