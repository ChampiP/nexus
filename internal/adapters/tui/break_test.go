package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"nexus/internal/countdown"
	"nexus/internal/tracking"
)

// fakeBreaks registra las llamadas que la TUI hace al servicio de breaks.
type fakeBreaks struct {
	active  *countdown.Break
	calls   []string
	err     error
	resumed int
}

func (f *fakeBreaks) Active() (*countdown.Break, error) { return f.active, nil }
func (f *fakeBreaks) StartBreak(d time.Duration, label string, ids []int64) (countdown.Break, error) {
	f.calls = append(f.calls, fmt.Sprintf("start:%s:%s:%v", d, label, ids))
	if f.err != nil {
		return countdown.Break{}, f.err
	}
	return countdown.Break{Label: label, EndsAt: time.Now().Add(d).Unix()}, nil
}
func (f *fakeBreaks) Extend(d time.Duration) error {
	f.calls = append(f.calls, "extend:"+d.String())
	return f.err
}
func (f *fakeBreaks) End(resume bool) (int, error) {
	f.calls = append(f.calls, fmt.Sprintf("end:%v", resume))
	return f.resumed, f.err
}

func breakModel(store *testStore, breaks *fakeBreaks) Model {
	m := NewModel(store, WithBreaks(breaks))
	m.width, m.height = 100, 40
	return m
}

func twoTimers() *testStore {
	return &testStore{running: []tracking.Entry{{ID: 1, Title: "Uno"}, {ID: 2, Title: "Dos"}}}
}

func pressKeys(t *testing.T, m Model, types ...tea.KeyType) Model {
	t.Helper()
	for _, k := range types {
		m = updateKey(t, m, tea.KeyMsg{Type: k})
	}
	return m
}

