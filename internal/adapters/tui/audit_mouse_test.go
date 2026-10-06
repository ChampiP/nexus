package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestWideRecentFocusSurvivesRefreshOnShortTerminal(t *testing.T) {
	m := NewModel(newRowsStore())
	m.width, m.height = 130, 24
	m.focus, m.focusedRecent = focusRecent, 0

	m.refresh()

	if m.focus != focusRecent || m.focusedRecent != 0 {
		t.Fatalf("tras actualizar, foco reciente = %v fila %d; quería reciente fila 0", m.focus, m.focusedRecent)
	}
}

func TestRenderedBreakAndPeriodMouseZones(t *testing.T) {
	for _, tc := range []struct {
		name  string
		width int
	}{
		{"narrow", 100},
		{"wide", 130},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewModel(&testStore{}, WithBreaks(&fakeBreaks{}))
			m.width, m.height = tc.width, 40
			m.screen = screenBreak
			breakLayout := m.breakLayout()
			breakZone := breakLayout.start
			lines := strings.Split(ansi.Strip(m.View()), "\n")
			if tc.width >= 120 {
				g, _ := m.wideGeometry()
				breakZone.x += g.rightX
			}
			assertZoneMatchesRenderedText(t, lines, breakZone, " Empezar break ")

			m.screen = screenTimers
			for _, focus := range []struct {
				name string
				kind focusTarget
			}{
				{"hoy", focusToday},
				{"semana", focusWeek},
			} {
				t.Run(focus.name, func(t *testing.T) {
					m.focus = focus.kind
					m.week = focus.kind == focusWeek
					if tc.width >= 120 {
						m.wideColumn = 1
					}
					layout := m.computeLayout()
					today, week := layout.today, layout.week
					if tc.width >= 120 {
						g, _ := m.wideGeometry()
						today, week = g.today, g.week
					}
					view := strings.Split(ansi.Strip(m.View()), "\n")
					assertZoneMatchesRenderedText(t, view, today, periodLabel(!m.week, "Hoy"))
					assertZoneMatchesRenderedText(t, view, week, periodLabel(m.week, "Semana"))
				})
			}
		})
	}
}

func periodLabel(selected bool, name string) string {
	if selected {
		return "  " + name + "  "
	}
	return name
}

func assertZoneMatchesRenderedText(t *testing.T, lines []string, zone rect, text string) {
	t.Helper()
	if zone.y < 0 || zone.y >= len(lines) {
		t.Fatalf("zona para %q fuera de la vista: y=%d", text, zone.y)
	}
	line := lines[zone.y]
	at := strings.Index(line, text)
	if at < 0 {
		t.Fatalf("no se encontró %q en la fila %d: %q", text, zone.y, line)
	}
	wantX, wantW := lipgloss.Width(line[:at]), lipgloss.Width(text)
	if zone.x != wantX || zone.w != wantW {
		t.Fatalf("zona de %q = (%d,%d), etiqueta dibujada en (%d,%d)", text, zone.x, zone.w, wantX, wantW)
	}
}
