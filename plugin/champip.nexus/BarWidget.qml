import QtQuick
import Quickshell.Io
import qs.Ui
import qs.Commons
import "Model.js" as Model

BarWidget {
  id: root
  moduleName: "champip.nexus"

  property int displayedTodaySeconds: client.todaySeconds

  // Shape contract required by Bar.findPanelWidget for shell summon/toggle routing.
  readonly property bool opened: panel.open

  function open() { panel.open = true }
  function close() { panel.open = false }
  function toggle() { panel.open = !panel.open }
  function poll() { client.refresh() }

  implicitWidth: button.implicitWidth
  implicitHeight: button.implicitHeight

  NexusClient { id: client }

  IpcHandler {
    target: "champip.nexus"
    function open(): void { root.open() }
    function close(): void { root.close() }
    function show(): void { root.open() }
    function hide(): void { root.close() }
    function toggle(): void { root.toggle() }
  }

  Component.onCompleted: {
    client.refresh()
    client.loadProjects("")
  }

  Timer {
    interval: 5000
    running: true
    repeat: true
    onTriggered: client.refresh()
  }

  Timer {
    interval: 1000
    running: true
    repeat: true
    triggeredOnStart: true
    onTriggered: {
      root.displayedTodaySeconds = Model.liveTodaySeconds(
        client.todaySeconds, client.running.length, client.sampledAtMs, Date.now())
    }
  }

  WidgetButton {
    id: button
    anchors.fill: parent
    bar: root.bar
    active: client.running.length > 0
    useActiveColor: false
    text: Model.barLabel(root.displayedTodaySeconds, client.running.length)
    tooltipText: client.running.length === 0
      ? "Nexus: sin temporizadores"
      : client.running.map(function(entry) {
          return entry.title + (entry.project ? " (" + entry.project + ")" : "")
            + "  " + Model.formatClock(entry.started_at, Date.now())
        }).join("\n")
    onPressed: function(buttonCode) {
      if (buttonCode === Qt.MiddleButton) client.stopAll()
      else if (buttonCode === Qt.LeftButton) root.toggle()
    }
  }

  NexusPanel {
    id: panel
    bar: root.bar
    anchorItem: button
    owner: root
    client: client
    open: false
  }
}
