package catalog

import (
	"fmt"
	"strings"
	"time"
)

// ProjectEntries is the port through which catalog changes keep the time entries' project
// columns in sync. The composition root implements it over the tracking module.
type ProjectEntries interface {
	Relink(fromID, toID int64, toName string) error
	RenameProject(id int64, name string) error
}

// Service implements the catalog use cases. Cross-module steps run first, then the catalog row,
// so a failure leaves consistent data and the operation can be retried.
type Service struct {
	repo    *SQLite
	entries ProjectEntries
	clock   func() time.Time
}

// NewService creates a Service; a nil clock defaults to time.Now.
func NewService(repo *SQLite, entries ProjectEntries, clock func() time.Time) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{repo: repo, entries: entries, clock: clock}
}

func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ErrEmptyName
	}
	return name, nil
}

// CreateOrganization adds an organization.
func (s *Service) CreateOrganization(name string) (Organization, error) {
	name, err := cleanName(name)
	if err != nil {
		return Organization{}, err
	}
	return s.repo.CreateOrganization(name)
}

// RenameOrganization renames an organization.
func (s *Service) RenameOrganization(id int64, name string) error {
	name, err := cleanName(name)
	if err != nil {
		return err
	}
	return s.repo.exec(`UPDATE organizations SET name = ? WHERE id = ?`, name, id)
}

// DeleteOrganization removes an organization; its clients remain without organization.
func (s *Service) DeleteOrganization(id int64) error {
	org, err := s.repo.Organization(id)
	if err != nil {
		return err
	}
	clients, err := s.repo.clientsForOrganization(id)
	if err != nil {
		return err
	}
	renamed := make(map[int64]string)
	reserved := make(map[string]bool)
	for _, client := range clients {
		name := client.Name
		if s.repo.clientTaken(0, name, 0) {
			name = uniqueName(client.Name+" ("+org.Name+")", func(candidate string) bool {
				return s.repo.clientTaken(0, candidate, 0) || s.repo.clientTaken(id, candidate, client.ID) || reserved[strings.ToLower(candidate)]
			})
			renamed[client.ID] = name
		}
		reserved[strings.ToLower(name)] = true
	}
	return s.repo.deleteOrganization(id, renamed)
}

// CreateClient adds a client; organizationID 0 means no organization.
func (s *Service) CreateClient(name string, organizationID int64) (Client, error) {
	name, err := cleanName(name)
	if err != nil {
		return Client{}, err
	}
	return s.repo.CreateClient(name, organizationID)
}

// RenameClient renames a client.
func (s *Service) RenameClient(id int64, name string) error {
	name, err := cleanName(name)
	if err != nil {
		return err
	}
	client, err := s.repo.Client(id)
	if err != nil {
		return err
	}
	if s.repo.clientTaken(derefID(client.OrganizationID), name, id) {
		return ErrDuplicateName
	}
	return s.repo.exec(`UPDATE clients SET name = ? WHERE id = ?`, name, id)
}

// MoveClient moves a client to an organization; organizationID 0 means no organization.
func (s *Service) MoveClient(clientID, organizationID int64) error {
	client, err := s.repo.Client(clientID)
	if err != nil {
		return err
	}
	if s.repo.clientTaken(organizationID, client.Name, clientID) {
		return ErrDuplicateName
	}
	return s.repo.exec(`UPDATE clients SET organization_id = NULLIF(?, 0) WHERE id = ?`, organizationID, clientID)
}

// DeleteClient removes a client; its projects remain without client.
func (s *Service) DeleteClient(id int64) error {
	client, err := s.repo.Client(id)
	if err != nil {
		return err
	}
	projects, err := s.repo.projectsForClient(id)
	if err != nil {
		return err
	}
	renamed := make(map[int64]string)
	reserved := make(map[string]bool)
	for _, project := range projects {
		name := project.Name
		if s.repo.ProjectNameTaken(name, 0, 0) {
			name = uniqueName(project.Name+" ("+client.Name+")", func(candidate string) bool {
				return s.repo.ProjectNameTaken(candidate, 0, 0) || s.repo.ProjectNameTaken(candidate, id, project.ID) || reserved[strings.ToLower(candidate)]
			})
			renamed[project.ID] = name
		}
		reserved[strings.ToLower(name)] = true
	}
	// El puerto de tracking usa otra conexión, por lo que actualizamos primero el texto legado;
	// los cambios del catálogo (renombres y borrado) sí se confirman juntos en una transacción.
	for projectID, name := range renamed {
		if err := s.entries.RenameProject(projectID, name); err != nil {
			return fmt.Errorf("rename project on entries: %w", err)
		}
	}
	return s.repo.deleteClient(id, renamed)
}

