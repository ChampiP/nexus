package tui

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"nexus/internal/catalog"
	"nexus/internal/tracking"
)

// fakeCatalog es un catálogo en memoria que registra las llamadas de mutación.
type fakeCatalog struct {
	orgs     []catalog.Organization
	clients  []catalog.Client
	projects []catalog.Project
	next     int64
	calls    []string
}

func newFakeCatalog() *fakeCatalog {
	f := &fakeCatalog{next: 100}
	f.orgs = []catalog.Organization{{ID: 1, Name: "Holinsys"}}
	f.clients = []catalog.Client{{ID: 1, Name: "Depilab", OrganizationID: i64(1)}}
	f.projects = []catalog.Project{
		{ID: 1, Name: "Depiloto", ClientID: i64(1)},
		{ID: 2, Name: "Lumirecon", ClientID: i64(1)},
		{ID: 3, Name: "web suelto"},
	}
	return f
}

func (f *fakeCatalog) Tree() ([]catalog.TreeOrganization, error) {
	var tree []catalog.TreeOrganization
	orgPos := map[int64]int{}
	for i := range f.orgs {
		o := f.orgs[i]
		orgPos[o.ID] = len(tree)
		tree = append(tree, catalog.TreeOrganization{Organization: &o})
	}
	loose := catalog.TreeOrganization{}
	clientPos := map[int64][2]int{}
	for i := range f.clients {
		c := f.clients[i]
		group := &loose
		pos := [2]int{-1, 0}
		if c.OrganizationID != nil {
			pos[0] = orgPos[*c.OrganizationID]
			group = &tree[pos[0]]
		}
		pos[1] = len(group.Clients)
		clientPos[c.ID] = pos
		group.Clients = append(group.Clients, catalog.TreeClient{Client: &c})
	}
	var noClient []catalog.Project
	for _, p := range f.projects {
		if p.ClientID == nil {
			noClient = append(noClient, p)
			continue
		}
		pos := clientPos[*p.ClientID]
		group := &loose
		if pos[0] >= 0 {
			group = &tree[pos[0]]
		}
		group.Clients[pos[1]].Projects = append(group.Clients[pos[1]].Projects, p)
	}
	if len(noClient) > 0 {
		loose.Clients = append(loose.Clients, catalog.TreeClient{Projects: noClient})
	}
	if len(loose.Clients) > 0 {
		tree = append(tree, loose)
	}
	return tree, nil
}

