package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"nexus/internal/catalog"
	"nexus/internal/tracking"
)

// Tracker is the narrow application boundary required by the terminal interface.
type Tracker interface {
	Start(tracking.StartInput) (tracking.Entry, error)
	StartLike(int64) (tracking.Entry, error)
	Stop(int64) error
	Edit(int64, tracking.EditInput) (tracking.Entry, error)
	Delete(int64) error
	Restore(int64) error
	Snapshot() ([]tracking.Entry, int64, []tracking.Entry, error)
	Projects(string) []tracking.ProjectUsage
	Report(time.Time) ([]tracking.ProjectTotal, error)
	CountByProject(int64) (int, error)
	BreaksSince(time.Time) ([]tracking.Entry, error)
	Get(int64) (tracking.Entry, error)
}

// Catalog es la frontera mínima del catálogo que necesita la pantalla de catálogo.
type Catalog interface {
	Tree() ([]catalog.TreeOrganization, error)
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
}

// Option personaliza el Model al crearlo.
type Option func(*Model)

// WithCatalog habilita la pantalla de catálogo y los proyectos del catálogo en los selectores.
func WithCatalog(c Catalog) Option { return func(m *Model) { m.catalog = c } }

type tickMsg time.Time

// Run starts the interactive terminal interface.
func Run(tracker Tracker, catalog Catalog, breaks Breaks) error {
	_, err := tea.NewProgram(NewModel(tracker, WithCatalog(catalog), WithBreaks(breaks)), tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(now time.Time) tea.Msg { return tickMsg(now) })
}
