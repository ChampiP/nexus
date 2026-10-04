package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"nexus/internal/store"
)

type statusEntry struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Project   string `json:"project"`
	StartedAt int64  `json:"started_at"`
	Elapsed   string `json:"elapsed"`
}

type statusOutput struct {
	Running      []statusEntry `json:"running"`
	Count        int           `json:"count"`
	TodaySeconds int64         `json:"today_seconds"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "nexus:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Println("TUI not implemented yet")
		return nil
	}
	dbPath, err := databasePath()
	if err != nil {
		return err
	}
	command := args[0]
	if command == "status" {
		jsonMode := false
		for _, arg := range args[1:] {
			if arg == "--json" {
				jsonMode = true
			}
		}
		return status(dbPath, jsonMode)
	}
	s, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer s.Close()

	switch command {
	case "start":
		return start(s, args[1:])
	case "stop":
		return stop(s, args[1:])
	case "ls":
		return list(s)
	case "report":
		return report(s, args[1:])
	default:
		return fmt.Errorf("unknown command %q", command)
	}
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

func status(path string, jsonMode bool) error {
	output := statusOutput{Running: []statusEntry{}}
	s, err := store.Open(path)
	if err == nil {
		now := time.Now()
		for _, entry := range s.Running() {
			output.Running = append(output.Running, statusEntry{
				ID: entry.ID, Title: entry.Title, Project: entry.Project,
				StartedAt: entry.StartedAt, Elapsed: elapsed(now.Unix() - entry.StartedAt),
			})
		}
		output.Count = len(output.Running)
		for _, total := range s.Totals(dayStart(now)) {
			output.TodaySeconds += total.Seconds
		}
		_ = s.Close()
	}
	if jsonMode {
		return json.NewEncoder(os.Stdout).Encode(output)
	}
	fmt.Printf("%d running · %s today\n", output.Count, humanDuration(output.TodaySeconds))
	return nil
}

func start(s *store.Store, args []string) error {
	project, description, title, err := parseStartArgs(args)
	if err != nil {
		return err
	}
	entry, err := s.Start(title, project, description)
	if err != nil {
		return err
	}
	fmt.Printf("started #%d %s\n", entry.ID, entry.Title)
	return nil
}

func parseStartArgs(args []string) (project, description, title string, err error) {
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-p", "--project":
			if i+1 == len(args) {
				return "", "", "", fmt.Errorf("%s requires a value", args[i])
			}
			i++
			project = args[i]
		case "-d", "--description":
			if i+1 == len(args) {
				return "", "", "", fmt.Errorf("%s requires a value", args[i])
			}
			i++
			description = args[i]
		default:
			positional = append(positional, args[i])
		}
	}
	if len(positional) != 1 || strings.TrimSpace(positional[0]) == "" {
		return "", "", "", fmt.Errorf("usage: nexus start <title> [-p project] [-d description]")
	}
	return project, description, positional[0], nil
}

func stop(s *store.Store, args []string) error {
	all := false
	var id int64
	if len(args) == 1 && args[0] == "--all" {
		all = true
	} else if len(args) > 1 {
		return fmt.Errorf("usage: nexus stop [id] [--all]")
	} else if len(args) == 1 {
		parsed, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid timer id %q", args[0])
		}
		id = parsed
	}
	if all {
		count, err := s.StopAll()
		if err != nil {
			return err
		}
		fmt.Printf("stopped %d timers\n", count)
		return nil
	}
	if id == 0 {
		running := s.Running()
		if len(running) == 0 {
			return fmt.Errorf("no running timers")
		}
		id = running[len(running)-1].ID
	}
	if err := s.Stop(id); err != nil {
		return err
	}
	fmt.Printf("stopped #%d\n", id)
	return nil
}

func list(s *store.Store) error {
	now := time.Now()
	running := s.Running()
	fmt.Println("Running:")
	if len(running) == 0 {
		fmt.Println("  (none)")
	}
	for _, entry := range running {
		fmt.Printf("  #%d %-24s %s\n", entry.ID, entry.Title, elapsed(now.Unix()-entry.StartedAt))
	}
	fmt.Println("Recent today:")
	count := 0
	for _, entry := range s.Recent(100) {
		if entry.StartedAt < dayStart(now).Unix() {
			continue
		}
		ended := "running"
		if entry.EndedAt != nil {
			ended = (time.Duration(*entry.EndedAt-entry.StartedAt) * time.Second).String()
		}
		fmt.Printf("  #%d %-24s %s\n", entry.ID, entry.Title, ended)
		count++
	}
	if count == 0 {
		fmt.Println("  (none)")
	}
	return nil
}

func report(s *store.Store, args []string) error {
	week := false
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.BoolVar(&week, "week", false, "show the last seven days")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: nexus report [--week]")
	}
	since := dayStart(time.Now())
	if week {
		since = since.AddDate(0, 0, -6)
	}
	totals := s.Totals(since)
	if len(totals) == 0 {
		fmt.Println("No tracked time")
		return nil
	}
	for _, total := range totals {
		project := total.Project
		if project == "" {
			project = "(no project)"
		}
		fmt.Printf("%-24s %s\n", project, humanDuration(total.Seconds))
	}
	return nil
}

func dayStart(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
}

func elapsed(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	return fmt.Sprintf("%02d:%02d:%02d", seconds/3600, seconds/60%60, seconds%60)
}

func humanDuration(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	d := time.Duration(seconds) * time.Second
	if d >= time.Hour {
		hours := int64(d / time.Hour)
		minutes := int64(d/time.Minute) % 60
		if minutes > 0 {
			return fmt.Sprintf("%dh%dm", hours, minutes)
		}
		return fmt.Sprintf("%dh", hours)
	}
	if d >= time.Minute {
		return fmt.Sprintf("%dm", int64(d/time.Minute))
	}
	return fmt.Sprintf("%ds", seconds)
}
