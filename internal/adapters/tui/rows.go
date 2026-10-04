package tui

import (
	"errors"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"nexus/internal/countdown"
	"nexus/internal/tracking"
)

// undoWindow es el tiempo en que el botón [Deshacer] sigue disponible tras eliminar.
const undoWindow = 10 * time.Second

// confirmState representa la confirmación de eliminación pendiente.
type confirmState struct {
	entry tracking.Entry
	// button es 0 para Eliminar y 1 para Cancelar (el valor por defecto).
	button int
}

// Secciones de la pantalla de Temporizadores; PgUp/PgDn saltan de una a otra.
const (
	secTabs = iota
	secForm
	secRunning
	secPeriod
	secRecent
	secUndo
)

// secButtons es la sección de botones del panel de edición, que no coexiste con las de la lista.
const secButtons = secRunning

// entryAction es un botón en línea de una fila de tarea; las etiquetas fijan su orden.
type entryAction int

const (
	entryResume entryAction = iota
	entryEdit
	entryStop
	entryDelete
)

func (a entryAction) label() string {
	return [...]string{"▶ Reanudar", "✎ Editar", "■ Detener", "✕ Eliminar"}[a]
}

func entryLabels(actions []entryAction) []string {
	labels := make([]string, len(actions))
	for i, a := range actions {
		labels[i] = a.label()
	}
	return labels
}

// runningActions son los botones de un temporizador en curso.
var runningActions = []entryAction{entryEdit, entryStop}

// recentActions son los botones de una fila de RECIENTES: una tarea detenida se puede reanudar.
func recentActions(entry tracking.Entry) []entryAction {
	if entry.EndedAt == nil {
		return []entryAction{entryEdit, entryDelete}
	}
	return []entryAction{entryResume, entryEdit, entryDelete}
}

// timersGrid describe los elementos enfocables de Temporizadores como filas; es el único lugar que fija su orden.
func (m Model) timersGrid() navGrid {
	one := func(section int, kind focusTarget) navRow {
		return navRow{section: section, pick: -1, cells: []navCell{{int(kind), 0, 0}}}
	}
	if m.edit != nil {
		return navGrid{
			one(secForm, focusTitle), one(secForm, focusProject), one(secForm, focusDescription),
			{section: secButtons, pick: -1, cells: []navCell{{int(focusEditSave), 0, 0}, {int(focusEditDelete), 0, 1}, {int(focusEditCancel), 0, 2}}},
		}
	}
	var grid navGrid
	if m.tabsVisible() {
		// Las pestañas son opciones excluyentes: ←/→ cambian de pantalla.
		grid = append(grid, navRow{section: secTabs, pick: 0, cells: []navCell{{int(focusTabs), 0, 0}}})
	}
	grid = append(grid, one(secForm, focusTitle), one(secForm, focusProject), one(secForm, focusDescription), one(secForm, focusStart))
	for i := range m.running {
		cells := make([]navCell, len(runningActions))
		for b := range cells {
			cells[b] = navCell{int(focusRunningStart), i, b}
		}
		grid = append(grid, navRow{section: secRunning, pick: -1, cells: cells})
	}
	pick := 0
	if m.week {
		pick = 1
	}
	grid = append(grid, navRow{section: secPeriod, pick: pick, cells: []navCell{{int(focusToday), 0, 0}, {int(focusWeek), 0, 1}}})
	for i, zone := range m.computeLayout().recent {
		cells := make([]navCell, len(zone.buttons))
		for b := range cells {
			cells[b] = navCell{int(focusRecent), i, b}
		}
		grid = append(grid, navRow{section: secRecent, pick: -1, cells: cells})
	}
	if m.undoActive() {
		grid = append(grid, one(secUndo, focusUndo))
	}
	return grid
}

// currentCell es la celda que corresponde al foco actual.
func (m Model) currentCell() navCell {
	switch m.focus {
	case focusRunningStart:
		return navCell{int(m.focus), m.focusedRunning, m.rowButton}
	case focusRecent:
		return navCell{int(m.focus), m.focusedRecent, m.rowButton}
	case focusWeek:
		return navCell{int(m.focus), 0, 1}
	case focusEditSave, focusEditDelete, focusEditCancel:
		return navCell{int(m.focus), 0, int(m.focus - focusEditSave)}
	}
	return navCell{int(m.focus), 0, 0}
}

