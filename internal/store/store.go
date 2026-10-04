// Package store implements the SQLite repository for Nexus.
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
	"nexus/internal/domain"
)

// Store owns the SQLite database connection.
type Store struct{ db *sql.DB }

// Open creates the database directory, configures SQLite, and applies migrations.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode = WAL; PRAGMA busy_timeout = 5000;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("configure database: %w", err)
	}
	const schema = `
CREATE TABLE IF NOT EXISTS entries (
    id INTEGER PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    project TEXT NOT NULL DEFAULT '',
    started_at INTEGER NOT NULL,
    ended_at INTEGER NULL
);
CREATE INDEX IF NOT EXISTS entries_ended_at_idx ON entries(ended_at);
CREATE INDEX IF NOT EXISTS entries_project_idx ON entries(project);
`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	return &Store{db: db}, nil
}

// Close releases the database connection.
func (s *Store) Close() error { return s.db.Close() }

// Insert persists a new entry.
func (s *Store) Insert(entry domain.Entry) (domain.Entry, error) {
	result, err := s.db.Exec(`INSERT INTO entries(title, description, project, started_at) VALUES (?, ?, ?, ?)`, entry.Title, entry.Description, entry.Project, entry.StartedAt)
	if err != nil {
		return domain.Entry{}, fmt.Errorf("insert timer: %w", err)
	}
	entry.ID, err = result.LastInsertId()
	if err != nil {
		return domain.Entry{}, fmt.Errorf("get timer id: %w", err)
	}
	return entry, nil
}

// Stop ends a running timer at endedAt.
func (s *Store) Stop(id, endedAt int64) error {
	result, err := s.db.Exec(`UPDATE entries SET ended_at = ? WHERE id = ? AND ended_at IS NULL`, endedAt, id)
	if err != nil {
		return fmt.Errorf("stop timer: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check stopped timer: %w", err)
	}
	if n == 0 {
		return domain.ErrNotRunning
	}
	return nil
}

// StopAll ends all currently running timers at endedAt.
func (s *Store) StopAll(endedAt int64) (int, error) {
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
func (s *Store) Running() ([]domain.Entry, error) {
	return s.queryEntries(`SELECT id, title, description, project, started_at, ended_at FROM entries WHERE ended_at IS NULL ORDER BY started_at, id`)
}

// Recent returns the most recently started entries, newest first.
func (s *Store) Recent(limit int) ([]domain.Entry, error) {
	if limit <= 0 {
		return []domain.Entry{}, nil
	}
	return s.queryEntries(`SELECT id, title, description, project, started_at, ended_at FROM entries ORDER BY started_at DESC, id DESC LIMIT ?`, limit)
}

// Totals returns project durations since the timestamp, clipped at now.
func (s *Store) Totals(since, now int64) ([]domain.ProjectTotal, error) {
	rows, err := s.db.Query(`SELECT project, started_at, ended_at FROM entries WHERE started_at < ?`, now)
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
	result := make([]domain.ProjectTotal, 0, len(cutoffTotals))
	for project, seconds := range cutoffTotals {
		result = append(result, domain.ProjectTotal{Project: project, Seconds: seconds})
	}
	return result, nil
}

// Projects returns per-project last-use times and accumulated durations.
func (s *Store) Projects(now int64) ([]domain.ProjectUsage, error) {
	rows, err := s.db.Query(`SELECT project, MAX(started_at), SUM(MAX(0, MIN(COALESCE(ended_at, ?), ?) - started_at)) FROM entries WHERE project <> '' AND started_at <= ? GROUP BY project`, now, now, now)
	if err != nil {
		return nil, fmt.Errorf("query projects: %w", err)
	}
	defer rows.Close()
	var projects []domain.ProjectUsage
	for rows.Next() {
		var project domain.ProjectUsage
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

func (s *Store) queryEntries(query string, args ...any) ([]domain.Entry, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query entries: %w", err)
	}
	defer rows.Close()
	var entries []domain.Entry
	for rows.Next() {
		var entry domain.Entry
		var ended sql.NullInt64
		if err := rows.Scan(&entry.ID, &entry.Title, &entry.Description, &entry.Project, &entry.StartedAt, &ended); err != nil {
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
		entries = []domain.Entry{}
	}
	return entries, nil
}
