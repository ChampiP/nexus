package tui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"nexus/internal/catalog"
)

// screen identifica la pantalla activa; el valor es el índice de su pestaña.
type screen int

const (
	screenTimers screen = iota
	screenCatalog
)

// catRowKind es el tipo de una fila del árbol del catálogo.
type catRowKind int

const (
	catOrg catRowKind = iota
	catClient
	catProject
	catHeader // «Sin organización» y «Sin cliente»: no se editan
)

// catRow es una fila ya aplanada del árbol, con lo necesario para mostrarla y operar sobre ella.
type catRow struct {
	kind     catRowKind
	depth    int
	id       int64
	parentID int64 // organización de un cliente o cliente de un proyecto; 0 si no tiene
	name     string
	archived bool
	seconds  int64 // tiempo registrado en la semana (solo proyectos)
	tasks    int   // tareas del proyecto
	children int   // clientes de una organización o proyectos de un cliente
}

// buildCatalogRows aplana el árbol: organizaciones > clientes > proyectos y, al final,
// «Sin organización» con sus clientes y «Sin cliente» con los proyectos sueltos.
func buildCatalogRows(tree []catalog.TreeOrganization, showArchived bool, stats func(catalog.Project) (int64, int)) []catRow {
	var rows []catRow
	projectRows := func(projects []catalog.Project, depth int, clientID int64) []catRow {
		var out []catRow
		for _, p := range projects {
			if p.ArchivedAt != nil && !showArchived {
				continue
			}
			seconds, tasks := stats(p)
			out = append(out, catRow{kind: catProject, depth: depth, id: p.ID, parentID: clientID, name: p.Name, archived: p.ArchivedAt != nil, seconds: seconds, tasks: tasks})
		}
		return out
	}
	for _, group := range tree {
		var inner []catRow
		var loose []catRow
		clients := 0
		for _, c := range group.Clients {
			if c.Client == nil {
				loose = projectRows(c.Projects, 2, 0)
				continue
			}
			clients++
			orgID := derefID(c.Client.OrganizationID)
			inner = append(inner, catRow{kind: catClient, depth: 1, id: c.Client.ID, parentID: orgID, name: c.Client.Name, children: len(c.Projects)})
			inner = append(inner, projectRows(c.Projects, 2, c.Client.ID)...)
		}
		if group.Organization != nil {
			rows = append(rows, catRow{kind: catOrg, id: group.Organization.ID, name: group.Organization.Name, children: clients})
			rows = append(rows, inner...)
			continue
		}
		if len(inner) == 0 && len(loose) == 0 {
			continue
		}
		rows = append(rows, catRow{kind: catHeader, name: "Sin organización"})
		rows = append(rows, inner...)
		if len(loose) > 0 {
			rows = append(rows, catRow{kind: catHeader, depth: 1, name: "Sin cliente"})
			rows = append(rows, loose...)
		}
	}
	return rows
}

func derefID(id *int64) int64 {
	if id == nil {
		return 0
	}
	return *id
}

// catAction es una acción disponible sobre una fila.
type catAction int

const (
	actRename catAction = iota
	actMove
	actMerge
	actArchive
	actDelete
)

// rowActions devuelve las acciones de la fila; el orden define el de sus botones.
func rowActions(r catRow) []catAction {
	switch r.kind {
	case catOrg:
		return []catAction{actRename, actDelete}
	case catClient:
		return []catAction{actRename, actMove, actDelete}
	case catProject:
		return []catAction{actRename, actMove, actMerge, actArchive, actDelete}
	}
	return nil
}

func actionLabel(a catAction, r catRow) string {
	switch a {
	case actRename:
		return "✎ Renombrar"
	case actMove:
		return "⇄ Mover"
	case actMerge:
		return "⊕ Unir"
	case actArchive:
		if r.archived {
			return "▣ Desarchivar"
		}
		return "▣ Archivar"
	}
	return "✕ Eliminar"
}

// rowLabels son las etiquetas de los botones en línea de la fila.
func rowLabels(r catRow) []string {
	actions := rowActions(r)
	labels := make([]string, len(actions))
	for i, a := range actions {
		labels[i] = actionLabel(a, r)
	}
	return labels
}

