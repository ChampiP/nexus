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
		card = m.startForm(m.width, l)
	}
	box := panel.Width(max(30, m.width-2)).Render(strings.Join(card, "\n"))
	for i, line := range strings.Split(box, "\n") {
		lines[l.cardY+i] = line
	}
	lines[l.todayY] = titleStyle.Render("BREAKS DE HOY")
	lines = append(lines, m.todayLines(l)...)
	if m.wellbeing != nil {
		for len(lines) <= l.pauseY {
			lines = append(lines, "")
		}
		box := panel.Width(max(30, m.width-2)).Render(strings.Join(m.activePauseLines(), "\n"))
		for i, line := range strings.Split(box, "\n") {
			y := l.pauseY + i
			if y < len(lines) {
				lines[y] = line
			} else {
				lines = append(lines, line)
			}
		}
	}
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
func (m Model) startForm(width int, l breakLayout) []string {
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
	durationFocus := -1
	if !m.bp.onTabs && f.row == breakRowDuration {
		durationFocus = f.choice
	}
	lines := []string{
		mark(breakRowDuration) + label.Render("Duración") + "  " + RenderOptions(breakDurationLabels, selected, durationFocus),
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
		titleWidth, projectWidth := 30, 18
		if width >= 120 {
			available := max(1, width-16)
			titleWidth, projectWidth = available*2/3, available-available*2/3
		}
		lines = append(lines, mark(breakRowFirst+i)+fmt.Sprintf("%s %s · %s", box, truncate(timer.Title, titleWidth), truncate(project, projectWidth)))
	}
	button := -1
	if !m.bp.onTabs && f.row == f.buttonsRow() {
		button = 0
	}
	return append(lines, renderButtons(breakStartLabels, button))
}

// todayLines son las líneas bajo «BREAKS DE HOY»: los breaks más recientes y el total.
func (m Model) todayLines(l breakLayout) []string {
	return m.todayLinesWidth(m.width, l)
}

func (m Model) todayLinesWidth(width int, l breakLayout) []string {
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
			titleWidth := 30
			if width >= 120 {
				titleWidth = max(1, width-45)
			}
			lines = append(lines, fmt.Sprintf("%s – %s · %s · %s", clockTime(entry.StartedAt), end, formatDuration(seconds), truncate(entry.Title, titleWidth)))
		}
	}
	return append(lines, titleStyle.Render("Total "+formatDuration(total)))
}

func (m Model) activePauseLines() []string {
	status := "Desactivadas"
	cfg := m.pauseStatus.Settings
	switch {
	case m.pauseErr != nil:
		status = "No se pudo cargar el estado de las pausas activas"
	case m.pauseStatus.DNDUntil != nil && m.pauseStatus.DNDUntil.After(m.now):
		status = "No molestar hasta las " + m.pauseStatus.DNDUntil.Format("15:04")
	case cfg.Enabled:
		status = fmt.Sprintf("Activadas · cada %d min, %s", int(cfg.Every/time.Minute), pauseDurationText(cfg.Duration))
		if !m.pauseStatus.Active {
			status += " · inactivo"
		} else {
			status += fmt.Sprintf(" · llevas %s frente a la pantalla · próxima en %s", pauseDurationText(m.pauseStatus.ActiveFor), pauseDurationText(max(time.Duration(time.Minute), m.pauseStatus.NextDue.Sub(m.now))))
		}
	}
	counters := m.pauseStatus.Counters
	enabled := 1
	if cfg.Enabled {
		enabled = 0
	}
	focusRow := func(row int) bool { return !m.bp.onTabs && m.bp.settingsRow == row }
	focusedCol := func(row int) int {
		if !m.bp.onTabs && m.bp.settingsRow == row {
			return m.bp.action
		}
		return -1
	}
	focusOption := func(row, col int) bool {
		return !m.bp.onTabs && m.bp.settingsRow == row && m.bp.action == col
	}
	statusLine := status + fmt.Sprintf(" · Hoy: %d hechas · %d saltada%s", counters.Done, counters.Skipped, pluralSuffix(counters.Skipped))
	if geometry, wide := m.wideGeometry(); wide {
		statusLine = truncateToWidth(statusLine, geometry.rightWidth-4)
	}
	lines := []string{
		titleStyle.Render("PAUSAS ACTIVAS"),
		statusLine,
	}
	if _, wide := m.wideGeometry(); wide {
		lines = append(lines, pauseRowPrefix("Activar", focusRow(0)))
		lines = append(lines, RenderOptions([]string{"Sí", "No"}, enabled, focusedCol(0)))
		lines = append(lines, pauseRowPrefix("Cada", focusRow(1)))
		for i, option := range []string{"10 min", "20 min", "30 min", "45 min", "60 min"} {
			lines = append(lines, RenderOption(option, pauseEveryIndex(cfg.Every) == i, focusOption(1, i)))
		}
		lines = append(lines, pauseRowPrefix("Dura", focusRow(2)))
		for i, option := range []string{"10 s", "15 s", "20 s", "30 s", "1 min", "2 min", "5 min"} {
			lines = append(lines, RenderOption(option, pauseDurationIndex(cfg.Duration) == i, focusOption(2, i)))
		}
		lines = append(lines, renderButtons(pauseButtonLabels(m.pauseStatus, m.now), pauseSelected(m.bp)))
		return lines
	}
	return append(lines,
		pauseRowPrefix("Activar", focusRow(0))+"     "+RenderOptions([]string{"Sí", "No"}, enabled, focusedCol(0)),
		pauseRowPrefix("Cada", focusRow(1))+RenderOptions([]string{"10 min", "20 min", "30 min", "45 min", "60 min"}, pauseEveryIndex(cfg.Every), focusedCol(1)),
		pauseRowPrefix("Dura", focusRow(2))+RenderOptions([]string{"10 s", "15 s", "20 s", "30 s", "1 min", "2 min", "5 min"}, pauseDurationIndex(cfg.Duration), focusedCol(2)),
		renderButtons(pauseButtonLabels(m.pauseStatus, m.now), pauseSelected(m.bp)),
	)
}

func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func pauseRowPrefix(name string, focused bool) string {
	mark := "  "
	if focused {
		mark = "▸ "
	}
	return mark + fmt.Sprintf("%-12s", name)
}

func pauseDurationText(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d s", int(d/time.Second))
	}
	return fmt.Sprintf("%d min", int(d/time.Minute))
}

func pauseSelected(page breakPage) int {
	if page.settingsRow == 3 && !page.onTabs {
		return page.action
	}
	return -1
}

func (m Model) breakHint() string {
	switch {
	case m.bp.onTabs:
		return "←→ o Enter cambiar de pantalla · ↓ continuar · Esc volver · clic en una pestaña"
	case m.bp.settingsRow >= 0 && m.wellbeing != nil:
		return "↑↓ mover · ←→ cambiar y guardar · Enter ejecutar · PgUp/PgDn sección · Esc volver · clic en un control"
	case m.brk != nil:
		return "←→ elegir acción · Enter ejecutar · ↑ pestañas · Esc volver · clic en un botón"
	}
	return "↑↓ mover · ←→ elegir duración · PgUp/PgDn sección · Enter marcar o empezar · Esc volver · clic en un control"
}
