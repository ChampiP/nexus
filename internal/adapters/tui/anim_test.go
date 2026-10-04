package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"nexus/internal/countdown"
	"nexus/internal/tracking"
)

func TestBreakFractionDrainsWithFakeClock(t *testing.T) {
	start := time.Unix(1_000, 0)
	b := countdown.Break{StartedAt: start.Unix(), EndsAt: start.Add(15 * time.Minute).Unix()}
	cases := []struct {
		at      time.Duration
		want    float64
		overdue bool
	}{
		{0, 1, false},
		{5 * time.Minute, 2.0 / 3, false},
		{7*time.Minute + 30*time.Second, 0.5, false},
		{15*time.Minute - time.Second, 1.0 / 900, false},
		{15 * time.Minute, 1, true},
		{20 * time.Minute, 1, true},
	}
	for _, c := range cases {
		got, overdue := breakFraction(b, start.Add(c.at))
		if d := got - c.want; d > 1e-9 || d < -1e-9 || overdue != c.overdue {
			t.Errorf("a %v: fracción=%v vencido=%v, quería %v %v", c.at, got, overdue, c.want, c.overdue)
		}
	}
	// Un reloj anterior al inicio no desborda la barra.
	if got, _ := breakFraction(b, start.Add(-time.Hour)); got != 1 {
		t.Errorf("antes del inicio: %v", got)
	}
	// Una duración nula no divide entre cero.
	if got, _ := breakFraction(countdown.Break{StartedAt: 5, EndsAt: 5}, time.Unix(4, 0)); got < 0 || got > 1 {
		t.Errorf("duración nula: %v", got)
	}
}

func TestBreakCardShowsProgressBar(t *testing.T) {
	m := onBreakPage(t, &testStore{}, &fakeBreaks{active: activeBreak(15 * time.Minute)})
	view := m.View()
	if !strings.Contains(view, "█") {
		t.Fatalf("la tarjeta debe mostrar la barra llena al inicio:\n%s", view)
	}
	m.now = time.Unix(m.brk.StartedAt, 0).Add(450 * time.Second)
	half := m.View()
	if strings.Count(half, "█") >= strings.Count(view, "█") || !strings.Contains(half, "░") {
		t.Fatalf("la barra debe vaciarse:\n%s", half)
	}
	m.now = time.Unix(m.brk.EndsAt, 0).Add(time.Minute)
	if over := m.View(); strings.Count(over, "█") != strings.Count(view, "█") || strings.Contains(over, "░") {
		t.Fatalf("vencido la barra queda llena:\n%s", over)
	}
}

func runningStore() *testStore {
	return &testStore{running: []tracking.Entry{{ID: 1, Title: "Uno", StartedAt: 100}}}
}

func TestInitStartsFastTickOnlyWhileRunning(t *testing.T) {
	m := NewModel(runningStore())
	if !m.fastActive {
		t.Fatal("con un temporizador en curso debe haber tick rápido")
	}
	msg := m.Init()()
	batch, ok := msg.(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatalf("Init debe lanzar el tick de 1 s y el rápido: %T %v", msg, msg)
	}
	if idle := NewModel(&testStore{}); idle.fastActive {
		t.Fatal("sin temporizadores no debe haber tick rápido")
	}
}

func TestFastTickStopsWhenNothingRuns(t *testing.T) {
	store := runningStore()
	m := NewModel(store)
	m = updateModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	frame := m.spin.View()
	next, cmd := m.Update(fastTickMsg{})
	m = next.(Model)
	if cmd == nil || !m.fastActive || m.spin.View() == frame {
		t.Fatalf("con temporizadores el tick rápido avanza el spinner y se reprograma (cmd=%v activo=%v)", cmd != nil, m.fastActive)
	}
	store.running = nil
	m = updateModel(t, m, tickMsg(time.Now()))
	next, cmd = m.Update(fastTickMsg{})
	m = next.(Model)
	if cmd != nil || m.fastActive {
		t.Fatalf("sin temporizadores el tick rápido debe detenerse (cmd=%v activo=%v)", cmd != nil, m.fastActive)
	}
}

func TestSecondTickRestartsFastTickWhenTimerAppears(t *testing.T) {
	store := &testStore{}
	m := NewModel(store)
	store.running = []tracking.Entry{{ID: 1, Title: "Nuevo"}}
	next, cmd := m.Update(tickMsg(time.Now()))
	m = next.(Model)
	if !m.fastActive {
		t.Fatal("al aparecer un temporizador se arranca el tick rápido")
	}
	if batch, ok := cmd().(tea.BatchMsg); !ok || len(batch) != 2 {
		t.Fatalf("el tick de 1 s debe ir acompañado del rápido: %v", batch)
	}
}

func TestStartingTimerStartsFastTick(t *testing.T) {
	m := NewModel(&testStore{})
	m = typeText(t, m, "Algo")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if !m.fastActive || cmd == nil {
		t.Fatal("iniciar un temporizador arranca el tick rápido")
	}
}

func TestNoFastTickOutsideTimersScreen(t *testing.T) {
	m := breakModel(runningStore(), &fakeBreaks{})
	m = pressKeys(t, m, tea.KeyUp, tea.KeyRight)
	if m.screen != screenBreak {
		t.Fatal("no se abrió Break")
	}
	next, cmd := m.Update(fastTickMsg{})
	if cmd != nil || next.(Model).fastActive {
		t.Fatal("el spinner solo anima en Temporizadores")
	}
}

func TestSpinnerShownOnRunningRowsAndHeader(t *testing.T) {
	m := NewModel(runningStore())
	m.width, m.height = 100, 40
	frame := strings.TrimSpace(m.spin.View())
	if frame == "" {
		t.Fatal("el spinner no tiene fotograma")
	}
	layout := m.computeLayout()
	if line := viewLine(t, m, layout.running[0].row.y); !strings.Contains(line, frame+" Uno") && !strings.Contains(line, frame+"Uno") {
		t.Fatalf("la fila en curso debe llevar el spinner antes del título: %q", line)
	}
	if header := viewLine(t, m, 0); !strings.Contains(header, frame) {
		t.Fatalf("el encabezado debe mostrar el spinner junto a Hoy: %q", header)
	}
	idle := NewModel(&testStore{})
	idle.width, idle.height = 100, 40
	if header := viewLine(t, idle, 0); strings.Contains(header, frame) {
		t.Fatalf("sin temporizadores no hay spinner: %q", header)
	}
}
