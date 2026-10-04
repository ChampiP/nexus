package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"nexus/internal/tracking"
)

func testGrid() navGrid {
	return navGrid{
		{section: 0, pick: -1, cells: []navCell{{1, 0, 0}}},
		{section: 1, pick: -1, cells: []navCell{{2, 0, 0}, {2, 0, 1}, {2, 0, 2}}},
		{section: 1, pick: -1, cells: []navCell{{3, 0, 0}}},
		{section: 2, pick: 1, cells: []navCell{{4, 0, 0}, {4, 0, 1}}},
		{section: 3, pick: -1, cells: []navCell{{5, 0, 0}, {5, 0, 1}, {5, 0, 2}}},
	}
}

func TestNavGridVerticalKeepsNearestColumn(t *testing.T) {
	g := testGrid()
	if got := g.vertical(navCell{2, 0, 2}, 1); got != (navCell{3, 0, 0}) {
		t.Fatalf("↓ a una fila de una columna = %+v", got)
	}
	// La fila de radio aterriza en la opción elegida, no en la columna.
	if got := g.vertical(navCell{3, 0, 0}, 1); got != (navCell{4, 0, 1}) {
		t.Fatalf("↓ a una fila radio = %+v", got)
	}
	if got := g.vertical(navCell{4, 0, 1}, 1); got != (navCell{5, 0, 1}) {
		t.Fatalf("↓ conserva la columna = %+v", got)
	}
	if got := g.vertical(navCell{5, 0, 2}, -1); got != (navCell{4, 0, 1}) {
		t.Fatalf("↑ = %+v", got)
	}
	if got := g.vertical(navCell{5, 0, 2}, 1); got != (navCell{5, 0, 2}) {
		t.Fatalf("↓ en la última fila no se mueve: %+v", got)
	}
	if got := g.vertical(navCell{1, 0, 0}, -1); got != (navCell{1, 0, 0}) {
		t.Fatalf("↑ en la primera fila no se mueve: %+v", got)
	}
}

func TestNavGridHorizontalStaysInRow(t *testing.T) {
	g := testGrid()
	if got := g.horizontal(navCell{2, 0, 1}, 1); got != (navCell{2, 0, 2}) {
		t.Fatalf("→ = %+v", got)
	}
	if got := g.horizontal(navCell{2, 0, 2}, 1); got != (navCell{2, 0, 2}) {
		t.Fatalf("→ sin vecino no salta ni envuelve: %+v", got)
	}
	if got := g.horizontal(navCell{2, 0, 0}, -1); got != (navCell{2, 0, 0}) {
		t.Fatalf("← sin vecino: %+v", got)
	}
}

func TestNavGridSectionsAndEnds(t *testing.T) {
	g := testGrid()
	if got := g.section(navCell{2, 0, 2}, 1); got != (navCell{4, 0, 1}) {
		t.Fatalf("PgDn salta a la siguiente sección: %+v", got)
	}
	if got := g.section(navCell{3, 0, 0}, 1); got != (navCell{4, 0, 1}) {
		t.Fatalf("PgDn desde la 2.ª fila de la sección: %+v", got)
	}
	if got := g.section(navCell{5, 0, 2}, 1); got != (navCell{5, 0, 2}) {
		t.Fatalf("PgDn en la última sección: %+v", got)
	}
	if got := g.section(navCell{4, 0, 0}, -1); got != (navCell{2, 0, 0}) {
		t.Fatalf("PgUp salta al primer elemento de la sección anterior: %+v", got)
	}
	if got := g.section(navCell{1, 0, 0}, -1); got != (navCell{1, 0, 0}) {
		t.Fatalf("PgUp en la primera sección: %+v", got)
	}
	if got := g.ends(true); got != (navCell{1, 0, 0}) {
		t.Fatalf("Home = %+v", got)
	}
	if got := g.ends(false); got != (navCell{5, 0, 2}) {
		t.Fatalf("End = %+v", got)
	}
}

