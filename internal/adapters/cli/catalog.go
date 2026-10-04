package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"nexus/internal/catalog"
)

var (
	addClientFlags  = map[string]string{"-o": "parent", "--organization": "parent"}
	addProjectFlags = map[string]string{"-c": "parent", "--client": "parent"}
	yesFlags        = map[string]string{"--json": "json", "--yes": "yes"}
	treeFlags       = map[string]string{"--json": "json", "--all": "all"}
)

// index es una vista plana del catálogo que sirve para resolver referencias por nombre o id.
type index struct {
	orgs     []catalog.Organization
	clients  []catalog.Client
	projects []catalog.Project
}

func loadIndex(c Catalog) (*index, error) {
	tree, err := c.Tree()
	if err != nil {
		return nil, err
	}
	x := &index{}
	for _, group := range tree {
		if group.Organization != nil {
			x.orgs = append(x.orgs, *group.Organization)
		}
		for _, client := range group.Clients {
			if client.Client != nil {
				x.clients = append(x.clients, *client.Client)
			}
			x.projects = append(x.projects, client.Projects...)
		}
	}
	return x, nil
}

// pick resuelve una referencia: el nombre (sin distinguir mayúsculas) gana sobre el id numérico.
func pick[T any](items []T, ref string, k kind, name func(T) string, id func(T) int64) (T, error) {
	var zero T
	var found []T
	for _, item := range items {
		if strings.EqualFold(name(item), strings.TrimSpace(ref)) {
			found = append(found, item)
		}
	}
	if len(found) == 0 {
		if n, err := strconv.ParseInt(strings.TrimSpace(ref), 10, 64); err == nil {
			for _, item := range items {
				if id(item) == n {
					return item, nil
				}
			}
		}
		return zero, errors.New(k.missing)
	}
	if len(found) > 1 {
		return zero, fmt.Errorf("hay varios elementos llamados «%s»; usa el id", ref)
	}
	return found[0], nil
}

// byID busca un elemento por id exacto, sin considerar nombres; se usa para releer tras un cambio.
func byID[T any](items []T, want int64, k kind, id func(T) int64) (T, error) {
	for _, item := range items {
		if id(item) == want {
			return item, nil
		}
	}
	var zero T
	return zero, errors.New(k.missing)
}

func (x *index) org(ref string) (catalog.Organization, error) {
	return pick(x.orgs, ref, kindOrg, func(o catalog.Organization) string { return o.Name }, func(o catalog.Organization) int64 { return o.ID })
}
func (x *index) client(ref string) (catalog.Client, error) {
	return pick(x.clients, ref, kindClient, func(c catalog.Client) string { return c.Name }, func(c catalog.Client) int64 { return c.ID })
}
func (x *index) project(ref string) (catalog.Project, error) {
	return pick(x.projects, ref, kindProject, func(p catalog.Project) string { return p.Name }, func(p catalog.Project) int64 { return p.ID })
}

// parentID resuelve la referencia opcional a un padre; "-" o vacío significan ninguno (id 0).
func parentID(ref string, resolve func(string) (int64, error)) (int64, error) {
	if ref == "" || ref == "-" {
		return 0, nil
	}
	return resolve(ref)
}

func (x *index) orgID(ref string) (int64, error) {
	o, err := x.org(ref)
	return o.ID, err
}
func (x *index) clientID(ref string) (int64, error) {
	c, err := x.client(ref)
	return c.ID, err
}

// confirm impide ejecutar una operación destructiva sin --yes y explica qué ocurriría.
func confirm(yes bool, message string) error {
	if yes {
		return nil
	}
	return errors.New(message + " Repite con --yes para confirmar.")
}

// emit imprime el objeto como JSON o, si no, la línea de texto.
func emit(stdout io.Writer, jsonMode bool, object any, text string) error {
	if jsonMode {
		return json.NewEncoder(stdout).Encode(object)
	}
	_, err := fmt.Fprintln(stdout, text)
	return err
}

func runCatalog(group string, args []string, c Catalog, tracker Tracker, stdout io.Writer) error {
	jsonMode := contains(args, "--json")
	var err error
	switch group {
	case "tree":
		err = treeCommand(args, c, stdout)
	case "org":
		err = orgCommand(args, c, stdout)
	case "client":
		err = clientCommand(args, c, stdout)
	default:
		err = projectCommand(args, c, tracker, stdout)
	}
	return commandError(err, jsonMode, stdout)
}

// subcommand separa el subcomando y valida el número de posicionales; usage se muestra al fallar.
func subcommand(args []string, valueFlags, boolFlags map[string]string, want int, usage string) (parsedArgs, error) {
	parsed, err := parseArgs(args, valueFlags, boolFlags)
	if err != nil {
		return parsed, err
	}
	if len(parsed.positional) != want {
		return parsed, errors.New("uso: " + usage)
	}
	return parsed, nil
}

