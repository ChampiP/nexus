package wellbeing

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	platformdb "nexus/internal/platform/db"
)

type SQLite struct{ db *sql.DB }

func NewSQLite(db *sql.DB) *SQLite { return &SQLite{db: db} }
func Migrations() []platformdb.Migration {
	return []platformdb.Migration{
		{Module: "wellbeing", Version: 1, Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(`CREATE TABLE wellbeing_settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`)
			return err
		}},
		{Module: "wellbeing", Version: 2, Up: migrateEvents},
	}
}

func migrateEvents(tx *sql.Tx) error {
	if _, err := tx.Exec(`CREATE TABLE wellbeing_events (
		id INTEGER PRIMARY KEY,
		at INTEGER NOT NULL,
		event TEXT NOT NULL CHECK(event IN ('shown','done','skipped','snoozed')),
		duration_seconds INTEGER NOT NULL DEFAULT 0
	);
	CREATE INDEX wellbeing_events_at_idx ON wellbeing_events(at);`); err != nil {
		return err
	}
	rows, err := tx.Query(`SELECT key, value FROM wellbeing_settings WHERE key LIKE '%:%'`)
	if err != nil {
		return err
	}
	type legacyCounter struct{ key, value string }
	var counters []legacyCounter
	for rows.Next() {
		var counter legacyCounter
		if err := rows.Scan(&counter.key, &counter.value); err != nil {
			rows.Close()
			return err
		}
		counters = append(counters, counter)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, counter := range counters {
		key, day, ok := strings.Cut(counter.key, ":")
		event := key
		if !ok || (event != "shown" && event != "done" && event != "skipped") {
			continue
		}
		date, err := time.ParseInLocation("2006-01-02", day, time.Local)
		if err != nil {
			continue
		}
		n, err := strconv.Atoi(counter.value)
		if err == nil && n > 0 {
			// Los contadores antiguos solo guardaban cantidades; cada unidad se ubica a mediodía local.
			at := time.Date(date.Year(), date.Month(), date.Day(), 12, 0, 0, 0, time.Local).Unix()
			for i := 0; i < n; i++ {
				if _, err := tx.Exec(`INSERT INTO wellbeing_events(at,event) VALUES(?,?)`, at, event); err != nil {
					return err
				}
			}
		}
		if _, err := tx.Exec(`DELETE FROM wellbeing_settings WHERE key = ?`, counter.key); err != nil {
			return err
		}
	}
	return nil
}
func (s *SQLite) Load() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT key,value FROM wellbeing_settings`)
	if err != nil {
		return nil, fmt.Errorf("load wellbeing settings: %w", err)
	}
	defer rows.Close()
	v := map[string]string{}
	for rows.Next() {
		var k, x string
		if err := rows.Scan(&k, &x); err != nil {
			return nil, err
		}
		v[k] = x
	}
	return v, rows.Err()
}
func (s *SQLite) Save(v map[string]string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := saveChanged(tx, v); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLite) SaveEvent(v map[string]string, event string, at int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := saveChanged(tx, v); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO wellbeing_events(at,event) VALUES(?,?)`, at, event); err != nil {
		return fmt.Errorf("save wellbeing event: %w", err)
	}
	return tx.Commit()
}

func saveChanged(tx *sql.Tx, values map[string]string) error {
	for key, value := range values {
		if _, err := tx.Exec(`INSERT INTO wellbeing_settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLite) Delete(key string) error {
	_, err := s.db.Exec(`DELETE FROM wellbeing_settings WHERE key = ?`, key)
	return err
}

func obsoleteSaveChanged(tx *sql.Tx, values map[string]string) error {
	rows, err := tx.Query(`SELECT key,value FROM wellbeing_settings`)
	if err != nil {
		return err
	}
	current := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			rows.Close()
			return err
		}
		current[key] = value
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for key, value := range values {
		if old, ok := current[key]; !ok || old != value {
			if _, err := tx.Exec(`INSERT INTO wellbeing_settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value); err != nil {
				return err
			}
		}
		delete(current, key)
	}
	for key := range current {
		if _, err := tx.Exec(`DELETE FROM wellbeing_settings WHERE key=?`, key); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLite) Counters(start, end int64) (Counters, error) {
	var result Counters
	rows, err := s.db.Query(`SELECT event, COUNT(*) FROM wellbeing_events WHERE at >= ? AND at < ? GROUP BY event`, start, end)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var event string
		var count int
		if err := rows.Scan(&event, &count); err != nil {
			return result, err
		}
		switch event {
		case "shown":
			result.Shown = count
		case "done":
			result.Done = count
		case "skipped":
			result.Skipped = count
		case "snoozed":
			result.Snoozed = count
		}
	}
	return result, rows.Err()
}
