package tracking

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/oklog/ulid/v2"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

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
		{Module: "tracking", Version: 3, Up: migrateTaskUID},
		{Module: "tracking", Version: 4, Up: migrateIntegrity},
	}
}

func migrateIntegrity(tx *sql.Tx) error {
	// Las tareas con varias sesiones abiertas se normalizan conservando abierta la más reciente.
	if _, err := tx.Exec(`UPDATE entries SET ended_at = (
		SELECT newest.started_at FROM entries AS newest
		WHERE newest.task_uid = entries.task_uid AND newest.ended_at IS NULL AND newest.deleted_at IS NULL
		ORDER BY newest.started_at DESC, newest.id DESC LIMIT 1)
		WHERE ended_at IS NULL AND deleted_at IS NULL AND id <> (
		SELECT newest.id FROM entries AS newest
		WHERE newest.task_uid = entries.task_uid AND newest.ended_at IS NULL AND newest.deleted_at IS NULL
		ORDER BY newest.started_at DESC, newest.id DESC LIMIT 1)`); err != nil {
		return err
	}
	_, err := tx.Exec(`
CREATE INDEX entries_started_at_idx ON entries(started_at);
CREATE INDEX entries_project_started_idx ON entries(project_id, started_at) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX entries_one_running_per_task_idx ON entries(task_uid) WHERE ended_at IS NULL AND deleted_at IS NULL;
CREATE TRIGGER entries_ended_after_started_insert BEFORE INSERT ON entries
WHEN NEW.ended_at IS NOT NULL AND NEW.ended_at < NEW.started_at
BEGIN SELECT RAISE(ABORT, 'ended_at precedes started_at'); END;
CREATE TRIGGER entries_ended_after_started_update BEFORE UPDATE OF started_at, ended_at ON entries
WHEN NEW.ended_at IS NOT NULL AND NEW.ended_at < NEW.started_at
BEGIN SELECT RAISE(ABORT, 'ended_at precedes started_at'); END;`)
	return err
}

// migrateTaskUID agrega task_uid y agrupa las sesiones idénticas existentes en una sola tarea.
// Es aditiva: no borra ni reescribe ninguna otra columna.
func migrateTaskUID(tx *sql.Tx) error {
	if _, err := tx.Exec(`ALTER TABLE entries ADD COLUMN task_uid TEXT; CREATE INDEX entries_task_uid_idx ON entries(task_uid);`); err != nil {
		return err
	}
	// Las filas de la fase 1 aún no tienen uid; se les asigna antes de agrupar.
	rows, err := tx.Query(`SELECT id FROM entries WHERE uid IS NULL`)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, id := range ids {
		if _, err := tx.Exec(`UPDATE entries SET uid = ? WHERE id = ?`, ulid.Make().String(), id); err != nil {
			return err
		}
	}
	// Cada fila es su propia tarea; luego las sesiones de trabajo vivas e idénticas
	// pasan a la tarea de la más antigua del grupo.
	_, err = tx.Exec(`
UPDATE entries SET task_uid = uid;
UPDATE entries SET task_uid = (
    SELECT e2.uid FROM entries e2
    WHERE e2.kind = 'work' AND e2.deleted_at IS NULL
      AND e2.title = entries.title AND e2.project = entries.project AND e2.description = entries.description
    ORDER BY e2.started_at, e2.id LIMIT 1)
WHERE kind = 'work' AND deleted_at IS NULL;`)
	return err
}

