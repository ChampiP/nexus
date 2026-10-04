package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"nexus/internal/adapters/cli"
	"nexus/internal/adapters/daemon"
	"nexus/internal/adapters/tui"
	"nexus/internal/catalog"
	"nexus/internal/countdown"
	platformdb "nexus/internal/platform/db"
	"nexus/internal/platform/notify"
	"nexus/internal/platform/presence"
	"nexus/internal/tracking"
	"nexus/internal/wellbeing"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "nexus:", err)
		os.Exit(1)
	}
}

// daemonTick es cada cuánto el daemon revisa si un break venció.
const daemonTick = 10 * time.Second

func run(args []string) error {
	path, err := databasePath()
	if err != nil {
		return err
	}
	tracker, catalogService, breaks, activePauses, closeDB, err := openApp(path)
	if err != nil {
		if len(args) > 0 && args[0] == "status" && hasJSONFlag(args[1:]) {
			return cli.Run(args, nil, os.Stdout, os.Stderr)
		}
		return err
	}
	defer closeDB()
	if len(args) == 1 && args[0] == "daemon" {
		return runDaemon(breaks, activePauses)
	}
	if len(args) == 0 {
		return tui.Run(tracker, catalogService, breaks, tui.WithWellbeing(activePauses), tui.WithTryPause(func() {
			if err := showPauseNow(activePauses); err != nil {
				fmt.Fprintln(os.Stderr, "nexus: no se pudo mostrar la pausa:", err)
			}
		}))
	}
	return cli.RunWithOptions(args, tracker, cli.Options{Catalog: catalogService, Breaks: breaks, Wellbeing: activePauses, Presenter: wellbeingPresenter{}}, os.Stdout, os.Stderr)
}

func showPauseNow(service *wellbeing.Service) error {
	now := time.Now()
	reminder, ok := service.Due(now)
	if !ok {
		settings, err := service.Settings()
		if err != nil {
			return err
		}
		reminder = wellbeing.Reminder{Duration: settings.Duration, Message: "Es momento de moverte un poco.", Tip: "Ponte de pie y estira la espalda."}
	}
	if err := service.MarkShown(now); err != nil {
		return err
	}
	result, err := (wellbeingPresenter{}).Show(context.Background(), reminder)
	if err != nil {
		return err
	}
	switch result {
	case "snooze":
		return service.Snooze(now, 10*time.Minute)
	case "skip":
		return service.Skip(now)
	default:
		return service.Done(now)
	}
}

// runDaemon corre el bucle de avisos hasta recibir SIGINT o SIGTERM; solo permite una instancia.
func runDaemon(breaks *countdown.Service, activePauses *wellbeing.Service) error {
	release, err := daemon.Lock(daemon.LockPath())
	if err != nil {
		return err
	}
	defer release()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	daemon.RunWithWellbeing(ctx, breaks, notifier{}, openNexus, activePauses, wellbeingPresenter{}, presence.New(nil), time.Now, daemonTick)
	return nil
}

// openNexus abre (o enfoca) el TUI de Nexus en una terminal, donde el usuario resuelve el break.
func openNexus() error {
	return exec.Command("omarchy-launch-or-focus-tui", "nexus").Start()
}

// notifier adapta notify.Send al puerto del daemon.
type notifier struct{}

func (notifier) Send(ctx context.Context, n notify.Notification) (string, error) {
	return notify.Send(ctx, n)
}

