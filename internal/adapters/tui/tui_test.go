package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"nexus/internal/tracking"
)

type testStore struct {
	started  []tracking.Entry
	stopped  []int64
	running  []tracking.Entry
	projects []tracking.ProjectUsage
	recent   []tracking.Entry
	trash    []tracking.Entry
	edits    []editCall
	deleted  []int64
	restored []int64
	editErr  error
}

type editCall struct {
	id    int64
	input tracking.EditInput
}

func removeEntry(entries []tracking.Entry, id int64) ([]tracking.Entry, *tracking.Entry) {
	for i, entry := range entries {
		if entry.ID == id {
			found := entry
			return append(entries[:i:i], entries[i+1:]...), &found
		}
	}
	return entries, nil
}

func (s *testStore) Edit(id int64, input tracking.EditInput) (tracking.Entry, error) {
	s.edits = append(s.edits, editCall{id, input})
	if s.editErr != nil {
		return tracking.Entry{}, s.editErr
	}
	var updated tracking.Entry
	for _, list := range []*[]tracking.Entry{&s.running, &s.recent} {
		for i := range *list {
			if (*list)[i].ID != id {
				continue
			}
			if input.Title != nil {
				(*list)[i].Title = *input.Title
			}
			if input.Project != nil {
				(*list)[i].Project = *input.Project
			}
			if input.Description != nil {
				(*list)[i].Description = *input.Description
			}
			updated = (*list)[i]
		}
	}
	return updated, nil
}

func (s *testStore) Delete(id int64) error {
	s.deleted = append(s.deleted, id)
	var gone *tracking.Entry
	s.running, gone = removeEntry(s.running, id)
	var inRecent *tracking.Entry
	s.recent, inRecent = removeEntry(s.recent, id)
	if inRecent != nil {
		gone = inRecent
	}
	if gone != nil {
		s.trash = append(s.trash, *gone)
	}
	return nil
}

func (s *testStore) Restore(id int64) error {
	s.restored = append(s.restored, id)
	var back *tracking.Entry
	s.trash, back = removeEntry(s.trash, id)
	if back == nil {
		return tracking.ErrNotFound
	}
	s.recent = append(s.recent, *back)
	if back.EndedAt == nil {
		s.running = append(s.running, *back)
	}
	return nil
}

func (s *testStore) Start(input tracking.StartInput) (tracking.Entry, error) {
	entry := tracking.Entry{ID: int64(len(s.started) + 1), Title: input.Title, Project: input.Project, Description: input.Description}
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
func (s *testStore) Snapshot() ([]tracking.Entry, int64, []tracking.Entry, error) {
	return append([]tracking.Entry(nil), s.running...), 0, append([]tracking.Entry(nil), s.recent...), nil
}
func (s *testStore) Projects(query string) []tracking.ProjectUsage {
	var matches []tracking.ProjectUsage
	for _, project := range s.projects {
		if strings.Contains(strings.ToLower(project.Name), strings.ToLower(query)) {
			matches = append(matches, project)
		}
	}
	return matches
}
func (s *testStore) Report(time.Time) ([]tracking.ProjectTotal, error) { return nil, nil }

func TestEnterStartsFromTitleAndResetsForm(t *testing.T) {
	db := &testStore{projects: []tracking.ProjectUsage{{Name: "Studio"}}}
	m := NewModel(db)
	m.inputs[0].SetValue("Draft")
	m.inputs[1].SetValue("Studio")
	m.inputs[2].SetValue("Ship it")
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(db.started) != 1 || db.started[0].Title != "Draft" || db.started[0].Project != "Studio" || db.started[0].Description != "Ship it" {
		t.Fatalf("started = %#v", db.started)
	}
	if m.focus != focusTitle || m.inputs[0].Value() != "" || m.inputs[2].Value() != "" {
		t.Fatalf("form not reset to title focus: focus=%v title=%q description=%q", m.focus, m.inputs[0].Value(), m.inputs[2].Value())
	}
}

func TestArrowFocusRingAndBlankEnter(t *testing.T) {
	m := NewModel(&testStore{})
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.focus != focusProject {
		t.Fatalf("blank title Enter focus = %v, want project", m.focus)
	}
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.focus != focusDescription {
		t.Fatalf("focus = %v, want description", m.focus)
	}
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.focus != focusStart {
		t.Fatalf("focus = %v, want start", m.focus)
	}
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.focus != focusToday {
		t.Fatalf("focus = %v, want today tab", m.focus)
	}
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if m.focus != focusStart {
		t.Fatalf("focus = %v, want start", m.focus)
	}
}

func TestProjectFocusShowsAllProjectsAndMarksCurrent(t *testing.T) {
	db := &testStore{projects: []tracking.ProjectUsage{{Name: "nexus", Seconds: 500}, {Name: "tbwa", Seconds: 300}}}
	m := NewModel(db)
	if got := m.inputs[1].Value(); got != "nexus" {
		t.Fatalf("initial project = %q, want nexus", got)
	}
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.pickerOpen {
		t.Fatal("Project focus should arrive with dropdown closed")
	}
	m.width = 100
	if !strings.Contains(m.View(), "‹ nexus ›") {
		t.Fatalf("inline project value missing from view: %q", m.View())
	}
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.pickerOpen || m.inputs[1].Value() != "" {
		t.Fatal("Enter should open the empty-query project list")
	}
	options := m.projectOptions()
	if len(options) != 3 || options[1].name != "nexus" || options[2].name != "tbwa" {
		t.Fatalf("empty-query options = %#v, want all projects", options)
	}
	if m.pickerIndex != 1 {
		t.Fatalf("highlight index = %d, want selected nexus option at 1", m.pickerIndex)
	}
	if label := renderProjectOption(options[m.pickerIndex], true); !strings.Contains(label, "▸") || !strings.Contains(label, "✓") {
		t.Fatalf("selected project option lacks visible markers: %q", label)
	}
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.inputs[1].Value() != "nexus" || m.focus != focusTitle {
		t.Fatalf("empty-query Enter changed project or focus: project=%q focus=%v", m.inputs[1].Value(), m.focus)
	}
}

func TestProjectArrowNavigationAndCycleWraps(t *testing.T) {
	db := &testStore{projects: []tracking.ProjectUsage{{Name: "nexus"}, {Name: "tbwa"}}}
	m := NewModel(db)
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != focusProject || m.pickerOpen {
		t.Fatalf("project focus state: focus=%v open=%v", m.focus, m.pickerOpen)
	}
	if !strings.Contains(m.hint(), "←→ cambiar proyecto · Enter ver lista · escribe para buscar · ↑↓ mover") {
		t.Fatalf("State A footer hint is incorrect: %q", m.hint())
	}
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if m.focus != focusTitle {
		t.Fatalf("up from Project should focus Title, got %v", m.focus)
	}
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.focus != focusProject || m.pickerOpen {
		t.Fatalf("down from Title should re-enter State A, got focus=%v open=%v", m.focus, m.pickerOpen)
	}
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyLeft})
	if m.selectedProject != "" {
		t.Fatalf("left should wrap nexus to Sin proyecto, got %q", m.selectedProject)
	}
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyLeft})
	if m.selectedProject != "tbwa" {
		t.Fatalf("left should wrap Sin proyecto to last project, got %q", m.selectedProject)
	}
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyRight})
	if m.selectedProject != "" {
		t.Fatalf("right should wrap last project to Sin proyecto, got %q", m.selectedProject)
	}
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyRight})
	if m.selectedProject != "nexus" {
		t.Fatalf("right should cycle to next project, got %q", m.selectedProject)
	}
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.focus != focusDescription || m.pickerOpen {
		t.Fatalf("down should focus description, got focus=%v open=%v", m.focus, m.pickerOpen)
	}
}

