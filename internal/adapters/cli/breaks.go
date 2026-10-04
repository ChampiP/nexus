package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"nexus/internal/countdown"
	"nexus/internal/tracking"
)

// Breaks describe los casos de uso del break que necesita la línea de comandos.
type Breaks interface {
	StartBreak(duration time.Duration, label string, stopIDs []int64) (countdown.Break, error)
	Active() (*countdown.Break, error)
	Extend(time.Duration) error
	End(resume bool) (int, error)
}

const (
	breakDefaultMinutes = 60
	breakExtendMinutes  = 10
	breakEntryLabel     = "☕ Break"
)

type breakOutput struct {
	Label            string `json:"label"`
	StartedAt        int64  `json:"started_at"`
	EndsAt           int64  `json:"ends_at"`
	Remaining        string `json:"remaining"`
	Overdue          string `json:"overdue"`
	OverdueSeconds   int64  `json:"overdue_seconds"`
	RemainingSeconds int64  `json:"remaining_seconds"`
}

type breakStartOutput struct {
	Label     string        `json:"label"`
	StartedAt int64         `json:"started_at"`
	EndsAt    int64         `json:"ends_at"`
	Stopped   []stoppedItem `json:"stopped"`
}

type stoppedItem struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

type breakEndOutput struct {
	Ended   bool          `json:"ended"`
	Resumed []stoppedItem `json:"resumed"`
}

// presentBreak devuelve nil cuando no hay break activo, lo que se serializa como null.
func presentBreak(b *countdown.Break, now time.Time) *breakOutput {
	if b == nil {
		return nil
	}
	remaining, overdue := int64(b.Remaining(now)/time.Second), int64(b.Overdue(now)/time.Second)
	return &breakOutput{b.Label, b.StartedAt, b.EndsAt, clock(remaining), clock(overdue), overdue, remaining}
}

// clock formatea segundos como H:MM:SS.
func clock(seconds int64) string {
	return fmt.Sprintf("%d:%02d:%02d", seconds/3600, seconds/60%60, seconds%60)
}

func clockTime(unix int64) string { return time.Unix(unix, 0).Format("15:04") }

func breakStatusText(b countdown.Break, now time.Time) string {
	if over := b.Overdue(now); over > 0 {
		return "Break excedido por " + clock(int64(over/time.Second))
	}
	return fmt.Sprintf("Break: quedan %s (hasta las %s)", clock(int64(b.Remaining(now)/time.Second)), clockTime(b.EndsAt))
}

// recentProject muestra los breaks como "☕ Break" en lugar de un proyecto.
func recentProject(e tracking.Entry) string {
	if e.Kind == tracking.KindBreak {
		return breakEntryLabel
	}
	return e.Project
}

func runBreak(args []string, tracker Tracker, opts Options, stdout io.Writer) error {
	if opts.Breaks == nil {
		return fmt.Errorf("el break no está disponible")
	}
	if len(args) > 0 {
		switch args[0] {
		case "status":
			return runBreakStatus(args[1:], opts, stdout)
		case "extend":
			return runBreakExtend(args[1:], opts, stdout)
		case "end":
			return runBreakEnd(args[1:], tracker, opts, stdout)
		}
	}
	return runBreakStart(args, tracker, opts, stdout)
}

func parseMinutes(s string) (time.Duration, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("Los minutos deben ser un número entero")
	}
	if n < 1 || n > 8*60 {
		return 0, countdown.ErrInvalidDuration
	}
	return time.Duration(n) * time.Minute, nil
}

func runBreakStart(args []string, tracker Tracker, opts Options, stdout io.Writer) error {
	jsonMode := contains(args, "--json")
	minutes, label, stop, stopGiven := strconv.Itoa(breakDefaultMinutes), "", "", false
	minutesGiven := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
		case "-m", "--stop":
			if i+1 >= len(args) {
				return commandError(fmt.Errorf("%s requiere un valor", args[i]), jsonMode, stdout)
			}
			i++
			if args[i-1] == "-m" {
				label = args[i]
			} else {
				stop, stopGiven = args[i], true
			}
		default:
			if strings.HasPrefix(args[i], "-") || minutesGiven {
				return commandError(fmt.Errorf("uso: nexus break [minutos] [-m etiqueta] [--stop all|none|3,5] [--json]"), jsonMode, stdout)
			}
			minutes, minutesGiven = args[i], true
		}
	}
	duration, err := parseMinutes(minutes)
	if err != nil {
		return commandError(err, jsonMode, stdout)
	}
	if tracker == nil {
		return commandError(fmt.Errorf("el registro de tiempo no está disponible"), jsonMode, stdout)
	}
	// Un break activo se rechaza antes de pedir decisiones sobre los timers.
	if active, err := opts.Breaks.Active(); err != nil {
		return commandError(err, jsonMode, stdout)
	} else if active != nil {
		return commandError(countdown.ErrBreakActive, jsonMode, stdout)
	}
	running, _, _, err := tracker.Snapshot()
	if err != nil {
		return commandError(err, jsonMode, stdout)
	}
	if len(running) > 0 && !stopGiven {
		names := make([]string, len(running))
		ids := make([]string, len(running))
		for i, e := range running {
			names[i] = fmt.Sprintf("#%d %s", e.ID, e.Title)
			ids[i] = strconv.FormatInt(e.ID, 10)
		}
		noun := "temporizadores"
		if len(running) == 1 {
			noun = "temporizador"
		}
		return commandError(fmt.Errorf("Hay %d %s en curso: %s. Repite con --stop all, --stop none o --stop %s.", len(running), noun, strings.Join(names, ", "), strings.Join(ids, ",")), jsonMode, stdout)
	}
	toStop, err := selectStop(stop, running)
	if err != nil {
		return commandError(err, jsonMode, stdout)
	}
	ids := make([]int64, len(toStop))
	for i, e := range toStop {
		ids[i] = e.ID
	}
	b, err := opts.Breaks.StartBreak(duration, label, ids)
	if err != nil {
		return commandError(err, jsonMode, stdout)
	}
	if jsonMode {
		stopped := make([]stoppedItem, 0, len(toStop))
		for _, e := range toStop {
			stopped = append(stopped, stoppedItem{e.ID, e.Title})
		}
		return json.NewEncoder(stdout).Encode(breakStartOutput{b.Label, b.StartedAt, b.EndsAt, stopped})
	}
	text := fmt.Sprintf("Break de %d min hasta las %s", int(duration/time.Minute), clockTime(b.EndsAt))
	if len(toStop) > 0 {
		text += " · detenidos: " + entryList(toStop)
	}
	_, err = fmt.Fprintln(stdout, text)
	return err
}

