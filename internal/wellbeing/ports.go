package wellbeing

import "time"

type Activity interface {
	Active(time.Time) (bool, error)
	BreakActive() (bool, error)
}

type Repository interface {
	Load() (map[string]string, error)
	Save(map[string]string) error
}

type eventRepository interface {
	SaveEvent(map[string]string, string, int64) error
	Counters(int64, int64) (Counters, error)
}

type Clock func() time.Time
