# Nexus architecture

> Status: proposal v2 · 2026-10-03 · Spanish version: [ARCHITECTURE.es.md](ARCHITECTURE.es.md)

Nexus is a local-first **personal control center** for people who work all day at a Linux desk.
It tracks time, and later shows and acts on work that lives in other tools (Notion, Google
Calendar, Gmail, Obsidian, Trello), so a glance at the bar replaces opening one window per tool.

It must work for its author first and for anyone who installs it from a public repository later.
That second goal shapes this document: **safe by default, local by default, nothing exposed to the
internet unless the user explicitly turns it on**.

---

## 1. Principles

1. **Local-first.** All data lives in one SQLite file on the user's machine. No Nexus cloud, no account.
2. **Works offline, degrades gracefully.** Tracking never depends on the network, the daemon, or an integration.
3. **One core, many faces.** CLI, TUI, bar popup and AI assistants are adapters over the same use cases.
4. **Least privilege.** Each integration starts read-only and asks for the smallest scope that works.
5. **The human approves effects.** Anything that changes an external system, or that an AI requested, needs explicit approval in Nexus itself.
6. **Boring technology.** Go, SQLite, systemd user services, freedesktop notifications, the system keyring.
7. **Reversible changes.** Schema migrations are additive and backed up; destructive steps are separate and explicit.
8. **No overengineering.** Build the simplest thing that works for one user today; add a layer only when a real need appears. Fast and good-looking beats feature-complete.

## 2. Feasibility research (2026-10-03)

Each row was checked against primary sources, not only search summaries. Where the sources disagreed, the table keeps the primary source.

