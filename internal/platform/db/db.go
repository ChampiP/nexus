// Package db opens and configures the Nexus SQLite database.
package db

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Open creates missing database directories (0700) and the file (0600) and opens path with WAL,
// a busy timeout and foreign keys enabled.
func Open(path string) (*sql.DB, error) {
	dir := filepath.Dir(path)
	// Directories created here are private; existing ones are left untouched because
	// NEXUS_DB may point into a directory the user shares with other files.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	if file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600); err != nil {
		return nil, fmt.Errorf("create database file: %w", err)
	} else if err := file.Close(); err != nil {
		return nil, fmt.Errorf("create database file: %w", err)
	}
	if err := secureFiles(path); err != nil {
		return nil, err
	}
	query := url.Values{}
	query.Add("_pragma", "busy_timeout(10000)")
	query.Add("_pragma", "journal_mode(WAL)")
	query.Add("_pragma", "foreign_keys(1)")
	query.Set("_txlock", "immediate")
	dsn := (&url.URL{Scheme: "file", Path: path, RawQuery: query.Encode()}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	// The WAL and shared-memory files may have been created by opening the connection.
	if err := secureFiles(path); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// secureFiles restricts the database file and any existing WAL/SHM siblings to 0600.
func secureFiles(path string) error {
	for _, name := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Chmod(name, 0o600); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("secure database file: %w", err)
		}
	}
	return nil
}