func TestNavGridLinearSkipsUnpickedRadioCells(t *testing.T) {
	g := testGrid()
	if got := g.linear(navCell{3, 0, 0}, 1); got != (navCell{4, 0, 1}) {
		t.Fatalf("Tab = %+v", got)
	}
	if got := g.linear(navCell{4, 0, 1}, 1); got != (navCell{5, 0, 0}) {
		t.Fatalf("Tab tras el radio = %+v", got)
	}
	if got := g.linear(navCell{5, 0, 0}, -1); got != (navCell{4, 0, 1}) {
		t.Fatalf("Shift+Tab = %+v", got)
	}
}

func TestInputEdgeLeavesOnlyWhenEmptyOrAtEdge(t *testing.T) {
	m := newRowsModel(newRowsStore())
	in := m.inputs[0]
	if !inputAtEdge(in, tea.KeyLeft) || !inputAtEdge(in, tea.KeyRight) {
		t.Fatal("un campo vacío cede ambas flechas")
	}
	in.SetValue("hola")
	in.SetCursor(2)
	if inputAtEdge(in, tea.KeyLeft) || inputAtEdge(in, tea.KeyRight) {
		t.Fatal("con el cursor en medio las flechas mueven el cursor")
	}
	in.SetCursor(0)
	if !inputAtEdge(in, tea.KeyLeft) || inputAtEdge(in, tea.KeyRight) {
		t.Fatal("en el borde izquierdo solo ← sale")
	}
	in.CursorEnd()
	if inputAtEdge(in, tea.KeyLeft) || !inputAtEdge(in, tea.KeyRight) {
		t.Fatal("en el borde derecho solo → sale")
	}
}

func TestTextInputKeepsArrowsForCursor(t *testing.T) {
	m := newRowsModel(newRowsStore())
	m = typeText(t, m, "hola")
	m = press(t, m, tea.KeyLeft, tea.KeyLeft)
	if m.focus != focusTitle || m.inputs[0].Position() != 2 {
		t.Fatalf("← debe mover el cursor: foco=%v pos=%d", m.focus, m.inputs[0].Position())
	}
}

// timersWithTabs es la pantalla de temporizadores con pestañas, un temporizador y dos recientes.
func timersWithTabs() Model {
	m := NewModel(newRowsStore(), WithBreaks(&fakeBreaks{}), WithCatalog(newFakeCatalog()))
	m.width, m.height = 100, 40
	return m
}

func TestTimersVerticalMovesKeepColumn(t *testing.T) {
	m := goTo(t, newRowsModel(newRowsStore()), focusRecent, 0)
	m = press(t, m, tea.KeyRight, tea.KeyRight)
	m = press(t, m, tea.KeyDown)
	if m.focus != focusRecent || m.focusedRecent != 1 || m.rowButton != 2 {
		t.Fatalf("↓ debe conservar la columna: fila=%d botón=%d", m.focusedRecent, m.rowButton)
	}
	m = press(t, m, tea.KeyUp)
	if m.focusedRecent != 0 || m.rowButton != 2 {
		t.Fatalf("↑ debe conservar la columna: fila=%d botón=%d", m.focusedRecent, m.rowButton)
	}
	m = goTo(t, newRowsModel(newRowsStore()), focusRunningStart, 0)
	m = press(t, m, tea.KeyRight, tea.KeyUp)
	if m.focus != focusStart {
		t.Fatalf("↑ desde el botón Detener = %v, quería Iniciar", m.focus)
	}
}

