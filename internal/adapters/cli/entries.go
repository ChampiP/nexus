package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"nexus/internal/adapters/projectlist"
	"nexus/internal/catalog"
	"nexus/internal/tracking"
)

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
				return commandError(fmt.Errorf("%s requiere un valor", args[i]), jsonMode, stdout)
			}
			i++
			if args[i-1] == "-p" || args[i-1] == "--project" {
				input.Project = args[i]
			} else {
				input.Description = args[i]
			}
		default:
			if strings.HasPrefix(args[i], "-") {
				return commandError(fmt.Errorf("opción desconocida %q", args[i]), jsonMode, stdout)
			}
			titles = append(titles, args[i])
		}
	}
	if len(titles) != 1 {
		return commandError(fmt.Errorf("uso: nexus start <título> [-p proyecto] [-d descripción]"), jsonMode, stdout)
	}
	input.Title = titles[0]
	entry, err := tracker.Start(input)
	if err != nil {
		return commandError(err, jsonMode, stdout)
	}
	if jsonMode {
		return json.NewEncoder(stdout).Encode(createdEntry{entry.ID, entry.Title, entry.Project, entry.Description, entry.StartedAt})
	}
	_, err = fmt.Fprintf(stdout, "iniciado #%d %s\n", entry.ID, entry.Title)
	return err
}

