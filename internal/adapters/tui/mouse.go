package tui

import tea "github.com/charmbracelet/bubbletea"

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

func (m *Model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	layout := m.computeLayout()
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
		if m.clickStatus(layout, msg.X, msg.Y) || m.confirm != nil {
			return m, nil
		}
		if len(layout.tabs) > 0 && layout.tabs[1].contains(msg.X, msg.Y) {
			m.switchScreen(screenCatalog)
			return m, nil
		}
		if len(layout.tabs) > 0 && layout.tabs[0].contains(msg.X, msg.Y) {
			m.focus = focusTabs
			m.closeProjectPicker()
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
