package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"nexus/internal/app"
	"nexus/internal/domain"
)

// Tracker exposes the use cases required by the terminal interface.
type Tracker interface {
	Start(app.StartInput) (domain.Entry, error)
	Stop(id int64) error
	StopAll() (int, error)
	Snapshot() ([]domain.Entry, int64, []domain.Entry, error)
	Report(time.Time) ([]domain.ProjectTotal, error)
}

type tickMsg time.Time

// Model is the Bubble Tea model for the existing Nexus terminal interface.
type Model struct {
	db           Tracker
	running      []domain.Entry
	recent       []domain.Entry
	totals       []domain.ProjectTotal
	todaySeconds int64
	lastProject  string
	selected     int
	week         bool
	formOpen     bool
	inputs       []textinput.Model
	focus        int
	width        int
	height       int
	now          time.Time
	message      string
}

var (
	accent = lipgloss.AdaptiveColor{Light: "#5B4BFF", Dark: "#A99CFF"}
	muted  = lipgloss.AdaptiveColor{Light: "#667085", Dark: "#A0A5B2"}
	panel  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.AdaptiveColor{Light: "#D8D7E8", Dark: "#414356"}).Padding(0, 1)
	title  = lipgloss.NewStyle().Bold(true).Foreground(accent)
	label  = lipgloss.NewStyle().Foreground(muted)
)

// NewModel creates a terminal model backed by the application tracker.
func NewModel(db Tracker) Model {
	m := Model{db: db, now: time.Now()}
	for i, placeholder := range []string{"Escribe un título", "Nombre del proyecto", "Descripción opcional"} {
		input := textinput.New()
		input.Placeholder = placeholder
		input.CharLimit = 120
		input.Width = 34
		if i == 0 {
			input.Focus()
		} else {
			input.Blur()
		}
		m.inputs = append(m.inputs, input)
	}
	m.refresh()
	return m
}

// Run starts the interactive terminal interface.
func Run(db Tracker) error {
	_, err := tea.NewProgram(NewModel(db), tea.WithAltScreen()).Run()
	return err
}

// Init starts the model's periodic refresh.
func (m Model) Init() tea.Cmd { return tick() }

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(now time.Time) tea.Msg { return tickMsg(now) })
}

func (m *Model) refresh() {
	m.now = time.Now()
	running, todaySeconds, recent, err := m.db.Snapshot()
	if err == nil {
		m.running = running
		m.todaySeconds = todaySeconds
		m.recent = recent
		if len(m.recent) > 8 {
			m.recent = m.recent[:8]
		}
	}
	m.totals, _ = m.db.Report(rangeStart(m.now, m.week))
	if len(m.recent) > 0 {
		m.lastProject = m.recent[0].Project
	}
	if m.selected >= len(m.running) {
		m.selected = len(m.running) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
}

// Update applies a Bubble Tea message to the model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tickMsg:
		m.refresh()
		return m, tick()
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC || (!m.formOpen && msg.Type == tea.KeyRunes && string(msg.Runes) == "q") {
			return m, tea.Quit
		}
		if m.formOpen {
			return m.updateForm(msg)
		}
		switch msg.Type {
		case tea.KeyUp:
			m.moveSelection(-1)
		case tea.KeyDown:
			m.moveSelection(1)
		case tea.KeyRunes:
			switch string(msg.Runes) {
			case "k":
				m.moveSelection(-1)
			case "j":
				m.moveSelection(1)
			case "n":
				m.openForm()
			case "x":
				m.stopSelected()
			case "X":
				if _, err := m.db.StopAll(); err != nil {
					m.message = "No se pudieron detener los temporizadores: " + err.Error()
				} else {
					m.message = "Todos los temporizadores detenidos"
				}
				m.refresh()
			case "r":
				m.week = !m.week
				m.refresh()
			}
		}
	}
	return m, nil
}