func TestProjectTypingReplacesPrefillAndStartUsesSelection(t *testing.T) {
	db := &testStore{projects: []tracking.ProjectUsage{{Name: "nexus", Seconds: 500}, {Name: "tbwa", Seconds: 300}}}
	m := NewModel(db)
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("tb")})
	options := m.projectOptions()
	var matches []string
	for _, option := range options {
		if !option.create {
			matches = append(matches, option.name)
		}
	}
	if len(matches) != 1 || matches[0] != "tbwa" {
		t.Fatalf("filtered project matches = %v, want [tbwa]", matches)
	}
	if got := m.inputs[1].Value(); got != "tb" {
		t.Fatalf("query = %q, want tb (not appended to nexus)", got)
	}
	if m.pickerIndex != 0 || options[0].name != "tbwa" || !options[len(options)-1].create {
		t.Fatalf("existing match should be first/highlighted and create last: index=%d options=%#v", m.pickerIndex, options)
	}
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if got := m.inputs[1].Value(); got != "tbwa" || m.focus != focusTitle {
		t.Fatalf("selected project=%q focus=%v", got, m.focus)
	}
	m.inputs[0].SetValue("Capture")
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(db.started) != 1 || db.started[0].Project != "tbwa" {
		t.Fatalf("started = %#v, want project tbwa", db.started)
	}
}

