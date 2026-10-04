package cli

import (
	"bytes"
	"context"
	"nexus/internal/wellbeing"
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

func (pauseWork) WorkRunning() (bool, error) { return true, nil }
func (pauseWork) BreakActive() (bool, error) { return false, nil }

type pausePresenter struct{ result string }

func (p pausePresenter) Show(context.Context, wellbeing.Reminder) (string, error) {
	return p.result, nil
}
func TestWellbeingCLIStatusSettingsAndActions(t *testing.T) {
	now := time.Date(2025, 1, 2, 15, 0, 0, 0, time.Local)
	s := wellbeing.NewService(&pauseMem{map[string]string{}}, pauseWork{}, func() time.Time { return now })
	run := func(args ...string) (string, error) {
		var out bytes.Buffer
		err := RunWithOptions(append([]string{"pausa"}, args...), nil, Options{Wellbeing: s, Presenter: pausePresenter{}, Now: func() time.Time { return now }}, &out, &bytes.Buffer{})
		return out.String(), err
	}
	out, e := run()
	if e != nil || !strings.Contains(out, "Pausas activas: cada 30 min, 30 s") {
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
