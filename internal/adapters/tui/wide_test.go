package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"nexus/internal/tracking"
)

func TestWideLayout(t *testing.T) {
	tests := []struct {
		name                string
		width               int
		ok                  bool
		left, center, right int
	}{
		{name: "below threshold", width: 119},
		{name: "threshold", width: 120, ok: true, left: 40, center: 42, right: 36},
		{name: "wide terminal", width: 160, ok: true, left: 53, center: 56, right: 49},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, left, center, right := wideLayout(tt.width)
			if ok != tt.ok || left != tt.left || center != tt.center || right != tt.right {
				t.Fatalf("wideLayout(%d) = (%t, %d, %d, %d), want (%t, %d, %d, %d)", tt.width, ok, left, center, right, tt.ok, tt.left, tt.center, tt.right)
			}
			if ok && (left < 36 || center < 36 || right < 36) {
				t.Fatalf("wideLayout(%d) produced a column narrower than 36: %d, %d, %d", tt.width, left, center, right)
			}
		})
	}
}

func TestWideModeTypingAndProjectSelection(t *testing.T) {
	// Verificar reproducción en modo wide (ancho 180): escritura en título, selector de proyecto e inicio de temporizador.
	db := &testStore{
		projects: []tracking.ProjectUsage{
			{Name: "personal", Seconds: 50},
			{Name: "nexus", Seconds: 100},
		},
	}
	m := NewModel(db)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 45})
	m = next.(Model)

	// Escribir "Hola" en el campo Título.
	for _, r := range "Hola" {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(Model)
	}
	if got := m.inputs[0].Value(); got != "Hola" {
		t.Fatalf("valor de título = %q, quería %q", got, "Hola")
	}

	// Navegar a Proyecto y abrir selector con Enter.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	if m.focus != focusProject {
		t.Fatalf("foco = %v, quería focusProject", m.focus)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if !m.pickerOpen {
		t.Fatalf("el selector de proyectos no se abrió con Enter")
	}

	// Seleccionar proyecto con Enter (o navegando y seleccionando).
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.selectedProject == "" {
		t.Fatalf("el proyecto seleccionado no debería estar vacío")
	}

	// Iniciar temporizador con Enter.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if len(db.started) != 1 || db.started[0].Title != "Hola" {
		t.Fatalf("temporizador iniciado = %#v, quería título 'Hola'", db.started)
	}
}

func TestWideModeTypingTitleAndDescription(t *testing.T) {
	// Comprobar que en modo wide (ancho 180) se puede escribir tanto en Título como en Descripción.
	db := &testStore{}
	m := NewModel(db)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 45})
	m = next.(Model)

	// Escribir en Título.
	for _, r := range "Refactorizar TUI" {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(Model)
	}
	if got := m.inputs[0].Value(); got != "Refactorizar TUI" {
		t.Fatalf("título = %q, quería 'Refactorizar TUI'", got)
	}

	// Navegar a Descripción (↓ desde Título va a Proyecto, ↓ va a Descripción).
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	if m.focus != focusDescription {
		t.Fatalf("foco = %v, quería focusDescription", m.focus)
	}

	// Escribir en Descripción.
	for _, r := range "Soporte de escritura a 180 columnas" {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(Model)
	}
	if got := m.inputs[2].Value(); got != "Soporte de escritura a 180 columnas" {
		t.Fatalf("descripción = %q, quería 'Soporte de escritura a 180 columnas'", got)
	}

	// Iniciar con Enter desde Descripción.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if len(db.started) != 1 || db.started[0].Title != "Refactorizar TUI" || db.started[0].Description != "Soporte de escritura a 180 columnas" {
		t.Fatalf("tarea iniciada = %#v", db.started)
	}
}

