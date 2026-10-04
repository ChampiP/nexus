# Nexus architecture

Nexus is a personal **control center**: one place (terminal, bar popup, later any AI client)
to track time and see/act on work that lives in other tools (Notion, Google Calendar, Gmail,
Obsidian) without opening a window for each one.

Today only the time tracker exists. This document fixes the shape so every future piece plugs in
without rewriting the core.

## Style

Modular monolith, hexagonal inside. One repository, one binary (`nexus`), modules with explicit
boundaries. Dependencies only point inward:

```
adapters in  ->  app (use cases)  ->  domain
adapters out <-  ports (interfaces owned by app)
```

## Domain model

```
Organization  (Holinsys, Emana)
 └─ Client    (e.g. Depilab)
     └─ Project   (Depiloto, Lumirecon, ...)
         └─ TimeEntry (title, description, started_at, ended_at, kind)
```

- `kind`: `work` | `break` (breaks can have a target duration).
- Today `project` is a free-text field on the entry. The catalog (organizations, clients,
  projects as entities) replaces it so rename/merge/report-by-client are a single row change.
- Every external object (Notion card, calendar event, mail thread) is linked through an
  `external_links(local_type, local_id, system, external_id)` table. Nexus never guesses identity.

## Modules (bounded contexts)

| Module | Owns | Status |
|---|---|---|
| `tracking` | timers, entries, edit/delete, breaks | exists (`internal/app`) |
| `catalog` | organizations, clients, projects | planned |
| `reminders` | break countdowns, movement/stretch nudges, notifications | planned |
| `integrations` | one connector per external system, behind one port | planned |
| `agenda` | read model: today's events, tasks, unread mail for the one-window view | planned |

## Adapters

| Direction | Adapter | Notes |
|---|---|---|
| in | CLI, TUI, bar popup (QML) | exist; QML talks to the CLI JSON contract only |
| in | MCP server | local stdio first; remote HTTPS needs OAuth (see Risks) |
| in | HTTP API | only if a client needs it |
| out | SQLite store | exists |
| out | Notifier (`notify-send` / D-Bus) | for reminders |
| out | Connectors: Notion, Google Calendar, Gmail, Obsidian | one package each, implementing the same port |

## Daemon

`nexus daemon` (systemd `--user` service) is the single long-running process. It owns everything
that must happen when no UI is open: break/movement reminders, connector sync, and later the MCP
server. UIs keep reading the SQLite database directly (WAL) and do not depend on the daemon, so
the tracker keeps working if the daemon is down.

## Integration rules

1. **Source of truth per field.** Time data: Nexus. Tasks: Notion. Events: Google Calendar.
   Mail: Gmail (read-only first). Conflicts resolve by that rule, never by "last write wins".
2. **Outbox.** Local changes are written to an `outbox` table in the same transaction; the daemon
   delivers them to connectors with retries and idempotency keys.
3. **Read first, write later.** Each connector ships read-only; write-back is a separate step.
4. **Secrets** live in the system keyring (libsecret), never in the database or the repo.
5. **Connector contract.** `Pull(since) -> []Change`, `Push(Change) -> ExternalID`; a fake
   connector drives the tests.

## Roadmap

| Phase | Scope |
|---|---|
| 0 | Time tracker: CLI, TUI, bar popup (done) |
| 1 | Edit/delete entries, rename/merge projects, catalog (organization > client > project) |
| 2 | Daemon + break countdown with notification + movement reminders |
| 3 | Read-only connectors: Calendar agenda, Notion tasks, Gmail unread, in popup/TUI |
| 4 | Write-back through the outbox (move a Notion card, create an event) |
| 5 | MCP server (stdio first, then remote) |
| 6 | Web automation (browser login and changes), isolated and last |

## Risks

- **Remote MCP and ChatGPT.** ChatGPT on the web cannot reach `localhost`; it needs a public HTTPS
  endpoint with OAuth. That endpoint would be able to act on Gmail and Calendar, so it needs its own
  threat model before it exists. Local MCP (stdio) has no such exposure.
- **Schema churn.** Phase 1 changes the project field into an entity; it needs a reversible
  migration that preserves existing entries.
- **Shell plugin coupling.** The bar popup caches QML in the shell; keep logic in Go and QML thin.
