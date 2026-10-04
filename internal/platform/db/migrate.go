package db

import (
	"database/sql"
	"fmt"
	"os"
	"time"
)

// Migration is one transactional schema step owned by a module.
type Migration struct {
	Module  string
	Version int
	Up      func(*sql.Tx) error
}

// Migrate applies the pending migrations in slice order, each in its own transaction together
// with its schema_migrations row. If the database already holds user tables, a VACUUM INTO
// backup is written before the first pending migration and its path is returned.
func Migrate(db *sql.DB, path string, migrations []Migration, now func() time.Time) (applied int, backup string, err error) {
	last := map[string]int{}
	for _, m := range migrations {
		if m.Version <= last[m.Module] {
			return 0, "", fmt.Errorf("migration %s v%d: versions must increase per module", m.Module, m.Version)
		}
		last[m.Module] = m.Version
	}
	var userTables int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name <> 'schema_migrations'`).Scan(&userTables); err != nil {
		return 0, "", fmt.Errorf("inspect database: %w", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (module TEXT NOT NULL, version INTEGER NOT NULL, applied_at INTEGER NOT NULL, PRIMARY KEY(module, version))`); err != nil {
		return 0, "", fmt.Errorf("create schema_migrations: %w", err)
	}
	type key struct {
		module  string
		version int
	}
	done := map[key]bool{}
	rows, err := db.Query(`SELECT module, version FROM schema_migrations`)
	if err != nil {
		return 0, "", fmt.Errorf("read schema_migrations: %w", err)
	}
	for rows.Next() {
		var k key
		if err := rows.Scan(&k.module, &k.version); err != nil {
			rows.Close()
			return 0, "", fmt.Errorf("scan schema_migrations: %w", err)
		}
		done[k] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, "", fmt.Errorf("read schema_migrations: %w", err)
	}
	rows.Close()

	for _, m := range migrations {
		if done[key{m.Module, m.Version}] {
			continue
		}
		if backup == "" && userTables > 0 {
			if backup, err = backupTo(db, path, now()); err != nil {
				return applied, "", err
			}
		}
		ran, err := applyOne(db, m, now())
		if err != nil {
			return applied, backup, fmt.Errorf("migration %s v%d: %w", m.Module, m.Version, err)
		}
		if ran {
			applied++
		}
	}
	return applied, backup, nil
}

// applyOne runs m and records it atomically. The schema_migrations insert comes first so that
// a concurrent process that already applied m makes this one a no-op.
func applyOne(db *sql.DB, m Migration, now time.Time) (bool, error) {
	tx, err := db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`INSERT OR IGNORE INTO schema_migrations(module, version, applied_at) VALUES (?, ?, ?)`, m.Module, m.Version, now.Unix())
	if err != nil {
		return false, err
	}
	if n, err := result.RowsAffected(); err != nil || n == 0 {
		return false, err
	}
	if err := m.Up(tx); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func backupTo(db *sql.DB, path string, now time.Time) (string, error) {
	backup := fmt.Sprintf("%s.bak-%d", path, now.Unix())
	// VACUUM INTO accepts an empty existing file, so the backup is never wider than 0600.
	file, err := os.OpenFile(backup, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", fmt.Errorf("create backup: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("create backup: %w", err)
	}
	if _, err := db.Exec(`VACUUM INTO ?`, backup); err != nil {
		os.Remove(backup)
		return "", fmt.Errorf("write backup: %w", err)
	}
	return backup, os.Chmod(backup, 0o600)
}
