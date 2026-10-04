package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"nexus/internal/store"
)

type testStore struct {
	started []store.Entry
	stopped []int64
	stopAll int
	running []store.Entry
}

func (s *testStore) Start(title, project, desc string) (store.Entry, error) {
	entry := store.Entry{ID: int64(len(s.started) + 1), Title: title, Project: project, Description: desc}
	s.started = append(s.started, entry)
	s.running = append(s.running, entry)
	return entry, nil
}
func (s *testStore) Stop(id int64) error {
	s.stopped = append(s.stopped, id)
	for i, entry := range s.running {
		if entry.ID == id {
			s.running = append(s.running[:i], s.running[i+1:]...)
			break
		}
	}
	return nil
}
func (s *testStore) StopAll() (int, error) {
	s.stopAll = len(s.running)
	s.running = nil
	return s.stopAll, nil
}
func (s *testStore) Running() []store.Entry                { return append([]store.Entry(nil), s.running...) }
func (s *testStore) Recent(int) []store.Entry              { return append([]store.Entry(nil), s.running...) }
func (s *testStore) Totals(time.Time) []store.ProjectTotal { return nil }

func TestModelUpdateStartsAndStopsTimer(t *testing.T) {
	db := &testStore{running: []store.Entry{{ID: 7, Title: "Existing", Project: "Studio"}}}
	m := NewModel(db)

	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if !m.formOpen {
		t.Fatal("n did not open the new timer form")
	}
	if got := m.inputs[1].Value(); got != "Studio" {
		t.Fatalf("project field = %q, want last-used project Studio", got)
	}
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("Draft")}, {Type: tea.KeyTab}, {Type: tea.KeyTab},
		{Type: tea.KeyRunes, Runes: []rune("Ship it")}, {Type: tea.KeyEnter},
	} {
		m = updateKey(t, m, key)
	}
	if len(db.started) != 1 || db.started[0].Title != "Draft" || db.started[0].Project != "Studio" || db.started[0].Description != "Ship it" {
		t.Fatalf("started = %#v, want one complete entry", db.started)
	}
	if m.formOpen {
		t.Fatal("form remained open after submit")
	}

	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if len(db.stopped) != 1 || db.stopped[0] != 7 {
		t.Fatalf("stopped = %v, want selected timer id 7", db.stopped)
	}
}

func updateKey(t *testing.T, m Model, key tea.KeyMsg) Model {
	t.Helper()
	next, _ := m.Update(key)
	return next.(Model)
}
