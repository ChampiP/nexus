package tui

import tea "github.com/charmbracelet/bubbletea"

// updateCatalogMouse resuelve clics y rueda en la pantalla de catálogo con las zonas de catalogLayout.
func (m *Model) updateCatalogMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	l := m.catalogLayout()
	c := &m.cat
	switch {
	case msg.Action != tea.MouseActionPress:
		return *m, nil
	case msg.Button == tea.MouseButtonWheelUp, msg.Button == tea.MouseButtonWheelDown:
		step := -1
		if msg.Button == tea.MouseButtonWheelDown {
			step = 1
		}
		if c.picker != nil {
			c.picker.index = min(max(0, c.picker.index+step), max(0, len(m.pickerOptions())-1))
			c.picker.scroll = m.catalogLayout().optionsStart
		} else if !m.catalogBusy() {
			c.scroll = min(max(0, c.scroll+step), max(0, len(c.rows)-l.visible))
		}
		return *m, nil
	case msg.Button != tea.MouseButtonLeft:
		return *m, nil
	}
	x, y := msg.X, msg.Y
	switch {
	case c.confirm != nil:
		for i, zone := range l.confirmButtons {
			if zone.contains(x, y) {
				m.activateCatalogConfirm(i)
			}
		}
	case c.picker != nil:
		for i, zone := range l.options {
			if zone.contains(x, y) {
				m.selectPickerIndex(l.optionsStart + i)
			}
		}
	case c.edit != nil:
		// Mientras se escribe, solo Enter y Esc actúan.
	default:
		m.clickCatalog(l, x, y)
	}
	return *m, nil
}

// clickCatalog resuelve un clic cuando no hay campo, selector ni confirmación abiertos.
func (m *Model) clickCatalog(l catalogLayout, x, y int) {
	c := &m.cat
	if m.clickTab(l.tabs, x, y) {
		return
	}
	for i, zone := range l.adds {
		if zone.contains(x, y) {
			c.focus, c.button = catFocusAdd, i
			m.activateCatalogFocus()
			return
		}
	}
	if l.toggle.contains(x, y) {
		c.focus = catFocusToggle
		m.activateCatalogFocus()
		return
	}
	for i, zone := range l.rows {
		index := l.first + i
		if c.rows[index].kind == catHeader {
			continue
		}
		for b, button := range zone.buttons {
			if button.contains(x, y) {
				c.button = b
				m.activateCatalogFocus()
				return
			}
		}
		if zone.row.contains(x, y) {
			c.focus, c.row, c.button = catFocusRow, index, 0
			m.revealCatalog()
			return
		}
	}
}
