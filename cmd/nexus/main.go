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
	tracker, closeDB, err := openTracker(path)
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
	return cli.Run(args, tracker, os.Stdout, os.Stderr)
}

// openTracker opens and migrates the database at path and builds the tracking use cases over it.
func openTracker(path string) (*tracking.Tracker, func() error, error) {
	db, err := platformdb.Open(path)
	if err != nil {
		return nil, nil, err
	}
	_, backup, err := migrateAndReconcile(db, path, time.Now)
	if backup != "" {
		fmt.Fprintln(os.Stderr, "nexus: copia de seguridad en", backup)
	}
	if err != nil {
		db.Close()
		return nil, nil, err
	}
	return tracking.NewTracker(tracking.NewSQLite(db), time.Now), db.Close, nil
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
