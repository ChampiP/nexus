// Package cli implements Nexus command parsing and output presenters.
package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"nexus/internal/tracking"
)

// Tracker describes the use cases required by the command line.
type Tracker interface {
	Start(tracking.StartInput) (tracking.Entry, error)
	Stop(int64) error
	StopLatest() error
	StopAll() (int, error)
	Snapshot() ([]tracking.Entry, int64, []tracking.Entry, error)
	Projects(string) []tracking.ProjectUsage
	Report(time.Time) ([]tracking.ProjectTotal, error)
}

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
type createdEntry struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Project     string `json:"project"`
	Description string `json:"description"`
	StartedAt   int64  `json:"started_at"`
}
type stoppedOutput struct {
	Stopped int `json:"stopped"`
}
type recentEntry struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Project   string `json:"project"`
	StartedAt int64  `json:"started_at"`
	EndedAt   *int64 `json:"ended_at"`
	Seconds   int64  `json:"seconds"`
}
type listOutput struct {
	Running []statusEntry `json:"running"`
	Recent  []recentEntry `json:"recent"`
}
type projectOutput struct {
	Name     string `json:"name"`
	LastUsed int64  `json:"last_used"`
	Seconds  int64  `json:"seconds"`
}
type errorOutput struct {
	Error string `json:"error"`
}

// Run executes a Nexus command. A nil tracker is supported by status --json so that
// status remains safe when the configured database cannot be opened.
func Run(args []string, tracker Tracker, stdout, stderr io.Writer) error {
	return localize(run(args, tracker, stdout, stderr))
}

// localize translates domain sentinel errors into user-facing Spanish; the domain keeps neutral text.
func localize(err error) error {
	switch {
	case errors.Is(err, tracking.ErrEmptyTitle):
		return errors.New("el título no puede estar vacío")
	case errors.Is(err, tracking.ErrNotRunning):
		return errors.New("el temporizador no está en curso")
	default:
		return err
	}
}

func run(args []string, tracker Tracker, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: nexus <start|stop|ls|status|report|projects>")
	}
	command, args := args[0], args[1:]
	if command == "status" {
		return runStatus(args, tracker, stdout)
	}
	if tracker == nil {
		return fmt.Errorf("tracker is unavailable")
	}
	switch command {
	case "start":
		return runStart(args, tracker, stdout)
	case "stop":
		return runStop(args, tracker, stdout)
	case "ls":
		return runList(args, tracker, stdout)
	case "projects":
		return runProjects(args, tracker, stdout)
	case "report":
		return runReport(args, tracker, stdout)
	default:
		return fmt.Errorf("comando desconocido %q", command)
	}
}

func runStart(args []string, tracker Tracker, stdout io.Writer) error {
	var input tracking.StartInput
	jsonMode := contains(args, "--json")
	var titles []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			jsonMode = true
		case "-p", "--project", "-d", "--description":
			if i+1 >= len(args) {
				return startError(fmt.Errorf("%s requiere un valor", args[i]), jsonMode, stdout)
			}
			i++
			if args[i-1] == "-p" || args[i-1] == "--project" {
				input.Project = args[i]
			} else {
				input.Description = args[i]
			}
		default:
			if strings.HasPrefix(args[i], "-") {
				return startError(fmt.Errorf("opción desconocida %q", args[i]), jsonMode, stdout)
			}
			titles = append(titles, args[i])
		}
	}
	if len(titles) != 1 {
		return startError(fmt.Errorf("uso: nexus start <título> [-p proyecto] [-d descripción]"), jsonMode, stdout)
	}
	input.Title = titles[0]
	entry, err := tracker.Start(input)
	if err != nil {
		return startError(err, jsonMode, stdout)
	}
	if jsonMode {
		return json.NewEncoder(stdout).Encode(createdEntry{entry.ID, entry.Title, entry.Project, entry.Description, entry.StartedAt})
	}
	_, err = fmt.Fprintf(stdout, "iniciado #%d %s\n", entry.ID, entry.Title)
	return err
}

func startError(err error, jsonMode bool, stdout io.Writer) error {
	if jsonMode {
		if encodeErr := json.NewEncoder(stdout).Encode(errorOutput{Error: localize(err).Error()}); encodeErr != nil {
			return encodeErr
		}
	}
	return err
}