func (m Model) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.formOpen = false
		m.message = ""
		return m, nil
	case tea.KeyTab, tea.KeyShiftTab:
		step := 1
		if msg.Type == tea.KeyShiftTab {
			step = -1
		}
		m.focus = (m.focus + step + len(m.inputs)) % len(m.inputs)
		m.focusInput()
		return m, nil
	case tea.KeyEnter:
		if m.focus < len(m.inputs)-1 {
			m.focus++
			m.focusInput()
			return m, nil
		}
		title := strings.TrimSpace(m.inputs[0].Value())
		if title == "" {
			m.message = "El título es obligatorio"
			return m, nil
		}
		entry, err := m.db.Start(app.StartInput{Title: title, Project: strings.TrimSpace(m.inputs[1].Value()), Description: strings.TrimSpace(m.inputs[2].Value())})
		if err != nil {
			m.message = "No se pudo iniciar: " + err.Error()
			return m, nil
		}
		m.formOpen = false
		m.message = "Temporizador iniciado: " + entry.Title
		m.refresh()
		return m, nil
	}
	var cmd tea.Cmd
	m.inputs[m.focus], cmd = m.inputs[m.focus].Update(msg)
	return m, cmd
}

func (m *Model) focusInput() {
	for i := range m.inputs {
		if i == m.focus {
			m.inputs[i].Focus()
		} else {
			m.inputs[i].Blur()
		}
	}
}

func (m *Model) openForm() {
	m.formOpen = true
	m.focus = 0
	m.inputs[0].SetValue("")
	m.inputs[1].SetValue(m.lastProject)
	m.inputs[2].SetValue("")
	m.focusInput()
	m.message = ""
}

func (m *Model) moveSelection(delta int) {
	if len(m.running) == 0 {
		return
	}
	m.selected = (m.selected + delta + len(m.running)) % len(m.running)
}

func (m *Model) stopSelected() {
	if len(m.running) == 0 {
		m.message = "No hay temporizadores activos"
		return
	}
	entry := m.running[m.selected]
	if err := m.db.Stop(entry.ID); err != nil {
		m.message = "No se pudo detener: " + err.Error()
	} else {
		m.message = "Temporizador detenido: " + entry.Title
	}
	m.refresh()
}

// View renders the current terminal interface.
func (m Model) View() string {
	if m.width == 0 {
		return "Iniciando Nexus…"
	}
	header := lipgloss.JoinHorizontal(lipgloss.Center,
		title.Render("NEXUS"),
		label.Render("  ·  Hoy  "+formatDuration(m.todaySeconds)),
		label.Render("  ·  "+m.now.Format("02/01/2006")),
	)
	var body string
	if m.formOpen {
		body = m.formView()
	} else {
		body = m.dashboardView()
	}
	footer := label.Render(" n Nuevo  ·  j/k o ↑/↓ Seleccionar  ·  x Detener  ·  X Detener todos  ·  r Hoy/Semana  ·  q Salir")
	if m.message != "" {
		footer = lipgloss.NewStyle().Foreground(accent).Render(m.message) + "\n" + footer
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, "", body, "", footer)
}

func (m Model) dashboardView() string {
	available := m.width - 4
	leftWidth := available
	rightWidth := available
	if m.width >= 100 {
		leftWidth = available * 43 / 100
		rightWidth = available - leftWidth - 2
	}
	running := panel.Width(max(20, leftWidth)).Render(m.runningView())
	dashboard := panel.Width(max(20, rightWidth)).Render(m.dashboardPanel())
	if m.width >= 100 {
		return lipgloss.JoinHorizontal(lipgloss.Top, running, "  ", dashboard)
	}
	return lipgloss.JoinVertical(lipgloss.Left, running, dashboard)
}

func (m Model) runningView() string {
	lines := []string{title.Render("EN CURSO")}
	if len(m.running) == 0 {
		lines = append(lines, label.Render("Aún no hay temporizadores. Pulsa n para empezar."))
		return strings.Join(lines, "\n")
	}
	for i, entry := range m.running {
		cursor := "  "
		if i == m.selected {
			cursor = lipgloss.NewStyle().Foreground(accent).Bold(true).Render("› ")
		}
		project := entry.Project
		if project == "" {
			project = "Sin proyecto"
		}
		lines = append(lines, fmt.Sprintf("%s%s\n  %s  ·  %s", cursor, lipgloss.NewStyle().Bold(i == m.selected).Render(entry.Title), label.Render(project), lipgloss.NewStyle().Foreground(accent).Render(clock(m.now.Unix()-entry.StartedAt))))
	}
	return strings.Join(lines, "\n")
}

