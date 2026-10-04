package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"nexus/internal/tracking"
)

func ended(seconds int64) *int64 { return &seconds }

func newRowsStore() *testStore {
	return &testStore{
		projects: []tracking.ProjectUsage{{Name: "nexus"}, {Name: "tbwa"}},
		running:  []tracking.Entry{{ID: 1, TaskUID: "t1", Title: "Corriendo", Project: "nexus", StartedAt: 100}},
		recent: []tracking.Entry{
			{ID: 2, Title: "Vieja A", Project: "tbwa", StartedAt: 10, EndedAt: ended(50)},
			{ID: 3, Title: "Vieja B", StartedAt: 5, EndedAt: ended(9)},
		},
	}
}

func newRowsModel(db *testStore) Model {
	m := NewModel(db)
	m.width, m.height = 100, 40
	return m
}

func press(t *testing.T, m Model, types ...tea.KeyType) Model {
	t.Helper()
	for _, typ := range types {
		m = updateKey(t, m, tea.KeyMsg{Type: typ})
	}
	return m
}

// goTo baja con ↓ hasta que el foco llegue a target (y la fila indicada).
func goTo(t *testing.T, m Model, target focusTarget, row int) Model {
	t.Helper()
	for i := 0; i < 30; i++ {
		if m.focus == target && (target != focusRecent || m.focusedRecent == row) && (target != focusRunningStart || m.focusedRunning == row) {
			return m
		}
		m = press(t, m, tea.KeyDown)
	}
	t.Fatalf("no se llegó al foco %v fila %d (foco=%v)", target, row, m.focus)
	return m
}

func TestFocusRingReachesRecentRows(t *testing.T) {
	m := newRowsModel(newRowsStore())
	want := []focusTarget{focusProject, focusDescription, focusStart, focusRunningStart, focusToday, focusRecent, focusRecent, focusRecent}
	for i, target := range want {
		m = press(t, m, tea.KeyDown)
		if m.focus != target {
			t.Fatalf("paso %d: foco = %v, quería %v", i, m.focus, target)
		}
	}
	if m.focusedRecent != 2 {
		t.Fatalf("fila reciente = %d, quería 2", m.focusedRecent)
	}
	m = press(t, m, tea.KeyDown)
	if m.focus != focusRecent || m.focusedRecent != 2 {
		t.Fatal("el foco debe detenerse en la última fila reciente")
	}
}

func TestLeftRightSwitchesRowButtons(t *testing.T) {
	m := goTo(t, newRowsModel(newRowsStore()), focusRunningStart, 0)
	if m.rowButton != 0 {
		t.Fatalf("el primer botón debe estar resaltado, rowButton=%d", m.rowButton)
	}
	m = press(t, m, tea.KeyRight, tea.KeyRight)
	if m.rowButton != 1 {
		t.Fatalf("rowButton = %d, quería 1", m.rowButton)
	}
	m = press(t, m, tea.KeyLeft)
	if m.rowButton != 0 {
		t.Fatalf("rowButton = %d, quería 0", m.rowButton)
	}
	m = goTo(t, m, focusRecent, 0)
	if m.rowButton != 0 {
		t.Fatal("al enfocar otra fila el primer botón vuelve a resaltarse")
	}
}

func TestEnterOnEditOpensPrefilledPanel(t *testing.T) {
	db := newRowsStore()
	db.recent[0].Description = "detalle"
	m := goTo(t, newRowsModel(db), focusRecent, 1)
	m = press(t, m, tea.KeyRight, tea.KeyEnter)
	view := m.View()
	for _, text := range []string{"Editar tarea #2", "[Guardar]", "[Eliminar]", "[Cancelar]"} {
		if !strings.Contains(view, text) {
			t.Fatalf("falta %q en la vista:\n%s", text, view)
		}
	}
	if m.inputs[0].Value() != "Vieja A" || m.inputs[1].Value() != "tbwa" || m.inputs[2].Value() != "detalle" {
		t.Fatalf("campos sin precargar: %q %q %q", m.inputs[0].Value(), m.inputs[1].Value(), m.inputs[2].Value())
	}
	if len(db.edits) != 0 {
		t.Fatal("abrir el panel no debe editar")
	}
}

