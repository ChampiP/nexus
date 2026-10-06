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
	repeat                      bool
	duration                    time.Duration
	checked                     chan struct{}
	action                      chan string
}

func (f *wbFake) Due(time.Time) (wellbeing.Reminder, bool) {
	f.mu.Lock()
	due, duration := f.due, f.duration
	f.mu.Unlock()
	if duration == 0 {
		duration = time.Second
	}
	if f.checked != nil {
		select {
		case f.checked <- struct{}{}:
		default:
		}
	}
	return wellbeing.Reminder{Duration: duration, Message: "m", Tip: "t"}, due
}
func (f *wbFake) MarkShown(time.Time) error {
	f.mu.Lock()
	f.marks++
	if !f.repeat {
		f.due = false
	}
	f.mu.Unlock()
	return nil
}
func (f *wbFake) Done(time.Time) error {
	f.mu.Lock()
	f.done++
	f.mu.Unlock()
	if f.action != nil {
		f.action <- "done"
	}
	return nil
}
func (f *wbFake) Snooze(time.Time, time.Duration) error {
	f.mu.Lock()
	f.snoozes++
	f.mu.Unlock()
	if f.action != nil {
		f.action <- "snooze"
	}
	return nil
}
func (f *wbFake) Skip(time.Time) error {
	f.mu.Lock()
	f.skips++
	f.mu.Unlock()
	if f.action != nil {
		f.action <- "skip"
	}
	return nil
}
func (f *wbFake) counts() (marks, done, snoozes, skips int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.marks, f.done, f.snoozes, f.skips
}

type busyFake struct {
	mu      sync.Mutex
	busy    bool
	checked chan struct{}
}

func (f *busyFake) Busy(context.Context) (bool, string) {
	f.mu.Lock()
	busy := f.busy
	f.mu.Unlock()
	if f.checked != nil {
		select {
		case f.checked <- struct{}{}:
		default:
		}
	}
	return busy, ""
}
func (f *busyFake) setBusy(value bool) { f.mu.Lock(); f.busy = value; f.mu.Unlock() }

type showFake struct {
	results chan string
	shown   chan wellbeing.Reminder
}