// Botones de las filas de acción inferiores; el orden define su índice.
var addLabels = []string{"+ Organización", "+ Cliente", "+ Proyecto"}

func toggleLabel(showArchived bool) string {
	if showArchived {
		return "Ocultar archivados"
	}
	return "Mostrar archivados"
}

// pluralize elige entre singular y plural según n.
func pluralize(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// deleteMessage es la pregunta de confirmación de eliminar, con las consecuencias y sus cantidades.
func deleteMessage(r catRow) string {
	switch r.kind {
	case catProject:
		text := "No tiene tareas."
		switch {
		case r.tasks == 1:
			text = "1 tarea quedará sin proyecto."
		case r.tasks > 1:
			text = fmt.Sprintf("%d tareas quedarán sin proyecto.", r.tasks)
		}
		return "¿Eliminar el proyecto «" + r.name + "»? " + text
	case catClient:
		text := "No tiene proyectos."
		switch {
		case r.children == 1:
			text = "Su proyecto quedará sin cliente."
		case r.children > 1:
			text = fmt.Sprintf("Sus %d proyectos quedarán sin cliente.", r.children)
		}
		return "¿Eliminar el cliente «" + r.name + "»? " + text
	}
	text := "No tiene clientes."
	switch {
	case r.children == 1:
		text = "Su cliente quedará sin organización."
	case r.children > 1:
		text = fmt.Sprintf("Sus %d clientes quedarán sin organización.", r.children)
	}
	return "¿Eliminar la organización «" + r.name + "»? " + text
}

// mergeMessage es la pregunta de confirmación de unir el proyecto from en el destino into.
func mergeMessage(from catRow, into string) string {
	text := "No tiene tareas que mover."
	switch {
	case from.tasks == 1:
		text = "1 tarea pasará a «" + into + "»."
	case from.tasks > 1:
		text = fmt.Sprintf("%d tareas pasarán a «%s».", from.tasks, into)
	}
	return "¿Unir «" + from.name + "» en «" + into + "»? " + text
}

// catalogErrorText traduce los errores del catálogo al español para la línea de estado.
func catalogErrorText(err error) string {
	switch {
	case errors.Is(err, catalog.ErrDuplicateName):
		return "Ya existe ese nombre; usa Unir para combinar proyectos"
	case errors.Is(err, catalog.ErrEmptyName):
		return "El nombre no puede estar vacío"
	case errors.Is(err, catalog.ErrNotFound):
		return "El elemento ya no existe"
	case errors.Is(err, catalog.ErrMergeSelf):
		return "No se puede unir un proyecto consigo mismo"
	}
	return "No se pudo completar: " + err.Error()
}

// pickOption es una opción de los selectores del catálogo; id 0 significa «ninguno».
type pickOption struct {
	id    int64
	label string
	name  string // nombre solo del elemento, sin su contexto; vacío si coincide con label
}

func (o pickOption) plainName() string {
	if o.name != "" {
		return o.name
	}
	return o.label
}

// filterPickOptions deja las opciones cuya etiqueta contiene la consulta, sin distinguir mayúsculas.
func filterPickOptions(all []pickOption, query string) []pickOption {
	query = strings.ToLower(strings.TrimSpace(query))
	var out []pickOption
	for _, o := range all {
		if strings.Contains(strings.ToLower(o.label), query) {
			out = append(out, o)
		}
	}
	return out
}

// pickPurpose indica qué hará la selección del selector.
type pickPurpose int

const (
	pickClientOrg     pickPurpose = iota // organización de un cliente nuevo
	pickProjectClient                    // cliente de un proyecto nuevo
	pickMoveClient
	pickMoveProject
	pickMergeInto
)

// catPicker es un selector con búsqueda abierto bajo el árbol.
type catPicker struct {
	purpose pickPurpose
	title   string
	all     []pickOption
	index   int
	scroll  int
	subject catRow // fila que se mueve o se une
	name    string // nombre pendiente al crear un cliente o un proyecto
}

// catInput es el campo de texto en línea: renombrar una fila o nombrar un elemento nuevo.
type catInput struct {
	create bool
	kind   catRowKind
	row    catRow
}

// catConfirm es la confirmación en línea; button 0 confirma y 1 cancela (valor por defecto).
type catConfirm struct {
	merge  bool
	row    catRow
	into   pickOption
	text   string
	button int
}

func (c catConfirm) labels() []string {
	if c.merge {
		return []string{"Unir", "Cancelar"}
	}
	return []string{"Eliminar", "Cancelar"}
}

// catFocus es el tipo de elemento enfocado en la pantalla de catálogo.
type catFocus int

const (
	catFocusTabs catFocus = iota
	catFocusRow
	catFocusAdd
	catFocusToggle
)

// catalogState es todo el estado de la pantalla de catálogo.
type catalogState struct {
	rows         []catRow
	showArchived bool
	focus        catFocus
	row          int // índice en rows de la fila enfocada
	button       int // botón resaltado de la fila o de las filas de acción
	scroll       int
	input        textinput.Model
	edit         *catInput
	picker       *catPicker
	confirm      *catConfirm
}

func (m *Model) initCatalogInput() {
	m.cat.input = textinput.New()
	m.cat.input.CharLimit = 120
	m.cat.input.Width = 42
}

// catalogEnabled indica si hay catálogo disponible (y por tanto pestañas).
func (m Model) catalogEnabled() bool { return m.catalog != nil }

// loadCatalog recarga el árbol (también lo usan los selectores de proyecto) y las filas si procede.
func (m *Model) loadCatalog() {
	if m.catalog == nil {
		return
	}
	if tree, err := m.catalog.Tree(); err == nil {
		m.tree = tree
	}
	if m.screen == screenCatalog {
		m.rebuildCatalogRows()
	}
}

// rebuildCatalogRows regenera las filas y conserva el foco sobre la misma fila si sigue existiendo.
func (m *Model) rebuildCatalogRows() {
	c := &m.cat
	var focused *catRow
	if c.focus == catFocusRow && c.row < len(c.rows) {
		r := c.rows[c.row]
		focused = &r
	}
	week := map[string]int64{}
	if totals, err := m.tracker.Report(rangeStart(m.now, true)); err == nil {
		for _, t := range totals {
			week[strings.ToLower(t.Project)] = t.Seconds
		}
	}
	stats := func(p catalog.Project) (int64, int) {
		tasks, _ := m.tracker.CountByProject(p.ID)
		return week[strings.ToLower(p.Name)], tasks
	}
	c.rows = buildCatalogRows(m.tree, c.showArchived, stats)
	if focused != nil {
		for i, r := range c.rows {
			if r.kind == focused.kind && r.id == focused.id && r.name == focused.name {
				c.row = i
				break
			}
		}
	}
	m.clampCatalog()
}

// clampCatalog mantiene el foco sobre una fila existente y editable.
func (m *Model) clampCatalog() {
	c := &m.cat
	if c.focus == catFocusRow {
		c.row = min(max(c.row, 0), max(0, len(c.rows)-1))
		if len(c.rows) == 0 {
			c.focus = catFocusAdd
		} else if c.rows[c.row].kind == catHeader {
			c.row = m.nearestEditable(c.row)
		}
		c.button = min(c.button, max(0, len(rowLabels(c.rows[min(c.row, len(c.rows)-1)]))-1))
	}
	if c.focus == catFocusAdd {
		c.button = min(c.button, len(addLabels)-1)
	}
	c.scroll = min(max(c.scroll, 0), max(0, len(c.rows)-m.catalogLayout().visible))
}

// nearestEditable devuelve la fila editable más cercana a i (hacia abajo y luego hacia arriba).
func (m Model) nearestEditable(i int) int {
	rows := m.cat.rows
	for j := i; j < len(rows); j++ {
		if rows[j].kind != catHeader {
			return j
		}
	}
	for j := i; j >= 0; j-- {
		if rows[j].kind != catHeader {
			return j
		}
	}
	return i
}

// switchScreen cambia de pantalla y deja el foco en la barra de pestañas.
func (m *Model) switchScreen(to screen) {
	if to == screenCatalog && !m.catalogEnabled() {
		return
	}
	m.cancelCatalogModal()
	m.closeProjectPicker()
	m.screen = to
	if to == screenCatalog {
		m.cat.focus = catFocusTabs
		m.cat.button = 0
		m.cat.scroll = 0
		m.refresh()
		return
	}
	m.focus = focusTabs
	m.syncInputFocus()
	m.refresh()
}

// catStop es una parada del anillo de foco de la pantalla de catálogo.
type catStop struct {
	focus catFocus
	row   int
}

func (m Model) catStops() []catStop {
	stops := []catStop{{catFocusTabs, 0}}
	for i, r := range m.cat.rows {
		if r.kind != catHeader {
			stops = append(stops, catStop{catFocusRow, i})
		}
	}
	return append(stops, catStop{catFocusAdd, 0}, catStop{catFocusToggle, 0})
}

func (m *Model) moveCatalogFocus(step int) {
	c := &m.cat
	stops := m.catStops()
	at := 0
	for i, s := range stops {
		if s.focus == c.focus && (s.focus != catFocusRow || s.row == c.row) {
			at = i
			break
		}
	}
	next := stops[min(max(at+step, 0), len(stops)-1)]
	if next.focus != c.focus || next.row != c.row {
		c.button = 0
	}
	c.focus, c.row = next.focus, next.row
	m.revealCatalog()
}

// revealCatalog ajusta el desplazamiento para que la fila enfocada quede visible.
func (m *Model) revealCatalog() {
	c := &m.cat
	if c.focus != catFocusRow {
		return
	}
	visible := max(1, m.catalogLayout().visible)
	if c.row < c.scroll {
		c.scroll = c.row
	}
	if c.row >= c.scroll+visible {
		c.scroll = c.row - visible + 1
	}
}

// cancelCatalogModal descarta cualquier campo, selector o confirmación abierto sin cambiar nada.
func (m *Model) cancelCatalogModal() {
	m.cat.edit, m.cat.picker, m.cat.confirm = nil, nil, nil
	m.cat.input.Blur()
}

func (m Model) catalogBusy() bool {
	return m.cat.edit != nil || m.cat.picker != nil || m.cat.confirm != nil
}

// updateCatalogKey procesa una tecla en la pantalla de catálogo.
func (m *Model) updateCatalogKey(key tea.KeyMsg) tea.Cmd {
	c := &m.cat
	if key.Type == tea.KeyEsc {
		if m.catalogBusy() {
			m.cancelCatalogModal()
			m.setMessage("Cancelado")
		} else {
			m.switchScreen(screenTimers)
		}
		return nil
	}
	switch {
	case c.confirm != nil:
		m.updateCatalogConfirm(key)
		return nil
	case c.picker != nil:
		return m.updateCatalogPicker(key)
	case c.edit != nil:
		if key.Type == tea.KeyEnter {
			m.submitCatalogInput()
			return nil
		}
		var cmd tea.Cmd
		c.input, cmd = c.input.Update(key)
		return cmd
	}
	switch key.Type {
	case tea.KeyUp, tea.KeyShiftTab:
		m.moveCatalogFocus(-1)
	case tea.KeyDown, tea.KeyTab:
		m.moveCatalogFocus(1)
	case tea.KeyLeft, tea.KeyRight:
		step := 1
		if key.Type == tea.KeyLeft {
			step = -1
		}
		switch c.focus {
		case catFocusTabs:
			m.switchScreen(screenTimers)
		case catFocusRow:
			c.button = min(max(c.button+step, 0), len(rowLabels(c.rows[c.row]))-1)
		case catFocusAdd:
			c.button = min(max(c.button+step, 0), len(addLabels)-1)
		}
	case tea.KeyEnter:
		m.activateCatalogFocus()
	}
	return nil
}

// activateCatalogFocus ejecuta el botón o la fila enfocada.
func (m *Model) activateCatalogFocus() {
	c := &m.cat
	switch c.focus {
	case catFocusTabs:
		m.switchScreen(screenTimers)
	case catFocusRow:
		m.activateRowAction(c.rows[c.row], rowActions(c.rows[c.row])[c.button])
	case catFocusAdd:
		m.startCreate([]catRowKind{catOrg, catClient, catProject}[c.button])
	case catFocusToggle:
		c.showArchived = !c.showArchived
		m.rebuildCatalogRows()
	}
}

func (m *Model) openInput(in catInput, value, placeholder string) {
	m.cat.edit = &in
	m.cat.input.SetValue(value)
	m.cat.input.CursorEnd()
	m.cat.input.Placeholder = placeholder
	m.cat.input.Focus()
}

func (m *Model) startCreate(kind catRowKind) {
	placeholder := map[catRowKind]string{catOrg: "Nombre de la organización", catClient: "Nombre del cliente", catProject: "Nombre del proyecto"}[kind]
	m.openInput(catInput{create: true, kind: kind}, "", placeholder)
}

func (m *Model) activateRowAction(r catRow, a catAction) {
	switch a {
	case actRename:
		m.openInput(catInput{kind: r.kind, row: r}, r.name, "Nuevo nombre")
	case actMove:
		m.openMovePicker(r)
	case actMerge:
		m.openMergePicker(r)
	case actArchive:
		if r.archived {
			m.finishCatalog("Desarchivado «"+r.name+"»", m.catalog.UnarchiveProject(r.id))
		} else {
			m.finishCatalog("Archivado «"+r.name+"»", m.catalog.ArchiveProject(r.id))
		}
	case actDelete:
		m.cat.confirm = &catConfirm{row: r, text: deleteMessage(r), button: 1}
	}
}

// finishCatalog refresca los datos y avisa el resultado; devuelve true si no hubo error.
func (m *Model) finishCatalog(done string, err error) bool {
	if err != nil {
		m.setMessage(catalogErrorText(err))
		return false
	}
	m.refresh()
	m.setMessage(done)
	m.revealCatalog()
	return true
}

// submitCatalogInput guarda el nombre escrito: renombra, crea o pasa a elegir el padre.
func (m *Model) submitCatalogInput() {
	c := &m.cat
	in := *c.edit
	name := strings.TrimSpace(c.input.Value())
	if name == "" {
		m.setMessage("Escribe un nombre")
		return
	}
	if !in.create {
		if name == in.row.name {
			m.cancelCatalogModal()
			m.setMessage("Sin cambios")
			return
		}
		var err error
		switch in.kind {
		case catOrg:
			err = m.catalog.RenameOrganization(in.row.id, name)
		case catClient:
			err = m.catalog.RenameClient(in.row.id, name)
		default:
			err = m.catalog.RenameProject(in.row.id, name)
		}
		// Ante un error el campo sigue abierto para corregir el nombre.
		if m.finishCatalog("Renombrado «"+name+"»", err) {
			m.cancelCatalogModal()
		}
		return
	}
	switch in.kind {
	case catOrg:
		_, err := m.catalog.CreateOrganization(name)
		if m.finishCatalog("Organización «"+name+"» creada", err) {
			m.cancelCatalogModal()
		}
	case catClient:
		m.cancelCatalogModal()
		m.openPicker(&catPicker{purpose: pickClientOrg, title: "Organización de «" + name + "»", all: m.orgOptions("Sin organización"), name: name})
	default:
		m.cancelCatalogModal()
		m.openPicker(&catPicker{purpose: pickProjectClient, title: "Cliente de «" + name + "»", all: m.clientOptions("Sin cliente"), name: name})
	}
}

// orgOptions lista las organizaciones precedidas por la opción «ninguna» con la etiqueta dada.
func (m Model) orgOptions(none string) []pickOption {
	options := []pickOption{{label: none}}
	for _, g := range m.tree {
		if g.Organization != nil {
			options = append(options, pickOption{id: g.Organization.ID, label: g.Organization.Name})
		}
	}
	return options
}

// clientOptions lista los clientes (con su organización) precedidos por la opción «ninguno».
func (m Model) clientOptions(none string) []pickOption {
	options := []pickOption{{label: none}}
	for _, g := range m.tree {
		for _, c := range g.Clients {
			if c.Client == nil {
				continue
			}
			text := c.Client.Name
			if g.Organization != nil {
				text += " · " + g.Organization.Name
			}
			options = append(options, pickOption{id: c.Client.ID, label: text})
		}
	}
	return options
}

func (m *Model) openPicker(p *catPicker) {
	m.cat.picker = p
	m.cat.input.SetValue("")
	m.cat.input.Placeholder = "Buscar"
	m.cat.input.Focus()
}

func (m *Model) openMovePicker(r catRow) {
	if r.kind == catClient {
		m.openPicker(&catPicker{purpose: pickMoveClient, title: "Mover «" + r.name + "» a la organización", all: m.orgOptions("Ninguno"), subject: r})
		return
	}
	m.openPicker(&catPicker{purpose: pickMoveProject, title: "Mover «" + r.name + "» al cliente", all: m.clientOptions("Ninguno"), subject: r})
}

func (m *Model) openMergePicker(r catRow) {
	var options []pickOption
	for _, g := range m.tree {
		for _, c := range g.Clients {
			for _, p := range c.Projects {
				if p.ID == r.id || p.ArchivedAt != nil {
					continue
				}
				text := p.Name
				if c.Client != nil {
					text += " · " + c.Client.Name
				}
				options = append(options, pickOption{id: p.ID, label: text, name: p.Name})
			}
		}
	}
	m.openPicker(&catPicker{purpose: pickMergeInto, title: "Unir «" + r.name + "» en", all: options, subject: r})
}

// pickerOptions devuelve las opciones visibles del selector según el texto de búsqueda.
func (m Model) pickerOptions() []pickOption {
	return filterPickOptions(m.cat.picker.all, m.cat.input.Value())
}

func (m *Model) updateCatalogPicker(key tea.KeyMsg) tea.Cmd {
	p := m.cat.picker
	options := m.pickerOptions()
	switch key.Type {
	case tea.KeyUp:
		p.index = max(0, p.index-1)
	case tea.KeyDown:
		p.index = min(max(0, len(options)-1), p.index+1)
	case tea.KeyEnter:
		m.selectPickerIndex(p.index)
		return nil
	default:
		var cmd tea.Cmd
		m.cat.input, cmd = m.cat.input.Update(key)
		p.index, p.scroll = 0, 0
		return cmd
	}
	p.scroll = m.catalogLayout().optionsStart
	return nil
}

// selectPickerIndex aplica la opción número i de la lista filtrada del selector.
func (m *Model) selectPickerIndex(i int) {
	options := m.pickerOptions()
	if i < 0 || i >= len(options) {
		return
	}
	p, chosen := *m.cat.picker, options[i]
	switch p.purpose {
	case pickMergeInto:
		m.cancelCatalogModal()
		m.cat.confirm = &catConfirm{merge: true, row: p.subject, into: chosen, text: mergeMessage(p.subject, chosen.plainName()), button: 1}
		return
	case pickClientOrg:
		_, err := m.catalog.CreateClient(p.name, chosen.id)
		m.cancelCatalogModal()
		m.finishCatalog("Cliente «"+p.name+"» creado", err)
	case pickProjectClient:
		_, err := m.catalog.CreateProject(p.name, chosen.id)
		m.cancelCatalogModal()
		m.finishCatalog("Proyecto «"+p.name+"» creado", err)
	case pickMoveClient:
		err := m.catalog.MoveClient(p.subject.id, chosen.id)
		m.cancelCatalogModal()
		m.finishCatalog("Movido «"+p.subject.name+"»", err)
	case pickMoveProject:
		err := m.catalog.MoveProject(p.subject.id, chosen.id)
		m.cancelCatalogModal()
		m.finishCatalog("Movido «"+p.subject.name+"»", err)
	}
}

func (m *Model) updateCatalogConfirm(key tea.KeyMsg) {
	c := m.cat.confirm
	switch key.Type {
	case tea.KeyLeft:
		c.button = 0
	case tea.KeyRight:
		c.button = 1
	case tea.KeyTab, tea.KeyShiftTab:
		c.button = 1 - c.button
	case tea.KeyEnter:
		m.activateCatalogConfirm(c.button)
	}
}

func (m *Model) activateCatalogConfirm(button int) {
	c := *m.cat.confirm
	m.cancelCatalogModal()
	if button != 0 {
		m.setMessage("Cancelado")
		return
	}
	if c.merge {
		m.finishCatalog("Unido «"+c.row.name+"» en «"+c.into.plainName()+"»", m.catalog.MergeProjects(c.row.id, c.into.id))
		return
	}
	var err error
	switch c.row.kind {
	case catOrg:
		err = m.catalog.DeleteOrganization(c.row.id)
	case catClient:
		err = m.catalog.DeleteClient(c.row.id)
	default:
		err = m.catalog.DeleteProject(c.row.id)
	}
	m.finishCatalog("Eliminado «"+c.row.name+"»", err)
}
