package wellbeing

import (
	platformdb "nexus/internal/platform/db"
	"path/filepath"
	"testing"
	"time"
)

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
	again := NewService(repo, w, time.Now)
	st, e := again.Status(now)
	if e != nil || st.Settings.Every != 20*time.Minute || st.Counters != (Counters{Shown: 1, Skipped: 1}) {
		t.Fatalf("status=%+v err=%v", st, e)
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
