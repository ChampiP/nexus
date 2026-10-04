function formatDuration(seconds) {
  var value = Math.max(0, Number(seconds) || 0)
  var hours = Math.floor(value / 3600)
  var minutes = Math.floor((value % 3600) / 60)
  return hours + "h " + (minutes < 10 ? "0" : "") + minutes + "m"
}

function formatClock(startedAt, now) {
  var elapsed = Math.max(0, Math.floor((Number(now) - Number(startedAt)) / 1000))
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
      var startedAt = entry && entry.started_at ? entry.started_at : now
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
    return aName < bName ? -1 : (aName > bName ? 1 : 0)
  })
  if (!needle) return list
  return list.filter(function(project) {
    return String(project.name || "").toLocaleLowerCase().indexOf(needle) >= 0
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

if (typeof module !== "undefined") {
  module.exports = { formatDuration: formatDuration, formatClock: formatClock,
    formatHMS: formatHMS, liveTodaySeconds: liveTodaySeconds, barLabel: barLabel,
    barTooltip: barTooltip, tooltipText: barTooltip, applyStatus: applyStatus,
    filterProjects: filterProjects, canCreateProject: canCreateProject }
}
