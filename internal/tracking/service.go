package tracking

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// StartInput contains the fields needed to start a timer.
type StartInput struct {
	Title       string
	Project     string
	ProjectID   int64
	Description string
	Kind        string // "" equivale a "work"
}

// Tracker implements timer use cases.
type Tracker struct {
	repository Repository
	clock      Clock
	projects   ProjectResolver
}

// Option customizes a Tracker.
type Option func(*Tracker)

// WithProjects makes the Tracker link entries to catalog projects through resolver.
func WithProjects(resolver ProjectResolver) Option {
	return func(t *Tracker) { t.projects = resolver }
}

// NewTracker creates a Tracker using the supplied repository and clock.
func NewTracker(repository Repository, clock Clock, options ...Option) *Tracker {
	if clock == nil {
		clock = time.Now
	}
	t := &Tracker{repository: repository, clock: clock}
	for _, option := range options {
		option(t)
	}
	return t
}

// resolve returns the catalog id for a project name; 0 when empty or no resolver is configured.
func (t *Tracker) resolve(name string) (int64, error) {
	if name == "" || t.projects == nil {
		return 0, nil
	}
	id, err := t.projects.EnsureProject(name)
	if err != nil {
		if errors.Is(err, ErrAmbiguousProject) {
			return 0, ErrAmbiguousProject
		}
		return 0, fmt.Errorf("resolve project %q: %w", name, err)
	}
	return id, nil
}

// resolveProjectName obtiene el nombre del catálogo para un id; retorna cadena vacía si no hay resolver disponible.
func (t *Tracker) resolveProjectName(id int64) (string, error) {
	if id <= 0 || t.projects == nil {
		return "", nil
	}
	if pnr, ok := t.projects.(interface{ ProjectName(int64) (string, error) }); ok {
		name, err := pnr.ProjectName(id)
		if err != nil {
			return "", fmt.Errorf("resolve project name #%d: %w", id, err)
		}
		return name, nil
	}
	return "", nil
}

// resolveProjectAssignment determina el nombre de texto y el id del proyecto según los campos provistos.
func (t *Tracker) resolveProjectAssignment(currentText string, currentID int64, inProject *string, inProjectID *int64) (string, int64, error) {
	if inProjectID != nil {
		id := *inProjectID
		if id > 0 {
			name, err := t.resolveProjectName(id)
			if err != nil {
				return "", 0, err
			}
			if name != "" {
				return name, id, nil
			}
			if inProject != nil {
				return *inProject, id, nil
			}
			return currentText, id, nil
		}
		if inProject != nil {
			return *inProject, 0, nil
		}
		return "", 0, nil
	}
	if inProject != nil {
		text := *inProject
		if text == "" {
			return "", 0, nil
		}
		id, err := t.resolve(text)
		if err != nil {
			if errors.Is(err, ErrAmbiguousProject) {
				return "", 0, ErrAmbiguousProject
			}
			return "", 0, err
		}
		return text, id, nil
	}
	return currentText, currentID, nil
}

// Start creates a timer after trimming and validating its title.
func (t *Tracker) Start(input StartInput) (Entry, error) {
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		return Entry{}, ErrEmptyTitle
	}
	switch input.Kind {
	case "":
		input.Kind = KindWork
	case KindWork, KindBreak:
	default:
		return Entry{}, ErrInvalidKind
	}
	var projectID int64
	var projectName string
	if input.ProjectID > 0 {
		projectID = input.ProjectID
		name, err := t.resolveProjectName(projectID)
		if err != nil {
			return Entry{}, fmt.Errorf("start timer: %w", err)
		}
		if name != "" {
			projectName = name
		} else {
			projectName = input.Project
		}
	} else if input.Project != "" {
		id, err := t.resolve(input.Project)
		if err != nil {
			if errors.Is(err, ErrAmbiguousProject) {
				return Entry{}, ErrAmbiguousProject
			}
			return Entry{}, fmt.Errorf("start timer: %w", err)
		}
		projectID = id
		projectName = input.Project
	}
	entry, err := t.repository.Insert(Entry{
		Kind:        input.Kind,
		Title:       input.Title,
		Project:     projectName,
		ProjectID:   projectID,
		Description: input.Description,
		StartedAt:   t.clock().Unix(),
	})
	if err != nil {
		return Entry{}, fmt.Errorf("start timer: %w", err)
	}
	return entry, nil
}

