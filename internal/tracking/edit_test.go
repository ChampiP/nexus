package tracking

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

type nameResolver struct{ ids map[string]int64 }

func (r *nameResolver) EnsureProject(name string) (int64, error) {
	if id, ok := r.ids[name]; ok {
		return id, nil
	}
	id := int64(len(r.ids) + 1)
	r.ids[name] = id
	return id, nil
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
	running, recent, _ = mustLists(t, repo)
	if len(running) != 0 || len(recent) != 1 || recent[0].EndedAt == nil {
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
