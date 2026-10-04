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
    filterProjects: filterProjects, canCreateProject: canCreateProject }
}