| Question | Finding | Consequence for Nexus |
|---|---|---|
| Can it ship as a public Omarchy plugin? | Yes. `omarchy plugin add <git-url>` clones a **whole git repo that has `manifest.json` at its root** into `~/.config/omarchy/plugins/<id>/`. The installer never runs build steps or hooks. Plugins run **unsandboxed** inside `omarchy-shell`. There is a community index (plugins.omarchy.org) and more than 1,100 repos use the GitHub topic `omarchy-plugin`. | The bar plugin needs its **own repository** (the Go source must not be cloned into the shell's plugin folder). The `nexus` binary is installed separately. Keep the QML small and free of secrets. |
| Can ChatGPT web use a Nexus MCP server? | ChatGPT web reaches MCP servers only over **public HTTPS**; it cannot reach `localhost`. OpenAI's docs describe developer mode with write actions as a beta for Business/Enterprise/Edu, but **the author verified on 2026-10-03 that a ChatGPT Plus account connects to a custom MCP server exposed through an ngrok URL and uses its tools**. OpenAI also offers **Secure MCP Tunnel** (`openai/tunnel-client`). | Feasible for the author's plan: a local HTTP MCP server exposed through a tunnel, protected by a secret URL token (§11.3). Local stdio MCP stays available for desktop agents. |
| Which MCP version? | Current spec is **2026-07-28**: stateless core (no `Mcp-Session-Id`), transports stdio and Streamable HTTP, authorization based on OAuth 2.1 + Protected Resource Metadata (RFC 9728) + resource indicators (RFC 8707). Official Go SDK: `github.com/modelcontextprotocol/go-sdk`. | Use the official Go SDK. Model state as explicit handles in tool arguments, never as connection state. |
| Gmail and Google Calendar for a public app? | Gmail read/modify scopes are **restricted**. A public app with a shared OAuth client would need Google verification plus a yearly paid **CASA** security assessment. Calendar scopes are **sensitive** (verification, no CASA). The usual way out for open-source local apps is **bring your own client** (BYOC): each user creates their own Google Cloud project and a "Desktop app" OAuth client. If the consent screen stays in *Testing*, refresh tokens expire after 7 days; publishing it to *In production* without verification avoids that (users see an "unverified app" warning). | Google connectors use **BYOC**, never a client ID shipped by Nexus. Loopback redirect + PKCE. Ship a setup guide. |
| Notion? | An **internal integration token** per workspace (the user shares the databases with it). API version **2025-09-03** replaced database queries with **data sources** (`data_source_id`). Rate limit is about 3 requests/second. Webhooks exist but need a public endpoint. | Token in the keyring; pin `Notion-Version`; token-bucket client; polling instead of webhooks (no public endpoint). |
| Obsidian? | A vault is a folder of Markdown files. | File adapter with atomic writes; no API, no plugin required. |
| Trello? | API key + user token for personal tools. | Same connector port; low priority. |
| Secrets on this desktop? | `gnome-keyring` provides `org.freedesktop.secrets`. | Store secrets through the Secret Service (keyring). Never in SQLite, config files or the repo. |
| Notification buttons? | The Omarchy notification server advertises `actions`. | "Back to work / +10 min / End break" can be buttons on the notification itself. |
| Is the name `nexus` free? The exact name `nexus` is free in AUR and the official repos, but `nexus-bin` and `nexus-cli` (unrelated, 0 and 1 votes) also install `/usr/bin/nexus`, and `nexus-bin` declares `provides=('nexus')`. | Package `nexus`, declaring `conflicts` with both (see §13). |

## 3. System context

```
                    ┌───────────────────────────── user's machine ─────────────────────────────┐
                    │                                                                            │
 Terminal  ───────► │  nexus (CLI / TUI)  ──┐                                                    │
 Omarchy bar ─────► │  bar plugin (QML) ────┼──►  Nexus core (use cases)  ──►  SQLite (nexus.db) │
 AI client (stdio)► │  nexus mcp ───────────┘            ▲                                       │
                    │                                    │                                       │
                    │  nexus daemon (systemd --user) ────┘  reminders · sync · outbox · notify   │
                    │        │                                                                   │
                    └────────┼───────────────────────────────────────────────────────────────────┘
                             ▼ HTTPS, user's own credentials (keyring)
              Notion · Google Calendar · Gmail · Trello          Obsidian vault (local files)
```

Optional, off by default: remote MCP endpoint for ChatGPT web (§11.3).

## 4. Architectural style

**Modular monolith with hexagonal (ports and adapters) modules.**

- One repository, one Go binary (`nexus`) with subcommands, one database.
- The code is split into **modules (bounded contexts)**. Each module owns its domain types, use cases, ports and tables.
- Dependencies point inward: adapters → use cases → domain. Adapters implement ports that the module defines.
- Modules talk to each other only through the other module's public service interface or domain events, **never through its tables**.
- The current layout is layered (`internal/domain`, `internal/app`, `internal/store`). It moves to **package-by-module** when the catalog lands (the first new module), so each new capability adds a folder instead of touching every layer.

Microservices are explicitly rejected: one user, one machine, one database. A module can still be extracted later because its boundary is already a port.

## 5. Modules

| Module | Responsibility | Depends on |
|---|---|---|
| `tracking` | Time entries: start, stop, edit, delete (soft), reports. Entry `kind`: `work` or `break`. | `catalog` (project lookup) |
| `catalog` | Organizations → clients → projects. Create, rename, merge, move, archive. | — |
| `countdown` | Timed countdowns (breaks, focus blocks) with a deadline and an overdue state. | `tracking` |
| `wellbeing` | Movement and posture reminders based on continuous work time and idle state. | `tracking` |
| `integrations` | One connector per external system behind one port; sync engine (inbox, outbox, links). | `catalog`, `tracking` |
| `agenda` | Read model for the "one glance" view: today's events, tasks, unread mail. | `integrations` |
| `approvals` | Pending actions that need a human decision (AI requests, external writes). | — |
| `automation` *(late, optional)* | Browser automation of the user's own websites. Separate process. | `approvals` |

Shared platform code (not a module, no business rules): database and migrations, clock, config,
keyring, notifier, logging, i18n.

## 6. Code layout (target)

```
cmd/nexus/                 composition root: wires modules and adapters, nothing else
internal/
  platform/
    db/                    SQLite open, pragmas, per-module migrations, backup
    clock/ config/ keyring/ notify/ i18n/ log/
  tracking/
    domain.go              entities, invariants, domain errors
    service.go             use cases
    ports.go               interfaces the module needs
    sqlite.go              repository adapter
    *_test.go
  catalog/  countdown/  wellbeing/  approvals/  agenda/
  integrations/
    port.go                Connector interface, capabilities, Change type
    sync/                  inbox, outbox, scheduler, link table
    notion/  gcal/  gmail/  obsidian/  trello/
  adapters/
    cli/                   commands, presenters, JSON contract
    tui/                   Bubble Tea UI
    mcp/                   MCP server (stdio, later Streamable HTTP)
    daemon/                long-running loop, local socket
plugin/                    development copy of the bar plugin (published to its own repo)
docs/
```

An **architecture test** (`go list` import graph) fails the build if a module imports another
module's internals, if `domain.go` imports anything but the standard library, or if an adapter
imports another adapter.

## 7. Runtime model

| Process | Lifetime | Role |
|---|---|---|
| `nexus <command>` | one shot | CLI. Writes SQLite directly. |
| `nexus` (no args) | interactive | TUI. Writes SQLite directly. |
| bar plugin | inside `omarchy-shell` | Calls the CLI with argv arrays and reads its versioned JSON. Never touches SQLite. |
| `nexus daemon` | systemd `--user` service | Deadlines (break ends, movement reminders), notifications with actions, sync and outbox delivery, approvals, optional MCP over HTTP. |
| `nexus mcp` | started by the AI client | MCP over stdio. Same use cases, same database. |

Rules:

- **The tracker works without the daemon.** Overdue state is computed from stored deadlines, so
  the bar and TUI still show "break +12 min" if the daemon is down; only the notification is lost.
- SQLite in WAL mode with a busy timeout handles the few concurrent writers.
- Later, the daemon exposes a local Unix socket (mode `0600`) so the UIs can subscribe to changes
  instead of polling. Polling stays as the fallback.

## 8. Data architecture

- **One file:** `$XDG_DATA_HOME/nexus/nexus.db`, directory `0700`, file `0600`.
- **Migrations per module:** table `schema_migrations(module, version, applied_at)`. Each step is
  transactional and idempotent. **Before the first pending migration**, a backup is written with
  `VACUUM INTO nexus.db.bak-<timestamp>`.
- **Expand → migrate → verify → contract.** Contract steps (dropping columns) need a separate
  release and an explicit decision.
- **Identifiers:** integer primary keys inside the database, plus a ULID `uid` for every entity
  that leaves the process (MCP, external links, exports), so ids can't be guessed or confused.
- **Time:** stored as UTC Unix seconds; time zones apply only at presentation.
- **Deletion:** user data is **soft-deleted** (`deleted_at`) and purged after a retention period,
  so a mistaken delete can be undone.
- **Events and outbox:** use cases append domain events (`events` table) in the same transaction
  as the change. The outbox for connectors and the audit log read from it.
- **External identity:** `external_links(local_type, local_uid, system, external_id, etag, synced_at)`.
  Nexus never matches external objects by name.

### Domain model

```
Organization (e.g. Holinsys, Emana)
 └─ Client (e.g. Depilab)
     └─ Project (e.g. Depiloto, Lumirecon)
         └─ TimeEntry  { title, description, kind: work|break, started_at, ended_at, deleted_at }

Countdown { kind: break|focus|custom, label, duration, started_at, ends_at, finished_at,
            entry_uid, resume_entry_uids[] }
Reminder  { rule, next_at, last_done_at }        PendingAction { requested_by, payload, status }
```

A project may have no client (personal projects); a client may have no organization. The current
free-text `entries.project` is migrated into `projects` and kept as a compatibility column for
one release (see the catalog plan in `odd/tasks/nexus-catalog.md`).

## 9. Breaks and well-being

**Break countdown.** "Break 1 h" starts a `break` entry and a `Countdown`. If work timers are
running, Nexus **asks every time** which ones to stop (all are preselected); the stopped ones are
remembered in `resume_entry_uids`. When the deadline passes:

1. The daemon sends a notification with actions: **Back to work** (stops the break, restarts the
   remembered timers), **+10 min**, **End break**.
2. The bar shows the overdue time (`break +12m`) in the warning color until the user acts.
3. Nexus never restarts work timers by itself; the user may not be back yet.

**Movement reminders.** A rule such as "after 50 minutes of continuous work, suggest moving for
5 minutes". It only counts time while a work timer runs and the user is not idle. Actions:
**Done**, **In 10 min**, **Skip**. Completed reminders are recorded for a weekly summary. The
source of the idle signal (Omarchy's idle service or the Wayland idle protocol) is an open question.

## 10. Integrations

### Connector port

```go
type Connector interface {
    ID() string
    Capabilities() Capabilities               // ReadTasks, WriteTasks, ReadEvents, ...
    Pull(ctx context.Context, cursor Cursor) ([]Change, Cursor, error)
    Push(ctx context.Context, change Change) (ExternalRef, error)  // only if a Write capability exists
}
```

A fake connector drives the sync tests. Each real connector lives in its own package and is
registered in the composition root.

### Sync rules

1. **Source of truth per field.** Time data: Nexus. Tasks: Notion (or Trello). Events: Google
   Calendar. Mail: Gmail. Conflicts resolve by this rule, never by "last write wins".
2. **Read-only first.** Every connector ships read-only; writing is a separate, later step.
3. **Outbox.** Writes go to the outbox in the same transaction as the local change; the daemon
   delivers them with retries, backoff and idempotency keys.
4. **Rate limits** are enforced in the client (token bucket per connector) and honor `Retry-After`.
5. **No public webhooks** by default: connectors poll with cursors.

### Per-system notes

| System | Auth | Notes |
|---|---|---|
| Google Calendar | BYOC Desktop OAuth client, loopback redirect, PKCE | Sensitive scope. Start with `calendar.readonly`. |
| Gmail | BYOC, same flow | Restricted scope. Start with metadata or read-only and show subject and sender only. Mail text is **untrusted input** (§12). |
| Notion | Internal integration token | Pin `Notion-Version: 2025-09-03` or newer; use `data_source_id`; ~3 req/s. |
| Obsidian | Local folder path | Read tasks and front matter; atomic writes; never delete user files. |
| Trello | API key + token | Same port as Notion for kanban data. |

## 11. AI access (MCP)

### 11.1 Local, first

`nexus mcp` speaks MCP over **stdio** using the official Go SDK. The AI client starts it as a
child process; nothing listens on the network. This works with Codex CLI, Claude Code and other
desktop clients today.

Agents that have a shell (Claude Code or Codex in another terminal) can already drive Nexus through
the CLI and its `--json` contract. MCP adds typed tools and the tier policy below, so it is the
preferred door for AI clients.

### 11.2 Tool design

| Tier | Examples | Policy |
|---|---|---|
| Read | `list_running`, `report`, `list_projects`, `agenda_today` | Allowed. |
| Write (local) | `start_timer`, `stop_timer`, `start_break` | Allowed in local mode; configurable. |
| Write (external) | `move_notion_card`, `create_event` | Creates a **PendingAction**; the user approves it in Nexus (notification button, popup or TUI). |
| Destructive | delete entries, delete projects, send mail | **Not exposed** to AI clients. |

Tool results that contain third-party text (mail, task descriptions) are returned as clearly
marked data, truncated, and stripped of markup. Every MCP call is written to the audit log.

### 11.3 Remote (ChatGPT web), opt-in

Verified path: ChatGPT connects to an MCP URL exposed through a tunnel. Nexus keeps it simple:

```
nexus mcp serve                                   # Streamable HTTP on 127.0.0.1, prints the URL to paste in ChatGPT
cloudflared tunnel --url http://127.0.0.1:<port>  # Cloudflare quick tunnel, no account needed
```

- **Off by default.** `nexus mcp serve` binds only to `127.0.0.1`. The tunnel is the user's choice:
  Cloudflare quick tunnel (no login; random `*.trycloudflare.com` URL that changes on every restart;
  intended for testing), ngrok, or a named Cloudflare tunnel (stable URL, needs an account).
- **Authentication v1: secret URL token.** Nexus generates a random 256-bit token, keeps it in the
  keyring, and serves MCP only at `/mcp/<token>`; any other path returns 404. This works with
  ChatGPT's "no authentication" connector setting.
  - Known weaknesses: the URL *is* the password. It can end up in tunnel logs, history and the
    ChatGPT settings, and it does not expire.
  - Mitigations: `nexus mcp token rotate`, constant-time comparison, rate limiting, audit log, and a
    reduced remote tool set (read tier, local timer actions and PendingActions; no destructive
    tools and no direct external writes).
- **Authentication v2, only if needed:** an OAuth 2.1 resource server with short-lived scoped
  tokens, once remote access can change mail or calendar data or more people use it.
- **Kill switch:** stopping `nexus mcp serve` or rotating the token cuts access immediately.

## 12. Security model

**Assets:** OAuth tokens and API keys; mail, calendar and task content; time records.

**Threats and controls:**

| Threat | Control |
|---|---|
| Another local user reads the data | Data directory `0700`, database `0600`; secrets only in the keyring. |
| A token leaks through files, logs or the repo | Keyring only; logs redact secrets; no secrets in config files. |
| Shell injection from user text | Every subprocess uses argv arrays; the QML plugin never builds shell strings. |
| Prompt injection through mail or task text | Third-party text is marked as data and truncated; external writes require human approval in Nexus; destructive tools are not exposed. |
| An AI or a compromised token triggers harmful writes | PendingAction approval, scoped short-lived tokens, audit log, kill switch. |
| Network exposure | Nothing listens on the network by default; HTTP binds to loopback; remote access is opt-in. |
| Malicious plugin update (shell runs plugins unsandboxed) | Minimal QML, no network access from QML, signed release tags, `omarchy plugin update` shows the diff. |
| Supply chain | Few dependencies, `govulncheck` in CI, checksums and signatures on releases. |

A `SECURITY.md` with a private disclosure channel is required before the first public release.

## 13. Distribution and publishing

**Two artifacts, released together:**

| Artifact | Contents | Install |
|---|---|---|
| `nexus` binary | CLI, TUI, daemon, MCP | AUR package, GitHub release (static Go binary, checksums, signature), `go install` |
| bar plugin | `manifest.json` at the root + QML + `Model.js` | `omarchy plugin add https://github.com/<owner>/omarchy-nexus` |

- The plugin is developed in `plugin/` in this repository and **mirrored by CI** to the plugin
  repository, because `omarchy plugin add` clones a whole repository from its root.
- The plugin detects a missing or too-old binary and shows "Install Nexus" instead of failing.
- **Contract versioning:** every JSON output of the CLI includes `"api": N`. The plugin declares the
  range it supports. Breaking the contract means a new major version.
- **Naming:** package `nexus` (AUR, built from source), binary `nexus`, plugin id `<owner>.nexus`.
  The package declares `conflicts=('nexus-bin' 'nexus-cli')` because both install `/usr/bin/nexus`.
  A prebuilt-binary package needs another name, since `nexus-bin` is taken.
- **License:** Apache-2.0 (already in the repository).
- **systemd:** the package installs a user unit (`nexus.service`) that is **disabled** until the
  user enables it.

## 14. Configuration, language, observability

- **Config:** `$XDG_CONFIG_HOME/nexus/config.toml` (reminder rules, enabled connectors, language).
- **Language:** user-facing text goes through message catalogs (`es`, `en`) chosen from config or
  `LANG`. Code, identifiers and logs stay in English.
- **Logs:** only the daemon logs, as structured `slog` lines to `$XDG_STATE_HOME/nexus/`,
  with rotation and secret redaction. `nexus doctor` checks paths, permissions, keyring, daemon,
  plugin version and connector health.

## 15. Quality gates

| Layer | Test |
|---|---|
| Domain and use cases | Unit tests with a fake clock and fake repositories |
| SQLite adapters and migrations | Temp-database tests: data preservation, idempotency, old binary + new schema |
| CLI JSON contract | Golden files per `api` version |
| Connectors | Recorded fixtures and the fake connector; never real accounts in CI |
| Plugin | `node` tests for `Model.js`; `omarchy plugin validate` in CI |
| Architecture | Import-graph test (§6) |
| Release | `go vet`, `gofmt`, `govulncheck`, reproducible build, checksums |

## 16. Roadmap

| Phase | Scope | Done when |
|---|---|---|
| 0 | Tracker: CLI, TUI, bar popup | **Done** |
| 1 | Catalog (organization → client → project), edit and soft-delete entries, rename/merge projects, package-by-module refactor, file permissions | Existing data migrated with backup; TUI and CLI can fix any mistake |
| 2 | Daemon, break countdown with notification actions, movement reminders, `nexus doctor` | A forgotten break produces a notification and a visible overdue state |
| 3 | Public release: plugin repository, AUR package, i18n (`es`/`en`), first-run tutorial, `SECURITY.md`, CI | A new user installs both artifacts from the README alone |
| 4 | MCP: local stdio and `nexus mcp serve` behind a tunnel with a secret URL token; read and local-write tiers; audit log | ChatGPT (Plus) and a desktop agent can start, stop and report timers |
| 5 | Read-only connectors: Calendar agenda, Notion tasks, Gmail unread, Obsidian; agenda in popup and TUI | Today's events and tasks visible without opening another window |
| 6 | Write-back through outbox and PendingActions (move a Notion card, create an event) | Every external write is approved, audited and retried safely |
| 7 | OAuth for remote MCP, only if remote access must change mail or calendar data | Threat model reviewed; tokens short-lived and scoped |
| 8 | Browser automation of the user's own sites (separate process) | Dry run and approval before any change |
| Later | Sync between several computers | Not planned yet; the `uid` and event log keep the door open |

## 17. Decisions and open questions

**Decisions**

- D1. Modular monolith, hexagonal modules, package-by-module from phase 1.
- D2. SQLite as the only database; per-module migrations with automatic backup.
- D3. Local-first; nothing exposed to the network by default.
- D4. Google integrations use bring-your-own OAuth client.
- D5. Local MCP over stdio first; remote MCP is opt-in and comes later.
- D6. External writes and AI-requested actions require approval inside Nexus.
- D7. The bar plugin is a thin client of the CLI JSON contract and lives in its own repository.
- D8. Starting a break asks every time which running timers to stop.
- D10. The package and the binary are both named `nexus`.
- D9. Remote MCP v1 uses a secret URL token behind a user-chosen tunnel (Cloudflare quick tunnel recommended); OAuth only when needed.

**Open questions**

- Q2. Idle signal source for movement reminders.
- Q3. Whether OpenAI Secure MCP Tunnel can serve ChatGPT web for the user's plan.
- Q6. Name for the prebuilt-binary package (`nexus-bin` is taken); choose an available one before the first public release.
- Q5. Retention period for soft-deleted entries (proposal: 30 days).

## 18. Sources

- Omarchy plugin installer and shell README (local: `/usr/share/omarchy/shell/README.md`, `omarchy plugin add --help`); community index https://plugins.omarchy.org
- MCP specification 2026-07-28 and changelog: https://modelcontextprotocol.io/specification/2026-07-28
- MCP Go SDK: https://github.com/modelcontextprotocol/go-sdk
- MCP in ChatGPT and Codex: https://learn.chatgpt.com/docs/extend/mcp.md
- OpenAI MCP and Secure MCP Tunnel: https://developers.openai.com/api/docs/guides/tools-connectors-mcp
- Developer mode and MCP apps in ChatGPT: https://help.openai.com/en/articles/12584461
- Google restricted scope verification: https://developers.google.com/identity/protocols/oauth2/production-readiness/restricted-scope-verification
- Google security assessment (CASA): https://support.google.com/cloud/answer/13465431
- Notion API 2025-09-03 upgrade guide: https://developers.notion.com/guides/get-started/upgrade-guide-2025-09-03
- OWASP MCP Top 10 (prompt injection, tool poisoning, token handling)