func (m Model) dashboardPanel() string {
	rangeName := "Hoy"
	if m.week {
		rangeName = "Semana"
	}
	lines := []string{title.Render("TIEMPO POR PROYECTO  ·  " + rangeName)}
	totals := append([]domain.ProjectTotal(nil), m.totals...)
	sort.Slice(totals, func(i, j int) bool { return totals[i].Seconds > totals[j].Seconds })
	maxSeconds := int64(0)
	for _, total := range totals {
		if total.Seconds > maxSeconds {
			maxSeconds = total.Seconds
		}
	}
	barWidth := m.width / 6
	if barWidth < 8 {
		barWidth = 8
	}
	for i, total := range totals {
		name := total.Project
		if name == "" {
			name = "Sin proyecto"
		}
		width := 0
		if maxSeconds > 0 {
			width = int(float64(total.Seconds) / float64(maxSeconds) * float64(barWidth))
		}
		if total.Seconds > 0 && width == 0 {
			width = 1
		}
		bar := lipgloss.NewStyle().Foreground(projectColor(i)).Render(strings.Repeat("█", width))
		lines = append(lines, fmt.Sprintf("%-18s %s %s", truncate(name, 18), bar, formatDuration(total.Seconds)))
	}
	if len(totals) == 0 {
		lines = append(lines, label.Render("Sin tiempo registrado en este período"))
	}
	lines = append(lines, "", title.Render("RECIENTES"))
	if len(m.recent) == 0 {
		lines = append(lines, label.Render("Todavía no hay registros"))
	}
	for _, entry := range m.recent {
		project := entry.Project
		if project == "" {
			project = "Sin proyecto"
		}
		seconds := m.now.Unix() - entry.StartedAt
		if entry.EndedAt != nil {
			seconds = *entry.EndedAt - entry.StartedAt
		}
		lines = append(lines, fmt.Sprintf("%-22s %-14s %s", truncate(entry.Title, 22), truncate(project, 14), formatDuration(seconds)))
	}
	return strings.Join(lines, "\n")
}

func (m Model) formView() string {
	lines := []string{title.Render("NUEVO TEMPORIZADOR"), ""}
	labels := []string{"Título", "Proyecto", "Descripción"}
	for i, input := range m.inputs {
		lines = append(lines, label.Render(labels[i]), input.View())
	}
	lines = append(lines, "", label.Render("Tab / Mayús+Tab mover  ·  Enter continuar/guardar  ·  Esc cancelar"))
	if m.message != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(accent).Render(m.message))
	}
	return panel.Width(min(max(34, m.width-6), 64)).Render(strings.Join(lines, "\n"))
}

func rangeStart(now time.Time, week bool) time.Time {
	y, month, day := now.Date()
	start := time.Date(y, month, day, 0, 0, 0, 0, now.Location())
	if week {
		start = start.AddDate(0, 0, -6)
	}
	return start
}

func sumTotals(totals []domain.ProjectTotal) int64 {
	var sum int64
	for _, total := range totals {
		sum += total.Seconds
	}
	return sum
}

func formatDuration(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	return fmt.Sprintf("%dh %02dm", seconds/3600, seconds/60%60)
}

func clock(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	return fmt.Sprintf("%02d:%02d:%02d", seconds/3600, seconds/60%60, seconds%60)
}

func projectColor(index int) lipgloss.AdaptiveColor {
	colors := []lipgloss.AdaptiveColor{
		{Light: "#6750A4", Dark: "#C4B5FD"},
		{Light: "#00796B", Dark: "#5EEAD4"},
		{Light: "#B45309", Dark: "#FCD34D"},
		{Light: "#0369A1", Dark: "#7DD3FC"},
		{Light: "#BE185D", Dark: "#F9A8D4"},
	}
	return colors[index%len(colors)]
}

func truncate(s string, limit int) string {
	if len([]rune(s)) <= limit {
		return s
	}
	return string([]rune(s)[:limit-1]) + "…"
}
