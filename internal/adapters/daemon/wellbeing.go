package daemon

import (
	"context"
	"log/slog"
	"time"

	"nexus/internal/wellbeing"
)

type Wellbeing interface {
	Due(time.Time) (wellbeing.Reminder, bool)
	MarkShown(time.Time) error
	Done(time.Time) error
	Snooze(time.Time, time.Duration) error
	Skip(time.Time) error
}
type Presenter interface {
	Show(context.Context, wellbeing.Reminder) (string, error)
}
type Result string

const (
	ResultDone   = "done"
	ResultSnooze = "snooze"
	ResultSkip   = "skip"
	// ResultDelegated indica que la capa nativa muestra la pausa e informa ella misma la elección.
	ResultDelegated = "delegated"
)

type BusyChecker interface {
	Busy(context.Context) (bool, string)
}

type wellbeingWait struct {
	busy  bool
	since time.Time
}

func showWellbeing(ctx context.Context, s Wellbeing, p Presenter, r wellbeing.Reminder, clock func() time.Time) {
	result, err := p.Show(ctx, r)
	if err != nil {
		if ctx.Err() == nil {
			slog.Error("no se pudo mostrar la pausa activa", "error", err)
		}
		return
	}
	if err = wellbeing.ApplyResult(s, clock(), result); err != nil {
		slog.Error("no se pudo guardar la respuesta de pausa activa", "error", err)
	}
}