func TestSaveSendsOnlyChangedFields(t *testing.T) {
	db := newRowsStore()
	m := goTo(t, newRowsModel(db), focusRecent, 1)
	m = press(t, m, tea.KeyRight, tea.KeyEnter)
	m.inputs[0].SetValue("Nueva")
	m = press(t, m, tea.KeyEnter)
	if len(db.edits) != 1 {
		t.Fatalf("ediciones = %d", len(db.edits))
	}
	got := db.edits[0]
	if got.task != "t2" || got.input.Title == nil || *got.input.Title != "Nueva" || got.input.Project != nil || got.input.Description != nil {
		t.Fatalf("edición = %#v", got)
	}
	if m.message != "Guardado: Nueva" || strings.Contains(m.View(), "Editar tarea") {
		t.Fatalf("estado tras guardar: %q", m.message)
	}
}

func TestSaveSendsChangedProjectAndDescription(t *testing.T) {
	db := newRowsStore()
	m := goTo(t, newRowsModel(db), focusRecent, 1)
	m = press(t, m, tea.KeyRight, tea.KeyEnter, tea.KeyDown, tea.KeyRight) // proyecto: tbwa → "Sin proyecto"
	m = press(t, m, tea.KeyDown)
	m.inputs[2].SetValue("nota")
	m = press(t, m, tea.KeyEnter)
	got := db.edits[0].input
	if got.Title != nil || got.Project == nil || *got.Project != "" || got.Description == nil || *got.Description != "nota" {
		t.Fatalf("edición = %#v", got)
	}
}

func TestEscCancelsEditWithoutCallingEdit(t *testing.T) {
	db := newRowsStore()
	m := goTo(t, newRowsModel(db), focusRecent, 0)
	m = press(t, m, tea.KeyRight, tea.KeyEnter)
	m.inputs[0].SetValue("Cambiado")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if cmd != nil || len(db.edits) != 0 || strings.Contains(m.View(), "Editar tarea") {
		t.Fatalf("Esc debe cerrar sin editar ni salir: edits=%v", db.edits)
	}
	if m.inputs[0].Value() != "" {
		t.Fatalf("el borrador del formulario principal debe restaurarse, título=%q", m.inputs[0].Value())
	}
}

func TestEscClosesPickerBeforeEditPanel(t *testing.T) {
	m := goTo(t, newRowsModel(newRowsStore()), focusRecent, 1)
	m = press(t, m, tea.KeyRight, tea.KeyEnter, tea.KeyDown, tea.KeyEnter)
	if !m.pickerOpen {
		t.Fatal("Enter en Proyecto debe abrir la lista")
	}
	m = press(t, m, tea.KeyEsc)
	if m.pickerOpen || !strings.Contains(m.View(), "Editar tarea #2") {
		t.Fatal("el primer Esc cierra solo la lista")
	}
}

func TestEditErrorsAreShownInSpanish(t *testing.T) {
	db := newRowsStore()
	db.editErr = tracking.ErrEmptyTitle
	m := goTo(t, newRowsModel(db), focusRecent, 0)
	m = press(t, m, tea.KeyRight, tea.KeyEnter)
	m.inputs[0].SetValue("x")
	m = press(t, m, tea.KeyEnter)
	if m.message != "El título no puede estar vacío" || !strings.Contains(m.View(), "Editar tarea") {
		t.Fatalf("mensaje = %q", m.message)
	}
	db.editErr = tracking.ErrNotFound
	m = press(t, m, tea.KeyEnter)
	if m.message != "La tarea ya no existe" {
		t.Fatalf("mensaje = %q", m.message)
	}
}

func TestDeleteRequiresConfirmationAndDefaultsToCancel(t *testing.T) {
	db := newRowsStore()
	m := goTo(t, newRowsModel(db), focusRecent, 1)
	m = press(t, m, tea.KeyRight, tea.KeyRight, tea.KeyEnter)
	view := m.View()
	if !strings.Contains(view, "¿Eliminar «Vieja A» y su sesión? Quedará en la papelera.") || !strings.Contains(view, "[Eliminar]") || !strings.Contains(view, "[Cancelar]") {
		t.Fatalf("falta la confirmación:\n%s", view)
	}
	if len(db.deleted) != 0 {
		t.Fatal("no debe eliminar antes de confirmar")
	}
	m = press(t, m, tea.KeyEnter) // Cancelar es el botón por defecto
	if len(db.deleted) != 0 || strings.Contains(m.View(), "¿Eliminar") {
		t.Fatal("Enter sobre Cancelar no elimina y cierra la confirmación")
	}
	m = press(t, m, tea.KeyEnter)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if cmd != nil || len(db.deleted) != 0 || strings.Contains(m.View(), "¿Eliminar") {
		t.Fatal("Esc cancela la confirmación sin salir")
	}
}

