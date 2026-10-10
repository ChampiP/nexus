package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"nexus/internal/catalog"
	"nexus/internal/countdown"
	platformdb "nexus/internal/platform/db"
	"nexus/internal/platform/presence"
	"nexus/internal/tracking"
	"nexus/internal/wellbeing"
)

type activityFixture map[string]string

func (f activityFixture) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	key := name + " " + strings.Join(args, " ")
	value, ok := f[key]
	if !ok {
		return nil, errors.New("unexpected presence command")
	}
	return []byte(value), nil
}

func TestStatusOpenFailureKeepsValidJSONAndExplainsError(t *testing.T) {
	badPath := filepath.Join(t.TempDir(), "database-directory")
	if err := os.Mkdir(badPath, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NEXUS_DB", badPath)
	capture, err := os.CreateTemp(t.TempDir(), "status-output")
	if err != nil {
		t.Fatal(err)
	}
	defer capture.Close()
	original := os.Stdout
	os.Stdout = capture
	runErr := run([]string{"status", "--json"})
	os.Stdout = original
	if runErr != nil {
		t.Fatal(runErr)
	}
	if _, err := capture.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(capture)
	if err != nil {
		t.Fatal(err)
	}
	var actual map[string]any
	if err := json.Unmarshal(output, &actual); err != nil {
		t.Fatalf("output=%s err=%v", output, err)
	}
	if actual["error"] != "no se pudo leer la base de datos" {
		t.Fatalf("output=%s", output)
	}
}

func TestStatusOpenFailureHelperKeepsValidJSON(t *testing.T) {
	var out bytes.Buffer
	if err := writeStatusOpenError(&out, errors.New("database is locked")); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["today_seconds"] != float64(0) || got["error"] != "no se pudo leer la base de datos" {
		t.Fatalf("status error JSON = %s", out.String())
	}
}

func TestWellActivityTreatsIdleAndLockedAsInactive(t *testing.T) {
	for _, tc := range []struct {
		name, idle, locked string
		want               bool
	}{
		{"active", `{"enabled":true,"idle":false,"inIdleCycle":false}`, "false", true},
		{"idle", `{"enabled":true,"idle":true,"inIdleCycle":true}`, "false", false},
		{"locked", `{"enabled":true,"idle":false,"inIdleCycle":false}`, "true", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			detector := presence.New(activityFixture{"omarchy-shell idle status": tc.idle, "omarchy-shell lock isLocked": tc.locked})
			active, err := (wellActivity{presence: detector}).Active(time.Now())
			if err != nil || active != tc.want {
				t.Fatalf("Active() = %v, %v; want %v", active, err, tc.want)
			}
		})
	}
}

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

func TestOrderedMigrationsAreIdempotentAndCreateIntegrityRules(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "fresh", true: "phase_one"}[legacy], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "nexus.db")
			db, err := platformdb.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if legacy {
				if _, err := db.Exec(legacySchema); err != nil {
					t.Fatal(err)
				}
			}
			for run := 0; run < 2; run++ {
				if _, _, err := platformdb.Migrate(db, path, orderedMigrations(), func() time.Time { return time.Unix(1_800_000_000+int64(run), 0) }); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range []string{"entries_started_at_idx", "entries_project_started_idx", "entries_one_running_per_task_idx", "clients_null_org_name_idx", "projects_null_client_name_idx", "countdowns_one_active_idx", "countdowns_active_idx", "wellbeing_events_at_idx"} {
				if got := count(t, db, `SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='`+name+`'`); got != 1 {
					t.Errorf("falta índice %s", name)
				}
			}
			for _, name := range []string{"entries_ended_after_started_insert", "entries_ended_after_started_update"} {
				if got := count(t, db, `SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND name='`+name+`'`); got != 1 {
					t.Errorf("falta trigger %s", name)
				}
			}
		})
	}
}

