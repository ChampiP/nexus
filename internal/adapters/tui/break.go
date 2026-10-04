package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"nexus/internal/countdown"
	"nexus/internal/tracking"
	"nexus/internal/wellbeing"
)

// Wellbeing es la frontera mínima de pausas activas que necesita la TUI.
type Wellbeing interface {
	Status(time.Time) (wellbeing.Status, error)
	UpdateSettings(wellbeing.Settings) error
	DND(time.Time, time.Duration) error
	ClearDND() error
}

// Breaks es la frontera mínima del módulo de breaks que necesita la TUI.
type Breaks interface {
	Active() (*countdown.Break, error)
	StartBreak(duration time.Duration, label string, stopIDs []int64) (countdown.Break, error)
	Extend(time.Duration) error
	End(resume bool) (int, error)
}

// WithBreaks habilita la pantalla de breaks y el indicador del encabezado.
func WithBreaks(b Breaks) Option { return func(m *Model) { m.breaks = b } }

// WithWellbeing habilita los ajustes de pausas activas en la pantalla Break.
func WithWellbeing(w Wellbeing) Option { return func(m *Model) { m.wellbeing = w } }

// WithTryPause inyecta la acción para mostrar una pausa de prueba.
func WithTryPause(run func()) Option { return func(m *Model) { m.tryPause = run } }

const (
	breakLabel  = "Break"
	extendBreak = 10 * time.Minute
	// defaultBreakChoice es la duración preseleccionada: 1 h.
	defaultBreakChoice = 2
)

// Etiquetas de los botones de la pantalla de break; el orden define su índice.
var (
	breakActionLabels   = []string{"Volver al trabajo", "+10 min", "Terminar break"}
	breakDurationLabels = []string{"15 min", "30 min", "1 h"}
	breakDurations      = []time.Duration{15 * time.Minute, 30 * time.Minute, time.Hour}
	breakStartLabels    = []string{"Empezar break"}
)

// Filas fijas del formulario; la lista de temporizadores va después y el botón al final.
const (
	breakRowDuration = 0
	breakRowCustom   = 1
	breakRowFirst    = 2 // primera fila de la lista «Detener:»
)

// breakForm es el formulario para empezar un break.
type breakForm struct {
	choice  int
	custom  textinput.Model
	timers  []tracking.Entry
	checked []bool
	row     int
}

// buttonsRow es el índice de la fila del botón [Empezar break].
func (f *breakForm) buttonsRow() int { return breakRowFirst + len(f.timers) }

func (f *breakForm) setRow(row int) { f.row = min(max(row, 0), f.buttonsRow()) }

// duration es el valor del campo numérico si lo hay, o el botón elegido.
func (f *breakForm) duration() time.Duration {
	if text := strings.TrimSpace(f.custom.Value()); text != "" {
		minutes, _ := strconv.Atoi(text)
		return time.Duration(minutes) * time.Minute
	}
	return breakDurations[f.choice]
}

// breakPage es el estado de la pantalla de break.
type breakPage struct {
	onTabs      bool // el foco está en la barra de pestañas
	action      int  // botón resaltado de la tarjeta del break activo
	form        breakForm
	resume      []string         // títulos de los temporizadores que se reanudarán al volver
	today       []tracking.Entry // breaks de hoy, en orden de inicio
	settingsRow int
}

func (m *Model) initBreakPage() {
	custom := textinput.New()
	custom.Prompt = ""
	custom.CharLimit = 4
	custom.Width = 4
	m.bp = breakPage{onTabs: true, settingsRow: -1, form: breakForm{choice: defaultBreakChoice, custom: custom}}
}

func breakOverdue(b countdown.Break, now time.Time) bool { return now.Unix() >= b.EndsAt }

// shortClock da m:ss, o h:mm:ss desde la hora.
func shortClock(seconds int64) string {
	if seconds >= 3600 {
		return formatDuration(seconds)
	}
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}

// indicatorText es el texto del indicador del encabezado de Temporizadores.
func indicatorText(b countdown.Break, now time.Time) string {
	left := b.EndsAt - now.Unix()
	if left <= 0 {
		return "☕ +" + shortClock(-left)
	}
	return "☕ quedan " + shortClock(left)
}