func TestConfirmDeletesThenUndoByButtonAndCtrlZ(t *testing.T) {
	db := newRowsStore()
	m := goTo(t, newRowsModel(db), focusRecent, 1)
	m = press(t, m, tea.KeyRight, tea.KeyRight, tea.KeyEnter, tea.KeyLeft, tea.KeyEnter)
	if len(db.deleted) != 1 || db.deleted[0] != "t2" {
		t.Fatalf("eliminadas = %v", db.deleted)
	}
	if !strings.Contains(m.View(), "Eliminada «Vieja A» · [Deshacer]") {
		t.Fatalf("falta el aviso con Deshacer:\n%s", m.View())
	}
	m = goTo(t, m, focusUndo, 0)
	m = press(t, m, tea.KeyEnter)
	if len(db.restored) != 1 || db.restored[0] != "t2" || len(m.recent) != 3 {
		t.Fatalf("restauradas = %v recientes = %d", db.restored, len(m.recent))
	}
	// Ctrl+Z restaura la última eliminada durante la sesión; el fake devuelve la restaurada al final.
	m = goTo(t, m, focusRecent, 1)
	m = press(t, m, tea.KeyRight, tea.KeyRight, tea.KeyEnter, tea.KeyLeft, tea.KeyEnter)
	if len(db.deleted) != 2 {
		t.Fatalf("eliminadas = %v", db.deleted)
	}
	m = press(t, m, tea.KeyCtrlZ)
	if len(db.restored) != 2 || len(m.recent) != 3 || m.message != "Restaurada «Vieja B»" {
		t.Fatalf("Ctrl+Z: restauradas=%v mensaje=%q", db.restored, m.message)
	}
}

func TestUndoButtonExpiresButCtrlZStillWorks(t *testing.T) {
	db := newRowsStore()
	m := goTo(t, newRowsModel(db), focusRecent, 1)
	m = press(t, m, tea.KeyRight, tea.KeyRight, tea.KeyEnter, tea.KeyLeft, tea.KeyEnter)
	m.undoUntil = m.now.Add(-1)
	m.refresh()
	if strings.Contains(m.View(), "[Deshacer]") {
		t.Fatal("el botón debe desaparecer a los 10 s")
	}
	m = press(t, m, tea.KeyCtrlZ)
	if len(db.restored) != 1 {
		t.Fatal("Ctrl+Z sigue restaurando")
	}
}

func TestEditRunningEntryKeepsItRunning(t *testing.T) {
	db := newRowsStore()
	m := goTo(t, newRowsModel(db), focusRunningStart, 0)
	m = press(t, m, tea.KeyEnter)
	if !strings.Contains(m.View(), "Editar tarea #1") {
		t.Fatal("no abrió el panel del temporizador")
	}
	m.inputs[0].SetValue("Renombrada")
	m = press(t, m, tea.KeyEnter)
	if len(db.stopped) != 0 || len(m.running) != 1 || m.running[0].Title != "Renombrada" {
		t.Fatalf("debe seguir corriendo: detenidos=%v running=%#v", db.stopped, m.running)
	}
}

func TestEditPanelDeleteButtonConfirmsAndCloses(t *testing.T) {
	db := newRowsStore()
	m := goTo(t, newRowsModel(db), focusRecent, 2)
	m = press(t, m, tea.KeyRight, tea.KeyEnter)
	m = press(t, m, tea.KeyDown, tea.KeyDown, tea.KeyDown, tea.KeyRight) // proyecto, descripción, Guardar, → Eliminar
	if m.focus != focusEditDelete {
		t.Fatalf("foco = %v, quería el botón Eliminar", m.focus)
	}
	m = press(t, m, tea.KeyEnter)
	if !strings.Contains(m.View(), "¿Eliminar «Vieja B» y su sesión?") || len(db.deleted) != 0 {
		t.Fatal("falta la confirmación")
	}
	m = press(t, m, tea.KeyEsc)
	if !strings.Contains(m.View(), "Editar tarea #3") {
		t.Fatal("Esc de la confirmación debe volver al panel")
	}
	m = press(t, m, tea.KeyEnter, tea.KeyLeft, tea.KeyEnter)
	if len(db.deleted) != 1 || db.deleted[0] != "t3" || strings.Contains(m.View(), "Editar tarea") {
		t.Fatalf("eliminadas=%v", db.deleted)
	}
}

