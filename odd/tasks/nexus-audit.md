# nexus-audit

Goal: fix the defects found by the 2026-10-05 read-only bug audit (Engram `nexus/audit-2026-10-05`).
Branch: fix/nexus-audit (from main 06253c6). One problem per worker, verify after each, commit per batch.

Non-goals: new features, refactors beyond the fix, changing the activity-based pause design.

## Batch 1 — bar and CLI
- [x] 1.1 Plugin: formatClock treats started_at (Unix seconds) as ms; projectArgv sends "#0" for id 0 (Model.js + Model.test.js)
- [x] 1.2 StopAll also closes the running break row (tracking/sqlite.go) — keep breaks to countdown
- [x] 1.3 `status --json` swallows Snapshot errors (bar shows 0); stop/ls/projects return plain errors in --json mode
- [x] 1.4 Numeric project names resolved as ids; only `#id` means id (cli/catalog.go)

## Batch 2 — pauses and break
- [ ] 2.1 inactive_since never deleted from SQLite → pauses never fire again (wellbeing)
- [ ] 2.2 StartBreak failure leaves work timers stopped; whitespace-only label
- [ ] 2.3 Dismissed/expired/"Abrir Nexus" notification counted as pause done

## Batch 3 — TUI
- [ ] 3.1 breakView index panic with long text
- [ ] 3.2 Wide: wideRunning ignores runScroll → Stop hits the wrong task
- [ ] 3.3 Wide + screenBreak: clicks in columns 0-1 ignored
- [ ] 3.4 Edit from Recent: focus stuck and edit buttons unreachable by keyboard
- [ ] 3.5 Ctrl+Z restores stale deletions and steals text-input undo

## Batch 4 — catalog, trash, tasks
- [ ] 4.1 Deleting a client/org fails with duplicate name (partial unique + ON DELETE SET NULL)
- [ ] 4.2 Restore/RestoreTask revives stopped sessions or violates one-running-per-task
- [ ] 4.3 EditTask returns a synthetic entry (EndedAt nil, StartedAt 0)

## Batch 5 — migrations and remaining medium findings
- [ ] 5.1 foreign_key_check runs after Commit (platform/db/migrate.go)
- [ ] 5.2 Remaining medium findings (to be listed after batch 4)
- [ ] 5.3 docs/ARCHITECTURE §9: pauses count computer use, not running timers
