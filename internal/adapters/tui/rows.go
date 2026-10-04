package tui

import (
	"errors"
	"fmt"
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
	task tracking.TaskSummary
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

// recentActions son los botones de una tarea detenida en RECIENTES.
func recentActions() []entryAction {
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
	} else if previous == focusProject {
		m.closeProjectPicker()
	}
	m.syncInputFocus()
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
			return recentActions()
		}
		return nil
	}
	return runningActions
}

// focusedTask devuelve la tarea de la fila enfocada y, si hay una sesión en curso en esa fila, su id.
func (m Model) focusedTask() (task tracking.TaskSummary, runningID int64, ok bool) {
	switch {
	case m.focus == focusRunningStart && m.focusedRunning < len(m.running):
		entry := m.running[m.focusedRunning]
		return m.taskOf(entry), entry.ID, true
	case m.focus == focusRecent && m.focusedRecent < len(m.recent):
		task = m.recent[m.focusedRecent]
		return task, task.RunningEntryID, true
	}
	return tracking.TaskSummary{}, 0, false
}

// taskOf devuelve el resumen de la tarea de una sesión; si no está en la lista usa solo la sesión.
func (m Model) taskOf(entry tracking.Entry) tracking.TaskSummary {
	if task, ok := m.tasks[entry.TaskUID]; ok {
		return task
	}
	return tracking.TaskSummary{TaskUID: entry.TaskUID, Title: entry.Title, Project: entry.Project, Description: entry.Description, LastEntryID: entry.ID, SessionCount: 1}
}

func (m *Model) activateRowButton() {
	task, runningID, ok := m.focusedTask()
	actions := m.rowActions()
	if !ok || m.rowButton >= len(actions) {
		return
	}
	switch actions[m.rowButton] {
	case entryResume:
		m.resume(task)
	case entryEdit:
		m.openEdit(task)
	case entryStop:
		m.stopSession(runningID, task.Title)
	default:
		m.askDelete(task)
	}
}

// resume inicia una sesión nueva de la tarea, salvo que ya tenga una en curso.
func (m *Model) resume(task tracking.TaskSummary) {
	resumed, err := m.tracker.Resume(task.LastEntryID)
	if errors.Is(err, tracking.ErrAlreadyRunning) {
		m.setMessage("«" + task.Title + "» ya está en curso")
		m.refresh()
		return
	}
	if err != nil {
		m.setMessage(errorText("No se pudo reanudar", err))
		return
	}
	m.refresh()
	m.setMessage("Reanudado: " + resumed.Title)
}

func (m *Model) askDelete(task tracking.TaskSummary) {
	m.confirm = &confirmState{task: task, button: 1}
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
	task := m.confirm.task
	m.confirm = nil
	if button == 0 {
		m.deleteTask(task)
	}
}

func (m *Model) deleteTask(task tracking.TaskSummary) {
	if err := m.tracker.DeleteTask(task.TaskUID); err != nil {
		m.setMessage(errorText("No se pudo eliminar", err))
		return
	}
	m.lastDeleted = &task
	if m.edit != nil {
		m.closeEdit()
	}
	m.refresh()
	m.setMessage("Eliminada «" + task.Title + "»")
	m.undoUntil = m.now.Add(undoWindow)
}

// restoreLast restaura la última tarea eliminada en la sesión, con todas sus sesiones.
func (m *Model) restoreLast() {
	if m.lastDeleted == nil {
		m.setMessage("No hay nada que deshacer")
		return
	}
	task := *m.lastDeleted
	if err := m.tracker.RestoreTask(task.TaskUID); err != nil {
		m.setMessage(errorText("No se pudo restaurar", err))
		return
	}
	m.lastDeleted = nil
	m.refresh()
	m.setMessage("Restaurada «" + task.Title + "»")
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
		return "¿Eliminar «" + truncate(m.confirm.task.Title, 30) + "» y " + sessionsText(m.confirm.task.SessionCount) + "? Quedará en la papelera.  ", confirmLabels
	}
	if m.undoActive() && m.message != "" {
		return m.message + " · ", undoLabels
	}
	return m.message, nil
}

// sessionsText cuenta las sesiones de una tarea: «su sesión» o «sus N sesiones».
func sessionsText(n int) string {
	if n <= 1 {
		return "su sesión"
	}
	return fmt.Sprintf("sus %d sesiones", n)
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

// renderButtons dibuja botones como píldoras coloreadas.
func renderButtons(labels []string, selected int) string {
	return renderButtonsOn(labels, selected, false)
}

// renderButtonsOn dibuja botones como píldoras, con fondo sutil en el separador si la fila está enfocada.
func renderButtonsOn(labels []string, selected int, rowFocused bool) string {
	separator := buttonSeparator
	if rowFocused {
		separator = focusRow.Render(buttonSeparator)
	}
	parts := make([]string, len(labels))
	for i, label := range labels {
		parts[i] = RenderButton(label, i == selected)
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
