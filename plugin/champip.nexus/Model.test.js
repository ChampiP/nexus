// Run with: node plugin/champip.nexus/Model.test.js
const assert = require("assert")
const Model = require("./Model.js")

assert.strictEqual(Model.formatHMS(22), "0:00:22")
assert.strictEqual(Model.formatHMS(3725), "1:02:05")
assert.strictEqual(Model.formatHMS(-3), "0:00:00")

// Today's total grows by one second per running timer, per elapsed second.
assert.strictEqual(Model.liveTodaySeconds(100, 0, 1000, 6000), 100)
assert.strictEqual(Model.liveTodaySeconds(100, 1, 1000, 6000), 105)
assert.strictEqual(Model.liveTodaySeconds(100, 3, 1000, 6000), 115)
assert.strictEqual(Model.liveTodaySeconds(100, 2, 6000, 1000), 100) // clock skew never goes negative

// Etiqueta inactiva muestra 0:00; etiqueta activa se mantiene sin cambios.
assert.strictEqual(Model.barLabel(240, 0), "󱎫 0:00")
assert.strictEqual(Model.barLabel(252, 3), "󱎫 3 · 0:04:12")

// Texto de ayuda (tooltip): total de hoy en reposo y estado sin actualizar si hay error.
assert.strictEqual(
  Model.tooltipText(2160, []),
  "Hoy: 0h 36m · sin temporizadores en curso"
)
assert.strictEqual(
  Model.tooltipText(2160, [], Date.now(), true),
  "Hoy: 0h 36m · sin temporizadores en curso (sin actualizar)"
)
const timers = [
  { title: "Arreglar bug", project: "nexus", started_at: 1000 },
  { title: "Revisión", project: "", started_at: 2000 }
]
assert.strictEqual(
  Model.tooltipText(2160, timers, 61000, false),
  "Arreglar bug (nexus)  00:01:00\nRevisión  00:00:59"
)
assert.strictEqual(
  Model.tooltipText(2160, timers, 61000, true),
  "Arreglar bug (nexus)  00:01:00\nRevisión  00:00:59 (sin actualizar)"
)

// applyStatus conserva los valores previos ante un error no vacío.
const prev = { running: [{ id: 1, title: "A" }], todaySeconds: 120, stale: false }
const nextGood = Model.applyStatus(prev, {
  running: [{ id: 2, title: "B" }],
  today_seconds: 150
})
assert.deepStrictEqual(nextGood.running, [{ id: 2, title: "B" }])
assert.strictEqual(nextGood.todaySeconds, 150)
assert.strictEqual(nextGood.stale, false)

const nextErr = Model.applyStatus(prev, {
  running: [],
  today_seconds: 0,
  count: 0,
  error: "no se pudo leer la base de datos"
})
assert.deepStrictEqual(nextErr.running, prev.running)
assert.strictEqual(nextErr.todaySeconds, 120)
assert.strictEqual(nextErr.stale, true)
assert.strictEqual(nextErr.error, "no se pudo leer la base de datos")

// Dos proyectos con el mismo nombre y clientes distintos se listan con etiqueta de cliente y no se colapsan.
const duplicateProjects = [
  { id: 7, name: "Diseno de web", client: "Acme", last_used: 100 },
  { id: 8, name: "Diseno de web", client: "Beta", last_used: 200 }
]

assert.strictEqual(Model.projectLabel(duplicateProjects[0]), "Diseno de web · Acme")
assert.strictEqual(Model.projectLabel(duplicateProjects[1]), "Diseno de web · Beta")
assert.strictEqual(Model.projectLabel({ id: 9, name: "Diseno de web", client: "" }), "Diseno de web")

const options = Model.projectOptions(duplicateProjects, "")
assert.strictEqual(options.length, 3) // "Sin proyecto" + 2 proyectos
const opt7 = options.find(function(o) { return o.id === 7 })
const opt8 = options.find(function(o) { return o.id === 8 })
assert.ok(opt7, "debe incluir la opción con id 7")
assert.ok(opt8, "debe incluir la opción con id 8")
assert.strictEqual(opt7.label, "Diseno de web · Acme")
assert.strictEqual(opt8.label, "Diseno de web · Beta")

// Filtrado por cliente y por nombre sin colapsar duplicados.
const filteredBeta = Model.filterProjects(duplicateProjects, "Beta")
assert.strictEqual(filteredBeta.length, 1)
assert.strictEqual(filteredBeta[0].id, 8)
assert.strictEqual(filteredBeta[0].client, "Beta")

const filteredAcme = Model.filterProjects(duplicateProjects, "Acme")
assert.strictEqual(filteredAcme.length, 1)
assert.strictEqual(filteredAcme[0].id, 7)

const filteredBoth = Model.filterProjects(duplicateProjects, "Diseno")
assert.strictEqual(filteredBoth.length, 2)

// Constructor de argv: devuelve ["-p", "#7"] para proyecto existente y el nombre simple para uno nuevo.
assert.deepStrictEqual(Model.projectArgv({ id: 7, name: "Diseno de web" }), ["-p", "#7"])
assert.deepStrictEqual(Model.projectArgv({ id: "#7", name: "Diseno de web" }), ["-p", "#7"])
assert.deepStrictEqual(Model.projectArgv({ name: "Diseno de web" }), ["-p", "Diseno de web"])
assert.deepStrictEqual(Model.projectArgv("Diseno de web"), ["-p", "Diseno de web"])
assert.deepStrictEqual(Model.projectArgv(null), [])
assert.deepStrictEqual(Model.projectArgv(""), [])
assert.deepStrictEqual(Model.projectArgv({ name: "" }), [])

console.log("Model.js ok")