// Resume inicia una sesión nueva de la tarea a la que pertenece la entrada id: misma tarea, otro intervalo.
// Si la tarea ya tiene una sesión en curso devuelve ErrAlreadyRunning junto con esa sesión.
func (t *Tracker) Resume(id int64) (Entry, error) {
	src, err := t.repository.Get(id)
	if err != nil {
		return Entry{}, fmt.Errorf("resume entry #%d: %w", id, err)
	}
	if src.Kind != KindWork {
		return Entry{}, ErrInvalidKind
	}
	if running, ok, err := t.repository.RunningInTask(src.TaskUID); err != nil {
		return Entry{}, fmt.Errorf("resume entry #%d: %w", id, err)
	} else if ok {
		return running, ErrAlreadyRunning
	}
	entry, err := t.repository.Insert(Entry{TaskUID: src.TaskUID, Kind: KindWork, Title: src.Title, Project: src.Project, ProjectID: src.ProjectID, Description: src.Description, StartedAt: t.clock().Unix()})
	if err != nil {
		return Entry{}, fmt.Errorf("resume entry #%d: %w", id, err)
	}
	return entry, nil
}

// BreakSeconds devuelve el tiempo total de break desde since; un break en curso cuenta hasta ahora.
func (t *Tracker) BreakSeconds(since time.Time) (int64, error) {
	n, err := t.repository.BreakSeconds(since.Unix(), t.clock().Unix())
	if err != nil {
		return 0, fmt.Errorf("calculate break time: %w", err)
	}
	return n, nil
}

// BreaksSince devuelve los breaks iniciados desde since, en orden de inicio.
func (t *Tracker) BreaksSince(since time.Time) ([]Entry, error) {
	entries, err := t.repository.BreaksSince(since.Unix())
	if err != nil {
		return nil, fmt.Errorf("list breaks: %w", err)
	}
	return entries, nil
}

// Get devuelve la entrada id o ErrNotFound si no existe o está en la papelera.
func (t *Tracker) Get(id int64) (Entry, error) {
	entry, err := t.repository.Get(id)
	if errors.Is(err, ErrNotFound) {
		return Entry{}, err
	}
	if err != nil {
		return Entry{}, fmt.Errorf("get entry #%d: %w", id, err)
	}
	return entry, nil
}

// IsRunning indica si la entrada id existe y sigue en curso (sea trabajo o break).
func (t *Tracker) IsRunning(id int64) (bool, error) {
	entry, err := t.repository.Get(id)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("get entry #%d: %w", id, err)
	}
	return entry.EndedAt == nil, nil
}

// StopMany detiene los timers indicados; ignora los que ya estaban detenidos.
func (t *Tracker) StopMany(ids []int64) error {
	for _, id := range ids {
		if err := t.Stop(id); err != nil && !errors.Is(err, ErrNotRunning) {
			return err
		}
	}
	return nil
}

// EditInput holds the optional changes for Edit; nil fields are left untouched.
type EditInput struct {
	Title       *string
	Project     *string
	ProjectID   *int64
	Description *string
}

// Edit changes a running or stopped entry. A project change updates both the name and its catalog link.
func (t *Tracker) Edit(id int64, in EditInput) (Entry, error) {
	entry, err := t.repository.Get(id)
	if err != nil {
		return Entry{}, fmt.Errorf("edit entry #%d: %w", id, err)
	}
	if in.Title != nil {
		title := strings.TrimSpace(*in.Title)
		if title == "" {
			return Entry{}, ErrEmptyTitle
		}
		entry.Title = title
	}
	if in.Description != nil {
		entry.Description = *in.Description
	}
	newProject, newProjectID, err := t.resolveProjectAssignment(entry.Project, entry.ProjectID, in.Project, in.ProjectID)
	if err != nil {
		if errors.Is(err, ErrAmbiguousProject) {
			return Entry{}, ErrAmbiguousProject
		}
		return Entry{}, fmt.Errorf("edit entry #%d: %w", id, err)
	}
	entry.Project = newProject
	entry.ProjectID = newProjectID
	if err := t.repository.Update(entry); err != nil {
		return Entry{}, fmt.Errorf("edit entry #%d: %w", id, err)
	}
	return entry, nil
}

// Delete soft-deletes an entry; a running one is stopped first so no timer lingers.
func (t *Tracker) Delete(id int64) error {
	if err := t.repository.SoftDelete(id, t.clock().Unix()); err != nil {
		return fmt.Errorf("delete entry #%d: %w", id, err)
	}
	return nil
}

// resumeWindow es el plazo en que restaurar una tarea borrada en curso la reanuda.
const resumeWindow = time.Minute