func runStop(args []string, tracker Tracker, stdout io.Writer) error {
	all, jsonMode := false, false
	var positional []string
	for _, arg := range args {
		switch arg {
		case "--all":
			all = true
		case "--json":
			jsonMode = true
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) > 1 || (all && len(positional) != 0) {
		return fmt.Errorf("uso: nexus stop [id] [--all]")
	}
	count := 1
	var err error
	var stoppedID int64
	if all {
		count, err = tracker.StopAll()
	} else if len(positional) == 0 {
		running, _, _, snapshotErr := tracker.Snapshot()
		if snapshotErr != nil {
			return snapshotErr
		}
		if len(running) > 0 {
			stoppedID = running[len(running)-1].ID
		}
		err = tracker.StopLatest()
	} else {
		stoppedID, err = strconv.ParseInt(positional[0], 10, 64)
		if err == nil {
			err = tracker.Stop(stoppedID)
		}
	}
	if err != nil {
		return err
	}
	if jsonMode {
		return json.NewEncoder(stdout).Encode(stoppedOutput{Stopped: count})
	}
	if all {
		_, err = fmt.Fprintf(stdout, "detenidos %d\n", count)
	} else {
		_, err = fmt.Fprintf(stdout, "detenido #%d\n", stoppedID)
	}
	return err
}

func runStatus(args []string, tracker Tracker, stdout io.Writer) error {
	jsonMode := contains(args, "--json")
	if hasUnknownStatusArgument(args) && !jsonMode {
		return fmt.Errorf("uso: nexus status [--json]")
	}
	output := statusOutput{Running: []statusEntry{}}
	if tracker != nil {
		running, today, _, err := tracker.Snapshot()
		if err == nil {
			output.TodaySeconds = today
			output.Count = len(running)
			for _, entry := range running {
				output.Running = append(output.Running, presentStatus(entry, time.Now()))
			}
		}
	}
	if jsonMode {
		return json.NewEncoder(stdout).Encode(output)
	}
	if output.Count == 0 {
		_, err := fmt.Fprintf(stdout, "En curso: sin temporizadores\nHoy: %s\n", humanDuration(output.TodaySeconds))
		return err
	}
	if _, err := fmt.Fprintf(stdout, "En curso: %d\nHoy: %s\n", output.Count, humanDuration(output.TodaySeconds)); err != nil {
		return err
	}
	for _, entry := range output.Running {
		if _, err := fmt.Fprintf(stdout, "  #%d %s %s\n", entry.ID, entry.Title, entry.Elapsed); err != nil {
			return err
		}
	}
	return nil
}

func runList(args []string, tracker Tracker, stdout io.Writer) error {
	jsonMode := contains(args, "--json")
	if len(args) > 0 && (!jsonMode || len(args) != 1) {
		return fmt.Errorf("uso: nexus ls [--json]")
	}
	running, _, recent, err := tracker.Snapshot()
	if err != nil {
		return err
	}
	now := time.Now()
	output := listOutput{Running: make([]statusEntry, 0, len(running)), Recent: make([]recentEntry, 0, len(recent))}
	for _, entry := range running {
		output.Running = append(output.Running, presentStatus(entry, now))
	}
	for _, entry := range recent {
		seconds := now.Unix() - entry.StartedAt
		if entry.EndedAt != nil {
			seconds = *entry.EndedAt - entry.StartedAt
		}
		output.Recent = append(output.Recent, recentEntry{entry.ID, entry.Title, entry.Project, entry.StartedAt, entry.EndedAt, seconds})
	}
	if jsonMode {
		return json.NewEncoder(stdout).Encode(output)
	}
	if len(running) == 0 {
		if _, err := fmt.Fprintln(stdout, "En curso: sin temporizadores"); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(stdout, "En curso:"); err != nil {
			return err
		}
		for _, entry := range output.Running {
			if _, err := fmt.Fprintf(stdout, "  #%d %s %s\n", entry.ID, entry.Title, entry.Elapsed); err != nil {
				return err
			}
		}
	}
	return nil
}

func runProjects(args []string, tracker Tracker, stdout io.Writer) error {
	fs := flag.NewFlagSet("projects", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	query := fs.String("q", "", "query")
	fs.StringVar(query, "query", "", "query")
	jsonMode := fs.Bool("json", false, "json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("uso: nexus projects [--json] [-q consulta]")
	}
	projects := tracker.Projects(*query)
	if *jsonMode {
		out := make([]projectOutput, 0, len(projects))
		for _, project := range projects {
			out = append(out, projectOutput{project.Name, project.LastUsed, project.Seconds})
		}
		return json.NewEncoder(stdout).Encode(out)
	}
	for _, project := range projects {
		if _, err := fmt.Fprintln(stdout, project.Name); err != nil {
			return err
		}
	}
	return nil
}

func runReport(args []string, tracker Tracker, stdout io.Writer) error {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	week := fs.Bool("week", false, "last seven days")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("uso: nexus report [--week]")
	}
	now := time.Now()
	since := dayStart(now)
	if *week {
		since = since.AddDate(0, 0, -6)
	}
	totals, err := tracker.Report(since)
	if err != nil {
		return err
	}
	if len(totals) == 0 {
		_, err = fmt.Fprintln(stdout, "Sin tiempo registrado")
		return err
	}
	for _, total := range totals {
		name := total.Project
		if name == "" {
			name = "Sin proyecto"
		}
		if _, err := fmt.Fprintf(stdout, "%-24s %s\n", name, humanDuration(total.Seconds)); err != nil {
			return err
		}
	}
	return nil
}

func presentStatus(entry tracking.Entry, now time.Time) statusEntry {
	return statusEntry{ID: entry.ID, Title: entry.Title, Project: entry.Project, StartedAt: entry.StartedAt, Elapsed: elapsed(now.Unix() - entry.StartedAt)}
}
func contains(args []string, value string) bool {
	for _, arg := range args {
		if arg == value {
			return true
		}
	}
	return false
}
func hasUnknownStatusArgument(args []string) bool {
	for _, arg := range args {
		if arg != "--json" {
			return true
		}
	}
	return false
}
func dayStart(now time.Time) time.Time {
	y, m, d := now.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, now.Location())
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
		h, m := int64(d/time.Hour), int64(d/time.Minute)%60
		if m > 0 {
			return fmt.Sprintf("%dh%dm", h, m)
		}
		return fmt.Sprintf("%dh", h)
	}
	if d >= time.Minute {
		return fmt.Sprintf("%dm", int64(d/time.Minute))
	}
	return fmt.Sprintf("%ds", seconds)
}
