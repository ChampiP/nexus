package wellbeing

import (
	"fmt"
	"testing"
	"time"
)

type memory struct{ values map[string]string }

func (m *memory) Load() (map[string]string, error) {
	x := map[string]string{}
	for k, v := range m.values {
		x[k] = v
	}
	return x, nil
}
func (m *memory) Save(x map[string]string) error { m.values = x; return nil }
func (m *memory) SaveEvent(x map[string]string, event string, at int64) error {
	for key, value := range x {
		m.values[key] = value
	}
	m.values[fmt.Sprintf("event:%d:%s", at, event)] = "1"
	return nil
}
func (m *memory) Counters(start, end int64) (Counters, error) {
	var result Counters
	for key := range m.values {
		var at int64
		var event string
		if _, err := fmt.Sscanf(key, "event:%d:%s", &at, &event); err == nil && at >= start && at < end {
			switch event {
			case "shown":
				result.Shown++
			case "done":
				result.Done++
			case "skipped":
				result.Skipped++
			case "snoozed":
				result.Snoozed++
			}
		}
	}
	return result, nil
}

type workState struct{ active, breaking bool }

func (w *workState) Active(time.Time) (bool, error) { return w.active, nil }
func (w *workState) BreakActive() (bool, error)     { return w.breaking, nil }

func TestDefaultsValidationDueAndCounters(t *testing.T) {
	now := time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)
	repo := &memory{map[string]string{}}
	w := &workState{active: true}
	s := NewService(repo, w, nil)
	if got, _ := s.Settings(); got != DefaultSettings() {
		t.Fatalf("defaults: %+v", got)
	}
	if _, ok := s.Due(now); ok {
		t.Fatal("early due")
	}
	for range 29 {
		now = now.Add(time.Minute)
		_, _ = s.Due(now)
	}
	now = now.Add(time.Minute)
	r, ok := s.Due(now)
	if !ok || r.Duration != 30*time.Second || r.Tip == "" {
		t.Fatalf("due=%+v %v", r, ok)
	}
	if e := s.MarkShown(now); e != nil {
		t.Fatal(e)
	}
	if e := s.Done(now); e != nil {
		t.Fatal(e)
	}
	st, e := s.Status(now)
	if e != nil || st.Counters != (Counters{Shown: 1, Done: 1}) {
		t.Fatalf("status=%+v err=%v", st, e)
	}
	if e := s.UpdateSettings(Settings{Enabled: true, Every: 15 * time.Minute, Duration: time.Second}); e != ErrInvalidSetting {
		t.Fatalf("invalid settings error=%v", e)
	}
}

func TestDueUsesActiveComputerWithNoRunningTimers(t *testing.T) {
	now := time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)
	repo := &memory{map[string]string{}}
	w := &workState{active: true}
	s := NewService(repo, w, nil)
	_, _ = s.Due(now)
	for range 29 {
		now = now.Add(time.Minute)
		_, _ = s.Due(now)
	}
	now = now.Add(time.Minute)
	if _, ok := s.Due(now); !ok {
		t.Fatal("active computer should become due regardless of running timers")
	}
}

func TestObservationGapContinuity(t *testing.T) {
	for _, tc := range []struct {
		name    string
		gap     time.Duration
		missing bool
		reset   bool
	}{
		{"shutdown", 8 * time.Hour, false, true},
		{"reset boundary", inactiveReset, false, true},
		{"short gap", time.Minute, false, false},
		{"existing install", 8 * time.Hour, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)
			repo := &memory{map[string]string{}}
			s := NewService(repo, &workState{active: true}, nil)
			_, _ = s.Due(start)
			if tc.missing {
				delete(repo.values, "observed_at")
			}
			resume := start.Add(tc.gap)
			_, due := s.Due(resume)
			wantStart := start
			if tc.reset {
				wantStart = resume
			}
			if due != (resume.Sub(wantStart) >= DefaultSettings().Every) {
				t.Fatalf("due after gap=%v", due)
			}
			if got := parseTime(repo.values["active_since"]); !got.Equal(wantStart) {
				t.Fatalf("start=%v want %v", got, wantStart)
			}
			if got := parseTime(repo.values["observed_at"]); !got.Equal(resume) {
				t.Fatalf("observation=%v want %v", got, resume)
			}
			if tc.reset {
				for elapsed := time.Minute; elapsed <= DefaultSettings().Every; elapsed += time.Minute {
					_, due = s.Due(resume.Add(elapsed))
					if due != (elapsed == DefaultSettings().Every) {
						t.Fatalf("elapsed=%v due=%v", elapsed, due)
					}
				}
			}
		})
	}
}

