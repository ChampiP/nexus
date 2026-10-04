package tui

import (
	"slices"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"nexus/internal/adapters/projectlist"
	"nexus/internal/tracking"
)

type projectOption struct {
	id         int64
	name       string
	client     string
	clientID   int64
	clientSet  bool
	seconds    int64
	create     bool
	header     bool
	selectable bool
	current    bool
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
		if slices.ContainsFunc(used, func(u tracking.ProjectUsage) bool {
			if project.ID > 0 && u.ProjectID > 0 {
				return u.ProjectID == project.ID
			}
			return strings.EqualFold(u.Name, project.Name)
		}) {
			m.selectedProject = project.Name
			m.selectedProjectID = project.ID
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
	groups := projectlist.BuildGroups(m.tree, m.tracker.Projects(""), query)
	options := make([]projectOption, 0, len(groups)+1)
	exact := false
	clients := make(map[int64]string)
	for _, group := range groups {
		if group.Kind != projectlist.ProjectGroup {
			options = append(options, projectOption{name: group.Name, header: true})
			continue
		}
		project := group.Option
		current := false
		if m.selectedProjectID > 0 {
			current = project.ID == m.selectedProjectID
		} else if m.selectedProject != "" {
			current = project.ID == 0 && strings.EqualFold(project.Name, m.selectedProject)
		}
		options = append(options, projectOption{id: project.ID, name: project.Name, client: project.Client, clientID: project.ClientID, seconds: project.Seconds, current: current, selectable: true})
		if project.ClientID != 0 {
			clients[project.ClientID] = project.Client
		}
		if strings.EqualFold(strings.TrimSpace(project.Name), query) {
			exact = true
		}
	}
	if query == "" {
		options = append([]projectOption{{name: "Sin proyecto", selectable: true, current: m.selectedProjectID == 0 && m.selectedProject == ""}}, options...)
	} else if !exact {
		if m.catalog != nil {
			for _, organization := range m.tree {
				for _, client := range organization.Clients {
					if client.Client != nil {
						clients[client.Client.ID] = client.Client.Name
					}
				}
			}
		}
		clientIDs := make([]int64, 0, len(clients))
		for id := range clients {
			clientIDs = append(clientIDs, id)
		}
		sort.Slice(clientIDs, func(i, j int) bool {
			if clients[clientIDs[i]] != clients[clientIDs[j]] {
				return strings.ToLower(clients[clientIDs[i]]) < strings.ToLower(clients[clientIDs[j]])
			}
			return clientIDs[i] < clientIDs[j]
		})
		for _, id := range clientIDs {
			options = append(options, projectOption{name: query, client: clients[id], clientID: id, create: true, selectable: true})
		}
		options = append(options, projectOption{name: query, create: true, clientSet: m.catalog != nil, selectable: true})
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

// advanceFocusAfterProjectSelection pasa el foco tras seleccionar un proyecto:
// en el panel de edición avanza al siguiente campo (Descripción);
// en nueva tarea avanza a Descripción si está vacía, o a [Iniciar] si ya tiene contenido.
func (m *Model) advanceFocusAfterProjectSelection() {
	if m.edit != nil {
		m.focus = focusDescription
	} else if strings.TrimSpace(m.inputs[2].Value()) == "" {
		m.focus = focusDescription
	} else {
		m.focus = focusStart
	}
}

func (m *Model) selectProject() {
	options := m.projectOptions()
	if len(options) == 0 {
		m.closeProjectPicker()
		m.advanceFocusAfterProjectSelection()
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
	if !option.selectable {
		return
	}
	if option.create && m.catalog != nil {
		created, err := m.catalog.CreateProject(option.name, option.clientID)
		if err != nil {
			m.setMessage(errorText("No se pudo crear el proyecto", err))
			return
		}
		m.loadCatalog()
		if created.Name != "" {
			m.selectedProject = created.Name
		} else {
			m.selectedProject = option.name
		}
		m.selectedProjectID = created.ID
	} else if option.name == "Sin proyecto" {
		m.selectedProject = ""
		m.selectedProjectID = 0
	} else {
		m.selectedProject = option.name
		m.selectedProjectID = option.id
	}
	m.inputs[1].SetValue(m.selectedProject)
	m.pickerOpen = false
	m.inputs[1].Placeholder = "Buscar o escribir un proyecto"
	m.advanceFocusAfterProjectSelection()
	m.syncInputFocus()
}

func projectOptionLabel(option projectOption) string {
	if option.header {
		return option.name
	}
	if option.create {
		label := "Crear «" + option.name + "»"
		if option.client != "" {
			return label + " en " + option.client
		}
		if option.clientSet {
			return label + " sin cliente"
		}
		return label
	}
	if option.name == "Sin proyecto" {
		return option.name
	}
	return option.name + "  ·  " + formatDuration(option.seconds)
}

func renderProjectOption(option projectOption, selected bool) string {
	if option.header {
		indent := ""
		if option.name != "Sin organización" {
			indent = "  "
		}
		return lipgloss.NewStyle().Bold(true).Foreground(accent).Render(indent + option.name)
	}
	style := label
	if selected {
		style = lipgloss.NewStyle().Bold(true).Foreground(accent)
	}
	prefix := "    "
	if option.selectable && selected {
		prefix = "  ▸ "
	}
	if option.create {
		prefix = "    "
	}
	marker := ""
	if option.current {
		marker = " ✓"
	}
	return prefix + style.Render(projectOptionLabel(option)+marker)
}

func (m Model) selectedProjectPath() string {
	if m.selectedProjectID <= 0 {
		return m.selectedProject
	}
	for _, project := range m.pickerProjects("") {
		if project.ID == m.selectedProjectID {
			parts := make([]string, 0, 3)
			if project.Organization != "" {
				parts = append(parts, project.Organization)
			}
			if project.Client != "" {
				parts = append(parts, project.Client)
			}
			parts = append(parts, project.Name)
			return strings.Join(parts, " › ")
		}
	}
	return m.selectedProject
}

// projectClientName busca el nombre del cliente para un id de proyecto dentro del catálogo.
func (m Model) projectClientName(projectID int64) string {
	if projectID <= 0 {
		return ""
	}
	for _, g := range m.tree {
		for _, c := range g.Clients {
			for _, p := range c.Projects {
				if p.ID == projectID {
					if c.Client != nil {
						return c.Client.Name
					}
					return ""
				}
			}
		}
	}
	for _, p := range m.pickerProjects("") {
		if p.ID == projectID {
			return p.Client
		}
	}
	return ""
}