func TestShutdownStopperAdaptsTracking(t *testing.T) {
	db, path := legacyDB(t)
	upgrade(t, db, path)
	tracker := tracking.NewTracker(tracking.NewSQLite(db), nil)
	stopper := shutdownStopper{tracker: tracker}
	entries, err := stopper.StopRunningAt(time.Unix(600, 0))
	if err != nil || len(entries) != 1 || entries[0].Title != "running" {
		t.Fatalf("stopped = %v, err=%v", entries, err)
	}
	if got := count(t, db, "SELECT COUNT(*) FROM entries WHERE title='running' AND ended_at=600"); got != 1 {
		t.Fatal("session did not end at heartbeat")
	}
	entries, err = stopper.StopRunningAt(time.Unix(600, 0))
	if err != nil || len(entries) != 0 {
		t.Fatalf("repeat = %v, err=%v", entries, err)
	}
}

func TestPurgingEntryNullsCountdownReference(t *testing.T) {
	db, path := legacyDB(t)
	upgrade(t, db, path)
	entry, err := tracking.NewSQLite(db).Insert(tracking.Entry{Title: "break", Kind: tracking.KindBreak, StartedAt: 10})
	if err != nil {
		t.Fatal(err)
	}
	breakRow, err := countdown.NewSQLite(db).Insert(countdown.Break{Label: "Break", StartedAt: 10, EndsAt: 20, EntryID: entry.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE entries SET deleted_at=1 WHERE id=?`, entry.ID); err != nil {
		t.Fatal(err)
	}
	if n, err := tracking.NewSQLite(db).Purge(2); err != nil || n != 1 {
		t.Fatalf("Purge=%d, %v", n, err)
	}
	var entryID sql.NullInt64
	if err := db.QueryRow(`SELECT entry_id FROM countdowns WHERE id=?`, breakRow.ID).Scan(&entryID); err != nil || entryID.Valid {
		t.Fatalf("entry_id=%v err=%v", entryID, err)
	}
}

func TestFreshDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "nexus.db")
	db, err := platformdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	applied, backup := upgrade(t, db, path)
	if applied != len(orderedMigrations()) || backup != "" {
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
	if applied != len(orderedMigrations()) || backup == "" {
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

func TestCatalogV3PreservesEntriesProjectLinksAndIDs(t *testing.T) {
	// (a) En una base migrada a la versión previa con entradas vinculadas a proyectos,
	// correr la nueva migración debe mantener intactos entries.project_id, ids/uids de proyectos
	// y dejar foreign_key_check vacío.
	// (b) Tras la migración, se permiten dos proyectos con el mismo nombre en distintos clientes,
	// pero colisionan bajo el mismo cliente o sin cliente.
	// (c) Segunda ejecución idempotente.
	path := filepath.Join(t.TempDir(), "nexus_v3_test.db")
	db, err := platformdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	allMigrations := orderedMigrations()
	var prevMigrations []platformdb.Migration
	for _, m := range allMigrations {
		if m.Module == "catalog" && m.Version == 3 {
			continue
		}
		prevMigrations = append(prevMigrations, m)
	}

	if _, _, err := platformdb.Migrate(db, path, prevMigrations, func() time.Time { return time.Unix(1_800_000_000, 0) }); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(`
INSERT INTO clients(id, uid, name) VALUES (1, 'c1', 'Cliente A'), (2, 'c2', 'Cliente B');
INSERT INTO projects(id, uid, name, client_id) VALUES (10, 'p10', 'Alpha', 1), (20, 'p20', 'Beta', NULL);
INSERT INTO entries(id, uid, title, project, project_id, started_at, kind) VALUES 
    (100, 'e100', 'Tarea 1', 'Alpha', 10, 1000, 'work'),
    (200, 'e200', 'Tarea 2', 'Beta', 20, 2000, 'work');
`); err != nil {
		t.Fatal(err)
	}

	type entryState struct {
		projectID sql.NullInt64
	}
	type projectState struct {
		id       int64
		uid      string
		name     string
		clientID sql.NullInt64
	}

	readEntries := func() map[int64]entryState {
		m := map[int64]entryState{}
		rows, err := db.Query(`SELECT id, project_id FROM entries`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			var s entryState
			if err := rows.Scan(&id, &s.projectID); err != nil {
				t.Fatal(err)
			}
			m[id] = s
		}
		return m
	}

	readProjects := func() map[int64]projectState {
		m := map[int64]projectState{}
		rows, err := db.Query(`SELECT id, uid, name, client_id FROM projects`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var p projectState
			if err := rows.Scan(&p.id, &p.uid, &p.name, &p.clientID); err != nil {
				t.Fatal(err)
			}
			m[p.id] = p
		}
		return m
	}

	entriesBefore := readEntries()
	projectsBefore := readProjects()

	applied, _, err := platformdb.Migrate(db, path, allMigrations, func() time.Time { return time.Unix(1_800_000_100, 0) })
	if err != nil {
		t.Fatalf("Migrate con catalog v3: %v", err)
	}
	if applied != 1 {
		t.Fatalf("se esperaba 1 migración aplicada, se aplicaron %d", applied)
	}

	entriesAfter := readEntries()
	if len(entriesAfter) != len(entriesBefore) {
		t.Fatalf("cantidad de entradas cambió: antes %d, después %d", len(entriesBefore), len(entriesAfter))
	}
	for id, before := range entriesBefore {
		after, ok := entriesAfter[id]
		if !ok {
			t.Fatalf("falta entrada id %d", id)
		}
		if before.projectID.Valid != after.projectID.Valid || before.projectID.Int64 != after.projectID.Int64 {
			t.Fatalf("entries.project_id modificado para id %d: antes %v, después %v", id, before.projectID, after.projectID)
		}
	}

	projectsAfter := readProjects()
	if len(projectsAfter) != len(projectsBefore) {
		t.Fatalf("cantidad de proyectos cambió: antes %d, después %d", len(projectsBefore), len(projectsAfter))
	}
	for id, before := range projectsBefore {
		after, ok := projectsAfter[id]
		if !ok {
			t.Fatalf("falta proyecto id %d", id)
		}
		if before.id != after.id || before.uid != after.uid || before.name != after.name {
			t.Fatalf("proyecto modificado para id %d: antes %+v, después %+v", id, before, after)
		}
	}

	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	var fkErrors []string
	for rows.Next() {
		var table, parent string
		var rowid, fkid int64
		_ = rows.Scan(&table, &rowid, &parent, &fkid)
		fkErrors = append(fkErrors, fmt.Sprintf("tabla %s fila %d", table, rowid))
	}
	rows.Close()
	if len(fkErrors) > 0 {
		t.Fatalf("foreign_key_check con infracciones: %v", fkErrors)
	}

	if _, err := db.Exec(`INSERT INTO projects(uid, name, client_id) VALUES ('u1', 'Diseno de web', 1)`); err != nil {
		t.Fatalf("error al crear 'Diseno de web' en cliente 1: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO projects(uid, name, client_id) VALUES ('u2', 'Diseno de web', 2)`); err != nil {
		t.Fatalf("error al crear 'Diseno de web' en cliente 2: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO projects(uid, name, client_id) VALUES ('u3', 'diseno de web', 1)`); err == nil {
		t.Fatal("se esperaba fallo de unicidad al insertar nombre duplicado en el mismo cliente")
	}
	if _, err := db.Exec(`INSERT INTO projects(uid, name, client_id) VALUES ('u4', 'Sin cliente x', NULL)`); err != nil {
		t.Fatalf("error al crear primer proyecto sin cliente: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO projects(uid, name, client_id) VALUES ('u5', 'sin cliente x', NULL)`); err == nil {
		t.Fatal("se esperaba fallo de unicidad al insertar nombre duplicado sin cliente")
	}

	appliedSecond, backupSecond, err := platformdb.Migrate(db, path, allMigrations, func() time.Time { return time.Unix(1_800_000_200, 0) })
	if err != nil || appliedSecond != 0 || backupSecond != "" {
		t.Fatalf("segunda ejecución no idempotente: applied=%d backup=%q err=%v", appliedSecond, backupSecond, err)
	}
}

func TestConcurrentOrderedMigrationsOnLegacyDatabase(t *testing.T) {
	// (e) Concurrencia: N goroutines con su propia conexión *sql.DB ejecutan Migrate
	// sobre una copia legada -> exactamente 1 copia de seguridad, cada migración se registra una vez,
	// enlaces de claves foráneas intactos.
	path := filepath.Join(t.TempDir(), "legacy_concurrent.db")
	seed, err := platformdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Exec(legacySchema); err != nil {
		t.Fatal(err)
	}
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}

	const workers = 8
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := platformdb.Open(path)
			if err != nil {
				errs <- err
				return
			}
			defer conn.Close()
			<-start
			_, _, err = platformdb.Migrate(conn, path, orderedMigrations(), func() time.Time { return time.Unix(1_800_000_000, 0) })
			if err != nil {
				errs <- err
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("Migrate concurrente falló: %v", err)
	}

	verifyDB, err := platformdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer verifyDB.Close()

	backups, err := filepath.Glob(path + ".bak-*")
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups=%v, want exactamente 1", backups)
	}

	var migrationCount int
	if err := verifyDB.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&migrationCount); err != nil {
		t.Fatal(err)
	}
	if migrationCount != len(orderedMigrations()) {
		t.Fatalf("migraciones en schema_migrations = %d, want %d", migrationCount, len(orderedMigrations()))
	}

	var linkedEntries int
	if err := verifyDB.QueryRow(`SELECT COUNT(*) FROM entries WHERE project <> '' AND project_id IS NOT NULL`).Scan(&linkedEntries); err != nil {
		t.Fatal(err)
	}
	var fkViolations int
	rows, err := verifyDB.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		fkViolations++
	}
	rows.Close()
	if fkViolations != 0 {
		t.Fatalf("foreign_key_check tiene %d infracciones", fkViolations)
	}
}

