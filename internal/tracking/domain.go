// Package tracking is the time-tracking module: entities, use cases, ports and the SQLite adapter.
package tracking

import "errors"

// KindWork is the default entry kind; KindBreak marks a break.
const (
	KindWork  = "work"
	KindBreak = "break"
)

// Entry is a tracked interval. A nil EndedAt denotes a running timer.
type Entry struct {
	ID          int64
	UID         string
	TaskUID     string // agrupa las sesiones de una misma tarea; vale UID cuando es la primera
	Kind        string
	Title       string
	Description string
	Project     string
	ProjectID   int64 // 0 means no catalog project (NULL)
	StartedAt   int64
	EndedAt     *int64
	DeletedAt   *int64 // nil si la tarea no está en la papelera
}

// TaskSummary agrupa las sesiones (entradas) de una tarea tal como la ve el usuario.
type TaskSummary struct {
	TaskUID     string
	Title       string
	Project     string
	ProjectID   int64
	Description string
	// TotalSeconds suma las sesiones no eliminadas; la sesión en curso cuenta hasta ahora.
	TotalSeconds   int64
	Running        bool
	RunningEntryID int64 // 0 si no hay sesión en curso
	LastEntryID    int64 // la sesión más reciente
	LastActivity   int64 // fin de la última sesión, o ahora si hay una en curso
	SessionCount   int
}

// ProjectTotal is the tracked duration for one project, in whole seconds.
type ProjectTotal struct {
	Project string
	Seconds int64
}

// ProjectUsage summarizes the most recent use and total duration of a project.
type ProjectUsage struct {
	Name     string
	LastUsed int64
	Seconds  int64
}

var (
	// ErrNotRunning indicates that a timer does not exist or is already stopped.
	ErrNotRunning = errors.New("timer is not running")
	// ErrNotFound indicates that an entry does not exist or is in the wrong deleted state.
	ErrNotFound = errors.New("entry not found")
	// ErrEmptyTitle indicates that a timer title is blank after trimming.
	ErrEmptyTitle = errors.New("title cannot be empty")
	// ErrAlreadyRunning indica que la tarea ya tiene una sesión en curso.
	ErrAlreadyRunning = errors.New("task already has a running session")
	// ErrInvalidKind indica que el tipo de entrada no es "work" ni "break".
	ErrInvalidKind = errors.New("invalid entry kind")
)
