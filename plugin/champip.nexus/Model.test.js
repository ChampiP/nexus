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

assert.strictEqual(Model.barLabel(240, 0), "󱎫 0h 04m")
assert.strictEqual(Model.barLabel(252, 3), "󱎫 3 · 0:04:12")
console.log("Model.js ok")
