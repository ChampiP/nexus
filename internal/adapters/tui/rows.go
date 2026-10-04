package tui

import (
	"errors"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

// focusStop es una parada del anillo de foco: un tipo de destino y, en filas, su índice.
type focusStop struct {
	kind  focusTarget
	index int
}

func (m Model) focusRing() []focusStop {
	if m.edit != nil {
		return []focusStop{{focusTitle, 0}, {focusProject, 0}, {focusDescription, 0}, {focusEditSave, 0}, {focusEditDelete, 0}, {focusEditCancel, 0}}
	}
	var stops []focusStop
	if m.catalogEnabled() {
		stops = append(stops, focusStop{focusTabs, 0})
	}
	stops = append(stops, focusStop{focusTitle, 0}, focusStop{focusProject, 0}, focusStop{focusDescription, 0}, focusStop{focusStart, 0})
	for i := range m.running {
		stops = append(stops, focusStop{focusRunningStart, i})
	}
	stops = append(stops, focusStop{focusToday, 0}, focusStop{focusWeek, 0})
	for i := range m.computeLayout().recent {
		stops = append(stops, focusStop{focusRecent, i})
	}
	if m.undoActive() {
		stops = append(stops, focusStop{focusUndo, 0})
	}
	return stops
}

func (m Model) currentStop() focusStop {
	switch m.focus {
	case focusRunningStart:
		return focusStop{m.focus, m.focusedRunning}
	case focusRecent:
		return focusStop{m.focus, m.focusedRecent}
	}
	return focusStop{m.focus, 0}
}

func (m *Model) moveFocus(step int) {
	ring := m.focusRing()
	at := 0
	current := m.currentStop()
	for i, stop := range ring {
		if stop == current {
			at = i
			break
		}
	}
	at = min(max(at+step, 0), len(ring)-1)
	stop := ring[at]
	previous := m.focus
	if stop != current {
		m.rowButton = 0
	}
	m.focus = stop.kind
	switch stop.kind {
	case focusRunningStart:
		m.focusedRunning = stop.index
		m.revealRunning()
	case focusRecent:
		m.focusedRecent = stop.index
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
	m.rowButton = min(m.rowButton, max(0, len(m.rowButtons())-1))
}

// rowButtons devuelve las etiquetas de los botones de la fila enfocada.
func (m Model) rowButtons() []string {
	if m.focus == focusRecent {
		return recentLabels
	}
	return runningLabels
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

// updateRowKey procesa ←/→/Enter sobre una fila; devuelve false si la tecla no le corresponde.
func (m *Model) updateRowKey(key tea.KeyMsg) bool {
	switch key.Type {
	case tea.KeyLeft:
		m.rowButton = max(0, m.rowButton-1)
	case tea.KeyRight:
		m.rowButton = min(len(m.rowButtons())-1, m.rowButton+1)
	case tea.KeyEnter:
		m.activateRowButton()
	default:
		return false
	}
	return true
}

func (m *Model) activateRowButton() {
	entry, ok := m.focusedEntry()
	if !ok {
		return
	}
	switch {
	case m.rowButton == 0:
		m.openEdit(entry)
	case m.focus == focusRunningStart:
		m.stopFocused()
	default:
		m.askDelete(entry)
	}
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

// renderButtons dibuja botones [Etiqueta]; el resaltado usa video inverso, no solo color.
func renderButtons(labels []string, selected int) string {
	parts := make([]string, len(labels))
	for i, label := range labels {
		style := lipgloss.NewStyle().Foreground(accent)
		if i == selected {
			style = style.Bold(true).Reverse(true)
		}
		parts[i] = style.Render(buttonText(label))
	}
	return strings.Join(parts, buttonSeparator)
}

// composeRow une el texto de una fila con sus botones alineados a la derecha, según las zonas del layout.
func composeRow(text string, x0 int, zones []rect, labels []string, selected int) string {
	room := max(1, zones[0].x-x0-1)
	text = truncate(text, room)
	padding := max(1, zones[0].x-x0-lipgloss.Width(text))
	return text + strings.Repeat(" ", padding) + renderButtons(labels, selected)
}

// selectedButton devuelve el botón resaltado de la fila (index, kind), o -1 si la fila no tiene el foco.
func (m Model) selectedButton(kind focusTarget, index int) int {
	if m.focus == kind && m.currentStop().index == index {
		return m.rowButton
	}
	return -1
}
