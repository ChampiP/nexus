package cli

import (
	"bytes"
	"context"
	"nexus/internal/wellbeing"
	"strconv"
	"strings"
	"testing"
	"time"
)

type pauseMem struct{ v map[string]string }

func (m *pauseMem) Load() (map[string]string, error) {
	x := map[string]string{}
	for k, v := range m.v {
		x[k] = v
	}
	return x, nil
}
func (m *pauseMem) Save(v map[string]string) error { m.v = v; return nil }

type pauseWork struct{}

func (pauseWork) Active(time.Time) (bool, error) { return true, nil }
func (pauseWork) BreakActive() (bool, error)     { return false, nil }

type pausePresenter struct{ result string }

func (p pausePresenter) Show(context.Context, wellbeing.Reminder) (string, error) {
	return p.result, nil
}
func TestWellbeingStatusSaysJustStartedBelowOneMinute(t *testing.T) {
	now := time.Now()
	repo := &pauseMem{map[string]string{"active_since": strconv.FormatInt(now.Unix(), 10)}}
	s := wellbeing.NewService(repo, pauseWork{}, func() time.Time { return now })
	var out bytes.Buffer
	if err := showWellbeingStatus(s, now, false, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "llevas 1 min") || !strings.Contains(out.String(), "recién empiezas") {
		t.Fatalf("status=%q", out.String())
	}
}

func TestWellbeingStatusShowsSnoozedCount(t *testing.T) {
	now := time.Date(2025, 1, 2, 15, 0, 0, 0, time.Local)
	repo := &pauseMem{map[string]string{"snoozed:2025-01-02": "3"}}
	s := wellbeing.NewService(repo, pauseWork{}, func() time.Time { return now })
	var out bytes.Buffer
	if err := showWellbeingStatus(s, now, false, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "3 pospuestas") {
		t.Fatalf("status=%q", out.String())
	}
}

func TestWellbeingCLIStatusSettingsAndActions(t *testing.T) {
	now := time.Date(2025, 1, 2, 15, 0, 0, 0, time.Local)
	repo := &pauseMem{map[string]string{"active_since": strconv.FormatInt(now.Add(-12*time.Minute).Unix(), 10)}}
	s := wellbeing.NewService(repo, pauseWork{}, func() time.Time { return now })
	run := func(args ...string) (string, error) {
		var out bytes.Buffer
		err := RunWithOptions(append([]string{"pausa"}, args...), nil, Options{Wellbeing: s, Presenter: pausePresenter{}, Now: func() time.Time { return now }}, &out, &bytes.Buffer{})
		return out.String(), err
	}
	out, e := run()
	if e != nil || !strings.Contains(out, "Pausas activas: cada 30 min, 30 s · llevas 12 min frente a la pantalla · próxima en 18 min") {
		t.Fatalf("status=%q err=%v", out, e)
	}
	if _, e = run("--cada", "20", "--dura", "17s"); e == nil {
		t.Fatal("unsupported duration accepted")
	}
	if _, e = run("--cada", "20", "--dura", "20s"); e != nil {
		t.Fatal(e)
	}
	if _, e = run("no-molestar", "30"); e != nil {
		t.Fatal(e)
	}
	out, e = run("estado", "--json")
	if e != nil || !strings.Contains(out, `"dnd_until"`) {
		t.Fatalf("json=%q err=%v", out, e)
	}
	out, e = run("--json")
	if e != nil || !strings.Contains(out, `"dnd_until"`) {
		t.Fatalf("pausa --json = %q err=%v", out, e)
	}
	if _, e = run("reanudar"); e != nil {
		t.Fatal(e)
	}
	if _, e = run("saltar"); e != nil {
		t.Fatal(e)
	}
	if _, e = run("incorrecto"); e == nil {
		t.Fatal("invalid command accepted")
	}
}
