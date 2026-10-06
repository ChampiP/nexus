function formatDuration(seconds) {
  var value = Math.max(0, Number(seconds) || 0)
  var hours = Math.floor(value / 3600)
  var minutes = Math.floor((value % 3600) / 60)
  return hours + "h " + (minutes < 10 ? "0" : "") + minutes + "m"
}

// startedAt viene de la CLI en segundos Unix; now está en milisegundos (Date.now()).
function formatClock(startedAt, now) {
  var elapsed = Math.max(0, Math.floor(Number(now) / 1000 - Number(startedAt)))
  var hours = Math.floor(elapsed / 3600)
  var minutes = Math.floor((elapsed % 3600) / 60)
  var seconds = elapsed % 60
  return pad(hours) + ":" + pad(minutes) + ":" + pad(seconds)
}

function formatHMS(seconds) {
  var value = Math.max(0, Math.floor(Number(seconds) || 0))
  return Math.floor(value / 3600) + ":" + pad(Math.floor((value % 3600) / 60)) + ":" + pad(value % 60)
}

// Today's total keeps growing between polls: one second per running timer per elapsed second.
function liveTodaySeconds(todaySeconds, runningCount, sampledAtMs, nowMs) {
  var elapsed = Math.max(0, Math.floor((Number(nowMs) - Number(sampledAtMs)) / 1000))
  return (Number(todaySeconds) || 0) + (Number(runningCount) || 0) * elapsed
}

// Etiqueta de la barra: reloj en vivo con el conteo de temporizadores activos; "0:00" en reposo.
function barLabel(todaySeconds, runningCount) {
  var count = Number(runningCount) || 0
  return count > 0
    ? "󱎫 " + count + " · " + formatHMS(todaySeconds)
    : "󱎫 0:00"
}

// Texto de ayuda: muestra el total del día cuando no hay tareas activas, o la lista en ejecución; añade sufijo si el estado no está actualizado.
function barTooltip(todaySeconds, running, nowMs, stale) {
  var isStale = stale
  var now = nowMs
  if (typeof nowMs === "boolean" && stale === undefined) {
    isStale = nowMs
    now = Date.now()
  } else if (now === undefined) {
    now = Date.now()
  }
  var list = Array.isArray(running) ? running : []
  var text = ""
  if (list.length === 0) {
    text = "Hoy: " + formatDuration(todaySeconds) + " · sin temporizadores en curso"
  } else {
    text = list.map(function(entry) {
      var title = String((entry && entry.title) || "")
      var project = entry && entry.project ? " (" + entry.project + ")" : ""
      var startedAt = entry && entry.started_at ? entry.started_at : now / 1000
      return title + project + "  " + formatClock(startedAt, now)
    }).join("\n")
  }
  if (isStale) {
    text += " (sin actualizar)"
  }
  return text
}

// Conserva el estado anterior si la respuesta contiene un error no vacío.
function applyStatus(prev, status) {
  var prior = prev || {}
  var priorRunning = Array.isArray(prior.running) ? prior.running : []
  var priorToday = Number(prior.todaySeconds !== undefined ? prior.todaySeconds : prior.today_seconds) || 0
  var payload = status || {}
  var err = typeof payload.error === "string" ? payload.error.trim() : ""

  if (err.length > 0) {
    return {
      running: priorRunning,
      todaySeconds: priorToday,
      stale: true,
      error: err
    }
  }

  var newRunning = Array.isArray(payload.running) ? payload.running : []
  var newToday = Number(payload.today_seconds !== undefined ? payload.today_seconds : payload.todaySeconds) || 0
  return {
    running: newRunning,
    todaySeconds: newToday,
    stale: false,
    error: ""
  }
}

function pad(value) { return value < 10 ? "0" + value : String(value) }

function filterProjects(projects, query) {
  var list = Array.isArray(projects) ? projects.slice() : []
  var needle = String(query || "").trim().toLocaleLowerCase()
  list.sort(function(a, b) {
    var recent = Number(b.last_used || 0) - Number(a.last_used || 0)
    if (recent) return recent
    var aName = String(a.name || "").toLowerCase()
    var bName = String(b.name || "").toLowerCase()
    if (aName !== bName) return aName < bName ? -1 : 1
    var aClient = String(a.client || "").toLowerCase()
    var bClient = String(b.client || "").toLowerCase()
    return aClient < bClient ? -1 : (aClient > bClient ? 1 : 0)
  })
  if (!needle) return list
  return list.filter(function(project) {
    var nameMatch = String((project && project.name) || "").toLocaleLowerCase().indexOf(needle) >= 0
    var clientMatch = String((project && project.client) || "").toLocaleLowerCase().indexOf(needle) >= 0
    return nameMatch || clientMatch
  })
}