// Restore brings a soft-deleted entry back.
func (t *Tracker) Restore(id int64) error {
	// Un deshacer inmediato devuelve la tarea tal como estaba, incluso si seguía corriendo.
	resumeSince := t.clock().Add(-resumeWindow).Unix()
	if err := t.repository.Restore(id, resumeSince); err != nil {
		return fmt.Errorf("restore entry #%d: %w", id, err)
	}
	return nil
}

// Deleted lists the trash, newest deletion first.
func (t *Tracker) Deleted(limit int) ([]Entry, error) {
	entries, err := t.repository.Deleted(limit)
	if err != nil {
		return nil, fmt.Errorf("list deleted entries: %w", err)
	}
	return entries, nil
}

// Purge hard-deletes entries deleted longer than olderThan ago and returns how many.
func (t *Tracker) Purge(olderThan time.Duration) (int, error) {
	n, err := t.repository.Purge(t.clock().Add(-olderThan).Unix())
	if err != nil {
		return 0, fmt.Errorf("purge deleted entries: %w", err)
	}
	return n, nil
}

// Relink points the entries of project fromID at toID (0 clears it) and sets their project name.
func (t *Tracker) Relink(fromProjectID, toProjectID int64, toName string) error {
	if err := t.repository.Relink(fromProjectID, toProjectID, toName); err != nil {
		return fmt.Errorf("relink entries: %w", err)
	}
	return nil
}

// CountByProject devuelve cuántas tareas vivas tiene un proyecto.
func (t *Tracker) CountByProject(projectID int64) (int, error) {
	n, err := t.repository.CountByProject(projectID)
	if err != nil {
		return 0, fmt.Errorf("count project entries: %w", err)
	}
	return n, nil
}

// RenameProject updates the project name stored on the entries of projectID.
func (t *Tracker) RenameProject(projectID int64, name string) error {
	if err := t.repository.RenameProject(projectID, name); err != nil {
		return fmt.Errorf("rename project on entries: %w", err)
	}
	return nil
}

// Stop ends the specified running timer.
func (t *Tracker) Stop(id int64) error {
	if err := t.repository.Stop(id, t.clock().Unix()); err != nil {
		return fmt.Errorf("stop timer #%d: %w", id, err)
	}
	return nil
}

// StopRunningAt detiene en at las sesiones de trabajo en curso que empezaron antes de at
// (por ejemplo, la PC se apagó). Devuelve las sesiones detenidas, ya con EndedAt.
func (t *Tracker) StopRunningAt(at time.Time) ([]Entry, error) {
	running, err := t.repository.Running()
	if err != nil {
		return nil, fmt.Errorf("list running timers: %w", err)
	}
	stopped := make([]Entry, 0)
	endedAt := at.Unix()
	for _, entry := range running {
		if entry.Kind != KindWork || entry.StartedAt >= endedAt {
			continue
		}
		if err := t.repository.Stop(entry.ID, endedAt); err != nil {
			if errors.Is(err, ErrNotRunning) {
				continue
			}
			return stopped, fmt.Errorf("stop timer #%d: %w", entry.ID, err)
		}
		stored, err := t.repository.Get(entry.ID)
		if err != nil {
			return stopped, fmt.Errorf("get entry #%d: %w", entry.ID, err)
		}
		stopped = append(stopped, stored)
	}
	return stopped, nil
}

// StopLatest ends the most recently started running timer.
func (t *Tracker) StopLatest() error {
	running, err := t.repository.Running()
	if err != nil {
		return fmt.Errorf("list running timers: %w", err)
	}
	if len(running) == 0 {
		return ErrNotRunning
	}
	latest := running[0]
	for _, entry := range running[1:] {
		if entry.StartedAt > latest.StartedAt || (entry.StartedAt == latest.StartedAt && entry.ID > latest.ID) {
			latest = entry
		}
	}
	return t.Stop(latest.ID)
}

// StopAll ends every currently running timer and returns the number stopped.
func (t *Tracker) StopAll() (int, error) {
	n, err := t.repository.StopAll(t.clock().Unix())
	if err != nil {
		return 0, fmt.Errorf("stop all timers: %w", err)
	}
	return n, nil
}

// Snapshot returns running entries, today's total seconds, and recent entries.
func (t *Tracker) Snapshot() (running []Entry, todaySeconds int64, recent []Entry, err error) {
	now := t.clock()
	running, err = t.repository.Running()
	if err != nil {
		return nil, 0, nil, fmt.Errorf("list running timers: %w", err)
	}
	recent, err = t.repository.Recent(100)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("list recent timers: %w", err)
	}
	totals, err := t.repository.Totals(dayStart(now).Unix(), now.Unix())
	if err != nil {
		return nil, 0, nil, fmt.Errorf("calculate today's totals: %w", err)
	}
	for _, total := range totals {
		todaySeconds += total.Seconds
	}
	return running, todaySeconds, recent, nil
}

