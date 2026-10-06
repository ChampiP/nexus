package tracking

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	platformdb "nexus/internal/platform/db"
)

// openLegacy abre una base con el esquema de las fases 1 y 2 (tracking v1 y v2) y devuelve la ruta,
// para aplicar después v3 como lo haría un binario nuevo sobre los datos reales.
func openLegacy(t *testing.T) (*sql.DB, string, []platformdb.Migration) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nexus.db")
	db, err := platformdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	stub := platformdb.Migration{Module: "catalog", Version: 1, Up: func(tx *sql.Tx) error {
		_, err := tx.Exec(`CREATE TABLE projects (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`)
		return err
	}}
	steps := Migrations()
	all := []platformdb.Migration{steps[0], stub}
	all = append(all, steps[1:]...)
	if _, _, err := platformdb.Migrate(db, path, all[:3], time.Now); err != nil {
		t.Fatal(err)
	}
	return db, path, all
}

func taskUIDs(t *testing.T, db *sql.DB) map[int64]string {
	t.Helper()
	rows, err := db.Query(`SELECT id, COALESCE(task_uid, '') FROM entries`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var id int64
		var uid string
		if err := rows.Scan(&id, &uid); err != nil {
			t.Fatal(err)
		}
		out[id] = uid
	}
	return out
}

