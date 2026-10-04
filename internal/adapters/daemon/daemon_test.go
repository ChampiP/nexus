package daemon

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"nexus/internal/countdown"
	"nexus/internal/platform/notify"
)

var t0 = time.Unix(1_000_000, 0)

type fakeBreaks struct {
	mu       sync.Mutex
	now      time.Time
	b        countdown.Break
	notified *time.Time
	calls    []string
	endErr   error
}

func (f *fakeBreaks) record(c string) { f.calls = append(f.calls, c) }

func (f *fakeBreaks) Active() (*countdown.Break, error) { return &f.b, nil }

func (f *fakeBreaks) DueNotification(now time.Time) (*countdown.Break, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if now.Unix() < f.b.EndsAt || f.notified != nil {
		return nil, false
	}
	b := f.b
	return &b, true
}

func (f *fakeBreaks) MarkNotified(id int64, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("mark")
	f.notified = &at
	return nil
}

func (f *fakeBreaks) Extend(d time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("extend " + d.String())
	return f.endErr
}

func (f *fakeBreaks) End(resume bool) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if resume {
		f.record("end resume")
	} else {
		f.record("end")
	}
	return 0, f.endErr
}

func (f *fakeBreaks) log() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

type fakeNotifier struct {
	sent    chan notify.Notification
	reply   chan string
	breaks  *fakeBreaks
	markedB []bool // si ya se había marcado al enviar
}

func newNotifier(b *fakeBreaks) *fakeNotifier {
	return &fakeNotifier{sent: make(chan notify.Notification, 10), reply: make(chan string), breaks: b}
}

