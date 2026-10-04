# nexus-break

Goal: break countdown that reminds the user to come back (docs/ARCHITECTURE.md §9, phase 2).
Keep it lean: one active break at a time, no movement reminders yet.

## Behavior
- Starting a break asks which running work timers to stop (all preselected) (D8).
- The break is a tracking entry with kind=break (excluded from work totals) plus a countdown with a deadline.
- When the deadline passes: notification with actions Volver al trabajo / +10 min / Terminar break,
  repeated every 10 min while overdue. The bar and TUI show remaining or overdue time without the daemon.
- "Volver al trabajo" ends the break and starts NEW entries copying the stopped timers (break time never
  counts as work). Nexus never resumes work by itself.

## Tasks
- [x] 1. Domain: tracking kind=break (excluded from work totals/running list) + countdown module (migration, use cases, ports) + tests (commit 902289e)
- [ ] 2. CLI: break start/status/extend/end, additive `break` field in status --json
- [ ] 3. Daemon: `nexus daemon` (deadline loop, notify-send actions), systemd user unit, make install
- [ ] 4. TUI: break button, duration + timers-to-stop panel, break banner with actions
- [ ] 5. Bar: label and popup show break remaining/overdue with actions (needs shell restart)
