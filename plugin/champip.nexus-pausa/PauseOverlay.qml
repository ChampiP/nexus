import Quickshell
import Quickshell.Io
import Quickshell.Wayland
import QtQuick
import QtQuick.Layouts
import qs.Commons
import qs.Ui
import "Model.js" as Model

Item {
  id: root

  property string omarchyPath: Quickshell.env("OMARCHY_PATH")
  property var shell: null
  property var manifest: null
  property bool opened: false
  property string message: "Es momento de moverte un poco."
  property string tip: "Ponte de pie y estira la espalda."
  property int durationSeconds: 30
  property int remainingSeconds: 30
  property double deadlineMs: 0
  property int selectedButton: 0

  readonly property color foreground: Color.foreground
  readonly property color cardBackground: Color.background
  readonly property color scrim: Qt.rgba(Color.background.r, Color.background.g, Color.background.b, 0.85)

  function open(payloadJson) {
    var payload = {}
    try { payload = JSON.parse(payloadJson || "{}") || {} } catch (e) { payload = {} }
    message = payload.message === undefined ? "Es momento de moverte un poco." : String(payload.message)
    tip = payload.tip === undefined ? "Ponte de pie y estira la espalda." : String(payload.tip)
    durationSeconds = Model.clampSeconds(payload.seconds)
    remainingSeconds = durationSeconds
    deadlineMs = Date.now() + durationSeconds * 1000
    selectedButton = 0
    opened = true
    countdown.restart()
  }

  function close() {
    opened = false
    countdown.stop()
  }

  function dismissWith(choice) {
    if (!opened) return
    opened = false
    countdown.stop()
    reportProcess.command = ["nexus", "pausa", choice]
    reportProcess.running = true
    if (shell && typeof shell.hide === "function")
      shell.hide((manifest && manifest.id) || "champip.nexus-pausa")
  }

  function updateCountdown() {
    var remainingMs = Math.max(0, deadlineMs - Date.now())
    remainingSeconds = Math.ceil(remainingMs / 1000)
    if (remainingMs <= 0) dismissWith("hecho")
  }

  function moveSelection(delta) {
    selectedButton = (selectedButton + delta + 2) % 2
  }

  function activateSelection() {
    dismissWith(selectedButton === 0 ? "posponer" : "saltar")
  }

  // El contenedor del host acepta elementos Item; aquí viven procesos y temporizadores.
  Item {
    visible: false

    Process { id: reportProcess }

    Timer {
      id: countdown
      interval: 100
      repeat: true
      onTriggered: root.updateCountdown()
    }
  }

  Variants {
    model: root.opened ? Quickshell.screens : []

    PanelWindow {
      id: overlayWindow
      required property var modelData
      screen: modelData
      visible: root.opened
      anchors { top: true; bottom: true; left: true; right: true }
      color: "transparent"
      WlrLayershell.namespace: "champip-nexus-pausa"
      WlrLayershell.layer: WlrLayer.Overlay
      WlrLayershell.keyboardFocus: WlrKeyboardFocus.Exclusive
      exclusionMode: ExclusionMode.Ignore

      Rectangle {
        anchors.fill: parent
        color: root.scrim

        MouseArea {
          anchors.fill: parent
          onClicked: {}
        }
      }

      Rectangle {
        id: card
        width: Math.min(Style.space(620), overlayWindow.width - Style.space(40))
        height: content.implicitHeight + Style.space(64)
        radius: Style.cornerRadius
        color: root.cardBackground
        anchors.centerIn: parent

        MouseArea { anchors.fill: parent; onClicked: {} }

        Item {
          id: keyCatcher
          anchors.fill: parent
          focus: true
          // Cada pantalla crea su propia capa al abrirse; el foco se pide aquí porque la raíz no ve este id.
          Component.onCompleted: Qt.callLater(function() { keyCatcher.forceActiveFocus() })

          Keys.priority: Keys.BeforeItem
          Keys.onPressed: function(event) {
            if (event.key === Qt.Key_Escape) {
              event.accepted = true
            } else if (event.key === Qt.Key_Tab || event.key === Qt.Key_Right || event.key === Qt.Key_Left) {
              root.moveSelection(event.key === Qt.Key_Left ? -1 : 1)
              event.accepted = true
            } else if (event.key === Qt.Key_Return || event.key === Qt.Key_Enter) {
              root.activateSelection()
              event.accepted = true
            }
          }

          ColumnLayout {
            id: content
            anchors.centerIn: parent
            width: Math.min(parent.width - Style.space(64), Style.space(540))
            spacing: Style.space(16)

            Text {
              text: "🚶 Hora de moverte"
              color: root.foreground
              font.family: Style.font.family
              font.pixelSize: Style.font.display
              font.bold: true
              horizontalAlignment: Text.AlignHCenter
              Layout.fillWidth: true
              wrapMode: Text.Wrap
            }

            Text {
              text: root.message
              color: root.foreground
              font.family: Style.font.family
              font.pixelSize: Style.font.title
              horizontalAlignment: Text.AlignHCenter
              Layout.fillWidth: true
              wrapMode: Text.Wrap
            }

            Text {
              text: root.tip
              color: root.foreground
              opacity: 0.78
              font.family: Style.font.family
              font.pixelSize: Style.font.body
              horizontalAlignment: Text.AlignHCenter
              Layout.fillWidth: true
              wrapMode: Text.Wrap
            }

            Text {
              text: Model.formatCountdown(root.remainingSeconds)
              color: root.foreground
              font.family: Style.font.family
              font.pixelSize: Style.font.displayLarge
              font.bold: true
              horizontalAlignment: Text.AlignHCenter
              Layout.fillWidth: true
            }

            Rectangle {
              Layout.fillWidth: true
              Layout.preferredHeight: Style.space(4)
              radius: height / 2
              color: Qt.rgba(root.foreground.r, root.foreground.g, root.foreground.b, 0.2)

              Rectangle {
                width: parent.width * (root.remainingSeconds / Math.max(1, root.durationSeconds))
                height: parent.height
                radius: parent.radius
                color: root.foreground
              }
            }

            RowLayout {
              Layout.alignment: Qt.AlignHCenter
              spacing: Style.space(12)

              Button {
                text: "Posponer 10 min"
                bordered: root.selectedButton === 0
                foreground: root.foreground
                fontFamily: Style.font.family
                fontSize: Style.font.body
                onClicked: root.dismissWith("posponer")
              }

              Button {
                text: "Saltar"
                bordered: root.selectedButton === 1
                foreground: root.foreground
                fontFamily: Style.font.family
                fontSize: Style.font.body
                onClicked: root.dismissWith("saltar")
              }
            }
          }
        }
      }
    }
  }
}
