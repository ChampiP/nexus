package countdown

import (
	"fmt"
	"time"
)

const (
	minDuration = time.Minute
	maxDuration = 8 * time.Hour
	// renotifyEvery es el intervalo entre avisos mientras el break está vencido.
	renotifyEvery = 10 * time.Minute
	defaultLabel  = "Break"
)

// Service implementa los casos de uso del break. Los pasos entre módulos van primero y la fila
// de countdown al final, para no dejar datos inconsistentes si algo falla.
type Service struct {
	repo   Repository
	timers Timers
	clock  func() time.Time
}

// NewService crea un Service; un reloj nulo usa time.Now.
func NewService(repo Repository, timers Timers, clock func() time.Time) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{repo: repo, timers: timers, clock: clock}
}

// StartBreak detiene stopIDs, inicia la entrada de break y registra la cuenta regresiva.
func (s *Service) StartBreak(duration time.Duration, label string, stopIDs []int64) (Break, error) {
	if duration < minDuration || duration > maxDuration {
		return Break{}, ErrInvalidDuration
	}
	if active, err := s.repo.Active(); err != nil {
		return Break{}, err
	} else if active != nil {
		return Break{}, ErrBreakActive
	}
	if label == "" {
		label = defaultLabel
	}
	if err := s.timers.StopMany(stopIDs); err != nil {
		return Break{}, fmt.Errorf("stop timers: %w", err)
	}
	entryID, err := s.timers.StartBreak(label)
	if err != nil {
		return Break{}, fmt.Errorf("start break entry: %w", err)
	}
	now := s.clock()
	b, err := s.repo.Insert(Break{
		Label:          label,
		StartedAt:      now.Unix(),
		EndsAt:         now.Add(duration).Unix(),
		EntryID:        entryID,
		ResumeEntryIDs: append([]int64{}, stopIDs...),
	})
	if err != nil {
		// Se detiene la entrada para que no quede un break huérfano corriendo.
		_ = s.timers.Stop(entryID)
		return Break{}, err
	}
	return b, nil
}

// Active devuelve el break sin terminar, o nil si no hay.
func (s *Service) Active() (*Break, error) { return s.repo.Active() }

// Extend retrasa la fecha límite desde el mayor entre ahora y la fecha actual.
func (s *Service) Extend(d time.Duration) error {
	b, err := s.active()
	if err != nil {
		return err
	}
	base := time.Unix(b.EndsAt, 0)
	if now := s.clock(); now.After(base) {
		base = now
	}
	return s.repo.SetEndsAt(b.ID, base.Add(d).Unix())
}

// End termina el break; con resume retoma los timers recordados que no estén en curso
// y devuelve cuántos se retomaron.
func (s *Service) End(resume bool) (resumed int, err error) {
	b, err := s.active()
	if err != nil {
		return 0, err
	}
	// Un break cuya entrada ya fue detenida a mano no debe impedir terminarlo.
	if running, err := s.timers.Running(b.EntryID); err != nil {
		return 0, err
	} else if running {
		if err := s.timers.Stop(b.EntryID); err != nil {
			return 0, fmt.Errorf("stop break entry: %w", err)
		}
	}
	if err := s.repo.Finish(b.ID, s.clock().Unix()); err != nil {
		return 0, err
	}
	if !resume {
		return 0, nil
	}
	for _, id := range b.ResumeEntryIDs {
		if running, err := s.timers.Running(id); err != nil {
			return resumed, err
		} else if running {
			continue
		}
		if err := s.timers.StartLike(id); err != nil {
			return resumed, fmt.Errorf("resume entry #%d: %w", id, err)
		}
		resumed++
	}
	return resumed, nil
}

// DueNotification devuelve el break activo si está vencido y toca avisar: nunca avisado,
// o el último aviso fue hace al menos 10 minutos.
func (s *Service) DueNotification(now time.Time) (*Break, bool) {
	b, err := s.repo.Active()
	if err != nil || b == nil || now.Unix() < b.EndsAt {
		return nil, false
	}
	if b.NotifiedAt != nil && now.Sub(time.Unix(*b.NotifiedAt, 0)) < renotifyEvery {
		return nil, false
	}
	return b, true
}

// MarkNotified registra el momento del último aviso.
func (s *Service) MarkNotified(id int64, at time.Time) error {
	return s.repo.MarkNotified(id, at.Unix())
}

func (s *Service) active() (*Break, error) {
	b, err := s.repo.Active()
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, ErrNoActiveBreak
	}
	return b, nil
}
