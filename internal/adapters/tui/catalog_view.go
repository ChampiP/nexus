package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// catalogView dibuja la pantalla de catálogo usando las zonas de catalogLayout.
func (m Model) catalogView() string {
	l := m.catalogLayout()
	c := m.cat
	width := m.width
	lines := make([]string, l.modalY)
	lines[0] = titleStyle.Render("NEXUS") + "  " + renderTabs(1, c.focus == catFocusTabs)
	heading := "ORGANIZACIÓN › CLIENTE › PROYECTO"
	if l.visible < len(c.rows) {
		heading += fmt.Sprintf("  ·  filas %d-%d de %d", l.first+1, l.first+l.visible, len(c.rows))
	}
	lines[l.headingY] = titleStyle.Render(heading)
	if len(c.rows) == 0 {
		lines[l.treeY] = label.Render("El catálogo está vacío; crea una organización, un cliente o un proyecto")
	}
	for i, zone := range l.rows {
		lines[zone.row.y] = m.renderCatalogRow(l.first+i, zone, width)
	}
	lines[l.addY] = m.renderAddRow(l)
	toggle := buttonText(toggleLabel(c.showArchived))
	if c.focus == catFocusToggle {
		toggle = lipgloss.NewStyle().Foreground(accent).Bold(true).Reverse(true).Render(toggle)
	} else {
		toggle = lipgloss.NewStyle().Foreground(accent).Render(toggle)
	}
	lines[l.toggleY] = "  " + toggle
	if c.edit != nil && c.edit.create {
		lines[l.addY] = "  " + label.Render(createPrompt(c.edit.kind)) + c.input.View()
	}
	switch {
	case c.picker != nil:
		options := m.pickerOptions()
		lines = append(lines, titleStyle.Render(c.picker.title), c.input.View())
		for i := range l.options {
			index := l.optionsStart + i
			prefix, style := "  ", label
			if index == c.picker.index {
				prefix, style = "▸ ", lipgloss.NewStyle().Bold(true).Foreground(accent)
			}
			lines = append(lines, prefix+style.Render(options[index].label))
		}
		if len(options) == 0 {
			lines = append(lines, label.Render("  Sin coincidencias"))
		}
	case c.confirm != nil:
		selected := c.confirm.button
		lines = append(lines, lipgloss.NewStyle().Foreground(accent).Render(c.confirm.text), "  "+renderButtons(c.confirm.labels(), selected))
	}
	footer := m.catalogHint()
	if m.message != "" {
		footer = lipgloss.NewStyle().Foreground(accent).Render(m.message) + "\n" + footer
	}
	return strings.Join(lines, "\n") + "\n\n" + label.Render(footer)
}

func createPrompt(kind catRowKind) string {
	switch kind {
	case catOrg:
		return "Nueva organización: "
	case catClient:
		return "Nuevo cliente: "
	}
	return "Nuevo proyecto: "
}

func (m Model) renderAddRow(l catalogLayout) string {
	selected := -1
	if m.cat.focus == catFocusAdd {
		selected = m.cat.button
	}
	return "  " + renderButtons(addLabels, selected)
}

// renderCatalogRow dibuja la fila index del árbol: sangría, nombre, estadísticas o botones en línea.
func (m Model) renderCatalogRow(index int, zone rowZones, width int) string {
	c := m.cat
	r := c.rows[index]
	focused := c.focus == catFocusRow && c.row == index
	prefix := "  "
	if focused {
		prefix = "▸ "
	}
	indent := strings.Repeat("  ", r.depth)
	if focused && c.edit != nil && !c.edit.create {
		return prefix + indent + label.Render("Renombrar: ") + c.input.View()
	}
	name := r.name
	if r.archived {
		name += " (archivado)"
	}
	switch r.kind {
	case catHeader:
		return "  " + indent + label.Render(name)
	case catOrg:
		name = titleStyle.Render(name)
	}
	stats := ""
	if r.kind == catProject {
		stats = fmt.Sprintf("%s esta semana · %s", formatDuration(r.seconds), pluralize(r.tasks, "tarea", "tareas"))
	}
	left := prefix + indent
	if len(zone.buttons) > 0 {
		text := left + truncate(r.name, 28)
		if r.archived {
			text += " (archivado)"
		}
		if r.kind == catProject {
			// Con los botones a la vista el espacio es corto: las estadísticas van solo si caben enteras.
			compact := fmt.Sprintf("  ·  %s · %s", formatDuration(r.seconds), pluralize(r.tasks, "tarea", "tareas"))
			if lipgloss.Width(text+compact) < zone.buttons[0].x {
				text += compact
			}
		}
		return composeRow(text, 0, zone.buttons, rowLabels(r), c.button)
	}
	if stats == "" {
		return left + name
	}
	padding := max(2, width-lipgloss.Width(left+name)-lipgloss.Width(stats)-1)
	return left + name + strings.Repeat(" ", padding) + label.Render(stats)
}

func (m Model) catalogHint() string {
	c := m.cat
	switch {
	case c.confirm != nil:
		return "←→ elegir · Enter confirmar · Esc cancelar · clic en un botón"
	case c.picker != nil:
		return "Escribe para buscar · ↑↓ elegir · Enter seleccionar · Esc cancelar"
	case c.edit != nil:
		return "Enter guardar · Esc cancelar"
	}
	switch c.focus {
	case catFocusTabs:
		return "←→ o Enter cambiar de pantalla · ↓ continuar · clic en una pestaña"
	case catFocusRow:
		return "↑↓ mover · ←→ elegir acción · Enter ejecutar · Esc volver"
	}
	return "↑↓ mover · ←→ elegir · Enter ejecutar · Esc volver"
}
