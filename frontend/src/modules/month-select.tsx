// @ts-nocheck
import { Select } from '@/components/ui/dashboard-primitives'

export function MonthSelect({ value, onChange, min = '2020-01', max = '2026-12' }) {
  const [minY, minM] = String(min).split('-').map((x) => Number(x))
  const [maxY, maxM] = String(max).split('-').map((x) => Number(x))
  const [curYRaw, curMRaw] = String(value || '').split('-')
  let curY = Number(curYRaw || minY)
  let curM = Number(curMRaw || minM)
  if (Number.isNaN(curY) || curY < minY) curY = minY
  if (Number.isNaN(curM) || curM < 1) curM = 1

  const years = []
  for (let y = minY; y <= maxY; y += 1) years.push(y)
  const startM = curY === minY ? minM : 1
  const endM = curY === maxY ? maxM : 12
  if (curM < startM) curM = startM
  if (curM > endM) curM = endM

  const apply = (y, m) => onChange(`${y}-${String(m).padStart(2, '0')}`)

  return (
    <div className="month-picker">
      <Select
        size="small"
        className="month-select year"
        value={String(curY)}
        options={years.map((y) => ({ value: String(y), label: `${y}年` }))}
        onChange={(v) => apply(Number(v), curM)}
      />
      <Select
        size="small"
        className="month-select month"
        value={String(curM)}
        options={Array.from({ length: endM - startM + 1 }, (_, i) => startM + i).map((m) => ({
          value: String(m),
          label: `${m}月`,
        }))}
        onChange={(v) => apply(curY, Number(v))}
      />
    </div>
  )
}
