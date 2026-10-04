# nexus-catalog

Goal: edit/delete entries, rename/merge/delete projects, and make projects real entities under
Client > Organization (see docs/ARCHITECTURE.md, phase 1).

Non-goals: daemon, reminders, integrations, dropping the legacy `entries.project` text column
(the contract step needs separate authorization).

## Migration contract (expand -> migrate -> verify)
- Versioned migrations through `PRAGMA user_version`; each step idempotent and transactional.
- Expand only: new tables `organizations`, `clients`, `projects`; new nullable `entries.project_id`.
  `entries.project` (text) is KEPT and stays the display name, so an older binary keeps working.
- Backfill on every open: one project per distinct non-empty `entries.project`, link rows whose
  `project_id` is NULL (covers entries written by an older binary during the overlap).
- Automatic backup before the first migration of a real DB (`VACUUM INTO nexus.db.bak-v<N>`).
- Rollback = run the previous binary (additive schema) or restore the backup.
- A running timer (ended_at NULL) must survive migration untouched.

## Tasks
- [ ] 1. Store: versioned migrations, catalog schema, backfill, backup (+ tests of preservation/idempotency/mixed version)
- [ ] 2. App: edit/delete entry; catalog use cases (projects, clients, organizations: create, rename, merge, move, delete)
- [ ] 3. CLI: edit, rm, project/client/org commands, additive JSON fields, Spanish text
- [ ] 4. TUI: edit/delete an entry from the lists (Enter, confirmation, mouse)
- [ ] 5. TUI: catalog screen (organization > client > project), rename/move/delete