func (f *fakeCatalog) id() int64 { f.next++; return f.next }
func (f *fakeCatalog) log(format string, args ...any) {
	f.calls = append(f.calls, fmt.Sprintf(format, args...))
}
func (f *fakeCatalog) taken(name string) bool {
	for _, p := range f.projects {
		if strings.EqualFold(p.Name, name) {
			return true
		}
	}
	return false
}
func (f *fakeCatalog) CreateOrganization(name string) (catalog.Organization, error) {
	f.log("CreateOrganization %s", name)
	o := catalog.Organization{ID: f.id(), Name: name}
	f.orgs = append(f.orgs, o)
	sort.Slice(f.orgs, func(i, j int) bool { return f.orgs[i].Name < f.orgs[j].Name })
	return o, nil
}
func (f *fakeCatalog) RenameOrganization(id int64, name string) error {
	f.log("RenameOrganization %d %s", id, name)
	return nil
}
func (f *fakeCatalog) DeleteOrganization(id int64) error {
	f.log("DeleteOrganization %d", id)
	return nil
}
func (f *fakeCatalog) CreateClient(name string, orgID int64) (catalog.Client, error) {
	f.log("CreateClient %s %d", name, orgID)
	c := catalog.Client{ID: f.id(), Name: name}
	if orgID != 0 {
		c.OrganizationID = &orgID
	}
	f.clients = append(f.clients, c)
	return c, nil
}
func (f *fakeCatalog) RenameClient(id int64, name string) error {
	f.log("RenameClient %d %s", id, name)
	return nil
}
func (f *fakeCatalog) MoveClient(clientID, orgID int64) error {
	f.log("MoveClient %d %d", clientID, orgID)
	return nil
}
func (f *fakeCatalog) DeleteClient(id int64) error { f.log("DeleteClient %d", id); return nil }
func (f *fakeCatalog) CreateProject(name string, clientID int64) (catalog.Project, error) {
	f.log("CreateProject %s %d", name, clientID)
	p := catalog.Project{ID: f.id(), Name: name}
	if clientID != 0 {
		p.ClientID = &clientID
	}
	f.projects = append(f.projects, p)
	return p, nil
}
func (f *fakeCatalog) RenameProject(id int64, name string) error {
	f.log("RenameProject %d %s", id, name)
	if f.taken(name) {
		return catalog.ErrDuplicateName
	}
	for i := range f.projects {
		if f.projects[i].ID == id {
			f.projects[i].Name = name
		}
	}
	return nil
}
func (f *fakeCatalog) MoveProject(projectID, clientID int64) error {
	f.log("MoveProject %d %d", projectID, clientID)
	for i := range f.projects {
		if f.projects[i].ID == projectID {
			f.projects[i].ClientID = nil
			if clientID != 0 {
				f.projects[i].ClientID = &clientID
			}
		}
	}
	return nil
}
func (f *fakeCatalog) setArchived(id int64, at *int64) {
	for i := range f.projects {
		if f.projects[i].ID == id {
			f.projects[i].ArchivedAt = at
		}
	}
}
func (f *fakeCatalog) ArchiveProject(id int64) error {
	f.log("ArchiveProject %d", id)
	f.setArchived(id, i64(1))
	return nil
}
func (f *fakeCatalog) UnarchiveProject(id int64) error {
	f.log("UnarchiveProject %d", id)
	f.setArchived(id, nil)
	return nil
}
func (f *fakeCatalog) DeleteProject(id int64) error { f.log("DeleteProject %d", id); return nil }
func (f *fakeCatalog) MergeProjects(from, into int64) error {
	f.log("MergeProjects %d %d", from, into)
	return nil
}

func newCatalogModel(t *testing.T) (Model, *fakeCatalog, *testStore) {
	t.Helper()
	store := &testStore{counts: map[int64]int{1: 3, 3: 12}, totals: []tracking.ProjectTotal{{Project: "Depiloto", Seconds: 4800}}}
	fake := newFakeCatalog()
	m := NewModel(store, WithCatalog(fake))
	m = updateModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	return m, fake, store
}

func updateModel(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(Model)
}

func typeText(t *testing.T, m Model, text string) Model {
	t.Helper()
	return updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)})
}

// openCatalog cambia a la pantalla de catálogo desde la barra de pestañas.
func openCatalog(t *testing.T, m Model) Model {
	t.Helper()
	m = goToTabs(t, m)
	return press(t, m, tea.KeyEnter)
}

func goToTabs(t *testing.T, m Model) Model {
	t.Helper()
	return press(t, m, tea.KeyUp)
}

// focusCatalogRow lleva el foco a la fila con ese nombre usando solo ↓.
func focusCatalogRow(t *testing.T, m Model, name string) Model {
	t.Helper()
	for m.cat.focus != catFocusTabs {
		m = press(t, m, tea.KeyUp)
	}
	for i := 0; i < 40; i++ {
		if m.cat.focus == catFocusRow && m.cat.rows[m.cat.row].name == name {
			return m
		}
		m = press(t, m, tea.KeyDown)
	}
	t.Fatalf("no se encontró la fila %q", name)
	return m
}