func TestRestoreDoesNotReviveSessionStoppedAtDeleteSecond(t *testing.T) {
	s, err := openSQLite(filepath.Join(t.TempDir(), "restore.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()

	entry, err := s.Insert(Entry{TaskUID: "task", Title: "task", StartedAt: 10})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Stop(entry.ID, 20); err != nil {
		t.Fatal(err)
	}
	if err := s.SoftDelete(entry.ID, 20); err != nil {
		t.Fatal(err)
	}
	if err := s.Restore(entry.ID, 0); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(entry.ID)
	if err != nil || got.EndedAt == nil || *got.EndedAt != 20 {
		t.Fatalf("restored stopped entry = %+v, %v; want ended_at 20", got, err)
	}
}

func TestRestoreTaskRevivesOnlySessionRunningAtDelete(t *testing.T) {
	s, err := openSQLite(filepath.Join(t.TempDir(), "restore-task.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()

	stopped, err := s.Insert(Entry{TaskUID: "task", Title: "task", StartedAt: 10})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Stop(stopped.ID, 20); err != nil {
		t.Fatal(err)
	}
	running, err := s.Insert(Entry{TaskUID: "task", Title: "task", StartedAt: 15})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SoftDeleteTask("task", 20); err != nil {
		t.Fatal(err)
	}
	if err := s.RestoreTask("task", 0); err != nil {
		t.Fatal(err)
	}
	stoppedAfter, err := s.Get(stopped.ID)
	if err != nil || stoppedAfter.EndedAt == nil || *stoppedAfter.EndedAt != 20 {
		t.Fatalf("stopped session = %+v, %v; want ended_at 20", stoppedAfter, err)
	}
	runningAfter, err := s.Get(running.ID)
	if err != nil || runningAfter.EndedAt != nil {
		t.Fatalf("formerly running session = %+v, %v; want running", runningAfter, err)
	}
}

func TestRestoreKeepsSessionStoppedWhenTaskAlreadyRunning(t *testing.T) {
	s, err := openSQLite(filepath.Join(t.TempDir(), "restore-conflict.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()

	deleted, err := s.Insert(Entry{TaskUID: "task", Title: "task", StartedAt: 10})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SoftDelete(deleted.ID, 20); err != nil {
		t.Fatal(err)
	}
	active, err := s.Insert(Entry{TaskUID: "task", Title: "task", StartedAt: 21})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Restore(deleted.ID, 0); err != nil {
		t.Fatalf("Restore with an active sibling: %v", err)
	}
	got, err := s.Get(deleted.ID)
	if err != nil || got.EndedAt == nil || *got.EndedAt != 20 {
		t.Fatalf("restored session = %+v, %v; want stopped at 20", got, err)
	}
	if _, err := s.Get(active.ID); err != nil {
		t.Fatalf("active sibling: %v", err)
	}
}

func TestMigrationV3GroupsIdenticalSessionsAndIsIdempotent(t *testing.T) {
	db, path, all := openLegacy(t)
	// Forma de los datos del usuario: #5 y #8 idénticos y detenidos, un break y una fila borrada.
	if _, err := db.Exec(`
INSERT INTO entries(id, uid, title, description, project, kind, started_at, ended_at, deleted_at) VALUES
 (1, 'U1', 'otra', '', 'p', 'work', 10, 20, NULL),
 (5, 'U5', 'diseno de pagina de inspira salud', 'd', 'inspira', 'work', 100, 200, NULL),
 (6, 'U6', 'Break', '', '', 'break', 210, 300, NULL),
 (7, 'U7', 'diseno de pagina de inspira salud', 'd', 'inspira', 'work', 310, 320, 400),
 (8, 'U8', 'diseno de pagina de inspira salud', 'd', 'inspira', 'work', 500, 600, NULL),
 (9, NULL, 'sin uid', '', '', 'work', 700, 710, NULL)`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, _, err := platformdb.Migrate(db, path, all, time.Now); err != nil {
			t.Fatal(err)
		}
	}
	got := taskUIDs(t, db)
	want := map[int64]string{1: "U1", 5: "U5", 6: "U6", 7: "U7", 8: "U5"}
	for id, uid := range want {
		if got[id] != uid {
			t.Errorf("task_uid de #%d = %q, quería %q", id, got[id], uid)
		}
	}
	var uid9 string
	_ = db.QueryRow(`SELECT uid FROM entries WHERE id = 9`).Scan(&uid9)
	if uid9 == "" || got[9] != uid9 {
		t.Errorf("la fila sin uid debe recibir uid y task_uid propio: uid=%q task_uid=%q", uid9, got[9])
	}
	var idx int
	_ = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='entries_task_uid_idx'`).Scan(&idx)
	if idx != 1 {
		t.Error("falta el índice de task_uid")
	}
}

func TestReconcileFillsNullTaskUID(t *testing.T) {
	s, err := openSQLite(filepath.Join(t.TempDir(), "nexus.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	// Una fila escrita por un binario anterior: sin uid ni task_uid.
	if _, err := s.db.Exec(`INSERT INTO entries(title, started_at) VALUES ('x', 1)`); err != nil {
		t.Fatal(err)
	}
	if err := s.Reconcile(fakeResolver{}); err != nil {
		t.Fatal(err)
	}
	var uid, task string
	_ = s.db.QueryRow(`SELECT uid, task_uid FROM entries`).Scan(&uid, &task)
	if uid == "" || task != uid {
		t.Fatalf("uid=%q task_uid=%q", uid, task)
	}
}

func TestIntegrityRulesRejectSecondRunningAndInvalidIntervals(t *testing.T) {
	s, err := openSQLite(filepath.Join(t.TempDir(), "integrity.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	first, err := s.Insert(Entry{TaskUID: "same", Title: "a", StartedAt: 10})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Insert(Entry{TaskUID: "same", Title: "b", StartedAt: 11}); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second running insert = %v", err)
	}
	if _, err := s.db.Exec(`INSERT INTO entries(uid,task_uid,kind,title,started_at,ended_at) VALUES('bad','bad-task','work','bad',20,19)`); err == nil {
		t.Fatal("invalid insert interval accepted")
	}
	if _, err := s.db.Exec(`UPDATE entries SET ended_at = 9 WHERE id = ?`, first.ID); err == nil {
		t.Fatal("invalid update interval accepted")
	}
}

func TestMigrationV4RepairsMultipleRunningSessions(t *testing.T) {
	db, path, all := openLegacy(t)
	if _, _, err := platformdb.Migrate(db, path, all[:4], time.Now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO entries(uid,task_uid,kind,title,started_at) VALUES
		('older','task','work','older',10),('newer','task','work','newer',20)`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := platformdb.Migrate(db, path, all, func() time.Time { return time.Now().Add(time.Minute) }); err != nil {
		t.Fatal(err)
	}
	var oldEnd sql.NullInt64
	if err := db.QueryRow(`SELECT ended_at FROM entries WHERE uid='older'`).Scan(&oldEnd); err != nil || !oldEnd.Valid || oldEnd.Int64 != 20 {
		t.Fatalf("older ended_at = %v, %v", oldEnd, err)
	}
	if got := countRunning(t, db, "task"); got != 1 {
		t.Fatalf("running sessions = %d", got)
	}
}

func countRunning(t *testing.T, db *sql.DB, task string) int {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM entries WHERE task_uid=? AND ended_at IS NULL AND deleted_at IS NULL`, task).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestStartIsNewTaskAndResumeJoinsIt(t *testing.T) {
	now := int64(1000)
	tracker := newBreakTracker(t, &now)
	a, _ := tracker.Start(StartInput{Title: "t", Project: "P", Description: "d"})
	if a.TaskUID == "" || a.TaskUID != a.UID {
		t.Fatalf("Start: task_uid = %q, uid = %q", a.TaskUID, a.UID)
	}
	other, _ := tracker.Start(StartInput{Title: "t", Project: "P", Description: "d"})
	if other.TaskUID == a.TaskUID {
		t.Fatal("dos Start iguales son tareas distintas")
	}
	now = 1100
	if _, err := tracker.Resume(a.ID); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("Resume con sesión en curso = %v", err)
	}
	if err := tracker.Stop(a.ID); err != nil {
		t.Fatal(err)
	}
	now = 1200
	b, err := tracker.Resume(a.ID)
	if err != nil || b.ID == a.ID || b.TaskUID != a.TaskUID || b.Title != "t" || b.Project != "P" || b.Description != "d" || b.EndedAt != nil || b.StartedAt != 1200 {
		t.Fatalf("Resume = %+v, %v", b, err)
	}
	if _, err := tracker.Resume(a.ID); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("segundo Resume = %v", err)
	}
	if _, err := tracker.Resume(9999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Resume inexistente = %v", err)
	}
	total, err := tracker.TaskTotal(a.TaskUID)
	now = 1500
	if err != nil || total != 100+0 {
		t.Fatalf("TaskTotal en 1200 = %d, %v", total, err)
	}
	if total, _ = tracker.TaskTotal(a.TaskUID); total != 100+300 {
		t.Fatalf("TaskTotal en 1500 = %d", total)
	}
}

func TestRecentTasksTotalsOrderAndRunning(t *testing.T) {
	now := int64(1000)
	tracker := newBreakTracker(t, &now)
	a, _ := tracker.Start(StartInput{Title: "a", Project: "P"})
	now = 1100
	_ = tracker.Stop(a.ID)
	b, _ := tracker.Start(StartInput{Title: "b"})
	now = 1200
	_ = tracker.Stop(b.ID)
	brk, _ := tracker.Start(StartInput{Title: "Break", Kind: KindBreak})
	_ = tracker.Stop(brk.ID)
	now = 1300
	a2, _ := tracker.Resume(a.ID)
	now = 1360
	tasks, err := tracker.RecentTasks(10)
	if err != nil || len(tasks) != 2 {
		t.Fatalf("RecentTasks = %+v, %v", tasks, err)
	}
	first, second := tasks[0], tasks[1]
	if first.TaskUID != a.TaskUID || first.Title != "a" || first.Project != "P" || first.TotalSeconds != 100+60 || !first.Running ||
		first.RunningEntryID != a2.ID || first.LastEntryID != a2.ID || first.LastActivity != 1360 || first.SessionCount != 2 {
		t.Fatalf("tarea en curso = %+v", first)
	}
	if second.Title != "b" || second.Running || second.RunningEntryID != 0 || second.TotalSeconds != 100 || second.LastActivity != 1200 || second.SessionCount != 1 || second.LastEntryID != b.ID {
		t.Fatalf("tarea detenida = %+v", second)
	}
	if limited, _ := tracker.RecentTasks(1); len(limited) != 1 || limited[0].Title != "a" {
		t.Fatalf("límite = %+v", limited)
	}
	if err := tracker.Delete(a2.ID); err != nil {
		t.Fatal(err)
	}
	tasks, _ = tracker.RecentTasks(10)
	if tasks[1].Title != "a" || tasks[1].SessionCount != 1 || tasks[1].Running || tasks[1].TotalSeconds != 100 {
		t.Fatalf("una sesión borrada no cuenta: %+v", tasks)
	}
}

func TestEditTaskUpdatesEverySession(t *testing.T) {
	now := int64(1000)
	tracker := newBreakTracker(t, &now)
	a, _ := tracker.Start(StartInput{Title: "a", Project: "P"})
	now = 1100
	_ = tracker.Stop(a.ID)
	a2, _ := tracker.Resume(a.ID)
	if _, err := tracker.EditTask(a.TaskUID, EditInput{Title: ptr(" nuevo "), Description: ptr("desc")}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{a.ID, a2.ID} {
		e, _ := tracker.Get(id)
		if e.Title != "nuevo" || e.Description != "desc" || e.Project != "P" {
			t.Fatalf("sesión #%d = %+v", id, e)
		}
	}
	if _, err := tracker.EditTask(a.TaskUID, EditInput{Title: ptr(" ")}); !errors.Is(err, ErrEmptyTitle) {
		t.Fatalf("título vacío = %v", err)
	}
	if _, err := tracker.EditTask("nada", EditInput{Title: ptr("x")}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("tarea inexistente = %v", err)
	}
}

func TestDeleteAndRestoreTaskAcrossSessions(t *testing.T) {
	now := int64(1000)
	tracker := newBreakTracker(t, &now)
	a, _ := tracker.Start(StartInput{Title: "a"})
	now = 1100
	_ = tracker.Stop(a.ID)
	now = 1200
	a2, _ := tracker.Resume(a.ID)
	now = 1230
	if err := tracker.DeleteTask(a.TaskUID); err != nil {
		t.Fatal(err)
	}
	if running, _, recent, _ := tracker.Snapshot(); len(running) != 0 || len(recent) != 0 {
		t.Fatalf("la tarea debe desaparecer: %v %v", running, recent)
	}
	if err := tracker.DeleteTask(a.TaskUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("borrar dos veces = %v", err)
	}
	now = 1240
	if err := tracker.RestoreTask(a.TaskUID); err != nil {
		t.Fatal(err)
	}
	tasks, _ := tracker.RecentTasks(10)
	if len(tasks) != 1 || tasks[0].SessionCount != 2 || !tasks[0].Running || tasks[0].RunningEntryID != a2.ID {
		t.Fatalf("deshacer inmediato debe reanudar la sesión en curso: %+v", tasks)
	}
	// Una sesión borrada antes, por separado, no vuelve con la tarea.
	now = 1250
	_ = tracker.Stop(a2.ID)
	a3, _ := tracker.Resume(a.ID)
	_ = tracker.Stop(a3.ID)
	if err := tracker.Delete(a.ID); err != nil {
		t.Fatal(err)
	}
	now = 1300
	if err := tracker.DeleteTask(a.TaskUID); err != nil {
		t.Fatal(err)
	}
	if err := tracker.RestoreTask(a.TaskUID); err != nil {
		t.Fatal(err)
	}
	tasks, _ = tracker.RecentTasks(10)
	if len(tasks) != 1 || tasks[0].SessionCount != 2 || tasks[0].Running {
		t.Fatalf("solo vuelven las sesiones borradas juntas: %+v", tasks)
	}
	if err := tracker.RestoreTask("nada"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("restaurar inexistente = %v", err)
	}
}
