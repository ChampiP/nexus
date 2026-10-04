package catalog

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/oklog/ulid/v2"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	platformdb "nexus/internal/platform/db"
)

// SQLite is the SQLite-backed catalog repository.
type SQLite struct{ db *sql.DB }

// NewSQLite returns a repository over a database migrated with Migrations.
func NewSQLite(db *sql.DB) *SQLite { return &SQLite{db: db} }

// Migrations returns the catalog schema steps.
func Migrations() []platformdb.Migration {
	return []platformdb.Migration{{Module: "catalog", Version: 1, Up: func(tx *sql.Tx) error {
		_, err := tx.Exec(`
CREATE TABLE organizations (
    id INTEGER PRIMARY KEY,
    uid TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    archived_at INTEGER NULL,
    UNIQUE(name COLLATE NOCASE)
);
CREATE TABLE clients (
    id INTEGER PRIMARY KEY,
    uid TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    organization_id INTEGER NULL REFERENCES organizations(id) ON DELETE SET NULL,
    archived_at INTEGER NULL,
    UNIQUE(organization_id, name COLLATE NOCASE)
);
CREATE TABLE projects (
    id INTEGER PRIMARY KEY,
    uid TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    client_id INTEGER NULL REFERENCES clients(id) ON DELETE SET NULL,
    archived_at INTEGER NULL,
    UNIQUE(name COLLATE NOCASE)
);`)
		return err
	}}, {Module: "catalog", Version: 2, Up: migrateClientNames}}
}

func migrateClientNames(tx *sql.Tx) error {
	// Los clientes sin organización duplicados se consolidan en la fila de menor id; sus proyectos se conservan y se reasignan.
	if _, err := tx.Exec(`UPDATE projects SET client_id = (
		SELECT MIN(keep.id) FROM clients AS keep
		WHERE keep.organization_id IS NULL AND keep.name = (
			SELECT dup.name FROM clients AS dup WHERE dup.id = projects.client_id)
			COLLATE NOCASE)
		WHERE client_id IN (SELECT id FROM clients WHERE organization_id IS NULL)
		AND EXISTS (SELECT 1 FROM clients AS dup JOIN clients AS keep
			ON keep.organization_id IS NULL AND keep.name = dup.name COLLATE NOCASE
			AND keep.id < dup.id WHERE dup.id = projects.client_id)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM clients WHERE organization_id IS NULL AND id NOT IN (
		SELECT MIN(id) FROM clients WHERE organization_id IS NULL GROUP BY name COLLATE NOCASE)`); err != nil {
		return err
	}
	// Se recortan solo nombres cuyo valor normalizado no colisiona dentro de su organización.
	if _, err := tx.Exec(`UPDATE clients AS c SET name = trim(name)
		WHERE name <> trim(name)
		AND NOT EXISTS (SELECT 1 FROM clients AS other WHERE other.organization_id IS c.organization_id
			AND other.id <> c.id AND other.name = trim(c.name) COLLATE NOCASE)`); err != nil {
		return err
	}
	_, err := tx.Exec(`CREATE UNIQUE INDEX clients_null_org_name_idx ON clients(name COLLATE NOCASE) WHERE organization_id IS NULL`)
	return err
}

// EnsureProject returns the project with that name (case-insensitive), creating it if needed.
func (s *SQLite) EnsureProject(name string) (Project, error) {
	if _, err := s.db.Exec(`INSERT INTO projects(uid, name) VALUES (?, ?) ON CONFLICT DO NOTHING`, ulid.Make().String(), name); err != nil {
		return Project{}, fmt.Errorf("create project: %w", err)
	}
	var p Project
	var client, archived sql.NullInt64
	err := s.db.QueryRow(`SELECT id, uid, name, client_id, archived_at FROM projects WHERE name = ? COLLATE NOCASE`, name).Scan(&p.ID, &p.UID, &p.Name, &client, &archived)
	if err != nil {
		return Project{}, fmt.Errorf("find project: %w", err)
	}
	if client.Valid {
		p.ClientID = &client.Int64
	}
	if archived.Valid {
		p.ArchivedAt = &archived.Int64
	}
	return p, nil
}

// mapErr translates SQLite constraint failures into the module's sentinel errors.
func mapErr(err error) error {
	var se *sqlite.Error
	if errors.As(err, &se) {
		switch se.Code() {
		case sqlite3.SQLITE_CONSTRAINT_UNIQUE:
			return ErrDuplicateName
		case sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY:
			return ErrNotFound
		}
	}
	return err
}

