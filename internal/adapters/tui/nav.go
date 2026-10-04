package tui

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// navCell es un elemento enfocable: su tipo (propio de cada pantalla), el índice de la fila de datos
// a la que pertenece y su columna dentro de la línea.
type navCell struct{ kind, index, col int }

// navRow es una línea de elementos enfocables. section agrupa filas para PgUp/PgDn; pick es la columna
// elegida de una fila de opciones excluyentes (pestañas, Hoy/Semana, duraciones) o -1 si no lo es.
type navRow struct {
	section int
	cells   []navCell
	pick    int
}

// navGrid es el modelo único de navegación: cada pantalla describe sus elementos como filas y el
// movimiento (↑ ↓ ← → Tab PgUp PgDn Home End) se resuelve aquí, igual en todas.
type navGrid []navRow

// locate devuelve la fila y la columna de la celda; si no existe, la primera.
func (g navGrid) locate(c navCell) (row, col int) {
	for r, line := range g {
		for i, cell := range line.cells {
			if cell == c {
				return r, i
			}
		}
	}
	return 0, 0
}

// landing es la celda de la fila r a la que se llega desde la columna col: la elegida en una fila
// de opciones y, si no, la columna más cercana.
func (g navGrid) landing(r, col int) navCell {
	line := g[r]
	if line.pick >= 0 {
		return line.cells[line.pick]
	}
	return line.cells[min(col, len(line.cells)-1)]
}

// vertical pasa a la fila anterior o siguiente conservando la columna más cercana.
func (g navGrid) vertical(c navCell, step int) navCell {
	if len(g) == 0 {
		return c
	}
	r, col := g.locate(c)
	r = min(max(r+step, 0), len(g)-1)
	return g.landing(r, col)
}

// horizontal pasa al vecino de la misma fila; en los bordes no se mueve ni envuelve.
func (g navGrid) horizontal(c navCell, step int) navCell {
	if len(g) == 0 {
		return c
	}
	r, col := g.locate(c)
	return g[r].cells[min(max(col+step, 0), len(g[r].cells)-1)]
}

// section salta al primer elemento de la sección siguiente o anterior; en los extremos no se mueve.
func (g navGrid) section(c navCell, step int) navCell {
	if len(g) == 0 {
		return c
	}
	r, _ := g.locate(c)
	current := g[r].section
	for i := r + step; i >= 0 && i < len(g); i += step {
		if g[i].section == current {
			continue
		}
		target := g[i].section
		for i > 0 && g[i-1].section == target && step < 0 {
			i--
		}
		return g.landing(i, 0)
	}
	return c
}

// ends devuelve el primer o el último elemento enfocable.
func (g navGrid) ends(first bool) navCell {
	if first {
		return g[0].cells[0]
	}
	last := g[len(g)-1]
	return last.cells[len(last.cells)-1]
}

// linear recorre todos los elementos en orden (Tab y Shift+Tab); de las filas de opciones solo visita la elegida.
func (g navGrid) linear(c navCell, step int) navCell {
	var stops []navCell
	for _, line := range g {
		if line.pick >= 0 {
			stops = append(stops, line.cells[line.pick])
			continue
		}
		stops = append(stops, line.cells...)
	}
	at := 0
	for i, stop := range stops {
		if stop == c {
			at = i
			break
		}
	}
	return stops[min(max(at+step, 0), len(stops)-1)]
}

// move resuelve una tecla de navegación desde c; ok es false si la tecla no es de navegación.
func (g navGrid) move(c navCell, key tea.KeyType) (target navCell, ok bool) {
	if len(g) == 0 {
		return c, false
	}
	switch key {
	case tea.KeyUp:
		return g.vertical(c, -1), true
	case tea.KeyDown:
		return g.vertical(c, 1), true
	case tea.KeyLeft:
		return g.horizontal(c, -1), true
	case tea.KeyRight:
		return g.horizontal(c, 1), true
	case tea.KeyPgUp:
		return g.section(c, -1), true
	case tea.KeyPgDown:
		return g.section(c, 1), true
	case tea.KeyHome:
		return g.ends(true), true
	case tea.KeyEnd:
		return g.ends(false), true
	case tea.KeyTab:
		return g.linear(c, 1), true
	case tea.KeyShiftTab:
		return g.linear(c, -1), true
	}
	return c, false
}

// inputAtEdge indica si ← o → deben salir del campo: está vacío o el cursor ya está en ese borde.
func inputAtEdge(in textinput.Model, key tea.KeyType) bool {
	if in.Value() == "" {
		return true
	}
	if key == tea.KeyLeft {
		return in.Position() == 0
	}
	return in.Position() >= len([]rune(in.Value()))
}
