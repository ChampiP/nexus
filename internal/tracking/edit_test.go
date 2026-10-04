package tracking

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

type nameResolver struct{ ids map[string]int64 }

func (r *nameResolver) EnsureProject(name string) (int64, error) {
	if name == "Ambiguo" {
		return 0, ErrAmbiguousProject
	}
	if id, ok := r.ids[name]; ok {
		return id, nil
	}
	id := int64(len(r.ids) + 1)
	r.ids[name] = id
	return id, nil
}

func (r *nameResolver) ProjectName(id int64) (string, error) {
	for name, pid := range r.ids {
		if pid == id {
			return name, nil
		}
	}
	return "", ErrNotFound
}

// newEditTracker returns a Tracker over a temp database whose clock is controlled through *now.
func newEditTracker(t *testing.T, now *int64) (*Tracker, *SQLite) {
	t.Helper()
	repo, err := openSQLite(filepath.Join(t.TempDir(), "nexus.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.db.Close() })
	for _, name := range []string{"p1", "p2", "p3"} {
		if _, err := repo.db.Exec(`INSERT INTO projects(name) VALUES (?)`, name); err != nil {
			t.Fatal(err)
		}
	}
	resolver := &nameResolver{ids: map[string]int64{"Alpha": 1, "Beta": 2}}
	clock := func() time.Time { return time.Unix(*now, 0) }
	return NewTracker(repo, clock, WithProjects(resolver)), repo
}

func projectID(t *testing.T, repo *SQLite, id int64) (v *int64) {
	t.Helper()
	var n *int64
	if err := repo.db.QueryRow(`SELECT project_id FROM entries WHERE id = ?`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func ptr[T any](v T) *T { return &v }

func TestStartWritesProjectID(t *testing.T) {
	now := int64(100)
	tracker, repo := newEditTracker(t, &now)
	with, err := tracker.Start(StartInput{Title: "a", Project: "Alpha"})
	if err != nil {
		t.Fatal(err)
	}
	without, err := tracker.Start(StartInput{Title: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if got := projectID(t, repo, with.ID); got == nil || *got != 1 {
		t.Fatalf("project_id = %v, want 1", got)
	}
	if got := projectID(t, repo, without.ID); got != nil {
		t.Fatalf("empty project must be NULL, got %v", *got)
	}
}

func TestEditRunningEntryKeepsItRunningAndRelinksProject(t *testing.T) {
	now := int64(100)
	tracker, repo := newEditTracker(t, &now)
	entry, _ := tracker.Start(StartInput{Title: "a", Project: "Alpha"})
	got, err := tracker.Edit(entry.ID, EditInput{Title: ptr("  New  "), Project: ptr("Beta"), Description: ptr("d")})
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "New" || got.Project != "Beta" || got.Description != "d" || got.EndedAt != nil {
		t.Fatalf("Edit = %+v", got)
	}
	if id := projectID(t, repo, entry.ID); id == nil || *id != 2 {
		t.Fatalf("project_id = %v, want 2", id)
	}
	running, _ := repo.Running()
	if len(running) != 1 || running[0].Title != "New" {
		t.Fatalf("running = %+v", running)
	}
	// Clearing the project nulls both columns; nil fields are left alone.
	got, err = tracker.Edit(entry.ID, EditInput{Project: ptr("")})
	if err != nil || got.Project != "" || got.Title != "New" || projectID(t, repo, entry.ID) != nil {
		t.Fatalf("clear project = %+v, %v", got, err)
	}
}

func TestEditStoppedEntryAndErrors(t *testing.T) {
	now := int64(100)
	tracker, _ := newEditTracker(t, &now)
	entry, _ := tracker.Start(StartInput{Title: "a"})
	now = 200
	if err := tracker.Stop(entry.ID); err != nil {
		t.Fatal(err)
	}
	got, err := tracker.Edit(entry.ID, EditInput{Title: ptr("b")})
	if err != nil || got.Title != "b" || got.EndedAt == nil || *got.EndedAt != 200 {
		t.Fatalf("Edit stopped = %+v, %v", got, err)
	}
	if _, err := tracker.Edit(entry.ID, EditInput{Title: ptr("  ")}); !errors.Is(err, ErrEmptyTitle) {
		t.Fatalf("blank title error = %v", err)
	}
	if _, err := tracker.Edit(999, EditInput{Title: ptr("x")}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing error = %v", err)
	}
	if err := tracker.Delete(entry.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := tracker.Edit(entry.ID, EditInput{Title: ptr("x")}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted error = %v", err)
	}
}

func TestDeleteStopsRunningHidesAndRestores(t *testing.T) {
	now := int64(100)
	tracker, repo := newEditTracker(t, &now)
	entry, _ := tracker.Start(StartInput{Title: "a"})
	now = 160
	if err := tracker.Delete(entry.ID); err != nil {
		t.Fatal(err)
	}
	running, recent, _ := mustLists(t, repo)
	if len(running) != 0 || len(recent) != 0 {
		t.Fatalf("deleted entry visible: %v %v", running, recent)
	}
	trash, err := tracker.Deleted(10)
	if err != nil || len(trash) != 1 || trash[0].EndedAt == nil || *trash[0].EndedAt != 160 {
		t.Fatalf("Deleted = %+v, %v", trash, err)
	}
	if err := tracker.Delete(entry.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("double delete error = %v", err)
	}
	if err := tracker.Restore(entry.ID); err != nil {
		t.Fatal(err)
	}
	// Restaurada al instante: vuelve a estar en curso, como antes de borrarla.
	running, recent, _ = mustLists(t, repo)
	if len(running) != 1 || len(recent) != 1 || recent[0].EndedAt != nil {
		t.Fatalf("after restore: %v %v", running, recent)
	}
	if err := tracker.Restore(entry.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("restore live error = %v", err)
	}
}

func mustLists(t *testing.T, repo *SQLite) (running, recent []Entry, err error) {
	t.Helper()
	running, err = repo.Running()
	if err != nil {
		t.Fatal(err)
	}
	recent, err = repo.Recent(10)
	if err != nil {
		t.Fatal(err)
	}
	return running, recent, nil
}

func TestDeletedListsNewestFirst(t *testing.T) {
	now := int64(100)
	tracker, _ := newEditTracker(t, &now)
	a, _ := tracker.Start(StartInput{Title: "a"})
	b, _ := tracker.Start(StartInput{Title: "b"})
	_ = tracker.Delete(a.ID)
	now = 150
	_ = tracker.Delete(b.ID)
	trash, _ := tracker.Deleted(1)
	if len(trash) != 1 || trash[0].ID != b.ID {
		t.Fatalf("Deleted(1) = %+v", trash)
	}
}

func TestPurgeRespectsAge(t *testing.T) {
	now := int64(1000)
	tracker, repo := newEditTracker(t, &now)
	old, _ := tracker.Start(StartInput{Title: "old"})
	fresh, _ := tracker.Start(StartInput{Title: "fresh"})
	live, _ := tracker.Start(StartInput{Title: "live"})
	_ = tracker.Delete(old.ID)
	now = 1900
	_ = tracker.Delete(fresh.ID)
	now = 2000
	n, err := tracker.Purge(500 * time.Second)
	if err != nil || n != 1 {
		t.Fatalf("Purge = %d, %v", n, err)
	}
	trash, _ := tracker.Deleted(10)
	if len(trash) != 1 || trash[0].ID != fresh.ID {
		t.Fatalf("trash = %+v", trash)
	}
	var count int
	_ = repo.db.QueryRow(`SELECT COUNT(*) FROM entries WHERE id = ?`, live.ID).Scan(&count)
	if count != 1 {
		t.Fatal("live entry purged")
	}
}

func TestRelinkAndRenameProject(t *testing.T) {
	now := int64(100)
	tracker, repo := newEditTracker(t, &now)
	a, _ := tracker.Start(StartInput{Title: "a", Project: "Alpha"})
	b, _ := tracker.Start(StartInput{Title: "b", Project: "Beta"})
	if err := tracker.RenameProject(1, "Alfa"); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.Get(a.ID); got.Project != "Alfa" {
		t.Fatalf("rename: %+v", got)
	}
	if err := tracker.Relink(1, 2, "Beta"); err != nil {
		t.Fatal(err)
	}
	if id := projectID(t, repo, a.ID); id == nil || *id != 2 {
		t.Fatalf("relink id = %v", id)
	}
	if got, _ := repo.Get(a.ID); got.Project != "Beta" {
		t.Fatalf("relink text: %+v", got)
	}
	if err := tracker.Relink(2, 0, ""); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{a.ID, b.ID} {
		if got, _ := repo.Get(id); got.Project != "" || projectID(t, repo, id) != nil {
			t.Fatalf("unlink %d: %+v", id, got)
		}
	}
}

// La papelera necesita saber cuándo se borró cada tarea, no cuándo empezó.
func TestDeletedCarriesDeletionTime(t *testing.T) {
	now := int64(100)
	tracker, _ := newEditTracker(t, &now)
	entry, _ := tracker.Start(StartInput{Title: "a"})
	now = 500
	if err := tracker.Delete(entry.ID); err != nil {
		t.Fatal(err)
	}
	trash, _ := tracker.Deleted(10)
	if len(trash) != 1 || trash[0].DeletedAt == nil || *trash[0].DeletedAt != 500 {
		t.Fatalf("Deleted() = %+v, quiero DeletedAt=500", trash)
	}
}

// Cuenta solo las tareas vivas de un proyecto, para avisar cuántas afecta un cambio.
func TestCountByProjectIgnoresDeleted(t *testing.T) {
	now := int64(100)
	tracker, _ := newEditTracker(t, &now)
	a, _ := tracker.Start(StartInput{Title: "a", Project: "Alpha"})
	_, _ = tracker.Start(StartInput{Title: "b", Project: "Alpha"})
	_, _ = tracker.Start(StartInput{Title: "c", Project: "Beta"})
	_ = tracker.Delete(a.ID)
	if n, err := tracker.CountByProject(1); err != nil || n != 1 {
		t.Fatalf("CountByProject(1) = %d, %v; quiero 1", n, err)
	}
}

// Deshacer un borrado reciente de un temporizador en curso lo deja corriendo, como estaba.
func TestRestoreSoonAfterDeletingRunningEntryResumesIt(t *testing.T) {
	now := int64(100)
	tracker, _ := newEditTracker(t, &now)
	entry, _ := tracker.Start(StartInput{Title: "a"})
	now = 200
	_ = tracker.Delete(entry.ID)
	now = 230
	if err := tracker.Restore(entry.ID); err != nil {
		t.Fatal(err)
	}
	running, _, _, _ := tracker.Snapshot()
	if len(running) != 1 || running[0].ID != entry.ID {
		t.Fatalf("running = %+v, quiero la tarea restaurada en curso", running)
	}
}

// Restaurar mucho después, o una tarea que ya estaba detenida, no la vuelve a iniciar.
func TestRestoreLateOrStoppedEntryStaysStopped(t *testing.T) {
	now := int64(100)
	tracker, _ := newEditTracker(t, &now)
	late, _ := tracker.Start(StartInput{Title: "tarde"})
	stopped, _ := tracker.Start(StartInput{Title: "detenida"})
	now = 150
	_ = tracker.Stop(stopped.ID)
	now = 200
	_ = tracker.Delete(late.ID)
	_ = tracker.Delete(stopped.ID)
	now = 200 + 3600
	_ = tracker.Restore(late.ID)
	now = 210 + 3600
	_ = tracker.Restore(stopped.ID)
	if running, _, _, _ := tracker.Snapshot(); len(running) != 0 {
		t.Fatalf("running = %+v, quiero ninguna", running)
	}
}

func TestStartWithProjectIDSetsIDAndText(t *testing.T) {
	// Comprueba que iniciar con ProjectID asigna tanto project_id como el nombre de texto resuelto.
	now := int64(100)
	tracker, repo := newEditTracker(t, &now)
	entry, err := tracker.Start(StartInput{Title: "tarea con id", ProjectID: 1})
	if err != nil {
		t.Fatalf("Start con ProjectID: %v", err)
	}
	if entry.ProjectID != 1 {
		t.Fatalf("entry.ProjectID = %d, quería 1", entry.ProjectID)
	}
	if entry.Project != "Alpha" {
		t.Fatalf("entry.Project = %q, quería 'Alpha'", entry.Project)
	}
	var storedText string
	var storedID sql.NullInt64
	if err := repo.db.QueryRow(`SELECT project, project_id FROM entries WHERE id = ?`, entry.ID).Scan(&storedText, &storedID); err != nil {
		t.Fatal(err)
	}
	if !storedID.Valid || storedID.Int64 != 1 || storedText != "Alpha" {
		t.Fatalf("almacenado = text %q, id %v", storedText, storedID)
	}
}

func TestStartWithAmbiguousNameReturnsErrAmbiguousProject(t *testing.T) {
	// Comprueba que resolver un nombre ambiguo retorna ErrAmbiguousProject y no inserta la entrada.
	now := int64(100)
	tracker, repo := newEditTracker(t, &now)
	_, err := tracker.Start(StartInput{Title: "tarea ambigua", Project: "Ambiguo"})
	if !errors.Is(err, ErrAmbiguousProject) {
		t.Fatalf("se esperaba ErrAmbiguousProject, se obtuvo %v", err)
	}
	var count int
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM entries WHERE title = 'tarea ambigua'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("se insertaron %d filas con nombre ambiguo; no debía insertarse ninguna", count)
	}
}

func TestEditAndEditTaskWithProjectID(t *testing.T) {
	// Comprueba que Edit y EditTask con ProjectID actualizan correctamente project_id y el nombre resuelto.
	now := int64(100)
	tracker, _ := newEditTracker(t, &now)
	entry, err := tracker.Start(StartInput{Title: "tarea base", Project: "Alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if entry.ProjectID != 1 {
		t.Fatalf("ProjectID inicial = %d, quería 1", entry.ProjectID)
	}

	pid2 := int64(2)
	edited, err := tracker.Edit(entry.ID, EditInput{ProjectID: &pid2})
	if err != nil {
		t.Fatalf("Edit con ProjectID: %v", err)
	}
	if edited.ProjectID != 2 || edited.Project != "Beta" {
		t.Fatalf("edited = id %d, project %q; quería id 2, project 'Beta'", edited.ProjectID, edited.Project)
	}

	pid1 := int64(1)
	taskEdited, err := tracker.EditTask(entry.TaskUID, EditInput{ProjectID: &pid1})
	if err != nil {
		t.Fatalf("EditTask con ProjectID: %v", err)
	}
	if taskEdited.ProjectID != 1 || taskEdited.Project != "Alpha" {
		t.Fatalf("taskEdited = id %d, project %q; quería id 1, project 'Alpha'", taskEdited.ProjectID, taskEdited.Project)
	}
}