func TestClickHitTestsRowButtons(t *testing.T) {
	db := newRowsStore()
	m := newRowsModel(db)
	layout := m.computeLayout()
	click := func(m Model, z rect) Model {
		return updateMouse(t, m, tea.MouseMsg{X: z.x, Y: z.y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	}
	m = click(m, layout.recent[1].row)
	if m.focus != focusRecent || m.focusedRecent != 1 {
		t.Fatalf("clic en la fila debe enfocarla, foco=%v", m.focus)
	}
	m = click(m, layout.recent[2].buttons[2])
	if m.confirm == nil || m.confirm.task.TaskUID != "t3" || len(db.deleted) != 0 {
		t.Fatal("clic en ✕ Eliminar abre la confirmación sin borrar")
	}
	layout = m.computeLayout()
	m = click(m, layout.statusButtons[0]) // [Eliminar]
	if len(db.deleted) != 1 || db.deleted[0] != "t3" {
		t.Fatalf("eliminadas = %v", db.deleted)
	}
	layout = m.computeLayout()
	if len(layout.statusButtons) != 1 {
		t.Fatal("falta el botón Deshacer en el layout")
	}
	m = click(m, layout.statusButtons[0])
	if len(db.restored) != 1 || db.restored[0] != "t3" {
		t.Fatalf("restauradas = %v", db.restored)
	}
	layout = m.computeLayout()
	m = click(m, layout.running[0].buttons[0])
	if m.edit == nil || m.edit.entry.ID != 1 {
		t.Fatal("clic en ✎ Editar abre el panel")
	}
	layout = m.computeLayout()
	m = click(m, layout.editButtons[2])
	if m.edit != nil {
		t.Fatal("clic en [Cancelar] cierra el panel")
	}
}

func TestClickConfirmCancelAndEditPanelFields(t *testing.T) {
	db := newRowsStore()
	m := newRowsModel(db)
	click := func(m Model, z rect) Model {
		return updateMouse(t, m, tea.MouseMsg{X: z.x, Y: z.y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	}
	m = click(m, m.computeLayout().recent[1].buttons[2])
	m = click(m, m.computeLayout().statusButtons[1]) // [Cancelar]
	if m.confirm != nil || len(db.deleted) != 0 {
		t.Fatal("Cancelar cierra sin borrar")
	}
	m = click(m, m.computeLayout().recent[1].buttons[1])
	m = click(m, m.computeLayout().fields[2])
	if m.focus != focusDescription {
		t.Fatalf("clic en Descripción, foco=%v", m.focus)
	}
	m = click(m, m.computeLayout().editButtons[1]) // [Eliminar] del panel
	if m.confirm == nil || m.confirm.task.TaskUID != "t2" {
		t.Fatal("[Eliminar] del panel pide confirmación")
	}
}

func TestLayoutMatchesRenderedLines(t *testing.T) {
	m := newRowsModel(newRowsStore())
	m.totals = []tracking.ProjectTotal{{Project: "nexus", Seconds: 60}}
	m = press(t, m, tea.KeyDown) // fuerza recálculo estable
	m = goTo(t, m, focusRecent, 1)
	m = press(t, m, tea.KeyRight, tea.KeyRight, tea.KeyEnter)
	layout := m.computeLayout()
	lines := strings.Split(m.View(), "\n")
	check := func(y int, z rect, text string) {
		t.Helper()
		if y >= len(lines) {
			t.Fatalf("línea %d fuera de la vista", y)
		}
		at := strings.Index(lines[y], text)
		if at < 0 || lipglossWidth(lines[y][:at]) != z.x {
			t.Fatalf("línea %d: %q no está en la columna %d: %q", y, text, z.x, lines[y])
		}
	}
	check(layout.running[0].row.y, layout.running[0].buttons[0], "[✎ Editar]")
	check(layout.recent[2].row.y, layout.recent[2].buttons[2], "[✕ Eliminar]")
	check(layout.statusY, layout.statusButtons[0], "[Eliminar]")
}
