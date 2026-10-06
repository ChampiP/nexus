# Nexus

Local-first time and task tracker for Linux desktops, built for [Omarchy](https://omarchy.org).
Nexus ships as one Go binary with a CLI, a terminal UI, an Omarchy bar widget and a small
background service that reminds you to take breaks and move.

Everything stays in a single SQLite file on your machine. There is no account, no cloud and no telemetry.

> The user interface is currently in **Spanish**. Internationalization is planned.

## Features

- **Timers.** Start, stop and resume work. One task can hold several sessions, and several timers can run at once.
- **Hierarchy.** Organization › Client › Project › Task › Session. Project names only need to be unique within their client.
- **Terminal UI.** One three-column screen (at 120+ columns wide) that you drive with the arrow keys, Enter, Esc, Tab and the mouse.
- **Omarchy bar widget.** Shows today's time and opens a quick-capture popup that can start and stop timers.
- **Breaks.** A break countdown pauses your timers and sends a notification when it ends.
- **Active pauses.** Movement reminders based on how long you have actually used the computer. They wait while you are idle or the screen is locked.
- **Trash and undo.** Deleted tasks go to the trash and can be restored.
- **Scriptable.** Most commands accept `--json`.

## Requirements

- Go 1.26 or newer (only to build from source)
- Linux with systemd (for the reminder service)
- Omarchy, for the bar widget and the native pause overlay. You can use the CLI and the TUI without it.

## Install

```sh
git clone https://github.com/ChampiP/nexus.git
cd nexus
make install
omarchy plugin enable champip.nexus
```

`make install` does four things:

- builds `nexus` into `~/.local/bin`
- links both Omarchy plugins into `~/.config/omarchy/plugins`
- installs and starts the `nexus.service` systemd user unit
- enables the pause overlay plugin

To build just the binary, run `make build`. To remove the service, run `make uninstall-service`.

## Usage

Run `nexus` with no arguments to open the terminal UI.

```sh
nexus start "Write report" -p "acme/website" -d "Q3 numbers"
nexus status
nexus stop --all
nexus resume <id>
nexus report --week

nexus break 15              # 15-minute break, pauses running timers
nexus pausa --cada 30 --dura 30s   # remind every 30 min of use, 30 s pause

nexus tree                  # organization > client > project
nexus client add acme
nexus project add website -c acme
```

Project references accept a name, `client/project` or `#id`. Run `nexus --help` for the full command list.

## Data

The database lives at `~/.local/share/nexus/nexus.db`. To use another file, set `NEXUS_DB`:

```sh
NEXUS_DB=/tmp/test.db nexus status
```

Migrations run automatically. They are idempotent and safe when several Nexus processes start at the same time.

## Development

```sh
make test   # go vet, all tests, and the architecture test
```

To work on Nexus without touching your real data, set both `NEXUS_DB` and `HOME` to temporary paths.

The architecture and design decisions are described in [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Roadmap

- Local web dashboard
- MCP connector so AI assistants can read and start timers (opt-in, local by default)
- Integrations: Notion, Google Calendar, Gmail, Obsidian
- Translations

## Contributing

Issues and pull requests are welcome. Please open an issue to discuss larger changes first, and run `make test` before you submit a pull request.

## License

[Apache License 2.0](LICENSE)
