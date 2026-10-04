package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"nexus/internal/app"
	"nexus/internal/domain"
)

// Tracker is the narrow application boundary required by the terminal interface.
type Tracker interface {
	Start(app.StartInput) (domain.Entry, error)
	Stop(int64) error
	Snapshot() ([]domain.Entry, int64, []domain.Entry, error)
	Projects(string) []domain.ProjectUsage
	Report(time.Time) ([]domain.ProjectTotal, error)
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