// indicatorVisible indica si el encabezado de Temporizadores muestra el indicador.
func (m Model) indicatorVisible() bool { return m.breaks != nil && m.brk != nil && m.edit == nil }

// loadBreak sincroniza el break activo, los breaks de hoy y el formulario.
func (m *Model) loadBreak() {
	if m.breaks == nil {
		return
	}
	active, err := m.breaks.Active()
	if err != nil {
		return
	}
	previous := m.brk
	m.brk = active
	if today, err := m.tracker.BreaksSince(rangeStart(m.now, false)); err == nil {
		m.bp.today = today
	}
	m.bp.resume = m.resumeTitles(active)
	switch {
	case active == nil:
		m.syncBreakForm()
		if previous != nil {
			m.bp.form.setRow(breakRowDuration)
		}
	case previous == nil:
		m.bp.action = 0
	}
	overdue := active != nil && breakOverdue(*active, m.now)
	if overdue && !m.wasOverdue && m.screen == screenBreak {
		m.bp.onTabs, m.bp.action = false, 0
	}
	m.wasOverdue = overdue
	m.syncBreakFocus()
}

// resumeTitles resuelve los títulos de los temporizadores recordados; omite los que ya no existen.
func (m *Model) resumeTitles(b *countdown.Break) []string {
	if b == nil {
		return nil
	}
	var titles []string
	for _, id := range b.ResumeEntryIDs {
		if entry, err := m.tracker.Get(id); err == nil {
			titles = append(titles, entry.Title)
		}
	}
	return titles
}

// syncBreakForm alinea la lista «Detener:» con los temporizadores en curso y conserva las marcas.
func (m *Model) syncBreakForm() {
	f := &m.bp.form
	marked := make(map[int64]bool, len(f.timers))
	for i, timer := range f.timers {
		marked[timer.ID] = f.checked[i]
	}
	f.timers = append([]tracking.Entry(nil), m.running...)
	f.checked = make([]bool, len(f.timers))
	for i, timer := range f.timers {
		value, known := marked[timer.ID]
		f.checked[i] = value || !known
	}
	f.setRow(f.row)
}

// syncBreakFocus da el cursor al campo numérico solo cuando está enfocado en la pantalla de break.
func (m *Model) syncBreakFocus() {
	f := &m.bp.form
	wideBreakFocus := m.width >= 120 && m.wideColumn == 2 && (m.screen == screenTimers || m.screen == screenBreak)
	if (wideBreakFocus || (m.screen == screenBreak && m.width < 120)) && m.brk == nil && !m.bp.onTabs && f.row == breakRowCustom {
		f.custom.Focus()
	} else {
		f.custom.Blur()
	}
}

// openBreakPage va directo a los botones del break activo, por ejemplo desde el indicador.
func (m *Model) openBreakPage() {
	m.switchScreen(screenBreak)
	m.wideColumn = 2
	m.bp.onTabs, m.bp.action = false, 0
	m.syncBreakFocus()
}

// Secciones de la pantalla de break; PgUp/PgDn saltan de una a otra.
const (
	brkSecTabs = iota
	brkSecMain // tarjeta del break activo, o duración y campo «Otro»
	brkSecTimers
	brkSecStart
	brkSecWellbeing
)

// Tipos de celda de la pantalla de break; en brkForm el índice es la fila del formulario.
const (
	brkTabs = iota
	brkAction
	brkForm
	brkWellbeing
)