func orgCommand(args []string, c Catalog, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("uso: nexus org <add|rename|rm> ...")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "add":
		parsed, err := subcommand(rest, nil, jsonFlags, 1, "nexus org add <nombre>")
		if err != nil {
			return err
		}
		org, err := c.CreateOrganization(parsed.positional[0])
		if err != nil {
			return localizeCatalog(kindOrg, err)
		}
		return emit(stdout, parsed.flags["json"], presentOrg(org), fmt.Sprintf("organización creada #%d %s", org.ID, org.Name))
	case "rename":
		parsed, err := subcommand(rest, nil, jsonFlags, 2, "nexus org rename <org> <nuevo>")
		if err != nil {
			return err
		}
		x, err := loadIndex(c)
		if err != nil {
			return err
		}
		org, err := x.org(parsed.positional[0])
		if err != nil {
			return err
		}
		if err := c.RenameOrganization(org.ID, parsed.positional[1]); err != nil {
			return localizeCatalog(kindOrg, err)
		}
		if x, err = loadIndex(c); err != nil {
			return err
		}
		if org, err = byID(x.orgs, org.ID, kindOrg, func(o catalog.Organization) int64 { return o.ID }); err != nil {
			return err
		}
		return emit(stdout, parsed.flags["json"], presentOrg(org), fmt.Sprintf("organización renombrada #%d %s", org.ID, org.Name))
	case "rm":
		parsed, err := subcommand(rest, nil, yesFlags, 1, "nexus org rm <org> --yes")
		if err != nil {
			return err
		}
		x, err := loadIndex(c)
		if err != nil {
			return err
		}
		org, err := x.org(parsed.positional[0])
		if err != nil {
			return err
		}
		if err := confirm(parsed.flags["yes"], fmt.Sprintf("Esto eliminará la organización «%s»; sus clientes quedarán sin organización.", org.Name)); err != nil {
			return err
		}
		if err := c.DeleteOrganization(org.ID); err != nil {
			return localizeCatalog(kindOrg, err)
		}
		return emit(stdout, parsed.flags["json"], okOutput{OK: true}, fmt.Sprintf("organización «%s» eliminada", org.Name))
	}
	return fmt.Errorf("subcomando desconocido %q; usa nexus help", sub)
}

func clientCommand(args []string, c Catalog, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("uso: nexus client <add|rename|mv|rm> ...")
	}
	sub, rest := args[0], args[1:]
	clientID := func(x *index, c catalog.Client) (catalog.Client, error) {
		return byID(x.clients, c.ID, kindClient, func(c catalog.Client) int64 { return c.ID })
	}
	switch sub {
	case "add":
		parsed, err := subcommand(rest, addClientFlags, jsonFlags, 1, "nexus client add <nombre> [-o organización]")
		if err != nil {
			return err
		}
		x, err := loadIndex(c)
		if err != nil {
			return err
		}
		orgID, err := parentID(parsed.values["parent"], x.orgID)
		if err != nil {
			return err
		}
		client, err := c.CreateClient(parsed.positional[0], orgID)
		if err != nil {
			return localizeCatalog(kindClient, err)
		}
		return emit(stdout, parsed.flags["json"], presentClient(client), fmt.Sprintf("cliente creado #%d %s", client.ID, client.Name))
	case "rename", "mv":
		usage := "nexus client rename <cliente> <nuevo>"
		if sub == "mv" {
			usage = "nexus client mv <cliente> <organización|->"
		}
		parsed, err := subcommand(rest, nil, jsonFlags, 2, usage)
		if err != nil {
			return err
		}
		x, err := loadIndex(c)
		if err != nil {
			return err
		}
		client, err := x.client(parsed.positional[0])
		if err != nil {
			return err
		}
		text := "cliente renombrado #%d %s"
		if sub == "rename" {
			err = c.RenameClient(client.ID, parsed.positional[1])
		} else {
			var orgID int64
			if orgID, err = parentID(parsed.positional[1], x.orgID); err != nil {
				return err
			}
			err = c.MoveClient(client.ID, orgID)
			text = "cliente movido #%d %s: sin organización"
			if orgID != 0 {
				org, _ := byID(x.orgs, orgID, kindOrg, func(o catalog.Organization) int64 { return o.ID })
				text = "cliente movido #%d %s: organización «" + escapeFormat(org.Name) + "»"
			}
		}
		if err != nil {
			return localizeCatalog(kindClient, err)
		}
		if x, err = loadIndex(c); err != nil {
			return err
		}
		if client, err = clientID(x, client); err != nil {
			return err
		}
		return emit(stdout, parsed.flags["json"], presentClient(client), fmt.Sprintf(text, client.ID, client.Name))
	case "rm":
		parsed, err := subcommand(rest, nil, yesFlags, 1, "nexus client rm <cliente> --yes")
		if err != nil {
			return err
		}
		x, err := loadIndex(c)
		if err != nil {
			return err
		}
		client, err := x.client(parsed.positional[0])
		if err != nil {
			return err
		}
		if err := confirm(parsed.flags["yes"], fmt.Sprintf("Esto eliminará el cliente «%s»; sus proyectos quedarán sin cliente.", client.Name)); err != nil {
			return err
		}
		if err := c.DeleteClient(client.ID); err != nil {
			return localizeCatalog(kindClient, err)
		}
		return emit(stdout, parsed.flags["json"], okOutput{OK: true}, fmt.Sprintf("cliente «%s» eliminado", client.Name))
	}
	return fmt.Errorf("subcomando desconocido %q; usa nexus help", sub)
}

