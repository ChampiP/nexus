package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// breakView dibuja la pantalla de break usando las zonas de breakLayout.
func (m Model) breakView() string {
	l := m.breakLayout()
	lines := make([]string, l.todayY+1)
	lines[0] = titleStyle.Render("NEXUS") + "  " + renderTabs(m.screens(), screenBreak, m.bp.onTabs)
	var card []string
	if m.brk != nil {
		lines[l.headingY] = titleStyle.Render("BREAK EN CURSO")
		card = m.activeCard()
	} else {
		lines[l.headingY] = titleStyle.Render("EMPEZAR BREAK")
		card = m.startForm(l)
	}
	box := panel.Width(max(30, m.width-2)).Render(strings.Join(card, "\n"))
	for i, line := range strings.Split(box, "\n") {
		lines[l.cardY+i] = line
	}
	lines[l.todayY] = titleStyle.Render("BREAKS DE HOY")
	lines = append(lines, m.todayLines(l)...)
	footer := m.breakHint()
	if m.message != "" {
		footer = lipgloss.NewStyle().Foreground(accent).Render(m.message) + "\n" + footer
	}
	return strings.Join(lines, "\n") + "\n\n" + label.Render(footer)
}

// activeCard son las líneas de la tarjeta del break en curso.
func (m Model) activeCard() []string {
	b := *m.brk
	color, status := accent, "Quedan "+formatDuration(b.EndsAt-m.now.Unix())
	if breakOverdue(b, m.now) {
		color, status = urgent, "Excedido +"+formatDuration(m.now.Unix()-b.EndsAt)
	}
	resume := "No se reanudará ningún temporizador"
	if len(m.bp.resume) > 0 {
		resume = "Al volver se reanudan: " + strings.Join(m.bp.resume, ", ")
	}
	selected := -1
	if !m.bp.onTabs {
		selected = m.bp.action
	}
	return []string{
		titleStyle.Render("☕ " + b.Label),
		lipgloss.NewStyle().Bold(true).Foreground(color).Render(status),
		m.breakBar(),
		fmt.Sprintf("De %s a %s", clockTime(b.StartedAt), clockTime(b.EndsAt)),
		label.Render(resume),
		"",
		renderButtons(breakActionLabels, selected),
	}
}

func clockTime(unix int64) string { return time.Unix(unix, 0).Format("15:04") }

// startForm son las líneas del formulario para empezar un break.
func (m Model) startForm(l breakLayout) []string {
	f := m.bp.form
	mark := func(row int) string {
		if !m.bp.onTabs && f.row == row {
			return "▸ "
		}
		return "  "
	}
	selected := f.choice
	if strings.TrimSpace(f.custom.Value()) != "" {
		selected = -1
	}
	lines := []string{
		mark(breakRowDuration) + label.Render("Duración") + "  " + renderButtons(breakDurationLabels, selected),
		mark(breakRowCustom) + label.Render("Otro (min): ") + f.custom.View(),
	}
	if len(f.timers) > 0 {
		lines = append(lines, "  "+label.Render("Detener:"))
	}
	for i, timer := range f.timers {
		box := "[ ]"
		if f.checked[i] {
			box = "[x]"
		}
		project := timer.Project
		if project == "" {
			project = "Sin proyecto"
		}
		lines = append(lines, mark(breakRowFirst+i)+fmt.Sprintf("%s %s · %s", box, truncate(timer.Title, 30), truncate(project, 18)))
	}
	button := -1
	if !m.bp.onTabs && f.row == f.buttonsRow() {
		button = 0
	}
	return append(lines, renderButtons(breakStartLabels, button))
}

// todayLines son las líneas bajo «BREAKS DE HOY»: los breaks más recientes y el total.
func (m Model) todayLines(l breakLayout) []string {
	if len(m.bp.today) == 0 {
		return []string{label.Render("Sin breaks hoy")}
	}
	var lines []string
	var total int64
	for i, entry := range m.bp.today {
		seconds, end := m.now.Unix()-entry.StartedAt, "en curso"
		if entry.EndedAt != nil {
			seconds, end = *entry.EndedAt-entry.StartedAt, clockTime(*entry.EndedAt)
		}
		total += seconds
		if i >= len(m.bp.today)-l.shown {
			lines = append(lines, fmt.Sprintf("%s – %s · %s · %s", clockTime(entry.StartedAt), end, formatDuration(seconds), truncate(entry.Title, 30)))
		}
	}
	return append(lines, titleStyle.Render("Total "+formatDuration(total)))
}

func (m Model) breakHint() string {
	switch {
	case m.bp.onTabs:
		return "←→ o Enter cambiar de pantalla · ↓ continuar · Esc volver · clic en una pestaña"
	case m.brk != nil:
		return "←→ elegir acción · Enter ejecutar · ↑ pestañas · Esc volver · clic en un botón"
	}
	return "↑↓ mover · ←→ elegir duración · Enter marcar o empezar · Esc volver · clic en un control"
}
