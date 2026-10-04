package tui

import (
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"nexus/internal/app"
	"nexus/internal/domain"
)

type focusTarget int

const (
	focusTitle focusTarget = iota
	focusProject
	focusDescription
	focusStart
	focusRunningStart
	focusToday
	focusWeek
)

// Model contains all state needed to render and update the single-screen interface.
type Model struct {
	tracker         Tracker
	inputs          []textinput.Model
	running         []domain.Entry
	recent          []domain.Entry
	totals          []domain.ProjectTotal
	todaySeconds    int64
	week            bool
	focus           focusTarget
	focusedRunning  int
	pickerOpen      bool
	pickerIndex     int
	pickerScroll    int
	runScroll       int
	width, height   int
	now             time.Time
	message         string
	selectedProject string
}

// NewModel creates a ready-to-type model backed by the application tracker.
func NewModel(tracker Tracker) Model {
	m := Model{tracker: tracker, now: time.Now()}
	m.initInputs()
	m.inputs[0].Focus()
	m.refresh()
	return m
}

func (m Model) Init() tea.Cmd { return tick() }

func (m *Model) refresh() {
	m.now = time.Now()
	running, seconds, recent, err := m.tracker.Snapshot()
	if err == nil {
		m.running, m.todaySeconds, m.recent = running, seconds, recent
		if len(m.recent) > 8 {
			m.recent = m.recent[:8]
		}
	}
	m.totals, _ = m.tracker.Report(rangeStart(m.now, m.week))
	if m.focusedRunning >= len(m.running) {
		m.focusedRunning = len(m.running) - 1
	}
	if m.focusedRunning < 0 {
		m.focusedRunning = 0
	}
	if m.focus >= focusRunningStart && m.focus < focusToday && int(m.focus-focusRunningStart) >= len(m.running) {
		m.focus = focusToday
	}
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tickMsg:
		m.refresh()
		return m, tick()
	case tea.MouseMsg:
		return m.updateMouse(msg)
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		if msg.Type == tea.KeyEsc {
			if m.pickerOpen {
				m.closeProjectPicker()
				return m, nil
			}
			return m, tea.Quit
		}
		cmd := m.updateKey(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) updateKey(key tea.KeyMsg) tea.Cmd {
	if m.focus == focusProject && !m.pickerOpen {
		switch key.Type {
		case tea.KeyLeft:
			m.cycleProject(-1)
			return nil
		case tea.KeyRight:
			m.cycleProject(1)
			return nil
		}
	}
	if m.pickerOpen && m.focus == focusProject {
		switch key.Type {
		case tea.KeyUp:
			m.movePicker(-1)
			return nil
		case tea.KeyDown:
			m.movePicker(1)
			return nil
		case tea.KeyEnter:
			m.selectProject()
			return nil
		}
	}
	if key.Type == tea.KeyTab || key.Type == tea.KeyShiftTab {
		step := 1
		if key.Type == tea.KeyShiftTab {
			step = -1
		}
		m.moveFocus(step)
		return nil
	}
	if m.focus == focusToday || m.focus == focusWeek {
		switch key.Type {
		case tea.KeyLeft, tea.KeyRight, tea.KeyEnter:
			m.week = !m.week
			m.refresh()
			return nil
		case tea.KeyUp:
			m.moveFocus(-1)
			return nil
		case tea.KeyDown:
			m.moveFocus(1)
			return nil
		}
	}
	if m.focus >= focusRunningStart {
		if key.Type == tea.KeyUp {
			m.moveFocus(-1)
			return nil
		}
		if key.Type == tea.KeyDown {
			m.moveFocus(1)
			return nil
		}
		if key.Type == tea.KeyEnter && m.focus < focusToday {
			m.stopFocused()
			return nil
		}
	}
	if key.Type == tea.KeyUp {
		m.moveFocus(-1)
		return nil
	}
	if key.Type == tea.KeyDown {
		m.moveFocus(1)
		return nil
	}
	if key.Type == tea.KeyEnter {
		switch m.focus {
		case focusProject:
			m.openProjectPicker()
			return nil
		case focusTitle, focusDescription, focusStart:
			if strings.TrimSpace(m.inputs[0].Value()) == "" {
				m.moveFocus(1)
			} else {
				m.startTimer()
			}
			return nil
		}
	}
	if m.focus == focusProject && !m.pickerOpen && (key.Type == tea.KeyBackspace || key.Type == tea.KeyDelete) {
		return nil
	}
	if m.focus == focusProject && (key.Type == tea.KeyRunes || key.Type == tea.KeySpace || key.Type == tea.KeyBackspace || key.Type == tea.KeyDelete) {
		if !m.pickerOpen {
			m.openProjectPicker()
		}
		var cmd tea.Cmd
		m.inputs[1], cmd = m.inputs[1].Update(key)
		m.pickerOpen, m.pickerIndex = true, 0
		return cmd
	}
	index := m.inputIndex()
	if index >= 0 {
		var cmd tea.Cmd
		m.inputs[index], cmd = m.inputs[index].Update(key)
		return cmd
	}
	return nil
}

func (m *Model) moveFocus(step int) {
	targets := []focusTarget{focusTitle, focusProject, focusDescription, focusStart}
	for i := range m.running {
		targets = append(targets, focusTarget(int(focusRunningStart)+i))
	}
	targets = append(targets, focusToday, focusWeek)
	at := 0
	for i, target := range targets {
		if target == m.focus {
			at = i
			break
		}
	}
	at += step
	if at < 0 {
		at = 0
	}
	if at >= len(targets) {
		at = len(targets) - 1
	}
	previous := m.focus
	m.focus = targets[at]
	if m.focus == focusProject {
		m.closeProjectPicker()
	} else {
		if previous == focusProject {
			m.closeProjectPicker()
		}
		m.syncInputFocus()
	}
}

func (m *Model) stopFocused() {
	index := int(m.focus) - int(focusRunningStart)
	if index < 0 || index >= len(m.running) {
		return
	}
	entry := m.running[index]
	if err := m.tracker.Stop(entry.ID); err != nil {
		m.message = "No se pudo detener: " + err.Error()
	} else {
		m.message = "Detenido: " + entry.Title
	}
	m.refresh()
}

func (m *Model) startTimer() {
	title := strings.TrimSpace(m.inputs[0].Value())
	if title == "" {
		m.message = "Escribe un título"
		return
	}
	entry, err := m.tracker.Start(app.StartInput{Title: title, Project: m.selectedProject, Description: strings.TrimSpace(m.inputs[2].Value())})
	if err != nil {
		m.message = "No se pudo iniciar: " + err.Error()
		return
	}
	m.inputs[0].SetValue("")
	m.inputs[2].SetValue("")
	m.focus = focusTitle
	m.pickerOpen = false
	m.message = "Iniciado: " + entry.Title
	m.syncInputFocus()
	m.refresh()
}

func (m *Model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	layout := m.computeLayout()
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
		for i, zone := range layout.fields {
			if zone.contains(msg.X, msg.Y) {
				m.focus = focusTarget(i)
				if m.focus == focusProject {
					m.openProjectPicker()
				} else {
					m.closeProjectPicker()
					m.syncInputFocus()
				}
				return m, nil
			}
		}
		if layout.start.contains(msg.X, msg.Y) {
			m.focus = focusStart
			m.startTimer()
			return m, nil
		}
		for i, row := range layout.running {
			runningIndex := m.runScroll + i
			if row.stop.contains(msg.X, msg.Y) {
				m.focus = focusTarget(int(focusRunningStart) + runningIndex)
				m.stopFocused()
				return m, nil
			}
			if row.row.contains(msg.X, msg.Y) {
				m.focus = focusTarget(int(focusRunningStart) + runningIndex)
				m.syncInputFocus()
				return m, nil
			}
		}
		for i, zone := range layout.options {
			if zone.contains(msg.X, msg.Y) {
				m.pickerIndex = layout.optionsStart + i
				m.selectProject()
				return m, nil
			}
		}
		if layout.today.contains(msg.X, msg.Y) {
			m.week = false
			m.focus = focusToday
			m.refresh()
			return m, nil
		}
		if layout.week.contains(msg.X, msg.Y) {
			m.week = true
			m.focus = focusWeek
			m.refresh()
			return m, nil
		}
	}
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonWheelUp {
		if m.pickerOpen {
			m.movePicker(-1)
		} else if len(m.running) > 0 && m.runScroll > 0 {
			m.runScroll--
		}
		return m, nil
	}
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonWheelDown {
		if m.pickerOpen {
			m.movePicker(1)
		} else if m.runScroll+1 < len(m.running) {
			m.runScroll++
		}
		return m, nil
	}
	return m, nil
}

