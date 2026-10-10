// Package daemon es el servicio que avisa cuando un break vence y atiende la respuesta del usuario.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"nexus/internal/countdown"
	"nexus/internal/platform/notify"
	"nexus/internal/wellbeing"
)

// extendBy es lo que suma la acción "+10 min".
const extendBy = 10 * time.Minute

const (
	heartbeatEvery = 30 * time.Second
	shutdownGap    = 2 * time.Minute
)

// StoppedEntry contiene solo el dato necesario para explicar la detención.
type StoppedEntry struct{ Title string }

// Stopper detiene las sesiones de trabajo en el último latido.
type Stopper interface {
	StopRunningAt(time.Time) ([]StoppedEntry, error)
}

// Heartbeat conserva el último instante en que el daemon estuvo activo.
type Heartbeat interface {
	Last() (time.Time, bool, error)
	Beat(time.Time) error
}

// Option agrega dependencias opcionales sin cambiar los llamadores existentes.
type Option func(*shutdownGuard)

// WithShutdownGuard habilita la recuperación tras apagados o suspensiones.
func WithShutdownGuard(stopper Stopper, heartbeat Heartbeat) Option {
	return func(g *shutdownGuard) { g.stopper, g.heartbeat = stopper, heartbeat }
}

type shutdownGuard struct {
	stopper   Stopper
	heartbeat Heartbeat
}

func (g *shutdownGuard) check(now time.Time, send func(notify.Notification)) {
	if g.stopper == nil || g.heartbeat == nil {
		return
	}
	last, present, err := g.heartbeat.Last()
	if err != nil {
		slog.Error("no se pudo leer el latido", "error", err)
		return
	}
	if present && now.Sub(last) > shutdownGap {
		entries, err := g.stopper.StopRunningAt(last)
		if len(entries) > 0 {
			subject := fmt.Sprintf("%d cronómetros", len(entries))
			verb := "Se detuvieron"
			resume := "reanudarlos"
			if len(entries) == 1 {
				subject = "«" + entries[0].Title + "»"
				verb = "Se detuvo"
				resume = "reanudarlo"
			}
			send(notify.Notification{
				Title:   "Nexus: cronómetro detenido",
				Body:    fmt.Sprintf("%s %s a las %s porque la PC se apagó o se suspendió. Puedes %s desde Nexus.", verb, subject, last.Format("15:04"), resume),
				Actions: []notify.Action{{ID: "default", Label: "Abrir Nexus"}},
			})
		}
		if err != nil {
			slog.Error("no se pudieron detener los cronómetros", "error", err)
			return
		}
	}
	if !present || now.Sub(last) >= heartbeatEvery {
		if err := g.heartbeat.Beat(now); err != nil {
			slog.Error("no se pudo escribir el latido", "error", err)
		}
	}
}

func shutdownReply(ctx context.Context, notifier Notifier, open Opener, n notify.Notification) {
	sendCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	id, err := notifier.Send(sendCtx, n)
	if err != nil {
		if sendCtx.Err() == nil {
			slog.Error("no se pudo enviar el aviso de detención", "error", err)
		}
		return
	}
	if sendCtx.Err() != nil || id != "default" {
		return
	}
	if err := open(); err != nil {
		slog.Error("no se pudo abrir Nexus", "error", err)
	}
}

// Breaks es lo que el daemon necesita de countdown.Service.
type Breaks interface {
	DueNotification(now time.Time) (*countdown.Break, bool)
	MarkNotified(id int64, at time.Time) error
	Extend(d time.Duration) error
	End(resume bool) (int, error)
}

// Notifier muestra una notificación y devuelve el id de la acción elegida ("" si se descartó).
type Notifier interface {
	Send(ctx context.Context, n notify.Notification) (string, error)
}

// Opener abre la vista de Nexus donde el usuario elige qué hacer con el break vencido.
type Opener func() error

// Run revisa cada tick si hay un break vencido que avisar, hasta que ctx se cancele.
// Solo hay una notificación pendiente a la vez; los errores se registran y no detienen el bucle.
func Run(ctx context.Context, breaks Breaks, notifier Notifier, open Opener, clock func() time.Time, tick time.Duration) {
	RunWithWellbeing(ctx, breaks, notifier, open, nil, nil, nil, clock, tick)
}