func clickAt(t *testing.T, m Model, r rect) Model {
	t.Helper()
	return updateModel(t, m, tea.MouseMsg{X: r.x, Y: r.y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
}

func TestTabsAreFirstFocusStopAndSwitchScreens(t *testing.T) {
	m, _, _ := newCatalogModel(t)
	grid := m.timersGrid()
	if grid[0].cells[0].kind != int(focusTabs) {
		t.Fatalf("la primera fila debe ser la barra de pestañas: %+v", grid[0])
	}
	m = goToTabs(t, m)
	if m.focus != focusTabs {
		t.Fatalf("focus = %v", m.focus)
	}
	m = press(t, m, tea.KeyRight)
	if m.screen != screenCatalog || m.cat.focus != catFocusTabs {
		t.Fatalf("→ debía abrir el catálogo con el foco en las pestañas: %v %v", m.screen, m.cat.focus)
	}
	m = press(t, m, tea.KeyLeft)
	if m.screen != screenTimers || m.focus != focusTabs {
		t.Fatalf("← debía volver a temporizadores: %v %v", m.screen, m.focus)
	}
	m = press(t, m, tea.KeyEnter)
	if m.screen != screenCatalog {
		t.Fatal("Enter debía cambiar de pantalla")
	}
	m = press(t, m, tea.KeyEnter)
	if m.screen != screenTimers {
		t.Fatal("Enter en la pestaña activa debía alternar")
	}
}

func TestTimersScreenWithoutCatalogHasNoTabs(t *testing.T) {
	m := NewModel(&testStore{})
	for _, row := range m.timersGrid() {
		if row.cells[0].kind == int(focusTabs) {
			t.Fatal("sin catálogo no debe haber pestañas")
		}
	}
}

func TestCatalogTreeRowsAndFocusSkipsHeaders(t *testing.T) {
	m, _, _ := newCatalogModel(t)
	m = openCatalog(t, m)
	var got []string
	for _, r := range m.cat.rows {
		got = append(got, string(rune('0'+r.depth))+":"+r.name)
	}
	if strings.Join(got, ",") != "0:Holinsys,1:Depilab,2:Depiloto,2:Lumirecon,0:Sin organización,1:Sin cliente,2:web suelto" {
		t.Fatalf("filas = %v", got)
	}
	view := m.View()
	for _, text := range []string{"Holinsys", "Sin organización", "Sin cliente", "1:20:00 esta semana · 3 tareas", "web suelto", "[+ Organización]", "[Mostrar archivados]"} {
		if !strings.Contains(view, text) {
			t.Errorf("la vista no contiene %q:\n%s", text, view)
		}
	}
	m = press(t, m, tea.KeyDown, tea.KeyDown, tea.KeyDown, tea.KeyDown, tea.KeyDown)
	if m.cat.rows[m.cat.row].name != "web suelto" {
		t.Fatalf("↓ debía saltar los encabezados; foco en %q", m.cat.rows[m.cat.row].name)
	}
}

func TestRowButtonsPerKindAndArrowSelection(t *testing.T) {
	m, _, _ := newCatalogModel(t)
	m = openCatalog(t, m)
	cases := map[string]int{"Holinsys": 2, "Depilab": 3, "Depiloto": 5}
	for name, count := range cases {
		m = focusCatalogRow(t, m, name)
		if got := len(rowLabels(m.cat.rows[m.cat.row])); got != count {
			t.Fatalf("%s: %d botones; quiero %d", name, got, count)
		}
	}
	m = focusCatalogRow(t, m, "Depiloto")
	for i := 0; i < 8; i++ {
		m = press(t, m, tea.KeyRight)
	}
	if m.cat.button != 4 {
		t.Fatalf("→ debía detenerse en el último botón: %d", m.cat.button)
	}
	m = press(t, m, tea.KeyLeft)
	if m.cat.button != 3 {
		t.Fatalf("← = %d", m.cat.button)
	}
}

func TestRenameSavesAndEscCancels(t *testing.T) {
	m, fake, _ := newCatalogModel(t)
	m = openCatalog(t, m)
	m = focusCatalogRow(t, m, "Lumirecon")
	m = press(t, m, tea.KeyEnter) // Renombrar
	if m.cat.edit == nil || m.cat.input.Value() != "Lumirecon" {
		t.Fatalf("el campo debía abrirse precargado: %+v %q", m.cat.edit, m.cat.input.Value())
	}
	if !strings.Contains(m.View(), "Renombrar: ") {
		t.Fatal("el campo debía mostrarse en la misma fila")
	}
	m = typeText(t, m, "X")
	m = press(t, m, tea.KeyEsc)
	if m.cat.edit != nil || len(fake.calls) != 0 || m.screen != screenCatalog {
		t.Fatalf("Esc debía cancelar sin cambios: calls=%v", fake.calls)
	}
	m = press(t, m, tea.KeyEnter)
	m = typeText(t, m, "2")
	m = press(t, m, tea.KeyEnter)
	if len(fake.calls) != 1 || fake.calls[0] != "RenameProject 2 Lumirecon2" || m.cat.edit != nil {
		t.Fatalf("calls = %v", fake.calls)
	}
	if m.cat.rows[m.cat.row].name != "Lumirecon2" {
		t.Fatalf("el foco debía seguir a la fila renombrada: %q", m.cat.rows[m.cat.row].name)
	}
}

func TestDuplicateNameShowsSpanishErrorAndKeepsInput(t *testing.T) {
	m, _, _ := newCatalogModel(t)
	m = openCatalog(t, m)
	m = focusCatalogRow(t, m, "Lumirecon")
	m = press(t, m, tea.KeyEnter)
	for i := 0; i < len("Lumirecon"); i++ {
		m = press(t, m, tea.KeyBackspace)
	}
	m = typeText(t, m, "depiloto")
	m = press(t, m, tea.KeyEnter)
	if m.message != "Ya existe ese nombre; usa Unir para combinar proyectos" {
		t.Fatalf("message = %q", m.message)
	}
	if m.cat.edit == nil {
		t.Fatal("el campo debía seguir abierto para corregir")
	}
}

func TestCreateClientAsksOrganizationWithSearchablePicker(t *testing.T) {
	m, fake, _ := newCatalogModel(t)
	m = openCatalog(t, m)
	for m.cat.focus != catFocusAdd {
		m = press(t, m, tea.KeyDown)
	}
	m = press(t, m, tea.KeyRight, tea.KeyEnter) // + Cliente
	if m.cat.edit == nil || !m.cat.edit.create {
		t.Fatal("+ Cliente debía abrir el campo de nombre")
	}
	m = typeText(t, m, "Nuevo")
	m = press(t, m, tea.KeyEnter)
	if m.cat.picker == nil || m.cat.edit != nil {
		t.Fatal("tras el nombre debía abrirse el selector de organización")
	}
	if got := m.pickerOptions(); len(got) != 2 || got[0].label != "Sin organización" || got[1].label != "Holinsys" {
		t.Fatalf("opciones = %+v", got)
	}
	m = typeText(t, m, "holi")
	if got := m.pickerOptions(); len(got) != 1 || got[0].label != "Holinsys" {
		t.Fatalf("filtrado = %+v", got)
	}
	m = press(t, m, tea.KeyEnter)
	if len(fake.calls) != 1 || fake.calls[0] != "CreateClient Nuevo 1" || m.cat.picker != nil {
		t.Fatalf("calls = %v", fake.calls)
	}
}

func TestCreateProjectWithoutClientAndOrganization(t *testing.T) {
	m, fake, _ := newCatalogModel(t)
	m = openCatalog(t, m)
	for m.cat.focus != catFocusAdd {
		m = press(t, m, tea.KeyDown)
	}
	m = press(t, m, tea.KeyRight, tea.KeyRight, tea.KeyEnter)
	m = typeText(t, m, "Libre")
	m = press(t, m, tea.KeyEnter, tea.KeyEnter) // primera opción: Sin cliente
	if len(fake.calls) != 1 || fake.calls[0] != "CreateProject Libre 0" {
		t.Fatalf("calls = %v", fake.calls)
	}
}

func TestCreateOrganizationInline(t *testing.T) {
	m, fake, _ := newCatalogModel(t)
	m = openCatalog(t, m)
	for m.cat.focus != catFocusAdd {
		m = press(t, m, tea.KeyDown)
	}
	m = press(t, m, tea.KeyEnter)
	m = typeText(t, m, "Acme")
	m = press(t, m, tea.KeyEnter)
	if len(fake.calls) != 1 || fake.calls[0] != "CreateOrganization Acme" {
		t.Fatalf("calls = %v", fake.calls)
	}
}

func TestMoveProjectWithPicker(t *testing.T) {
	m, fake, _ := newCatalogModel(t)
	m = openCatalog(t, m)
	m = focusCatalogRow(t, m, "web suelto")
	m = press(t, m, tea.KeyRight, tea.KeyEnter) // Mover
	if m.cat.picker == nil {
		t.Fatal("Mover debía abrir el selector")
	}
	if got := m.pickerOptions(); got[0].label != "Ninguno" || got[1].label != "Depilab · Holinsys" {
		t.Fatalf("opciones = %+v", got)
	}
	m = typeText(t, m, "depi")
	m = press(t, m, tea.KeyEnter)
	if len(fake.calls) != 1 || fake.calls[0] != "MoveProject 3 1" {
		t.Fatalf("calls = %v", fake.calls)
	}
	var names []string
	for _, r := range m.cat.rows {
		names = append(names, r.name)
	}
	if strings.Contains(strings.Join(names, ","), "Sin cliente") {
		t.Fatalf("el proyecto movido ya no debía estar sin cliente: %v", names)
	}
}

func TestMoveClientOffersOrganizationsAndNone(t *testing.T) {
	m, fake, _ := newCatalogModel(t)
	m = openCatalog(t, m)
	m = focusCatalogRow(t, m, "Depilab")
	m = press(t, m, tea.KeyRight, tea.KeyEnter)
	if got := m.pickerOptions(); len(got) != 2 || got[0].label != "Ninguno" || got[1].label != "Holinsys" {
		t.Fatalf("opciones = %+v", got)
	}
	m = press(t, m, tea.KeyEnter)
	if len(fake.calls) != 1 || fake.calls[0] != "MoveClient 1 0" {
		t.Fatalf("calls = %v", fake.calls)
	}
}

func TestMergeAsksConfirmationWithCancelDefault(t *testing.T) {
	m, fake, _ := newCatalogModel(t)
	m = openCatalog(t, m)
	m = focusCatalogRow(t, m, "web suelto")
	m = press(t, m, tea.KeyRight, tea.KeyRight, tea.KeyEnter) // Unir
	for _, o := range m.pickerOptions() {
		if o.label == "web suelto" {
			t.Fatal("el selector de unir no debe ofrecer el propio proyecto")
		}
	}
	m = typeText(t, m, "depiloto")
	m = press(t, m, tea.KeyEnter)
	if m.cat.confirm == nil || m.cat.confirm.text != "¿Unir «web suelto» en «Depiloto»? 12 tareas pasarán a «Depiloto»." || m.cat.confirm.button != 1 {
		t.Fatalf("confirmación = %+v", m.cat.confirm)
	}
	if !strings.Contains(m.View(), "[Unir]") || !strings.Contains(m.View(), "12 tareas pasarán") {
		t.Fatal("la confirmación debía verse en pantalla")
	}
	m = press(t, m, tea.KeyEnter) // Cancelar por defecto
	if m.cat.confirm != nil || len(fake.calls) != 0 {
		t.Fatalf("Enter sobre Cancelar no debía cambiar nada: %v", fake.calls)
	}
	m = press(t, m, tea.KeyEnter)
	m = typeText(t, m, "depiloto")
	m = press(t, m, tea.KeyEnter, tea.KeyLeft, tea.KeyEnter)
	if len(fake.calls) != 1 || fake.calls[0] != "MergeProjects 3 1" {
		t.Fatalf("calls = %v", fake.calls)
	}
}

func TestDeleteConfirmationEscCancelsAndConfirmDeletes(t *testing.T) {
	m, fake, _ := newCatalogModel(t)
	m = openCatalog(t, m)
	m = focusCatalogRow(t, m, "web suelto")
	m = press(t, m, tea.KeyRight, tea.KeyRight, tea.KeyRight, tea.KeyRight, tea.KeyEnter)
	if m.cat.confirm == nil || m.cat.confirm.text != "¿Eliminar el proyecto «web suelto»? 12 tareas quedarán sin proyecto." || m.cat.confirm.button != 1 {
		t.Fatalf("confirmación = %+v", m.cat.confirm)
	}
	m = press(t, m, tea.KeyEsc)
	if m.cat.confirm != nil || len(fake.calls) != 0 || m.screen != screenCatalog {
		t.Fatalf("Esc debía cancelar: %v", fake.calls)
	}
	m = press(t, m, tea.KeyEnter, tea.KeyLeft, tea.KeyEnter)
	if len(fake.calls) != 1 || fake.calls[0] != "DeleteProject 3" {
		t.Fatalf("calls = %v", fake.calls)
	}
}

func TestDeleteOrganizationMessageMentionsClients(t *testing.T) {
	m, _, _ := newCatalogModel(t)
	m = openCatalog(t, m)
	m = focusCatalogRow(t, m, "Holinsys")
	m = press(t, m, tea.KeyRight, tea.KeyEnter)
	if m.cat.confirm == nil || m.cat.confirm.text != "¿Eliminar la organización «Holinsys»? Su cliente quedará sin organización." {
		t.Fatalf("confirmación = %+v", m.cat.confirm)
	}
}

func TestArchiveToggleAndShowArchived(t *testing.T) {
	m, fake, _ := newCatalogModel(t)
	m = openCatalog(t, m)
	m = focusCatalogRow(t, m, "Lumirecon")
	m = press(t, m, tea.KeyRight, tea.KeyRight, tea.KeyRight, tea.KeyEnter) // Archivar
	if len(fake.calls) != 1 || fake.calls[0] != "ArchiveProject 2" {
		t.Fatalf("calls = %v", fake.calls)
	}
	for _, r := range m.cat.rows {
		if r.name == "Lumirecon" {
			t.Fatal("el proyecto archivado debía ocultarse")
		}
	}
	for m.cat.focus != catFocusToggle {
		m = press(t, m, tea.KeyDown)
	}
	m = press(t, m, tea.KeyEnter)
	if !m.cat.showArchived || !strings.Contains(m.View(), "[Ocultar archivados]") || !strings.Contains(m.View(), "Lumirecon (archivado)") {
		t.Fatalf("el interruptor debía mostrar los archivados:\n%s", m.View())
	}
	m = focusCatalogRow(t, m, "Lumirecon")
	if labels := rowLabels(m.cat.rows[m.cat.row]); labels[3] != "▣ Desarchivar" {
		t.Fatalf("labels = %v", labels)
	}
}

func TestCatalogClicksUseSharedLayout(t *testing.T) {
	m, fake, _ := newCatalogModel(t)
	m = openCatalog(t, m)
	l := m.catalogLayout()
	// Clic en una fila la enfoca y muestra sus botones.
	m = clickAt(t, m, l.rows[2].row)
	if m.cat.focus != catFocusRow || m.cat.rows[m.cat.row].name != "Depiloto" {
		t.Fatalf("foco = %v %d", m.cat.focus, m.cat.row)
	}
	l = m.catalogLayout()
	if len(l.rows[2].buttons) != 5 {
		t.Fatalf("botones de la fila enfocada = %d", len(l.rows[2].buttons))
	}
	// Clic en [✕ Eliminar] abre la confirmación sin borrar.
	m = clickAt(t, m, l.rows[2].buttons[4])
	if m.cat.confirm == nil || len(fake.calls) != 0 {
		t.Fatal("el clic en Eliminar debía pedir confirmación")
	}
	l = m.catalogLayout()
	m = clickAt(t, m, l.confirmButtons[1]) // Cancelar
	if m.cat.confirm != nil || len(fake.calls) != 0 {
		t.Fatal("Cancelar no debía cambiar nada")
	}
	// Fila de acciones: + Organización.
	l = m.catalogLayout()
	m = clickAt(t, m, l.adds[0])
	if m.cat.edit == nil || !m.cat.edit.create || m.cat.edit.kind != catOrg {
		t.Fatal("el clic en + Organización debía abrir el campo")
	}
	m = press(t, m, tea.KeyEsc)
	// Interruptor de archivados.
	l = m.catalogLayout()
	m = clickAt(t, m, l.toggle)
	if !m.cat.showArchived {
		t.Fatal("el clic debía alternar los archivados")
	}
	// Pestañas.
	m = clickAt(t, m, m.catalogLayout().tabs[0])
	if m.screen != screenTimers {
		t.Fatal("el clic en Temporizadores debía cambiar de pantalla")
	}
	m = clickAt(t, m, m.computeLayout().tabs[1])
	if m.screen != screenCatalog {
		t.Fatal("el clic en Catálogo debía cambiar de pantalla")
	}
}

func TestCatalogPickerOptionClick(t *testing.T) {
	m, fake, _ := newCatalogModel(t)
	m = openCatalog(t, m)
	m = focusCatalogRow(t, m, "web suelto")
	m = press(t, m, tea.KeyRight, tea.KeyEnter)
	l := m.catalogLayout()
	m = clickAt(t, m, l.options[1])
	if len(fake.calls) != 1 || fake.calls[0] != "MoveProject 3 1" {
		t.Fatalf("calls = %v", fake.calls)
	}
}

func TestCatalogScrollKeepsFocusedRowVisibleAndWheelScrolls(t *testing.T) {
	m, fake, _ := newCatalogModel(t)
	for i := 0; i < 30; i++ {
		fake.projects = append(fake.projects, catalog.Project{ID: int64(10 + i), Name: fmt.Sprintf("p%02d", i)})
	}
	m = updateModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 20})
	m = openCatalog(t, m)
	m.loadCatalog()
	l := m.catalogLayout()
	if l.visible >= len(m.cat.rows) {
		t.Fatalf("el árbol debía exceder la pantalla: %d de %d", l.visible, len(m.cat.rows))
	}
	for m.cat.focus != catFocusAdd {
		m = press(t, m, tea.KeyDown)
		l = m.catalogLayout()
		if m.cat.focus == catFocusRow && (m.cat.row < l.first || m.cat.row >= l.first+l.visible) {
			t.Fatalf("la fila enfocada %d salió de la ventana [%d,%d)", m.cat.row, l.first, l.first+l.visible)
		}
	}
	m.cat.scroll = 0
	m = updateModel(t, m, tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	if m.cat.scroll != 1 {
		t.Fatalf("la rueda debía desplazar el árbol: %d", m.cat.scroll)
	}
	m = updateModel(t, m, tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp})
	if m.cat.scroll != 0 {
		t.Fatalf("scroll = %d", m.cat.scroll)
	}
}