// breakGrid describe los elementos enfocables de la pantalla de break como filas.
func (m Model) breakGrid() navGrid {
	grid := navGrid{{section: brkSecTabs, pick: 0, cells: []navCell{{brkTabs, 0, 0}}}}
	if m.brk != nil {
		actions := make([]navCell, len(breakActionLabels))
		for i := range actions {
			actions[i] = navCell{brkAction, 0, i}
		}
		grid = append(grid, navRow{section: brkSecMain, pick: -1, cells: actions})
	} else {
		f := m.bp.form
		durations := make([]navCell, len(breakDurationLabels))
		for i := range durations {
			durations[i] = navCell{brkForm, breakRowDuration, i}
		}
		grid = append(grid,
			navRow{section: brkSecMain, pick: f.choice, cells: durations},
			navRow{section: brkSecMain, pick: -1, cells: []navCell{{brkForm, breakRowCustom, 0}}})
		for i := range f.timers {
			grid = append(grid, navRow{section: brkSecTimers, pick: -1, cells: []navCell{{brkForm, breakRowFirst + i, 0}}})
		}
		grid = append(grid, navRow{section: brkSecStart, pick: -1, cells: []navCell{{brkForm, f.buttonsRow(), 0}}})
	}
	if m.wellbeing != nil {
		enabled := 0
		if m.pauseStatus.Settings.Enabled {
			enabled = 0
		} else {
			enabled = 1
		}
		every := pauseEveryIndex(m.pauseStatus.Settings.Every)
		duration := pauseDurationIndex(m.pauseStatus.Settings.Duration)
		grid = append(grid,
			navRow{section: brkSecWellbeing, pick: enabled, cells: []navCell{{brkWellbeing, 0, 0}, {brkWellbeing, 0, 1}}},
			navRow{section: brkSecWellbeing, pick: every, cells: pauseCells(1, 5)},
			navRow{section: brkSecWellbeing, pick: duration, cells: pauseCells(2, 7)},
			navRow{section: brkSecWellbeing, pick: -1, cells: []navCell{{brkWellbeing, 3, 0}, {brkWellbeing, 3, 1}}})
	}
	return grid
}

func pauseCells(row, count int) []navCell {
	cells := make([]navCell, count)
	for i := range cells {
		cells[i] = navCell{brkWellbeing, row, i}
	}
	return cells
}

func pauseEveryIndex(d time.Duration) int {
	for i, minutes := range []time.Duration{10, 20, 30, 45, 60} {
		if d == minutes*time.Minute {
			return i
		}
	}
	return 2
}

func pauseDurationIndex(d time.Duration) int {
	for i, duration := range []time.Duration{10 * time.Second, 15 * time.Second, 20 * time.Second, 30 * time.Second, time.Minute, 2 * time.Minute, 5 * time.Minute} {
		if d == duration {
			return i
		}
	}
	return 3
}

// breakCell es la celda que corresponde al foco actual.
func (m Model) breakCell() navCell {
	p := m.bp
	switch {
	case p.onTabs:
		return navCell{brkTabs, 0, 0}
	case m.brk != nil && p.settingsRow < 0:
		return navCell{brkAction, 0, p.action}
	case p.settingsRow >= 0 && m.wellbeing != nil:
		return navCell{brkWellbeing, p.settingsRow, p.action}
	case m.brk != nil:
		return navCell{brkAction, 0, p.action}
	case p.form.row == breakRowDuration:
		return navCell{brkForm, breakRowDuration, p.form.choice}
	}
	return navCell{brkForm, p.form.row, 0}
}

// goToBreak pasa el foco a la celda indicada.
func (m *Model) goToBreak(c navCell) {
	p := &m.bp
	p.onTabs = c.kind == brkTabs
	if c.kind != brkWellbeing {
		p.settingsRow = -1
	}
	switch c.kind {
	case brkAction:
		p.action = c.col
	case brkForm:
		p.form.setRow(c.index)
		if c.index == breakRowDuration {
			p.form.choice = c.col
		}
	case brkWellbeing:
		p.settingsRow = c.index
		p.action = c.col
		p.onTabs = false
	}
}

