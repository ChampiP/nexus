package tui

import (
	"time"

	"github.com/charmbracelet/lipgloss"
	"nexus/internal/wellbeing"
)

// breakDurationX es el ancho de «▸ Duración  » antes de los botones de duración.
const breakDurationX = 12

// Líneas de contenido de la tarjeta del break activo: etiqueta, estado, barra, horario, reanudación, línea en blanco y botones.
const breakCardLines = 7

// breakLayout reúne las coordenadas de la pantalla de break; dibujo y ratón la comparten.
type breakLayout struct {
	tabs         []rect
	headingY     int
	cardY        int // borde superior del recuadro
	actions      []rect
	durations    []rect
	custom       rect
	checks       []rect
	start        rect
	todayY       int // encabezado «BREAKS DE HOY»
	shown        int // breaks de hoy visibles; los más recientes
	pauseY       int
	pauseOptions [3][]rect
	pauseButtons []rect
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
		reserve := 5
		if m.wellbeing != nil {
			reserve = 13
		}
		l.shown = min(l.shown, max(1, m.height-l.todayY-reserve))
	}
	if m.wellbeing != nil {
		todayLines := len(m.todayLines(l))
		l.pauseY = l.todayY + 1 + todayLines + 1
		l.pauseOptions[0] = buttonRects(l.pauseY+3, recentContentX+19, []string{"Sí", "No"})
		l.pauseOptions[1] = buttonRects(l.pauseY+4, recentContentX+14, []string{"10 min", "20 min", "30 min", "45 min", "60 min"})
		l.pauseOptions[2] = buttonRects(l.pauseY+5, recentContentX+14, []string{"10 s", "15 s", "20 s", "30 s", "1 min", "2 min", "5 min"})
		l.pauseButtons = buttonRects(l.pauseY+6, recentContentX, pauseButtonLabels(m.pauseStatus, m.now))
	}
	return l
}

func pauseButtonLabels(status wellbeing.Status, now time.Time) []string {
	label := "No molestar 1 hora"
	if status.DNDUntil != nil && status.DNDUntil.After(now) {
		label = "Quitar no molestar"
	}
	return []string{label, "Probar ahora"}
}

// fillBreakIndicator ubica el indicador de break al final del encabezado de Temporizadores.
func (l *screenLayout) fillBreakIndicator(m Model) {
	if m.indicatorVisible() {
		x := lipgloss.Width(m.headerBase(len(l.tabs) > 0)) + 2
		l.breakIndicator = rect{x, 0, lipgloss.Width(indicatorText(*m.brk, m.now)), 1}
	}
}
