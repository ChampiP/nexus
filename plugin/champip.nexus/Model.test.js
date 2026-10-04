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

console.log("Model.js ok")

