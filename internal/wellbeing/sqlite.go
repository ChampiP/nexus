package wellbeing

import (
	"database/sql"
	"fmt"

	platformdb "nexus/internal/platform/db"
)

type SQLite struct{ db *sql.DB }

func NewSQLite(db *sql.DB) *SQLite { return &SQLite{db: db} }
func Migrations() []platformdb.Migration {
	return []platformdb.Migration{{Module: "wellbeing", Version: 1, Up: func(tx *sql.Tx) error {
		_, err := tx.Exec(`CREATE TABLE wellbeing_settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`)
		return err
	}}}
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
	if _, err := tx.Exec(`DELETE FROM wellbeing_settings`); err != nil {
		return err
	}
	for k, x := range v {
		if _, err := tx.Exec(`INSERT INTO wellbeing_settings(key,value) VALUES(?,?)`, k, x); err != nil {
			return err
		}
	}
	return tx.Commit()
}
