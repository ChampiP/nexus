package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"nexus/internal/tracking"
)

// wideLayout divides wide terminals into three panels with one-column gutters.
func wideLayout(width int) (ok bool, left, center, right int) {
	if width < 120 {
		return false, 0, 0, 0
	}
	usable := width - 2
	left = usable * 34 / 100
	center = usable * 36 / 100
	right = usable - left - center
	if left < 36 {
		left = 36
	}
	if center < 36 {
		center = 36
	}
	right = usable - left - center
	return true, left, center, right
}

type wideNavigation struct {
	columns [3]navGrid
	last    [3]navCell
	hasLast [3]bool
}

func (m Model) wideNavigationGrid() wideNavigation {
	var grid wideNavigation
	timers := m.timersGrid()
	for _, row := range timers {
		cell := row.cells[0]
		switch focusTarget(cell.kind) {
		case focusTitle, focusProject, focusDescription, focusStart, focusEditSave, focusEditDelete, focusEditCancel:
			grid.columns[0] = append(grid.columns[0], row)
		case focusRunningStart:
			grid.columns[0] = append(grid.columns[0], row)
		case focusToday, focusWeek, focusRecent, focusUndo:
			grid.columns[1] = append(grid.columns[1], row)
		}
	}
	breaks := m.breakGrid()
	for _, row := range breaks {
		if row.cells[0].kind != brkTabs {
			grid.columns[2] = append(grid.columns[2], row)
		}
	}
	for column := range grid.columns {
		if len(grid.columns[column]) == 0 {
			continue
		}
		grid.last[column] = grid.columns[column][0].cells[0]
		remembered := m.wideRemembered[column]
		for _, row := range grid.columns[column] {
			for _, cell := range row.cells {
				if m.wideHasRemembered[column] && cell == remembered {
					grid.last[column] = remembered
					grid.hasLast[column] = true
				}
			}
		}
	}
	return grid
}

func (g *wideNavigation) move(current navCell, column int, key tea.KeyType) (navCell, int, bool) {
	if column < 0 || column >= len(g.columns) || len(g.columns[column]) == 0 {
		return current, column, false
	}
	grid := g.columns[column]
	row, cellColumn := grid.locate(current)
	var target navCell
	switch key {
	case tea.KeyUp:
		if row == 0 {
			return navCell{int(focusTabs), 0, 0}, column, true
		}
		target = grid.vertical(current, -1)
	case tea.KeyDown:
		if current.kind == int(focusTabs) {
			if g.hasLast[column] {
				target = g.last[column]
			} else {
				target = grid[0].cells[0]
			}
		} else {
			target = grid.vertical(current, 1)
		}
	case tea.KeyLeft, tea.KeyRight:
		step := 1
		if key == tea.KeyLeft {
			step = -1
		}
		if cellColumn+step >= 0 && cellColumn+step < len(grid[row].cells) {
			target = grid.horizontal(current, step)
		} else {
			next := column + step
			if next < 0 || next >= len(g.columns) || len(g.columns[next]) == 0 {
				return current, column, true
			}
			g.last[column] = current
			if g.hasLast[next] {
				target = g.last[next]
			} else {
				target = g.columns[next].landing(min(row, len(g.columns[next])-1), cellColumn)
			}
			column = next
		}
	case tea.KeyPgUp:
		target = grid.section(current, -1)
	case tea.KeyPgDown:
		target = grid.section(current, 1)
	case tea.KeyHome:
		target = grid[0].cells[0]
	case tea.KeyEnd:
		last := grid[len(grid)-1]
		target = last.cells[len(last.cells)-1]
	case tea.KeyTab, tea.KeyShiftTab:
		step := 1
		if key == tea.KeyShiftTab {
			step = -1
		}
		linearTarget := grid.linear(current, step)
		if linearTarget != current {
			target = linearTarget
		} else {
			next := column + step
			if next < 0 || next >= len(g.columns) || len(g.columns[next]) == 0 {
				return current, column, true
			}
			if step > 0 {
				target = g.columns[next][0].cells[0]
			} else {
				last := g.columns[next][len(g.columns[next])-1]
				target = last.cells[len(last.cells)-1]
			}
			column = next
		}
	default:
		return current, column, false
	}
	g.last[column] = target
	return target, column, true
}

