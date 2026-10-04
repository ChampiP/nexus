package tracking

import "time"

// Repository provides persistence operations required by Tracker.
type Repository interface {
	Insert(Entry) (Entry, error)
	Stop(id, endedAt int64) error
	StopAll(endedAt int64) (int, error)
	Running() ([]Entry, error)
	Recent(limit int) ([]Entry, error)
	Totals(since, now int64) ([]ProjectTotal, error)
	Projects(now int64) ([]ProjectUsage, error)
	Get(id int64) (Entry, error)
	Update(Entry) error
	SoftDelete(id, at int64) error
	Restore(id int64) error
	Deleted(limit int) ([]Entry, error)
	Purge(before int64) (int, error)
	Relink(fromID, toID int64, toName string) error
	RenameProject(id int64, name string) error
	CountByProject(projectID int64) (int, error)
}

// Clock supplies the current time to Tracker.
type Clock func() time.Time

// ProjectResolver maps a project name to the catalog's project id, creating the project if needed.
type ProjectResolver interface {
	EnsureProject(name string) (id int64, err error)
}
