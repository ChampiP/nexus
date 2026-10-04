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
	if err := m.Save(x); err != nil {
		return err
	}
	key := fmt.Sprintf("event:%d:%s", at, event)
	m.values[key] = "1"
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
			}
		}
	}
	return result, nil
}

type workState struct{ running, breaking bool }

func (w *workState) WorkRunning() (bool, error) { return w.running, nil }
func (w *workState) BreakActive() (bool, error) { return w.breaking, nil }

func TestDefaultsValidationDueAndCounters(t *testing.T) {
	now := time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)
	repo := &memory{map[string]string{}}
	w := &workState{running: true}
	s := NewService(repo, w, func() time.Time { return now })
	if got, _ := s.Settings(); got != DefaultSettings() {
		t.Fatalf("defaults: %+v", got)
	}
	if _, ok := s.Due(now); ok {
		t.Fatal("early due")
	}
	now = now.Add(30 * time.Minute)
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
func TestShortIdlePreservesContinuousWork(t *testing.T) {
	now := time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)
	repo := &memory{map[string]string{}}
	w := &workState{running: true}
	s := NewService(repo, w, func() time.Time { return now })
	_, _ = s.Due(now)
	w.running = false
	_, _ = s.Due(now)
	now = now.Add(4 * time.Minute)
	w.running = true
	_, _ = s.Due(now)
	now = now.Add(26 * time.Minute)
	if _, ok := s.Due(now); !ok {
		t.Fatal("short idle should preserve the original work stretch")
	}
}

func TestScheduleBlocksAndIdleReset(t *testing.T) {
	now := time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)
	repo := &memory{map[string]string{}}
	w := &workState{running: true}
	s := NewService(repo, w, func() time.Time { return now })
	_ = s.MarkShown(now)
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
	w.running = false
	_, _ = s.Due(now)
	now = now.Add(6 * time.Minute)
	w.running = true
	if _, ok := s.Due(now); ok {
		t.Fatal("fresh work stretch must wait for interval")
	}
}

// 15 segundos es una duración válida: el usuario la pidió para pausas cortas.
func TestFifteenSecondsIsAllowed(t *testing.T) {
	s := NewService(&memory{map[string]string{}}, &workState{}, time.Now)
	if err := s.UpdateSettings(Settings{Enabled: true, Every: 20 * time.Minute, Duration: 15 * time.Second}); err != nil {
		t.Fatalf("15 s debería aceptarse: %v", err)
	}
}
