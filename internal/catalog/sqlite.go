package catalog

import (
	"database/sql"
	"fmt"

	"github.com/oklog/ulid/v2"

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
	}}}
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
