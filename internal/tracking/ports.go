package tracking

import "time"

// Repository provides persistence operations required by Tracker.
type Repository interface {
	Insert(Entry) (Entry, error)
	Stop(id, endedAt int64) error
	StopAll(endedAt int64) (int, error)
	// Running devuelve solo los timers de trabajo en curso; los breaks quedan fuera.
	Running() ([]Entry, error)
	Recent(limit int) ([]Entry, error)
	// Totals y Projects cuentan solo entradas de trabajo.
	Totals(since, now int64) ([]ProjectTotal, error)
	Projects(now int64) ([]ProjectUsage, error)
	// BreakSeconds suma la duración de los breaks desde since, recortada en now.
	BreakSeconds(since, now int64) (int64, error)
	// BreaksSince devuelve los breaks no eliminados iniciados desde since, en orden de inicio.
	BreaksSince(since int64) ([]Entry, error)
	Get(id int64) (Entry, error)
	Update(Entry) error
	SoftDelete(id, at int64) error
	// Restore saca la tarea de la papelera; si estaba en curso al borrarse y el borrado
	// ocurrió en o después de resumeSince, vuelve a quedar en curso.
	Restore(id, resumeSince int64) error
	Deleted(limit int) ([]Entry, error)
	Purge(before int64) (int, error)
	Relink(fromID, toID int64, toName string) error
	RenameProject(id int64, name string) error
	CountByProject(projectID int64) (int, error)
	// RecentTasks agrupa las sesiones de trabajo no eliminadas por tarea, la más reciente primero.
	RecentTasks(now int64, limit int) ([]TaskSummary, error)
	// TaskTotal suma los segundos de las sesiones no eliminadas de la tarea, hasta now.
	TaskTotal(taskUID string, now int64) (int64, error)
	// RunningInTask devuelve la sesión en curso de la tarea, si hay una.
	RunningInTask(taskUID string) (Entry, bool, error)
	// UpdateTask reescribe título, descripción y proyecto de todas las sesiones vivas de la tarea.
	UpdateTask(taskUID string, entry Entry) error
	// SoftDeleteTask elimina todas las sesiones vivas de la tarea, deteniendo la que esté en curso.
	SoftDeleteTask(taskUID string, at int64) error
	// RestoreTask restaura las sesiones eliminadas junto en el último borrado de la tarea.
	RestoreTask(taskUID string, resumeSince int64) error
}

// Clock supplies the current time to Tracker.
type Clock func() time.Time

// ProjectResolver maps a project name to the catalog's project id, creating the project if needed.
type ProjectResolver interface {
	EnsureProject(name string) (id int64, err error)
}

// ProjectNameResolver obtiene el nombre del catálogo correspondiente a un id de proyecto.
type ProjectNameResolver interface {
	ProjectName(id int64) (string, error)
}
