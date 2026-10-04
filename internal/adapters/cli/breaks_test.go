package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nexus/internal/catalog"
	"nexus/internal/countdown"
	platformdb "nexus/internal/platform/db"
	"nexus/internal/tracking"
)

// breakTimers replica el cableado de cmd/nexus entre countdown y tracking.
type breakTimers struct{ tracker *tracking.Tracker }

func (t breakTimers) StopMany(ids []int64) error { return t.tracker.StopMany(ids) }
func (t breakTimers) StartBreak(label string) (int64, error) {
	e, err := t.tracker.Start(tracking.StartInput{Title: label, Kind: tracking.KindBreak})
	return e.ID, err
}
func (t breakTimers) Stop(id int64) error { return t.tracker.Stop(id) }
func (t breakTimers) StartLike(id int64) error {
	_, err := t.tracker.StartLike(id)
	return err
}
func (t breakTimers) Running(id int64) (bool, error) { return t.tracker.IsRunning(id) }

type breakApp struct {
	tracker *tracking.Tracker
	opts    Options
	now     *time.Time
}

func newBreakApp(t *testing.T) breakApp {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nexus.db")
	db, err := platformdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	steps := tracking.Migrations()
	migrations := append([]platformdb.Migration{steps[0]}, catalog.Migrations()...)
	migrations = append(migrations, steps[1:]...)
	migrations = append(migrations, countdown.Migrations()...)
	if _, _, err := platformdb.Migrate(db, path, migrations, time.Now); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2025, 3, 4, 15, 0, 0, 0, time.Local)
	clk := func() time.Time { return now }
	repo := catalog.NewSQLite(db)
	tracker := tracking.NewTracker(tracking.NewSQLite(db), clk, tracking.WithProjects(testProjects{repo}))
	svc := countdown.NewService(countdown.NewSQLite(db), breakTimers{tracker}, clk)
	a := breakApp{tracker: tracker, now: &now}
	a.opts = Options{Catalog: catalog.NewService(repo, testEntries{tracker}, clk), Breaks: svc, Now: clk}
	return a
}

func (a breakApp) run(args ...string) (string, error) {
	var out, errOut bytes.Buffer
	err := RunWithOptions(args, a.tracker, a.opts, &out, &errOut)
	return out.String(), err
}

func (a breakApp) must(t *testing.T, args ...string) string {
	t.Helper()
	out, err := a.run(args...)
	if err != nil {
		t.Fatalf("%v: %q, %v", args, out, err)
	}
	return out
}

func (a breakApp) twoTimers(t *testing.T) {
	t.Helper()
	a.must(t, "start", "Informe")
	a.must(t, "start", "Diseño")
}

func TestBreakStopGuardChangesNothing(t *testing.T) {
	a := newBreakApp(t)
	a.twoTimers(t)
	out, err := a.run("break", "30")
	want := "Hay 2 temporizadores en curso: #1 Informe, #2 Diseño. Repite con --stop all, --stop none o --stop 1,2."
	if err == nil || err.Error() != want {
		t.Fatalf("guard err = %v, %q", err, out)
	}
	if b, _ := a.opts.Breaks.Active(); b != nil {
		t.Fatal("el guard no debe crear un break")
	}
	if running, _, _, _ := a.tracker.Snapshot(); len(running) != 2 {
		t.Fatalf("los timers no deben cambiar: %d", len(running))
	}
}

func TestBreakStopVariants(t *testing.T) {
	cases := []struct {
		stop        string
		wantRunning int
		wantText    string
	}{
		{"all", 0, "Break de 30 min hasta las 15:30 · detenidos: #1 Informe, #2 Diseño\n"},
		{"none", 2, "Break de 30 min hasta las 15:30\n"},
		{"2", 1, "Break de 30 min hasta las 15:30 · detenidos: #2 Diseño\n"},
	}
	for _, c := range cases {
		a := newBreakApp(t)
		a.twoTimers(t)
		wantOut(t, a.must(t, "break", "30", "--stop", c.stop), c.wantText)
		if running, _, _, _ := a.tracker.Snapshot(); len(running) != c.wantRunning {
			t.Fatalf("--stop %s: en curso = %d, se esperaba %d", c.stop, len(running), c.wantRunning)
		}
	}
}

func TestBreakWithoutTimersNeedsNoStopAndDefaultsTo60(t *testing.T) {
	a := newBreakApp(t)
	wantOut(t, a.must(t, "break", "-m", "Café"), "Break de 60 min hasta las 16:00\n")
	b, _ := a.opts.Breaks.Active()
	if b == nil || b.Label != "Café" {
		t.Fatalf("break = %+v", b)
	}
}

