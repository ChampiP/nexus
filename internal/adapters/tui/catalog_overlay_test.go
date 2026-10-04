package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"nexus/internal/tracking"
)

func newWideCatalogModel(width, height int) (Model, *fakeCatalog) {
	cat := newFakeCatalog()
	m := NewModel(&testStore{}, WithCatalog(cat))
	m.width, m.height = width, height
	return m, cat
}

func TestCatalogOverlayOpenViaHeaderClickAndKeyboard(t *testing.T) {
	m, _ := newWideCatalogModel(140, 40)
	m.focus = focusProject

	// 1. Abrir mediante clic en el encabezado [Catálogo]
	tabZone := m.wideCatalogTabZone()
	if tabZone.w == 0 || tabZone.h == 0 {
		t.Fatal("wideCatalogTabZone devolvió zona vacía")
	}
	m = updateMouse(t, m, tea.MouseMsg{
		X:      tabZone.x,
		Y:      tabZone.y,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})

	if !m.isCatalogOverlayOpen() {
		t.Fatal("clic en el encabezado [Catálogo] debía abrir el panel superpuesto")
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "CATÁLOGO · Organización › Cliente › Proyecto") {
		t.Fatalf("la vista superpuesta debe contener el título del catálogo:\n%s", view)
	}
	if !strings.Contains(view, "✕ Cerrar") {
		t.Fatalf("la vista superpuesta debe contener el botón de cerrar [✕ Cerrar]:\n%s", view)
	}

	// 2. Cerrar mediante clic en [✕ Cerrar]
	geo, ok := m.catalogOverlayGeometry()
	if !ok {
		t.Fatal("catalogOverlayGeometry no disponible")
	}
	m = updateMouse(t, m, tea.MouseMsg{
		X:      geo.closeBtn.x,
		Y:      geo.closeBtn.y,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	if m.isCatalogOverlayOpen() {
		t.Fatal("clic en [✕ Cerrar] debía cerrar el panel superpuesto")
	}
	if m.focus != focusProject {
		t.Fatalf("foco después de cerrar por [✕ Cerrar] = %v, quería focusProject", m.focus)
	}

	// 3. Abrir mediante teclado (navegando con ↑ desde el tope o Enter en focusTabs)
	m.focus = focusTitle
	m = press(t, m, tea.KeyUp)
	if m.focus != focusTabs {
		t.Fatalf("↑ desde título debía enfocar focusTabs, obtuvo %v", m.focus)
	}
	m = press(t, m, tea.KeyEnter)
	if !m.isCatalogOverlayOpen() {
		t.Fatal("Enter en focusTabs debía abrir el panel superpuesto")
	}

	// 4. Cerrar con Esc restaura el foco
	m = press(t, m, tea.KeyEsc)
	if m.isCatalogOverlayOpen() {
		t.Fatal("Esc debía cerrar el panel superpuesto")
	}
	if m.focus != focusTabs {
		t.Fatalf("foco después de cerrar por Esc = %v, quería focusTabs", m.focus)
	}
}

func TestCatalogOverlayEscAndCloseRestorePreviousFocus(t *testing.T) {
	testCases := []struct {
		name       string
		startFocus focusTarget
		startCol   int
		closeBy    string // "esc" o "click"
	}{
		{"focusDescription_esc", focusDescription, 0, "esc"},
		{"focusProject_click", focusProject, 0, "click"},
		{"focusToday_esc", focusToday, 1, "esc"},
		{"focusToday_click", focusToday, 1, "click"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := newWideCatalogModel(160, 45)
			m.focus = tc.startFocus
			m.wideColumn = tc.startCol

			tabZone := m.wideCatalogTabZone()
			m = updateMouse(t, m, tea.MouseMsg{
				X:      tabZone.x,
				Y:      tabZone.y,
				Action: tea.MouseActionPress,
				Button: tea.MouseButtonLeft,
			})
			if !m.isCatalogOverlayOpen() {
				t.Fatal("panel superpuesto no abrió")
			}

			if tc.closeBy == "click" {
				geo, _ := m.catalogOverlayGeometry()
				m = updateMouse(t, m, tea.MouseMsg{
					X:      geo.closeBtn.x,
					Y:      geo.closeBtn.y,
					Action: tea.MouseActionPress,
					Button: tea.MouseButtonLeft,
				})
			} else {
				m = press(t, m, tea.KeyEsc)
			}

			if m.isCatalogOverlayOpen() {
				t.Fatal("panel superpuesto debía estar cerrado")
			}
			if m.focus != tc.startFocus {
				t.Fatalf("foco restaurado = %v, quería %v", m.focus, tc.startFocus)
			}
			if m.wideColumn != tc.startCol {
				t.Fatalf("columna restaurada = %d, quería %d", m.wideColumn, tc.startCol)
			}
		})
	}
}

func TestCatalogOverlayOutsideClicksIgnored(t *testing.T) {
	m, _ := newWideCatalogModel(160, 45)
	m.focus = focusDescription
	m.openCatalogOverlay()
	if !m.isCatalogOverlayOpen() {
		t.Fatal("overlay debía estar abierto")
	}

	geo, ok := m.catalogOverlayGeometry()
	if !ok {
		t.Fatal("geometría no disponible")
	}

	// Clics a la izquierda del panel (columna 0)
	outsideLeftX := geo.panelX - 5
	outsideLeftY := geo.panelY + 5
	m = updateMouse(t, m, tea.MouseMsg{
		X:      outsideLeftX,
		Y:      outsideLeftY,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	if !m.isCatalogOverlayOpen() {
		t.Fatal("clic fuera no debe cerrar el overlay")
	}
	if m.focus == focusTitle || m.focus == focusProject {
		t.Fatal("clic fuera no debe alterar el foco de las columnas inferiores")
	}

	// Clics a la derecha del panel (columna 2)
	outsideRightX := geo.panelX + geo.panelWidth + 5
	outsideRightY := geo.panelY + 5
	m = updateMouse(t, m, tea.MouseMsg{
		X:      outsideRightX,
		Y:      outsideRightY,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	if !m.isCatalogOverlayOpen() {
		t.Fatal("clic a la derecha fuera del panel no debe cerrar el overlay")
	}
}

func TestCatalogOverlayRenameUpdatesProjectPicker(t *testing.T) {
	m, fake := newWideCatalogModel(160, 45)
	m.focus = focusProject
	tabZone := m.wideCatalogTabZone()
	m = updateMouse(t, m, tea.MouseMsg{
		X:      tabZone.x,
		Y:      tabZone.y,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	if !m.isCatalogOverlayOpen() {
		t.Fatal("overlay debía abrir")
	}

	// Navegar a Lumirecon y renombrarlo a LumireconRenamed
	// Buscar la fila de Lumirecon
	lumIndex := -1
	for i, r := range m.cat.rows {
		if r.kind == catProject && r.name == "Lumirecon" {
			lumIndex = i
			break
		}
	}
	if lumIndex < 0 {
		t.Fatalf("no se encontró Lumirecon en rows: %+v", m.cat.rows)
	}

	m.cat.focus = catFocusRow
	m.cat.row = lumIndex
	m.cat.button = 0 // actRename es el botón 0

	// Activar renombrar
	m = press(t, m, tea.KeyEnter)
	if m.cat.edit == nil {
		t.Fatal("debía abrir campo de edición de nombre")
	}
	m.cat.input.SetValue("LumireconRenamed")
	m = press(t, m, tea.KeyEnter) // Confirmar renombrado

	if len(fake.calls) == 0 || !strings.Contains(fake.calls[len(fake.calls)-1], "LumireconRenamed") {
		t.Fatalf("RenameProject no fue llamado con el nuevo nombre: %v", fake.calls)
	}

	// Cerrar el overlay
	m = press(t, m, tea.KeyEsc)
	if m.isCatalogOverlayOpen() {
		t.Fatal("overlay debía cerrarse")
	}

	// Verificar que el selector de proyecto de la columna izquierda refleje el nuevo nombre inmediatamente
	options := m.projectOptions()
	found := false
	for _, opt := range options {
		if opt.name == "LumireconRenamed" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("el selector de proyectos no reflejó el renombrado: %+v", options)
	}
}

func TestCatalogOverlayHitboxAlignmentAt140And200(t *testing.T) {
	for _, width := range []int{140, 200} {
		t.Run(fmt.Sprintf("width_%d", width), func(t *testing.T) {
			m, _ := newWideCatalogModel(width, 45)
			tabZone := m.wideCatalogTabZone()
			m = updateMouse(t, m, tea.MouseMsg{
				X:      tabZone.x,
				Y:      tabZone.y,
				Action: tea.MouseActionPress,
				Button: tea.MouseButtonLeft,
			})
			if !m.isCatalogOverlayOpen() {
				t.Fatalf("overlay no abrió en ancho %d", width)
			}

			geo, ok := m.catalogOverlayGeometry()
			if !ok {
				t.Fatalf("catalogOverlayGeometry falló en ancho %d", width)
			}

			view := strings.Split(ansi.Strip(m.View()), "\n")

			// 1. Título y botón cerrar
			assertWideZoneMatchesLabel(t, view, geo.closeBtn, "[✕ Cerrar]")

			// 2. Botones de agregar
			adds := []string{"+ Organización", "+ Cliente", "+ Proyecto"}
			for i, label := range adds {
				b := geo.layout.adds[i]
				zone := rect{
					x: geo.contentX + b.x,
					y: geo.panelY + b.y,
					w: b.w,
					h: b.h,
				}
				assertWideZoneMatchesLabel(t, view, zone, label)
			}

			// 3. Botón de alternar archivados
			toggleRect := geo.layout.toggle
			toggleZone := rect{
				x: geo.contentX + toggleRect.x,
				y: geo.panelY + toggleRect.y,
				w: toggleRect.w,
				h: toggleRect.h,
			}
			assertWideZoneMatchesLabel(t, view, toggleZone, "Mostrar archivados")

			// 4. Botones en línea de la fila enfocada (por ejemplo Renombrar)
			if len(geo.layout.rows) > 0 && len(geo.layout.rows[0].buttons) > 0 {
				btn := geo.layout.rows[0].buttons[0]
				btnZone := rect{
					x: geo.contentX + btn.x,
					y: geo.panelY + btn.y,
					w: btn.w,
					h: btn.h,
				}
				assertWideZoneMatchesLabel(t, view, btnZone, "✎ Renombrar")
			}
		})
	}
}

func TestNarrowModeCatalogUnchanged(t *testing.T) {
	cat := newFakeCatalog()
	m := NewModel(&testStore{}, WithCatalog(cat))
	m.width, m.height = 100, 30

	if ok, _, _, _ := wideLayout(m.width); ok {
		t.Fatal("ancho 100 no debe ser considerado ancho")
	}

	// Cambiar a pantalla de catálogo por pestaña
	m.switchScreen(screenCatalog)
	if m.screen != screenCatalog {
		t.Fatalf("pantalla = %v, quería screenCatalog", m.screen)
	}
	if m.isCatalogOverlayOpen() {
		t.Fatal("modo estrecho no debe activar overlay")
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "ORGANIZACIÓN › CLIENTE › PROYECTO") {
		t.Fatalf("vista estrecha de catálogo debe ser la habitual:\n%s", view)
	}
}

func TestCaptureCatalogOverlayAt180x45(t *testing.T) {
	cat := newFakeCatalog()
	store := &testStore{
		running: []tracking.Entry{
			{ID: 1, Title: "Refactor arquitectura TUI", Project: "Depiloto", Kind: tracking.KindWork},
		},
		recent: []tracking.Entry{
			{ID: 2, TaskUID: "task-2", Title: "Auditoría base de datos", Project: "Depiloto", EndedAt: timePtr(1)},
			{ID: 3, TaskUID: "task-3", Title: "Diseño pausas activas", Project: "Lumirecon", EndedAt: timePtr(1)},
		},
		totals: []tracking.ProjectTotal{
			{Project: "Depiloto", Seconds: 7200},
			{Project: "Lumirecon", Seconds: 3600},
		},
	}
	m := NewModel(store, WithCatalog(cat))
	m.width, m.height = 180, 45
	m.openCatalogOverlay()
	view := ansi.Strip(m.View())
	t.Logf("ANSI-stripped capture at 180x45:\n%s", view)
}