// openApp abre y migra la base de datos en path y construye los casos de uso de seguimiento, catálogo y break.
func openApp(path string) (*tracking.Tracker, *catalog.Service, *countdown.Service, *wellbeing.Service, func() error, error) {
	db, err := platformdb.Open(path)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	_, backup, err := migrateAndReconcile(db, path, time.Now)
	if backup != "" {
		fmt.Fprintln(os.Stderr, "nexus: copia de seguridad en", backup)
	}
	if err != nil {
		db.Close()
		return nil, nil, nil, nil, nil, err
	}
	tracker, catalogService, breaks := wire(db, time.Now)
	activePauses := wellbeing.NewService(wellbeing.NewSQLite(db), wellWork{tracker: tracker, breaks: breaks}, time.Now)
	if _, err := tracker.Purge(trashRetention); err != nil {
		fmt.Fprintln(os.Stderr, "nexus: no se pudo purgar la papelera:", err)
	}
	return tracker, catalogService, breaks, activePauses, db.Close, nil
}

// trashRetention is how long soft-deleted entries stay restorable.
const trashRetention = 30 * 24 * time.Hour

// wire builds the module use cases over a migrated database and connects them through their ports.
func wire(db *sql.DB, now func() time.Time) (*tracking.Tracker, *catalog.Service, *countdown.Service) {
	catalogRepo := catalog.NewSQLite(db)
	tracker := tracking.NewTracker(tracking.NewSQLite(db), now, tracking.WithProjects(catalogProjects{catalogRepo}))
	breaks := countdown.NewService(countdown.NewSQLite(db), trackerTimers{tracker}, now)
	return tracker, catalog.NewService(catalogRepo, trackerEntries{tracker}, now), breaks
}

type wellWork struct {
	tracker *tracking.Tracker
	breaks  *countdown.Service
}

func (w wellWork) WorkRunning() (bool, error) {
	entries, _, _, err := w.tracker.Snapshot()
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if e.Kind == tracking.KindWork {
			return true, nil
		}
	}
	return false, nil
}
func (w wellWork) BreakActive() (bool, error) { b, err := w.breaks.Active(); return b != nil, err }

// trackerTimers adapts tracking to the port countdown uses to control timers.
type trackerTimers struct{ tracker *tracking.Tracker }

func (t trackerTimers) StopMany(ids []int64) error { return t.tracker.StopMany(ids) }

func (t trackerTimers) StartBreak(label string) (int64, error) {
	entry, err := t.tracker.Start(tracking.StartInput{Title: label, Kind: tracking.KindBreak})
	return entry.ID, err
}

func (t trackerTimers) Stop(id int64) error { return t.tracker.Stop(id) }

// Resume reanuda la tarea de la entrada id; si ya tiene una sesión en curso no hay nada que hacer.
func (t trackerTimers) Resume(id int64) error {
	_, err := t.tracker.Resume(id)
	if errors.Is(err, tracking.ErrAlreadyRunning) {
		return nil
	}
	return err
}

func (t trackerTimers) Running(id int64) (bool, error) { return t.tracker.IsRunning(id) }

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

// orderedMigrations enumera el orden por dependencias entre módulos.
func orderedMigrations() []platformdb.Migration {
	trackingSteps := tracking.Migrations()
	catalogSteps := catalog.Migrations()
	countdownSteps := countdown.Migrations()
	wellbeingSteps := wellbeing.Migrations()
	return []platformdb.Migration{
		trackingSteps[0],  // tracking v1: esquema base
		catalogSteps[0],   // catalog v1: tablas referenciadas por tracking
		catalogSteps[1],   // catalog v2: unicidad de clientes
		trackingSteps[1],  // tracking v2: referencia projects
		trackingSteps[2],  // tracking v3: identidad de tareas
		trackingSteps[3],  // tracking v4: integridad e índices
		countdownSteps[0], // countdown v1
		countdownSteps[1], // countdown v2: referencia entries
		wellbeingSteps[0], // wellbeing v1
		wellbeingSteps[1], // wellbeing v2: registro de eventos
	}
}

// migrateAndReconcile applies module migrations in dependency order and then completes rows
// written by older binaries.
func migrateAndReconcile(db *sql.DB, path string, now func() time.Time) (int, string, error) {
	applied, backup, err := platformdb.Migrate(db, path, orderedMigrations(), now)
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
