# nexus-wellbeing

Goal: configurable active-break reminder that covers the screen (docs/ARCHITECTURE.md §9, movement reminders).

## Agreed behavior
- Every N minutes of work (a work timer running, no break active): N in {10,20,30,45,60}, default 30.
- Pause duration D in {10s,20s,30s,1m,2m,5m}, default 30 s, max 5 min.
- A native Omarchy overlay covers all screens: message, rotating tip, countdown; [Posponer 10 min] [Saltar];
  closes by itself when D ends. Fallback: critical notification when the overlay is not available.
- Never interrupts a meeting: microphone in use (pactl source-outputs), fullscreen window, or meeting-like
  window title; it waits until that ends + 2 min.
- Settings in the TUI Break tab ("Pausas activas") and CLI (`nexus pausa`); "No molestar 1 hora".
- Evidence: 5 min of light walking every 30 min offsets prolonged sitting (Columbia 2023); AOA 20-20-20 for eyes.

## Tasks
- [x] 1. Domain + CLI + daemon: wellbeing module (settings, schedule, snooze/skip/dnd), presence detection, daemon trigger, `nexus pausa`
- [x] 2. Overlay: QML overlay in the bar plugin, summoned by the daemon, reports back via CLI (needs one shell restart)
- [x] 3. TUI: "Pausas activas" settings in the Break tab
