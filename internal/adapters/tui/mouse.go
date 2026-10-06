package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *Model) updateWideMouse(msg tea.MouseMsg, g wideGeometry, layout screenLayout) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m.updateMouseNarrowFallback(msg)
	}
	// Igual que en modo angosto: primero la barra de estado y, con confirmación abierta, nada más responde.
	if m.clickStatus(layout, msg.X, msg.Y) || m.confirm != nil {
		return m, nil
	}
	if tab := m.wideCatalogTabZone(); tab.contains(msg.X, msg.Y) {
		m.openCatalogOverlay()
		return m, nil
	}
	if msg.X >= g.rightX && msg.X < g.rightX+g.rightWidth {
		m.wideColumn = 2
		m.syncInputFocus()
		local := msg
		local.X -= g.rightX
		return m.updateBreakMouse(local)
	}
	if msg.X >= g.leftX && msg.X < g.leftX+g.leftWidth {
		m.wideColumn = 0
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
		for i, zone := range layout.options {
			if zone.contains(msg.X, msg.Y) {
				options := m.projectOptions()
				if i < len(options) && options[i].selectable {
					m.pickerIndex = i
					m.selectProject()
				}
				return m, nil
			}
		}
		if m.edit != nil {
			for i, zone := range layout.editButtons {
				if zone.contains(msg.X, msg.Y) {
					m.activateEditButton(focusEditSave + focusTarget(i))
					return m, nil
				}
			}
			return m, nil
		}
		if layout.start.contains(msg.X, msg.Y) {
			m.focus = focusStart
			m.startTimer()
			return m, nil
		}
		if m.clickRows(focusRunningStart, layout.running, m.runScroll, msg.X, msg.Y) {
			return m, nil
		}
		return m, nil
	}
	if msg.X >= g.centerX && msg.X < g.centerX+g.centerWidth {
		m.wideColumn = 1
		m.syncInputFocus()
		if m.clickRows(focusRecent, layout.recent, 0, msg.X, msg.Y) {
			return m, nil
		}
		if layout.today.contains(msg.X, msg.Y) {
			m.week, m.focus = false, focusToday
			m.refresh()
		} else if layout.week.contains(msg.X, msg.Y) {
			m.week, m.focus = true, focusWeek
			m.refresh()
		}
	}
	return m, nil
}

func (m *Model) updateMouseNarrowFallback(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		if m.pickerOpen {
			m.movePicker(-1)
		} else if len(m.running) > 0 && m.runScroll > 0 {
			m.runScroll--
		}
	case tea.MouseButtonWheelDown:
		if m.pickerOpen {
			m.movePicker(1)
		} else if m.runScroll+1 < len(m.running) {
			m.runScroll++
		}
	}
	return m, nil
}

// focusRow enfoca una fila (temporizador o reciente) y deja resaltado el botón indicado.
func (m *Model) focusRow(kind focusTarget, index, button int) {
	if m.pickerOpen {
		m.closeProjectPicker()
	}
	m.focus = kind
	if kind == focusRunningStart {
		m.focusedRunning = index
	} else {
		m.focusedRecent = index
	}
	m.rowButton = button
	m.syncInputFocus()
}

// clickRows resuelve clics sobre filas y botones en línea; devuelve true si hubo coincidencia.
func (m *Model) clickRows(kind focusTarget, zones []rowZones, offset, x, y int) bool {
	for i, row := range zones {
		for b, zone := range row.buttons {
			if zone.contains(x, y) {
				m.focusRow(kind, offset+i, b)
				m.activateRowButton()
				return true
			}
		}
		if row.row.contains(x, y) {
			m.focusRow(kind, offset+i, 0)
			return true
		}
	}
	return false
}

// clickStatus resuelve clics sobre los botones de la barra de estado (confirmación o Deshacer).
func (m *Model) clickStatus(layout screenLayout, x, y int) bool {
	for i, zone := range layout.statusButtons {
		if !zone.contains(x, y) {
			continue
		}
		if m.confirm != nil {
			m.activateConfirm(i)
		} else {
			m.restoreLast()
		}
		return true
	}
	return false
}

func (m Model) wideMouseLayout() (wideGeometry, screenLayout, bool) {
	geometry, wide := m.wideGeometry()
	layout := m.computeLayout()
	if !wide {
		return geometry, layout, false
	}
	layout.options = nil
	layout.fields = [3]rect{
		{geometry.leftContentX + 2, 4, geometry.leftWidth - 6, 1},
		{geometry.leftContentX, 6, geometry.leftWidth - 4, 1},
		{geometry.leftContentX + 2, 8, geometry.leftWidth - 6, 1},
	}
	if m.pickerOpen {
		for i := range m.projectOptions() {
			layout.options = append(layout.options, rect{geometry.leftContentX, 9 + i, geometry.leftWidth - 4, 1})
		}
	}
	if m.edit == nil {
		layout.start = rect{geometry.leftContentX + 2, geometry.startY, lipgloss.Width("[ Iniciar ]"), 1}
	} else {
		layout.editButtons = buttonRects(geometry.startY, geometry.leftContentX, editLabels)
	}
	layout.running, layout.recent = geometry.runningRows, geometry.recent
	layout.today, layout.week = geometry.today, geometry.week
	// La barra de estado va bajo el contenido de tres columnas: cabecera + contenido + línea en blanco.
	layout.statusY = 1 + lipgloss.Height(m.wideContent(geometry)) + 1
	layout.statusButtons = nil
	if text, labels := m.statusBar(); len(labels) > 0 {
		layout.statusButtons = buttonRects(layout.statusY, lipgloss.Width(text), labels)
	}
	return geometry, layout, true
}

func (m *Model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	geometry, layout, wide := m.wideMouseLayout()
	if wide {
		return m.updateWideMouse(msg, geometry, layout)
	}
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
		if m.clickStatus(layout, msg.X, msg.Y) || m.confirm != nil {
			return m, nil
		}
		if layout.breakIndicator.contains(msg.X, msg.Y) {
			m.openBreakPage()
			return m, nil
		}
		if m.clickTab(layout.tabs, msg.X, msg.Y) {
			return m, nil
		}
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
		for i, zone := range layout.options {
			if zone.contains(msg.X, msg.Y) {
				// Los encabezados de organización y cliente no son seleccionables: el clic no hace nada.
				if options := m.projectOptions(); layout.optionsStart+i < len(options) && !options[layout.optionsStart+i].selectable {
					return m, nil
				}
				m.pickerIndex = layout.optionsStart + i
				m.selectProject()
				return m, nil
			}
		}
		if m.edit != nil {
			for i, zone := range layout.editButtons {
				if zone.contains(msg.X, msg.Y) {
					m.activateEditButton(focusEditSave + focusTarget(i))
					return m, nil
				}
			}
			return m, nil
		}
		if layout.start.contains(msg.X, msg.Y) {
			m.focus = focusStart
			m.startTimer()
			return m, nil
		}
		if m.clickRows(focusRunningStart, layout.running, m.runScroll, msg.X, msg.Y) || m.clickRows(focusRecent, layout.recent, 0, msg.X, msg.Y) {
			return m, nil
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
