package wellbeing

import "time"

type Work interface {
	WorkRunning() (bool, error)
	BreakActive() (bool, error)
}

type Repository interface {
	Load() (map[string]string, error)
	Save(map[string]string) error
}

type Clock func() time.Time
