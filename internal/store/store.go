package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Entry is a single tracked interval. A nil EndedAt denotes a running timer.
type Entry struct {
	ID          int64
	Title       string
	Description string
	Project     string
	StartedAt   int64
	EndedAt     *int64
}

// ProjectTotal is the tracked duration for one project, in whole seconds.
type ProjectTotal struct {
	Project string
	Seconds int64
}

// Store owns the SQLite database and the clock used for timer transitions.
type Store struct {
	db  *sql.DB
	now func() time.Time
}

// Open creates the database directory, configures SQLite, and applies schema migrations.
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
`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	return &Store{db: db, now: time.Now}, nil
}

// Close releases the database connection.
func (s *Store) Close() error { return s.db.Close() }

// Start begins a timer. Multiple timers may run concurrently.
func (s *Store) Start(title, project, desc string) (Entry, error) {
	started := s.now().Unix()
	result, err := s.db.Exec(`INSERT INTO entries(title, description, project, started_at) VALUES (?, ?, ?, ?)`, title, desc, project, started)
	if err != nil {
		return Entry{}, fmt.Errorf("start timer: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Entry{}, fmt.Errorf("get timer id: %w", err)
	}
	return Entry{ID: id, Title: title, Description: desc, Project: project, StartedAt: started}, nil
}

// Stop ends a running timer, or returns an error if it is not running.
func (s *Store) Stop(id int64) error {
	result, err := s.db.Exec(`UPDATE entries SET ended_at = ? WHERE id = ? AND ended_at IS NULL`, s.now().Unix(), id)
	if err != nil {
		return fmt.Errorf("stop timer: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check stopped timer: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("timer #%d is not running", id)
	}
	return nil
}

// StopAll ends all currently running timers and returns the number stopped.
func (s *Store) StopAll() (int, error) {
	result, err := s.db.Exec(`UPDATE entries SET ended_at = ? WHERE ended_at IS NULL`, s.now().Unix())
	if err != nil {
		return 0, fmt.Errorf("stop all timers: %w", err)
	}
	n, err := result.RowsAffected()
	return int(n), err
}

// Running returns active timers in start order. Query errors produce an empty slice.
func (s *Store) Running() []Entry {
	entries, err := s.queryEntries(`SELECT id, title, description, project, started_at, ended_at FROM entries WHERE ended_at IS NULL ORDER BY started_at, id`)
	if err != nil {
		return []Entry{}
	}
	return entries
}

// Recent returns the most recently started entries, newest first.
func (s *Store) Recent(limit int) []Entry {
	if limit <= 0 {
		return []Entry{}
	}
	entries, err := s.queryEntries(`SELECT id, title, description, project, started_at, ended_at FROM entries ORDER BY started_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return []Entry{}
	}
	return entries
}

// Totals returns project durations since the given time, clipping intervals at since and now.
func (s *Store) Totals(since time.Time) []ProjectTotal {
	rows, err := s.db.Query(`SELECT project, started_at, ended_at FROM entries WHERE started_at <= ?`, s.now().Unix())
	if err != nil {
		return []ProjectTotal{}
	}
	defer rows.Close()
	cutoff, now := since.Unix(), s.now().Unix()
	totals := make(map[string]int64)
	for rows.Next() {
		var project string
		var started int64
		var ended sql.NullInt64
		if rows.Scan(&project, &started, &ended) != nil {
			return []ProjectTotal{}
		}
		start := started
		if start < cutoff {
			start = cutoff
		}
		end := now
		if ended.Valid && ended.Int64 < end {
			end = ended.Int64
		}
		if end > start {
			totals[project] += end - start
		} else if !ended.Valid && start == end {
			// Retain a currently running project's zero-second total.
			totals[project] += 0
		}
	}
	if rows.Err() != nil {
		return []ProjectTotal{}
	}
	result := make([]ProjectTotal, 0, len(totals))
	for project, seconds := range totals {
		result = append(result, ProjectTotal{Project: project, Seconds: seconds})
	}
	return result
}

func (s *Store) queryEntries(query string, args ...any) ([]Entry, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []Entry
	for rows.Next() {
		var entry Entry
		var ended sql.NullInt64
		if err := rows.Scan(&entry.ID, &entry.Title, &entry.Description, &entry.Project, &entry.StartedAt, &ended); err != nil {
			return nil, err
		}
		if ended.Valid {
			entry.EndedAt = &ended.Int64
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}
