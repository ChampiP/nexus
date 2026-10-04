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
}

// Clock supplies the current time to Tracker.
type Clock func() time.Time
