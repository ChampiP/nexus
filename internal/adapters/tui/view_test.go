package tui

import (
	"strings"
	"testing"
	"time"

	"nexus/internal/tracking"
)

func fixtureModel(width int, store *testStore) Model {
	m := NewModel(store)
	m.width = width
	m.now = time.Date(2026, time.October, 4, 12, 0, 0, 0, time.UTC)
	return m
}

func TestNarrowViewUnchanged(t *testing.T) {
	m := fixtureModel(100, &testStore{})
	const want = "NEXUS  ·  Hoy 0:00:00  ·  04/10/2026\n\nTítulo\n> Escribe un título                       \nProyecto\n‹ Sin proyecto ›\n\nDescripción\n> Descripción opcional                    \n    Iniciar  \n\nEN CURSO\nAún no hay temporizadores activos\n  Hoy      Semana\n\n╭──────────────────────────────────────────────────────────────────────────────────────────────────╮\n│ TIEMPO POR PROYECTO · Hoy                                                                        │\n│ Sin tiempo registrado en este período                                                            │\n│                                                                                                  │\n│ RECIENTES                                                                                        │\n│ Todavía no hay registros                                                                         │\n╰──────────────────────────────────────────────────────────────────────────────────────────────────╯\n\n↑↓ mover · Enter iniciar · clic para seleccionar · Ctrl+C salir"
	if got := m.View(); got != want {
		t.Fatalf("narrow view changed:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestWideViewShowsAllPanels(t *testing.T) {
	m := fixtureModel(160, &testStore{})
	view := m.View()
	t.Logf("160x40 fixture:\n%s", view)
	for _, heading := range []string{"NUEVA TAREA", "EN CURSO", "RECIENTES", "☕ BREAK", "PAUSAS ACTIVAS"} {
		if !strings.Contains(view, heading) {
			t.Errorf("wide view missing %q:\n%s", heading, view)
		}
	}
}

func TestWideViewShowsLongProjectNameWhenItFits(t *testing.T) {
	name := "Organización Cliente Proyecto de nombre largo"
	store := &testStore{totals: []tracking.ProjectTotal{{Project: name, Seconds: 90}}}
	m := fixtureModel(200, store)
	if got := m.View(); !strings.Contains(got, name) {
		t.Fatalf("project name was truncated:\n%s", got)
	}
}