// RunWithWellbeing revisa breaks y pausas activas en el mismo tick.
func RunWithWellbeing(ctx context.Context, breaks Breaks, notifier Notifier, open Opener, service Wellbeing, presenter Presenter, busy BusyChecker, clock func() time.Time, tick time.Duration, options ...Option) {
	guard := &shutdownGuard{}
	for _, option := range options {
		option(guard)
	}
	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	sendCtx, cancelSend := context.WithCancel(ctx)
	defer cancelSend()

	pending := false
	wellbeingPending := false
	wait := &wellbeingWait{}
	finished := make(chan struct{}, 1)
	wellbeingFinished := make(chan struct{}, 1)
	var workers sync.WaitGroup
	check := func() {
		guard.check(clock(), func(n notify.Notification) {
			workers.Add(1)
			go func() {
				defer workers.Done()
				shutdownReply(sendCtx, notifier, open, n)
			}()
		})
		if !wellbeingPending && service != nil && presenter != nil && busy != nil {
			r, ok := service.Due(clock())
			if ok {
				isBusy, _ := busy.Busy(ctx)
				if isBusy {
					wait.busy = true
					wait.since = time.Time{}
				} else if wait.busy {
					wait.busy = false
					wait.since = clock()
				} else if wait.since.IsZero() || clock().Sub(wait.since) >= 2*time.Minute {
					if err := service.MarkShown(clock()); err != nil {
						slog.Error("no se pudo registrar la pausa activa", "error", err)
					} else {
						wellbeingPending = true
						wait.since = time.Time{}
						workers.Add(1)
						go func(reminder wellbeing.Reminder) {
							defer workers.Done()
							defer func() { wellbeingFinished <- struct{}{} }()
							showCtx, cancel := context.WithTimeout(sendCtx, reminder.Duration+60*time.Second)
							defer cancel()
							showWellbeing(showCtx, service, presenter, reminder, clock)
						}(r)
					}
				}
			} else {
				wait.busy = false
				wait.since = time.Time{}
			}
		}
		if pending {
			return
		}
		now := clock()
		b, due := breaks.DueNotification(now)
		if !due {
			return
		}
		// Se marca antes de enviar: si el envío falla o el proceso muere, no se repite en cada tick.
		if err := breaks.MarkNotified(b.ID, now); err != nil {
			slog.Error("no se pudo marcar el aviso", "error", err)
			return
		}
		pending = true
		workers.Add(1)
		go func() {
			defer workers.Done()
			defer func() { finished <- struct{}{} }()
			reply(sendCtx, breaks, notifier, open, notification(*b, now))
		}()
	}

	check()
	for {
		select {
		case <-ctx.Done():
			cancelSend()
			workers.Wait()
			return
		case <-finished:
			pending = false
		case <-wellbeingFinished:
			wellbeingPending = false
		case <-ticker.C:
			check()
		}
	}
}

// reply envía la notificación y aplica la acción elegida.
func reply(ctx context.Context, breaks Breaks, notifier Notifier, open Opener, n notify.Notification) {
	sendCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	id, err := notifier.Send(sendCtx, n)
	ctx = sendCtx
	if err != nil {
		if ctx.Err() == nil {
			slog.Error("no se pudo enviar la notificación", "error", err)
		}
		return
	}
	if ctx.Err() != nil {
		return
	}
	switch id {
	case "default":
		// Clic en la tarjeta: la mayoría de servidores (incluido Omarchy) no dibuja botones.
		err = open()
	case "back":
		_, err = breaks.End(true)
	case "extend":
		err = breaks.Extend(extendBy)
	case "end":
		_, err = breaks.End(false)
	default:
		return
	}
	// Si el break ya terminó en otro lado, no hay nada que hacer.
	if err != nil && !errors.Is(err, countdown.ErrNoActiveBreak) {
		slog.Error("no se pudo aplicar la respuesta", "acción", id, "error", err)
	}
}

func notification(b countdown.Break, now time.Time) notify.Notification {
	return notify.Notification{
		Title:   "☕ Se acabó tu break",
		Body:    body(b, now),
		Urgency: "critical",
		Actions: []notify.Action{
			{ID: "default", Label: "Abrir Nexus"},
			{ID: "back", Label: "Volver al trabajo"},
			{ID: "extend", Label: "+10 min"},
			{ID: "end", Label: "Terminar break"},
		},
	}
}

// body explica el estado: minutos de más si ya pasó al menos uno, o la duración del break si acaba de vencer.
func body(b countdown.Break, now time.Time) string {
	if over := b.Overdue(now); over >= time.Minute {
		return fmt.Sprintf("Llevas %d min de más. Toca para marcar tu vuelta.", int(over/time.Minute))
	}
	total := time.Unix(b.EndsAt, 0).Sub(time.Unix(b.StartedAt, 0))
	return fmt.Sprintf("Tu break de %d min terminó. Toca para marcar tu vuelta.", int(total/time.Minute))
}
