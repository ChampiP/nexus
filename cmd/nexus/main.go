package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"nexus/internal/adapters/cli"
	"nexus/internal/adapters/tui"
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

// openTracker opens the database at path and builds the tracking use cases over it.
func openTracker(path string) (*tracking.Tracker, func() error, error) {
	db, err := platformdb.Open(path)
	if err != nil {
		return nil, nil, err
	}
	repository, err := tracking.NewSQLite(db)
	if err != nil {
		db.Close()
		return nil, nil, err
	}
	return tracking.NewTracker(repository, time.Now), db.Close, nil
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
	return filepath.Join(home, ".local", "share", "nexus", "nexus.db"), nil
}
