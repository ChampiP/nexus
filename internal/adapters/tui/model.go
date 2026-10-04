package tui

import (
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"nexus/internal/catalog"
	"nexus/internal/countdown"
	"nexus/internal/tracking"
)

type focusTarget int

const (
	focusTitle focusTarget = iota
	focusProject
	focusDescription
	focusStart
	focusRunningStart // fila de un temporizador activo; el índice está en focusedRunning
	focusToday
	focusWeek
	focusRecent   // fila de recientes; el índice está en focusedRecent
	focusUndo     // botón [Deshacer] de la barra de estado
	focusTabs     // barra superior de pantallas; es el primer elemento del anillo
	focusEditSave // botones del panel de edición
	focusEditDelete
	focusEditCancel
)

// Model contains all state needed to render and update the single-screen interface.
type Model struct {
	tracker         Tracker
	inputs          []textinput.Model
	running         []tracking.Entry
	recent          []tracking.TaskSummary // tareas de RECIENTES, una fila por tarea
	tasks           map[string]tracking.TaskSummary
	totals          []tracking.ProjectTotal
	todaySeconds    int64
	week            bool
	focus           focusTarget
	focusedRunning  int
	focusedRecent   int
	rowButton       int
	edit            *editState
	confirm         *confirmState
	lastDeleted     *tracking.TaskSummary
	undoUntil       time.Time
	pickerOpen      bool
	pickerIndex     int
	pickerScroll    int
	runScroll       int
	width, height   int
	now             time.Time
	message         string
	selectedProject string
	catalog         Catalog
	tree            []catalog.TreeOrganization
	screen          screen
	cat             catalogState
	breaks          Breaks
	brk             *countdown.Break
	bp              breakPage
	wasOverdue      bool
	spin            spinner.Model
	fastActive      bool // hay un tick rápido en vuelo
}

// NewModel creates a ready-to-type model backed by the application tracker.
func NewModel(tracker Tracker, options ...Option) Model {
	m := Model{tracker: tracker, now: time.Now(), spin: newSpinner()}
	for _, option := range options {
		option(&m)
	}
	m.loadCatalog()
	m.initCatalogInput()
	m.initBreakPage()
	m.initInputs()
	m.inputs[0].Focus()
	m.refresh()
	if m.brk != nil && breakOverdue(*m.brk, m.now) {
		// Con el break vencido se abre directo en la pantalla de break, con el foco en [Volver al trabajo].
		m.openBreakPage()
	}
	m.fastActive = m.animating()
	return m
}

const (
	// taskLimit acota las tareas que se piden al caso de uso; recentRows, las que se muestran.
	taskLimit  = 100
	recentRows = 8
)

// workEntries descarta los breaks: la pantalla de Temporizadores solo muestra trabajo.
func workEntries(entries []tracking.Entry) []tracking.Entry {
	work := make([]tracking.Entry, 0, len(entries))
	for _, entry := range entries {
		if entry.Kind != tracking.KindBreak {
			work = append(work, entry)
		}
	}
	return work
}

func (m Model) Init() tea.Cmd {
	if m.fastActive {
		return tea.Batch(tick(), fastTick())
	}
	return tick()
}

func (m *Model) refresh() {
	m.now = time.Now()
	running, seconds, _, err := m.tracker.Snapshot()
	if err == nil {
		m.running, m.todaySeconds = running, seconds
	}
	if tasks, err := m.tracker.RecentTasks(taskLimit); err == nil {
		m.tasks = make(map[string]tracking.TaskSummary, len(tasks))
		for _, task := range tasks {
			m.tasks[task.TaskUID] = task
		}
		m.recent = tasks[:min(len(tasks), recentRows)]
	}
	m.totals, _ = m.tracker.Report(rangeStart(m.now, m.week))
	m.loadCatalog()
	m.loadBreak()
	m.clampFocus()
}

