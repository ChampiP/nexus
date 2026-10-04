package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"nexus/internal/tracking"
)

// TestPickerSelectMovesFocusToDescriptionWhenEmpty comprueba que al seleccionar un proyecto
// con Enter cuando Descripción está vacía, el foco pase a Descripción y la escritura se inserte allí.
func TestPickerSelectMovesFocusToDescriptionWhenEmpty(t *testing.T) {
	for _, width := range []int{100, 180} {
		t.Run(testModeName(width), func(t *testing.T) {
			db := &testStore{projects: []tracking.ProjectUsage{{Name: "nexus"}, {Name: "tbwa"}}}
			m := NewModel(db)
			next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
			m = next.(Model)

			// Abrir selector de proyectos y elegir con Enter.
			m.focus = focusProject
			m.openProjectPicker()
			if !m.pickerOpen {
				t.Fatal("el selector de proyectos debía estar abierto")
			}

			next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = next.(Model)

			if m.pickerOpen {
				t.Fatal("el selector de proyectos debía cerrarse tras presionar Enter")
			}
			if m.focus != focusDescription {
				t.Fatalf("foco = %v, quería focusDescription", m.focus)
			}

			// Escribir texto y verificar que va directo al campo Descripción.
			m = typeText(t, m, "texto descriptivo")
			if got := m.inputs[2].Value(); got != "texto descriptivo" {
				t.Fatalf("valor en Descripción = %q, quería 'texto descriptivo'", got)
			}
		})
	}
}

// TestPickerSelectMovesFocusToStartWhenDescriptionFilled comprueba que al seleccionar un proyecto
// con Enter cuando Descripción ya tiene contenido, el foco pase directamente a Iniciar.
func TestPickerSelectMovesFocusToStartWhenDescriptionFilled(t *testing.T) {
	for _, width := range []int{100, 180} {
		t.Run(testModeName(width), func(t *testing.T) {
			db := &testStore{projects: []tracking.ProjectUsage{{Name: "nexus"}, {Name: "tbwa"}}}
			m := NewModel(db)
			next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
			m = next.(Model)

			// Descripción precargada.
			m.inputs[2].SetValue("descripción previa")

			m.focus = focusProject
			m.openProjectPicker()
			next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = next.(Model)

			if m.pickerOpen {
				t.Fatal("el selector de proyectos debía cerrarse tras presionar Enter")
			}
			if m.focus != focusStart {
				t.Fatalf("foco = %v, quería focusStart", m.focus)
			}
		})
	}
}

// TestPickerMouseSelectMovesFocus comprueba que seleccionar una opción con clic del ratón
// pase el foco a Descripción si está vacía, o a Iniciar si ya está llena.
func TestPickerMouseSelectMovesFocus(t *testing.T) {
	for _, width := range []int{100, 180} {
		t.Run(testModeName(width)+"_vacio", func(t *testing.T) {
			db := &testStore{projects: []tracking.ProjectUsage{{Name: "nexus"}, {Name: "tbwa"}}}
			m := NewModel(db)
			next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 35})
			m = next.(Model)

			// Abrir selector.
			m.focus = focusProject
			m.openProjectPicker()

			optionX, optionY := pickerOptionCoord(t, m, width, 0)
			next, _ = m.Update(tea.MouseMsg{X: optionX, Y: optionY, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
			m = next.(Model)

			if m.pickerOpen {
				t.Fatal("el selector de proyectos debía cerrarse al hacer clic")
			}
			if m.focus != focusDescription {
				t.Fatalf("foco = %v, quería focusDescription", m.focus)
			}

			// Verificar que la escritura va al campo Descripción.
			m = typeText(t, m, "por raton")
			if got := m.inputs[2].Value(); got != "por raton" {
				t.Fatalf("valor en Descripción = %q, quería 'por raton'", got)
			}
		})

		t.Run(testModeName(width)+"_lleno", func(t *testing.T) {
			db := &testStore{projects: []tracking.ProjectUsage{{Name: "nexus"}, {Name: "tbwa"}}}
			m := NewModel(db)
			next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 35})
			m = next.(Model)

			m.inputs[2].SetValue("ya lleno")
			m.focus = focusProject
			m.openProjectPicker()

			optionX, optionY := pickerOptionCoord(t, m, width, 0)
			next, _ = m.Update(tea.MouseMsg{X: optionX, Y: optionY, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
			m = next.(Model)

			if m.pickerOpen {
				t.Fatal("el selector de proyectos debía cerrarse al hacer clic")
			}
			if m.focus != focusStart {
				t.Fatalf("foco = %v, quería focusStart", m.focus)
			}
		})
	}
}

