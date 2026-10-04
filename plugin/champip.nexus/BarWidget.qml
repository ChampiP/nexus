import QtQuick
import Quickshell.Io
import qs.Ui

// Polls `nexus status --json` once per second. Left click opens the TUI,
// middle click stops every running timer.
BarWidget {
  id: root
  moduleName: "champip.nexus"

  property var running: []
  property int todaySeconds: 0

  function fmt(sec) {
    var h = Math.floor(sec / 3600), m = Math.floor((sec % 3600) / 60)
    return h + "h " + (m < 10 ? "0" : "") + m + "m"
  }

  implicitWidth: button.implicitWidth
  implicitHeight: button.implicitHeight

  Process {
    id: poll
    command: ["nexus", "status", "--json"]
    stdout: StdioCollector {
      onStreamFinished: {
        try {
          var s = JSON.parse(text)
          root.running = s.running || []
          root.todaySeconds = s.today_seconds || 0
        } catch (e) {
          root.running = []
        }
      }
    }
  }

  Timer {
    interval: 1000
    running: true
    repeat: true
    triggeredOnStart: true
    onTriggered: if (!poll.running) poll.running = true
  }

  WidgetButton {
    id: button
    anchors.fill: parent
    bar: root.bar
    active: root.running.length > 0
    useActiveColor: false
    text: root.running.length > 0
      ? "󱎫 " + root.running.length + " · " + root.fmt(root.todaySeconds)
      : "󱎫 " + root.fmt(root.todaySeconds)
    tooltipText: root.running.length === 0
      ? "Nexus: sin temporizadores"
      : root.running.map(function(e) {
          return e.title + (e.project ? " (" + e.project + ")" : "") + "  " + e.elapsed
        }).join("\n")

    onPressed: function(b) {
      if (!root.bar) return
      if (b === Qt.MiddleButton) root.bar.run("nexus stop --all")
      else root.bar.run("omarchy-launch-or-focus-tui nexus")
    }
  }
}