func TestTimerPickerListsCatalogOnlyProjectsWithClient(t *testing.T) {
	store := &testStore{projects: []tracking.ProjectUsage{{Name: "Depiloto", LastUsed: 50, Seconds: 4800}}}
	m := NewModel(store, WithCatalog(newFakeCatalog()))
	m = updateModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m = press(t, m, tea.KeyDown) // título → proyecto
	m = press(t, m, tea.KeyEnter)
	view := m.View()
	for _, text := range []string{"Depiloto  ·  1:20:00", "Lumirecon  ·  0:00:00"} {
		if !strings.Contains(view, text) {
			t.Errorf("el selector no contiene %q:\n%s", text, view)
		}
	}
	m = typeText(t, m, "depilab")
	options := m.projectOptions()
	var found []string
	for _, option := range options {
		if option.selectable && !option.create {
			found = append(found, option.name)
		}
	}
	if len(found) != 2 || found[0] != "Depiloto" || found[1] != "Lumirecon" {
		t.Fatalf("la búsqueda por cliente falló: %+v", options)
	}
	m.inputs[1].SetValue("zzz nuevo")
	options = m.projectOptions()
	if last := options[len(options)-1]; !last.create || projectOptionLabel(last) != "Crear «zzz nuevo» sin cliente" {
		t.Fatalf("debía ofrecer crear: %+v", options)
	}
}