func projectCommand(args []string, c Catalog, tracker Tracker, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("uso: nexus project <add|rename|mv|archive|unarchive|merge|rm> ...")
	}
	sub, rest := args[0], args[1:]
	if sub == "add" {
		parsed, err := subcommand(rest, addProjectFlags, jsonFlags, 1, "nexus project add <nombre> [-c cliente]")
		if err != nil {
			return err
		}
		x, err := loadIndex(c)
		if err != nil {
			return err
		}
		clientID, err := parentID(parsed.values["parent"], x.clientID)
		if err != nil {
			return err
		}
		project, err := c.CreateProject(parsed.positional[0], clientID)
		if err != nil {
			return localizeCatalog(kindProject, err)
		}
		return emit(stdout, parsed.flags["json"], presentProject(project), fmt.Sprintf("proyecto creado #%d %s", project.ID, project.Name))
	}
	// Los demás subcomandos operan sobre un proyecto existente; solo cambian los posicionales y la acción.
	positionals := map[string]int{"rename": 2, "mv": 2, "archive": 1, "unarchive": 1, "merge": 2, "rm": 1}
	usages := map[string]string{
		"rename":    "nexus project rename <proyecto> <nuevo>",
		"mv":        "nexus project mv <proyecto> <cliente|->",
		"archive":   "nexus project archive <proyecto>",
		"unarchive": "nexus project unarchive <proyecto>",
		"merge":     "nexus project merge <origen> <destino> --yes",
		"rm":        "nexus project rm <proyecto> --yes",
	}
	want, ok := positionals[sub]
	if !ok {
		return fmt.Errorf("subcomando desconocido %q; usa nexus help", sub)
	}
	parsed, err := subcommand(rest, nil, yesFlags, want, usages[sub])
	if err != nil {
		return err
	}
	x, err := loadIndex(c)
	if err != nil {
		return err
	}
	project, err := x.project(parsed.positional[0])
	if err != nil {
		return err
	}
	jsonMode := parsed.flags["json"]
	text := ""
	switch sub {
	case "rename":
		err, text = c.RenameProject(project.ID, parsed.positional[1]), "proyecto renombrado #%d %s"
	case "archive":
		err, text = c.ArchiveProject(project.ID), "proyecto archivado #%d %s"
	case "unarchive":
		err, text = c.UnarchiveProject(project.ID), "proyecto desarchivado #%d %s"
	case "mv":
		var clientID int64
		if clientID, err = parentID(parsed.positional[1], x.clientID); err != nil {
			return err
		}
		text = "proyecto movido #%d %s: sin cliente"
		if clientID != 0 {
			client, _ := byID(x.clients, clientID, kindClient, func(c catalog.Client) int64 { return c.ID })
			text = "proyecto movido #%d %s: cliente «" + escapeFormat(client.Name) + "»"
		}
		err = c.MoveProject(project.ID, clientID)
	case "merge":
		into, err := x.project(parsed.positional[1])
		if err != nil {
			return err
		}
		if into.ID == project.ID {
			return localizeCatalog(kindProject, catalog.ErrMergeSelf)
		}
		if err := confirm(parsed.flags["yes"], fmt.Sprintf("Esto unirá el proyecto «%s» en «%s»; %s y «%s» dejará de existir.", project.Name, into.Name, movedEntries(countEntries(tracker, project.ID), into.Name), project.Name)); err != nil {
			return err
		}
		if err := c.MergeProjects(project.ID, into.ID); err != nil {
			return localizeCatalog(kindProject, err)
		}
		return emit(stdout, jsonMode, presentProject(into), fmt.Sprintf("proyecto «%s» unido en «%s»", project.Name, into.Name))
	case "rm":
		if err := confirm(parsed.flags["yes"], fmt.Sprintf("Esto eliminará el proyecto «%s»; %s.", project.Name, orphanedEntries(countEntries(tracker, project.ID)))); err != nil {
			return err
		}
		if err := c.DeleteProject(project.ID); err != nil {
			return localizeCatalog(kindProject, err)
		}
		return emit(stdout, jsonMode, okOutput{OK: true}, fmt.Sprintf("proyecto «%s» eliminado", project.Name))
	}
	if err != nil {
		return localizeCatalog(kindProject, err)
	}
	if x, err = loadIndex(c); err != nil {
		return err
	}
	if project, err = byID(x.projects, project.ID, kindProject, func(p catalog.Project) int64 { return p.ID }); err != nil {
		return err
	}
	return emit(stdout, jsonMode, presentProject(project), fmt.Sprintf(text, project.ID, project.Name))
}

