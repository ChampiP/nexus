package tracking

import (
	"errors"
	"testing"
	"time"
)

type memoryRepository struct {
	Repository   // los métodos de tareas no se usan en estas pruebas
	entries      []Entry
	projectUsage []ProjectUsage
}

func (r *memoryRepository) Insert(entry Entry) (Entry, error) {
	entry.ID = int64(len(r.entries) + 1)
	r.entries = append(r.entries, entry)
	return entry, nil
}
func (r *memoryRepository) Stop(id, at int64) error {
	for i := range r.entries {
		if r.entries[i].ID == id && r.entries[i].EndedAt == nil {
			r.entries[i].EndedAt = &at
			return nil
		}
	}
	return ErrNotRunning
}
func (r *memoryRepository) StopAll(at int64) (int, error) {
	n := 0
	for i := range r.entries {
		if r.entries[i].EndedAt == nil {
			r.entries[i].EndedAt = &at
			n++
		}
	}
	return n, nil
}
func (r *memoryRepository) Running() ([]Entry, error) {
	var entries []Entry
	for _, e := range r.entries {
		if e.EndedAt == nil {
			entries = append(entries, e)
		}
	}
	return entries, nil
}
func (r *memoryRepository) Recent(limit int) ([]Entry, error) {
	entries := append([]Entry(nil), r.entries...)
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	if limit < len(entries) {
		entries = entries[:limit]
	}
	return entries, nil
}
func (r *memoryRepository) Totals(since, now int64) ([]ProjectTotal, error) {
	return nil, nil
}
func (r *memoryRepository) Projects(now int64) ([]ProjectUsage, error) {
	return append([]ProjectUsage(nil), r.projectUsage...), nil
}

func TestStartTrimsTitleAndRejectsBlank(t *testing.T) {
	repo := &memoryRepository{}
	tracker := NewTracker(repo, func() time.Time { return time.Unix(100, 0) })
	entry, err := tracker.Start(StartInput{Title: "  Report  ", Project: "P"})
	if err != nil || entry.Title != "Report" {
		t.Fatalf("Start() = %+v, %v", entry, err)
	}
	_, err = tracker.Start(StartInput{Title: " \n\t "})
	if !errors.Is(err, ErrEmptyTitle) {
		t.Fatalf("blank Start() error = %v", err)
	}
	if len(repo.entries) != 1 {
		t.Fatalf("inserted %d entries", len(repo.entries))
	}
}

func TestStopRunningAt(t *testing.T) {
	at := time.Unix(200, 500)
	for _, tt := range []struct {
		name    string
		entries []Entry
		stopped int
	}{
		{name: "work before cutoff", entries: []Entry{{ID: 1, Kind: KindWork, Title: "Report", StartedAt: 100}}, stopped: 1},
		{name: "work after cutoff", entries: []Entry{{ID: 1, Kind: KindWork, StartedAt: 201}}},
		{name: "work at cutoff", entries: []Entry{{ID: 1, Kind: KindWork, StartedAt: 200}}},
		{name: "running break", entries: []Entry{{ID: 1, Kind: KindBreak, StartedAt: 100}}},
		{name: "empty"},
		{name: "mixed", entries: []Entry{
			{ID: 1, Kind: KindBreak, StartedAt: 100},
			{ID: 2, Kind: KindWork, StartedAt: 201},
			{ID: 3, Kind: KindWork, StartedAt: 100},
			{ID: 4, Kind: KindWork, StartedAt: 150},
		}, stopped: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &memoryRepository{entries: tt.entries}
			tracker := NewTracker(repo, func() time.Time { return time.Unix(999, 0) })
			got, err := tracker.StopRunningAt(at)
			if err != nil || got == nil || len(got) != tt.stopped {
				t.Fatalf("StopRunningAt() = %+v, %v; want %d stopped", got, err, tt.stopped)
			}
			for _, entry := range got {
				stored, err := repo.Get(entry.ID)
				if err != nil || entry.EndedAt == nil || *entry.EndedAt != at.Unix() || entry != stored {
					t.Fatalf("returned entry = %+v, stored = %+v, error = %v", entry, stored, err)
				}
			}
			for _, entry := range repo.entries {
				shouldStop := entry.Kind == KindWork && entry.StartedAt < at.Unix()
				if shouldStop {
					if entry.EndedAt == nil || *entry.EndedAt != at.Unix() {
						t.Fatalf("eligible entry not stopped at cutoff: %+v", entry)
					}
				} else if entry.EndedAt != nil {
					t.Fatalf("ineligible entry stopped: %+v", entry)
				}
			}
		})
	}
}