func (n *fakeNotifier) Send(ctx context.Context, x notify.Notification) (string, error) {
	n.breaks.mu.Lock()
	n.markedB = append(n.markedB, n.breaks.notified != nil)
	n.breaks.mu.Unlock()
	n.sent <- x
	select {
	case id := <-n.reply:
		return id, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

type harness struct {
	breaks *fakeBreaks
	not    *fakeNotifier
	cancel context.CancelFunc
	done   chan struct{}
}

func start(t *testing.T, now time.Time, endsAt time.Time) *harness {
	t.Helper()
	b := &fakeBreaks{now: now, b: countdown.Break{ID: 7, StartedAt: endsAt.Add(-time.Hour).Unix(), EndsAt: endsAt.Unix()}}
	n := newNotifier(b)
	ctx, cancel := context.WithCancel(context.Background())
	h := &harness{breaks: b, not: n, cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(h.done)
		open := func() error { b.mu.Lock(); b.record("open"); b.mu.Unlock(); return nil }
		Run(ctx, b, n, open, func() time.Time { return now }, 2*time.Millisecond)
	}()
	t.Cleanup(func() { cancel(); <-h.done })
	return h
}

func (h *harness) waitSent(t *testing.T) notify.Notification {
	t.Helper()
	select {
	case n := <-h.not.sent:
		return n
	case <-time.After(time.Second):
		t.Fatal("no se envió la notificación")
		return notify.Notification{}
	}
}

func (h *harness) waitCalls(t *testing.T, n int) []string {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if c := h.breaks.log(); len(c) >= n {
			return c
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("llamadas esperadas %d, hubo %v", n, h.breaks.log())
	return nil
}

func TestNoNotificationBeforeDeadline(t *testing.T) {
	h := start(t, t0, t0.Add(time.Hour))
	select {
	case <-h.not.sent:
		t.Fatal("no debía notificar antes del plazo")
	case <-time.After(30 * time.Millisecond):
	}
	if c := h.breaks.log(); len(c) != 0 {
		t.Fatalf("llamadas inesperadas: %v", c)
	}
}

func TestNotifiesOnceWhenOverdueWithMarkFirst(t *testing.T) {
	h := start(t, t0, t0.Add(-12*time.Minute))
	n := h.waitSent(t)
	if n.Title != "☕ Se acabó tu break" || n.Body != "Llevas 12 min de más. Toca para marcar tu vuelta." || n.Urgency != "critical" {
		t.Fatalf("notificación inesperada: %+v", n)
	}
	// "default" es lo que dispara un clic en la tarjeta; los botones sirven en servidores que sí los dibujan.
	want := []notify.Action{{ID: "default", Label: "Abrir Nexus"}, {ID: "back", Label: "Volver al trabajo"}, {ID: "extend", Label: "+10 min"}, {ID: "end", Label: "Terminar break"}}
	if len(n.Actions) != 4 || n.Actions[0] != want[0] || n.Actions[1] != want[1] || n.Actions[2] != want[2] || n.Actions[3] != want[3] {
		t.Fatalf("acciones: %+v", n.Actions)
	}
	if len(h.not.markedB) != 1 || !h.not.markedB[0] {
		t.Fatalf("MarkNotified debe ocurrir antes de enviar: %v", h.not.markedB)
	}
}

func TestBodyJustOverdue(t *testing.T) {
	h := start(t, t0, t0.Add(-20*time.Second))
	if n := h.waitSent(t); n.Body != "Tu break de 60 min terminó. Toca para marcar tu vuelta." {
		t.Fatalf("cuerpo: %q", n.Body)
	}
}

func TestBodyUsesBreakDuration(t *testing.T) {
	b := countdown.Break{StartedAt: t0.Add(-30 * time.Minute).Unix(), EndsAt: t0.Unix()}
	if got := body(b, t0); got != "Tu break de 30 min terminó. Toca para marcar tu vuelta." {
		t.Fatalf("cuerpo: %q", got)
	}
}

func TestNoSecondNotificationWhilePending(t *testing.T) {
	h := start(t, t0, t0.Add(-time.Minute))
	h.waitSent(t)
	// Aunque el servicio siguiera devolviendo vencido, no hay un segundo aviso pendiente.
	h.breaks.mu.Lock()
	h.breaks.notified = nil
	h.breaks.mu.Unlock()
	select {
	case <-h.not.sent:
		t.Fatal("segunda notificación con una pendiente")
	case <-time.After(30 * time.Millisecond):
	}
}

func TestReplies(t *testing.T) {
	for _, tc := range []struct {
		reply string
		want  []string
	}{
		{"default", []string{"mark", "open"}},
		{"back", []string{"mark", "end resume"}},
		{"extend", []string{"mark", "extend 10m0s"}},
		{"end", []string{"mark", "end"}},
	} {
		t.Run(tc.reply, func(t *testing.T) {
			h := start(t, t0, t0.Add(-time.Minute))
			h.waitSent(t)
			h.not.reply <- tc.reply
			got := h.waitCalls(t, 2)
			if got[0] != tc.want[0] || got[1] != tc.want[1] {
				t.Fatalf("llamadas = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDismissDoesNothing(t *testing.T) {
	h := start(t, t0, t0.Add(-time.Minute))
	h.waitSent(t)
	h.not.reply <- ""
	time.Sleep(30 * time.Millisecond)
	if c := h.breaks.log(); len(c) != 1 || c[0] != "mark" {
		t.Fatalf("llamadas = %v", c)
	}
}

func TestNoActiveBreakOnReplyIsIgnored(t *testing.T) {
	h := start(t, t0, t0.Add(-time.Minute))
	h.breaks.endErr = countdown.ErrNoActiveBreak
	h.waitSent(t)
	h.not.reply <- "back"
	h.waitCalls(t, 2)
	// El bucle sigue vivo: cancelar lo detiene limpiamente.
	h.cancel()
	select {
	case <-h.done:
	case <-time.After(time.Second):
		t.Fatal("el bucle no se detuvo")
	}
}

func TestOtherReplyErrorDoesNotCrash(t *testing.T) {
	h := start(t, t0, t0.Add(-time.Minute))
	h.breaks.endErr = errors.New("boom")
	h.waitSent(t)
	h.not.reply <- "end"
	h.waitCalls(t, 2)
	h.cancel()
	<-h.done
}

func TestCancelStopsAndCancelsPending(t *testing.T) {
	h := start(t, t0, t0.Add(-time.Minute))
	h.waitSent(t)
	h.cancel()
	select {
	case <-h.done:
	case <-time.After(time.Second):
		t.Fatal("Run no terminó al cancelar")
	}
	if c := h.breaks.log(); len(c) != 1 {
		t.Fatalf("no debía actuar tras cancelar: %v", c)
	}
}

func TestLockPreventsSecondInstance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nexus-daemon.lock")
	release, err := Lock(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Lock(path); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("segundo Lock = %v, want ErrAlreadyRunning", err)
	}
	release()
	release2, err := Lock(path)
	if err != nil {
		t.Fatalf("tras liberar debe poder bloquear: %v", err)
	}
	release2()
}