func treeCommand(args []string, c Catalog, stdout io.Writer) error {
	parsed, err := subcommand(args, nil, treeFlags, 0, "nexus tree [--json] [--all]")
	if err != nil {
		return err
	}
	tree, err := c.Tree()
	if err != nil {
		return err
	}
	all := parsed.flags["all"]
	// visible filtra los proyectos archivados salvo con --all.
	visible := func(projects []catalog.Project) []catalog.Project {
		var out []catalog.Project
		for _, p := range projects {
			if all || p.ArchivedAt == nil {
				out = append(out, p)
			}
		}
		return out
	}
	var out []treeOrgOutput
	var lines []string
	for _, group := range tree {
		node := treeOrgOutput{Clients: []treeClientOutput{}}
		var groupLines []string
		if group.Organization != nil {
			org := presentOrg(*group.Organization)
			node.Organization = &org
			groupLines = append(groupLines, label(org.Name, org.Archived))
		}
		for _, client := range group.Clients {
			projects := visible(client.Projects)
			if client.Client == nil && len(projects) == 0 {
				continue
			}
			item := treeClientOutput{Projects: []projectObject{}}
			name := "Sin cliente"
			if client.Client != nil {
				c := presentClient(*client.Client)
				item.Client = &c
				name = label(c.Name, c.Archived)
			}
			groupLines = append(groupLines, "  "+name)
			for _, p := range projects {
				po := presentProject(p)
				item.Projects = append(item.Projects, po)
				groupLines = append(groupLines, "    "+label(po.Name, po.Archived))
			}
			node.Clients = append(node.Clients, item)
		}
		if group.Organization == nil {
			if len(node.Clients) == 0 {
				continue
			}
			groupLines = append([]string{"Sin organización"}, groupLines...)
		}
		out = append(out, node)
		lines = append(lines, groupLines...)
	}
	if parsed.flags["json"] {
		if out == nil {
			out = []treeOrgOutput{}
		}
		return json.NewEncoder(stdout).Encode(out)
	}
	if len(lines) == 0 {
		_, err = fmt.Fprintln(stdout, "Sin elementos en el catálogo")
		return err
	}
	_, err = fmt.Fprintln(stdout, strings.Join(lines, "\n"))
	return err
}

func label(name string, archived bool) string {
	if archived {
		return name + " (archivado)"
	}
	return name
}

// escapeFormat protege un nombre de usuario que se inserta en una plantilla de Sprintf.
func escapeFormat(name string) string { return strings.ReplaceAll(name, "%", "%%") }

// countEntries devuelve cuántas tareas tiene el proyecto, o -1 si no se puede saber.
func countEntries(tracker Tracker, projectID int64) int {
	if tracker == nil {
		return -1
	}
	n, err := tracker.CountByProject(projectID)
	if err != nil {
		return -1
	}
	return n
}

// orphanedEntries describe qué pasa con las tareas al eliminar su proyecto.
func orphanedEntries(n int) string {
	switch {
	case n < 0:
		return "sus tareas quedarán sin proyecto"
	case n == 0:
		return "no tiene tareas"
	case n == 1:
		return "1 tarea quedará sin proyecto"
	default:
		return fmt.Sprintf("%d tareas quedarán sin proyecto", n)
	}
}

// movedEntries describe qué pasa con las tareas al unir su proyecto en otro.
func movedEntries(n int, into string) string {
	switch {
	case n < 0:
		return fmt.Sprintf("sus tareas pasarán a «%s»", into)
	case n == 0:
		return "no tiene tareas que mover"
	case n == 1:
		return fmt.Sprintf("1 tarea pasará a «%s»", into)
	default:
		return fmt.Sprintf("%d tareas pasarán a «%s»", n, into)
	}
}