type concurrentlyStoppedRepository struct {
	*memoryRepository
}

func (r *concurrentlyStoppedRepository) Stop(id, at int64) error {
	if id == 1 {
		if err := r.memoryRepository.Stop(id, at-1); err != nil {
			return err
		}
	}
	return r.memoryRepository.Stop(id, at)
}

func TestStopRunningAtSkipsConcurrentlyStoppedEntry(t *testing.T) {
	repo := &concurrentlyStoppedRepository{&memoryRepository{entries: []Entry{
		{ID: 1, Kind: KindWork, StartedAt: 100},
		{ID: 2, Kind: KindWork, StartedAt: 100},
	}}}
	tracker := NewTracker(repo, nil)
	got, err := tracker.StopRunningAt(time.Unix(200, 0))
	if err != nil || len(got) != 1 || got[0].ID != 2 || got[0].EndedAt == nil || *got[0].EndedAt != 200 {
		t.Fatalf("StopRunningAt() = %+v, %v", got, err)
	}
	if repo.entries[0].EndedAt == nil || *repo.entries[0].EndedAt != 199 {
		t.Fatalf("concurrently stopped entry overwritten: %+v", repo.entries[0])
	}
}

func TestProjectsSortsAndFiltersInTracker(t *testing.T) {
	repo := &memoryRepository{projectUsage: []ProjectUsage{
		{Name: "Alpha", LastUsed: 20, Seconds: 30},
		{Name: "beta", LastUsed: 40, Seconds: 50},
		{Name: "ALPINE", LastUsed: 40, Seconds: 10},
		{Name: "", LastUsed: 99, Seconds: 1},
	}}
	tracker := NewTracker(repo, func() time.Time { return time.Unix(100, 0) })
	got := tracker.Projects("alp")
	if len(got) != 2 || got[0].Name != "ALPINE" || got[1].Name != "Alpha" {
		t.Fatalf("Projects(\"alp\") = %+v", got)
	}
	all := tracker.Projects("")
	if len(all) != 3 || all[0].Name != "ALPINE" || all[1].Name != "beta" {
		t.Fatalf("Projects(\"\") = %+v", all)
	}
}

func (r *memoryRepository) Get(id int64) (Entry, error) {
	for _, entry := range r.entries {
		if entry.ID == id {
			return entry, nil
		}
	}
	return Entry{}, ErrNotFound
}
func (r *memoryRepository) Update(Entry) error                  { return ErrNotFound }
func (r *memoryRepository) SoftDelete(id, at int64) error       { return ErrNotFound }
func (r *memoryRepository) Restore(id, resumeSince int64) error { return ErrNotFound }
func (r *memoryRepository) Deleted(limit int) ([]Entry, error)  { return nil, nil }
func (r *memoryRepository) Purge(before int64) (int, error)     { return 0, nil }
func (r *memoryRepository) Relink(fromID, toID int64, toName string) error {
	return nil
}
func (r *memoryRepository) RenameProject(id int64, name string) error    { return nil }
func (r *memoryRepository) CountByProject(projectID int64) (int, error)  { return 0, nil }
func (r *memoryRepository) BreakSeconds(since, now int64) (int64, error) { return 0, nil }
func (r *memoryRepository) BreaksSince(since int64) ([]Entry, error)     { return nil, nil }
