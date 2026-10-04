package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"nexus/internal/tracking"
)

// recentContentX es la columna donde empieza el contenido del panel (borde + relleno).
const recentContentX = 2

func (m Model) View() string {
	if m.width == 0 {
		return "Iniciando Nexus…"
	}
	if m.screen == screenCatalog {
		return m.catalogView()
	}
	if ok, left, center, right := wideLayout(m.width); ok && (m.screen == screenTimers || m.screen == screenBreak) {
		return m.wideView(left, center, right)
	}
	if m.screen == screenBreak {
		return m.breakView()
	}
	layout := m.computeLayout()
	lines := make([]string, layout.dashboardY)
	lines[0] = m.headerBase(len(layout.tabs) > 0)
	if m.indicatorVisible() {
		color := accent
		if breakOverdue(*m.brk, m.now) {
			color = urgent
		}
		lines[0] += "  " + lipgloss.NewStyle().Bold(true).Foreground(color).Render(indicatorText(*m.brk, m.now))
	}
	lines[2] = label.Render("Título")
	lines[3] = m.inputs[0].View()
	lines[4] = label.Render("Proyecto")
	projectField := m.inputs[1].View()
	if !m.pickerOpen {
		project := m.selectedProjectPath()
		if project == "" {
			project = "Sin proyecto"
		}
		projectField = "‹ " + project + " ›"
		if m.focus == focusProject {
			projectField = lipgloss.NewStyle().Bold(true).Foreground(accent).Render(projectField)
		}
	}
	lines[5] = projectField
	options := m.projectOptions()
	for i, zone := range layout.options {
		index := layout.optionsStart + i
		if index < len(options) {
			lines[zone.y] = renderProjectOption(options[index], index == m.pickerIndex)
		}
	}
	lines[layout.descriptionY] = label.Render("Descripción")
	lines[layout.descriptionY+1] = m.inputs[2].View()
	if m.edit != nil {
		lines[1] = titleStyle.Render(m.editTitle())
		lines[layout.startY] = m.editButtonsLine()
	} else {
		lines[layout.startY] = startLine(m.focus == focusStart)
	}
	lines[layout.runningY] = titleStyle.Render("EN CURSO")
	for i, row := range layout.running {
		index := m.runScroll + i
		entry := m.running[index]
		project := entry.Project
		if project == "" {
			project = "Sin proyecto"
		}
		focused := m.focus == focusRunningStart && m.focusedRunning == index
		prefix := "  "
		if focused {
			prefix = "▸ "
		}
		spin := ""
		if entry.Kind != tracking.KindBreak {
			spin = m.spinnerFrame()
		}
		text := fmt.Sprintf("%s%s%s  ·  %s  ·  %s", prefix, spin, truncate(entry.Title, 24), truncate(project, 18), m.sessionAndTotal(entry))
		lines[row.row.y] = composeRow(text, 0, row.buttons, entryLabels(runningActions), m.selectedButton(focusRunningStart, index), focused)
	}
	if len(m.running) == 0 {
		lines[layout.runningY+1] = label.Render("Aún no hay temporizadores activos")
	}
	today := "Hoy"
	week := "Semana"
	if !m.week {
		today = "[ Hoy ]"
	} else {
		week = "[ Semana ]"
	}
	if m.focus == focusToday {
		today = "▸ " + today
	}
	if m.focus == focusWeek {
		week = "▸ " + week
	}
	todayStyle, weekStyle := titleStyle, titleStyle
	if m.focus == focusToday {
		todayStyle = focusButton
	}
	if m.focus == focusWeek {
		weekStyle = focusButton
	}
	lines[layout.tabsY] = todayStyle.Render(today) + "    " + weekStyle.Render(week)
	dashboard := m.dashboardPanel(layout)
	for i, line := range strings.Split(dashboard, "\n") {
		y := layout.dashboardY + i
		if y < len(lines) {
			lines[y] = line
		} else {
			lines = append(lines, line)
		}
	}
	footer := m.hint()
	if status, labels := m.statusBar(); status != "" || len(labels) > 0 {
		line := lipgloss.NewStyle().Foreground(accent).Render(status)
		if len(labels) > 0 {
			line += renderButtons(labels, m.statusSelected())
		}
		footer = line + "\n" + footer
	}
	return strings.Join(lines, "\n") + "\n\n" + label.Render(footer)
}

// sessionAndTotal es el tiempo de la sesión en curso y, si es una tarea de trabajo, el total de la tarea.
func (m Model) sessionAndTotal(entry tracking.Entry) string {
	session := clock(m.now.Unix() - entry.StartedAt)
	if task, ok := m.tasks[entry.TaskUID]; ok && entry.Kind != tracking.KindBreak {
		return session + " · total " + formatDuration(task.TotalSeconds)
	}
	return session
}

