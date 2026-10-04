# nexus-tracker

Goal: time tracker with multiple concurrent timers. Go binary `nexus` (CLI + Bubble Tea TUI), SQLite storage, Omarchy shell bar widget.

Non-goals (MVP): pause/resume (stop + start again), sync, invoicing, tags, multi-user.

Decisions
- Go + modernc.org/sqlite (no cgo) + bubbletea/lipgloss. One static binary, fast cold start (bar polls it).
- DB: ~/.local/share/nexus/nexus.db, WAL. Table `entries(id, title, description, project, started_at, ended_at NULL=running)`. Many rows with ended_at NULL = many timers.
- Bar widget (QML) polls `nexus status --json`; click opens a terminal running `nexus`.

## Tasks
- [ ] 1. Store + CLI: start/stop/ls/status --json/report (tests first)
- [ ] 2. TUI: live running timers, start form, stop, today/week dashboard by project
- [ ] 3. Bar widget plugin (manifest + BarWidget.qml) + `make install`