// Reconcile completes rows written by older binaries: it assigns a uid where missing and links
// task_uid (una tarea propia) y links project_id by project name. It is idempotent.
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
	if _, err := s.db.Exec(`UPDATE entries SET task_uid = uid WHERE task_uid IS NULL`); err != nil {
		return fmt.Errorf("reconcile task uids: %w", err)
	}
	names, err := s.queryStrings(`SELECT DISTINCT project FROM entries WHERE project_id IS NULL AND project <> ''`)
	if err != nil {
		return err
	}
	for _, name := range names {
		id, err := resolver.EnsureProject(name)
		if err != nil {
			if errors.Is(err, ErrAmbiguousProject) {
				continue
			}
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
	if entry.TaskUID == "" {
		entry.TaskUID = entry.UID
	}
	result, err := s.db.Exec(`INSERT INTO entries(uid, task_uid, kind, title, description, project, project_id, started_at) VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, 0), ?)`, entry.UID, entry.TaskUID, entry.Kind, entry.Title, entry.Description, entry.Project, entry.ProjectID, entry.StartedAt)
	if err != nil {
		var sqliteErr *sqlite.Error
		if errors.As(err, &sqliteErr) && sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE {
			return Entry{}, ErrAlreadyRunning
		}
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
	return s.queryEntries(`SELECT ` + entryColumns + ` FROM entries WHERE deleted_at IS NULL AND ended_at IS NULL AND kind = 'work' ORDER BY started_at, id`)
}

// Recent returns the most recently started entries, newest first.
func (s *SQLite) Recent(limit int) ([]Entry, error) {
	if limit <= 0 {
		return []Entry{}, nil
	}
	return s.queryEntries(`SELECT `+entryColumns+` FROM entries WHERE deleted_at IS NULL ORDER BY started_at DESC, id DESC LIMIT ?`, limit)
}

// Totals returns project durations since the timestamp, clipped at now.
func (s *SQLite) Totals(since, now int64) ([]ProjectTotal, error) {
	rows, err := s.db.Query(`SELECT COALESCE(project_id, 0), project, started_at, ended_at FROM entries WHERE deleted_at IS NULL AND kind = 'work' AND started_at < ?`, now)
	if err != nil {
		return nil, fmt.Errorf("query totals: %w", err)
	}
	defer rows.Close()
	type totalKey struct {
		id   int64
		text string
	}
	type totalVal struct {
		project   string
		projectID int64
		seconds   int64
		lastUsed  int64
	}
	cutoffTotals := make(map[totalKey]*totalVal)
	for rows.Next() {
		var projectID int64
		var project string
		var started int64
		var ended sql.NullInt64
		if err := rows.Scan(&projectID, &project, &started, &ended); err != nil {
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
			var key totalKey
			if projectID > 0 {
				key = totalKey{id: projectID}
			} else {
				key = totalKey{text: project}
			}
			val, ok := cutoffTotals[key]
			if !ok {
				val = &totalVal{project: project, projectID: projectID}
				cutoffTotals[key] = val
			}
			if started >= val.lastUsed && project != "" {
				val.project = project
				val.lastUsed = started
			}
			val.seconds += end - start
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate totals: %w", err)
	}
	result := make([]ProjectTotal, 0, len(cutoffTotals))
	for _, val := range cutoffTotals {
		result = append(result, ProjectTotal{Project: val.project, ProjectID: val.projectID, Seconds: val.seconds})
	}
	return result, nil
}

// Projects returns per-project last-use times and accumulated durations.
func (s *SQLite) Projects(now int64) ([]ProjectUsage, error) {
	rows, err := s.db.Query(`SELECT COALESCE(project_id, 0), project, MAX(started_at), SUM(MAX(0, MIN(COALESCE(ended_at, ?), ?) - started_at)) FROM entries WHERE deleted_at IS NULL AND kind = 'work' AND (project_id IS NOT NULL OR project <> '') AND started_at <= ? GROUP BY CASE WHEN project_id IS NOT NULL THEN project_id ELSE 0 END, CASE WHEN project_id IS NULL THEN project ELSE '' END`, now, now, now)
	if err != nil {
		return nil, fmt.Errorf("query projects: %w", err)
	}
	defer rows.Close()
	var projects []ProjectUsage
	for rows.Next() {
		var project ProjectUsage
		if err := rows.Scan(&project.ProjectID, &project.Name, &project.LastUsed, &project.Seconds); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate projects: %w", err)
	}
	return projects, nil
}

// BreakSeconds suma los intervalos de break solapados con [since, now].
func (s *SQLite) BreakSeconds(since, now int64) (int64, error) {
	var total int64
	err := s.db.QueryRow(`SELECT COALESCE(SUM(MAX(0, MIN(COALESCE(ended_at, ?), ?) - MAX(started_at, ?))), 0) FROM entries WHERE deleted_at IS NULL AND kind = 'break'`, now, now, since).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("query break seconds: %w", err)
	}
	return total, nil
}

// BreaksSince devuelve los breaks no eliminados iniciados en o después de since, en orden de inicio.
func (s *SQLite) BreaksSince(since int64) ([]Entry, error) {
	return s.queryEntries(`SELECT `+entryColumns+` FROM entries WHERE deleted_at IS NULL AND kind = 'break' AND started_at >= ? ORDER BY started_at, id`, since)
}

const entryColumns = `id, COALESCE(uid, ''), COALESCE(task_uid, uid, ''), kind, title, description, project, COALESCE(project_id, 0), started_at, ended_at, deleted_at`

// Get returns a live (not deleted) entry or ErrNotFound.
func (s *SQLite) Get(id int64) (Entry, error) {
	entries, err := s.queryEntries(`SELECT `+entryColumns+` FROM entries WHERE id = ? AND deleted_at IS NULL`, id)
	if err != nil {
		return Entry{}, err
	}
	if len(entries) == 0 {
		return Entry{}, ErrNotFound
	}
	return entries[0], nil
}

// Update rewrites the editable fields of a live entry.
func (s *SQLite) Update(entry Entry) error {
	result, err := s.db.Exec(`UPDATE entries SET title = ?, description = ?, project = ?, project_id = NULLIF(?, 0) WHERE id = ? AND deleted_at IS NULL`, entry.Title, entry.Description, entry.Project, entry.ProjectID, entry.ID)
	return expectRow(result, err, "update entry")
}

// SoftDelete marks a live entry deleted, ending it at `at` if it was running.
func (s *SQLite) SoftDelete(id, at int64) error {
	result, err := s.db.Exec(`UPDATE entries SET ended_at = COALESCE(ended_at, ?), deleted_at = ? WHERE id = ? AND deleted_at IS NULL`, at, at, id)
	return expectRow(result, err, "delete entry")
}

// Restore clears the deletion mark of a deleted entry.
func (s *SQLite) Restore(id, resumeSince int64) error {
	// ended_at = deleted_at marca una tarea que estaba en curso cuando se borró.
	result, err := s.db.Exec(`UPDATE entries SET
		ended_at = CASE WHEN ended_at = deleted_at AND deleted_at >= ? THEN NULL ELSE ended_at END,
		deleted_at = NULL
		WHERE id = ? AND deleted_at IS NOT NULL`, resumeSince, id)
	return expectRow(result, err, "restore entry")
}

// Deleted returns soft-deleted entries, most recently deleted first.
func (s *SQLite) Deleted(limit int) ([]Entry, error) {
	if limit <= 0 {
		return []Entry{}, nil
	}
	return s.queryEntries(`SELECT `+entryColumns+` FROM entries WHERE deleted_at IS NOT NULL ORDER BY deleted_at DESC, id DESC LIMIT ?`, limit)
}

// Purge hard-deletes entries whose deleted_at is before the timestamp.
func (s *SQLite) Purge(before int64) (int, error) {
	result, err := s.db.Exec(`DELETE FROM entries WHERE deleted_at IS NOT NULL AND deleted_at < ?`, before)
	if err != nil {
		return 0, fmt.Errorf("purge entries: %w", err)
	}
	n, err := result.RowsAffected()
	return int(n), err
}

// Relink moves the entries of project fromID to toID (0 clears the link) with the given name.
func (s *SQLite) Relink(fromID, toID int64, toName string) error {
	if _, err := s.db.Exec(`UPDATE entries SET project_id = NULLIF(?, 0), project = ? WHERE project_id = ?`, toID, toName, fromID); err != nil {
		return fmt.Errorf("relink entries: %w", err)
	}
	return nil
}

// RenameProject sets the project name text on the entries of project id.
func (s *SQLite) RenameProject(id int64, name string) error {
	if _, err := s.db.Exec(`UPDATE entries SET project = ? WHERE project_id = ?`, name, id); err != nil {
		return fmt.Errorf("rename project on entries: %w", err)
	}
	return nil
}

func expectRow(result sql.Result, err error, what string) error {
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	if n, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("%s: %w", what, err)
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}

// CountByProject cuenta las tareas vivas (no borradas) de un proyecto del catálogo.
func (s *SQLite) CountByProject(projectID int64) (int, error) {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM entries WHERE project_id = ? AND deleted_at IS NULL`, projectID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count project entries: %w", err)
	}
	return n, nil
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
		var ended, deleted sql.NullInt64
		if err := rows.Scan(&entry.ID, &entry.UID, &entry.TaskUID, &entry.Kind, &entry.Title, &entry.Description, &entry.Project, &entry.ProjectID, &entry.StartedAt, &ended, &deleted); err != nil {
			return nil, fmt.Errorf("scan entry: %w", err)
		}
		if ended.Valid {
			entry.EndedAt = &ended.Int64
		}
		if deleted.Valid {
			entry.DeletedAt = &deleted.Int64
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