// CreateProject adds a project; clientID 0 means no client.
func (s *Service) CreateProject(name string, clientID int64) (Project, error) {
	name, err := cleanName(name)
	if err != nil {
		return Project{}, err
	}
	return s.repo.CreateProject(name, clientID)
}

// RenameProject renames a project and the name shown on its entries. Use MergeProjects to
// combine two projects.
func (s *Service) RenameProject(id int64, name string) error {
	name, err := cleanName(name)
	if err != nil {
		return err
	}
	proj, err := s.repo.Project(id)
	if err != nil {
		return err
	}
	if s.repo.ProjectNameTaken(name, derefID(proj.ClientID), id) {
		return ErrDuplicateName
	}
	if err := s.entries.RenameProject(id, name); err != nil {
		return fmt.Errorf("rename project on entries: %w", err)
	}
	return s.repo.exec(`UPDATE projects SET name = ? WHERE id = ?`, name, id)
}

// MoveProject mueve un proyecto a un cliente; clientID 0 significa sin cliente.
func (s *Service) MoveProject(projectID, clientID int64) error {
	proj, err := s.repo.Project(projectID)
	if err != nil {
		return err
	}
	if clientID != 0 {
		if _, err := s.repo.Client(clientID); err != nil {
			return err
		}
	}
	if s.repo.ProjectNameTaken(proj.Name, clientID, projectID) {
		return ErrDuplicateName
	}
	return s.repo.exec(`UPDATE projects SET client_id = NULLIF(?, 0) WHERE id = ?`, clientID, projectID)
}

// ArchiveProject hides a project from pickers without losing its history.
func (s *Service) ArchiveProject(id int64) error {
	return s.repo.exec(`UPDATE projects SET archived_at = ? WHERE id = ?`, s.clock().Unix(), id)
}

// UnarchiveProject reverses ArchiveProject.
func (s *Service) UnarchiveProject(id int64) error {
	return s.repo.exec(`UPDATE projects SET archived_at = NULL WHERE id = ?`, id)
}

// DeleteProject removes a project; its entries keep their hours with no project.
func (s *Service) DeleteProject(id int64) error {
	if _, err := s.repo.Project(id); err != nil {
		return err
	}
	if err := s.entries.Relink(id, 0, ""); err != nil {
		return fmt.Errorf("unlink entries: %w", err)
	}
	return s.repo.exec(`DELETE FROM projects WHERE id = ?`, id)
}

// MergeProjects moves the entries of fromID to intoID and deletes fromID.
func (s *Service) MergeProjects(fromID, intoID int64) error {
	if fromID == intoID {
		return ErrMergeSelf
	}
	if _, err := s.repo.Project(fromID); err != nil {
		return err
	}
	into, err := s.repo.Project(intoID)
	if err != nil {
		return err
	}
	if err := s.entries.Relink(fromID, intoID, into.Name); err != nil {
		return fmt.Errorf("move entries: %w", err)
	}
	return s.repo.exec(`DELETE FROM projects WHERE id = ?`, fromID)
}

// Tree returns organizations > clients > projects, including the groups without organization
// and without client, ordered by name.
func (s *Service) Tree() ([]TreeOrganization, error) { return s.repo.Tree() }

// ProjectsNamed devuelve todos los proyectos con ese nombre (ignorando mayúsculas/minúsculas) en todos los clientes.
func (s *Service) ProjectsNamed(name string) ([]Project, error) { return s.repo.ProjectsNamed(name) }

func uniqueName(base string, taken func(string) bool) string {
	if !taken(base) {
		return base
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s %d", base, suffix)
		if !taken(candidate) {
			return candidate
		}
	}
}

func derefID(id *int64) int64 {
	if id == nil {
		return 0
	}
	return *id
}
