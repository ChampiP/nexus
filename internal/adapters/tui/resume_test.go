package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"nexus/internal/tracking"
)

// viewLine devuelve la línea y de la vista.
func viewLine(t *testing.T, m Model, y int) string {
	t.Helper()
	lines := strings.Split(m.View(), "\n")
	if y >= len(lines) {
		t.Fatalf("la vista no tiene la línea %d", y)
	}
	return lines[y]
}

func TestRecentOmitsRunningTasksButKeepsStoppedTasks(t *testing.T) {
	m := newRowsModel(newRowsStore())
	if len(m.recent) != 2 || m.recent[0].Title != "Vieja A" || m.recent[1].Title != "Vieja B" {
		t.Fatalf("recientes = %+v, se esperaban solo tareas detenidas", m.recent)
	}
	if _, ok := m.tasks["t1"]; !ok {
		t.Fatal("la tarea en curso debe conservarse en el total de tareas")
	}
}

func TestRecentButtonsAreForStoppedTasks(t *testing.T) {
	m := newRowsModel(newRowsStore())
	layout := m.computeLayout()
	if got := len(layout.recent[0].buttons); got != 3 {
		t.Fatalf("fila detenida: %d botones, quería 3", got)
	}
	if line := viewLine(t, m, layout.recent[0].row.y); !strings.Contains(line, "[▶ Reanudar] [✎ Editar] [✕ Eliminar]") {
		t.Fatalf("fila detenida sin Reanudar primero: %q", line)
	}
	if line := viewLine(t, m, layout.running[0].row.y); !strings.Contains(line, "[✎ Editar] [■ Detener]") || strings.Contains(line, "Reanudar") {
		t.Fatalf("fila en curso cambió: %q", line)
	}
}

func TestResumeButtonCallsResumeAndReports(t *testing.T) {
	db := newRowsStore()
	m := goTo(t, newRowsModel(db), focusRecent, 0)
	m = press(t, m, tea.KeyEnter)
	if len(db.resumed) != 1 || db.resumed[0] != 2 {
		t.Fatalf("Resume = %v", db.resumed)
	}
	if m.message != "Reanudado: Vieja A" {
		t.Fatalf("mensaje = %q", m.message)
	}
	if len(m.running) != 2 {
		t.Fatalf("el temporizador reanudado debe aparecer en curso: %d", len(m.running))
	}
	if len(m.recent) != 1 {
		t.Fatalf("la tarea en curso no debe estar en RECIENTES: %d filas", len(m.recent))
	}
}

func TestResumeReportsTaskAlreadyRunning(t *testing.T) {
	db := newRowsStore()
	m := goTo(t, newRowsModel(db), focusRecent, 0)
	// Otra sesión de la misma tarea arrancó desde fuera (por ejemplo, la CLI) antes de refrescar.
	db.running = append(db.running, tracking.Entry{ID: 50, TaskUID: "t2", Title: "Vieja A", StartedAt: 60})
	m = press(t, m, tea.KeyEnter)
	if len(db.resumed) != 0 {
		t.Fatalf("no debía reanudar: %v", db.resumed)
	}
	if m.message != "«Vieja A» ya está en curso" {
		t.Fatalf("mensaje = %q", m.message)
	}
}

func TestResumeByClickUsesLayout(t *testing.T) {
	db := newRowsStore()
	m := newRowsModel(db)
	zone := m.computeLayout().recent[1].buttons[0]
	m = click(t, m, zone)
	if len(db.resumed) != 1 || db.resumed[0] != 3 {
		t.Fatalf("clic en Reanudar: %v", db.resumed)
	}
}

func TestRecentEditAndDeleteKeepWorking(t *testing.T) {
	db := newRowsStore()
	m := goTo(t, newRowsModel(db), focusRecent, 0)
	m = press(t, m, tea.KeyRight, tea.KeyEnter)
	if m.edit == nil || m.edit.entry.TaskUID != "t2" {
		t.Fatal("el segundo botón debe ser Editar")
	}
	m = press(t, m, tea.KeyEsc)
	m = goTo(t, m, focusRecent, 0)
	m = press(t, m, tea.KeyRight, tea.KeyRight, tea.KeyEnter)
	if m.confirm == nil || m.confirm.task.TaskUID != "t2" {
		t.Fatal("el tercer botón debe ser Eliminar")
	}
}