type wideGeometry struct {
	leftWidth, centerWidth, rightWidth int
	leftX, centerX, rightX             int
	leftContentX, centerContentX       int
	formY, startY, runningY            int
	runningRows                        []rowZones
	today, week                        rect
	recent                             []rowZones
}

func (m Model) wideGeometry() (wideGeometry, bool) {
	ok, left, center, right := wideLayout(m.width)
	if !ok {
		return wideGeometry{}, false
	}
	g := wideGeometry{
		leftWidth: left, centerWidth: center, rightWidth: right,
		leftX: 0, centerX: left + 1, rightX: left + center + 2,
	}
	g.leftContentX, g.centerContentX = g.leftX+2, g.centerX+2
	g.formY = 3
	formLines := strings.Split(m.wideTaskForm(left), "\n")
	g.startY = g.formY + len(formLines) - 1
	runningPanelY := 1 + len(formLines) + 3
	visible := len(m.running) - m.runScroll
	if m.height > 0 {
		visible = min(visible, max(1, m.height-runningPanelY-4))
	}
	visible = max(0, visible)
	for i := 0; i < visible; i++ {
		y := runningPanelY + 2 + i
		buttons := buttonRects(y, g.leftContentX+left-4-buttonsWidth(entryLabels(runningActions)), entryLabels(runningActions))
		g.runningRows = append(g.runningRows, rowZones{row: rect{g.leftContentX, y, left - 4, 1}, buttons: buttons})
	}
	g.today = rect{g.centerContentX, 3, lipgloss.Width("[ Hoy ]"), 1}
	if m.week {
		g.today.w = lipgloss.Width("Hoy")
		g.week = rect{g.centerContentX + lipgloss.Width("Hoy    "), 3, lipgloss.Width("[ Semana ]"), 1}
	} else {
		g.week = rect{g.centerContentX + lipgloss.Width("[ Hoy ]    "), 3, lipgloss.Width("Semana"), 1}
	}
	totalsLines := max(1, len(m.projectBars()))
	recentY := 2 + 4 + totalsLines
	for i := range m.recent {
		y := recentY + i
		labels := entryLabels(recentActions())
		buttons := buttonRects(y, g.centerContentX+center-4-buttonsWidth(labels), labels)
		g.recent = append(g.recent, rowZones{row: rect{g.centerContentX, y, center - 4, 1}, buttons: buttons})
	}
	return g, true
}

