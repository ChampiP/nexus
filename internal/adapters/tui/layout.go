package tui

type rect struct{ x, y, w, h int }

func (r rect) contains(x, y int) bool { return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h }

type runningZones struct{ row, stop rect }
type screenLayout struct {
	fields                                            [3]rect
	start                                             rect
	running                                           []runningZones
	options                                           []rect
	optionsStart                                      int
	today, week                                       rect
	descriptionY, startY, runningY, tabsY, dashboardY int
}

// computeLayout is the single source of screen coordinates for rendering and mouse hit testing.
func (m Model) computeLayout() screenLayout {
	width := m.width
	if width < 1 {
		width = 80
	}
	l := screenLayout{}
	l.fields[0] = rect{0, 3, width, 1}
	l.fields[1] = rect{0, 5, width, 1}
	y := 6
	if m.pickerOpen {
		options := m.projectOptions()
		count := len(options)
		if count > 5 {
			count = 5
		}
		pickerScroll := m.pickerScroll
		if pickerScroll > len(options)-count {
			pickerScroll = max(0, len(options)-count)
		}
		if m.pickerIndex < pickerScroll {
			pickerScroll = m.pickerIndex
		}
		if m.pickerIndex >= pickerScroll+count {
			pickerScroll = m.pickerIndex - count + 1
		}
		l.optionsStart = pickerScroll
		for i := 0; i < count; i++ {
			l.options = append(l.options, rect{0, y + i, width, 1})
		}
		y += count
	}
	l.descriptionY = y + 1
	l.fields[2] = rect{0, l.descriptionY + 1, width, 1}
	l.startY = l.descriptionY + 2
	l.start = rect{0, l.startY, width, 1}
	l.runningY = l.startY + 2
	visible := len(m.running) - m.runScroll
	if m.height > 0 {
		visible = min(visible, max(1, m.height-l.runningY-8))
	}
	if visible < 0 {
		visible = 0
	}
	for i := 0; i < visible; i++ {
		rowY := l.runningY + 1 + i
		l.running = append(l.running, runningZones{row: rect{0, rowY, width, 1}, stop: rect{max(0, width-11), rowY, 11, 1}})
	}
	l.tabsY = l.runningY + 1 + visible + 1
	l.today = rect{0, l.tabsY, 12, 1}
	l.week = rect{13, l.tabsY, 12, 1}
	l.dashboardY = l.tabsY + 2
	return l
}