func TestBreakStopRejectsNonRunningID(t *testing.T) {
	a := newBreakApp(t)
	a.twoTimers(t)
	if _, err := a.run("break", "--stop", "9"); err == nil || !strings.Contains(err.Error(), "#9") {
		t.Fatalf("err = %v", err)
	}
}

func TestBreakStatusExtendEnd(t *testing.T) {
	a := newBreakApp(t)
	wantOut(t, a.must(t, "break", "status"), "Sin break activo\n")
	a.must(t, "start", "Informe")
	a.must(t, "break", "45", "--stop", "all")
	*a.now = a.now.Add(2*time.Minute + 50*time.Second)
	wantOut(t, a.must(t, "break", "status"), "Break: quedan 0:42:10 (hasta las 15:45)\n")
	wantOut(t, a.must(t, "break", "extend"), "Break extendido hasta las 15:55\n")
	wantOut(t, a.must(t, "break", "extend", "5"), "Break extendido hasta las 16:00\n")
	*a.now = time.Date(2025, 3, 4, 16, 12, 33, 0, time.Local)
	wantOut(t, a.must(t, "break", "status"), "Break excedido por 0:12:33\n")
	out := a.must(t, "break", "end")
	if !strings.HasPrefix(out, "Break terminado · reanudados: #") || !strings.Contains(out, "Informe") {
		t.Fatalf("end = %q", out)
	}
	wantOut(t, a.must(t, "break", "status"), "Sin break activo\n")
}

func TestBreakEndNoResume(t *testing.T) {
	a := newBreakApp(t)
	a.must(t, "start", "Informe")
	a.must(t, "break", "10", "--stop", "all")
	wantOut(t, a.must(t, "break", "end", "--no-resume"), "Break terminado\n")
	if running, _, _, _ := a.tracker.Snapshot(); len(running) != 0 {
		t.Fatal("no debe retomar timers")
	}
}

func TestBreakErrorsAreLocalized(t *testing.T) {
	a := newBreakApp(t)
	for args, want := range map[string]string{
		"end":    "No hay un break activo",
		"extend": "No hay un break activo",
		"0":      "La duración debe estar entre 1 minuto y 8 horas",
		"481":    "La duración debe estar entre 1 minuto y 8 horas",
	} {
		argv := []string{"break", args}
		if _, err := a.run(argv...); err == nil || err.Error() != want {
			t.Fatalf("%v: err = %v, se esperaba %q", argv, err, want)
		}
	}
	a.must(t, "break", "5")
	if _, err := a.run("break", "5"); err == nil || err.Error() != "Ya hay un break en curso" {
		t.Fatalf("break activo: %v", err)
	}
}

func TestStatusJSONBreakField(t *testing.T) {
	a := newBreakApp(t)
	wantOut(t, a.must(t, "status", "--json"), "{\"running\":[],\"count\":0,\"today_seconds\":0,\"break\":null}\n")
	a.must(t, "break", "10")
	*a.now = a.now.Add(12*time.Minute + 5*time.Second)
	var got struct {
		Break *breakOutput `json:"break"`
	}
	if err := json.Unmarshal([]byte(a.must(t, "status", "--json")), &got); err != nil || got.Break == nil {
		t.Fatalf("break = %+v, %v", got.Break, err)
	}
	b := got.Break
	if b.Label != "Break" || b.Overdue != "0:02:05" || b.OverdueSeconds != 125 || b.Remaining != "0:00:00" || b.RemainingSeconds != 0 {
		t.Fatalf("break = %+v", b)
	}
	if out := a.must(t, "status"); !strings.Contains(out, "Break excedido por 0:02:05") {
		t.Fatalf("status = %q", out)
	}
}

func TestStatusJSONWithoutDatabaseHasNullBreak(t *testing.T) {
	var out bytes.Buffer
	if err := RunWithOptions([]string{"status", "--json"}, nil, Options{}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	wantOut(t, out.String(), "{\"running\":[],\"count\":0,\"today_seconds\":0,\"break\":null}\n")
}

func TestLsLabelsBreakEntries(t *testing.T) {
	a := newBreakApp(t)
	a.must(t, "break", "5")
	a.must(t, "break", "end")
	if out := a.must(t, "ls", "--json"); !strings.Contains(out, "☕ Break") {
		t.Fatalf("ls = %q", out)
	}
}
