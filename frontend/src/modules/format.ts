
export function fmtNum(v: unknown, digits = 2) {
  const n = Number(v)
  if (Number.isNaN(n)) return '-'
  return n.toFixed(digits)
}

export function fmtPct(v: unknown) {
  const n = Number(v)
  if (Number.isNaN(n)) return '-'
  return `${n.toFixed(2)}%`
}

export function fmtTime(value: unknown) {
  if (!value) return '-'
  const input =
    value instanceof Date || typeof value === 'string' || typeof value === 'number'
      ? value
      : String(value)
  const d = new Date(input)
  if (Number.isNaN(d.getTime())) return '-'
  return d.toLocaleString()
}
