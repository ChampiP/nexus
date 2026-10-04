package wellbeing

import (
	platformdb "nexus/internal/platform/db"
	"path/filepath"
	"testing"
	"time"
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
	w := &workState{running: true}
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
	if e != nil || st.Settings.Every != 20*time.Minute || st.Counters != (Counters{Shown: 1, Done: 1, Skipped: 1}) {
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
