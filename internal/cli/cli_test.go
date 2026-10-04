package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nexus/internal/app"
	"nexus/internal/store"
)

func testTracker(t *testing.T) *app.Tracker {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "nexus.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return app.NewTracker(db, func() time.Time { return time.Date(2025, 3, 4, 12, 0, 0, 0, time.Local) })
}

func invoke(t *testing.T, tracker Tracker, args ...string) (string, error) {
	t.Helper()
	var out, stderr bytes.Buffer
	err := Run(args, tracker, &out, &stderr)
	return out.String(), err
}

func TestStartStopAndProjectsSpanishText(t *testing.T) {
	tracker := testTracker(t)
	out, err := invoke(t, tracker, "start", "Informe", "-p", "Trabajo")
	if err != nil || !strings.HasPrefix(out, "iniciado #1 Informe") {
		t.Fatalf("start = %q, %v", out, err)
	}
	out, err = invoke(t, tracker, "start", "Reunión", "-p", "Personal")
	if err != nil || !strings.HasPrefix(out, "iniciado #2 Reunión") {
		t.Fatalf("start = %q, %v", out, err)
	}
	out, err = invoke(t, tracker, "projects")
	if err != nil || out != "Personal\nTrabajo\n" {
		t.Fatalf("projects = %q, %v", out, err)
	}
	out, err = invoke(t, tracker, "stop", "1")
	if err != nil || out != "detenido #1\n" {
		t.Fatalf("stop = %q, %v", out, err)
	}
}

func TestJSONContracts(t *testing.T) {
	tracker := testTracker(t)
	out, err := invoke(t, tracker, "start", " Informe ", "-p", "Trabajo", "-d", "nota", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var created map[string]any
	if err := json.Unmarshal([]byte(out), &created); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"id", "title", "project", "description", "started_at"} {
		if _, ok := created[key]; !ok {
			t.Errorf("created JSON missing %q: %s", key, out)
		}
	}
	out, err = invoke(t, tracker, "status", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var status map[string]any
	if err := json.Unmarshal([]byte(out), &status); err != nil {
		t.Fatal(err)
	}
	if len(status) != 3 || status["count"] != float64(1) {
		t.Fatalf("status JSON = %#v", status)
	}
	out, err = invoke(t, tracker, "ls", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var listing map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &listing); err != nil {
		t.Fatal(err)
	}
	if len(listing) != 2 || len(listing["running"]) == 0 || len(listing["recent"]) == 0 {
		t.Fatalf("ls JSON = %s", out)
	}
	out, err = invoke(t, tracker, "projects", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var projects []map[string]any
	if err := json.Unmarshal([]byte(out), &projects); err != nil || len(projects) != 1 || projects[0]["name"] != "Trabajo" {
		t.Fatalf("projects JSON = %s, %v", out, err)
	}
	out, err = invoke(t, tracker, "stop", "--all", "--json")
	if err != nil || out != "{\"stopped\":1}\n" {
		t.Fatalf("stop JSON = %q, %v", out, err)
	}
}

func TestStartJSONErrorAndStatusWithoutDatabase(t *testing.T) {
	out, err := invoke(t, testTracker(t), "start", "  ", "--json")
	if err == nil || !strings.Contains(out, `"error"`) {
		t.Fatalf("start error = %q, %v", out, err)
	}
	out, err = invoke(t, nil, "status", "--json")
	if err != nil || out != "{\"running\":[],\"count\":0,\"today_seconds\":0}\n" {
		t.Fatalf("missing DB status = %q, %v", out, err)
	}
}

func TestErrorsAreLocalizedToSpanish(t *testing.T) {
	out, err := invoke(t, testTracker(t), "start", "  ", "--json")
	if err == nil || err.Error() != "el título no puede estar vacío" || !strings.Contains(out, "el título no puede estar vacío") {
		t.Fatalf("empty title: out=%q err=%v", out, err)
	}
	if _, err = invoke(t, testTracker(t), "stop", "999"); err == nil || err.Error() != "el temporizador no está en curso" {
		t.Fatalf("stop unknown: err=%v", err)
	}
}
