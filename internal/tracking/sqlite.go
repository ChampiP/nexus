package tracking

import (
	"database/sql"
	"fmt"

	"github.com/oklog/ulid/v2"

	platformdb "nexus/internal/platform/db"
)

// SQLite is the SQLite-backed Repository.
type SQLite struct{ db *sql.DB }

// NewSQLite returns a repository over a database migrated with Migrations.
func NewSQLite(db *sql.DB) *SQLite { return &SQLite{db: db} }

// Migrations returns the tracking schema steps. Version 2 references the catalog's projects
// table, so it must be applied after catalog version 1.
func Migrations() []platformdb.Migration {
	return []platformdb.Migration{
		{Module: "tracking", Version: 1, Up: func(tx *sql.Tx) error {
			// IF NOT EXISTS adopts databases created by binaries that predate migrations.
			_, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS entries (
    id INTEGER PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    project TEXT NOT NULL DEFAULT '',
    started_at INTEGER NOT NULL,
    ended_at INTEGER NULL
);
CREATE INDEX IF NOT EXISTS entries_ended_at_idx ON entries(ended_at);
CREATE INDEX IF NOT EXISTS entries_project_idx ON entries(project);`)
			return err
		}},
		{Module: "tracking", Version: 2, Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
ALTER TABLE entries ADD COLUMN uid TEXT;
ALTER TABLE entries ADD COLUMN kind TEXT NOT NULL DEFAULT 'work' CHECK(kind IN ('work','break'));
ALTER TABLE entries ADD COLUMN deleted_at INTEGER NULL;
ALTER TABLE entries ADD COLUMN project_id INTEGER NULL REFERENCES projects(id) ON DELETE SET NULL;
CREATE UNIQUE INDEX entries_uid_idx ON entries(uid) WHERE uid IS NOT NULL;
CREATE INDEX entries_project_id_idx ON entries(project_id);`)
			return err
		}},
	}
}

// Reconcile completes rows written by older binaries: it assigns a uid where missing and links
// project_id by project name. It is idempotent.
func (s *SQLite) Reconcile(resolver ProjectResolver) error {
	ids, err := s.queryInts(`SELECT id FROM entries WHERE uid IS NULL`)
	if err != nil {
		return err
	}
	if len(ids) > 0 {
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("reconcile uids: %w", err)
		}
		defer tx.Rollback()
		for _, id := range ids {
			if _, err := tx.Exec(`UPDATE entries SET uid = ? WHERE id = ? AND uid IS NULL`, ulid.Make().String(), id); err != nil {
				return fmt.Errorf("reconcile uids: %w", err)
			}
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("reconcile uids: %w", err)
		}
	}
	names, err := s.queryStrings(`SELECT DISTINCT project FROM entries WHERE project_id IS NULL AND project <> ''`)
	if err != nil {
		return err
	}
	for _, name := range names {
		id, err := resolver.EnsureProject(name)
		if err != nil {
			return fmt.Errorf("resolve project %q: %w", name, err)
		}
		if _, err := s.db.Exec(`UPDATE entries SET project_id = ? WHERE project = ? AND project_id IS NULL`, id, name); err != nil {
			return fmt.Errorf("link project %q: %w", name, err)
		}
	}
	return nil
}