func TestCatalogProjectsAdapterReturnsErrAmbiguousProject(t *testing.T) {
	// Comprueba que el adaptador catalogProjects en main.go retorna ErrAmbiguousProject
	// cuando el nombre existe bajo más de un cliente.
	path := filepath.Join(t.TempDir(), "nexus_ambig.db")
	db, err := platformdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, _, err := platformdb.Migrate(db, path, orderedMigrations(), time.Now); err != nil {
		t.Fatal(err)
	}

	catRepo := catalog.NewSQLite(db)
	c1, err := catRepo.CreateClient("Cliente 1", 0)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := catRepo.CreateClient("Cliente 2", 0)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := catRepo.CreateProject("SharedName", c1.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := catRepo.CreateProject("SharedName", c2.ID); err != nil {
		t.Fatal(err)
	}

	adapter := catalogProjects{catalog: catRepo}
	_, err = adapter.EnsureProject("SharedName")
	if !errors.Is(err, catalog.ErrAmbiguousProject) {
		t.Fatalf("se esperaba catalog.ErrAmbiguousProject, se obtuvo %v", err)
	}
	if !errors.Is(err, tracking.ErrAmbiguousProject) {
		t.Fatalf("se esperaba tracking.ErrAmbiguousProject, se obtuvo %v", err)
	}

	name, err := adapter.ProjectName(1)
	if err != nil || name != "SharedName" {
		t.Fatalf("ProjectName(1) = %q, %v", name, err)
	}
}

// Cada paso de cada módulo debe estar en la lista ordenada exactamente una vez; si no, la app real nunca lo aplica.
func TestOrderedMigrationsIncludeEveryModuleStep(t *testing.T) {
	want := map[string]int{}
	for _, steps := range [][]platformdb.Migration{tracking.Migrations(), catalog.Migrations(), countdown.Migrations(), wellbeing.Migrations()} {
		for _, s := range steps {
			want[fmt.Sprintf("%s v%d", s.Module, s.Version)]++
		}
	}
	got := map[string]int{}
	for _, s := range orderedMigrations() {
		got[fmt.Sprintf("%s v%d", s.Module, s.Version)]++
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("orderedMigrations = %v, want %v", got, want)
	}
}
