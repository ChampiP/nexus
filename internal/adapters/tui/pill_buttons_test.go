package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"nexus/internal/tracking"
	"nexus/internal/wellbeing"
)

func TestButtonPillStylesAndWidth(t *testing.T) {
	orig := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(orig)
	labels := []string{"Guardar", "■ Detener", "✕ Eliminar", "▶ Iniciar", "Reiniciar"}
	for _, label := range labels {
		t.Run(label, func(t *testing.T) {
			oldBracketed := buttonText(label)
			wantWidth := lipgloss.Width(oldBracketed)

			unfocused := RenderButton(label, false, false)
			focused := RenderButton(label, true, false)
			destructive := RenderButton(label, false, true)
			destructiveFocused := RenderButton(label, true, true)

			for name, rendered := range map[string]string{
				"unfocused":          unfocused,
				"focused":            focused,
				"destructive":        destructive,
				"destructiveFocused": destructiveFocused,
			} {
				if w := lipgloss.Width(rendered); w != wantWidth {
					t.Errorf("ancho de botón %s = %d, esperado = %d", name, w, wantWidth)
				}
				stripped := ansi.Strip(rendered)
				if strings.Contains(stripped, "[") || strings.Contains(stripped, "]") {
					t.Errorf("el botón %s no debe contener corchetes: %q", name, stripped)
				}
			}

			if unfocused == focused {
				t.Errorf("botón enfocado y no enfocado no deben tener el mismo estilo")
			}
			if unfocused == destructive {
				t.Errorf("botón destructivo y no destructivo no deben tener el mismo estilo")
			}
			if focused == destructive {
				t.Errorf("botón enfocado y destructivo no deben tener el mismo estilo")
			}
			if destructive == destructiveFocused {
				t.Errorf("botón destructivo enfocado y no enfocado no deben tener el mismo estilo")
			}
		})
	}

	// Verifica autodetección de destructivo en botones con Eliminar o Detener.
	if RenderButton("✕ Eliminar", false) != RenderButton("✕ Eliminar", false, true) {
		t.Errorf("✕ Eliminar debió autodetectarse como destructivo")
	}
	if RenderButton("■ Detener", false) != RenderButton("■ Detener", false, true) {
		t.Errorf("■ Detener debió autodetectarse como destructivo")
	}
}

func TestOptionPillStylesAndWidth(t *testing.T) {
	orig := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(orig)
	options := []string{"15 min", "30 min", "Sí", "No", "10 s"}
	for _, opt := range options {
		t.Run(opt, func(t *testing.T) {
			oldBracketed := buttonText(opt)
			wantWidth := lipgloss.Width(oldBracketed)

			plain := RenderOption(opt, false, false)
			selected := RenderOption(opt, true, false)
			selectedFocused := RenderOption(opt, true, true)
			plainFocused := RenderOption(opt, false, true)

			for name, rendered := range map[string]string{
				"plain":           plain,
				"selected":        selected,
				"selectedFocused": selectedFocused,
				"plainFocused":    plainFocused,
			} {
				if w := lipgloss.Width(rendered); w != wantWidth {
					t.Errorf("ancho de opción %s = %d, esperado = %d", name, w, wantWidth)
				}
				stripped := ansi.Strip(rendered)
				if strings.Contains(stripped, "[") || strings.Contains(stripped, "]") {
					t.Errorf("la opción %s no debe contener corchetes: %q", name, stripped)
				}
			}

			if plain == selected {
				t.Errorf("opción seleccionada y no seleccionada deben tener estilos distintos")
			}
			if selected == selectedFocused {
				t.Errorf("opción seleccionada con y sin foco deben tener estilos distintos")
			}
			if plain == plainFocused {
				t.Errorf("opción enfocada debe tener estilo distinto al plano")
			}
		})
	}
}