func click(t *testing.T, m Model, r rect) Model {
	t.Helper()
	return updateMouse(t, m, tea.MouseMsg{X: r.x, Y: r.y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
}

func activeBreak(endsIn time.Duration) *countdown.Break {
	now := time.Now()
	return &countdown.Break{Label: "Break", StartedAt: now.Unix(), EndsAt: now.Add(endsIn).Unix()}
}

// onBreakPage abre la pantalla de break con el foco en la barra de pestañas y la deja lista para dibujar.
func onBreakPage(t *testing.T, store *testStore, breaks *fakeBreaks) Model {
	t.Helper()
	m := breakModel(store, breaks)
	if m.screen == screenBreak {
		return m // un break vencido ya abre en esta pantalla
	}
	m = pressKeys(t, m, tea.KeyUp)    // barra de pestañas
	m = pressKeys(t, m, tea.KeyRight) // Break
	if m.screen != screenBreak {
		t.Fatalf("screen = %v, want break", m.screen)
	}
	return m
}

// screenLine devuelve la línea y de la vista sin los caracteres anteriores a la columna x.
func screenLine(t *testing.T, view string, x, y int) string {
	t.Helper()
	lines := strings.Split(view, "\n")
	if y >= len(lines) {
		t.Fatalf("la vista no tiene la línea %d", y)
	}
	runes := []rune(lines[y])
	if x > len(runes) {
		return ""
	}
	return string(runes[x:])
}

func TestIndicatorTextRemainingAndOverdue(t *testing.T) {
	now := time.Unix(10_000, 0)
	remaining := countdown.Break{EndsAt: now.Unix() + 750}
	if got, want := indicatorText(remaining, now), "☕ quedan 12:30"; got != want {
		t.Errorf("remaining = %q, want %q", got, want)
	}
	overdue := countdown.Break{EndsAt: now.Unix() - 48}
	if got, want := indicatorText(overdue, now), "☕ +0:48"; got != want {
		t.Errorf("overdue = %q, want %q", got, want)
	}
}

func TestTabsOrderAndAvailability(t *testing.T) {
	cases := []struct {
		name            string
		breaks, catalog bool
		want            []screen
	}{
		{"nada", false, false, []screen{screenTimers}},
		{"solo break", true, false, []screen{screenTimers, screenBreak}},
		{"solo catálogo", false, true, []screen{screenTimers, screenCatalog}},
		{"ambos", true, true, []screen{screenTimers, screenBreak, screenCatalog}},
	}
	for _, c := range cases {
		var options []Option
		if c.breaks {
			options = append(options, WithBreaks(&fakeBreaks{}))
		}
		if c.catalog {
			options = append(options, WithCatalog(newFakeCatalog()))
		}
		m := NewModel(twoTimers(), options...)
		m.width, m.height = 100, 40
		if fmt.Sprint(m.screens()) != fmt.Sprint(c.want) {
			t.Errorf("%s: screens = %v, want %v", c.name, m.screens(), c.want)
		}
		if len(m.computeLayout().tabs) != len(c.want) && len(c.want) > 1 {
			t.Errorf("%s: tabs = %d", c.name, len(m.computeLayout().tabs))
		}
	}
	m := NewModel(twoTimers(), WithBreaks(&fakeBreaks{}), WithCatalog(newFakeCatalog()))
	m.width = 100
	header := strings.Split(m.View(), "\n")[0]
	timers, brk, cat := strings.Index(header, "Temporizadores"), strings.Index(header, "Break"), strings.Index(header, "Catálogo")
	if !(timers >= 0 && timers < brk && brk < cat) {
		t.Fatalf("orden de pestañas incorrecto: %q", header)
	}
}

func TestTabSwitchingKeysAndEsc(t *testing.T) {
	m := NewModel(twoTimers(), WithBreaks(&fakeBreaks{}), WithCatalog(newFakeCatalog()))
	m.width, m.height = 100, 40
	m = pressKeys(t, m, tea.KeyUp)
	if m.focus != focusTabs {
		t.Fatalf("focus = %v, want tabs", m.focus)
	}
	steps := []struct {
		key  tea.KeyType
		want screen
	}{
		{tea.KeyRight, screenBreak},
		{tea.KeyRight, screenCatalog},
		{tea.KeyLeft, screenBreak},
		{tea.KeyEnter, screenCatalog},
		{tea.KeyEnter, screenTimers},
		{tea.KeyLeft, screenCatalog},
		{tea.KeyLeft, screenBreak},
		{tea.KeyLeft, screenTimers},
	}
	for i, s := range steps {
		m = pressKeys(t, m, s.key)
		if m.screen != s.want {
			t.Fatalf("paso %d: screen = %v, want %v", i, m.screen, s.want)
		}
	}
	m = pressKeys(t, m, tea.KeyRight)
	m = pressKeys(t, m, tea.KeyEsc)
	if m.screen != screenTimers {
		t.Fatalf("Esc en Break: screen = %v, want temporizadores", m.screen)
	}
}

func TestTabClicksSwitchScreens(t *testing.T) {
	m := NewModel(twoTimers(), WithBreaks(&fakeBreaks{}), WithCatalog(newFakeCatalog()))
	m.width, m.height = 100, 40
	m = click(t, m, m.computeLayout().tabs[1])
	if m.screen != screenBreak || !m.bp.onTabs {
		t.Fatalf("clic en Break: screen = %v", m.screen)
	}
	m = click(t, m, m.breakLayout().tabs[2])
	if m.screen != screenCatalog {
		t.Fatalf("clic en Catálogo: screen = %v", m.screen)
	}
	m = click(t, m, m.catalogLayout().tabs[1])
	if m.screen != screenBreak {
		t.Fatalf("clic en Break desde el catálogo: screen = %v", m.screen)
	}
	m = click(t, m, m.breakLayout().tabs[0])
	if m.screen != screenTimers {
		t.Fatalf("clic en Temporizadores: screen = %v", m.screen)
	}
}

func TestTimersPageHasNoBreakUI(t *testing.T) {
	store := twoTimers()
	store.breaks = []tracking.Entry{{ID: 9, Kind: tracking.KindBreak, Title: "Break", StartedAt: time.Now().Unix() - 600}}
	for _, active := range []*countdown.Break{nil, activeBreak(time.Hour)} {
		m := breakModel(store, &fakeBreaks{active: active})
		view := m.View()
		for _, text := range []string{"[☕ Break]", "Volver al trabajo", "+10 min", "Terminar break", "Break hoy", "Empezar break", "BREAKS DE HOY"} {
			if strings.Contains(view, text) {
				t.Errorf("active=%v: la página de Temporizadores no debe mostrar %q:\n%s", active != nil, text, view)
			}
		}
		if active == nil && strings.Contains(view, "☕") {
			t.Errorf("sin break activo no debe haber indicador:\n%s", view)
		}
	}
}

func TestHeaderIndicatorRemainingOverdueAndClick(t *testing.T) {
	endsAt := time.Now().Add(24 * time.Hour)
	brk := &countdown.Break{Label: "Break", StartedAt: endsAt.Unix() - 3600, EndsAt: endsAt.Unix()}
	m := breakModel(twoTimers(), &fakeBreaks{active: brk})
	m.now = endsAt.Add(-750 * time.Second)
	header := strings.Split(m.View(), "\n")[0]
	if !strings.Contains(header, "☕ quedan 12:30") {
		t.Fatalf("falta el indicador en el encabezado: %q", header)
	}
	layout := m.computeLayout()
	if got := screenLine(t, header, layout.breakIndicator.x, 0); !strings.HasPrefix(got, "☕ quedan 12:30") {
		t.Fatalf("la zona del indicador no coincide con el dibujo: %q", got)
	}
	m.now = endsAt.Add(48 * time.Second)
	header = strings.Split(m.View(), "\n")[0]
	if !strings.Contains(header, "☕ +0:48") || strings.Contains(header, "quedan") {
		t.Fatalf("falta el indicador de vencido: %q", header)
	}
	next := click(t, m, m.computeLayout().breakIndicator)
	if next.screen != screenBreak {
		t.Fatalf("el clic en el indicador debe abrir Break; screen = %v", next.screen)
	}
	if next.bp.onTabs || next.bp.action != 0 {
		t.Fatalf("el foco debe quedar en [Volver al trabajo]: %+v", next.bp)
	}
}

func TestRecentExcludesBreaks(t *testing.T) {
	store := twoTimers()
	store.running = nil
	store.recent = []tracking.Entry{{ID: 5, Kind: tracking.KindBreak, Title: "Descanso largo"}, {ID: 1, Kind: tracking.KindWork, Title: "Informe", EndedAt: ended(100)}, {ID: 6, Title: "Sin tipo", EndedAt: ended(100)}}
	m := breakModel(store, &fakeBreaks{})
	if len(m.recent) != 2 || m.recent[0].Title != "Informe" || m.recent[1].Title != "Sin tipo" {
		t.Fatalf("recent = %+v", m.recent)
	}
	if strings.Contains(m.View(), "Descanso largo") {
		t.Fatal("los breaks no deben aparecer en RECIENTES")
	}
	// El límite de 8 se aplica después de filtrar.
	store.recent = nil
	for i := 0; i < 3; i++ {
		store.recent = append(store.recent, tracking.Entry{ID: int64(100 + i), Kind: tracking.KindBreak, Title: "Break"})
	}
	for i := 0; i < 8; i++ {
		store.recent = append(store.recent, tracking.Entry{ID: int64(i + 1), Title: fmt.Sprintf("T%d", i), EndedAt: ended(100)})
	}
	if m = breakModel(store, &fakeBreaks{}); len(m.recent) != 8 {
		t.Fatalf("recent = %d, want 8 de trabajo", len(m.recent))
	}
}

func TestBreakPageActiveCardTexts(t *testing.T) {
	start := time.Date(2026, 3, 4, 15, 0, 0, 0, time.Local)
	end := start.Add(40 * time.Minute)
	store := &testStore{running: []tracking.Entry{{ID: 1, Title: "Informe"}, {ID: 2, Title: "Diseño"}, {ID: 3, Title: "Otro"}}}
	brk := &countdown.Break{Label: "Break", StartedAt: start.Unix(), EndsAt: end.Unix(), ResumeEntryIDs: []int64{1, 2, 77}}
	m := onBreakPage(t, store, &fakeBreaks{active: brk})
	m.now = end.Add(-2530 * time.Second)
	view := m.View()
	for _, text := range []string{"☕ Break", "Quedan 0:42:10", "De 15:00 a 15:40", "Al volver se reanudan: Informe, Diseño", "[Volver al trabajo]", "[+10 min]", "[Terminar break]"} {
		if !strings.Contains(view, text) {
			t.Errorf("falta %q en:\n%s", text, view)
		}
	}
	if strings.Contains(view, "Otro") || strings.Contains(view, "Empezar break") {
		t.Errorf("la tarjeta no debe mostrar el formulario ni temporizadores no recordados:\n%s", view)
	}
	m.now = end.Add(753 * time.Second)
	if view = m.View(); !strings.Contains(view, "Excedido +0:12:33") || strings.Contains(view, "Quedan") {
		t.Errorf("falta el estado vencido:\n%s", view)
	}
	empty := &countdown.Break{Label: "Break", StartedAt: start.Unix(), EndsAt: end.Unix()}
	m = onBreakPage(t, store, &fakeBreaks{active: empty})
	if !strings.Contains(m.View(), "No se reanudará ningún temporizador") {
		t.Errorf("falta el texto sin temporizadores:\n%s", m.View())
	}
}

func TestBreakPageButtonsCallRightMethod(t *testing.T) {
	cases := []struct {
		rights int
		want   string
	}{{0, "end:true"}, {1, "extend:10m0s"}, {2, "end:false"}}
	for _, c := range cases {
		breaks := &fakeBreaks{active: activeBreak(time.Hour)}
		m := onBreakPage(t, twoTimers(), breaks)
		m = pressKeys(t, m, tea.KeyDown)
		for i := 0; i < c.rights; i++ {
			m = pressKeys(t, m, tea.KeyRight)
		}
		pressKeys(t, m, tea.KeyEnter)
		if fmt.Sprint(breaks.calls) != "["+c.want+"]" {
			t.Errorf("rights=%d calls = %v, want %s", c.rights, breaks.calls, c.want)
		}
	}
}

func TestBreakPageButtonClicksUseLayout(t *testing.T) {
	for i, want := range []string{"end:true", "extend:10m0s", "end:false"} {
		breaks := &fakeBreaks{active: activeBreak(time.Hour)}
		m := onBreakPage(t, twoTimers(), breaks)
		layout := m.breakLayout()
		if len(layout.actions) != 3 {
			t.Fatalf("actions = %v", layout.actions)
		}
		if got := screenLine(t, m.View(), layout.actions[i].x, layout.actions[i].y); !strings.HasPrefix(got, buttonText(breakActionLabels[i])) {
			t.Fatalf("el botón %d no está donde dice el layout: %q", i, got)
		}
		click(t, m, layout.actions[i])
		if fmt.Sprint(breaks.calls) != "["+want+"]" {
			t.Errorf("button %d calls = %v, want %s", i, breaks.calls, want)
		}
	}
}

func TestReturnToWorkMessageAndFormAfterEnd(t *testing.T) {
	breaks := &fakeBreaks{active: activeBreak(-time.Minute), resumed: 2}
	m := breakModel(twoTimers(), breaks)
	breaks.active = nil // el servicio ya no lo devuelve tras terminar
	m = pressKeys(t, m, tea.KeyEnter)
	if m.message != "De vuelta: reanudados 2 temporizadores" {
		t.Fatalf("message = %q", m.message)
	}
	if m.screen != screenBreak || !strings.Contains(m.View(), "[Empezar break]") {
		t.Fatalf("tras terminar debe verse el formulario:\n%s", m.View())
	}
}

func TestBreakPageStartFormDefaults(t *testing.T) {
	breaks := &fakeBreaks{}
	m := onBreakPage(t, twoTimers(), breaks)
	f := m.bp.form
	if f.choice != 2 || len(f.checked) != 2 || !f.checked[0] || !f.checked[1] {
		t.Fatalf("form = %+v", f)
	}
	view := m.View()
	for _, text := range []string{"[15 min]", "[30 min]", "[1 h]", "Otro (min)", "Detener:", "[x] Uno", "[x] Dos", "[Empezar break]"} {
		if !strings.Contains(view, text) {
			t.Errorf("falta %q en:\n%s", text, view)
		}
	}
	for _, text := range []string{"Cancelar", "Volver al trabajo"} {
		if strings.Contains(view, text) {
			t.Errorf("no debe aparecer %q:\n%s", text, view)
		}
	}
	m = pressKeys(t, m, tea.KeyEsc)
	if m.screen != screenTimers || len(breaks.calls) != 0 {
		t.Fatalf("Esc debe volver sin llamadas: screen=%v calls=%v", m.screen, breaks.calls)
	}
}

func TestBreakPageStartPassesSelectedIDs(t *testing.T) {
	breaks := &fakeBreaks{}
	m := onBreakPage(t, twoTimers(), breaks)
	m = pressKeys(t, m, tea.KeyDown)              // sale de las pestañas: duración
	m = pressKeys(t, m, tea.KeyLeft, tea.KeyLeft) // 1 h → 30 min → 15 min
	m = pressKeys(t, m, tea.KeyDown, tea.KeyDown) // Otro → primera fila
	m = pressKeys(t, m, tea.KeyEnter)             // desmarca "Uno"
	if m.bp.form.checked[0] || !strings.Contains(m.View(), "[ ] Uno") {
		t.Fatalf("la fila no se desmarcó: %+v", m.bp.form.checked)
	}
	m = pressKeys(t, m, tea.KeyDown, tea.KeyDown, tea.KeyEnter) // [Empezar break]
	if fmt.Sprint(breaks.calls) != "[start:15m0s:Break:[2]]" {
		t.Fatalf("calls = %v", breaks.calls)
	}
	if !strings.HasPrefix(m.message, "Break de 15 min hasta las ") {
		t.Fatalf("message = %q", m.message)
	}
}

func TestBreakPageCustomMinutes(t *testing.T) {
	breaks := &fakeBreaks{}
	m := onBreakPage(t, twoTimers(), breaks)
	m = pressKeys(t, m, tea.KeyDown, tea.KeyDown)
	m = updateKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x2a5")})
	if got := m.bp.form.custom.Value(); got != "25" {
		t.Fatalf("custom = %q, solo deben entrar dígitos", got)
	}
	pressKeys(t, m, tea.KeyEnter)
	if fmt.Sprint(breaks.calls) != "[start:25m0s:Break:[1 2]]" {
		t.Fatalf("calls = %v", breaks.calls)
	}
}

func TestBreakPageWithoutRunningTimersHasNoChecklist(t *testing.T) {
	m := onBreakPage(t, &testStore{}, &fakeBreaks{})
	if strings.Contains(m.View(), "Detener:") {
		t.Fatal("sin temporizadores no debe haber lista")
	}
}

func TestBreakFormKeepsMarksAcrossRefresh(t *testing.T) {
	store := twoTimers()
	m := onBreakPage(t, store, &fakeBreaks{})
	m = pressKeys(t, m, tea.KeyDown, tea.KeyDown, tea.KeyDown, tea.KeyEnter) // desmarca "Uno"
	store.running = append(store.running, tracking.Entry{ID: 3, Title: "Tres"})
	m.refresh()
	if got := fmt.Sprint(m.bp.form.checked); got != "[false true true]" {
		t.Fatalf("checked = %s, want [false true true]", got)
	}
}

func TestBreakErrorsAreSpanish(t *testing.T) {
	cases := map[error]string{
		countdown.ErrBreakActive:     "Ya hay un break en curso",
		countdown.ErrInvalidDuration: "La duración debe estar entre 1 minuto y 8 horas",
		countdown.ErrNoActiveBreak:   "No hay un break activo",
	}
	for err, want := range cases {
		m := onBreakPage(t, twoTimers(), &fakeBreaks{err: err})
		m = pressKeys(t, m, tea.KeyDown, tea.KeyDown, tea.KeyDown, tea.KeyDown, tea.KeyDown, tea.KeyEnter)
		if m.message != want {
			t.Errorf("start %v: message = %q, want %q", err, m.message, want)
		}
	}
	m := onBreakPage(t, twoTimers(), &fakeBreaks{active: activeBreak(time.Hour), err: countdown.ErrNoActiveBreak})
	m = pressKeys(t, m, tea.KeyDown, tea.KeyRight, tea.KeyRight, tea.KeyEnter)
	if m.message != "No hay un break activo" {
		t.Errorf("end: message = %q", m.message)
	}
}

func TestBreakPageFormClicks(t *testing.T) {
	breaks := &fakeBreaks{}
	m := onBreakPage(t, twoTimers(), breaks)
	layout := m.breakLayout()
	if got := screenLine(t, m.View(), layout.durations[0].x, layout.durations[0].y); !strings.HasPrefix(got, "[15 min]") {
		t.Fatalf("la zona de duración no coincide con el dibujo: %q", got)
	}
	if got := screenLine(t, m.View(), layout.start.x, layout.start.y); !strings.HasPrefix(got, "[Empezar break]") {
		t.Fatalf("la zona de inicio no coincide con el dibujo: %q", got)
	}
	m = click(t, m, layout.durations[0])
	m = click(t, m, layout.checks[1])
	if m.bp.form.choice != 0 || m.bp.form.checked[1] {
		t.Fatalf("form = %+v", m.bp.form)
	}
	m = click(t, m, layout.custom)
	if m.bp.form.row != breakRowCustom || !m.bp.form.custom.Focused() {
		t.Fatalf("clic en Otro: row=%d", m.bp.form.row)
	}
	click(t, m, layout.start)
	if fmt.Sprint(breaks.calls) != "[start:15m0s:Break:[1]]" {
		t.Fatalf("calls = %v", breaks.calls)
	}
}

func TestBreaksTodayLinesAndTotal(t *testing.T) {
	day := time.Date(2026, 3, 4, 0, 0, 0, 0, time.Local)
	at := func(h, min int) int64 {
		return day.Add(time.Duration(h)*time.Hour + time.Duration(min)*time.Minute).Unix()
	}
	end1, end2 := at(15, 40), at(17, 25)
	store := &testStore{breaks: []tracking.Entry{
		{ID: 1, Kind: tracking.KindBreak, Title: "Break", StartedAt: at(15, 0), EndedAt: &end1},
		{ID: 2, Kind: tracking.KindBreak, Title: "Almuerzo", StartedAt: at(17, 0), EndedAt: &end2},
	}}
	m := onBreakPage(t, store, &fakeBreaks{})
	view := m.View()
	for _, text := range []string{"BREAKS DE HOY", "15:00 – 15:40 · 0:40:00 · Break", "17:00 – 17:25 · 0:25:00 · Almuerzo", "Total 1:05:00"} {
		if !strings.Contains(view, text) {
			t.Errorf("falta %q en:\n%s", text, view)
		}
	}
	m = onBreakPage(t, &testStore{}, &fakeBreaks{})
	if view = m.View(); !strings.Contains(view, "Sin breaks hoy") || strings.Contains(view, "Total") {
		t.Errorf("sin breaks:\n%s", view)
	}
	// Un break en curso cuenta hasta ahora.
	store.breaks = []tracking.Entry{{ID: 3, Kind: tracking.KindBreak, Title: "Break", StartedAt: at(9, 0)}}
	m = onBreakPage(t, store, &fakeBreaks{active: &countdown.Break{Label: "Break", StartedAt: at(9, 0), EndsAt: at(10, 0)}})
	m.now = time.Unix(at(9, 5), 0)
	if view = m.View(); !strings.Contains(view, "09:00 – en curso · 0:05:00 · Break") || !strings.Contains(view, "Total 0:05:00") {
		t.Errorf("break en curso:\n%s", view)
	}
}

func TestBreakPageHistoryStaysBelowCardAndFitsScreen(t *testing.T) {
	store := twoTimers()
	for i := 0; i < 30; i++ {
		end := int64(1000 + i*100 + 50)
		store.breaks = append(store.breaks, tracking.Entry{ID: int64(i + 1), Kind: tracking.KindBreak, Title: fmt.Sprintf("B%02d", i), StartedAt: int64(1000 + i*100), EndedAt: &end})
	}
	m := onBreakPage(t, store, &fakeBreaks{active: activeBreak(time.Hour)})
	m.height = 30
	lines := strings.Split(m.View(), "\n")
	if len(lines) > m.height {
		t.Fatalf("la vista mide %d líneas y la pantalla %d", len(lines), m.height)
	}
	if view := m.View(); !strings.Contains(view, "B29") || strings.Contains(view, "B00") || !strings.Contains(view, "Total") {
		t.Fatalf("deben verse los más recientes y el total:\n%s", view)
	}
}

func TestStartupOnBreakPageWhenOverdue(t *testing.T) {
	breaks := &fakeBreaks{active: activeBreak(-time.Minute), resumed: 1}
	m := breakModel(twoTimers(), breaks)
	if m.screen != screenBreak || m.bp.onTabs || m.bp.action != 0 {
		t.Fatalf("screen=%v onTabs=%v action=%d, want Break con foco en [Volver al trabajo]", m.screen, m.bp.onTabs, m.bp.action)
	}
	if !strings.Contains(m.View(), "Excedido +") {
		t.Fatalf("falta el texto de vencido:\n%s", m.View())
	}
	breaks.active = nil
	pressKeys(t, m, tea.KeyEnter)
	if fmt.Sprint(breaks.calls) != "[end:true]" {
		t.Fatalf("calls = %v", breaks.calls)
	}
}

func TestStartupOnTimersWhenNotOverdue(t *testing.T) {
	for _, active := range []*countdown.Break{nil, activeBreak(time.Hour)} {
		m := breakModel(twoTimers(), &fakeBreaks{active: active})
		if m.screen != screenTimers {
			t.Errorf("active=%v: screen = %v, want temporizadores", active != nil, m.screen)
		}
	}
}

func TestBecomingOverdueOnBreakPageFocusesReturn(t *testing.T) {
	breaks := &fakeBreaks{active: activeBreak(time.Hour)}
	m := breakModel(twoTimers(), breaks)
	m = pressKeys(t, m, tea.KeyUp, tea.KeyRight)
	breaks.active = activeBreak(-time.Second)
	next, _ := m.Update(tickMsg(time.Now()))
	if got := next.(Model); got.screen != screenBreak || got.bp.onTabs || got.bp.action != 0 {
		t.Fatalf("screen=%v onTabs=%v action=%d", got.screen, got.bp.onTabs, got.bp.action)
	}
}