func (f *showFake) Show(ctx context.Context, r wellbeing.Reminder) (string, error) {
	select {
	case f.shown <- r:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	select {
	case result := <-f.results:
		return result, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

type noBreaks struct{}

func (noBreaks) DueNotification(time.Time) (*countdown.Break, bool) { return nil, false }
func (noBreaks) MarkNotified(int64, time.Time) error                { return nil }
func (noBreaks) Extend(time.Duration) error                         { return nil }
func (noBreaks) End(bool) (int, error)                              { return 0, nil }

type noNotify struct{}

func (noNotify) Send(context.Context, notify.Notification) (string, error) { return "", nil }

func awaitSignal(t *testing.T, ch <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal(message)
	}
}

type cancelBlockingPresenter struct {
	started  chan struct{}
	canceled chan struct{}
	release  chan struct{}
}

func (p *cancelBlockingPresenter) Show(ctx context.Context, _ wellbeing.Reminder) (string, error) {
	close(p.started)
	<-ctx.Done()
	close(p.canceled)
	<-p.release
	return "", ctx.Err()
}

func TestRunWaitsForWellbeingPresenterOnCancel(t *testing.T) {
	service := &wbFake{due: true}
	presenter := &cancelBlockingPresenter{
		started: make(chan struct{}), canceled: make(chan struct{}), release: make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		RunWithWellbeing(ctx, noBreaks{}, noNotify{}, nil, service, presenter, &busyFake{}, time.Now, time.Hour)
	}()
	awaitSignal(t, presenter.started, "presenter did not start")
	cancel()
	awaitSignal(t, presenter.canceled, "presenter context was not canceled")
	select {
	case <-done:
		t.Fatal("RunWithWellbeing returned while the presenter was still running")
	case <-time.After(20 * time.Millisecond):
	}
	close(presenter.release)
	awaitSignal(t, done, "RunWithWellbeing did not return after presenter finished")
}

func TestWellbeingWaitsUntilTwoMinutesAfterBusyAndMapsReply(t *testing.T) {
	now := time.Now()
	var mu sync.Mutex
	clock := func() time.Time { mu.Lock(); defer mu.Unlock(); return now }
	advance := func(d time.Duration) { mu.Lock(); now = now.Add(d); mu.Unlock() }
	service := &wbFake{due: true, checked: make(chan struct{}, 32), action: make(chan string, 1)}
	busy := &busyFake{busy: true, checked: make(chan struct{}, 32)}
	presenter := &showFake{results: make(chan string, 1), shown: make(chan wellbeing.Reminder, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		RunWithWellbeing(ctx, noBreaks{}, noNotify{}, nil, service, presenter, busy, clock, 2*time.Millisecond)
	}()
	defer func() { cancel(); awaitSignal(t, done, "daemon did not stop") }()

	awaitSignal(t, busy.checked, "initial busy check did not run")
	busy.setBusy(false)
	awaitSignal(t, busy.checked, "non-busy check did not run")
	advance(2 * time.Minute)
	select {
	case <-presenter.shown:
	case <-time.After(time.Second):
		t.Fatal("reminder not shown after grace")
	}
	presenter.results <- "skip"
	select {
	case result := <-service.action:
		if result != "skip" {
			t.Fatalf("action=%q", result)
		}
	case <-time.After(time.Second):
		t.Fatal("skip result was not recorded")
	}
	marks, _, _, skips := service.counts()
	if marks != 1 || skips != 1 {
		t.Fatalf("marks=%d skips=%d", marks, skips)
	}
}

func TestWellbeingShowsWhenNotBusy(t *testing.T) {
	service := &wbFake{due: true, action: make(chan string, 1)}
	busy := &busyFake{}
	p := &showFake{results: make(chan string, 1), shown: make(chan wellbeing.Reminder, 1)}
	p.results <- "done"
	ctx, cancel := context.WithCancel(context.Background())
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
	select {
	case <-service.action:
	case <-time.After(time.Second):
		t.Fatal("done action was not recorded")
	}
	marks, completed, _, _ := service.counts()
	if marks != 1 || completed != 1 {
		t.Fatalf("marks=%d done=%d", marks, completed)
	}
	cancel()
	awaitSignal(t, done, "daemon did not stop")
}

type blockingThenShowing struct {
	mu    sync.Mutex
	calls int
	shown chan int
}

func (p *blockingThenShowing) Show(ctx context.Context, _ wellbeing.Reminder) (string, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.mu.Unlock()
	p.shown <- call
	if call == 1 {
		<-ctx.Done()
		return "", ctx.Err()
	}
	return "done", nil
}

func TestWellbeingPresenterDeadlineFreesLaneForLaterReminder(t *testing.T) {
	service := &wbFake{due: true, repeat: true, duration: -59 * time.Second}
	presenter := &blockingThenShowing{shown: make(chan int, 2)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		RunWithWellbeing(ctx, noBreaks{}, noNotify{}, nil, service, presenter, &busyFake{}, time.Now, time.Millisecond)
	}()
	defer func() { cancel(); awaitSignal(t, done, "daemon did not stop") }()
	select {
	case call := <-presenter.shown:
		if call != 1 {
			t.Fatalf("first call=%d", call)
		}
	case <-time.After(time.Second):
		t.Fatal("first reminder not shown")
	}
	select {
	case call := <-presenter.shown:
		if call != 2 {
			t.Fatalf("second call=%d", call)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("lane did not free after presenter deadline")
	}
}

// Cuando la capa nativa se encarga, ella informa lo que eligió el usuario; el daemon no registra nada.
func TestWellbeingDelegatedResultRecordsNothing(t *testing.T) {
	service := &wbFake{due: true}
	showWellbeing(context.Background(), service, &showFake{results: delegated(), shown: make(chan wellbeing.Reminder, 1)}, wellbeing.Reminder{}, time.Now)
	_, done, snoozes, skips := service.counts()
	if done != 0 || snoozes != 0 || skips != 0 {
		t.Fatalf("done=%d snoozes=%d skips=%d; quería ninguno", done, snoozes, skips)
	}
}

func delegated() chan string {
	c := make(chan string, 1)
	c <- ResultDelegated
	return c
}

func TestShowWellbeingClosedOrClickedRecordsNothing(t *testing.T) {
	for _, result := range []string{"", "default"} {
		service := &wbFake{due: true}
		showWellbeing(context.Background(), service, &showFake{results: func() chan string { c := make(chan string, 1); c <- result; return c }(), shown: make(chan wellbeing.Reminder, 1)}, wellbeing.Reminder{}, time.Now)
		if _, done, snoozes, skips := service.counts(); done+snoozes+skips != 0 {
			t.Fatalf("%q recorded done=%d snoozes=%d skips=%d", result, done, snoozes, skips)
		}
	}
}