func (m *Model) updateWideKey(key tea.KeyMsg) tea.Cmd {
	if m.handleUndoKey(key) {
		return nil
	}
	if key.Type == tea.KeyEsc {
		if m.screen == screenBreak {
			m.switchScreen(screenTimers)
		} else if m.confirm != nil {
			m.confirm = nil
		} else if m.pickerOpen {
			m.closeProjectPicker()
		} else if m.edit != nil {
			m.closeEdit()
		} else {
			return tea.Quit
		}
		return nil
	}
	if m.screen == screenTimers && m.confirm != nil {
		m.updateConfirm(key)
		return nil
	}
	if m.focus == focusTabs {
		if key.Type == tea.KeyEnter {
			m.openCatalogOverlay()
			return nil
		}
		if m.stepScreen(key.Type) {
			return nil
		}
	}

	// Manejo del selector de proyectos abierto en la columna 0.
	if m.pickerOpen && m.focus == focusProject && (m.width < 120 || m.wideColumn == 0) {
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
		case tea.KeyTab, tea.KeyShiftTab:
			m.closeProjectPicker()
		default:
			if key.Type == tea.KeyRunes || key.Type == tea.KeySpace || key.Type == tea.KeyBackspace || key.Type == tea.KeyDelete {
				var cmd tea.Cmd
				m.inputs[1], cmd = m.inputs[1].Update(key)
				m.pickerIndex = 0
				for i, option := range m.projectOptions() {
					if option.selectable {
						m.pickerIndex = i
						break
					}
				}
				return cmd
			}
			return nil
		}
	}

	// Abrir selector de proyectos con teclas de texto o Enter cuando está cerrado.
	if m.focus == focusProject && !m.pickerOpen && (m.width < 120 || m.wideColumn == 0) {
		if key.Type == tea.KeyEnter {
			m.openProjectPicker()
			return nil
		}
		if key.Type == tea.KeyRunes || key.Type == tea.KeySpace || key.Type == tea.KeyBackspace || key.Type == tea.KeyDelete {
			m.openProjectPicker()
			var cmd tea.Cmd
			m.inputs[1], cmd = m.inputs[1].Update(key)
			m.pickerIndex = 0
			for i, option := range m.projectOptions() {
				if option.selectable {
					m.pickerIndex = i
					break
				}
			}
			return cmd
		}
	}

	// Columna 2 (Breaks / Pausas activas).
	if m.wideColumn == 2 {
		if m.brk == nil && m.breakCell().kind == brkForm && m.breakCell().index == breakRowCustom {
			if key.Type == tea.KeyLeft || key.Type == tea.KeyRight {
				if !inputAtEdge(m.bp.form.custom, key.Type) {
					var cmd tea.Cmd
					m.bp.form.custom, cmd = m.bp.form.custom.Update(key)
					return cmd
				}
			} else if key.Type != tea.KeyUp && key.Type != tea.KeyDown && key.Type != tea.KeyPgUp && key.Type != tea.KeyPgDown && key.Type != tea.KeyHome && key.Type != tea.KeyEnd && key.Type != tea.KeyTab && key.Type != tea.KeyShiftTab {
				return m.updateBreakKey(key)
			}
		}
		if key.Type == tea.KeyEnter {
			return m.updateBreakKey(key)
		}
	}

	// Navegación dentro del campo de texto activo con flechas horizontales.
	if index := m.inputIndex(); index >= 0 && (m.width < 120 || m.wideColumn == 0) && (key.Type == tea.KeyLeft || key.Type == tea.KeyRight) {
		if !inputAtEdge(m.inputs[index], key.Type) {
			var cmd tea.Cmd
			m.inputs[index], cmd = m.inputs[index].Update(key)
			return cmd
		}
	}

	current := m.currentCell()
	if m.wideColumn == 2 {
		current = m.breakCell()
	}
	if m.focus == focusTabs {
		current = navCell{int(focusTabs), 0, 0}
	}
	grid := m.wideNavigationGrid()
	column := m.wideColumn
	if m.focus != focusTabs {
		if column != 2 {
			column = grid.columnFor(current, column)
		}
		m.wideRemembered[column] = current
		m.wideHasRemembered[column] = true
	}
	target, nextColumn, ok := grid.move(current, column, key.Type)
	if ok {
		m.wideColumn = nextColumn
		m.wideRemembered[nextColumn] = target
		m.wideHasRemembered[nextColumn] = true
		if target.kind == int(focusTabs) {
			m.focus = focusTabs
			m.bp.onTabs = false
			m.syncInputFocus()
			m.syncBreakFocus()
			return nil
		}
		if nextColumn == 2 {
			m.goToBreak(target)
			if target.kind == brkWellbeing && target.index < 3 && (key.Type == tea.KeyLeft || key.Type == tea.KeyRight) {
				m.savePauseSelection(target.index, target.col)
			}
			m.syncInputFocus()
			m.syncBreakFocus()
			return nil
		}
		m.goTo(target)
		m.syncInputFocus()
		m.syncBreakFocus()
		return nil
	}

	// Teclas de acción o edición que no fueron consumidas por la navegación.
	if key.Type == tea.KeyEnter {
		return m.updateKey(key)
	}

	// Entrada de texto en inputs activos (Título, Descripción).
	index := m.inputIndex()
	if index >= 0 && (m.width < 120 || m.wideColumn == 0) {
		var cmd tea.Cmd
		m.inputs[index], cmd = m.inputs[index].Update(key)
		return cmd
	}

	if m.wideColumn == 2 {
		return m.updateBreakKey(key)
	}

	return m.updateKey(key)
}

