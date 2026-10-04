package tracking

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// StartInput contains the fields needed to start a timer.
type StartInput struct {
	Title       string
	Project     string
	Description string
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
		return 0, fmt.Errorf("resolve project %q: %w", name, err)
	}
	return id, nil
}

// Start creates a timer after trimming and validating its title.
func (t *Tracker) Start(input StartInput) (Entry, error) {
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		return Entry{}, ErrEmptyTitle
	}
	projectID, err := t.resolve(input.Project)
	if err != nil {
		return Entry{}, fmt.Errorf("start timer: %w", err)
	}
	entry, err := t.repository.Insert(Entry{Title: input.Title, Project: input.Project, ProjectID: projectID, Description: input.Description, StartedAt: t.clock().Unix()})
	if err != nil {
		return Entry{}, fmt.Errorf("start timer: %w", err)
	}
	return entry, nil
}

// EditInput holds the optional changes for Edit; nil fields are left untouched.
type EditInput struct {
	Title       *string
	Project     *string
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
	if in.Project != nil {
		entry.Project = *in.Project
		if entry.ProjectID, err = t.resolve(entry.Project); err != nil {
			return Entry{}, fmt.Errorf("edit entry #%d: %w", id, err)
		}
	}
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

// Restore brings a soft-deleted entry back.
func (t *Tracker) Restore(id int64) error {
	if err := t.repository.Restore(id); err != nil {
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
