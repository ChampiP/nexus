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
}

// NewTracker creates a Tracker using the supplied repository and clock.
func NewTracker(repository Repository, clock Clock) *Tracker {
	if clock == nil {
		clock = time.Now
	}
	return &Tracker{repository: repository, clock: clock}
}

// Start creates a timer after trimming and validating its title.
func (t *Tracker) Start(input StartInput) (Entry, error) {
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		return Entry{}, ErrEmptyTitle
	}
	entry, err := t.repository.Insert(Entry{Title: input.Title, Project: input.Project, Description: input.Description, StartedAt: t.clock().Unix()})
	if err != nil {
		return Entry{}, fmt.Errorf("start timer: %w", err)
	}
	return entry, nil
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
