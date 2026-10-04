import QtQuick
import QtQuick.Controls
import Quickshell
import Quickshell.Io
import qs.Ui
import qs.Commons
import "Model.js" as Model

KeyboardPanel {
  id: root
  required property QtObject client
  readonly property color textColor: bar ? bar.foreground : Color.foreground
  readonly property string fontName: bar ? bar.fontFamily : Style.font.family
  property bool dropdownOpen: false
  property string selectedProject: ""
  property bool defaultProjectChosen: false
  property int focusIndex: 0
  property double nowMs: Date.now()

  focusTarget: keyCatcher
  contentWidth: fittedContentWidth(Style.space(390))
  contentHeight: fittedContentHeight(contentColumn.implicitHeight, Style.space(680))

  function projectOptions() {
    var matches = Model.filterProjects(client.projects, projectSearch.text)
    var result = []
    if (!projectSearch.text.trim()) result.push({ name: "", label: "Sin proyecto", kind: "none" })
    for (var i = 0; i < matches.length; i++)
      result.push({ name: String(matches[i].name), label: String(matches[i].name), kind: "project" })
    if (Model.canCreateProject(client.projects, projectSearch.text)) {
      var name = projectSearch.text.trim()
      var exact = false
      for (var j = 0; j < matches.length; j++)
        if (matches[j].name.toLocaleLowerCase() === name.toLocaleLowerCase()) exact = true
      if (!exact) result.push({ name: name, label: "Crear «" + name + "»", kind: "create" })
    }
    return result
  }

  function selectProject(name) {
    selectedProject = name
    defaultProjectChosen = true
    dropdownOpen = false
    projectSearch.text = ""
    focusIndex = 1
    projectButton.forceActiveFocus()
  }

  function moveFocus(delta) {
    focusIndex = Math.max(0, Math.min(3 + client.running.length, focusIndex + delta))
    if (focusIndex === 0) titleField.forceActiveFocus()
    else if (focusIndex === 1) projectButton.forceActiveFocus()
    else if (focusIndex === 2) descriptionField.forceActiveFocus()
    else if (focusIndex === 3) startButton.forceActiveFocus()
    else if (focusIndex >= 4) {
      var timerIndex = focusIndex - 4
      var timerItem = runningRepeater.itemAt(timerIndex)
      if (timerItem && timerItem.stopControl) timerItem.stopControl.forceActiveFocus()
    }
  }

  function activateFocus() {
    if (focusIndex === 0 || focusIndex === 2 || focusIndex === 3) startTimer()
    else if (focusIndex === 1) {
      dropdownOpen = !dropdownOpen
      if (dropdownOpen) Qt.callLater(function() { projectSearch.forceActiveFocus() })
    } else {
      var timerIndex = focusIndex - 4
      var timerItem = runningRepeater.itemAt(timerIndex)
      if (timerItem) client.stop(timerItem.entry.id)
    }
  }

  function startTimer() {
    var title = titleField.text.trim()
    if (!title) { titleField.forceActiveFocus(); return }
    client.start(title, selectedProject, descriptionField.text.trim())
    titleField.text = ""
    descriptionField.text = ""
    focusIndex = 0
    titleField.forceActiveFocus()
  }

  function openDetail() {
    detailProc.running = true
    owner.close()
  }

  function formatToday() {
    return "Hoy " + Model.formatHMS(Model.liveTodaySeconds(client.todaySeconds, client.running.length, client.sampledAtMs, nowMs))
  }

  onOpenChanged: {
    if (open) {
      focusIndex = 0
      client.loadProjects("")
      Qt.callLater(function() { titleField.forceActiveFocus() })
    } else {
      dropdownOpen = false
    }
  }

  // KeyboardPanel hosts Items only, so non-visual helpers live in an invisible Item.
  Item {
    visible: false

    Process {
      id: detailProc
      command: ["omarchy-launch-or-focus-tui", "nexus"]
    }

    Timer {
      interval: 1000
      running: root.open
      repeat: true
      triggeredOnStart: true
      onTriggered: root.nowMs = Date.now()
    }

    Connections {
      target: client
      function onProjectsChanged() {
        if (!root.defaultProjectChosen && root.client.projects.length > 0) {
          root.selectedProject = String(root.client.projects[0].name || "")
          root.defaultProjectChosen = true
        }
      }
    }
  }

  PanelKeyCatcher {
    id: keyCatcher
    anchors.fill: parent
    blocked: root.dropdownOpen
    onMoveRequested: function(dx, dy) { if (dy) root.moveFocus(dy) }
    onActivateRequested: root.activateFocus()
    onCloseRequested: {
      if (root.dropdownOpen) {
        root.dropdownOpen = false
        projectSearch.text = ""
        projectButton.forceActiveFocus()
      } else root.owner.close()
    }

    Column {
      id: contentColumn
      width: parent.width
      spacing: Style.space(10)

      Row {
        width: parent.width
        spacing: Style.space(8)
        Text {
          text: "Nexus"
          color: root.textColor
          font.family: root.fontName
          font.pixelSize: Style.font.title
          font.bold: true
        }
        Item { width: 1; height: 1 }
        Text {
          text: root.formatToday()
          color: Qt.darker(root.textColor, 1.35)
          font.family: root.fontName
          font.pixelSize: Style.font.body
          anchors.verticalCenter: parent.verticalCenter
        }
      }

      TextField {
        id: titleField
        width: parent.width
        placeholderText: "¿En qué trabajas?"
        foreground: root.textColor
        activeFocusOnTab: true
        onActiveFocusChanged: if (activeFocus) root.focusIndex = 0
        Keys.onReturnPressed: root.startTimer()
        Keys.onEnterPressed: root.startTimer()
      }

      Column {
        width: parent.width
        spacing: Style.space(4)
        Button {
          id: projectButton
          width: parent.width
          text: root.selectedProject || "Sin proyecto"
          leftAlign: true
          bordered: true
          focusable: true
          hasCursor: root.focusIndex === 1
          foreground: root.textColor
          onClicked: {
            root.focusIndex = 1
            root.dropdownOpen = !root.dropdownOpen
            if (root.dropdownOpen) Qt.callLater(function() { projectSearch.forceActiveFocus() })
          }
          onHovered: function(hovered) { if (hovered) root.focusIndex = 1 }
        }
        Column {
          visible: root.dropdownOpen
          width: parent.width
          spacing: Style.space(4)
          TextField {
            id: projectSearch
            width: parent.width
            placeholderText: "Buscar proyecto"
            foreground: root.textColor
            onTextChanged: client.loadProjects(text.trim())
            Keys.onPressed: function(event) {
              if (event.key === Qt.Key_Escape) { root.dropdownOpen = false; projectSearch.text = ""; projectButton.forceActiveFocus(); event.accepted = true }
              else if (event.key === Qt.Key_Down) { var opts = root.projectOptions(); if (opts.length) optionRepeater.itemAt(0).forceActiveFocus(); event.accepted = true }
            }
          }
          Repeater {
            id: optionRepeater
            model: root.projectOptions()
            delegate: Button {
              required property var modelData
              required property int index
              width: parent.width
              text: modelData.label
              leftAlign: true
              bordered: true
              focusable: true
              foreground: root.textColor
              onClicked: root.selectProject(modelData.name)
              Keys.onPressed: function(event) {
                if (event.key === Qt.Key_Down && index < optionRepeater.count - 1) {
                  optionRepeater.itemAt(index + 1).forceActiveFocus(); event.accepted = true
                } else if (event.key === Qt.Key_Up) {
                  if (index > 0) optionRepeater.itemAt(index - 1).forceActiveFocus()
                  else projectSearch.forceActiveFocus()
                  event.accepted = true
                } else if (event.key === Qt.Key_Escape) {
                  root.dropdownOpen = false; projectSearch.text = ""; projectButton.forceActiveFocus(); event.accepted = true
                }
              }
            }
          }
          Text {
            visible: root.projectOptions().length === 0
            text: "Sin resultados"
            color: Qt.darker(root.textColor, 1.4)
            font.family: root.fontName
            font.pixelSize: Style.font.caption
          }
        }
      }

      TextField {
        id: descriptionField
        width: parent.width
        placeholderText: "Descripción (opcional)"
        foreground: root.textColor
        activeFocusOnTab: true
        onActiveFocusChanged: if (activeFocus) root.focusIndex = 2
        Keys.onReturnPressed: root.startTimer()
        Keys.onEnterPressed: root.startTimer()
      }

      Button {
        id: startButton
        width: parent.width
        text: "Iniciar"
        bordered: true
        focusable: true
        hasCursor: root.focusIndex === 3
        foreground: root.textColor
        onClicked: root.startTimer()
        onHovered: function(hovered) { if (hovered) root.focusIndex = 3 }
      }

      Rectangle { width: parent.width; height: 1; color: Qt.rgba(root.textColor.r, root.textColor.g, root.textColor.b, 0.15) }
      Text {
        text: "En curso"
        color: root.textColor
        font.family: root.fontName
        font.pixelSize: Style.font.body
        font.bold: true
      }

      Text {
        visible: client.running.length === 0
        width: parent.width
        text: "Sin temporizadores activos"
        color: Qt.darker(root.textColor, 1.45)
        font.family: root.fontName
        font.pixelSize: Style.font.bodySmall
      }

      Repeater {
        id: runningRepeater
        model: client.running
        delegate: Row {
          id: timerRow
          required property var modelData
          required property int index
          property var entry: modelData
          property alias stopControl: stopButton
          width: contentColumn.width
          spacing: Style.space(6)
          readonly property real stopWidth: Style.space(90)
          Column {
            width: parent.width - timerRow.stopWidth - parent.spacing
            Text {
              text: timerRow.entry.title
              color: root.textColor
              font.family: root.fontName
              font.pixelSize: Style.font.body
              elide: Text.ElideRight
              width: parent.width
            }
            Text {
              text: (timerRow.entry.project || "Sin proyecto") + " · " + Model.formatClock(timerRow.entry.started_at, root.nowMs)
              color: Qt.darker(root.textColor, 1.4)
              font.family: root.fontName
              font.pixelSize: Style.font.caption
              elide: Text.ElideRight
              width: parent.width
            }
          }
          Button {
            id: stopButton
            text: "■ Detener"
            bordered: true
            focusable: true
            foreground: root.textColor
            onClicked: client.stop(timerRow.entry.id)
            onHovered: function(hovered) { if (hovered) root.focusIndex = 4 + timerRow.index }
          }
        }
      }

      Text {
        id: errorText
        visible: text !== ""
        width: parent.width
        text: client.errorMessage
        color: Color.urgent
        wrapMode: Text.WordWrap
        font.family: root.fontName
        font.pixelSize: Style.font.caption
      }

      Button {
        width: parent.width
        text: "Abrir detalle"
        bordered: true
        foreground: root.textColor
        onClicked: root.openDetail()
      }
    }
  }
}
