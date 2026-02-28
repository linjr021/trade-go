// @ts-nocheck

export function fmtNum(v, digits = 2) {
  const n = Number(v)
  if (Number.isNaN(n)) return '-'
  return n.toFixed(digits)
}

export function fmtPct(v) {
  const n = Number(v)
  if (Number.isNaN(n)) return '-'
  return `${n.toFixed(2)}%`
}

export function fmtTime(value) {
  if (!value) return '-'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return '-'
  return d.toLocaleString()
}
