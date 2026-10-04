package db

import (
	"os"
	"path/filepath"
	"testing"
)

// Open must never widen or narrow the permissions of a directory it did not create:
// NEXUS_DB may point into a shared or home directory.
func TestOpenLeavesExistingDirectoryPermissions(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := Open(filepath.Join(dir, "n.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Fatalf("existing directory mode = %o, want 755", got)
	}
}

func TestOpenCreatesPrivateDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new", "nexus")
	db, err := Open(filepath.Join(dir, "n.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("created directory mode = %o, want 700", got)
	}
}