// twoSessionStore es una tarea con dos sesiones detenidas (2:15:00 en total) y otra con una.
func twoSessionStore() *testStore {
	return &testStore{
		recent: []tracking.Entry{
			{ID: 4, TaskUID: "A", Title: "Diseño", Project: "nexus", StartedAt: 20000, EndedAt: ended(20000 + 3600)},
			{ID: 2, TaskUID: "A", Title: "Diseño", Project: "nexus", StartedAt: 10000, EndedAt: ended(10000 + 4500)},
			{ID: 3, Title: "Otra", StartedAt: 5, EndedAt: ended(65)},
		},
	}
}

func TestRecentShowsOneRowPerTaskWithTotal(t *testing.T) {
	m := newRowsModel(twoSessionStore())
	if len(m.recent) != 2 {
		t.Fatalf("filas = %d, quería 2 (una por tarea)", len(m.recent))
	}
	line := viewLine(t, m, m.computeLayout().recent[0].row.y)
	if !strings.Contains(line, "Diseño") || !strings.Contains(line, "2:15:00") {
		t.Fatalf("la fila debe mostrar el total de la tarea: %q", line)
	}
}

func TestRunningRowShowsSessionAndTaskTotal(t *testing.T) {
	now := time.Now().Unix()
	db := &testStore{
		running: []tracking.Entry{{ID: 9, TaskUID: "A", Title: "Diseño", StartedAt: now - 11}},
		recent:  []tracking.Entry{{ID: 2, TaskUID: "A", Title: "Diseño", StartedAt: 10000, EndedAt: ended(10000 + 8100)}},
	}
	m := newRowsModel(db)
	line := viewLine(t, m, m.computeLayout().running[0].row.y)
	if !strings.Contains(line, "00:00:11 · total 2:15:") {
		t.Fatalf("EN CURSO debe mostrar sesión y total: %q", line)
	}
}

func TestDeleteConfirmationCountsSessions(t *testing.T) {
	m := goTo(t, newRowsModel(twoSessionStore()), focusRecent, 0)
	m = press(t, m, tea.KeyRight, tea.KeyRight, tea.KeyEnter)
	if view := m.View(); !strings.Contains(view, "¿Eliminar «Diseño» y sus 2 sesiones? Quedará en la papelera.") {
		t.Fatalf("falta la confirmación con sesiones:\n%s", view)
	}
	m = press(t, m, tea.KeyEsc)
	m = goTo(t, m, focusRecent, 1)
	m = press(t, m, tea.KeyRight, tea.KeyRight, tea.KeyEnter)
	if view := m.View(); !strings.Contains(view, "¿Eliminar «Otra» y su sesión? Quedará en la papelera.") {
		t.Fatalf("singular esperado:\n%s", view)
	}
}

func TestDeleteAndUndoWholeTask(t *testing.T) {
	db := twoSessionStore()
	m := goTo(t, newRowsModel(db), focusRecent, 0)
	m = press(t, m, tea.KeyRight, tea.KeyRight, tea.KeyEnter, tea.KeyLeft, tea.KeyEnter)
	if len(db.deleted) != 1 || db.deleted[0] != "A" || len(m.recent) != 1 {
		t.Fatalf("eliminadas = %v, filas = %d", db.deleted, len(m.recent))
	}
	m = press(t, m, tea.KeyCtrlZ)
	if len(db.restored) != 1 || db.restored[0] != "A" || len(m.recent) != 2 {
		t.Fatalf("restauradas = %v, filas = %d", db.restored, len(m.recent))
	}
}

func TestEditTaskAppliesToTaskUID(t *testing.T) {
	db := twoSessionStore()
	m := goTo(t, newRowsModel(db), focusRecent, 0)
	m = press(t, m, tea.KeyRight, tea.KeyEnter)
	m.inputs[0].SetValue("Rediseño")
	m = press(t, m, tea.KeyEnter)
	if len(db.edits) != 1 || db.edits[0].task != "A" {
		t.Fatalf("ediciones = %+v", db.edits)
	}
}
