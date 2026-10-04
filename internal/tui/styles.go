package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	accent     = lipgloss.AdaptiveColor{Light: "#5B4BFF", Dark: "#A99CFF"}
	muted      = lipgloss.AdaptiveColor{Light: "#667085", Dark: "#A0A5B2"}
	panel      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.AdaptiveColor{Light: "#D8D7E8", Dark: "#414356"}).Padding(0, 1)
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(accent)
	label      = lipgloss.NewStyle().Foreground(muted)
)

func rangeStart(now time.Time, week bool) time.Time {
	y, month, day := now.Date()
	start := time.Date(y, month, day, 0, 0, 0, 0, now.Location())
	if week {
		start = start.AddDate(0, 0, -6)
	}
	return start
}
func formatDuration(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	return fmt.Sprintf("%d:%02d:%02d", seconds/3600, seconds/60%60, seconds%60)
}
func clock(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	return fmt.Sprintf("%02d:%02d:%02d", seconds/3600, seconds/60%60, seconds%60)
}
func projectColor(index int) lipgloss.AdaptiveColor {
	colors := []lipgloss.AdaptiveColor{{Light: "#6750A4", Dark: "#C4B5FD"}, {Light: "#00796B", Dark: "#5EEAD4"}, {Light: "#B45309", Dark: "#FCD34D"}, {Light: "#0369A1", Dark: "#7DD3FC"}, {Light: "#BE185D", Dark: "#F9A8D4"}}
	return colors[index%len(colors)]
}
func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit || limit <= 0 {
		return value
	}
	return string(runes[:limit-1]) + "…"
}