// TestPickerReproducedSequenceStartsTimerWithoutProjectCreation comprueba la secuencia exacta reproducida:
// seleccionar proyecto con Enter → escribir texto de descripción → presionar Enter inicia el temporizador
// con esa descripción y no crea ningún proyecto accidental en el catálogo.
func TestPickerReproducedSequenceStartsTimerWithoutProjectCreation(t *testing.T) {
	for _, width := range []int{100, 180} {
		t.Run(testModeName(width), func(t *testing.T) {
			cat := newFakeCatalog()
			db := &testStore{projects: []tracking.ProjectUsage{{Name: "nexus"}}}
			m := NewModel(db, WithCatalog(cat))
			next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 35})
			m = next.(Model)

			// Título establecido en el formulario.
			m.inputs[0].SetValue("Mi tarea nueva")

			// Abrir selector en Proyecto y seleccionar con Enter.
			m.focus = focusProject
			m.openProjectPicker()
			next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = next.(Model)

			if m.focus != focusDescription {
				t.Fatalf("tras selección de proyecto el foco debía ser focusDescription, obtuvo %v", m.focus)
			}

			// Escribir la descripción deseada.
			descText := "Mi texto de descripcion"
			m = typeText(t, m, descText)

			if got := m.inputs[2].Value(); got != descText {
				t.Fatalf("la descripción debía contener %q, obtuvo %q", descText, got)
			}

			// Presionar Enter para iniciar el temporizador.
			next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = next.(Model)

			// Verificar que se inició el temporizador con la descripción ingresada.
			if len(db.started) != 1 {
				t.Fatalf("se esperaba 1 temporizador iniciado, hubo %d", len(db.started))
			}
			started := db.started[0]
			if started.Title != "Mi tarea nueva" {
				t.Fatalf("título = %q, quería 'Mi tarea nueva'", started.Title)
			}
			if started.Description != descText {
				t.Fatalf("descripción = %q, quería %q", started.Description, descText)
			}

			// Verificar que NO se haya llamado a CreateProject en el catálogo.
			for _, call := range cat.calls {
				if strings.HasPrefix(call, "CreateProject") {
					t.Fatalf("no debía llamarse a CreateProject, llamada registrada: %s", call)
				}
			}
		})
	}
}

