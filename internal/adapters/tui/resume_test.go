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

func TestRecentButtonsDependOnStoppedState(t *testing.T) {
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

func TestResumeButtonCallsStartLikeAndReports(t *testing.T) {
	db := newRowsStore()
	m := goTo(t, newRowsModel(db), focusRecent, 0)
	m = press(t, m, tea.KeyEnter)
	if len(db.likes) != 1 || db.likes[0] != 2 {
		t.Fatalf("StartLike = %v", db.likes)
	}
	if m.message != "Reanudado: Vieja A" {
		t.Fatalf("mensaje = %q", m.message)
	}
	if len(m.running) != 2 {
		t.Fatalf("el temporizador reanudado debe aparecer en curso: %d", len(m.running))
	}
}

func TestResumeIgnoredWhenIdenticalTaskRunning(t *testing.T) {
	db := newRowsStore()
	db.recent[0] = tracking.Entry{ID: 2, Title: "Corriendo", Project: "nexus", StartedAt: 10, EndedAt: ended(50)}
	m := goTo(t, newRowsModel(db), focusRecent, 0)
	m = press(t, m, tea.KeyEnter)
	if len(db.likes) != 0 {
		t.Fatalf("no debía reanudar: %v", db.likes)
	}
	if m.message != "«Corriendo» ya está en curso" {
		t.Fatalf("mensaje = %q", m.message)
	}
	// Mismo título en otro proyecto no es un duplicado.
	db.recent[0].Project = "tbwa"
	m = updateModel(t, m, tickMsg(time.Now()))
	m = press(t, m, tea.KeyEnter)
	if len(db.likes) != 1 {
		t.Fatalf("otro proyecto debe poder reanudarse: %v", db.likes)
	}
}

func TestResumeByClickUsesLayout(t *testing.T) {
	db := newRowsStore()
	m := newRowsModel(db)
	zone := m.computeLayout().recent[1].buttons[0]
	m = click(t, m, zone)
	if len(db.likes) != 1 || db.likes[0] != 3 {
		t.Fatalf("clic en Reanudar: %v", db.likes)
	}
}

func TestResumedRecentEditAndDeleteKeepWorking(t *testing.T) {
	db := newRowsStore()
	m := goTo(t, newRowsModel(db), focusRecent, 0)
	m = press(t, m, tea.KeyRight, tea.KeyEnter)
	if m.edit == nil || m.edit.entry.ID != 2 {
		t.Fatal("el segundo botón debe ser Editar")
	}
	m = press(t, m, tea.KeyEsc)
	m = goTo(t, m, focusRecent, 0)
	m = press(t, m, tea.KeyRight, tea.KeyRight, tea.KeyEnter)
	if m.confirm == nil || m.confirm.entry.ID != 2 {
		t.Fatal("el tercer botón debe ser Eliminar")
	}
}