func (s *SQLite) queryInts(query string) ([]int64, error) {
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("reconcile: %w", err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("reconcile: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *SQLite) queryStrings(query string) ([]string, error) {
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("reconcile: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("reconcile: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Insert persists a new entry, assigning its uid and defaulting its kind to work.
func (s *SQLite) Insert(entry Entry) (Entry, error) {
	entry.UID = ulid.Make().String()
	if entry.Kind == "" {
		entry.Kind = KindWork
	}
	result, err := s.db.Exec(`INSERT INTO entries(uid, kind, title, description, project, started_at) VALUES (?, ?, ?, ?, ?, ?)`, entry.UID, entry.Kind, entry.Title, entry.Description, entry.Project, entry.StartedAt)
	if err != nil {
		return Entry{}, fmt.Errorf("insert timer: %w", err)
	}
	entry.ID, err = result.LastInsertId()
	if err != nil {
		return Entry{}, fmt.Errorf("get timer id: %w", err)
	}
	return entry, nil
}

// Stop ends a running timer at endedAt.
func (s *SQLite) Stop(id, endedAt int64) error {
	result, err := s.db.Exec(`UPDATE entries SET ended_at = ? WHERE id = ? AND ended_at IS NULL`, endedAt, id)
	if err != nil {
		return fmt.Errorf("stop timer: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check stopped timer: %w", err)
	}
	if n == 0 {
		return ErrNotRunning
	}
	return nil
}

// StopAll ends all currently running timers at endedAt.
func (s *SQLite) StopAll(endedAt int64) (int, error) {
	result, err := s.db.Exec(`UPDATE entries SET ended_at = ? WHERE ended_at IS NULL`, endedAt)
	if err != nil {
		return 0, fmt.Errorf("stop all timers: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count stopped timers: %w", err)
	}
	return int(n), nil
}

// Running returns active timers in start order.
func (s *SQLite) Running() ([]Entry, error) {
	return s.queryEntries(`SELECT id, COALESCE(uid, ''), kind, title, description, project, started_at, ended_at FROM entries WHERE deleted_at IS NULL AND ended_at IS NULL ORDER BY started_at, id`)
}

// Recent returns the most recently started entries, newest first.
func (s *SQLite) Recent(limit int) ([]Entry, error) {
	if limit <= 0 {
		return []Entry{}, nil
	}
	return s.queryEntries(`SELECT id, COALESCE(uid, ''), kind, title, description, project, started_at, ended_at FROM entries WHERE deleted_at IS NULL ORDER BY started_at DESC, id DESC LIMIT ?`, limit)
}

// Totals returns project durations since the timestamp, clipped at now.
func (s *SQLite) Totals(since, now int64) ([]ProjectTotal, error) {
	rows, err := s.db.Query(`SELECT project, started_at, ended_at FROM entries WHERE deleted_at IS NULL AND started_at < ?`, now)
	if err != nil {
		return nil, fmt.Errorf("query totals: %w", err)
	}
	defer rows.Close()
	cutoffTotals := make(map[string]int64)
	for rows.Next() {
		var project string
		var started int64
		var ended sql.NullInt64
		if err := rows.Scan(&project, &started, &ended); err != nil {
			return nil, fmt.Errorf("scan total: %w", err)
		}
		start := started
		if start < since {
			start = since
		}
		end := now
		if ended.Valid && ended.Int64 < end {
			end = ended.Int64
		}
		if end > start {
			cutoffTotals[project] += end - start
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate totals: %w", err)
	}
	result := make([]ProjectTotal, 0, len(cutoffTotals))
	for project, seconds := range cutoffTotals {
		result = append(result, ProjectTotal{Project: project, Seconds: seconds})
	}
	return result, nil
}

// Projects returns per-project last-use times and accumulated durations.
func (s *SQLite) Projects(now int64) ([]ProjectUsage, error) {
	rows, err := s.db.Query(`SELECT project, MAX(started_at), SUM(MAX(0, MIN(COALESCE(ended_at, ?), ?) - started_at)) FROM entries WHERE deleted_at IS NULL AND project <> '' AND started_at <= ? GROUP BY project`, now, now, now)
	if err != nil {
		return nil, fmt.Errorf("query projects: %w", err)
	}
	defer rows.Close()
	var projects []ProjectUsage
	for rows.Next() {
		var project ProjectUsage
		if err := rows.Scan(&project.Name, &project.LastUsed, &project.Seconds); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate projects: %w", err)
	}
	return projects, nil
}

func (s *SQLite) queryEntries(query string, args ...any) ([]Entry, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query entries: %w", err)
	}
	defer rows.Close()
	var entries []Entry
	for rows.Next() {
		var entry Entry
		var ended sql.NullInt64
		if err := rows.Scan(&entry.ID, &entry.UID, &entry.Kind, &entry.Title, &entry.Description, &entry.Project, &entry.StartedAt, &ended); err != nil {
			return nil, fmt.Errorf("scan entry: %w", err)
		}
		if ended.Valid {
			entry.EndedAt = &ended.Int64
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate entries: %w", err)
	}
	if entries == nil {
		entries = []Entry{}
	}
	return entries, nil
}
