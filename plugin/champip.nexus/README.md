# Nexus Omarchy bar plugin

## Install

Install the Nexus CLI with `make install`, then enable the `champip.nexus` bar widget in Omarchy. This checkout can be linked at `~/.config/omarchy/plugins/champip.nexus`; shell hot reload picks up saved plugin files.

## Usage

- Click the bar widget to open or close the quick-capture popup. Enter a title and press Enter or **Iniciar** to start a timer; choose a project and optional description first if needed.
- Use the project search to select an existing project, **Sin proyecto**, or create a project from unmatched text.
- Stop active timers in the popup. Middle-click the bar widget to stop all timers.
- **Abrir detalle** runs `omarchy-launch-or-focus-tui nexus` and closes the popup.
- Toggle from the shell IPC wrapper: `omarchy-shell champip.nexus toggle`. The plugin also supports `open` and `close` (plus `show`/`hide`) on target `champip.nexus`.

## CLI contract

The plugin invokes `nexus` with Quickshell `Process` argument arrays (never a shell command string):

- `nexus status --json` returns `{running:[{id,title,project,started_at,elapsed}],count,today_seconds}`.
- `nexus projects --json [-q query]` returns project objects with `name`, `last_used`, and `seconds`.
- Starts use `nexus start <title> [-p project] [-d description] --json`.
- Stops use `nexus stop <id> --json` or `nexus stop --all --json`.

## Structure

- `BarWidget.qml` owns the shared client, bar button, and plugin IPC target.
- `NexusClient.qml` contains all CLI process calls and JSON parsing.
- `NexusPanel.qml` is the keyboard- and mouse-driven quick-access popup.
- `Model.js` contains pure formatting and project filtering helpers.
- `manifest.json` registers `BarWidget.qml` as the `barWidget` entry point.

## Development notes

- The shell caches compiled QML components and directory listings. After editing,
  adding or renaming plugin files, run `omarchy restart shell`; `rescanPlugins` is
  not enough once a component has loaded (or failed to load).
- Do not name a local type like a `qs.Ui` type (for example `Panel`): the Ui type wins
  and the local file is silently shadowed. That is why the popup is `NexusPanel.qml`.
- `KeyboardPanel` only hosts `Item` children; keep `Process`/`Timer`/`Connections`
  inside an invisible `Item`.
- Toggle from the command line with `omarchy-shell champip.nexus toggle`.
- `omarchy-shell shell summon <id>` prints `ok` or `unknown` and always exits 0: read stdout, not the exit code.
- `summon champip.nexus` opens the bar popup, so the pause overlay is a separate plugin (`champip.nexus-pausa`).
- Inside `Variants`, ids of per-screen items are not visible from the root; let each item take focus itself.
- `import "/abs/path"` is invalid in QML; use `import qs.Ui`.
