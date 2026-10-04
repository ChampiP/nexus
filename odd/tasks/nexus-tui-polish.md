# nexus-tui-polish

User feedback (2026-10-04): cannot resume a stopped task; arrow navigation is inconsistent (←/→ does not
move between buttons everywhere, no quick way down); the TUI feels static.

## Tasks
- [x] 1. Resume a stopped task ([▶ Reanudar] on stopped rows, `nexus resume <id>`), uniform ←/→ ↑/↓ navigation
  plus PgUp/PgDn between sections, light animation (spinner on running timers, draining break progress bar,
  clearer focus highlight)
- [x] 2. A task has many sessions: resuming adds a session to the SAME task (task_uid on entries, migration grouping identical title+project+description), Recientes shows one row per task with its total, edit/delete act on the whole task