func TestEditPickerAlsoUsesCatalog(t *testing.T) {
	store := newRowsStore()
	m := NewModel(store, WithCatalog(newFakeCatalog()))
	found := false
	for _, o := range m.projectOptions() {
		if o.name == "Lumirecon" && o.client == "Depilab" {
			found = true
		}
	}
	if !found {
		t.Fatal("el selector debía incluir el proyecto del catálogo sin tareas")
	}
}

func TestPickerShowsPathGroupsSearchAndSkipsHeaders(t *testing.T) {
	m := NewModel(&testStore{projects: []tracking.ProjectUsage{{Name: "Depiloto"}}}, WithCatalog(newFakeCatalog()))
	m.selectedProject = "Depiloto"
	m.width = 100
	if !strings.Contains(m.View(), "‹ Holinsys › Depilab › Depiloto ›") {
		t.Fatalf("falta la ruta completa del proyecto:\n%s", m.View())
	}
	m.focus, m.pickerOpen = focusProject, true
	m.inputs[1].SetValue("holinsys")
	options := m.projectOptions()
	foundOrg, foundClient, foundProject := false, false, false
	for _, option := range options {
		foundOrg = foundOrg || option.header && option.name == "Holinsys"
		foundClient = foundClient || option.header && option.name == "Depilab"
		foundProject = foundProject || option.name == "Depiloto" && option.selectable
	}
	if !foundOrg || !foundClient || !foundProject {
		t.Fatalf("la búsqueda por organización debe conservar encabezados y proyecto: %+v", options)
	}
	m.pickerIndex = 1
	m.movePicker(1)
	if !options[m.pickerIndex].selectable {
		t.Fatalf("↓ enfocó un encabezado: índice=%d opción=%+v", m.pickerIndex, options[m.pickerIndex])
	}
}

