package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	accent     = lipgloss.AdaptiveColor{Light: "#5B4BFF", Dark: "#A99CFF"}
	muted      = lipgloss.AdaptiveColor{Light: "#667085", Dark: "#A0A5B2"}
	panel      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.AdaptiveColor{Light: "#D8D7E8", Dark: "#414356"}).Padding(0, 1)
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(accent)
	urgent     = lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#F87171"}
	label      = lipgloss.NewStyle().Foreground(muted)
	// Resaltado del foco: el botón enfocado lleva fondo de acento y la fila enfocada un fondo sutil.
	focusInk    = lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#1B1B2F"}
	rowBg       = lipgloss.AdaptiveColor{Light: "#EDEBFF", Dark: "#2A2B3D"}
	focusButton = lipgloss.NewStyle().Bold(true).Foreground(focusInk).Background(accent)
	focusRow    = lipgloss.NewStyle().Background(rowBg)

	// Botones normales como píldoras: fondo suave no enfocado y acento fuerte enfocado.
	buttonBg    = lipgloss.AdaptiveColor{Light: "#EAE8FF", Dark: "#2B2A4A"}
	buttonFg    = lipgloss.AdaptiveColor{Light: "#4838D6", Dark: "#C4B5FD"}
	buttonStyle = lipgloss.NewStyle().Foreground(buttonFg).Background(buttonBg)

	// Botones destructivos como píldoras con fondo rojizo.
	destructiveBg           = lipgloss.AdaptiveColor{Light: "#FEE2E2", Dark: "#441C1C"}
	destructiveFg           = lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#FCA5A5"}
	destructiveStyle        = lipgloss.NewStyle().Foreground(destructiveFg).Background(destructiveBg)
	destructiveFocusedBg    = lipgloss.AdaptiveColor{Light: "#DC2626", Dark: "#EF4444"}
	destructiveFocusedStyle = lipgloss.NewStyle().Bold(true).Foreground(focusInk).Background(destructiveFocusedBg)

	// Opciones seleccionables: la seleccionada es píldora de acento, las demás texto con relleno.
	optionStyle                = lipgloss.NewStyle().Foreground(muted)
	optionFocusedStyle         = lipgloss.NewStyle().Bold(true).Underline(true).Foreground(accent)
	optionSelectedStyle        = lipgloss.NewStyle().Foreground(focusInk).Background(accent)
	optionSelectedFocusedStyle = lipgloss.NewStyle().Bold(true).Underline(true).Foreground(focusInk).Background(accent)
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

// buttonPillText reemplaza los corchetes por un espacio a cada lado para mantener el ancho idéntico.
func buttonPillText(label string) string {
	return " " + label + " "
}

// isDestructiveButton determina si una etiqueta corresponde a una acción destructiva o de detención.
func isDestructiveButton(label string) bool {
	clean := strings.TrimSpace(label)
	return strings.Contains(clean, "Eliminar") || strings.Contains(clean, "Detener")
}

// RenderButton formatea un botón como píldora coloreada; el enfocado lleva acento y negrita.
func RenderButton(label string, focused bool, destructive ...bool) string {
	isDestr := isDestructiveButton(label)
	if len(destructive) > 0 {
		isDestr = destructive[0]
	}
	text := buttonPillText(label)
	switch {
	case isDestr && focused:
		return destructiveFocusedStyle.Render(text)
	case isDestr:
		return destructiveStyle.Render(text)
	case focused:
		return focusButton.Render(text)
	default:
		return buttonStyle.Render(text)
	}
}

// RenderOption formatea una opción de fila de selección como píldora si está seleccionada.
func RenderOption(label string, selected bool, focused bool) string {
	text := buttonPillText(label)
	switch {
	case selected && focused:
		return optionSelectedFocusedStyle.Render(text)
	case selected:
		return optionSelectedStyle.Render(text)
	case focused:
		return optionFocusedStyle.Render(text)
	default:
		return optionStyle.Render(text)
	}
}

// RenderOptions formatea una lista de opciones contiguas separadas por espacio.
func RenderOptions(labels []string, selected int, focused int) string {
	parts := make([]string, len(labels))
	for i, l := range labels {
		parts[i] = RenderOption(l, i == selected, i == focused)
	}
	return strings.Join(parts, buttonSeparator)
}

// RenderPeriodOption formatea las opciones Hoy y Semana respetando el ancho de cada estado.
func RenderPeriodOption(name string, selected bool, focused bool) string {
	if selected {
		return RenderOption(" "+name+" ", true, focused)
	}
	style := optionStyle
	if focused {
		style = optionFocusedStyle
	}
	return style.Render(name)
}
