package tui

import (
	"strings"
	"testing"
	"time"

	"nexus/internal/catalog"
	"nexus/internal/tracking"
)

func TestProjectsUniquePerClient(t *testing.T) {
	// Fixture con "Diseno de web" bajo los clientes A y B.
	cat := &fakeCatalog{
		clients: []catalog.Client{
			{ID: 10, Name: "A"},
			{ID: 20, Name: "B"},
		},
		projects: []catalog.Project{
			{ID: 101, Name: "Diseno de web", ClientID: i64(10)},
			{ID: 102, Name: "Diseno de web", ClientID: i64(20)},
		},
	}
	db := &testStore{
		projects: []tracking.ProjectUsage{
			{ProjectID: 101, Name: "Diseno de web", Seconds: 100},
			{ProjectID: 102, Name: "Diseno de web", Seconds: 200},
		},
		totals: []tracking.ProjectTotal{
			{ProjectID: 101, Project: "Diseno de web", Seconds: 100},
			{ProjectID: 102, Project: "Diseno de web", Seconds: 200},
		},
		recent: []tracking.Entry{
			{ID: 1, TaskUID: "task-1", Title: "Tarea previa", Project: "Diseno de web", ProjectID: 101, StartedAt: time.Now().Add(-time.Hour).Unix(), EndedAt: i64(time.Now().Unix())},
		},
	}

	m := NewModel(db, WithCatalog(cat))

	// 1. Selector muestra ambos con su cliente
	m.openProjectPicker()
	opts := m.projectOptions()
	var foundA, foundB bool
	for _, opt := range opts {
		if opt.name == "Diseno de web" && opt.client == "A" {
			foundA = true
		}
		if opt.name == "Diseno de web" && opt.client == "B" {
			foundB = true
		}
	}
	if !foundA || !foundB {
		t.Fatalf("el selector debe mostrar ambas opciones con sus clientes: foundA=%v, foundB=%v, opts=%#v", foundA, foundB, opts)
	}

	// 2. Seleccionar el de B inicia un temporizador con el ProjectID de B (assert StartInput)
	pickerIdx := -1
	for i, opt := range opts {
		if opt.name == "Diseno de web" && opt.client == "B" {
			pickerIdx = i
			break
		}
	}
	if pickerIdx == -1 {
		t.Fatalf("no se encontró la opción de B en el selector")
	}
	m.pickerIndex = pickerIdx
	m.selectProject()

	m.inputs[0].SetValue("Nueva tarea")
	m.startTimer()

	if len(db.startInputs) == 0 {
		t.Fatalf("se esperaba que se llamara a Start")
	}
	lastStart := db.startInputs[len(db.startInputs)-1]
	if lastStart.ProjectID != 102 {
		t.Fatalf("StartInput.ProjectID = %d, quería 102 (cliente B)", lastStart.ProjectID)
	}

	// 3. La ruta en línea muestra el cliente de B
	inlinePath := m.selectedProjectPath()
	if !strings.Contains(inlinePath, "B") {
		t.Fatalf("la ruta en línea debía mostrar el cliente B, obtuvo %q", inlinePath)
	}
	if strings.Contains(inlinePath, "A") {
		t.Fatalf("la ruta en línea no debe mostrar el cliente A, obtuvo %q", inlinePath)
	}

	// 4. Editar una tarea del proyecto de A al de B guarda el id de B
	recentTasks, err := db.RecentTasks(10)
	if err != nil || len(recentTasks) == 0 {
		t.Fatalf("no se encontraron tareas recientes: %v", err)
	}
	m.openEdit(recentTasks[0])
	m.openProjectPicker()
	optsEdit := m.projectOptions()
	pickerIdxB := -1
	for i, opt := range optsEdit {
		if opt.name == "Diseno de web" && opt.client == "B" {
			pickerIdxB = i
			break
		}
	}
	if pickerIdxB == -1 {
		t.Fatalf("no se encontró la opción de B en el selector de edición")
	}
	m.pickerIndex = pickerIdxB
	m.selectProject()
	m.saveEdit()

	if len(db.edits) == 0 {
		t.Fatalf("se esperaba llamada a EditTask")
	}
	lastEdit := db.edits[len(db.edits)-1]
	if lastEdit.input.ProjectID == nil || *lastEdit.input.ProjectID != 102 {
		t.Fatalf("EditTask debía guardar ProjectID 102, obtuvo %#v", lastEdit.input.ProjectID)
	}

	// 5. Las barras muestran dos filas separadas con desambiguación "proyecto (cliente)"
	m.width = 100
	rendered := m.dashboardPanel(m.computeLayout())
	if !strings.Contains(rendered, "Diseno de web (A)") || !strings.Contains(rendered, "Diseno de web (B)") {
		t.Fatalf("las barras deben mostrar dos filas separadas con etiquetas 'proyecto (cliente)', obtenido:\n%s", rendered)
	}
}

