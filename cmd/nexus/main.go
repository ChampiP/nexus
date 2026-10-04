package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"nexus/internal/adapters/cli"
	"nexus/internal/adapters/tui"
	"nexus/internal/catalog"
	platformdb "nexus/internal/platform/db"
	"nexus/internal/tracking"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "nexus:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	path, err := databasePath()
	if err != nil {
		return err
	}
	tracker, catalogService, closeDB, err := openApp(path)
	if err != nil {
		if len(args) > 0 && args[0] == "status" && hasJSONFlag(args[1:]) {
			return cli.Run(args, nil, os.Stdout, os.Stderr)
		}
		return err
	}
	defer closeDB()
	if len(args) == 0 {
		return tui.Run(tracker)
	}
	return cli.RunWithCatalog(args, tracker, catalogService, os.Stdout, os.Stderr)
}

// openApp abre y migra la base de datos en path y construye los casos de uso de seguimiento y catálogo.
func openApp(path string) (*tracking.Tracker, *catalog.Service, func() error, error) {
	db, err := platformdb.Open(path)
	if err != nil {
		return nil, nil, nil, err
	}
	_, backup, err := migrateAndReconcile(db, path, time.Now)
	if backup != "" {
		fmt.Fprintln(os.Stderr, "nexus: copia de seguridad en", backup)
	}
	if err != nil {
		db.Close()
		return nil, nil, nil, err
	}
	tracker, catalogService := wire(db, time.Now)
	if _, err := tracker.Purge(trashRetention); err != nil {
		fmt.Fprintln(os.Stderr, "nexus: no se pudo purgar la papelera:", err)
	}
	return tracker, catalogService, db.Close, nil
}

// trashRetention is how long soft-deleted entries stay restorable.
const trashRetention = 30 * 24 * time.Hour

// wire builds the module use cases over a migrated database and connects them through their ports.
func wire(db *sql.DB, now func() time.Time) (*tracking.Tracker, *catalog.Service) {
	catalogRepo := catalog.NewSQLite(db)
	tracker := tracking.NewTracker(tracking.NewSQLite(db), now, tracking.WithProjects(catalogProjects{catalogRepo}))
	return tracker, catalog.NewService(catalogRepo, trackerEntries{tracker}, now)
}

// trackerEntries adapts tracking to the port catalog uses to keep entries' project columns in sync.
type trackerEntries struct{ tracker *tracking.Tracker }

func (t trackerEntries) Relink(fromID, toID int64, toName string) error {
	return t.tracker.Relink(fromID, toID, toName)
}

func (t trackerEntries) RenameProject(id int64, name string) error {
	return t.tracker.RenameProject(id, name)
}

// catalogProjects adapts the catalog to the port tracking uses to link entries to projects.
type catalogProjects struct{ catalog *catalog.SQLite }

func (c catalogProjects) EnsureProject(name string) (int64, error) {
	project, err := c.catalog.EnsureProject(name)
	return project.ID, err
}

// migrateAndReconcile applies every module's migrations (tracking v2 needs the catalog tables)
// and then completes rows written by older binaries.
func migrateAndReconcile(db *sql.DB, path string, now func() time.Time) (int, string, error) {
	trackingSteps := tracking.Migrations()
	migrations := append([]platformdb.Migration{trackingSteps[0]}, catalog.Migrations()...)
	migrations = append(migrations, trackingSteps[1:]...)
	applied, backup, err := platformdb.Migrate(db, path, migrations, now)
	if err != nil {
		return applied, backup, err
	}
	err = tracking.NewSQLite(db).Reconcile(catalogProjects{catalog.NewSQLite(db)})
	return applied, backup, err
}

func hasJSONFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--json" {
			return true
		}
	}
	return false
}

func databasePath() (string, error) {
	if path := os.Getenv("NEXUS_DB"); path != "" {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	dir := filepath.Join(home, ".local", "share", "nexus")
	// Nexus owns its default data directory, so it is made private even if an older
	// version created it world-readable. Failure is not fatal: the files are 0600.
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		_ = os.Chmod(dir, 0o700)
	}
	return filepath.Join(dir, "nexus.db"), nil
}
