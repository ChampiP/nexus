package cli

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"nexus/internal/tracking"
)

// failingSnapshotTracker falla al leer la instantánea; el resto delega en el tracker real.
type failingSnapshotTracker struct{ Tracker }

func (failingSnapshotTracker) Snapshot() ([]tracking.Entry, int64, []tracking.Entry, error) {
	return nil, 0, nil, errors.New("disco ilegible")
}

func TestStatusSnapshotErrorKeepsShapeAndReportsError(t *testing.T) {
	tracker := failingSnapshotTracker{testTracker(t)}
	out, err := invoke(t, tracker, "status", "--json")
	if err != nil {
		t.Fatalf("status --json err = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("JSON inválido %q: %v", out, err)
	}
	if msg, _ := got["error"].(string); msg == "" {
		t.Fatalf("falta error en %q", out)
	}
	for _, key := range []string{"running", "count", "today_seconds", "break"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("falta %q en %q", key, out)
		}
	}
	if out, err = invoke(t, tracker, "status"); err == nil || strings.Contains(out, "sin temporizadores") {
		t.Fatalf("status texto = %q, %v", out, err)
	}
}

func TestJSONErrorsGoToStdout(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"stop inexistente", []string{"stop", "999", "--json"}, "el temporizador no está en curso"},
		{"ls con fallo", []string{"ls", "--json"}, "disco ilegible"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tracker Tracker = testTracker(t)
			if tt.args[0] == "ls" {
				tracker = failingSnapshotTracker{tracker}
			}
			out, err := invoke(t, tracker, tt.args...)
			if err == nil || out != "{\"error\":\""+tt.want+"\"}\n" {
				t.Fatalf("out = %q, err = %v", out, err)
			}
		})
	}
}