func TestWideModePickerSearchAndKeyboardSelect(t *testing.T) {
	// Comprobar que al escribir en Proyecto se abre el selector, se filtra la búsqueda y se selecciona con el teclado.
	db := &testStore{
		projects: []tracking.ProjectUsage{
			{Name: "alpha", Seconds: 100},
			{Name: "nexus", Seconds: 200},
			{Name: "omega", Seconds: 300},
		},
	}
	m := NewModel(db)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 45})
	m = next.(Model)

	// Navegar a Proyecto.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)

	// Escribir 'n' 'e' 'x' para filtrar en el selector de proyectos.
	for _, r := range "nex" {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(Model)
	}
	if !m.pickerOpen {
		t.Fatalf("el selector debía abrirse al escribir")
	}
	if got := m.inputs[1].Value(); got != "nex" {
		t.Fatalf("búsqueda del selector = %q, quería 'nex'", got)
	}

	// Seleccionar la opción filtrada con Enter.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.pickerOpen {
		t.Fatalf("el selector debía cerrarse tras presionar Enter")
	}
	if m.selectedProject != "nexus" {
		t.Fatalf("proyecto seleccionado = %q, quería 'nexus'", m.selectedProject)
	}
	if m.focus != focusDescription {
		t.Fatalf("tras seleccionar proyecto el foco debía volver a focusDescription, obtuvo %v", m.focus)
	}
}

func TestWideModePickerMouseSelect(t *testing.T) {
	// Comprobar que en modo wide (ancho 180) la selección de proyectos mediante clic del ratón funciona correctamente.
	db := &testStore{
		projects: []tracking.ProjectUsage{
			{Name: "cliente-x", Seconds: 100},
			{Name: "cliente-y", Seconds: 200},
		},
	}
	m := NewModel(db)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 45})
	m = next.(Model)

	_, layout, ok := m.wideMouseLayout()
	if !ok {
		t.Fatal("no se pudo obtener la disposición de ratón para modo ancho")
	}

	// Clic en el campo Proyecto para abrir el selector.
	projectZone := layout.fields[1]
	next, _ = m.Update(tea.MouseMsg{X: projectZone.x, Y: projectZone.y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	m = next.(Model)
	if !m.pickerOpen {
		t.Fatalf("el selector debía abrirse al hacer clic en el campo proyecto")
	}

	// Obtener zonas de opciones y hacer clic en la primera opción seleccionable.
	_, layout, _ = m.wideMouseLayout()
	if len(layout.options) == 0 {
		t.Fatal("se esperaban opciones en el selector de proyectos")
	}
	optionZone := layout.options[0]
	next, _ = m.Update(tea.MouseMsg{X: optionZone.x, Y: optionZone.y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	m = next.(Model)
	if m.pickerOpen {
		t.Fatalf("el selector debía cerrarse al hacer clic en una opción")
	}
	if m.focus != focusDescription {
		t.Fatalf("tras seleccionar proyecto el foco debía regresar a focusDescription, obtuvo %v", m.focus)
	}
}

func TestWideModeSingleFocusConsistency(t *testing.T) {
	// Verificar que en todo momento solo un elemento visual muestre foco consistente en modo wide (ancho 180).
	db := &testStore{
		projects: []tracking.ProjectUsage{
			{Name: "nexus", Seconds: 100},
		},
	}
	m := NewModel(db)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 45})
	m = next.(Model)

	// Estado inicial: Título enfocado.
	if !m.inputs[0].Focused() {
		t.Fatal("el campo Título debía tener el cursor de foco")
	}
	if m.inputs[1].Focused() || m.inputs[2].Focused() {
		t.Fatal("los campos de Proyecto o Descripción no deben tener foco simultáneo")
	}

	// Al navegar a Proyecto, Título debe perder el foco inmediatamente.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	if m.focus != focusProject {
		t.Fatalf("foco = %v, quería focusProject", m.focus)
	}
	if m.inputs[0].Focused() {
		t.Fatal("el campo Título no debe seguir enfocado al pasar a Proyecto")
	}

	// Al navegar a la columna 1 (Dashboard), ningún input de la columna 0 debe estar enfocado.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(Model)
	if m.wideColumn != 1 {
		t.Fatalf("columna esperada = 1, obtuvo %d", m.wideColumn)
	}
	if m.inputs[0].Focused() || m.inputs[1].Focused() || m.inputs[2].Focused() {
		t.Fatal("ningún campo de texto debe tener foco activo cuando la columna activa es la del dashboard")
	}

	// En la columna 1, flecha derecha pasa de Hoy a Semana, y luego a la columna 2 (Breaks).
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight}) // pasa a Semana
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight}) // pasa a la columna 2
	m = next.(Model)
	if m.wideColumn != 2 {
		t.Fatalf("columna esperada = 2, obtuvo %d", m.wideColumn)
	}
	if m.inputs[0].Focused() || m.inputs[1].Focused() || m.inputs[2].Focused() {
		t.Fatal("ningún campo de texto debe tener foco activo en la columna de breaks")
	}
}
