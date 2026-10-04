import QtQuick
import Quickshell.Io
import "Model.js" as Model

// Non-visual Item: its default property hosts the Process children.
Item {
  id: root
  visible: false

  property var running: []
  property int todaySeconds: 0
  property var projects: []
  property string errorMessage: ""
  property bool stale: false
  property double sampledAtMs: Date.now()
  property string pendingProjectsQuery: ""
  property string activeProjectsQuery: ""
  signal error(string message)

  function refresh() {
    if (!statusProc.running) statusProc.running = true
  }

  function start(title, project, description) {
    var args = ["nexus", "start", String(title)]
    if (project) args.push("-p", String(project))
    if (description) args.push("-d", String(description))
    args.push("--json")
    runAction(args, "start")
  }

  function stop(id) { runAction(["nexus", "stop", String(id), "--json"], "stop") }
  function stopAll() { runAction(["nexus", "stop", "--all", "--json"], "stopAll") }

  function loadProjects(query) {
    var requested = String(query || "")
    pendingProjectsQuery = requested
    if (projectsProc.running) return
    startProjectsQuery()
  }

  function startProjectsQuery() {
    activeProjectsQuery = pendingProjectsQuery
    projectsProc.command = activeProjectsQuery
      ? ["nexus", "projects", "--json", "-q", activeProjectsQuery]
      : ["nexus", "projects", "--json"]
    projectsProc.running = true
  }

  function runAction(argv, action) {
    if (actionProc.running) return
    actionKind = action
    actionProc.command = argv
    actionProc.running = true
  }

  function reportFailure(message) {
    errorMessage = String(message || "No se pudo ejecutar Nexus")
    error(errorMessage)
  }

  property string actionKind: ""

  Process {
    id: statusProc
    command: ["nexus", "status", "--json"]
    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: {
        try {
          var status = JSON.parse(String(text || "{}"))
          var applied = Model.applyStatus({
            running: root.running,
            todaySeconds: root.todaySeconds,
            stale: root.stale
          }, status)
          root.running = applied.running
          root.todaySeconds = applied.todaySeconds
          root.stale = applied.stale
          if (applied.stale) {
            root.reportFailure(applied.error || "No se pudo leer el estado de Nexus")
          } else {
            root.errorMessage = ""
            root.sampledAtMs = Date.now()
          }
        } catch (e) {
          root.stale = true
          root.reportFailure("No se pudo leer el estado de Nexus")
        }
      }
    }
    onExited: function(code) {
      if (code !== 0) {
        root.stale = true
        root.reportFailure("No se pudo consultar el estado de Nexus")
      }
    }
  }

  Process {
    id: projectsProc
    command: ["nexus", "projects", "--json"]
    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: {
        try {
          var result = JSON.parse(String(text || "[]"))
          root.projects = Array.isArray(result) ? result : []
        } catch (e) { root.reportFailure("No se pudo leer la lista de proyectos") }
      }
    }
    onExited: function(code) {
      if (code !== 0) root.reportFailure("No se pudieron consultar los proyectos")
      if (root.pendingProjectsQuery !== root.activeProjectsQuery) root.startProjectsQuery()
    }
  }

  Process {
    id: actionProc
    command: ["nexus", "status", "--json"]
    stdout: StdioCollector { id: actionStdout; waitForEnd: true }
    stderr: StdioCollector { id: actionStderr; waitForEnd: true }
    onExited: function(code) {
      if (code !== 0) {
        root.reportFailure(String(actionStderr.text || actionStdout.text || "Nexus devolvió un error").trim())
      } else {
        try {
          var output = String(actionStdout.text || "").trim()
          var parsed = output ? JSON.parse(output) : {}
          if (parsed.error) root.reportFailure(parsed.error)
          else root.errorMessage = ""
        } catch (e) { root.reportFailure("No se pudo leer la respuesta de Nexus") }
      }
      root.refresh()
    }
  }
}