func TestTimersHorizontalOnEveryRow(t *testing.T) {
	m := timersWithTabs()
	m = press(t, m, tea.KeyDown, tea.KeyDown, tea.KeyDown) // tabs ya no: título → … → Iniciar
	if m.focus != focusStart {
		t.Fatalf("foco = %v", m.focus)
	}
	m = press(t, m, tea.KeyLeft, tea.KeyRight)
	if m.focus != focusStart {
		t.Fatal("[Iniciar] no tiene vecinos en su fila")
	}
	m = goTo(t, m, focusToday, 0)
	m = press(t, m, tea.KeyRight)
	if m.focus != focusWeek || !m.week {
		t.Fatalf("→ en Hoy: foco=%v semana=%v", m.focus, m.week)
	}
	m = press(t, m, tea.KeyRight)
	if m.focus != focusWeek {
		t.Fatal("→ sin vecino no debe envolver")
	}
	m = press(t, m, tea.KeyLeft)
	if m.focus != focusToday || m.week {
		t.Fatalf("← en Semana: foco=%v semana=%v", m.focus, m.week)
	}
	m = goTo(t, m, focusRecent, 0)
	m = press(t, m, tea.KeyRight, tea.KeyRight, tea.KeyRight)
	if m.rowButton != 2 {
		t.Fatalf("→ se detiene en el último botón: %d", m.rowButton)
	}
	m = press(t, m, tea.KeyLeft, tea.KeyLeft, tea.KeyLeft)
	if m.rowButton != 0 {
		t.Fatalf("← se detiene en el primero: %d", m.rowButton)
	}
}

func TestTimersDownFromPeriodLandsOnSelectedOption(t *testing.T) {
	m := goTo(t, newRowsModel(newRowsStore()), focusToday, 0)
	m = press(t, m, tea.KeyRight, tea.KeyUp, tea.KeyDown)
	if m.focus != focusWeek || !m.week {
		t.Fatalf("la fila Hoy/Semana debe recibir el foco en la opción elegida: %v", m.focus)
	}
}

func TestTimersPageKeysJumpBetweenSections(t *testing.T) {
	m := timersWithTabs()
	want := []struct {
		focus focusTarget
		row   int
	}{{focusRunningStart, 0}, {focusToday, 0}, {focusRecent, 0}}
	for i, w := range want {
		m = press(t, m, tea.KeyPgDown)
		if m.focus != w.focus || (w.focus == focusRunningStart && m.focusedRunning != w.row) || (w.focus == focusRecent && m.focusedRecent != w.row) {
			t.Fatalf("PgDn %d: foco=%v", i, m.focus)
		}
	}
	m = press(t, m, tea.KeyPgDown)
	if m.focus != focusRecent || m.focusedRecent != 0 {
		t.Fatalf("PgDn en la última sección no se mueve: %v", m.focus)
	}
	m = press(t, m, tea.KeyPgUp)
	if m.focus != focusToday {
		t.Fatalf("PgUp → %v, quería Hoy", m.focus)
	}
	m = press(t, m, tea.KeyPgUp, tea.KeyPgUp)
	if m.focus != focusTitle {
		t.Fatalf("PgUp ×2 → %v, quería el primer campo del formulario", m.focus)
	}
	m = press(t, m, tea.KeyPgUp)
	if m.focus != focusTabs {
		t.Fatalf("PgUp → %v, quería las pestañas", m.focus)
	}
}

func TestTimersHomeAndEnd(t *testing.T) {
	m := goTo(t, timersWithTabs(), focusRunningStart, 0)
	m = press(t, m, tea.KeyEnd)
	if m.focus != focusRecent || m.focusedRecent != 1 || m.rowButton != 2 {
		t.Fatalf("End: foco=%v fila=%d botón=%d", m.focus, m.focusedRecent, m.rowButton)
	}
	m = press(t, m, tea.KeyHome)
	if m.focus != focusTabs {
		t.Fatalf("Home → %v, quería las pestañas", m.focus)
	}
	m = NewModel(newRowsStore())
	m.width, m.height = 100, 40
	m = press(t, m, tea.KeyEnd, tea.KeyHome)
	if m.focus != focusTitle {
		t.Fatalf("sin pestañas Home → %v, quería el título", m.focus)
	}
}

