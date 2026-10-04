// Package countdown is the break-countdown module: entities, use cases, ports and the SQLite adapter.
package countdown

import (
	"errors"
	"time"
)

// KindBreak es el único tipo de cuenta regresiva por ahora.
const KindBreak = "break"

// Break es una cuenta regresiva con fecha límite. Un FinishedAt nulo indica que sigue activa.
type Break struct {
	ID             int64
	UID            string
	Label          string
	StartedAt      int64
	EndsAt         int64
	FinishedAt     *int64
	EntryID        int64   // entrada de tracking kind=break asociada
	ResumeEntryIDs []int64 // timers detenidos al empezar, para retomarlos al volver
	NotifiedAt     *int64
}

// Remaining devuelve el tiempo que falta hasta la fecha límite, nunca negativo.
func (b Break) Remaining(now time.Time) time.Duration {
	if d := time.Unix(b.EndsAt, 0).Sub(now); d > 0 {
		return d
	}
	return 0
}

// Overdue devuelve cuánto se pasó de la fecha límite, nunca negativo.
func (b Break) Overdue(now time.Time) time.Duration {
	if d := now.Sub(time.Unix(b.EndsAt, 0)); d > 0 {
		return d
	}
	return 0
}

var (
	// ErrBreakActive indica que ya hay un break en curso.
	ErrBreakActive = errors.New("a break is already active")
	// ErrNoActiveBreak indica que no hay break en curso.
	ErrNoActiveBreak = errors.New("no active break")
	// ErrInvalidDuration indica una duración fuera del rango permitido.
	ErrInvalidDuration = errors.New("break duration must be between 1 minute and 8 hours")
)