func (m *Model) cycleProject(delta int) {
	projects := m.tracker.Projects("")
	choices := make([]string, 1, len(projects)+1)
	for _, project := range projects {
		choices = append(choices, project.Name)
	}
	index := -1
	for i, project := range choices {
		if strings.EqualFold(project, m.selectedProject) {
			index = i
			break
		}
	}
	index = (index + delta + len(choices)) % len(choices)
	m.selectedProject = choices[index]
	m.inputs[1].SetValue(m.selectedProject)
}

func (m *Model) movePicker(delta int) {
	options := m.projectOptions()
	if len(options) == 0 {
		return
	}
	m.pickerIndex += delta
	if m.pickerIndex < 0 {
		m.pickerIndex = 0
	}
	if m.pickerIndex >= len(options) {
		m.pickerIndex = len(options) - 1
	}
}

func (m *Model) focusInput() { m.syncInputFocus() }

func (m *Model) syncInputFocus() {
	for i := range m.inputs {
		if m.inputIndex() == i {
			m.inputs[i].Focus()
		} else {
			m.inputs[i].Blur()
		}
	}
}

func (m Model) inputIndex() int {
	switch m.focus {
	case focusTitle:
		return 0
	case focusProject:
		if m.pickerOpen {
			return 1
		}
	case focusDescription:
		return 2
	}
	return -1
}

func (m *Model) refreshTotalsOrder() {
	sort.Slice(m.totals, func(i, j int) bool { return m.totals[i].Seconds > m.totals[j].Seconds })
}