func TestEditPanelNavigation(t *testing.T) {
	m := goTo(t, newRowsModel(newRowsStore()), focusRecent, 0)
	m = press(t, m, tea.KeyRight, tea.KeyEnter)
	if m.edit == nil {
		t.Fatal("el panel de edición no se abrió")
	}
	m = press(t, m, tea.KeyPgDown)
	if m.focus != focusEditSave {
		t.Fatalf("PgDn → %v, quería Guardar", m.focus)
	}
	m = press(t, m, tea.KeyRight, tea.KeyRight, tea.KeyRight)
	if m.focus != focusEditCancel {
		t.Fatalf("→ → foco = %v, quería Cancelar", m.focus)
	}
	m = press(t, m, tea.KeyUp)
	if m.focus != focusDescription {
		t.Fatalf("↑ desde los botones = %v", m.focus)
	}
	m = press(t, m, tea.KeyEnd)
	if m.focus != focusEditCancel {
		t.Fatalf("End = %v", m.focus)
	}
	m = press(t, m, tea.KeyHome)
	if m.focus != focusTitle {
		t.Fatalf("Home = %v", m.focus)
	}
}

func TestUndoIsLastStop(t *testing.T) {
	db := newRowsStore()
	m := goTo(t, newRowsModel(db), focusRecent, 0)
	m = press(t, m, tea.KeyRight, tea.KeyRight, tea.KeyEnter, tea.KeyLeft, tea.KeyEnter)
	if !m.undoActive() {
		t.Fatal("tras eliminar debe haber Deshacer")
	}
	m = press(t, m, tea.KeyEnd)
	if m.focus != focusUndo {
		t.Fatalf("End → %v, quería Deshacer", m.focus)
	}
}

// Catálogo

func TestCatalogNavigation(t *testing.T) {
	m, _, _ := newCatalogModel(t)
	m = openCatalog(t, m)
	m = press(t, m, tea.KeyPgDown)
	if m.cat.focus != catFocusRow || m.cat.rows[m.cat.row].name != "Holinsys" {
		t.Fatalf("PgDn desde las pestañas → %v %q", m.cat.focus, m.cat.rows[m.cat.row].name)
	}
	m = press(t, m, tea.KeyPgDown)
	if m.cat.focus != catFocusAdd || m.cat.button != 0 {
		t.Fatalf("PgDn desde el árbol → %v botón %d", m.cat.focus, m.cat.button)
	}
	m = press(t, m, tea.KeyRight, tea.KeyRight, tea.KeyDown)
	if m.cat.focus != catFocusToggle {
		t.Fatalf("↓ desde las altas → %v", m.cat.focus)
	}
	m = press(t, m, tea.KeyUp)
	if m.cat.focus != catFocusAdd || m.cat.button != 0 {
		t.Fatalf("↑ desde el interruptor → %v botón %d", m.cat.focus, m.cat.button)
	}
	m = press(t, m, tea.KeyPgUp)
	if m.cat.focus != catFocusRow || m.cat.rows[m.cat.row].name != "Holinsys" {
		t.Fatalf("PgUp → %v", m.cat.focus)
	}
	m = press(t, m, tea.KeyEnd)
	if m.cat.focus != catFocusToggle {
		t.Fatalf("End → %v", m.cat.focus)
	}
	m = press(t, m, tea.KeyHome)
	if m.cat.focus != catFocusTabs {
		t.Fatalf("Home → %v", m.cat.focus)
	}
}

func TestCatalogVerticalKeepsColumnAcrossRows(t *testing.T) {
	m, _, _ := newCatalogModel(t)
	m = openCatalog(t, m)
	m = focusCatalogRow(t, m, "Depiloto")
	m = press(t, m, tea.KeyRight, tea.KeyRight, tea.KeyRight)
	m = press(t, m, tea.KeyUp)
	if m.cat.rows[m.cat.row].name != "Depilab" || m.cat.button != 2 {
		t.Fatalf("↑ → %q botón %d", m.cat.rows[m.cat.row].name, m.cat.button)
	}
	m = press(t, m, tea.KeyUp)
	if m.cat.rows[m.cat.row].name != "Holinsys" || m.cat.button != 1 {
		t.Fatalf("↑ a una fila con menos botones → %q botón %d", m.cat.rows[m.cat.row].name, m.cat.button)
	}
	m = press(t, m, tea.KeyDown)
	if m.cat.button != 1 {
		t.Fatalf("↓ → botón %d", m.cat.button)
	}
}