func (m Model) update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case fastTickMsg:
		if !m.animating() {
			m.fastActive = false
			return m, nil
		}
		m.advanceSpinner()
		return m, fastTick()
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tickMsg:
		m.refresh()
		return m, tick()
	case tea.MouseMsg:
		if m.screen == screenCatalog {
			_, cmd := m.updateCatalogMouse(msg)
			return m, cmd
		}
		if m.screen == screenBreak {
			_, cmd := m.updateBreakMouse(msg)
			return m, cmd
		}
		_, cmd := m.updateMouse(msg)
		return m, cmd
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		if m.screen == screenCatalog {
			return m, m.updateCatalogKey(msg)
		}
		if m.screen == screenBreak {
			return m, m.updateBreakKey(msg)
		}
		if msg.Type == tea.KeyCtrlZ && m.confirm == nil {
			m.restoreLast()
			return m, nil
		}
		if msg.Type == tea.KeyEsc {
			switch {
			case m.confirm != nil:
				m.confirm = nil
			case m.pickerOpen:
				m.closeProjectPicker()
			case m.edit != nil:
				m.closeEdit()
			default:
				return m, tea.Quit
			}
			return m, nil
		}
		cmd := m.updateKey(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) updateKey(key tea.KeyMsg) tea.Cmd {
	if m.confirm != nil {
		m.updateConfirm(key)
		return nil
	}
	if m.focus == focusTabs && m.stepScreen(key.Type) {
		return nil
	}
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
	if m.navigate(key) {
		return nil
	}
	if m.focus == focusToday || m.focus == focusWeek {
		// La opción enfocada ya es la elegida; Enter no cambia nada.
		return nil
	}
	if (m.focus == focusRunningStart || m.focus == focusRecent) && key.Type == tea.KeyEnter {
		m.activateRowButton()
		return nil
	}
	if m.edit != nil && m.focus >= focusEditSave {
		if key.Type == tea.KeyEnter {
			m.activateEditButton(m.focus)
		}
		return nil
	}
	if m.focus == focusUndo && key.Type == tea.KeyEnter {
		m.restoreLast()
		return nil
	}
	if key.Type == tea.KeyEnter {
		switch m.focus {
		case focusProject:
			m.openProjectPicker()
			return nil
		case focusTitle, focusDescription, focusStart:
			if m.edit != nil {
				m.saveEdit()
				return nil
			}
			if strings.TrimSpace(m.inputs[0].Value()) == "" {
				m.goTo(m.timersGrid().vertical(m.currentCell(), 1))
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

// navigate aplica las teclas de navegación del modelo único; devuelve true si la tecla se consumió.
// En un campo de texto ←/→ siguen moviendo el cursor salvo que esté vacío o en el borde y haya vecino.
func (m *Model) navigate(key tea.KeyMsg) bool {
	if m.pickerOpen && m.focus == focusProject && key.Type != tea.KeyTab && key.Type != tea.KeyShiftTab {
		return false
	}
	grid := m.timersGrid()
	current := m.currentCell()
	target, ok := grid.move(current, key.Type)
	if !ok {
		return false
	}
	if index := m.inputIndex(); index >= 0 && (key.Type == tea.KeyLeft || key.Type == tea.KeyRight) {
		if target == current || !inputAtEdge(m.inputs[index], key.Type) {
			return false
		}
	}
	m.goTo(target)
	return true
}

// stopSession detiene la sesión en curso id de la tarea title.
func (m *Model) stopSession(id int64, title string) {
	if err := m.tracker.Stop(id); err != nil {
		m.setMessage(errorText("No se pudo detener", err))
	} else {
		m.setMessage("Detenido: " + title)
	}
	m.refresh()
}

func (m *Model) startTimer() {
	title := strings.TrimSpace(m.inputs[0].Value())
	if title == "" {
		m.setMessage("Escribe un título")
		return
	}
	entry, err := m.tracker.Start(tracking.StartInput{Title: title, Project: m.selectedProject, Description: strings.TrimSpace(m.inputs[2].Value())})
	if err != nil {
		m.setMessage(errorText("No se pudo iniciar", err))
		return
	}
	m.inputs[0].SetValue("")
	m.inputs[2].SetValue("")
	m.focus = focusTitle
	m.pickerOpen = false
	m.setMessage("Iniciado: " + entry.Title)
	m.syncInputFocus()
	m.refresh()
}

func (m *Model) cycleProject(delta int) {
	projects := m.pickerProjects("")
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
