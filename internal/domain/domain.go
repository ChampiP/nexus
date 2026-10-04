// Package domain contains Nexus's storage- and UI-independent domain types.
package domain

import "errors"

// Entry is a tracked interval. A nil EndedAt denotes a running timer.
type Entry struct {
	ID          int64
	Title       string
	Description string
	Project     string
	StartedAt   int64
	EndedAt     *int64
}

// ProjectTotal is the tracked duration for one project, in whole seconds.
type ProjectTotal struct {
	Project string
	Seconds int64
}

// ProjectUsage summarizes the most recent use and total duration of a project.
type ProjectUsage struct {
	Name     string
	LastUsed int64
	Seconds  int64
}

var (
	// ErrNotRunning indicates that a timer does not exist or is already stopped.
	ErrNotRunning = errors.New("timer is not running")
	// ErrEmptyTitle indicates that a timer title is blank after trimming.
	ErrEmptyTitle = errors.New("title cannot be empty")
)
