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
  property bool pendingRefresh: false
  property var actionQueue: []
  signal error(string message)

  function refresh() {
    if (statusProc.running) {
      pendingRefresh = true
      return
    }
    statusProc.running = true
  }

  // Inicia un nuevo temporizador construyendo el argv mediante Model.startArgv.
  function start(title, project, description) {
    var args = Model.startArgv(title, project, description)
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
    actionQueue = Model.enqueueAction(actionQueue, argv, action)
    startNextAction()
  }

  function startNextAction() {
    if (actionProc.running || actionQueue.length === 0) return
    var next = Model.dequeueAction(actionQueue)
    actionQueue = next.queue
    actionKind = next.item.action
    actionProc.command = next.item.argv
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
      if (root.pendingRefresh) {
        root.pendingRefresh = false
        root.refresh()
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
      root.startNextAction()
    }
  }
}
