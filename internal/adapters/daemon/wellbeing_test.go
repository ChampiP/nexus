package daemon

import (
	"context"
	"nexus/internal/countdown"
	"nexus/internal/platform/notify"
	"nexus/internal/wellbeing"
	"sync"
	"testing"
	"time"
)

type wbFake struct {
	mu                          sync.Mutex
	marks, done, snoozes, skips int
	due                         bool
}

func (f *wbFake) Due(time.Time) (wellbeing.Reminder, bool) {
	return wellbeing.Reminder{Duration: time.Second, Message: "m", Tip: "t"}, f.due
}
func (f *wbFake) MarkShown(time.Time) error             { f.mu.Lock(); f.marks++; f.mu.Unlock(); return nil }
func (f *wbFake) Done(time.Time) error                  { f.done++; return nil }
func (f *wbFake) Snooze(time.Time, time.Duration) error { f.snoozes++; return nil }
func (f *wbFake) Skip(time.Time) error                  { f.skips++; return nil }

type busyFake struct {
	mu   sync.Mutex
	busy bool
}

func (f *busyFake) Busy(context.Context) (bool, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.busy, ""
}

type showFake struct {
	results chan string
	shown   chan wellbeing.Reminder
}

func (f *showFake) Show(_ context.Context, r wellbeing.Reminder) (string, error) {
	f.shown <- r
	return <-f.results, nil
}

type noBreaks struct{}

func (noBreaks) DueNotification(time.Time) (*countdown.Break, bool) { return nil, false }
func (noBreaks) MarkNotified(int64, time.Time) error                { return nil }
func (noBreaks) Extend(time.Duration) error                         { return nil }
func (noBreaks) End(bool) (int, error)                              { return 0, nil }

type noNotify struct{}

func (noNotify) Send(context.Context, notify.Notification) (string, error) { return "", nil }
func TestWellbeingWaitsUntilTwoMinutesAfterBusyAndMapsReply(t *testing.T) {
	now := time.Now()
	var mu sync.Mutex
	clock := func() time.Time { mu.Lock(); defer mu.Unlock(); return now }
	advance := func(d time.Duration) { mu.Lock(); now = now.Add(d); mu.Unlock() }
	service := &wbFake{due: true}
	busy := &busyFake{busy: true}
	presenter := &showFake{results: make(chan string, 1), shown: make(chan wellbeing.Reminder, 2)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		RunWithWellbeing(ctx, noBreaks{}, noNotify{}, nil, service, presenter, busy, clock, 2*time.Millisecond)
	}()
	defer func() { cancel(); <-done }()
	time.Sleep(20 * time.Millisecond)
	if service.marks != 0 {
		t.Fatal("reminder marked while busy")
	}
	busy.mu.Lock()
	busy.busy = false
	busy.mu.Unlock()
	time.Sleep(10 * time.Millisecond)
	if service.marks != 0 {
		t.Fatalf("reminder marked before grace: %d", service.marks)
	}
	advance(2 * time.Minute)
	select {
	case <-presenter.shown:
	case <-time.After(time.Second):
		t.Fatal("reminder not shown after grace")
	}
	presenter.results <- "skip"
	deadline := time.Now().Add(time.Second)
	for service.skips == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if service.skips != 1 {
		t.Fatalf("skip mapping: %d", service.skips)
	}
}
func TestWellbeingShowsWhenNotBusy(t *testing.T) {
	service := &wbFake{due: true}
	busy := &busyFake{}
	p := &showFake{results: make(chan string, 1), shown: make(chan wellbeing.Reminder, 1)}
	p.results <- ""
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		RunWithWellbeing(ctx, noBreaks{}, noNotify{}, nil, service, p, busy, time.Now, 2*time.Millisecond)
	}()
	select {
	case <-p.shown:
	case <-time.After(time.Second):
		t.Fatal("reminder not shown")
	}
	deadline := time.Now().Add(time.Second)
	for service.done == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if service.marks != 1 || service.done != 1 {
		t.Fatalf("marks=%d done=%d", service.marks, service.done)
	}
	cancel()
	<-done
}

// Cuando la capa nativa se encarga, ella informa lo que eligió el usuario; el daemon no registra nada.
func TestWellbeingDelegatedResultRecordsNothing(t *testing.T) {
	service := &wbFake{due: true}
	showWellbeing(context.Background(), service, &showFake{results: delegated(), shown: make(chan wellbeing.Reminder, 1)}, wellbeing.Reminder{}, time.Now)
	if service.done != 0 || service.snoozes != 0 || service.skips != 0 {
		t.Fatalf("done=%d snoozes=%d skips=%d; quería ninguno", service.done, service.snoozes, service.skips)
	}
}

func delegated() chan string {
	c := make(chan string, 1)
	c <- ResultDelegated
	return c
}