func (g wideNavigation) columnFor(cell navCell, fallback int) int {
	for column, grid := range g.columns {
		for _, row := range grid {
			for _, candidate := range row.cells {
				if candidate == cell {
					return column
				}
			}
		}
	}
	return fallback
}

// wideContent arma las tres columnas; wideView y la geometría del ratón comparten su altura.
func (m Model) wideContent(geometry wideGeometry) string {
	leftWidth, centerWidth, rightWidth := geometry.leftWidth, geometry.centerWidth, geometry.rightWidth
	left := lipgloss.JoinVertical(lipgloss.Left,
		widePanel(leftWidth, titleStyle.Render("NUEVA TAREA"), m.wideTaskForm(leftWidth)),
		widePanel(leftWidth, titleStyle.Render("EN CURSO"), m.wideRunning(leftWidth)),
	)
	center := m.wideDashboard(centerWidth)
	right := m.wideBreakPanels(rightWidth)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", center, " ", right)
}

func (m Model) wideView(leftWidth, centerWidth, rightWidth int) string {
	geometry, _ := m.wideGeometry()
	content := m.wideContent(geometry)
	header := m.headerBase(false) + "  " + renderTabs([]screen{screenCatalog}, screenCatalog, m.focus == focusTabs || m.isCatalogOverlayOpen())
	footer := m.hint()
	if m.isCatalogOverlayOpen() {
		footer = m.catalogHint()
	}
	if status, labels := m.statusBar(); status != "" || len(labels) > 0 {
		line := lipgloss.NewStyle().Foreground(accent).Render(status)
		if len(labels) > 0 {
			line += renderButtons(labels, m.statusSelected())
		}
		footer = line + "\n" + footer
	}
	baseView := header + "\n" + content + "\n\n" + label.Render(footer)
	if m.isCatalogOverlayOpen() {
		return m.compositeCatalogOverlay(baseView)
	}
	return baseView
}

func widePanel(width int, heading, body string) string {
	return panel.Width(width - 2).Render(heading + "\n" + body)
}

func (m Model) wideTaskForm(width int) string {
	formActive := m.width < 120 || m.wideColumn == 0
	projectField := m.inputs[1].View()
	if !m.pickerOpen {
		project := m.selectedProjectPath()
		if project == "" {
			project = "Sin proyecto"
		}
		projectField = "‹ " + truncateToWidth(project, width-8) + " ›"
		if formActive && m.focus == focusProject {
			projectField = lipgloss.NewStyle().Bold(true).Foreground(accent).Render(projectField)
		}
	}
	lines := []string{
		label.Render("Título"), m.inputs[0].View(),
		label.Render("Proyecto"), projectField,
		label.Render("Descripción"), m.inputs[2].View(),
	}
	if m.pickerOpen {
		for i, option := range m.projectOptions() {
			lines = append(lines, renderProjectOption(option, i == m.pickerIndex))
		}
	}
	if m.edit != nil {
		lines = append(lines, m.editButtonsLine())
	} else {
		lines = append(lines, startLine(formActive && m.focus == focusStart))
	}
	return strings.Join(lines, "\n")
}

func (m Model) wideRunning(width int) string {
	if len(m.running) == 0 {
		return label.Render("Aún no hay temporizadores activos")
	}
	columnActive := m.width < 120 || m.wideColumn == 0
	var lines []string
	// Solo se dibuja la ventana visible, la misma que usa wideGeometry para los clics.
	g, _ := m.wideGeometry()
	end := min(len(m.running), m.runScroll+len(g.runningRows))
	for i := m.runScroll; i < end; i++ {
		entry := m.running[i]
		project := entry.Project
		if project == "" {
			project = "Sin proyecto"
		}
		prefix := "  "
		focused := columnActive && m.focus == focusRunningStart && m.focusedRunning == i
		if focused {
			prefix = "▸ "
		}
		spin := ""
		if entry.Kind != tracking.KindBreak {
			spin = m.spinnerFrame()
		}
		nameWidth := max(8, width-38)
		projectWidth := max(8, width-nameWidth-35)
		text := fmt.Sprintf("%s%s%s  ·  %s  ·  %s", prefix, spin, truncate(entry.Title, nameWidth), truncate(project, projectWidth), m.sessionAndTotal(entry))
		selectedButton := -1
		if columnActive {
			selectedButton = m.selectedButton(focusRunningStart, i)
		}
		buttons := renderButtons(entryLabels(runningActions), selectedButton)
		text = truncateToWidth(text, max(1, width-4-lipgloss.Width(buttons)-2))
		lines = append(lines, text+"  "+buttons)
	}
	return strings.Join(lines, "\n")
}

