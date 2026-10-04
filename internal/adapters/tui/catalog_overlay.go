package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// catalogOverlayGeometry reúne las coordenadas y zonas del panel superpuesto del catálogo.
type catalogOverlayGeometry struct {
	panelX, panelY          int
	panelWidth, panelHeight int
	contentX, contentY      int
	innerWidth, innerHeight int
	closeBtn                rect
	layout                  catalogLayout
}

// isCatalogOverlayOpen indica si el catálogo está abierto como panel superpuesto en pantalla ancha.
func (m Model) isCatalogOverlayOpen() bool {
	return m.catOverlay && m.width >= 120
}

// openCatalogOverlay abre el panel superpuesto del catálogo y conserva el foco previo.
func (m *Model) openCatalogOverlay() {
	if m.width < 120 {
		m.switchScreen(screenCatalog)
		return
	}
	m.catOverlay = true
	m.catOverlayPrevFocus = m.focus
	m.catOverlayPrevCol = m.wideColumn
	m.loadCatalog()
	m.rebuildCatalogRows()
	if len(m.cat.rows) > 0 {
		m.cat.focus = catFocusRow
		m.cat.row = m.nearestEditable(0)
		m.cat.button = 0
	} else {
		m.cat.focus = catFocusAdd
		m.cat.button = 0
	}
	m.revealCatalog()
}

// closeCatalogOverlay cierra el panel superpuesto y devuelve el foco a su posición previa.
func (m *Model) closeCatalogOverlay() {
	m.cancelCatalogModal()
	m.catOverlay = false
	m.focus = m.catOverlayPrevFocus
	m.wideColumn = m.catOverlayPrevCol
	m.syncInputFocus()
	m.syncBreakFocus()
}

// catalogOverlayGeometry calcula la geometría del panel centrado al 70% del ancho.
func (m Model) catalogOverlayGeometry() (catalogOverlayGeometry, bool) {
	if m.width < 120 {
		return catalogOverlayGeometry{}, false
	}
	panelWidth := m.width * 70 / 100
	panelX := (m.width - panelWidth) / 2
	panelY := 1
	panelHeight := 40
	if m.height > 0 {
		footerLines := 3
		if status, labels := m.statusBar(); status != "" || len(labels) > 0 {
			footerLines++
		}
		panelHeight = max(10, m.height-panelY-footerLines)
	}
	innerWidth := panelWidth - 4
	innerHeight := panelHeight - 2
	contentX := panelX + 2
	contentY := panelY + 1
	closeW := lipgloss.Width(RenderButton("✕ Cerrar", false))
	closeBtn := rect{
		x: contentX + innerWidth - closeW,
		y: panelY + 1,
		w: closeW,
		h: 1,
	}
	l := m.computeCatalogLayout(innerWidth, innerHeight, true)
	return catalogOverlayGeometry{
		panelX:      panelX,
		panelY:      panelY,
		panelWidth:  panelWidth,
		panelHeight: panelHeight,
		contentX:    contentX,
		contentY:    contentY,
		innerWidth:  innerWidth,
		innerHeight: innerHeight,
		closeBtn:    closeBtn,
		layout:      l,
	}, true
}

// wideCatalogTabZone devuelve la zona interactiva del encabezado [Catálogo] en modo ancho.
func (m Model) wideCatalogTabZone() rect {
	x := lipgloss.Width(m.headerBase(false)) + 2
	w := lipgloss.Width(tabText(screenCatalog, true))
	return rect{x: x, y: 0, w: w, h: 1}
}

// renderCatalogOverlayPanel dibuja la caja con borde y contenido del panel superpuesto.
func (m Model) renderCatalogOverlayPanel(geo catalogOverlayGeometry) string {
	lines := m.renderCatalogLines(geo.innerWidth, geo.layout, true)
	body := strings.Join(lines, "\n")
	return panel.Width(geo.panelWidth - 2).Height(geo.panelHeight - 2).Render(body)
}

// compositeCatalogOverlay superpone el panel sobre las tres columnas del fondo.
func (m Model) compositeCatalogOverlay(baseView string) string {
	geo, ok := m.catalogOverlayGeometry()
	if !ok {
		return baseView
	}
	panelView := m.renderCatalogOverlayPanel(geo)
	overlayLines := strings.Split(panelView, "\n")
	bgLines := strings.Split(baseView, "\n")

	totalLines := len(bgLines)
	if m.height > 0 && m.height > totalLines {
		totalLines = m.height
	}
	if geo.panelY+len(overlayLines)+2 > totalLines {
		totalLines = geo.panelY + len(overlayLines) + 2
	}

	screenLines := make([]string, totalLines)
	if len(bgLines) > 0 {
		screenLines[0] = bgLines[0]
	}

	for i, fgLine := range overlayLines {
		y := geo.panelY + i
		if y >= totalLines {
			break
		}
		bg := ""
		if y < len(bgLines)-2 {
			bg = bgLines[y]
		}
		leftPart := ansi.Truncate(bg, geo.panelX, "")
		leftW := ansi.StringWidth(leftPart)
		if leftW < geo.panelX {
			leftPart += strings.Repeat(" ", geo.panelX-leftW)
		}
		rightPart := ansi.TruncateLeft(bg, geo.panelX+geo.panelWidth, "")
		screenLines[y] = leftPart + fgLine + rightPart
	}

	footer := m.catalogHint()
	if status, labels := m.statusBar(); status != "" || len(labels) > 0 {
		line := lipgloss.NewStyle().Foreground(accent).Render(status)
		if len(labels) > 0 {
			line += renderButtons(labels, m.statusSelected())
		}
		footer = line + "\n" + footer
	}
	footerLines := strings.Split(label.Render(footer), "\n")
	footerStart := totalLines - len(footerLines)
	for i, fLine := range footerLines {
		idx := footerStart + i
		if idx >= 0 && idx < totalLines {
			screenLines[idx] = fLine
		}
	}

	return strings.Join(screenLines, "\n")
}

// updateCatalogOverlayMouse gestiona los clics y la rueda dentro y fuera del panel superpuesto.
func (m *Model) updateCatalogOverlayMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	geo, ok := m.catalogOverlayGeometry()
	if !ok {
		return *m, nil
	}
	panelRect := rect{x: geo.panelX, y: geo.panelY, w: geo.panelWidth, h: geo.panelHeight}
	// Los clics fuera del panel no hacen nada y no alcanzan las columnas de abajo
	if !panelRect.contains(msg.X, msg.Y) {
		return *m, nil
	}
	// Clic sobre [✕ Cerrar] cierra el panel y devuelve el foco
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && geo.closeBtn.contains(msg.X, msg.Y) {
		m.closeCatalogOverlay()
		return *m, nil
	}
	// Clics y rueda dentro del panel: traducir a coordenadas relativas al contenido
	local := msg
	local.X = msg.X - geo.contentX
	local.Y = msg.Y - geo.panelY
	return m.updateCatalogMouse(local)
}