// goTo pasa el foco a la celda indicada.
func (m *Model) goTo(c navCell) {
	previous := m.focus
	m.focus = focusTarget(c.kind)
	switch m.focus {
	case focusRunningStart:
		m.focusedRunning, m.rowButton = c.index, c.col
		m.revealRunning()
	case focusRecent:
		m.focusedRecent, m.rowButton = c.index, c.col
	case focusToday, focusWeek:
		if week := m.focus == focusWeek; week != m.week {
			m.week = week
			m.refresh()
		}
	}
	if m.focus == focusProject {
		m.closeProjectPicker()
	} else {
		if previous == focusProject {
			m.closeProjectPicker()
		}
		m.syncInputFocus()
	}
}

// revealRunning ajusta el desplazamiento para que la fila enfocada quede visible.
func (m *Model) revealRunning() {
	visible := max(1, len(m.computeLayout().running))
	if m.focusedRunning < m.runScroll {
		m.runScroll = m.focusedRunning
	}
	if m.focusedRunning >= m.runScroll+visible {
		m.runScroll = m.focusedRunning - visible + 1
	}
}

// clampFocus mantiene el foco sobre algo que existe después de refrescar los datos.
func (m *Model) clampFocus() {
	m.focusedRunning = min(max(m.focusedRunning, 0), max(0, len(m.running)-1))
	visible := len(m.computeLayout().recent)
	m.focusedRecent = min(max(m.focusedRecent, 0), max(0, visible-1))
	switch {
	case m.focus == focusRunningStart && len(m.running) == 0,
		m.focus == focusRecent && visible == 0,
		m.focus == focusUndo && !m.undoActive():
		m.focus = focusToday
	}
	m.rowButton = min(m.rowButton, max(0, len(m.rowActions())-1))
}

// rowActions devuelve los botones de la fila enfocada.
func (m Model) rowActions() []entryAction {
	if m.focus == focusRecent {
		if m.focusedRecent < len(m.recent) {
			return recentActions(m.recent[m.focusedRecent])
		}
		return nil
	}
	return runningActions
}

// focusedEntry devuelve la tarea de la fila enfocada.
func (m Model) focusedEntry() (tracking.Entry, bool) {
	switch {
	case m.focus == focusRunningStart && m.focusedRunning < len(m.running):
		return m.running[m.focusedRunning], true
	case m.focus == focusRecent && m.focusedRecent < len(m.recent):
		return m.recent[m.focusedRecent], true
	}
	return tracking.Entry{}, false
}

func (m *Model) activateRowButton() {
	entry, ok := m.focusedEntry()
	actions := m.rowActions()
	if !ok || m.rowButton >= len(actions) {
		return
	}
	switch actions[m.rowButton] {
	case entryResume:
		m.resume(entry)
	case entryEdit:
		m.openEdit(entry)
	case entryStop:
		m.stopFocused()
	default:
		m.askDelete(entry)
	}
}

// resume inicia una copia de la tarea detenida, salvo que una idéntica ya esté en curso.
func (m *Model) resume(entry tracking.Entry) {
	for _, running := range m.running {
		if running.Title == entry.Title && running.Project == entry.Project {
			m.setMessage("«" + entry.Title + "» ya está en curso")
			return
		}
	}
	copied, err := m.tracker.StartLike(entry.ID)
	if err != nil {
		m.setMessage(errorText("No se pudo reanudar", err))
		return
	}
	m.refresh()
	m.setMessage("Reanudado: " + copied.Title)
}

func (m *Model) askDelete(entry tracking.Entry) {
	m.confirm = &confirmState{entry: entry, button: 1}
}

func (m *Model) updateConfirm(key tea.KeyMsg) {
	switch key.Type {
	case tea.KeyLeft:
		m.confirm.button = 0
	case tea.KeyRight:
		m.confirm.button = 1
	case tea.KeyTab, tea.KeyShiftTab:
		m.confirm.button = 1 - m.confirm.button
	case tea.KeyEnter:
		m.activateConfirm(m.confirm.button)
	}
}

func (m *Model) activateConfirm(button int) {
	entry := m.confirm.entry
	m.confirm = nil
	if button == 0 {
		m.deleteEntry(entry)
	}
}

