package tui

import "github.com/charmbracelet/lipgloss"

// breakDurationX es el ancho de «▸ Duración  » antes de los botones de duración.
const breakDurationX = 12

// Líneas de contenido de la tarjeta del break activo: etiqueta, estado, horario, reanudación, línea en blanco y botones.
const breakCardLines = 6

// breakLayout reúne las coordenadas de la pantalla de break; dibujo y ratón la comparten.
type breakLayout struct {
	tabs      []rect
	headingY  int
	cardY     int // borde superior del recuadro
	actions   []rect
	durations []rect
	custom    rect
	checks    []rect
	start     rect
	todayY    int // encabezado «BREAKS DE HOY»
	shown     int // breaks de hoy visibles; los más recientes
}

// breakLayout calcula las posiciones a partir del estado.
func (m Model) breakLayout() breakLayout {
	width := m.width
	if width < 1 {
		width = 80
	}
	l := breakLayout{tabs: tabRects(m.screens()), headingY: 1, cardY: 2}
	y := l.cardY + 1
	x := recentContentX
	var content int
	if m.brk != nil {
		l.actions = buttonRects(y+breakCardLines-1, x, breakActionLabels)
		content = breakCardLines
	} else {
		f := m.bp.form
		l.durations = buttonRects(y, x+breakDurationX, breakDurationLabels)
		l.custom = rect{0, y + 1, width, 1}
		next := y + 2
		if len(f.timers) > 0 {
			next++ // línea «Detener:»
			for i := range f.timers {
				l.checks = append(l.checks, rect{0, next + i, width, 1})
			}
			next += len(f.timers)
		}
		l.start = rect{x, next, lipgloss.Width(buttonText(breakStartLabels[0])), 1}
		content = next + 1 - y
	}
	l.todayY = l.cardY + content + 2 + 1
	l.shown = len(m.bp.today)
	if m.height > 0 {
		// Se reservan el encabezado, la línea de total y las tres líneas del pie.
		l.shown = min(l.shown, max(1, m.height-l.todayY-5))
	}
	return l
}

// fillBreakIndicator ubica el indicador de break al final del encabezado de Temporizadores.
func (l *screenLayout) fillBreakIndicator(m Model) {
	if m.indicatorVisible() {
		x := lipgloss.Width(m.headerBase(len(l.tabs) > 0)) + 2
		l.breakIndicator = rect{x, 0, lipgloss.Width(indicatorText(*m.brk, m.now)), 1}
	}
}