func TestPickerCreateUnderClientOrWithoutClientAndMouseHeaders(t *testing.T) {
	for _, tc := range []struct {
		name       string
		choose     int
		wantCall   string
		wantClient string
	}{
		{name: "cliente", choose: 0, wantCall: "CreateProject Nueva 1", wantClient: "Depilab"},
		{name: "sin cliente", choose: 1, wantCall: "CreateProject Nueva 0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeCatalog()
			m := NewModel(&testStore{}, WithCatalog(fake))
			m.focus, m.pickerOpen = focusProject, true
			m.inputs[1].SetValue("Nueva")
			options := m.projectOptions()
			create := make([]projectOption, 0)
			for _, option := range options {
				if option.create {
					create = append(create, option)
				}
			}
			wantLabel := "Crear «Nueva» en Depilab"
			if tc.choose == 1 {
				wantLabel = "Crear «Nueva» sin cliente"
			}
			if len(create) != 2 || projectOptionLabel(create[tc.choose]) != wantLabel {
				t.Fatalf("opciones de creación = %+v", create)
			}
			m.pickerIndex = indexOfOption(options, create[tc.choose])
			m = press(t, m, tea.KeyEnter)
			if m.selectedProject != "Nueva" || len(fake.calls) != 1 || fake.calls[0] != tc.wantCall {
				t.Fatalf("proyecto=%q llamadas=%v", m.selectedProject, fake.calls)
			}
			if tc.wantClient != "" && create[tc.choose].client != tc.wantClient {
				t.Fatalf("cliente = %q", create[tc.choose].client)
			}
		})
	}
}

