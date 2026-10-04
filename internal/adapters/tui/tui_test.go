package tui

import (
	"fmt"
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
	// deleted y restored son las tareas (task_uid) eliminadas y restauradas.
	deleted  []string
	restored []string
	// resumed son los ids que se reanudaron con Resume.
	resumed []int64
	editErr error
	totals  []tracking.ProjectTotal
	counts  map[int64]int
	// breaks son los breaks de hoy que informa el fake.
	breaks []tracking.Entry
}

type editCall struct {
	task  string
	input tracking.EditInput
}

// taskOf es la tarea de una entrada del fake; sin TaskUID cada entrada es su propia tarea.
func taskOf(e tracking.Entry) string {
	if e.TaskUID != "" {
		return e.TaskUID
	}
	return fmt.Sprintf("t%d", e.ID)
}

// takeTask separa de entries las sesiones de la tarea y devuelve el resto y las separadas.
func takeTask(entries []tracking.Entry, task string) (rest, taken []tracking.Entry) {
	for _, e := range entries {
		if taskOf(e) == task {
			taken = append(taken, e)
		} else {
			rest = append(rest, e)
		}
	}
	return rest, taken
}

// allEntries une en curso y recientes sin repetir ids; las sesiones en curso van primero.
func (s *testStore) allEntries() []tracking.Entry {
	seen := map[int64]bool{}
	var all []tracking.Entry
	for _, list := range [][]tracking.Entry{s.running, s.recent} {
		for _, e := range list {
			if !seen[e.ID] && e.Kind != tracking.KindBreak {
				seen[e.ID] = true
				all = append(all, e)
			}
		}
	}
	return all
}

func (s *testStore) RecentTasks(limit int) ([]tracking.TaskSummary, error) {
	now := time.Now().Unix()
	byTask := map[string]*tracking.TaskSummary{}
	var order []*tracking.TaskSummary
	for _, e := range s.allEntries() {
		key := taskOf(e)
		task := byTask[key]
		if task == nil {
			task = &tracking.TaskSummary{TaskUID: key, Title: e.Title, Project: e.Project, Description: e.Description, LastEntryID: e.ID}
			byTask[key] = task
			order = append(order, task)
		}
		task.SessionCount++
		end := now
		if e.EndedAt != nil {
			end = *e.EndedAt
		} else {
			task.Running, task.RunningEntryID, task.LastEntryID = true, e.ID, e.ID
		}
		task.TotalSeconds += end - e.StartedAt
		task.LastActivity = max(task.LastActivity, end)
	}
	var tasks []tracking.TaskSummary
	for _, task := range order {
		tasks = append(tasks, *task)
	}
	if len(tasks) > limit {
		tasks = tasks[:limit]
	}
	return tasks, nil
}

func (s *testStore) EditTask(task string, input tracking.EditInput) (tracking.Entry, error) {
	s.edits = append(s.edits, editCall{task, input})
	if s.editErr != nil {
		return tracking.Entry{}, s.editErr
	}
	var updated tracking.Entry
	for _, list := range []*[]tracking.Entry{&s.running, &s.recent} {
		for i := range *list {
			if taskOf((*list)[i]) != task {
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

func (s *testStore) DeleteTask(task string) error {
	s.deleted = append(s.deleted, task)
	var fromRunning, fromRecent []tracking.Entry
	s.running, fromRunning = takeTask(s.running, task)
	s.recent, fromRecent = takeTask(s.recent, task)
	s.trash = append(s.trash, fromRecent...)
	for _, e := range fromRunning {
		if !containsID(fromRecent, e.ID) {
			s.trash = append(s.trash, e)
		}
	}
	return nil
}

func containsID(entries []tracking.Entry, id int64) bool {
	for _, e := range entries {
		if e.ID == id {
			return true
		}
	}
	return false
}

func (s *testStore) RestoreTask(task string) error {
	s.restored = append(s.restored, task)
	var back []tracking.Entry
	s.trash, back = takeTask(s.trash, task)
	if len(back) == 0 {
		return tracking.ErrNotFound
	}
	for _, e := range back {
		s.recent = append(s.recent, e)
		if e.EndedAt == nil {
			s.running = append(s.running, e)
		}
	}
	return nil
}

func (s *testStore) Start(input tracking.StartInput) (tracking.Entry, error) {
	entry := tracking.Entry{ID: int64(len(s.started) + 1), Title: input.Title, Project: input.Project, Description: input.Description}
	s.started = append(s.started, entry)
	s.running = append(s.running, entry)
	return entry, nil
}

// Resume imita al Tracker: nueva sesión de la misma tarea, salvo que ya tenga una en curso.
func (s *testStore) Resume(id int64) (tracking.Entry, error) {
	src, err := s.Get(id)
	if err != nil {
		return tracking.Entry{}, err
	}
	for _, e := range s.running {
		if taskOf(e) == taskOf(src) {
			return e, tracking.ErrAlreadyRunning
		}
	}
	s.resumed = append(s.resumed, id)
	entry := tracking.Entry{ID: 100 + int64(len(s.resumed)), TaskUID: taskOf(src), Title: src.Title, Project: src.Project, Description: src.Description, StartedAt: time.Now().Unix()}
	s.running = append(s.running, entry)
	return entry, nil
}
func (s *testStore) Stop(id int64) error {
	s.stopped = append(s.stopped, id)
	end := time.Now().Unix()
	for i, entry := range s.running {
		if entry.ID == id {
			s.running = append(s.running[:i], s.running[i+1:]...)
			entry.EndedAt = &end
			s.recent = append([]tracking.Entry{entry}, removeID(s.recent, id)...)
			break
		}
	}
	return nil
}

func removeID(entries []tracking.Entry, id int64) []tracking.Entry {
	var out []tracking.Entry
	for _, e := range entries {
		if e.ID != id {
			out = append(out, e)
		}
	}
	return out
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
func (s *testStore) Report(time.Time) ([]tracking.ProjectTotal, error) { return s.totals, nil }
func (s *testStore) BreaksSince(time.Time) ([]tracking.Entry, error)   { return s.breaks, nil }
func (s *testStore) Get(id int64) (tracking.Entry, error) {
	for _, list := range [][]tracking.Entry{s.running, s.recent, s.trash} {
		for _, entry := range list {
			if entry.ID == id {
				return entry, nil
			}
		}
	}
	return tracking.Entry{}, tracking.ErrNotFound
}
func (s *testStore) CountByProject(id int64) (int, error) { return s.counts[id], nil }

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