// exec runs a single-row statement; no matching row yields ErrNotFound.
func (s *SQLite) exec(query string, args ...any) error {
	result, err := s.db.Exec(query, args...)
	if err != nil {
		return mapErr(err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLite) create(query string, args ...any) (int64, string, error) {
	uid := ulid.Make().String()
	result, err := s.db.Exec(query, append([]any{uid}, args...)...)
	if err != nil {
		return 0, "", mapErr(err)
	}
	id, err := result.LastInsertId()
	return id, uid, err
}

// CreateOrganization inserts an organization.
func (s *SQLite) CreateOrganization(name string) (Organization, error) {
	id, uid, err := s.create(`INSERT INTO organizations(uid, name) VALUES (?, ?)`, name)
	return Organization{ID: id, UID: uid, Name: name}, err
}

// CreateClient inserts a client; organizationID 0 means none.
func (s *SQLite) CreateClient(name string, organizationID int64) (Client, error) {
	if s.clientTaken(organizationID, name, 0) {
		return Client{}, ErrDuplicateName
	}
	id, uid, err := s.create(`INSERT INTO clients(uid, name, organization_id) VALUES (?, ?, NULLIF(?, 0))`, name, organizationID)
	c := Client{ID: id, UID: uid, Name: name}
	if organizationID != 0 {
		c.OrganizationID = &organizationID
	}
	return c, err
}

// CreateProject inserts a project; clientID 0 means none.
func (s *SQLite) CreateProject(name string, clientID int64) (Project, error) {
	id, uid, err := s.create(`INSERT INTO projects(uid, name, client_id) VALUES (?, ?, NULLIF(?, 0))`, name, clientID)
	p := Project{ID: id, UID: uid, Name: name}
	if clientID != 0 {
		p.ClientID = &clientID
	}
	return p, err
}

// clientTaken reports whether another client (not exceptID) already has name in the organization.
// The unique index cannot catch this for clients without organization (NULLs compare distinct).
func (s *SQLite) clientTaken(organizationID int64, name string, exceptID int64) bool {
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM clients WHERE organization_id IS NULLIF(?, 0) AND name = ? COLLATE NOCASE AND id <> ?`, organizationID, name, exceptID).Scan(&one)
	return err == nil
}

// Client returns a client by id.
func (s *SQLite) Client(id int64) (Client, error) {
	var c Client
	var org, archived sql.NullInt64
	err := s.db.QueryRow(`SELECT id, uid, name, organization_id, archived_at FROM clients WHERE id = ?`, id).Scan(&c.ID, &c.UID, &c.Name, &org, &archived)
	if errors.Is(err, sql.ErrNoRows) {
		return Client{}, ErrNotFound
	}
	if err != nil {
		return Client{}, err
	}
	if org.Valid {
		c.OrganizationID = &org.Int64
	}
	if archived.Valid {
		c.ArchivedAt = &archived.Int64
	}
	return c, nil
}

// Project returns a project by id.
func (s *SQLite) Project(id int64) (Project, error) {
	var p Project
	var client, archived sql.NullInt64
	err := s.db.QueryRow(`SELECT id, uid, name, client_id, archived_at FROM projects WHERE id = ?`, id).Scan(&p.ID, &p.UID, &p.Name, &client, &archived)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	if err != nil {
		return Project{}, err
	}
	if client.Valid {
		p.ClientID = &client.Int64
	}
	if archived.Valid {
		p.ArchivedAt = &archived.Int64
	}
	return p, nil
}

// ProjectNameTaken reports whether a project other than exceptID has name (case-insensitive).
func (s *SQLite) ProjectNameTaken(name string, exceptID int64) bool {
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM projects WHERE name = ? COLLATE NOCASE AND id <> ?`, name, exceptID).Scan(&one)
	return err == nil
}

// Tree reads the whole catalog grouped organization > client > project, ordered by name.
func (s *SQLite) Tree() ([]TreeOrganization, error) {
	orgs, err := s.db.Query(`SELECT id, uid, name, archived_at FROM organizations ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, err
	}
	defer orgs.Close()
	var tree []TreeOrganization
	orgIndex := map[int64]int{}
	for orgs.Next() {
		var o Organization
		var archived sql.NullInt64
		if err := orgs.Scan(&o.ID, &o.UID, &o.Name, &archived); err != nil {
			return nil, err
		}
		if archived.Valid {
			o.ArchivedAt = &archived.Int64
		}
		orgIndex[o.ID] = len(tree)
		tree = append(tree, TreeOrganization{Organization: &o})
	}
	if err := orgs.Err(); err != nil {
		return nil, err
	}
	noOrg := TreeOrganization{}

	clients, err := s.db.Query(`SELECT id, uid, name, organization_id, archived_at FROM clients ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, err
	}
	defer clients.Close()
	type position struct{ org, client int } // org -1 = no organization
	clientPos := map[int64]position{}
	for clients.Next() {
		var c Client
		var org, archived sql.NullInt64
		if err := clients.Scan(&c.ID, &c.UID, &c.Name, &org, &archived); err != nil {
			return nil, err
		}
		if archived.Valid {
			c.ArchivedAt = &archived.Int64
		}
		group := &noOrg
		pos := position{org: -1}
		if org.Valid {
			c.OrganizationID = &org.Int64
			pos.org = orgIndex[org.Int64]
			group = &tree[pos.org]
		}
		pos.client = len(group.Clients)
		clientPos[c.ID] = pos
		group.Clients = append(group.Clients, TreeClient{Client: &c})
	}
	if err := clients.Err(); err != nil {
		return nil, err
	}

	projects, err := s.db.Query(`SELECT id, uid, name, client_id, archived_at FROM projects ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, err
	}
	defer projects.Close()
	var noClient []Project
	for projects.Next() {
		var p Project
		var client, archived sql.NullInt64
		if err := projects.Scan(&p.ID, &p.UID, &p.Name, &client, &archived); err != nil {
			return nil, err
		}
		if archived.Valid {
			p.ArchivedAt = &archived.Int64
		}
		if !client.Valid {
			noClient = append(noClient, p)
			continue
		}
		p.ClientID = &client.Int64
		pos := clientPos[client.Int64]
		group := &noOrg
		if pos.org >= 0 {
			group = &tree[pos.org]
		}
		group.Clients[pos.client].Projects = append(group.Clients[pos.client].Projects, p)
	}
	if err := projects.Err(); err != nil {
		return nil, err
	}
	if len(noClient) > 0 {
		noOrg.Clients = append(noOrg.Clients, TreeClient{Projects: noClient})
	}
	if len(noOrg.Clients) > 0 {
		tree = append(tree, noOrg)
	}
	return tree, nil
}
