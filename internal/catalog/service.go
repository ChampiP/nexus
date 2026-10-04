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
	return s.repo.exec(`DELETE FROM organizations WHERE id = ?`, id)
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
	return s.repo.exec(`DELETE FROM clients WHERE id = ?`, id)
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
	if _, err := s.repo.Project(id); err != nil {
		return err
	}
	if s.repo.ProjectNameTaken(name, id) {
		return ErrDuplicateName
	}
	if err := s.entries.RenameProject(id, name); err != nil {
		return fmt.Errorf("rename project on entries: %w", err)
	}
	return s.repo.exec(`UPDATE projects SET name = ? WHERE id = ?`, name, id)
}

// MoveProject moves a project to a client; clientID 0 means no client.
func (s *Service) MoveProject(projectID, clientID int64) error {
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

func derefID(id *int64) int64 {
	if id == nil {
		return 0
	}
	return *id
}
