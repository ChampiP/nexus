package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"testing"
	"time"

	"nexus/internal/catalog"
	platformdb "nexus/internal/platform/db"
	"nexus/internal/tracking"
)

const legacySchema = `
CREATE TABLE entries (
    id INTEGER PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    project TEXT NOT NULL DEFAULT '',
    started_at INTEGER NOT NULL,
    ended_at INTEGER NULL
);
CREATE INDEX entries_ended_at_idx ON entries(ended_at);
CREATE INDEX entries_project_idx ON entries(project);
INSERT INTO entries(title, description, project, started_at, ended_at) VALUES ('stopped', 'd1', 'nexus', 100, 200);
INSERT INTO entries(title, description, project, started_at, ended_at) VALUES ('bare', '', '', 300, 400);
INSERT INTO entries(title, description, project, started_at, ended_at) VALUES ('running', 'long one', 'inspira salud (emana)', 500, NULL);
`

func legacyDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nexus.db")
	db, err := platformdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(legacySchema); err != nil {
		t.Fatal(err)
	}
	return db, path
}

func upgrade(t *testing.T, db *sql.DB, path string) (int, string) {
	t.Helper()
	applied, backup, err := migrateAndReconcile(db, path, func() time.Time { return time.Unix(1_800_000_000, 0) })
	if err != nil {
		t.Fatal(err)
	}
	return applied, backup
}

func count(t *testing.T, db *sql.DB, query string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestFreshDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "nexus.db")
	db, err := platformdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	applied, backup := upgrade(t, db, path)
	if applied != 6 || backup != "" {
		t.Fatalf("applied = %d, backup = %q", applied, backup)
	}
	for file, want := range map[string]os.FileMode{filepath.Dir(path): 0o700, path: 0o600} {
		info, err := os.Stat(file)
		if err != nil || info.Mode().Perm() != want {
			t.Errorf("%s: %v, %v (want %v)", file, info, err, want)
		}
	}
}

func TestLegacyDatabaseIsAdoptedWithoutLosingData(t *testing.T) {
	db, path := legacyDB(t)
	applied, backup := upgrade(t, db, path)
	if applied != 6 || backup == "" {
		t.Fatalf("applied = %d, backup = %q", applied, backup)
	}
	old, err := sql.Open("sqlite", backup)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	if n := count(t, old, `SELECT COUNT(*) FROM entries WHERE (title, project) IN (('stopped','nexus'),('bare',''),('running','inspira salud (emana)'))`); n != 3 {
		t.Fatalf("backup has %d old rows", n)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM entries WHERE uid IS NULL OR uid = '' OR kind <> 'work' OR deleted_at IS NOT NULL`); n != 0 {
		t.Fatalf("%d rows lack uid/kind", n)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM entries WHERE (project <> '') <> (project_id IS NOT NULL)`); n != 0 {
		t.Fatalf("%d rows with wrong project_id", n)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM entries e JOIN projects p ON p.id = e.project_id WHERE p.name = e.project`); n != 2 {
		t.Fatalf("%d rows linked to a matching project", n)
	}
	var names []string
	rows, err := db.Query(`SELECT name FROM projects`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	rows.Close()
	sort.Strings(names)
	if want := []string{"inspira salud (emana)", "nexus"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("projects = %v", names)
	}
	var title, description string
	var started int64
	var ended sql.NullInt64
	if err := db.QueryRow(`SELECT title, description, started_at, ended_at FROM entries WHERE id = 3`).Scan(&title, &description, &started, &ended); err != nil {
		t.Fatal(err)
	}
	if title != "running" || description != "long one" || started != 500 || ended.Valid {
		t.Fatalf("running entry changed: %q %q %d %v", title, description, started, ended)
	}
}

func TestIdempotent(t *testing.T) {
	db, path := legacyDB(t)
	upgrade(t, db, path)
	snapshot := func() string {
		var out string
		rows, err := db.Query(`SELECT id, uid, project_id FROM entries ORDER BY id`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			var uid string
			var pid sql.NullInt64
			if err := rows.Scan(&id, &uid, &pid); err != nil {
				t.Fatal(err)
			}
			out += uid + "|" + string(rune('0'+pid.Int64)) + ";"
		}
		return out
	}
	before := snapshot()
	applied, backup := upgrade(t, db, path)
	if applied != 0 || backup != "" || snapshot() != before {
		t.Fatalf("second run: applied=%d backup=%q changed=%v", applied, backup, snapshot() != before)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM projects`); n != 2 {
		t.Fatalf("projects = %d", n)
	}
}

