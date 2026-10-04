package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"nexus/internal/tracking"
)

// Tracker is the narrow application boundary required by the terminal interface.
type Tracker interface {
	Start(tracking.StartInput) (tracking.Entry, error)
	Stop(int64) error
	Snapshot() ([]tracking.Entry, int64, []tracking.Entry, error)
	Projects(string) []tracking.ProjectUsage
	Report(time.Time) ([]tracking.ProjectTotal, error)
}

type tickMsg time.Time

// Run starts the interactive terminal interface.
func Run(tracker Tracker) error {
	_, err := tea.NewProgram(NewModel(tracker), tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(now time.Time) tea.Msg { return tickMsg(now) })
}
