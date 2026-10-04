package tui

import "github.com/charmbracelet/lipgloss"

type rect struct{ x, y, w, h int }

func (r rect) contains(x, y int) bool { return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h }

// rowZones agrupa la fila completa y los botones en línea de un temporizador o un reciente.
type rowZones struct {
	row     rect
	buttons []rect
}

type screenLayout struct {
	fields                                            [3]rect
	start                                             rect
	editButtons                                       []rect
	running                                           []rowZones
	recent                                            []rowZones
	options                                           []rect
	optionsStart                                      int
	today, week                                       rect
	statusButtons                                     []rect
	descriptionY, startY, runningY, tabsY, dashboardY int
	recentY, statusY                                  int
}

// Etiquetas de los botones; el orden define su índice en rowButton.
var (
	runningLabels   = []string{"✎ Editar", "■ Detener"}
	recentLabels    = []string{"✎ Editar", "✕ Eliminar"}
	editLabels      = []string{"Guardar", "Eliminar", "Cancelar"}
	confirmLabels   = []string{"Eliminar", "Cancelar"}
	undoLabels      = []string{"Deshacer"}
	buttonSeparator = " "
)

func buttonText(label string) string { return "[" + label + "]" }

// buttonsWidth es el ancho total de una fila de botones separados por un espacio.
func buttonsWidth(labels []string) int {
	total := 0
	for i, label := range labels {
		if i > 0 {
			total += len(buttonSeparator)
		}
		total += lipgloss.Width(buttonText(label))
	}
	return total
}

// buttonRects calcula la zona de cada botón de una fila que empieza en la columna x.
func buttonRects(y, x int, labels []string) []rect {
	zones := make([]rect, len(labels))
	for i, label := range labels {
		w := lipgloss.Width(buttonText(label))
		zones[i] = rect{x, y, w, 1}
		x += w + len(buttonSeparator)
	}
	return zones
}

// rightButtonRects alinea los botones a la derecha, terminando en la columna right.
func rightButtonRects(y, right int, labels []string) []rect {
	return buttonRects(y, max(0, right-buttonsWidth(labels)), labels)
}

// computeLayout is the single source of screen coordinates for rendering and mouse hit testing.
func (m Model) computeLayout() screenLayout {
	width := m.width
	if width < 1 {
		width = 80
	}
	l := screenLayout{}
	l.fields[0] = rect{0, 3, width, 1}
	l.fields[1] = rect{0, 5, width, 1}
	y := 6
	if m.pickerOpen {
		options := m.projectOptions()
		count := len(options)
		if count > 5 {
			count = 5
		}
		pickerScroll := m.pickerScroll
		if pickerScroll > len(options)-count {
			pickerScroll = max(0, len(options)-count)
		}
		if m.pickerIndex < pickerScroll {
			pickerScroll = m.pickerIndex
		}
		if m.pickerIndex >= pickerScroll+count {
			pickerScroll = m.pickerIndex - count + 1
		}
		l.optionsStart = pickerScroll
		for i := 0; i < count; i++ {
			l.options = append(l.options, rect{0, y + i, width, 1})
		}
		y += count
	}
	l.descriptionY = y + 1
	l.fields[2] = rect{0, l.descriptionY + 1, width, 1}
	l.startY = l.descriptionY + 2
	if m.edit != nil {
		// El panel de edición ocupa el lugar del botón Iniciar.
		l.editButtons = buttonRects(l.startY, 0, editLabels)
	} else {
		l.start = rect{0, l.startY, width, 1}
	}
	l.runningY = l.startY + 2
	visible := len(m.running) - m.runScroll
	if m.height > 0 {
		visible = min(visible, max(1, m.height-l.runningY-8))
	}
	if visible < 0 {
		visible = 0
	}
	for i := 0; i < visible; i++ {
		rowY := l.runningY + 1 + i
		l.running = append(l.running, rowZones{row: rect{0, rowY, width, 1}, buttons: rightButtonRects(rowY, width, runningLabels)})
	}
	l.tabsY = l.runningY + 1 + visible + 1
	l.today = rect{0, l.tabsY, 12, 1}
	l.week = rect{13, l.tabsY, 12, 1}
	l.dashboardY = l.tabsY + 2
	l.fillDashboard(m, width)
	return l
}

// fillDashboard ubica las filas visibles de recientes y la barra de estado bajo el panel.
// El panel mide: borde, título, totales, línea en blanco, encabezado RECIENTES, filas y borde.
func (l *screenLayout) fillDashboard(m Model, width int) {
	totalsLines := max(1, len(m.totals))
	l.recentY = l.dashboardY + 4 + totalsLines
	count := len(m.recent)
	if m.height > 0 {
		// Se reservan el borde inferior, la línea en blanco y las dos líneas del pie.
		count = min(count, max(0, m.height-l.recentY-4))
	}
	right := max(30, width-2)
	for i := 0; i < count; i++ {
		rowY := l.recentY + i
		l.recent = append(l.recent, rowZones{row: rect{0, rowY, width, 1}, buttons: rightButtonRects(rowY, right, recentLabels)})
	}
	recentLines := count
	if len(m.recent) == 0 {
		recentLines = 1
	}
	l.statusY = l.dashboardY + 5 + totalsLines + recentLines + 1
	if text, labels := m.statusBar(); len(labels) > 0 {
		l.statusButtons = buttonRects(l.statusY, lipgloss.Width(text), labels)
	}
}
