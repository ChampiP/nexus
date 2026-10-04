package catalog

import (
	"path/filepath"
	"testing"
	"time"

	platformdb "nexus/internal/platform/db"
)

func openCatalog(t *testing.T) *SQLite {
	t.Helper()
	path := filepath.Join(t.TempDir(), "n.db")
	db, err := platformdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, _, err := platformdb.Migrate(db, path, Migrations(), time.Now); err != nil {
		t.Fatal(err)
	}
	return NewSQLite(db)
}

func TestEnsureProjectCreatesOnceCaseInsensitively(t *testing.T) {
	c := openCatalog(t)
	first, err := c.EnsureProject("Nexus")
	if err != nil || first.ID == 0 || len(first.UID) != 26 || first.Name != "Nexus" {
		t.Fatalf("EnsureProject = %+v, %v", first, err)
	}
	again, err := c.EnsureProject("nEXUS")
	if err != nil || again.ID != first.ID || again.Name != "Nexus" || again.UID != first.UID {
		t.Fatalf("EnsureProject(case) = %+v, %v", again, err)
	}
	other, err := c.EnsureProject("otro")
	if err != nil || other.ID == first.ID {
		t.Fatalf("EnsureProject(other) = %+v, %v", other, err)
	}
}