func TestCycleProjectUsesIDs(t *testing.T) {
	cat := &fakeCatalog{
		clients: []catalog.Client{
			{ID: 10, Name: "A"},
			{ID: 20, Name: "B"},
		},
		projects: []catalog.Project{
			{ID: 101, Name: "Diseno de web", ClientID: i64(10)},
			{ID: 102, Name: "Diseno de web", ClientID: i64(20)},
		},
	}
	db := &testStore{
		projects: []tracking.ProjectUsage{
			{ProjectID: 101, Name: "Diseno de web", Seconds: 100},
			{ProjectID: 102, Name: "Diseno de web", Seconds: 200},
		},
	}
	m := NewModel(db, WithCatalog(cat))

	// Posicionar en el proyecto 101 (cliente A)
	m.selectedProjectID = 101
	m.selectedProject = "Diseno de web"

	// Ciclar hacia adelante (delta = 1)
	m.cycleProject(1)
	if m.selectedProjectID != 102 {
		t.Fatalf("se esperaba avanzar al id 102 (cliente B), obtuvo %d", m.selectedProjectID)
	}
	if !strings.Contains(m.selectedProjectPath(), "B") {
		t.Fatalf("la ruta en línea tras ciclar debía corresponder a B, obtuvo %q", m.selectedProjectPath())
	}

	// Ciclar hacia atrás (delta = -1)
	m.cycleProject(-1)
	if m.selectedProjectID != 101 {
		t.Fatalf("se esperaba retroceder al id 101 (cliente A), obtuvo %d", m.selectedProjectID)
	}
	if !strings.Contains(m.selectedProjectPath(), "A") {
		t.Fatalf("la ruta en línea tras ciclar atrás debía corresponder a A, obtuvo %q", m.selectedProjectPath())
	}
}

func TestCreateProjectUnderClientSelectsNewID(t *testing.T) {
	cat := &fakeCatalog{
		clients: []catalog.Client{
			{ID: 20, Name: "B"},
		},
	}
	db := &testStore{}
	m := NewModel(db, WithCatalog(cat))

	m.openProjectPicker()
	m.inputs[1].SetValue("Nueva Marca")
	opts := m.projectOptions()

	createIdx := -1
	for i, opt := range opts {
		if opt.create && opt.clientID == 20 && opt.name == "Nueva Marca" {
			createIdx = i
			break
		}
	}
	if createIdx == -1 {
		t.Fatalf("no se encontró la opción de crear en cliente B: %#v", opts)
	}

	m.pickerIndex = createIdx
	m.selectProject()

	if m.selectedProject != "Nueva Marca" {
		t.Fatalf("selectedProject = %q, quería 'Nueva Marca'", m.selectedProject)
	}
	if m.selectedProjectID <= 0 {
		t.Fatalf("selectedProjectID debía ser > 0 tras crear, obtuvo %d", m.selectedProjectID)
	}
}

func TestCatalogMoveAndRenameDuplicatePerClient(t *testing.T) {
	cat := &fakeCatalog{
		clients: []catalog.Client{
			{ID: 10, Name: "A"},
			{ID: 20, Name: "B"},
		},
		projects: []catalog.Project{
			{ID: 101, Name: "Diseno de web", ClientID: i64(10)},
			{ID: 102, Name: "Identidad", ClientID: i64(10)},
			{ID: 103, Name: "Diseno de web", ClientID: i64(20)},
		},
	}
	db := &testStore{}
	m := NewModel(db, WithCatalog(cat))

	// 1. Renombrar en el mismo cliente a un nombre ya existente debe fallar con error de duplicado.
	m.cat.edit = &catInput{kind: catProject, row: catRow{id: 102, name: "Identidad", parentID: 10}}
	m.cat.input.SetValue("Diseno de web")
	m.submitCatalogInput()
	if !strings.Contains(m.message, "Ya existe ese nombre") {
		t.Fatalf("renombrar a duplicado en mismo cliente debía mostrar mensaje de duplicado, obtuvo %q", m.message)
	}

	// 2. Renombrar en cliente B a un nombre que existe en cliente A pero no en B debe tener éxito.
	m.cat.edit = &catInput{kind: catProject, row: catRow{id: 103, name: "Diseno de web", parentID: 20}}
	m.cat.input.SetValue("Identidad")
	m.submitCatalogInput()
	if strings.Contains(m.message, "Ya existe ese nombre") {
		t.Fatalf("renombrar con mismo nombre en diferente cliente no debe dar error de duplicado, mensaje: %q", m.message)
	}

	// 3. Mover proyecto 103 de cliente B al cliente A cuando cliente A ya tiene "Diseno de web"
	m.openPicker(&catPicker{
		purpose: pickMoveProject,
		subject: catRow{id: 103, name: "Diseno de web", parentID: 20},
		all:     []pickOption{{id: 10, label: "A"}},
	})
	m.selectPickerIndex(0)
	if !strings.Contains(m.message, "Ya existe ese nombre") {
		t.Fatalf("mover a cliente con nombre colisionante debe mostrar error de duplicado, obtuvo %q", m.message)
	}
}
