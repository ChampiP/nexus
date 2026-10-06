package countdown

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/oklog/ulid/v2"

	platformdb "nexus/internal/platform/db"
)

// SQLite es el Repository respaldado por SQLite.
type SQLite struct{ db *sql.DB }

// NewSQLite devuelve un repositorio sobre una base migrada con Migrations.
func NewSQLite(db *sql.DB) *SQLite { return &SQLite{db: db} }

// Migrations devuelve los pasos de esquema de countdown. No referencia otras tablas.
func Migrations() []platformdb.Migration {
	return []platformdb.Migration{
		{Module: "countdown", Version: 1, Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
CREATE TABLE countdowns (
    id INTEGER PRIMARY KEY,
    uid TEXT UNIQUE NOT NULL,
    kind TEXT NOT NULL DEFAULT 'break',
    label TEXT NOT NULL DEFAULT '',
    started_at INTEGER NOT NULL,
    ends_at INTEGER NOT NULL,
    finished_at INTEGER NULL,
    entry_id INTEGER NULL,
    resume_entry_ids TEXT NOT NULL DEFAULT '[]',
    notified_at INTEGER NULL
);
CREATE INDEX countdowns_finished_at_idx ON countdowns(finished_at);`)
			return err
		}},
		{Module: "countdown", Version: 2, Up: migrateIntegrity},
	}
}

func migrateIntegrity(tx *sql.Tx) error {
	// Se termina a todas las cuentas activas salvo la más reciente; también se corrigen intervalos invertidos y tipos desconocidos antes de validar.
	if _, err := tx.Exec(`UPDATE countdowns SET finished_at = ends_at
		WHERE finished_at IS NULL AND id NOT IN (SELECT id FROM countdowns
			WHERE finished_at IS NULL ORDER BY started_at DESC, id DESC LIMIT 1);
		UPDATE countdowns SET ends_at = started_at WHERE ends_at < started_at;
		UPDATE countdowns SET kind = 'break' WHERE kind NOT IN ('break','focus','custom');`); err != nil {
		return err
	}
	// La tabla hija puede reemplazarse con claves foráneas activas: DROP de la tabla hija no invalida sus padres;
	// PRAGMA foreign_keys no es modificable dentro de la transacción que Migrate abre por cada paso.
	if _, err := tx.Exec(`
CREATE TABLE countdowns_new (
    id INTEGER PRIMARY KEY,
    uid TEXT UNIQUE NOT NULL,
    kind TEXT NOT NULL DEFAULT 'break' CHECK(kind IN ('break','focus','custom')),
    label TEXT NOT NULL DEFAULT '',
    started_at INTEGER NOT NULL,
    ends_at INTEGER NOT NULL CHECK(ends_at >= started_at),
    finished_at INTEGER NULL,
    entry_id INTEGER NULL REFERENCES entries(id) ON DELETE SET NULL,
    resume_entry_ids TEXT NOT NULL DEFAULT '[]',
    notified_at INTEGER NULL
);
INSERT INTO countdowns_new(id, uid, kind, label, started_at, ends_at, finished_at, entry_id, resume_entry_ids, notified_at)
-- Se conserva la referencia solo si la entrada todavía existe.
SELECT id, uid, kind, label, started_at, ends_at, finished_at,
	CASE WHEN EXISTS (SELECT 1 FROM entries e WHERE e.id = countdowns.entry_id) THEN entry_id END,
	resume_entry_ids, notified_at FROM countdowns;
DROP TABLE countdowns;
ALTER TABLE countdowns_new RENAME TO countdowns;
CREATE INDEX countdowns_finished_at_idx ON countdowns(finished_at);
CREATE INDEX countdowns_active_idx ON countdowns(started_at DESC, id DESC) WHERE finished_at IS NULL;
CREATE UNIQUE INDEX countdowns_one_active_idx ON countdowns((1)) WHERE finished_at IS NULL;`); err != nil {
		return err
	}
	return nil
}

// Insert guarda un break nuevo y le asigna id y uid.
func (s *SQLite) Insert(b Break) (Break, error) {
	ids, err := json.Marshal(append([]int64{}, b.ResumeEntryIDs...))
	if err != nil {
		return Break{}, fmt.Errorf("encode resume ids: %w", err)
	}
	b.UID = ulid.Make().String()
	result, err := s.db.Exec(`INSERT INTO countdowns(uid, kind, label, started_at, ends_at, entry_id, resume_entry_ids) VALUES (?, ?, ?, ?, ?, NULLIF(?, 0), ?)`,
		b.UID, KindBreak, b.Label, b.StartedAt, b.EndsAt, b.EntryID, string(ids))
	if err != nil {
		return Break{}, fmt.Errorf("insert countdown: %w", err)
	}
	if b.ID, err = result.LastInsertId(); err != nil {
		return Break{}, fmt.Errorf("get countdown id: %w", err)
	}
	return b, nil
}

// Active devuelve el break sin terminar, o nil.
func (s *SQLite) Active() (*Break, error) {
	var b Break
	var finished, notified sql.NullInt64
	var ids string
	err := s.db.QueryRow(`SELECT id, uid, label, started_at, ends_at, finished_at, COALESCE(entry_id, 0), resume_entry_ids, notified_at FROM countdowns WHERE finished_at IS NULL ORDER BY id DESC LIMIT 1`).
		Scan(&b.ID, &b.UID, &b.Label, &b.StartedAt, &b.EndsAt, &finished, &b.EntryID, &ids, &notified)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query active countdown: %w", err)
	}
	if strings.TrimSpace(ids) != "" {
		if err := json.Unmarshal([]byte(ids), &b.ResumeEntryIDs); err != nil {
			return nil, fmt.Errorf("decode resume ids: %w", err)
		}
	}
	if finished.Valid {
		b.FinishedAt = &finished.Int64
	}
	if notified.Valid {
		b.NotifiedAt = &notified.Int64
	}
	return &b, nil
}

// Finish marca el break como terminado en at.
func (s *SQLite) Finish(id, at int64) error {
	return s.exec(`UPDATE countdowns SET finished_at = ? WHERE id = ? AND finished_at IS NULL`, at, id)
}

// SetEndsAt mueve la fecha límite y limpia el último aviso.
func (s *SQLite) SetEndsAt(id, endsAt int64) error {
	return s.exec(`UPDATE countdowns SET ends_at = ?, notified_at = NULL WHERE id = ? AND finished_at IS NULL`, endsAt, id)
}

// MarkNotified registra el momento del último aviso.
func (s *SQLite) MarkNotified(id, at int64) error {
	return s.exec(`UPDATE countdowns SET notified_at = ? WHERE id = ? AND finished_at IS NULL`, at, id)
}

func (s *SQLite) exec(query string, args ...any) error {
	result, err := s.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("update countdown: %w", err)
	}
	if n, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("update countdown: %w", err)
	} else if n == 0 {
		return ErrNoActiveBreak
	}
	return nil
}