// startLine dibuja [ Iniciar ] en la fila del formulario.
func startLine(focused bool) string {
	mark := "  "
	if focused {
		mark = "▸ "
	}
	if focused {
		return titleStyle.Render("▸ ") + focusButton.Render("[ Iniciar ]")
	}
	return lipgloss.NewStyle().Foreground(accent).Bold(true).Render(mark + "[ Iniciar ]")
}

// headerBase es la línea del encabezado sin el indicador de break: título, pestañas y resumen de hoy.
func (m Model) headerBase(tabs bool) string {
	stats := label.Render(fmt.Sprintf("  ·  Hoy %s  ·  %s", formatDuration(m.todaySeconds), m.now.Format("02/01/2006")))
	if m.animating() {
		stats = label.Render("  ·  ") + m.spin.View() + label.Render(fmt.Sprintf("Hoy %s  ·  %s", formatDuration(m.todaySeconds), m.now.Format("02/01/2006")))
	}
	if tabs {
		return titleStyle.Render("NEXUS") + "  " + renderTabs(m.screens(), screenTimers, m.focus == focusTabs) + stats
	}
	return titleStyle.Render("NEXUS") + stats
}

func (m Model) hint() string {
	if m.confirm != nil {
		return "←→ elegir · Enter confirmar · Esc cancelar · clic en un botón"
	}
	if m.edit != nil && !(m.focus == focusProject) {
		return "↑↓ mover · Enter guardar · Esc cancelar · clic en un campo o botón"
	}
	switch m.focus {
	case focusTabs:
		return "←→ o Enter cambiar de pantalla · ↓ continuar · clic en una pestaña"
	case focusTitle:
		return "↑↓ mover · Enter iniciar · clic para seleccionar · Ctrl+C salir"
	case focusProject:
		if m.pickerOpen {
			return "↑↓ elegir · Enter seleccionar · Esc cerrar"
		}
		return "←→ cambiar proyecto · Enter ver lista · escribe para buscar · ↑↓ mover"
	case focusDescription:
		return "↑↓ mover · Enter iniciar · clic para seleccionar"
	case focusStart:
		return "Enter iniciar · ↑↓ mover · clic para iniciar"
	case focusToday, focusWeek:
		return "←→ cambiar período · ↑↓ mover · PgUp/PgDn sección · Ctrl+C salir"
	case focusUndo:
		return "Enter deshacer · ↑↓ mover · Ctrl+Z deshacer"
	default:
		return "←→ elegir acción · Enter ejecutar · ↑↓ mover · PgUp/PgDn sección · Ctrl+Z deshacer"
	}
}

func (m Model) dashboardPanel(layout screenLayout) string {
	rangeName := "Hoy"
	if m.week {
		rangeName = "Semana"
	}
	lines := []string{titleStyle.Render("TIEMPO POR PROYECTO · " + rangeName)}
	totals := append([]tracking.ProjectTotal(nil), m.totals...)
	sort.Slice(totals, func(i, j int) bool { return totals[i].Seconds > totals[j].Seconds })
	maxSeconds := int64(0)
	for _, item := range totals {
		if item.Seconds > maxSeconds {
			maxSeconds = item.Seconds
		}
	}
	barWidth := m.width / 6
	if barWidth < 8 {
		barWidth = 8
	}
	for i, item := range totals {
		name := item.Project
		if name == "" {
			name = "Sin proyecto"
		}
		width := 0
		if maxSeconds > 0 {
			width = int(float64(item.Seconds) / float64(maxSeconds) * float64(barWidth))
			if item.Seconds > 0 && width == 0 {
				width = 1
			}
		}
		lines = append(lines, fmt.Sprintf("%-18s %s %s", truncate(name, 18), lipgloss.NewStyle().Foreground(projectColor(i)).Render(strings.Repeat("█", width)), formatDuration(item.Seconds)))
	}
	if len(totals) == 0 {
		lines = append(lines, label.Render("Sin tiempo registrado en este período"))
	}
	lines = append(lines, "", titleStyle.Render("RECIENTES"))
	if len(m.recent) == 0 {
		lines = append(lines, label.Render("Todavía no hay registros"))
	}
	for i, zone := range layout.recent {
		task := m.recent[i]
		project := task.Project
		if project == "" {
			project = "Sin proyecto"
		}
		focused := m.focus == focusRecent && m.focusedRecent == i
		prefix := "  "
		if focused {
			prefix = "▸ "
		}
		text := fmt.Sprintf("%s%-22s %-14s %s", prefix, truncate(task.Title, 22), truncate(project, 14), formatDuration(task.TotalSeconds))
		lines = append(lines, composeRow(text, recentContentX, zone.buttons, entryLabels(recentActions()), m.selectedButton(focusRecent, i), focused))
	}
	return panel.Width(max(30, m.width-2)).Render(strings.Join(lines, "\n"))
}