func TestCatalogAddRowAndToggleHorizontal(t *testing.T) {
	m, _, _ := newCatalogModel(t)
	m = openCatalog(t, m)
	for m.cat.focus != catFocusAdd {
		m = press(t, m, tea.KeyDown)
	}
	m = press(t, m, tea.KeyRight, tea.KeyRight, tea.KeyRight)
	if m.cat.button != 2 {
		t.Fatalf("→ en las altas: botón %d", m.cat.button)
	}
	m = press(t, m, tea.KeyDown, tea.KeyLeft, tea.KeyRight)
	if m.cat.focus != catFocusToggle {
		t.Fatal("el interruptor no tiene vecinos en su fila")
	}
}

// Break

func TestBreakFormNavigation(t *testing.T) {
	m := onBreakPage(t, twoTimers(), &fakeBreaks{})
	m = press(t, m, tea.KeyPgDown)
	if m.bp.onTabs || m.bp.form.row != breakRowDuration {
		t.Fatalf("PgDn desde las pestañas: fila %d", m.bp.form.row)
	}
	m = press(t, m, tea.KeyPgDown)
	if m.bp.form.row != breakRowFirst {
		t.Fatalf("PgDn → fila %d, quería la lista «Detener»", m.bp.form.row)
	}
	m = press(t, m, tea.KeyPgDown)
	if m.bp.form.row != m.bp.form.buttonsRow() {
		t.Fatalf("PgDn → fila %d, quería el botón", m.bp.form.row)
	}
	m = press(t, m, tea.KeyPgUp, tea.KeyPgUp)
	if m.bp.form.row != breakRowDuration {
		t.Fatalf("PgUp ×2 → fila %d", m.bp.form.row)
	}
	m = press(t, m, tea.KeyEnd)
	if m.bp.form.row != m.bp.form.buttonsRow() {
		t.Fatalf("End → fila %d", m.bp.form.row)
	}
	m = press(t, m, tea.KeyHome)
	if !m.bp.onTabs {
		t.Fatal("Home debe ir a las pestañas")
	}
}

func TestBreakDurationsHorizontalAndVerticalKeepChoice(t *testing.T) {
	m := onBreakPage(t, &testStore{}, &fakeBreaks{})
	m = press(t, m, tea.KeyDown)
	m = press(t, m, tea.KeyLeft)
	if m.bp.form.choice != defaultBreakChoice-1 {
		t.Fatalf("← → elección %d", m.bp.form.choice)
	}
	m = press(t, m, tea.KeyLeft, tea.KeyLeft)
	if m.bp.form.choice != 0 {
		t.Fatalf("← se detiene en la primera: %d", m.bp.form.choice)
	}
	m = press(t, m, tea.KeyDown, tea.KeyUp)
	if m.bp.form.row != breakRowDuration || m.bp.form.choice != 0 {
		t.Fatalf("bajar y subir no debe cambiar la duración: fila %d elección %d", m.bp.form.row, m.bp.form.choice)
	}
}

func TestBreakActiveCardNavigation(t *testing.T) {
	m := onBreakPage(t, &testStore{}, &fakeBreaks{active: activeBreak(10 * 60e9)})
	m = press(t, m, tea.KeyDown)
	if m.bp.onTabs || m.bp.action != 0 {
		t.Fatalf("↓ desde las pestañas: acción %d", m.bp.action)
	}
	m = press(t, m, tea.KeyRight, tea.KeyRight, tea.KeyRight)
	if m.bp.action != 2 {
		t.Fatalf("→ se detiene en el último botón: %d", m.bp.action)
	}
	m = press(t, m, tea.KeyUp)
	if !m.bp.onTabs {
		t.Fatal("↑ debe volver a las pestañas")
	}
	m = press(t, m, tea.KeyPgDown)
	if m.bp.onTabs || m.bp.action != 0 {
		t.Fatalf("PgDn → acción %d", m.bp.action)
	}
	m = press(t, m, tea.KeyEnd)
	if m.bp.action != 2 {
		t.Fatalf("End → acción %d", m.bp.action)
	}
}

var _ = tracking.Entry{}
