package daemon

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestHeartbeatStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nexus.db.alive")
	store := NewHeartbeat(path)
	if _, ok, err := store.Last(); err != nil || ok {
		t.Fatalf("missing: ok=%v err=%v", ok, err)
	}
	if err := os.WriteFile(path, []byte("garbage"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.Last(); err != nil || ok {
		t.Fatalf("garbage: ok=%v err=%v", ok, err)
	}
	for _, at := range []time.Time{t0, t0.Add(time.Minute)} {
		if err := store.Beat(at); err != nil {
			t.Fatal(err)
		}
		got, ok, err := store.Last()
		if err != nil || !ok || !got.Equal(at) {
			t.Fatalf("Last = %v %v %v", got, ok, err)
		}
		data, err := os.ReadFile(path)
		if err != nil || string(data) != strconv.FormatInt(at.Unix(), 10) {
			t.Fatalf("content = %q, err=%v", data, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("mode = %v", info.Mode())
		}
	}
	files, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(files) != 1 {
		t.Fatalf("files = %v, err=%v", files, err)
	}
}

func TestHeartbeatErrors(t *testing.T) {
	store := NewHeartbeat(filepath.Join(t.TempDir(), "missing", "nexus.db.alive"))
	if err := store.Beat(t0); err == nil {
		t.Fatal("expected write error")
	}
	store = NewHeartbeat(t.TempDir())
	if _, _, err := store.Last(); err == nil {
		t.Fatal("expected read error")
	}
}