func (m Model) wideDashboard(width int) string {
	bars := m.projectBars()
	var period int64
	for _, item := range bars {
		period += item.seconds
	}
	heading := fmt.Sprintf("HOY %s · SEMANA %s", formatDuration(m.todaySeconds), formatDuration(period))
	dashActive := m.width < 120 || m.wideColumn == 1
	todayFocus := dashActive && m.focus == focusToday
	weekFocus := dashActive && m.focus == focusWeek
	today := RenderPeriodOption("Hoy", !m.week, todayFocus)
	week := RenderPeriodOption("Semana", m.week, weekFocus)
	if todayFocus {
		today = "▸ " + today
	}
	if weekFocus {
		week = "▸ " + week
	}
	periodSelector := today + "    " + week
	lines := []string{titleStyle.Render(heading), periodSelector}
	maxSeconds := int64(0)
	for _, item := range bars {
		maxSeconds = max(maxSeconds, item.seconds)
	}
	barWidth := max(4, width/7)
	nameWidth := max(12, width-barWidth-15)
	for i, item := range bars {
		name := item.label
		bar := 0
		if maxSeconds > 0 {
			bar = int(float64(item.seconds) / float64(maxSeconds) * float64(barWidth))
			if item.seconds > 0 && bar == 0 {
				bar = 1
			}
		}
		lines = append(lines, fmt.Sprintf("%-*s %s %s", nameWidth, truncate(name, nameWidth), lipgloss.NewStyle().Foreground(projectColor(i)).Render(strings.Repeat("█", bar)), formatDuration(item.seconds)))
	}
	if len(bars) == 0 {
		lines = append(lines, label.Render("Sin tiempo registrado en este período"))
	}
	lines = append(lines, "", titleStyle.Render("RECIENTES"))
	if len(m.recent) == 0 {
		lines = append(lines, label.Render("Todavía no hay registros"))
	} else {
		nameWidth := max(10, width-32)
		projectWidth := max(8, width-nameWidth-20)
		for i, task := range m.recent {
			project := task.Project
			if project == "" {
				project = "Sin proyecto"
			}
			text := fmt.Sprintf("  %-*s %-*s %s", nameWidth, truncate(task.Title, nameWidth), projectWidth, truncate(project, projectWidth), formatDuration(task.TotalSeconds))
			selectedButton := -1
			if dashActive {
				selectedButton = m.selectedButton(focusRecent, i)
			}
			buttons := renderButtons(entryLabels(recentActions()), selectedButton)
			text = truncateToWidth(text, max(1, width-4-lipgloss.Width(buttons)-2))
			lines = append(lines, text+"  "+buttons)
		}
	}
	return panel.Width(width - 2).Render(strings.Join(lines, "\n"))
}

func (m Model) wideBreakPanels(width int) string {
	layout := m.breakLayout()
	var breakBody []string
	if m.brk == nil {
		breakBody = append(breakBody, m.startForm(width, layout)...)
	} else {
		breakBody = append(breakBody, m.activeCard(width-4)...)
	}
	breakBody = append(breakBody, titleStyle.Render("BREAKS DE HOY"))
	breakBody = append(breakBody, m.todayLinesWidth(width, layout)...)
	breakPanel := widePanel(width, titleStyle.Render("☕ BREAK"), strings.Join(breakBody, "\n"))
	pause := label.Render("Sin estado de pausas activas")
	if m.wellbeing != nil {
		pause = strings.Join(m.activePauseLines(), "\n")
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		breakPanel,
		widePanel(width, titleStyle.Render("🚶 PAUSAS ACTIVAS"), pause),
	)
}

func truncateToWidth(value string, width int) string {
	return truncate(value, max(1, width))
}