func (m *Model) deleteEntry(entry tracking.Entry) {
	if err := m.tracker.Delete(entry.ID); err != nil {
		m.setMessage(errorText("No se pudo eliminar", err))
		return
	}
	m.lastDeleted = &entry
	if m.edit != nil {
		m.closeEdit()
	}
	m.refresh()
	m.setMessage("Eliminada «" + entry.Title + "»")
	m.undoUntil = m.now.Add(undoWindow)
}

// restoreLast restaura la última tarea eliminada en la sesión.
func (m *Model) restoreLast() {
	if m.lastDeleted == nil {
		m.setMessage("No hay nada que deshacer")
		return
	}
	entry := *m.lastDeleted
	if err := m.tracker.Restore(entry.ID); err != nil {
		m.setMessage(errorText("No se pudo restaurar", err))
		return
	}
	m.lastDeleted = nil
	m.refresh()
	m.setMessage("Restaurada «" + entry.Title + "»")
}

// undoActive indica si el botón [Deshacer] sigue disponible.
func (m Model) undoActive() bool {
	return m.lastDeleted != nil && m.now.Before(m.undoUntil)
}

// setMessage reemplaza el estado; cualquier aviso nuevo oculta el botón [Deshacer].
func (m *Model) setMessage(text string) {
	m.message = text
	m.undoUntil = time.Time{}
	m.clampFocus()
}

// errorText traduce los errores de los casos de uso al español para la línea de estado.
func errorText(prefix string, err error) string {
	switch {
	case errors.Is(err, tracking.ErrEmptyTitle):
		return "El título no puede estar vacío"
	case errors.Is(err, tracking.ErrNotFound):
		return "La tarea ya no existe"
	case errors.Is(err, countdown.ErrBreakActive):
		return "Ya hay un break en curso"
	case errors.Is(err, countdown.ErrInvalidDuration):
		return "La duración debe estar entre 1 minuto y 8 horas"
	case errors.Is(err, countdown.ErrNoActiveBreak):
		return "No hay un break activo"
	}
	return prefix + ": " + err.Error()
}

// statusBar devuelve el texto de la línea de estado y los botones que lo acompañan.
func (m Model) statusBar() (string, []string) {
	if m.confirm != nil {
		return "¿Eliminar «" + truncate(m.confirm.entry.Title, 30) + "»? Quedará en la papelera.  ", confirmLabels
	}
	if m.undoActive() && m.message != "" {
		return m.message + " · ", undoLabels
	}
	return m.message, nil
}

// statusSelected es el botón resaltado de la barra de estado, o -1 si ninguno.
func (m Model) statusSelected() int {
	switch {
	case m.confirm != nil:
		return m.confirm.button
	case m.focus == focusUndo:
		return 0
	}
	return -1
}

// renderButtons dibuja botones [Etiqueta]; el enfocado lleva fondo de acento, no solo color.
func renderButtons(labels []string, selected int) string {
	return renderButtonsOn(labels, selected, false)
}

// renderButtonsOn es renderButtons para una fila enfocada, con el fondo sutil también entre botones.
func renderButtonsOn(labels []string, selected int, rowFocused bool) string {
	base := lipgloss.NewStyle().Foreground(accent)
	separator := buttonSeparator
	if rowFocused {
		base = base.Background(rowBg)
		separator = focusRow.Render(buttonSeparator)
	}
	parts := make([]string, len(labels))
	for i, label := range labels {
		style := base
		if i == selected {
			style = focusButton
		}
		parts[i] = style.Render(buttonText(label))
	}
	return strings.Join(parts, separator)
}

// composeRow une el texto de una fila con sus botones alineados a la derecha, según las zonas del layout.
// Una fila enfocada se dibuja con fondo sutil.
func composeRow(text string, x0 int, zones []rect, labels []string, selected int, focused bool) string {
	room := max(1, zones[0].x-x0-1)
	text = truncate(text, room)
	padding := strings.Repeat(" ", max(1, zones[0].x-x0-lipgloss.Width(text)))
	if focused {
		return focusRow.Render(text+padding) + renderButtonsOn(labels, selected, true)
	}
	return text + padding + renderButtons(labels, selected)
}

// selectedButton devuelve el botón resaltado de la fila (index, kind), o -1 si la fila no tiene el foco.
func (m Model) selectedButton(kind focusTarget, index int) int {
	if m.focus == kind && m.currentCell().index == index {
		return m.rowButton
	}
	return -1
}