func commandError(err error, jsonMode bool, stdout io.Writer) error {
	if err == nil {
		return nil
	}
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
		output.Recent = append(output.Recent, recentEntry{entry.ID, entry.Title, entry.Project, entry.StartedAt, entry.EndedAt, seconds, entry.UID, entry.Description})
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

// runProjects lista los proyectos elegibles: catálogo no archivado combinado con el uso registrado.
func runProjects(args []string, tracker Tracker, cat Catalog, stdout io.Writer) error {
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
	var tree []catalog.TreeOrganization
	if cat != nil {
		var err error
		if tree, err = cat.Tree(); err != nil {
			return err
		}
	}
	projects := projectlist.Build(tree, tracker.Projects(""), *query)
	if *jsonMode {
		out := make([]projectOutput, 0, len(projects))
		for _, project := range projects {
			out = append(out, projectOutput{project.Name, project.LastUsed, project.Seconds, project.Client, project.Organization})
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

// Opciones de los comandos nuevos: cada mapa asocia la opción con su nombre canónico.
var (
	editValueFlags = map[string]string{"-t": "title", "--title": "title", "-p": "project", "--project": "project", "-d": "description", "--description": "description"}
	jsonFlags      = map[string]string{"--json": "json"}
)

// parsedArgs es el resultado de parseArgs: argumentos posicionales, opciones con valor y opciones booleanas.
type parsedArgs struct {
	positional []string
	values     map[string]string
	flags      map[string]bool
}

// parseArgs separa argumentos posicionales y opciones en cualquier orden. Las opciones con valor
// distinguen "ausente" de "vacío", algo que el paquete flag no permite junto a posicionales previos.
func parseArgs(args []string, valueFlags, boolFlags map[string]string) (parsedArgs, error) {
	parsed := parsedArgs{values: map[string]string{}, flags: map[string]bool{}}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if name, ok := valueFlags[arg]; ok {
			if i+1 >= len(args) {
				return parsed, fmt.Errorf("%s requiere un valor", arg)
			}
			i++
			parsed.values[name] = args[i]
		} else if name, ok := boolFlags[arg]; ok {
			parsed.flags[name] = true
		} else if strings.HasPrefix(arg, "-") && arg != "-" {
			return parsed, fmt.Errorf("opción desconocida %q", arg)
		} else {
			parsed.positional = append(parsed.positional, arg)
		}
	}
	return parsed, nil
}

func parseID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("id inválido «%s»", value)
	}
	return id, nil
}

func runEdit(args []string, tracker Tracker, stdout io.Writer) error {
	jsonMode := contains(args, "--json")
	parsed, err := parseArgs(args, editValueFlags, jsonFlags)
	const usage = "uso: nexus edit <id> [-t título] [-p proyecto] [-d descripción] [--json]"
	if err != nil {
		return commandError(err, jsonMode, stdout)
	}
	if len(parsed.positional) != 1 || len(parsed.values) == 0 {
		return commandError(fmt.Errorf(usage), jsonMode, stdout)
	}
	id, err := parseID(parsed.positional[0])
	if err != nil {
		return commandError(err, jsonMode, stdout)
	}
	var input tracking.EditInput
	if v, ok := parsed.values["title"]; ok {
		input.Title = &v
	}
	if v, ok := parsed.values["project"]; ok {
		input.Project = &v
	}
	if v, ok := parsed.values["description"]; ok {
		input.Description = &v
	}
	entry, err := tracker.Edit(id, input)
	if err != nil {
		return commandError(localize(err), jsonMode, stdout)
	}
	if jsonMode {
		return json.NewEncoder(stdout).Encode(presentEntry(entry))
	}
	_, err = fmt.Fprintf(stdout, "editado #%d %s\n", entry.ID, entry.Title)
	return err
}

// runEntryID ejecuta rm y restore, que comparten forma: un id y --json.
func runEntryID(name string, args []string, apply func(int64) error, text func(int64) string, stdout io.Writer) error {
	jsonMode := contains(args, "--json")
	parsed, err := parseArgs(args, nil, jsonFlags)
	if err == nil && len(parsed.positional) != 1 {
		err = fmt.Errorf("uso: nexus %s <id> [--json]", name)
	}
	var id int64
	if err == nil {
		id, err = parseID(parsed.positional[0])
	}
	if err == nil {
		err = apply(id)
	}
	if err != nil {
		return commandError(localize(err), jsonMode, stdout)
	}
	if jsonMode {
		return json.NewEncoder(stdout).Encode(okOutput{OK: true})
	}
	_, err = fmt.Fprintln(stdout, text(id))
	return err
}

func runRemove(args []string, tracker Tracker, stdout io.Writer) error {
	return runEntryID("rm", args, tracker.Delete, func(id int64) string {
		return fmt.Sprintf("eliminado #%d (restaurar: nexus restore %d)", id, id)
	}, stdout)
}

func runRestore(args []string, tracker Tracker, stdout io.Writer) error {
	return runEntryID("restore", args, tracker.Restore, func(id int64) string { return fmt.Sprintf("restaurado #%d", id) }, stdout)
}

// trashLimit acota cuántas tareas muestra la papelera.
const trashLimit = 100

func runTrash(args []string, tracker Tracker, stdout io.Writer) error {
	jsonMode := contains(args, "--json")
	parsed, err := parseArgs(args, nil, jsonFlags)
	if err == nil && len(parsed.positional) != 0 {
		err = fmt.Errorf("uso: nexus trash [--json]")
	}
	var entries []tracking.Entry
	if err == nil {
		entries, err = tracker.Deleted(trashLimit)
	}
	if err != nil {
		return commandError(localize(err), jsonMode, stdout)
	}
	if jsonMode {
		out := make([]entryOutput, 0, len(entries))
		for _, entry := range entries {
			out = append(out, presentEntry(entry))
		}
		return json.NewEncoder(stdout).Encode(out)
	}
	if len(entries) == 0 {
		_, err = fmt.Fprintln(stdout, "Papelera vacía")
		return err
	}
	for _, entry := range entries {
		project := ""
		if entry.Project != "" {
			project = " (" + entry.Project + ")"
		}
		// Se muestra cuándo se borró, que es lo que importa para restaurar a tiempo.
		deleted := entry.StartedAt
		if entry.DeletedAt != nil {
			deleted = *entry.DeletedAt
		}
		if _, err := fmt.Fprintf(stdout, "#%d %s%s · borrada %s\n", entry.ID, entry.Title, project, time.Unix(deleted, 0).Format("2006-01-02 15:04")); err != nil {
			return err
		}
	}
	return nil
}
