# nexus-catalog

Goal: edit/delete entries, rename/merge/delete projects, and make projects real entities under
Client > Organization (see docs/ARCHITECTURE.md, phase 1).

Next phase after this one: break countdown with reminder (user priority).

Non-goals: daemon, reminders, integrations, dropping the legacy `entries.project` text column
(the contract step needs separate authorization).

Follows docs/ARCHITECTURE.md v2 (phase 1): package-by-module (`internal/tracking`, `internal/catalog`,
`internal/platform/db`), entity `uid` (ULID), soft delete (`deleted_at`), entry `kind` (`work|break`),
data dir `0700` / database `0600`.

## Migration contract (expand -> migrate -> verify)
- Per-module migrations in `schema_migrations(module, version, applied_at)`; each step idempotent and transactional.
- Expand only: new tables `organizations`, `clients`, `projects`; new nullable `entries.project_id`.
  `entries.project` (text) is KEPT and stays the display name, so an older binary keeps working.
- Backfill on every open: one project per distinct non-empty `entries.project`, link rows whose
  `project_id` is NULL (covers entries written by an older binary during the overlap).
- Automatic backup before the first migration of a real DB (`VACUUM INTO nexus.db.bak-v<N>`).
- Rollback = run the previous binary (additive schema) or restore the backup.
- A running timer (ended_at NULL) must survive migration untouched.

## Tasks
- [x] 1a. Safe refactor to package-by-module: internal/tracking (domain, service, ports, sqlite), internal/platform/db, internal/adapters/{cli,tui}; behavior unchanged, tests green before and after (commit f84db33)
- [x] 1b. Per-module migrations (schema_migrations), catalog schema, backfill from entries.project, automatic backup, entry uid/kind/deleted_at, data dir 0700 / db 0600 (+ tests: preservation, running timer survives, idempotency, old binary still works) (commit 828c039; verified on a copy of the real DB)
- [ ] 2. App: edit/delete entry; catalog use cases (projects, clients, organizations: create, rename, merge, move, delete)
- [ ] 3. CLI: edit, rm, project/client/org commands, additive JSON fields, Spanish text
- [ ] 4. TUI: edit/delete an entry from the lists (Enter, confirmation, mouse)
- [ ] 5. TUI: catalog screen (organization > client > project), rename/move/delete