func indexOfOption(options []projectOption, target projectOption) int {
	for i, option := range options {
		if option.create && option.clientID == target.clientID {
			return i
		}
	}
	return -1
}

func TestPickerMouseIgnoresHeadersAndSelectsProject(t *testing.T) {
	m := NewModel(&testStore{}, WithCatalog(newFakeCatalog()))
	m.focus, m.pickerOpen = focusProject, true
	m.width, m.height = 100, 40
	options := m.projectOptions()
	layout := m.computeLayout()
	header := -1
	project := -1
	for i, option := range options {
		if option.header && header < 0 {
			header = i
		}
		if option.selectable && option.name == "Depiloto" {
			project = i
		}
	}
	before := m.pickerIndex
	m = clickAt(t, m, layout.options[header])
	if !m.pickerOpen {
		t.Fatal("clic en encabezado no debe cerrar la lista")
	}
	// Un encabezado no es una opción: el clic no debe mover el resaltado.
	if m.pickerIndex != before {
		t.Fatalf("clic en encabezado movió el resaltado de %d a %d", before, m.pickerIndex)
	}
	layout = m.computeLayout()
	m.pickerIndex = project
	m = clickAt(t, m, layout.options[project])
	if m.pickerOpen || m.selectedProject != "Depiloto" {
		t.Fatalf("clic en proyecto: abierto=%v proyecto=%q", m.pickerOpen, m.selectedProject)
	}
}