// Projects returns non-empty project usage sorted by most recent use, filtered by substring.
func (t *Tracker) Projects(query string) []ProjectUsage {
	projects, err := t.repository.Projects(t.clock().Unix())
	if err != nil {
		return []ProjectUsage{}
	}
	query = strings.ToLower(strings.TrimSpace(query))
	filtered := make([]ProjectUsage, 0, len(projects))
	for _, project := range projects {
		if strings.TrimSpace(project.Name) == "" || !strings.Contains(strings.ToLower(project.Name), query) {
			continue
		}
		filtered = append(filtered, project)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].LastUsed == filtered[j].LastUsed {
			return strings.ToLower(filtered[i].Name) < strings.ToLower(filtered[j].Name)
		}
		return filtered[i].LastUsed > filtered[j].LastUsed
	})
	return filtered
}

// Report returns project totals for intervals overlapping since.
func (t *Tracker) Report(since time.Time) ([]ProjectTotal, error) {
	totals, err := t.repository.Totals(since.Unix(), t.clock().Unix())
	if err != nil {
		return nil, fmt.Errorf("report tracked time: %w", err)
	}
	return totals, nil
}

func dayStart(now time.Time) time.Time {
	year, month, day := now.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, now.Location())
}

// RecentTasks devuelve las tareas de trabajo con su tiempo total, la de actividad más reciente primero.
func (t *Tracker) RecentTasks(limit int) ([]TaskSummary, error) {
	tasks, err := t.repository.RecentTasks(t.clock().Unix(), limit)
	if err != nil {
		return nil, fmt.Errorf("list recent tasks: %w", err)
	}
	return tasks, nil
}

// TaskTotal devuelve el tiempo total de la tarea; una sesión en curso cuenta hasta ahora.
func (t *Tracker) TaskTotal(taskUID string) (int64, error) {
	total, err := t.repository.TaskTotal(taskUID, t.clock().Unix())
	if err != nil {
		return 0, fmt.Errorf("task total: %w", err)
	}
	return total, nil
}

// EditTask cambia título, proyecto o descripción en todas las sesiones de la tarea y devuelve
// la sesión más reciente ya actualizada.
func (t *Tracker) EditTask(taskUID string, in EditInput) (Entry, error) {
	tasks, err := t.repository.RecentTasks(t.clock().Unix(), 1<<30)
	if err != nil {
		return Entry{}, fmt.Errorf("edit task: %w", err)
	}
	var latest Entry
	found := false
	for _, task := range tasks {
		if task.TaskUID == taskUID {
			latest, err = t.repository.Get(task.LastEntryID)
			if err != nil {
				return Entry{}, fmt.Errorf("edit task: %w", err)
			}
			found = true
			break
		}
	}
	if !found {
		return Entry{}, fmt.Errorf("edit task: %w", ErrNotFound)
	}
	if in.Title != nil {
		title := strings.TrimSpace(*in.Title)
		if title == "" {
			return Entry{}, ErrEmptyTitle
		}
		latest.Title = title
	}
	if in.Description != nil {
		latest.Description = *in.Description
	}
	newProject, newProjectID, err := t.resolveProjectAssignment(latest.Project, latest.ProjectID, in.Project, in.ProjectID)
	if err != nil {
		if errors.Is(err, ErrAmbiguousProject) {
			return Entry{}, ErrAmbiguousProject
		}
		return Entry{}, fmt.Errorf("edit task: %w", err)
	}
	latest.Project = newProject
	latest.ProjectID = newProjectID
	if err := t.repository.UpdateTask(taskUID, latest); err != nil {
		return Entry{}, fmt.Errorf("edit task: %w", err)
	}
	return latest, nil
}

// DeleteTask elimina todas las sesiones de la tarea; la que esté en curso se detiene primero.
func (t *Tracker) DeleteTask(taskUID string) error {
	if err := t.repository.SoftDeleteTask(taskUID, t.clock().Unix()); err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	return nil
}

// RestoreTask restaura las sesiones eliminadas juntas; un deshacer inmediato reanuda la que estaba en curso.
func (t *Tracker) RestoreTask(taskUID string) error {
	resumeSince := t.clock().Add(-resumeWindow).Unix()
	if err := t.repository.RestoreTask(taskUID, resumeSince); err != nil {
		return fmt.Errorf("restore task: %w", err)
	}
	return nil
}