func TestStatusAfterObservationGap(t *testing.T) {
	start := time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)
	s := NewService(&memory{map[string]string{}}, &workState{active: true}, nil)
	_, _ = s.Due(start)
	now := start.Add(8 * time.Hour)
	status, err := s.Status(now)
	if err != nil || status.ActiveFor != 0 || !status.NextDue.Equal(now.Add(DefaultSettings().Every)) {
		t.Fatalf("status=%+v err=%v", status, err)
	}
}

func TestInactiveObservationsArePersisted(t *testing.T) {
	now := time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)
	repo := &memory{map[string]string{}}
	s := NewService(repo, &workState{}, nil)
	_, _ = s.Due(now)
	_, _ = s.Due(now.Add(time.Minute))
	if got := parseTime(repo.values["observed_at"]); !got.Equal(now.Add(time.Minute)) {
		t.Fatalf("observation=%v", got)
	}
	if got := parseTime(repo.values["inactive_since"]); !got.Equal(now) {
		t.Fatalf("inactivity start=%v", got)
	}
}

func TestInactiveContinuityRules(t *testing.T) {
	cases := []struct {
		name     string
		inactive time.Duration
		reset    bool
	}{{"short blip", time.Minute, false}, {"two minute break", 2 * time.Minute, true}, {"locked", 2 * time.Minute, true}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)
			repo := &memory{map[string]string{}}
			w := &workState{active: true}
			s := NewService(repo, w, nil)
			_, _ = s.Due(now)
			w.active = false
			_, _ = s.Due(now)
			w.active = true
			resume := now.Add(tc.inactive)
			_, _ = s.Due(resume)
			if tc.reset {
				if got := parseTime(repo.values["active_since"]); !got.Equal(resume) {
					t.Fatalf("continuity start=%v want %v", got, resume)
				}
			} else if got := parseTime(repo.values["active_since"]); !got.Equal(now) {
				t.Fatalf("short inactive blip reset continuity: %v", got)
			}
		})
	}
}

func TestBreakSnoozeAndDNDBlockDue(t *testing.T) {
	now := time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)
	repo := &memory{map[string]string{}}
	w := &workState{active: true}
	s := NewService(repo, w, nil)
	_, _ = s.Due(now)
	w.breaking = true
	now = now.Add(time.Hour)
	if _, ok := s.Due(now); ok {
		t.Fatal("break must block")
	}
	w.breaking = false
	_ = s.Snooze(now, 10*time.Minute)
	if _, ok := s.Due(now); ok {
		t.Fatal("snooze must block")
	}
	_ = s.DND(now, time.Hour)
	if _, ok := s.Due(now); ok {
		t.Fatal("DND must block")
	}
	_ = s.ClearDND()
}

func TestFifteenSecondsIsAllowed(t *testing.T) {
	s := NewService(&memory{map[string]string{}}, &workState{}, time.Now)
	if err := s.UpdateSettings(Settings{Enabled: true, Every: 20 * time.Minute, Duration: 15 * time.Second}); err != nil {
		t.Fatalf("15 s debería aceptarse: %v", err)
	}
}

func TestApplyResultRecordsOnlyExplicitChoices(t *testing.T) {
	now := time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		result string
		want   Counters
	}{
		{"done", Counters{Done: 1}},
		{"snooze", Counters{Snoozed: 1}},
		{"skip", Counters{Skipped: 1}},
		{"delegated", Counters{}},
		{"default", Counters{}},
		{"", Counters{}},
	}
	for _, c := range cases {
		s := NewService(&memory{map[string]string{}}, &workState{active: true}, nil)
		if err := ApplyResult(s, now, c.result); err != nil {
			t.Fatalf("%q: %v", c.result, err)
		}
		got, _ := s.repo.(*memory).Counters(now.Unix()-1, now.Unix()+1)
		if got != c.want {
			t.Fatalf("%q: got %+v want %+v", c.result, got, c.want)
		}
	}
}
