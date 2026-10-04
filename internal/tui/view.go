package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"nexus/internal/domain"
)

func (m Model) View() string {
	if m.width == 0 {
		return "Iniciando Nexus…"
	}
	layout := m.computeLayout()
	lines := make([]string, layout.dashboardY)
	lines[0] = titleStyle.Render("NEXUS") + label.Render(fmt.Sprintf("  ·  Hoy %s  ·  %s", formatDuration(m.todaySeconds), m.now.Format("02/01/2006")))
	lines[2] = label.Render("Título")
	lines[3] = m.inputs[0].View()
	lines[4] = label.Render("Proyecto")
	projectField := m.inputs[1].View()
	if !m.pickerOpen {
		project := m.selectedProject
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
	button := "[ Iniciar ]"
	if m.focus == focusStart {
		button = "› " + button
	}
	lines[layout.startY] = lipgloss.NewStyle().Foreground(accent).Bold(true).Render(button)
	lines[layout.runningY] = titleStyle.Render("EN CURSO")
	for i, row := range layout.running {
		entry := m.running[m.runScroll+i]
		project := entry.Project
		if project == "" {
			project = "Sin proyecto"
		}
		prefix := "  "
		if int(m.focus) == int(focusRunningStart)+i {
			prefix = "› "
		}
		text := fmt.Sprintf("%s%s  ·  %s  ·  %s", prefix, truncate(entry.Title, 24), truncate(project, 18), clock(m.now.Unix()-entry.StartedAt))
		button := "■ Detener"
		text = truncate(text, max(1, m.width-12))
		padding := max(1, m.width-lipgloss.Width(text)-lipgloss.Width(button))
		lines[row.row.y] = text + strings.Repeat(" ", padding) + label.Render(button)
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
		today = "› " + today
	}
	if m.focus == focusWeek {
		week = "› " + week
	}
	lines[layout.tabsY] = titleStyle.Render(today) + "    " + titleStyle.Render(week)
	dashboard := m.dashboardPanel()
	for i, line := range strings.Split(dashboard, "\n") {
		y := layout.dashboardY + i
		if y < len(lines) {
			lines[y] = line
		} else {
			lines = append(lines, line)
		}
	}
	footer := m.hint()
	if m.message != "" {
		footer = lipgloss.NewStyle().Foreground(accent).Render(m.message) + "\n" + footer
	}
	return strings.Join(lines, "\n") + "\n\n" + label.Render(footer)
}

func (m Model) hint() string {
	switch m.focus {
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
		return "←→ o Enter cambiar período · ↑↓ mover · Ctrl+C salir"
	default:
		return "Enter detener · ↑↓ elegir temporizador · clic en ■ Detener"
	}
}

func (m Model) dashboardPanel() string {
	rangeName := "Hoy"
	if m.week {
		rangeName = "Semana"
	}
	lines := []string{titleStyle.Render("TIEMPO POR PROYECTO · " + rangeName)}
	totals := append([]domain.ProjectTotal(nil), m.totals...)
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
	return panel.Width(max(30, m.width-2)).Render(strings.Join(lines, "\n"))
}
