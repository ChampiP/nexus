package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"nexus/internal/tracking"
)

// formDraft guarda el formulario principal mientras el panel de edición reutiliza sus campos.
type formDraft struct {
	title, description, project string
	focus                       focusTarget
}

// editState es el panel de edición abierto sobre una tarea.
type editState struct {
	entry tracking.Entry
	draft formDraft
}

// openEdit reemplaza el formulario superior por el panel de edición precargado con la tarea.
func (m *Model) openEdit(entry tracking.Entry) {
	if m.edit != nil {
		return
	}
	m.edit = &editState{entry: entry, draft: formDraft{m.inputs[0].Value(), m.inputs[2].Value(), m.selectedProject, m.focus}}
	m.selectedProject = entry.Project
	m.inputs[0].SetValue(entry.Title)
	m.inputs[1].SetValue(entry.Project)
	m.inputs[2].SetValue(entry.Description)
	m.focus = focusTitle
	m.syncInputFocus()
}

// closeEdit cierra el panel y devuelve el formulario principal y el foco a su estado previo.
func (m *Model) closeEdit() {
	if m.edit == nil {
		return
	}
	draft := m.edit.draft
	m.closeProjectPicker()
	m.edit = nil
	m.inputs[0].SetValue(draft.title)
	m.inputs[2].SetValue(draft.description)
	m.selectedProject = draft.project
	m.inputs[1].SetValue(draft.project)
	m.focus = draft.focus
	m.syncInputFocus()
	m.clampFocus()
}

// saveEdit envía solo los campos modificados; sin cambios simplemente cierra el panel.
func (m *Model) saveEdit() {
	if m.pickerOpen {
		m.closeProjectPicker()
	}
	entry := m.edit.entry
	var input tracking.EditInput
	if title := m.inputs[0].Value(); strings.TrimSpace(title) != entry.Title {
		input.Title = &title
	}
	if m.selectedProject != entry.Project {
		project := m.selectedProject
		input.Project = &project
	}
	if description := strings.TrimSpace(m.inputs[2].Value()); description != entry.Description {
		input.Description = &description
	}
	if input == (tracking.EditInput{}) {
		m.closeEdit()
		m.setMessage("Sin cambios")
		return
	}
	updated, err := m.tracker.Edit(entry.ID, input)
	if err != nil {
		m.setMessage(errorText("No se pudo guardar", err))
		return
	}
	m.closeEdit()
	m.refresh()
	m.setMessage("Guardado: " + updated.Title)
}

// updateEditButtons procesa las teclas cuando el foco está en [Guardar] [Eliminar] [Cancelar].
func (m *Model) updateEditButtons(key tea.KeyMsg) {
	switch key.Type {
	case tea.KeyLeft:
		m.focus = max(focusEditSave, m.focus-1)
	case tea.KeyRight:
		m.focus = min(focusEditCancel, m.focus+1)
	case tea.KeyUp, tea.KeyShiftTab:
		m.moveFocus(-1)
	case tea.KeyDown, tea.KeyTab:
		m.moveFocus(1)
	case tea.KeyEnter:
		m.activateEditButton(m.focus)
	}
}

func (m *Model) activateEditButton(button focusTarget) {
	m.focus = button
	if m.pickerOpen {
		m.closeProjectPicker()
	}
	switch button {
	case focusEditSave:
		m.saveEdit()
	case focusEditDelete:
		m.askDelete(m.edit.entry)
	case focusEditCancel:
		m.closeEdit()
	}
}

// editTitle es el encabezado del panel de edición.
func (m Model) editTitle() string {
	return fmt.Sprintf("Editar tarea #%d", m.edit.entry.ID)
}

// editButtonsLine dibuja la fila de botones del panel, resaltando el que tiene el foco.
func (m Model) editButtonsLine() string {
	selected := int(m.focus) - int(focusEditSave)
	if m.focus < focusEditSave {
		selected = -1
	}
	return renderButtons(editLabels, selected)
}