func TestEntryFromOldBinaryIsReconciled(t *testing.T) {
	db, path := legacyDB(t)
	upgrade(t, db, path)
	if _, err := db.Exec(`INSERT INTO entries(title, description, project, started_at) VALUES ('old', '', 'NEXUS', 900)`); err != nil {
		t.Fatal(err)
	}
	upgrade(t, db, path)
	repository := tracking.NewSQLite(db)
	running, err := repository.Running()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range running {
		if e.Title == "old" {
			found = e.UID != "" && e.Kind == "work"
		}
	}
	if !found {
		t.Fatalf("Running() = %+v", running)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM entries WHERE title = 'old' AND project_id = (SELECT id FROM projects WHERE name = 'nexus')`); n != 1 {
		t.Fatal("old entry not linked case-insensitively")
	}
	if n := count(t, db, `SELECT COUNT(*) FROM projects`); n != 2 {
		t.Fatalf("projects = %d", n)
	}
}

func wiredServices(t *testing.T, now *int64) (*tracking.Tracker, *catalog.Service, *sql.DB) {
	t.Helper()
	db, path := legacyDB(t)
	upgrade(t, db, path)
	tracker, service, _ := wire(db, func() time.Time { return time.Unix(*now, 0) })
	return tracker, service, db
}

func TestMergeKeepsHoursAttributedToTarget(t *testing.T) {
	now := int64(1000)
	tracker, service, db := wiredServices(t, &now)
	from := count(t, db, `SELECT id FROM projects WHERE name = 'nexus'`)
	into, err := service.CreateProject("Destino", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.MergeProjects(int64(from), into.ID); err != nil {
		t.Fatal(err)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM entries WHERE title = 'stopped' AND project = 'Destino' AND project_id = `+strconv.FormatInt(into.ID, 10)); n != 1 {
		t.Fatal("entry not moved to target")
	}
	totals, err := tracker.Report(time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	for _, total := range totals {
		if total.Project == "Destino" && total.Seconds == 100 {
			return
		}
	}
	t.Fatalf("hours not attributed to target: %+v", totals)
}

func TestDeleteProjectKeepsEntriesWithoutProject(t *testing.T) {
	now := int64(1000)
	tracker, service, db := wiredServices(t, &now)
	id := int64(count(t, db, `SELECT id FROM projects WHERE name = 'nexus'`))
	if err := service.DeleteProject(id); err != nil {
		t.Fatal(err)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM entries WHERE title = 'stopped' AND project = '' AND project_id IS NULL`); n != 1 {
		t.Fatal("entry must keep existing with empty project and NULL project_id")
	}
	totals, _ := tracker.Report(time.Unix(0, 0))
	for _, total := range totals {
		if total.Project == "" && total.Seconds >= 100 {
			return
		}
	}
	t.Fatalf("hours lost: %+v", totals)
}

func TestRenameProjectUpdatesEntriesAndNewEntriesLink(t *testing.T) {
	now := int64(1000)
	tracker, service, db := wiredServices(t, &now)
	id := int64(count(t, db, `SELECT id FROM projects WHERE name = 'nexus'`))
	if err := service.RenameProject(id, "Nexus 2"); err != nil {
		t.Fatal(err)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM entries WHERE project = 'Nexus 2' AND project_id = `+strconv.FormatInt(id, 10)); n != 1 {
		t.Fatal("entries not renamed")
	}
	entry, err := tracker.Start(tracking.StartInput{Title: "n", Project: "nexus 2"})
	if err != nil {
		t.Fatal(err)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM entries WHERE id = `+strconv.FormatInt(entry.ID, 10)+` AND project_id = `+strconv.FormatInt(id, 10)); n != 1 {
		t.Fatal("new entry not linked to existing project")
	}
}

func TestBreakFlowThroughWiredAdapter(t *testing.T) {
	now := int64(1000)
	tracker, _, db := wiredServices(t, &now)
	_, _, breaks := wire(db, func() time.Time { return time.Unix(now, 0) })
	running, _, _, err := tracker.Snapshot()
	if err != nil || len(running) == 0 {
		t.Fatalf("running = %+v, %v", running, err)
	}
	ids := []int64{running[0].ID}
	if _, err := breaks.StartBreak(10*time.Minute, "", ids); err != nil {
		t.Fatal(err)
	}
	if after, _, _, _ := tracker.Snapshot(); len(after) != len(running)-1 {
		t.Fatalf("work timers after break = %+v", after)
	}
	now = 1100
	if n, err := breaks.End(true); err != nil || n != 1 {
		t.Fatalf("End = %d, %v", n, err)
	}
	after, _, _, _ := tracker.Snapshot()
	if len(after) != len(running) || after[len(after)-1].ID == ids[0] {
		t.Fatalf("resumed running = %+v", after)
	}
	if secs, _ := tracker.BreakSeconds(time.Unix(0, 0)); secs != 100 {
		t.Fatalf("BreakSeconds = %d", secs)
	}
}
