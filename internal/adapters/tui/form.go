package tui

import (
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"nexus/internal/adapters/projectlist"
	"nexus/internal/tracking"
)

type projectOption struct {
	name    string
	client  string
	seconds int64
	create  bool
	current bool
}

func (m *Model) initInputs() {
	for _, placeholder := range []string{"Escribe un título", "Buscar o escribir un proyecto", "Descripción opcional"} {
		input := textinput.New()
		input.Placeholder = placeholder
		input.CharLimit = 120
		input.Width = 42
		m.inputs = append(m.inputs, input)
	}
	// El proyecto inicial es el último usado (no archivado), no uno del catálogo sin tareas.
	used := m.tracker.Projects("")
	for _, project := range m.pickerProjects("") {
		if slices.ContainsFunc(used, func(u tracking.ProjectUsage) bool { return strings.EqualFold(u.Name, project.Name) }) {
			m.selectedProject = project.Name
			m.inputs[1].SetValue(m.selectedProject)
			break
		}
	}
}

// pickerProjects une el catálogo con el uso registrado; es la misma lista que muestra la CLI.
func (m Model) pickerProjects(query string) []projectlist.Option {
	return projectlist.Build(m.tree, m.tracker.Projects(""), query)
}

func (m Model) projectOptions() []projectOption {
	query := ""
	if m.focus == focusProject && m.pickerOpen {
		query = strings.TrimSpace(m.inputs[1].Value())
	}
	projects := m.pickerProjects(query)
	options := make([]projectOption, 0, len(projects)+1)
	exact := false
	for _, project := range projects {
		options = append(options, projectOption{name: project.Name, client: project.Client, seconds: project.Seconds, current: strings.EqualFold(project.Name, m.selectedProject)})
		if strings.EqualFold(strings.TrimSpace(project.Name), query) {
			exact = true
		}
	}
	if query == "" {
		options = append([]projectOption{{name: "Sin proyecto", current: m.selectedProject == ""}}, options...)
	} else if !exact {
		createOption := projectOption{name: query, create: true}
		options = append(options, createOption)
	}
	return options
}

func (m *Model) openProjectPicker() {
	m.focus = focusProject
	m.pickerOpen = true
	m.inputs[1].Placeholder = m.selectedProject
	if m.inputs[1].Placeholder == "" {
		m.inputs[1].Placeholder = "Buscar o escribir un proyecto"
	}
	m.inputs[1].SetValue("")
	m.pickerIndex = 0
	for i, option := range m.projectOptions() {
		if option.current {
			m.pickerIndex = i
			break
		}
	}
	m.pickerScroll = 0
	m.syncInputFocus()
}

func (m *Model) closeProjectPicker() {
	m.pickerOpen = false
	m.inputs[1].SetValue(m.selectedProject)
	m.inputs[1].Placeholder = "Buscar o escribir un proyecto"
	m.syncInputFocus()
}

func (m *Model) keepProjectAndReturnToTitle() {
	m.closeProjectPicker()
	m.focus = focusTitle
	m.syncInputFocus()
}

func (m *Model) selectProject() {
	options := m.projectOptions()
	if len(options) == 0 {
		m.closeProjectPicker()
		m.focus = focusTitle
		m.syncInputFocus()
		return
	}
	if m.pickerIndex < 0 {
		m.pickerIndex = 0
	}
	if m.pickerIndex >= len(options) {
		m.pickerIndex = len(options) - 1
	}
	option := options[m.pickerIndex]
	if option.name == "Sin proyecto" {
		m.selectedProject = ""
	} else {
		m.selectedProject = option.name
	}
	m.inputs[1].SetValue(m.selectedProject)
	m.pickerOpen = false
	m.inputs[1].Placeholder = "Buscar o escribir un proyecto"
	m.focus = focusTitle
	m.syncInputFocus()
}

func projectOptionLabel(option projectOption) string {
	if option.create {
		return "Crear «" + option.name + "»"
	}
	if option.name == "Sin proyecto" {
		return option.name
	}
	name := option.name
	if option.client != "" {
		name += " · " + option.client
	}
	return name + "  ·  " + formatDuration(option.seconds)
}

func renderProjectOption(option projectOption, selected bool) string {
	style := label
	if selected {
		style = lipgloss.NewStyle().Bold(true).Foreground(accent)
	}
	prefix := "  "
	if selected {
		prefix = "▸ "
	}
	marker := ""
	if option.current {
		marker = " ✓"
	}
	return prefix + style.Render(projectOptionLabel(option)+marker)
}
