package tui

// pickerRows es el máximo de opciones visibles del selector.
const pickerRows = 6

// catalogLayout reúne las coordenadas de la pantalla de catálogo; dibujo y ratón la comparten.
type catalogLayout struct {
	tabs            []rect
	headingY, treeY int
	first, visible  int // primera fila visible y cuántas caben
	rows            []rowZones
	addY, toggleY   int
	adds            []rect
	toggle          rect
	modalY          int
	titleY, searchY int
	options         []rect
	optionsStart    int
	confirmButtons  []rect
}

// modalHeight es el alto del área bajo las acciones: selector (título, búsqueda, opciones) o confirmación.
func (m Model) modalHeight() int {
	switch {
	case m.cat.picker != nil:
		return 2 + min(pickerRows, max(1, len(m.pickerOptions())))
	case m.cat.confirm != nil:
		return 2
	}
	return 0
}

// catalogLayout calcula todas las posiciones a partir del estado; es la única fuente de coordenadas.
func (m Model) catalogLayout() catalogLayout {
	width := m.width
	if width < 1 {
		width = 80
	}
	l := catalogLayout{tabs: tabRects(), headingY: 1, treeY: 2}
	total := len(m.cat.rows)
	modal := m.modalHeight()
	l.visible = total
	if m.height > 0 {
		// Reservado: barra y encabezado, línea en blanco y dos acciones, área modal con su separador y el pie.
		reserved := 2 + 3 + modal + 3
		if modal > 0 {
			reserved++
		}
		l.visible = min(total, max(3, m.height-reserved))
	}
	l.first = min(max(m.cat.scroll, 0), max(0, total-l.visible))
	for i := 0; i < l.visible; i++ {
		y := l.treeY + i
		zones := rowZones{row: rect{0, y, width, 1}}
		index := l.first + i
		if m.cat.focus == catFocusRow && index == m.cat.row && !m.catalogBusy() {
			zones.buttons = rightButtonRects(y, width, rowLabels(m.cat.rows[index]))
		}
		l.rows = append(l.rows, zones)
	}
	l.addY = l.treeY + max(1, l.visible) + 1
	l.toggleY = l.addY + 1
	l.adds = buttonRects(l.addY, 2, addLabels)
	l.toggle = rect{2, l.toggleY, buttonsWidth([]string{toggleLabel(m.cat.showArchived)}), 1}
	l.modalY = l.toggleY + 2
	switch {
	case m.cat.picker != nil:
		options := m.pickerOptions()
		p := m.cat.picker
		count := min(pickerRows, len(options))
		scroll := min(max(p.scroll, 0), max(0, len(options)-count))
		if p.index < scroll {
			scroll = p.index
		}
		if p.index >= scroll+count {
			scroll = p.index - count + 1
		}
		l.titleY, l.searchY = l.modalY, l.modalY+1
		l.optionsStart = scroll
		for i := 0; i < count; i++ {
			l.options = append(l.options, rect{0, l.modalY + 2 + i, width, 1})
		}
	case m.cat.confirm != nil:
		l.confirmButtons = buttonRects(l.modalY+1, 2, m.cat.confirm.labels())
	}
	return l
}