// updateBreakKey procesa una tecla en la pantalla de break.
func (m *Model) updateBreakKey(key tea.KeyMsg) tea.Cmd {
	if key.Type == tea.KeyEsc {
		m.switchScreen(screenTimers)
		return nil
	}
	defer m.syncBreakFocus()
	p := &m.bp
	if p.onTabs && m.stepScreen(key.Type) {
		return nil
	}
	target, nav := m.breakGrid().move(m.breakCell(), key.Type)
	current := m.breakCell()
	f := &p.form
	if nav && m.brk == nil && !p.onTabs && f.row == breakRowDuration && (key.Type == tea.KeyLeft || key.Type == tea.KeyRight) {
		f.custom.SetValue("") // elegir una duración descarta el valor escrito
	}
	if nav && m.brk == nil && !p.onTabs && f.row == breakRowCustom && (key.Type == tea.KeyLeft || key.Type == tea.KeyRight) {
		if target == current || !inputAtEdge(f.custom, key.Type) {
			var cmd tea.Cmd
			f.custom, cmd = f.custom.Update(key)
			return cmd
		}
	}
	if nav {
		if current.kind == brkWellbeing && (key.Type == tea.KeyLeft || key.Type == tea.KeyRight) && current.index < 3 {
			m.goToBreak(target)
			m.savePauseSelection(current.index, target.col)
			return nil
		}
		m.goToBreak(target)
		return nil
	}
	switch {
	case p.onTabs:
	case p.settingsRow >= 0 && m.wellbeing != nil:
		if key.Type == tea.KeyEnter && p.settingsRow == 3 {
			m.activateWellbeingButton(p.action)
		}
	case m.brk != nil:
		if key.Type == tea.KeyEnter {
			m.activateBreakAction(p.action)
		}
	default:
		return m.updateBreakFormKey(key)
	}
	return nil
}

func (m *Model) loadWellbeing() {
	if m.wellbeing == nil {
		return
	}
	m.pauseStatus, m.pauseErr = m.wellbeing.Status(m.now)
}

func (m *Model) savePauseSelection(row, column int) {
	if m.wellbeing == nil {
		return
	}
	cfg := m.pauseStatus.Settings
	switch row {
	case 0:
		cfg.Enabled = column == 0
	case 1:
		cfg.Every = []time.Duration{10, 20, 30, 45, 60}[column] * time.Minute
	case 2:
		cfg.Duration = []time.Duration{10 * time.Second, 15 * time.Second, 20 * time.Second, 30 * time.Second, time.Minute, 2 * time.Minute, 5 * time.Minute}[column]
	}
	if cfg == m.pauseStatus.Settings {
		return
	}
	if err := m.wellbeing.UpdateSettings(cfg); err != nil {
		m.setMessage("No se pudieron guardar las pausas activas")
		return
	}
	m.pauseStatus.Settings = cfg
	m.pauseErr = nil
	m.setMessage("Guardado")
}

func (m *Model) activateWellbeingButton(button int) {
	if m.wellbeing == nil {
		return
	}
	if button == 0 {
		var err error
		if m.pauseStatus.DNDUntil != nil && m.pauseStatus.DNDUntil.After(m.now) {
			err = m.wellbeing.ClearDND()
		} else {
			err = m.wellbeing.DND(m.now, time.Hour)
		}
		if err != nil {
			m.setMessage("No se pudo actualizar No molestar")
			return
		}
		m.loadWellbeing()
		m.setMessage("Guardado")
		return
	}
	if m.tryPause != nil {
		go m.tryPause()
	}
}

// updateBreakFormKey procesa Enter y el texto del campo «Otro» en el formulario de inicio.
func (m *Model) updateBreakFormKey(key tea.KeyMsg) tea.Cmd {
	f := &m.bp.form
	switch key.Type {
	case tea.KeyEnter:
		m.activateBreakRow()
	case tea.KeyRunes, tea.KeyBackspace, tea.KeyDelete:
		if f.row != breakRowCustom {
			return nil
		}
		key.Runes = onlyDigits(key.Runes)
		if key.Type == tea.KeyRunes && len(key.Runes) == 0 {
			return nil
		}
		var cmd tea.Cmd
		f.custom, cmd = f.custom.Update(key)
		return cmd
	}
	return nil
}

// onlyDigits descarta lo que no sea dígito, por ejemplo al pegar texto en el campo.
func onlyDigits(runes []rune) []rune {
	var digits []rune
	for _, r := range runes {
		if unicode.IsDigit(r) {
			digits = append(digits, r)
		}
	}
	return digits
}

