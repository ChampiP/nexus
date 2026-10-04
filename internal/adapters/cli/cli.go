// Package cli implements Nexus command parsing and output presenters.
package cli

import (
	"fmt"
	"io"
	"time"

	"nexus/internal/catalog"
	"nexus/internal/tracking"
)

// Tracker describes the use cases required by the command line.
type Tracker interface {
	Start(tracking.StartInput) (tracking.Entry, error)
	Stop(int64) error
	StopLatest() error
	StopAll() (int, error)
	Snapshot() ([]tracking.Entry, int64, []tracking.Entry, error)
	Projects(string) []tracking.ProjectUsage
	Report(time.Time) ([]tracking.ProjectTotal, error)
	Edit(int64, tracking.EditInput) (tracking.Entry, error)
	Delete(int64) error
	Restore(int64) error
	Deleted(int) ([]tracking.Entry, error)
	CountByProject(int64) (int, error)
}

// Catalog describe los casos de uso del catálogo que necesita la línea de comandos.
type Catalog interface {
	CreateOrganization(string) (catalog.Organization, error)
	RenameOrganization(int64, string) error
	DeleteOrganization(int64) error
	CreateClient(string, int64) (catalog.Client, error)
	RenameClient(int64, string) error
	MoveClient(clientID, organizationID int64) error
	DeleteClient(int64) error
	CreateProject(string, int64) (catalog.Project, error)
	RenameProject(int64, string) error
	MoveProject(projectID, clientID int64) error
	ArchiveProject(int64) error
	UnarchiveProject(int64) error
	DeleteProject(int64) error
	MergeProjects(fromID, intoID int64) error
	Tree() ([]catalog.TreeOrganization, error)
}

// Run executes a Nexus command. A nil tracker is supported by status --json so that
// status remains safe when the configured database cannot be opened.
func Run(args []string, tracker Tracker, stdout, stderr io.Writer) error {
	return RunWithCatalog(args, tracker, nil, stdout, stderr)
}

// RunWithCatalog ejecuta un comando de Nexus con acceso al catálogo; un catálogo nulo hace que
// los comandos de catálogo fallen con un mensaje claro en lugar de entrar en pánico.
func RunWithCatalog(args []string, tracker Tracker, catalog Catalog, stdout, stderr io.Writer) error {
	return localize(run(args, tracker, catalog, stdout, stderr))
}

func run(args []string, tracker Tracker, catalog Catalog, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(stderr, usageText)
		return fmt.Errorf("falta el comando")
	}
	command, args := args[0], args[1:]
	switch command {
	case "help", "-h", "--help":
		_, err := fmt.Fprint(stdout, usageText)
		return err
	case "status":
		return runStatus(args, tracker, stdout)
	case "org", "client", "project", "tree":
		if catalog == nil {
			return fmt.Errorf("el catálogo no está disponible")
		}
		return runCatalog(command, args, catalog, tracker, stdout)
	}
	if tracker == nil {
		return fmt.Errorf("el registro de tiempo no está disponible")
	}
	switch command {
	case "start":
		return runStart(args, tracker, stdout)
	case "stop":
		return runStop(args, tracker, stdout)
	case "ls":
		return runList(args, tracker, stdout)
	case "projects":
		return runProjects(args, tracker, stdout)
	case "report":
		return runReport(args, tracker, stdout)
	case "edit":
		return runEdit(args, tracker, stdout)
	case "rm":
		return runRemove(args, tracker, stdout)
	case "restore":
		return runRestore(args, tracker, stdout)
	case "trash":
		return runTrash(args, tracker, stdout)
	default:
		fmt.Fprint(stderr, usageText)
		return fmt.Errorf("comando desconocido %q", command)
	}
}

// usageText lista todos los comandos agrupados por tema.
const usageText = `Uso: nexus [comando]

Sin comando se abre la interfaz interactiva.

Temporizadores:
  nexus start <título> [-p proyecto] [-d descripción] [--json]   inicia un temporizador
  nexus stop [id] [--all] [--json]                               detiene uno o todos
  nexus status [--json]                                          muestra lo que está en curso
  nexus ls [--json]                                              lista en curso y recientes
  nexus report [--week]                                          tiempo por proyecto

Tareas:
  nexus edit <id> [-t título] [-p proyecto] [-d descripción] [--json]
  nexus rm <id> [--json]                                         envía la tarea a la papelera
  nexus restore <id> [--json]                                    la recupera de la papelera
  nexus trash [--json]                                           lista la papelera
  nexus projects [-q consulta] [--json]                          proyectos usados

Catálogo (las referencias aceptan nombre o id; "-" significa ninguno):
  nexus tree [--all] [--json]                                    organización > cliente > proyecto
  nexus org add <nombre>
  nexus org rename <org> <nuevo>
  nexus org rm <org> --yes
  nexus client add <nombre> [-o organización]
  nexus client rename <cliente> <nuevo>
  nexus client mv <cliente> <organización|->
  nexus client rm <cliente> --yes
  nexus project add <nombre> [-c cliente]
  nexus project rename <proyecto> <nuevo>
  nexus project mv <proyecto> <cliente|->
  nexus project archive <proyecto>
  nexus project unarchive <proyecto>
  nexus project merge <origen> <destino> --yes
  nexus project rm <proyecto> --yes

Los comandos de catálogo aceptan --json; los destructivos exigen --yes.
`

func contains(args []string, value string) bool {
	for _, arg := range args {
		if arg == value {
			return true
		}
	}
	return false
}
func hasUnknownStatusArgument(args []string) bool {
	for _, arg := range args {
		if arg != "--json" {
			return true
		}
	}
	return false
}
func dayStart(now time.Time) time.Time {
	y, m, d := now.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, now.Location())
}
func elapsed(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	return fmt.Sprintf("%02d:%02d:%02d", seconds/3600, seconds/60%60, seconds%60)
}
func humanDuration(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	d := time.Duration(seconds) * time.Second
	if d >= time.Hour {
		h, m := int64(d/time.Hour), int64(d/time.Minute)%60
		if m > 0 {
			return fmt.Sprintf("%dh%dm", h, m)
		}
		return fmt.Sprintf("%dh", h)
	}
	if d >= time.Minute {
		return fmt.Sprintf("%dm", int64(d/time.Minute))
	}
	return fmt.Sprintf("%ds", seconds)
}