func TestProjectPickerEscEnterAndMouseTransitions(t *testing.T) {
	db := &testStore{projects: []tracking.ProjectUsage{{Name: "nexus"}, {Name: "tbwa"}}}
	m := NewModel(db)
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.pickerOpen || m.pickerIndex != 1 {
		t.Fatalf("Enter should open and highlight current project: open=%v index=%d", m.pickerOpen, m.pickerIndex)
	}
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.pickerOpen || m.focus != focusTitle || m.selectedProject != "nexus" {
		t.Fatalf("Enter selection state: open=%v focus=%v project=%q", m.pickerOpen, m.focus, m.selectedProject)
	}
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("tb")})
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.pickerOpen || m.focus != focusProject || m.selectedProject != "nexus" {
		t.Fatalf("Esc should close without changing project: open=%v focus=%v project=%q", m.pickerOpen, m.focus, m.selectedProject)
	}
	m.width, m.height = 100, 30
	field := m.computeLayout().fields[1]
	m = updateMouse(t, m, tea.MouseMsg{X: field.x, Y: field.y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if !m.pickerOpen || m.focus != focusProject || m.inputs[1].Value() != "" {
		t.Fatalf("click should open full picker: open=%v focus=%v query=%q", m.pickerOpen, m.focus, m.inputs[1].Value())
	}
}

func TestProjectPickerCreateOrderingAndExactMatch(t *testing.T) {
	db := &testStore{projects: []tracking.ProjectUsage{{Name: "tbwa"}, {Name: "toolbox"}}}
	m := NewModel(db)
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("tb")})
	options := m.projectOptions()
	if len(options) != 2 || options[0].name != "tbwa" || options[0].create || !options[len(options)-1].create {
		t.Fatalf("matching projects must precede create option: %#v", options)
	}
	m.inputs[1].SetValue("TBWA")
	options = m.projectOptions()
	if len(options) != 1 || options[0].create || options[0].name != "tbwa" {
		t.Fatalf("exact case-insensitive match must not offer create: %#v", options)
	}
	m.inputs[1].SetValue("missing")
	options = m.projectOptions()
	if len(options) != 1 || !options[0].create || options[0].name != "missing" {
		t.Fatalf("no match should offer create as the only option: %#v", options)
	}
}

func TestProjectPickerTabKeepsConfirmedProjectAndMovesToDescription(t *testing.T) {
	db := &testStore{projects: []tracking.ProjectUsage{{Name: "nexus"}, {Name: "tbwa"}}}
	m := NewModel(db)
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("tb")})
	if !strings.Contains(m.hint(), "↑↓ elegir · Enter seleccionar · Esc cerrar") {
		t.Fatalf("State B footer hint is incorrect: %q", m.hint())
	}
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != focusDescription || m.selectedProject != "nexus" || m.inputs[1].Value() != "nexus" {
		t.Fatalf("Tab focus=%v project=%q field=%q; want description and confirmed nexus", m.focus, m.selectedProject, m.inputs[1].Value())
	}
}

func TestProjectPickerFiltersSelectsAndCreates(t *testing.T) {
	db := &testStore{projects: []tracking.ProjectUsage{{Name: "Studio", Seconds: 3600}, {Name: "Study", Seconds: 90}}}
	m := NewModel(db)
	m.focus = focusProject
	m.focusInput()
	m.inputs[1].SetValue("")
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Stu")})
	if got := m.projectOptions(); len(got) != 3 || got[0].name != "Studio" || !got[len(got)-1].create {
		t.Fatalf("filtered options = %#v", got)
	}
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.inputs[1].Value() != "Studio" || m.focus != focusTitle {
		t.Fatalf("selected project=%q focus=%v", m.inputs[1].Value(), m.focus)
	}
	m.focus = focusProject
	m.inputs[1].SetValue("New project")
	m.pickerOpen = true
	m.pickerIndex = 0
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.inputs[1].Value() != "New project" || m.focus != focusTitle {
		t.Fatalf("create selection project=%q focus=%v", m.inputs[1].Value(), m.focus)
	}
}

func TestEnterStopsFocusedTimerAndTabToggles(t *testing.T) {
	db := &testStore{running: []tracking.Entry{{ID: 7, Title: "Existing"}}}
	m := NewModel(db)
	m.focus = focusRunningStart
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyRight})
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(db.stopped) != 1 || db.stopped[0] != 7 {
		t.Fatalf("stopped = %v", db.stopped)
	}
	m.focus = focusToday
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyRight})
	if !m.week {
		t.Fatal("right arrow did not select Semana")
	}
}

func TestMouseClickStopUsesRenderedLayout(t *testing.T) {
	db := &testStore{running: []tracking.Entry{{ID: 42, Title: "Timer"}}}
	m := NewModel(db)
	m.width, m.height = 110, 34
	layout := m.computeLayout()
	stop := layout.running[0].buttons[1]
	m = updateMouse(t, m, tea.MouseMsg{X: stop.x, Y: stop.y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if len(db.stopped) != 1 || db.stopped[0] != 42 {
		t.Fatalf("click stopped = %v", db.stopped)
	}
}

func updateKey(t *testing.T, m Model, key tea.KeyMsg) Model {
	t.Helper()
	next, _ := m.Update(key)
	return next.(Model)
}
func updateMouse(t *testing.T, m Model, mouse tea.MouseMsg) Model {
	t.Helper()
	next, _ := m.Update(mouse)
	switch model := next.(type) {
	case Model:
		return model
	case *Model:
		return *model
	default:
		t.Fatalf("unexpected model type %T", next)
		return m
	}
}

func TestFormatDurationShowsSeconds(t *testing.T) {
	cases := map[int64]string{-5: "0:00:00", 22: "0:00:22", 3725: "1:02:05"}
	for seconds, want := range cases {
		if got := formatDuration(seconds); got != want {
			t.Errorf("formatDuration(%d) = %q, want %q", seconds, got, want)
		}
	}
}

func lipglossWidth(s string) int { return lipgloss.Width(s) }
