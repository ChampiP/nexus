# nexus-quick-access

Goal: fast capture. Arrow/Enter/mouse driven TUI (no letter shortcuts to memorize), a bar popup for quick start/stop, and a layered architecture that scales.

Non-goals: logging, pause/resume, sync.

## Architecture (target)
- internal/domain: pure types + sentinel errors (Entry, ProjectUsage, ProjectTotal, ErrNotRunning, ErrEmptyTitle)
- internal/app: Tracker use cases (Start, Stop, StopAll, Snapshot, Projects, Report). Defines the Repository interface and the Clock. No SQL, no UI.
- internal/store: SQLite implementation of app.Repository (+ migrations)
- internal/cli: command parsing + presenters (Spanish text, --json)
- internal/tui: Bubble Tea adapter, depends on app.Tracker only
- plugin/champip.nexus: QML, talks to the `nexus` CLI JSON only (Process with argv arrays, never shell strings)
- cmd/nexus/main.go: composition root only

## Tasks
- [x] 1. Layered refactor + `projects` query + CLI in Spanish + --json contract (commit b92f0af)
- [x] 2. TUI: arrows/Enter/mouse, always-ready title input, searchable project picker, visible description (commit ba3f4df)
- [ ] 3. Bar popup panel: quick start (title + searchable project), running timers with stop buttons, open detail TUI
