# nexus-shutdown-stop

Goal: a running timer must stop at the moment the PC shut down, suspended, or crashed, instead of counting the off time. The user resumes it afterwards (Resume already exists).
Branch: fix/stop-on-shutdown (from main 233ba43).

Design: the daemon writes a heartbeat (Unix seconds) to `<db>.alive` about every 30 s. On startup and on every tick, if the last heartbeat is older than 2 minutes, running work sessions started before it are stopped at the heartbeat time and the user gets a notification. Breaks (countdown) are out of scope.

Evidence: entries #13 (11.6 h, PC off 12:28 to 22:47 on 2026-10-06) and #16 (8.1 h, PC off 00:17 to 07:00 on 2026-10-09) counted the off time.

Non-goals: idle/lock auto-stop, asking on resume, changing breaks.

- [x] 1 tracking: `StopRunningAt(at)` stops running work sessions started before `at`, ending at `at` (+ tests)
- [x] 2 daemon: file heartbeat, gap detection on startup and every tick, notification, wiring in cmd/nexus (+ tests)
- [x] 3 wellbeing: active pause reminder fires right after boot because active_since survives the shutdown; store the last observation time and reset the active streak when the gap is >= inactiveReset (+ tests)
- [ ] 3b verify isolated (HOME and NEXUS_DB temporary, copy of the real DB)
- [ ] 4 commit, install, restart nexus.service; back up the DB and fix #13 and #16 to the shutdown times
