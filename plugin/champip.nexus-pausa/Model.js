function clampSeconds(value) {
  if (typeof value !== "number") return 30
  var seconds = value
  if (!isFinite(seconds)) return 30
  return Math.max(5, Math.min(300, Math.floor(seconds)))
}

function formatCountdown(seconds) {
  var value = Math.max(0, Math.floor(Number(seconds) || 0))
  var minutes = Math.floor(value / 60)
  var remainder = value % 60
  return minutes + ":" + (remainder < 10 ? "0" : "") + remainder
}

if (typeof module !== "undefined") {
  module.exports = { clampSeconds: clampSeconds, formatCountdown: formatCountdown }
}