// activateBreakAction ejecuta Volver al trabajo (0), +10 min (1) o Terminar break (2).
func (m *Model) activateBreakAction(button int) {
	if m.brk == nil {
		return
	}
	m.bp.onTabs, m.bp.action = false, button
	var text string
	var err error
	switch button {
	case 0:
		var resumed int
		if resumed, err = m.breaks.End(true); err == nil {
			text = resumedText(resumed)
		}
	case 1:
		if err = m.breaks.Extend(extendBreak); err == nil {
			text = "Break extendido 10 min"
		}
	default:
		if _, err = m.breaks.End(false); err == nil {
			text = "Break terminado"
		}
	}
	m.refresh()
	if err != nil {
		text = errorText("No se pudo actualizar el break", err)
	}
	m.setMessage(text)
}

func resumedText(n int) string {
	switch n {
	case 0:
		return "De vuelta: no había temporizadores para reanudar"
	case 1:
		return "De vuelta: reanudado 1 temporizador"
	}
	return fmt.Sprintf("De vuelta: reanudados %d temporizadores", n)
}

// activateBreakRow: Enter alterna una fila de la lista o empieza el break desde cualquier otra.
func (m *Model) activateBreakRow() {
	f := &m.bp.form
	if f.row >= breakRowFirst && f.row < f.buttonsRow() {
		i := f.row - breakRowFirst
		f.checked[i] = !f.checked[i]
		return
	}
	m.startBreak()
}

func (m *Model) startBreak() {
	f := &m.bp.form
	var ids []int64
	for i, timer := range f.timers {
		if f.checked[i] {
			ids = append(ids, timer.ID)
		}
	}
	duration := f.duration()
	b, err := m.breaks.StartBreak(duration, breakLabel, ids)
	if err != nil {
		m.refresh()
		m.setMessage(errorText("No se pudo empezar el break", err))
		return
	}
	f.choice = defaultBreakChoice
	f.custom.SetValue("")
	m.bp.onTabs, m.bp.action = false, 0
	m.refresh()
	m.setMessage(fmt.Sprintf("Break de %d min hasta las %s", int(duration/time.Minute), time.Unix(b.EndsAt, 0).Format("15:04")))
}

// updateBreakMouse resuelve clics con las zonas de breakLayout, la única fuente de coordenadas.
func (m *Model) updateBreakMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return *m, nil
	}
	l := m.breakLayout()
	x, y := msg.X, msg.Y
	if m.clickTab(l.tabs, x, y) {
		return *m, nil
	}
	if geometry, wide := m.wideGeometry(); wide && m.screen == screenBreak {
		x -= geometry.rightX
	}
	defer m.syncBreakFocus()
	f := &m.bp.form
	for i, zone := range l.actions {
		if zone.contains(x, y) {
			m.activateBreakAction(i)
			return *m, nil
		}
	}
	for row, zones := range l.pauseOptions {
		for col, zone := range zones {
			if zone.contains(x, y) {
				m.bp.onTabs, m.bp.settingsRow, m.bp.action = false, row, col
				m.savePauseSelection(row, col)
				return *m, nil
			}
		}
	}
	for i, zone := range l.pauseButtons {
		if zone.contains(x, y) {
			m.bp.onTabs, m.bp.settingsRow, m.bp.action = false, 3, i
			m.activateWellbeingButton(i)
			return *m, nil
		}
	}
	for i, zone := range l.durations {
		if zone.contains(x, y) {
			m.bp.onTabs = false
			f.setRow(breakRowDuration)
			f.choice = i
			f.custom.SetValue("")
			return *m, nil
		}
	}
	if m.brk == nil && l.custom.contains(x, y) {
		m.bp.onTabs = false
		f.setRow(breakRowCustom)
		return *m, nil
	}
	for i, zone := range l.checks {
		if zone.contains(x, y) {
			m.bp.onTabs = false
			f.setRow(breakRowFirst + i)
			m.activateBreakRow()
			return *m, nil
		}
	}
	if m.brk == nil && l.start.contains(x, y) {
		m.bp.onTabs = false
		f.setRow(f.buttonsRow())
		m.startBreak()
	}
	return *m, nil
}
