package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"nexus/internal/app"
	"nexus/internal/cli"
	"nexus/internal/store"
	"nexus/internal/tui"
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
	if len(args) == 0 {
		db, err := store.Open(path)
		if err != nil {
			return err
		}
		defer db.Close()
		return tui.Run(app.NewTracker(db, time.Now))
	}
	db, openErr := store.Open(path)
	if openErr != nil {
		if args[0] == "status" && hasJSONFlag(args[1:]) {
			return cli.Run(args, nil, os.Stdout, os.Stderr)
		}
		return openErr
	}
	defer db.Close()
	return cli.Run(args, app.NewTracker(db, time.Now), os.Stdout, os.Stderr)
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
