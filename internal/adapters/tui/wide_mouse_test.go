package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"nexus/internal/tracking"
)

func TestWideMouseZonesMatchRenderedControls(t *testing.T) {
	store := &testStore{running: []tracking.Entry{{ID: 42, Title: "Timer"}}}
	m := NewModel(store)
	m.width, m.height = 160, 40
	view := strings.Split(ansi.Strip(m.View()), "\n")
	_, layout, ok := m.wideMouseLayout()
	if !ok {
		t.Fatal("wide layout unavailable at width 160")
	}

	controls := []struct {
		name  string
		zone  rect
		label string
	}{
		{"title field", layout.fields[0], "Escribe un título"},
		{"project field", layout.fields[1], "‹ Sin proyecto ›"},
		{"description field", layout.fields[2], "Descripción opcional"},
		{"start", layout.start, "[ Iniciar ]"},
		{"stop", layout.running[0].buttons[1], "[■ Detener]"},
		{"today", layout.today, "[ Hoy ]"},
		{"week", layout.week, "Semana"},
	}
	for _, control := range controls {
		t.Run(control.name, func(t *testing.T) {
			assertWideZoneMatchesLabel(t, view, control.zone, control.label)
		})
	}
	m = updateMouse(t, m, tea.MouseMsg{X: layout.week.x, Y: layout.week.y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if !m.week {
		t.Fatal("clicking Semana did not change the dashboard period")
	}
	_, layout, _ = m.wideMouseLayout()
	m = updateMouse(t, m, tea.MouseMsg{X: layout.today.x, Y: layout.today.y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if m.week {
		t.Fatal("clicking Hoy did not change the dashboard period")
	}
}

func assertWideZoneMatchesLabel(t *testing.T, lines []string, zone rect, label string) {
	t.Helper()
	if zone.y < 0 || zone.y >= len(lines) {
		t.Fatalf("zone for %q has y=%d outside rendered view", label, zone.y)
	}
	line := ansi.Strip(lines[zone.y])
	match := label
	if strings.HasPrefix(label, "[") && strings.HasSuffix(label, "]") {
		match = " " + label[1:len(label)-1] + " "
	}
	index := strings.Index(line, match)
	if index < 0 {
		clean := strings.Trim(label, "[]")
		index = strings.Index(line, clean)
		if index < 0 {
			t.Fatalf("label %q not rendered on zone row %d: %q", label, zone.y, line)
		}
	}
	labelX := lipgloss.Width(line[:index])
	labelWidth := lipgloss.Width(match)
	if zone.x > labelX || zone.x+zone.w < labelX+labelWidth {
		t.Fatalf("zone for %q = (%d,%d,%d), rendered at x=%d width=%d", label, zone.x, zone.y, zone.w, labelX, labelWidth)
	}
}

func TestWideMousePickerOptionsAreClickable(t *testing.T) {
	m := NewModel(&testStore{})
	m.width, m.height = 160, 40
	_, layout, _ := m.wideMouseLayout()
	field := layout.fields[1]
	m = updateMouse(t, m, tea.MouseMsg{X: field.x, Y: field.y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	_, layout, _ = m.wideMouseLayout()
	option := layout.options[0]
	assertWideZoneMatchesLabel(t, strings.Split(ansi.Strip(m.View()), "\n"), option, "  ▸ Sin proyecto")
	m = updateMouse(t, m, tea.MouseMsg{X: option.x, Y: option.y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if m.pickerOpen {
		t.Fatal("clicking the rendered picker option did not close the picker")
	}
}

func TestWideMouseStartsTimerAtRenderedButton(t *testing.T) {
	store := &testStore{}
	m := NewModel(store)
	m.width, m.height = 160, 40
	m.inputs[0].SetValue("Wide task")
	_, layout, _ := m.wideMouseLayout()
	zone := layout.start
	m = updateMouse(t, m, tea.MouseMsg{X: zone.x, Y: zone.y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if len(store.started) != 1 || store.started[0].Title != "Wide task" {
		t.Fatalf("clicking rendered start zone started %+v", store.started)
	}
}

func TestWideMouseStopsRenderedRunningButton(t *testing.T) {
	store := &testStore{running: []tracking.Entry{{ID: 42, Title: "Timer"}}}
	m := NewModel(store)
	m.width, m.height = 160, 40
	_, layout, _ := m.wideMouseLayout()
	zone := layout.running[0].buttons[1]
	assertWideZoneMatchesLabel(t, strings.Split(ansi.Strip(m.View()), "\n"), zone, "[■ Detener]")
	m = updateMouse(t, m, tea.MouseMsg{X: zone.x, Y: zone.y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if len(store.stopped) != 1 || store.stopped[0] != 42 {
		t.Fatalf("clicking rendered stop zone stopped %v", store.stopped)
	}
}

func TestWideMouseRecentResumeAndEditButtons(t *testing.T) {
	store := &testStore{recent: []tracking.Entry{{ID: 51, TaskUID: "task-51", Title: "Recent task", EndedAt: timePtr(1)}}}
	m := NewModel(store)
	m.width, m.height = 160, 40
	_, layout, _ := m.wideMouseLayout()
	view := strings.Split(ansi.Strip(m.View()), "\n")
	resume, edit := layout.recent[0].buttons[0], layout.recent[0].buttons[1]
	assertWideZoneMatchesLabel(t, view, resume, "[▶ Reanudar]")
	m = updateMouse(t, m, tea.MouseMsg{X: resume.x, Y: resume.y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if len(store.resumed) != 1 || store.resumed[0] != 51 {
		t.Fatalf("resume click recorded %v", store.resumed)
	}
	editStore := &testStore{recent: []tracking.Entry{{ID: 52, TaskUID: "task-52", Title: "Editable", EndedAt: timePtr(1)}}}
	editModel := NewModel(editStore)
	editModel.width, editModel.height = 160, 40
	_, layout, _ = editModel.wideMouseLayout()
	edit = layout.recent[0].buttons[1]
	assertWideZoneMatchesLabel(t, strings.Split(ansi.Strip(editModel.View()), "\n"), edit, "[✎ Editar]")
	editModel = updateMouse(t, editModel, tea.MouseMsg{X: edit.x, Y: edit.y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if editModel.edit == nil {
		t.Fatal("edit click did not open the task editor")
	}
}

func TestWideMouseBreakDurationStartAndPauseSelection(t *testing.T) {
	store := &testStore{running: []tracking.Entry{{ID: 61, Title: "Timer"}}}
	breaks := &fakeBreaks{}
	wellbeing := &fakeWellbeing{}
	m := NewModel(store, WithBreaks(breaks), WithWellbeing(wellbeing))
	m.width, m.height = 160, 80
	g, _, ok := m.wideMouseLayout()
	if !ok {
		t.Fatal("wide layout unavailable at width 160")
	}
	bl := m.breakLayout()
	custom := bl.custom
	m = updateMouse(t, m, tea.MouseMsg{X: g.rightX + custom.x, Y: custom.y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if m.bp.form.row != breakRowCustom {
		t.Fatal("clicking the rendered custom-duration input did not focus it")
	}
	g, _, _ = m.wideMouseLayout()
	bl = m.breakLayout()
	duration := bl.durations[1]
	assertWideZoneMatchesLabel(t, strings.Split(ansi.Strip(m.View()), "\n"), rect{g.rightX + duration.x, duration.y, duration.w, duration.h}, "[30 min]")
	m = updateMouse(t, m, tea.MouseMsg{X: g.rightX + duration.x, Y: duration.y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if m.bp.form.choice != 1 {
		t.Fatalf("duration choice = %d, want 30-minute option", m.bp.form.choice)
	}
	g, _, _ = m.wideMouseLayout()
	bl = m.breakLayout()
	every := bl.pauseOptions[1][2]
	assertWideZoneMatchesLabel(t, strings.Split(ansi.Strip(m.View()), "\n"), rect{g.rightX + every.x, every.y, every.w, every.h}, "[30 min]")
	m = updateMouse(t, m, tea.MouseMsg{X: g.rightX + every.x, Y: every.y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if len(wellbeing.updates) != 1 || wellbeing.updates[0].Every != 30*time.Minute {
		t.Fatalf("pause cadence updates = %v", wellbeing.updates)
	}
	g, _, _ = m.wideMouseLayout()
	bl = m.breakLayout()
	start := bl.start
	assertWideZoneMatchesLabel(t, strings.Split(ansi.Strip(m.View()), "\n"), rect{g.rightX + start.x, start.y, start.w, start.h}, "[Empezar break]")
	m = updateMouse(t, m, tea.MouseMsg{X: g.rightX + start.x, Y: start.y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if len(breaks.calls) != 1 || !strings.Contains(breaks.calls[0], "30m0s") {
		t.Fatalf("start break calls = %v", breaks.calls)
	}
	checkModel := NewModel(&testStore{running: []tracking.Entry{{ID: 62, Title: "Checked timer"}}}, WithBreaks(&fakeBreaks{}))
	checkModel.width, checkModel.height = 160, 80
	checkGeometry, _, _ := checkModel.wideMouseLayout()
	checkLayout := checkModel.breakLayout()
	check := checkLayout.checks[0]
	assertWideZoneMatchesLabel(t, strings.Split(ansi.Strip(checkModel.View()), "\n"), rect{checkGeometry.rightX + check.x, check.y, check.w, check.h}, "[x] Checked timer")
	checkModel = updateMouse(t, checkModel, tea.MouseMsg{X: checkGeometry.rightX + check.x, Y: check.y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if checkModel.bp.form.checked[0] {
		t.Fatal("clicking the rendered checked timer did not toggle it off")
	}
}

func TestWideMousePauseZonesMatchRenderedControls(t *testing.T) {
	for _, width := range []int{160, 180, 200} {
		t.Run(fmt.Sprintf("width_%d", width), func(t *testing.T) {
			wellbeing := &fakeWellbeing{}
			called := make(chan struct{}, 1)
			m := NewModel(&testStore{}, WithWellbeing(wellbeing), WithTryPause(func() { called <- struct{}{} }))
			m.width, m.height = width, 45
			g, _, ok := m.wideMouseLayout()
			if !ok {
				t.Fatalf("wide layout unavailable at width %d", width)
			}
			view := strings.Split(ansi.Strip(m.View()), "\n")
			layout := m.breakLayout()
			options := [][]string{
				{"[Sí]", "[No]"},
				{"[10 min]", "[20 min]", "[30 min]", "[45 min]", "[60 min]"},
				{"[10 s]", "[15 s]", "[20 s]", "[30 s]", "[1 min]", "[2 min]", "[5 min]"},
			}
			for row, labels := range options {
				for col, label := range labels {
					zone := layout.pauseOptions[row][col]
					assertWideZoneMatchesLabel(t, view, rect{g.rightX + zone.x, zone.y, zone.w, zone.h}, label)
				}
			}
			for i, label := range []string{"[No molestar 1 hora]", "[Probar ahora]"} {
				zone := layout.pauseButtons[i]
				assertWideZoneMatchesLabel(t, view, rect{g.rightX + zone.x, zone.y, zone.w, zone.h}, label)
			}

			every := layout.pauseOptions[1][3]
			m = updateMouse(t, m, tea.MouseMsg{X: g.rightX + every.x, Y: every.y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
			if len(wellbeing.updates) != 1 || wellbeing.updates[0].Every != 45*time.Minute {
				t.Fatalf("clicking [45 min] saved %v", wellbeing.updates)
			}

			layout = m.breakLayout()
			probe := layout.pauseButtons[1]
			m = updateMouse(t, m, tea.MouseMsg{X: g.rightX + probe.x + probe.w, Y: probe.y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
			select {
			case <-called:
				t.Fatal("click outside [Probar ahora] invoked test-pause")
			default:
			}
		})
	}
}

func TestWideMouseCanExtendActiveBreak(t *testing.T) {
	breaks := &fakeBreaks{active: activeBreak(time.Hour)}
	m := NewModel(&testStore{}, WithBreaks(breaks))
	m.width, m.height = 160, 80
	g, _, _ := m.wideMouseLayout()
	layout := m.breakLayout()
	zone := layout.actions[1]
	assertWideZoneMatchesLabel(t, strings.Split(ansi.Strip(m.View()), "\n"), rect{g.rightX + zone.x, zone.y, zone.w, zone.h}, "[+10 min]")
	m = updateMouse(t, m, tea.MouseMsg{X: g.rightX + zone.x, Y: zone.y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if len(breaks.calls) != 1 || breaks.calls[0] != "extend:10m0s" {
		t.Fatalf("active break calls = %v", breaks.calls)
	}
}

func timePtr(value int64) *int64 { return &value }

func TestWideMouseStopsTaskDrawnOnScrolledRow(t *testing.T) {
	var running []tracking.Entry
	for i := 1; i <= 6; i++ {
		running = append(running, tracking.Entry{ID: int64(i), Title: fmt.Sprintf("Timer-%d", i)})
	}
	store := &testStore{running: running}
	m := NewModel(store)
	m.width, m.height = 130, 30
	m.runScroll = 2
	_, layout, _ := m.wideMouseLayout()
	if len(layout.running) == 0 {
		t.Fatal("sin filas clicables")
	}
	zone := layout.running[0].buttons[1]
	line := ansi.Strip(strings.Split(m.View(), "\n")[zone.y])
	if !strings.Contains(line, "Timer-3") {
		t.Fatalf("la primera fila visible debe dibujar Timer-3: %q", line)
	}
	m = updateMouse(t, m, tea.MouseMsg{X: zone.x, Y: zone.y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if len(store.stopped) != 1 || store.stopped[0] != 3 {
		t.Fatalf("detenidos = %v, quería [3]", store.stopped)
	}
}

func TestWideRunScrollResetsWhenEverythingFits(t *testing.T) {
	store := &testStore{running: []tracking.Entry{{ID: 1, Title: "A"}, {ID: 2, Title: "B"}}}
	m := NewModel(store)
	m.width, m.height = 130, 30
	m.runScroll = 1
	m.refresh()
	if m.runScroll != 0 {
		t.Fatalf("runScroll = %d, quería 0", m.runScroll)
	}
}

func wideClick(t *testing.T, m Model, x, y int) Model {
	t.Helper()
	return updateMouse(t, m, tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
}

func newWideRowsModel(db *testStore) Model {
	m := NewModel(db)
	m.width, m.height = 130, 40
	return m
}

func TestWideBreakScreenRoutesClicksByColumn(t *testing.T) {
	db := newRowsStore()
	m := NewModel(db, WithBreaks(&fakeBreaks{}))
	m.width, m.height = 130, 40
	m.openBreakPage()
	if m.screen != screenBreak {
		t.Fatal("no se abrió la pantalla de break")
	}
	_, layout, _ := m.wideMouseLayout()
	m = wideClick(t, m, layout.fields[2].x, layout.fields[2].y)
	if m.focus != focusDescription {
		t.Fatalf("clic en la descripción de la columna 0 con break abierto: foco=%v", m.focus)
	}
	m.inputs[0].SetValue("Desde break")
	_, layout, _ = m.wideMouseLayout()
	m = wideClick(t, m, layout.start.x, layout.start.y)
	if len(db.started) != 1 || db.started[0].Title != "Desde break" {
		t.Fatalf("clic en [Iniciar] con break abierto inició %+v", db.started)
	}
	geometry, _, _ := m.wideMouseLayout()
	l := m.breakLayout()
	want := (m.bp.form.choice + 1) % len(l.durations)
	zone := l.durations[want]
	m = wideClick(t, m, geometry.rightX+zone.x, zone.y)
	if m.bp.form.choice != want {
		t.Fatalf("clic en la columna derecha: duración=%d, quería %d", m.bp.form.choice, want)
	}
}

func TestWideStatusButtonsAndConfirmBlocking(t *testing.T) {
	db := newRowsStore()
	m := newWideRowsModel(db)
	_, layout, _ := m.wideMouseLayout()
	m = wideClick(t, m, layout.recent[1].buttons[2].x, layout.recent[1].buttons[2].y)
	if m.confirm == nil {
		t.Fatal("clic en ✕ Eliminar debe abrir la confirmación")
	}
	// Con la confirmación abierta, las columnas de atrás no responden.
	_, layout, _ = m.wideMouseLayout()
	stop := layout.running[0].buttons[1]
	m = wideClick(t, m, stop.x, stop.y)
	if len(db.stopped) != 0 || m.confirm == nil {
		t.Fatalf("clic detrás de la confirmación actuó: detenidas=%v", db.stopped)
	}
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	_, layout, _ = m.wideMouseLayout()
	if len(layout.statusButtons) != 2 {
		t.Fatalf("botones de estado = %d", len(layout.statusButtons))
	}
	assertWideZoneMatchesLabel(t, lines, layout.statusButtons[0], "[Eliminar]")
	m = wideClick(t, m, layout.statusButtons[1].x, layout.statusButtons[1].y)
	if m.confirm != nil || len(db.deleted) != 0 {
		t.Fatal("[Cancelar] debe cerrar sin borrar")
	}
	_, layout, _ = m.wideMouseLayout()
	m = wideClick(t, m, layout.recent[1].buttons[2].x, layout.recent[1].buttons[2].y)
	_, layout, _ = m.wideMouseLayout()
	m = wideClick(t, m, layout.statusButtons[0].x, layout.statusButtons[0].y)
	if len(db.deleted) != 1 || db.deleted[0] != "t3" {
		t.Fatalf("[Eliminar] eliminó %v", db.deleted)
	}
	_, layout, _ = m.wideMouseLayout()
	if len(layout.statusButtons) != 1 {
		t.Fatal("falta [Deshacer] en el layout ancho")
	}
	assertWideZoneMatchesLabel(t, strings.Split(ansi.Strip(m.View()), "\n"), layout.statusButtons[0], "[Deshacer]")
	m = wideClick(t, m, layout.statusButtons[0].x, layout.statusButtons[0].y)
	if len(db.restored) != 1 || db.restored[0] != "t3" {
		t.Fatalf("[Deshacer] restauró %v", db.restored)
	}
}
