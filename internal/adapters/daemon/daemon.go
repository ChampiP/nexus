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
)

// extendBy es lo que suma la acción "+10 min".
const extendBy = 10 * time.Minute

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
	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	var wg sync.WaitGroup
	defer wg.Wait()
	sendCtx, cancelSend := context.WithCancel(ctx)
	defer cancelSend()

	pending := false
	finished := make(chan struct{}, 1)
	check := func() {
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
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { finished <- struct{}{} }()
			reply(sendCtx, breaks, notifier, open, notification(*b, now))
		}()
	}

	check()
	for {
		select {
		case <-ctx.Done():
			return
		case <-finished:
			pending = false
		case <-ticker.C:
			check()
		}
	}
}

// reply envía la notificación y aplica la acción elegida.
func reply(ctx context.Context, breaks Breaks, notifier Notifier, open Opener, n notify.Notification) {
	id, err := notifier.Send(ctx, n)
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