func TestButtonHitboxesAtWidth100And180(t *testing.T) {
	for _, width := range []int{100, 180} {
		t.Run("width", func(t *testing.T) {
			store := &testStore{
				running: []tracking.Entry{
					{ID: 10, TaskUID: "task-10", Title: "Activa", Project: "Nexus", StartedAt: time.Now().Add(-15 * time.Minute).Unix()},
				},
				recent: []tracking.Entry{
					{ID: 20, TaskUID: "task-20", Title: "Reciente", Project: "Nexus", StartedAt: time.Now().Add(-2 * time.Hour).Unix(), EndedAt: timePtr(1)},
				},
				totals: []tracking.ProjectTotal{
					{Project: "Nexus", Seconds: 900},
				},
			}
			breaks := &fakeBreaks{}
			wellbeingMock := &fakeWellbeing{
				status: wellbeing.Status{
					Settings: wellbeing.Settings{Enabled: true, Every: 30 * time.Minute, Duration: 20 * time.Second},
				},
			}
			m := NewModel(store, WithBreaks(breaks), WithWellbeing(wellbeingMock))
			m.width, m.height = width, 45
			m.now = time.Now()

			checkButtonInZone := func(t *testing.T, lines []string, zone rect, expectedLabel string) {
				t.Helper()
				if zone.y < 0 || zone.y >= len(lines) {
					t.Fatalf("zona y=%d fuera del rango de líneas (%d)", zone.y, len(lines))
				}
				line := lines[zone.y]
				runes := []rune(line)
				if zone.x < 0 || zone.x+zone.w > len(runes) {
					t.Fatalf("zona x=[%d,%d) fuera de rango de línea (%d): %q", zone.x, zone.x+zone.w, len(runes), line)
				}
				segment := string(runes[zone.x : zone.x+zone.w])
				if !strings.Contains(segment, expectedLabel) {
					t.Errorf("la zona [%d, %d) en línea %d contiene %q, no incluye la etiqueta %q", zone.x, zone.x+zone.w, zone.y, segment, expectedLabel)
				}
			}

			if width == 100 {
				m.screen = screenTimers
				view := ansi.Strip(m.View())
				lines := strings.Split(view, "\n")
				layout := m.computeLayout()

				checkButtonInZone(t, lines, layout.start, "Iniciar")
				for _, r := range layout.running {
					for i, b := range r.buttons {
						checkButtonInZone(t, lines, b, entryLabels(runningActions)[i])
					}
				}
				for _, r := range layout.recent {
					for i, b := range r.buttons {
						checkButtonInZone(t, lines, b, entryLabels(recentActions())[i])
					}
				}

				// Pantalla Break a ancho 100
				m.screen = screenBreak
				breakView := ansi.Strip(m.View())
				breakLines := strings.Split(breakView, "\n")
				bl := m.breakLayout()
				for i, b := range bl.durations {
					checkButtonInZone(t, breakLines, b, breakDurationLabels[i])
				}
				checkButtonInZone(t, breakLines, bl.start, breakStartLabels[0])
				for i, b := range bl.pauseButtons {
					checkButtonInZone(t, breakLines, b, pauseButtonLabels(wellbeingMock.status, m.now)[i])
				}
			} else {
				m.screen = screenTimers
				view := ansi.Strip(m.View())
				lines := strings.Split(view, "\n")
				geom, ok := m.wideGeometry()
				if !ok {
					t.Fatal("no se pudo calcular la geometría ancha")
				}

				checkButtonInZone(t, lines, rect{geom.leftContentX, geom.startY, startWidth, 1}, "Iniciar")
				for _, r := range geom.runningRows {
					for i, b := range r.buttons {
						checkButtonInZone(t, lines, b, entryLabels(runningActions)[i])
					}
				}
				for _, r := range geom.recent {
					for i, b := range r.buttons {
						checkButtonInZone(t, lines, b, entryLabels(recentActions())[i])
					}
				}
				bl := m.breakLayout()
				for i, b := range bl.durations {
					checkButtonInZone(t, lines, rect{geom.rightX + b.x, b.y, b.w, b.h}, breakDurationLabels[i])
				}
				checkButtonInZone(t, lines, rect{geom.rightX + bl.start.x, bl.start.y, bl.start.w, bl.start.h}, breakStartLabels[0])
				for i, b := range bl.pauseButtons {
					checkButtonInZone(t, lines, rect{geom.rightX + b.x, b.y, b.w, b.h}, pauseButtonLabels(wellbeingMock.status, m.now)[i])
				}
			}

			// Pantalla Catálogo
			m.screen = screenCatalog
			catView := ansi.Strip(m.View())
			catLines := strings.Split(catView, "\n")
			cl := m.catalogLayout()
			for i, b := range cl.adds {
				checkButtonInZone(t, catLines, b, addLabels[i])
			}
			checkButtonInZone(t, catLines, rect{2, cl.toggleY, lipgloss.Width(buttonText(toggleLabel(false))), 1}, toggleLabel(false))
		})
	}
}
