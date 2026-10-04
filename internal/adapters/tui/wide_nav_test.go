package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestWideNavigationKeepsColumnAndRememberedCells(t *testing.T) {
	m := timersWithTabs()
	m.width = 120
	grid := m.wideNavigationGrid()
	lastRow := grid.columns[0][len(grid.columns[0])-1]
	lastLeft := lastRow.cells[len(lastRow.cells)-1]
	row, col := grid.columns[0].locate(lastLeft)
	center := grid.columns[1].landing(min(row, len(grid.columns[1])-1), col)
	grid.last[0] = lastLeft
	grid.hasLast[0] = true
	got, column, ok := grid.move(lastLeft, 0, tea.KeyRight)
	if !ok || column != 1 || got != center {
		t.Fatalf("→ en el borde izquierdo = (%+v, %d, %v), quería centro", got, column, ok)
	}
	got, column, ok = grid.move(got, column, tea.KeyLeft)
	if column == 1 {
		got, column, ok = grid.move(got, column, tea.KeyLeft)
	}
	if !ok || column != 0 || got != lastLeft {
		t.Fatalf("← debe volver a la celda recordada = (%+v, %d, %v)", got, column, ok)
	}
	got, column, _ = grid.move(grid.columns[0][0].cells[0], 0, tea.KeyDown)
	if column != 0 || grid.columnFor(got, column) != 0 {
		t.Fatalf("↓ salió de la columna: (%+v, %d)", got, column)
	}
}

func TestWideNavigationLinearSectionsAndEndsAreColumnScoped(t *testing.T) {
	m := timersWithTabs()
	m.width = 120
	grid := m.wideNavigationGrid()
	left := grid.columns[0][0].cells[0]
	got, column, _ := grid.move(left, 0, tea.KeyTab)
	if column != 0 || got != grid.columns[0][1].cells[0] {
		t.Fatalf("Tab en izquierda = (%+v, %d)", got, column)
	}
	lastRow := grid.columns[0][len(grid.columns[0])-1]
	lastLeft := lastRow.cells[len(lastRow.cells)-1]
	got, column, _ = grid.move(lastLeft, 0, tea.KeyTab)
	if column != 1 || got != grid.columns[1][0].cells[0] {
		t.Fatalf("Tab cruza a centro = (%+v, %d)", got, column)
	}
	got, column, _ = grid.move(grid.columns[1][0].cells[0], 1, tea.KeyHome)
	if column != 1 || got != grid.columns[1][0].cells[0] {
		t.Fatalf("Home debe quedar en el principio de centro: (%+v, %d)", got, column)
	}
	if _, column, _ := grid.move(lastLeft, 0, tea.KeyEnd); column != 0 {
		t.Fatalf("End salió de la columna izquierda: %d", column)
	}
}

func TestWideNavigationHeaderAndDangerousPauseTransition(t *testing.T) {
	m := timersWithTabs()
	m.width = 120
	grid := m.wideNavigationGrid()
	top := grid.columns[0][0].cells[0]
	got, column, _ := grid.move(top, 0, tea.KeyUp)
	if got.kind != int(focusTabs) || column != 0 {
		t.Fatalf("↑ desde el principio debe alcanzar Catálogo: (%+v, %d)", got, column)
	}

	m.wellbeing = &fakeWellbeing{}
	m.loadWellbeing()
	grid = m.wideNavigationGrid()
	pauseRows := grid.columns[2]
	var duration navCell
	for _, row := range pauseRows {
		if row.cells[0].kind == brkWellbeing && row.cells[0].index == 2 {
			duration = row.cells[0]
		}
	}
	got, column, _ = grid.move(duration, 2, tea.KeyDown)
	if column != 2 || got.kind != brkWellbeing || got.index != 3 || got.col != 0 {
		t.Fatalf("↓ desde Dura debe llegar al primer botón de pausa, no al segundo: (%+v, %d)", got, column)
	}
}

func TestWideTabWalksColumnsAndSectionsStayLocal(t *testing.T) {
	m := timersWithTabs()
	m.width = 120
	grid := m.wideNavigationGrid()
	leftEnd := grid.columns[0][len(grid.columns[0])-1]
	last := leftEnd.cells[len(leftEnd.cells)-1]
	center, column, _ := grid.move(last, 0, tea.KeyTab)
	if column != 1 || center != grid.columns[1][0].cells[0] {
		t.Fatalf("Tab al final izquierda = (%+v, %d)", center, column)
	}
	period := grid.columns[1][0].cells[0]
	recent := grid.columns[1][len(grid.columns[1])-1].cells[0]
	toRecent, column, _ := grid.move(period, 1, tea.KeyPgDown)
	if column != 1 || grid.columnFor(toRecent, column) != 1 || toRecent.kind != int(focusRecent) {
		t.Fatalf("PgDn del centro debe saltar a RECIENTES sin salir: (%+v, %d)", toRecent, column)
	}
	first, column, _ := grid.move(recent, 1, tea.KeyHome)
	if column != 1 || first != period {
		t.Fatalf("Home en recientes debe quedarse en centro: (%+v, %d)", first, column)
	}
	lastRow := grid.columns[1][len(grid.columns[1])-1]
	last, column, _ = grid.move(period, 1, tea.KeyEnd)
	if column != 1 || last != lastRow.cells[len(lastRow.cells)-1] {
		t.Fatalf("End en centro = (%+v, %d)", last, column)
	}
	endSection, column, _ := grid.move(recent, 1, tea.KeyPgDown)
	if column != 1 || endSection != recent {
		t.Fatalf("PgDn no debe salir del centro: (%+v, %d)", endSection, column)
	}
}

func TestWideTitleInputKeepsCursorUntilItsEdge(t *testing.T) {
	m := timersWithTabs()
	m.width = 120
	m.inputs[0].SetValue("nexus")
	m.inputs[0].SetCursor(2)
	updated, _ := m.update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(Model)
	if m.focus != focusTitle || m.inputs[0].Position() != 3 {
		t.Fatalf("→ en medio del título debe mover cursor: foco=%v posición=%d", m.focus, m.inputs[0].Position())
	}
	m.inputs[0].CursorEnd()
	updated, _ = m.update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(Model)
	if m.focus == focusTitle {
		t.Fatal("→ en el borde del título debe poder salir del campo")
	}
}

func TestWideOverdueBreakStartsOnReturnToWork(t *testing.T) {
	breaks := &fakeBreaks{active: activeBreak(-time.Minute)}
	m := NewModel(&testStore{}, WithBreaks(breaks))
	m.width = 120
	if m.screen != screenBreak || m.breakCell() != (navCell{brkAction, 0, 0}) {
		t.Fatalf("break vencido: pantalla=%v foco=%+v", m.screen, m.breakCell())
	}
}
