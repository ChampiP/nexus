package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"nexus/internal/countdown"
)

// fastTickEvery es el ritmo del spinner; solo corre mientras algo se anima.
const fastTickEvery = 120 * time.Millisecond

// fastTickMsg avanza un fotograma de las animaciones.
type fastTickMsg struct{}

func fastTick() tea.Cmd {
	return tea.Tick(fastTickEvery, func(time.Time) tea.Msg { return fastTickMsg{} })
}

func newSpinner() spinner.Model {
	return spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(lipgloss.NewStyle().Foreground(accent)))
}

// animating indica si hay algo que animar: el spinner de los temporizadores de trabajo en curso.
func (m Model) animating() bool {
	return m.screen == screenTimers && len(workEntries(m.running)) > 0
}

// spinnerFrame es el fotograma actual sin color, para incrustarlo en una fila con fondo propio.
func (m Model) spinnerFrame() string {
	plain := m.spin
	plain.Style = lipgloss.NewStyle()
	return plain.View()
}

// advanceSpinner pasa al siguiente fotograma sin depender del reloj propio del spinner.
func (m *Model) advanceSpinner() {
	m.spin, _ = m.spin.Update(m.spin.Tick())
}

// Update procesa un mensaje y arranca el tick rápido solo si algo empieza a animarse.
func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(message)
	model, ok := next.(Model)
	if !ok {
		return next, cmd
	}
	if model.animating() && !model.fastActive {
		model.fastActive = true
		cmd = tea.Batch(cmd, fastTick())
	}
	return model, cmd
}

// breakFraction es la fracción restante del break (1 al empezar, 0 al terminar) y si ya venció.
// Vencido, la barra se muestra llena.
func breakFraction(b countdown.Break, now time.Time) (fraction float64, overdue bool) {
	if breakOverdue(b, now) {
		return 1, true
	}
	total := b.EndsAt - b.StartedAt
	if total <= 0 {
		return 1, false
	}
	return min(max(float64(b.EndsAt-now.Unix())/float64(total), 0), 1), false
}

// progressBarWidth es el largo de la barra de la tarjeta del break.
const progressBarWidth = 30

// breakBar dibuja la barra del break; vencido usa el color urgente. bubbles/progress arrastra una
// dependencia (harmonica) que no está en go.sum, así que la barra se dibuja con lipgloss.
func (m Model) breakBar() string {
	fraction, overdue := breakFraction(*m.brk, m.now)
	color := accent
	if overdue {
		color = urgent
	}
	width := min(progressBarWidth, max(10, m.width-8))
	filled := int(fraction*float64(width) + 0.5)
	if fraction > 0 {
		filled = max(filled, 1)
	}
	return lipgloss.NewStyle().Foreground(color).Render(strings.Repeat("█", filled)) +
		lipgloss.NewStyle().Foreground(muted).Render(strings.Repeat("░", width-filled))
}