func entryList(entries []tracking.Entry) string {
	parts := make([]string, len(entries))
	for i, e := range entries {
		parts[i] = fmt.Sprintf("#%d %s", e.ID, e.Title)
	}
	return strings.Join(parts, ", ")
}

// selectStop resuelve el valor de --stop contra los timers en curso.
func selectStop(value string, running []tracking.Entry) ([]tracking.Entry, error) {
	switch value {
	case "", "none":
		return nil, nil
	case "all":
		return running, nil
	}
	var picked []tracking.Entry
	for _, part := range strings.Split(value, ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("--stop acepta all, none o ids separados por comas (por ejemplo 3,5)")
		}
		found := false
		for _, e := range running {
			if e.ID == id {
				picked, found = append(picked, e), true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("el temporizador #%d no está en curso", id)
		}
	}
	return picked, nil
}

func runBreakStatus(args []string, opts Options, stdout io.Writer) error {
	jsonMode := contains(args, "--json")
	if len(args) > 1 || (len(args) == 1 && !jsonMode) {
		return fmt.Errorf("uso: nexus break status [--json]")
	}
	b, err := opts.Breaks.Active()
	if err != nil {
		return commandError(err, jsonMode, stdout)
	}
	now := opts.Now()
	if jsonMode {
		return json.NewEncoder(stdout).Encode(presentBreak(b, now))
	}
	text := "Sin break activo"
	if b != nil {
		text = breakStatusText(*b, now)
	}
	_, err = fmt.Fprintln(stdout, text)
	return err
}

func runBreakExtend(args []string, opts Options, stdout io.Writer) error {
	if len(args) > 1 {
		return fmt.Errorf("uso: nexus break extend [minutos]")
	}
	minutes := strconv.Itoa(breakExtendMinutes)
	if len(args) == 1 {
		minutes = args[0]
	}
	duration, err := parseMinutes(minutes)
	if err != nil {
		return err
	}
	if err := opts.Breaks.Extend(duration); err != nil {
		return err
	}
	b, err := opts.Breaks.Active()
	if err != nil {
		return err
	}
	if b == nil {
		return countdown.ErrNoActiveBreak
	}
	_, err = fmt.Fprintf(stdout, "Break extendido hasta las %s\n", clockTime(b.EndsAt))
	return err
}

func runBreakEnd(args []string, tracker Tracker, opts Options, stdout io.Writer) error {
	resume, jsonMode := true, false
	for _, arg := range args {
		switch arg {
		case "--resume":
			resume = true
		case "--no-resume":
			resume = false
		case "--json":
			jsonMode = true
		default:
			return fmt.Errorf("uso: nexus break end [--resume|--no-resume] [--json]")
		}
	}
	before := map[int64]bool{}
	if tracker != nil {
		running, _, _, err := tracker.Snapshot()
		if err != nil {
			return commandError(err, jsonMode, stdout)
		}
		for _, e := range running {
			before[e.ID] = true
		}
	}
	count, err := opts.Breaks.End(resume)
	if err != nil {
		return commandError(err, jsonMode, stdout)
	}
	// Los timers retomados son entradas nuevas: las que no estaban en curso antes de terminar.
	var resumed []tracking.Entry
	if count > 0 && tracker != nil {
		running, _, _, err := tracker.Snapshot()
		if err != nil {
			return commandError(err, jsonMode, stdout)
		}
		for _, e := range running {
			if !before[e.ID] {
				resumed = append(resumed, e)
			}
		}
	}
	if jsonMode {
		items := make([]stoppedItem, 0, len(resumed))
		for _, e := range resumed {
			items = append(items, stoppedItem{e.ID, e.Title})
		}
		return json.NewEncoder(stdout).Encode(breakEndOutput{true, items})
	}
	text := "Break terminado"
	if len(resumed) > 0 {
		text += " · reanudados: " + entryList(resumed)
	}
	_, err = fmt.Fprintln(stdout, text)
	return err
}