function canCreateProject(projects, query) {
  var name = String(query || "").trim()
  if (!name) return false
  var list = Array.isArray(projects) ? projects : []
  for (var i = 0; i < list.length; i++) {
    if (String(list[i].name || "").trim().toLocaleLowerCase() === name.toLocaleLowerCase()) return false
  }
  return true
}

// Etiqueta legible de un proyecto: si tiene cliente asociado muestra "proyecto · cliente", de lo contrario solo el nombre.
function projectLabel(project) {
  if (!project) return ""
  var name = String(project.name || "").trim()
  var client = String(project.client || "").trim()
  return client ? name + " · " + client : name
}

// Construye las opciones para el selector de proyectos, incluyendo etiqueta con cliente, id y opción de creación.
function projectOptions(projects, query) {
  var text = String(query || "").trim()
  var matches = filterProjects(projects, text)
  var result = []
  if (!text) {
    result.push({ id: null, name: "", client: "", label: "Sin proyecto", kind: "none" })
  }
  for (var i = 0; i < matches.length; i++) {
    var p = matches[i]
    result.push({
      id: p.id !== undefined && p.id !== null ? p.id : null,
      name: String(p.name || ""),
      client: String(p.client || ""),
      label: projectLabel(p),
      kind: "project"
    })
  }
  if (canCreateProject(projects, text)) {
    var exact = false
    for (var j = 0; j < matches.length; j++) {
      if (String(matches[j].name || "").toLocaleLowerCase() === text.toLocaleLowerCase()) {
        exact = true
        break
      }
    }
    if (!exact) {
      result.push({ id: null, name: text, client: "", label: "Crear «" + text + "»", kind: "create" })
    }
  }
  return result
}

// Genera los argumentos de línea de comandos para el proyecto seleccionado.
// Si tiene id asignado utiliza "#<id>"; si es nuevo o sin id, utiliza el nombre simple.
function projectArgv(project) {
  if (!project) return []
  if (typeof project === "number") {
    return ["-p", "#" + project]
  }
  if (typeof project === "string") {
    var trimmed = project.trim()
    if (!trimmed) return []
    return ["-p", trimmed]
  }
  // id 0 = proyecto usado sin fila en el catálogo: se envía por nombre.
  if (project.id !== undefined && project.id !== null && String(project.id).trim() !== "" && String(project.id).trim() !== "0") {
    var idStr = String(project.id).trim()
    return ["-p", idStr.indexOf("#") === 0 ? idStr : "#" + idStr]
  }
  var name = String(project.name || "").trim()
  if (!name) return []
  return ["-p", name]
}

// Agrega una acción al final de la cola, conservando la cola y los argv originales.
function enqueueAction(queue, argv, action) {
  var result = Array.isArray(queue) ? queue.slice() : []
  result.push({ argv: Array.isArray(argv) ? argv.slice() : [], action: String(action || "") })
  return result
}

// Extrae la siguiente acción de la cola sin modificarla.
function dequeueAction(queue) {
  var list = Array.isArray(queue) ? queue : []
  return {
    item: list.length ? list[0] : null,
    queue: list.slice(1)
  }
}

// Construye los argumentos completos para "nexus start".
function startArgv(title, project, description) {
  var args = ["nexus", "start", String(title || "")]
  var p = projectArgv(project)
  for (var i = 0; i < p.length; i++) args.push(p[i])
  if (description) args.push("-d", String(description))
  args.push("--json")
  return args
}

if (typeof module !== "undefined") {
  module.exports = {
    formatDuration: formatDuration,
    formatClock: formatClock,
    formatHMS: formatHMS,
    liveTodaySeconds: liveTodaySeconds,
    barLabel: barLabel,
    barTooltip: barTooltip,
    tooltipText: barTooltip,
    applyStatus: applyStatus,
    filterProjects: filterProjects,
    canCreateProject: canCreateProject,
    projectLabel: projectLabel,
    projectOptions: projectOptions,
    buildProjectOptions: projectOptions,
    projectArgv: projectArgv,
    buildProjectArgs: projectArgv,
    enqueueAction: enqueueAction,
    dequeueAction: dequeueAction,
    startArgv: startArgv
  }
}