// TestEditPanelPickerSelectMovesFocusToNextField comprueba que en el panel de edición
// tras seleccionar un proyecto en el selector, el foco avance al campo siguiente (Descripción),
// tanto con teclado como con ratón, y que la secuencia de edición guarde sin crear proyecto.
func TestEditPanelPickerSelectMovesFocusToNextField(t *testing.T) {
	for _, width := range []int{100, 180} {
		t.Run(testModeName(width)+"_teclado", func(t *testing.T) {
			cat := newFakeCatalog()
			db := &testStore{
				recent: []tracking.Entry{
					{ID: 10, TaskUID: "t10", Title: "Tarea en edicion", Project: "Depiloto", Description: "Desc original"},
				},
				projects: []tracking.ProjectUsage{{Name: "Depiloto"}, {Name: "Lumirecon"}},
			}
			m := NewModel(db, WithCatalog(cat))
			next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 35})
			m = next.(Model)

			// Abrir panel de edición.
			m.openEdit(tracking.TaskSummary{
				TaskUID:     "t10",
				LastEntryID: 10,
				Title:       "Tarea en edicion",
				Project:     "Depiloto",
				Description: "Desc original",
			})
			if m.edit == nil {
				t.Fatal("el panel de edición debía estar abierto")
			}

			// Abrir selector en Proyecto y seleccionar con Enter.
			m.focus = focusProject
			m.openProjectPicker()
			next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = next.(Model)

			if m.pickerOpen {
				t.Fatal("el selector debía cerrarse")
			}
			// El foco debe avanzar al campo siguiente después de Proyecto (Descripción).
			if m.focus != focusDescription {
				t.Fatalf("en panel de edición el foco debía ser focusDescription, obtuvo %v", m.focus)
			}

			// Escribir texto adicional en descripción.
			m = typeText(t, m, " actualizada")
			if !strings.Contains(m.inputs[2].Value(), "actualizada") {
				t.Fatalf("el campo Descripción debía contener 'actualizada', tiene %q", m.inputs[2].Value())
			}

			// Presionar Enter para guardar los cambios.
			next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = next.(Model)

			// Verificar que la tarea se guardó y no se creó ningún proyecto.
			if len(db.edits) != 1 {
				t.Fatalf("se esperaba 1 edición guardada, hubo %d", len(db.edits))
			}
			if db.edits[0].input.Description == nil || !strings.Contains(*db.edits[0].input.Description, "actualizada") {
				t.Fatalf("descripción guardada incorrecta: %#v", db.edits[0].input)
			}
			for _, call := range cat.calls {
				if strings.HasPrefix(call, "CreateProject") {
					t.Fatalf("no debía llamarse a CreateProject, llamada registrada: %s", call)
				}
			}
		})

		t.Run(testModeName(width)+"_raton", func(t *testing.T) {
			cat := newFakeCatalog()
			db := &testStore{
				recent: []tracking.Entry{
					{ID: 10, TaskUID: "t10", Title: "Tarea en edicion", Project: "Depiloto", Description: "Desc original"},
				},
				projects: []tracking.ProjectUsage{{Name: "Depiloto"}, {Name: "Lumirecon"}},
			}
			m := NewModel(db, WithCatalog(cat))
			next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 35})
			m = next.(Model)

			m.openEdit(tracking.TaskSummary{
				TaskUID:     "t10",
				LastEntryID: 10,
				Title:       "Tarea en edicion",
				Project:     "Depiloto",
				Description: "Desc original",
			})

			m.focus = focusProject
			m.openProjectPicker()

			optionX, optionY := pickerOptionCoord(t, m, width, 0)
			next, _ = m.Update(tea.MouseMsg{X: optionX, Y: optionY, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
			m = next.(Model)

			if m.pickerOpen {
				t.Fatal("el selector debía cerrarse al hacer clic")
			}
			if m.focus != focusDescription {
				t.Fatalf("foco = %v, quería focusDescription", m.focus)
			}
		})
	}
}

// TestPickerSelectCreateOptionMovesFocus comprueba que al seleccionar la opción
// de creación ("Crear «x» en <cliente>") también se mueva el foco adecuadamente.
func TestPickerSelectCreateOptionMovesFocus(t *testing.T) {
	for _, width := range []int{100, 180} {
		t.Run(testModeName(width), func(t *testing.T) {
			cat := newFakeCatalog()
			db := &testStore{}
			m := NewModel(db, WithCatalog(cat))
			next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 35})
			m = next.(Model)

			m.focus = focusProject
			m.openProjectPicker()
			m.inputs[1].SetValue("NuevoProyecto")

			// Seleccionar la última opción (opción de creación).
			options := m.projectOptions()
			m.pickerIndex = len(options) - 1
			if !options[m.pickerIndex].create {
				t.Fatalf("la opción seleccionada debía ser create: %#v", options[m.pickerIndex])
			}

			next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = next.(Model)

			if m.pickerOpen {
				t.Fatal("el selector de proyectos debía cerrarse")
			}
			if m.focus != focusDescription {
				t.Fatalf("tras crear proyecto con descripción vacía el foco debía ser focusDescription, obtuvo %v", m.focus)
			}
		})
	}
}

func testModeName(width int) string {
	if width >= 120 {
		return "wide_180"
	}
	return "narrow_100"
}

func pickerOptionCoord(t *testing.T, m Model, width, optionIndex int) (int, int) {
	t.Helper()
	if width >= 120 {
		_, layout, ok := m.wideMouseLayout()
		if !ok || len(layout.options) <= optionIndex {
			t.Fatalf("no se encontraron opciones en layout ancho (total %d)", len(layout.options))
		}
		return layout.options[optionIndex].x, layout.options[optionIndex].y
	}
	layout := m.computeLayout()
	if len(layout.options) <= optionIndex {
		t.Fatalf("no se encontraron opciones en layout estrecho (total %d)", len(layout.options))
	}
	return layout.options[optionIndex].x, layout.options[optionIndex].y
}
