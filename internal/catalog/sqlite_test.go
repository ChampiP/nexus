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

func TestNullOrganizationClientUniquenessAndMergeMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "merge.db")
	db, err := platformdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	steps := Migrations()
	if _, _, err := platformdb.Migrate(db, path, steps[:1], time.Now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO clients(id,uid,name) VALUES(1,'one','Acme'),(2,'two','acme'),(3,'three',' Widget '),(4,'four','Widget'); INSERT INTO projects(id,uid,name,client_id) VALUES(1,'p1','Project 1',2)`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := platformdb.Migrate(db, path, steps, time.Now); err != nil {
		t.Fatal(err)
	}
	var count, clientID int
	if err := db.QueryRow(`SELECT COUNT(*) FROM clients WHERE organization_id IS NULL`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT client_id FROM projects WHERE id=1`).Scan(&clientID); err != nil {
		t.Fatal(err)
	}
	if count != 3 || clientID != 1 {
		t.Fatalf("clients=%d project client=%d", count, clientID)
	}
	var padded string
	if err := db.QueryRow(`SELECT name FROM clients WHERE id=3`).Scan(&padded); err != nil || padded != " Widget " {
		t.Fatalf("collision name = %q, %v", padded, err)
	}
	if _, err := db.Exec(`INSERT INTO clients(uid,name) VALUES('duplicate','ACME')`); err == nil {
		t.Fatal("duplicate NULL-org client accepted")
	}
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
