// @ts-nocheck
import { envFieldDefs, systemSettingDefaults } from '@/modules/constants'

const legacyEmptyLikeModels = new Set(['chat-model'])

export function clamp(value, min, max) {
  const n = Number(value)
  if (Number.isNaN(n)) return min
  return Math.min(max, Math.max(min, n))
}

export function toFixedNumber(value, digits = 2) {
  const n = Number(value)
  if (Number.isNaN(n)) return 0
  return Number(n.toFixed(digits))
}

export function normalizeDecimal(value, min = 0, max = 1000000) {
  return toFixedNumber(clamp(value, min, max), 2)
}

export function normalizeLeverage(value) {
  return Math.round(clamp(value, 1, 150))
}

export function normalizeTradeSettings(raw) {
  return {
    positionSizingMode: String(raw?.positionSizingMode || 'contracts'),
    highConfidenceAmount: normalizeDecimal(raw?.highConfidenceAmount, 0, 1000000),
    lowConfidenceAmount: normalizeDecimal(raw?.lowConfidenceAmount, 0, 1000000),
    highConfidenceMarginPct: normalizeDecimal(raw?.highConfidenceMarginPct, 0, 100),
    lowConfidenceMarginPct: normalizeDecimal(raw?.lowConfidenceMarginPct, 0, 100),
    leverage: normalizeLeverage(raw?.leverage),
  }
}

export function sleep(ms) {
  return new Promise((resolve) => {
    setTimeout(resolve, ms)
  })
}

export function loadPaperLocalRecords() {
  try {
    const raw = localStorage.getItem('paper-local-records')
    if (!raw) return []
    const arr = JSON.parse(raw)
    if (!Array.isArray(arr)) return []
    return arr.filter((r) => r && typeof r === 'object')
  } catch {
    return []
  }
}

export function calcPaperPnL(signal, lastPrice, currentPrice, size) {
  const qty = Number(size || 0)
  if (qty <= 0 || lastPrice <= 0 || currentPrice <= 0) return 0
  const side = String(signal || '').toUpperCase()
  if (side === 'BUY') return (currentPrice - lastPrice) * qty
  if (side === 'SELL') return (lastPrice - currentPrice) * qty
  return 0
}

export function resolveRequestError(e, fallback = '请求失败') {
  const apiError = String(e?.response?.data?.error || '').trim()
  if (apiError) return apiError
  const status = Number(e?.response?.status || 0)
  const contentType = String(e?.response?.headers?.['content-type'] || '').toLowerCase()
  const rawBody = typeof e?.response?.data === 'string' ? e.response.data.trim() : ''
  if (status === 500 && contentType.includes('text/plain') && rawBody === '') {
    return '后端服务不可用（可能未启动）。请先启动 Go 后端（默认 127.0.0.1:8080）'
  }
  const msg = String(e?.message || '').trim()
  if (!msg) return fallback
  if (/network error/i.test(msg)) return '网络连接失败，请确认后端服务已启动且 /api 代理可达'
  if (/timeout|ECONNABORTED/i.test(msg)) return '请求超时，请稍后重试'
  return msg
}

export function countHanChars(value) {
  const text = String(value || '')
  const hits = text.match(/[\u3400-\u4DBF\u4E00-\u9FFF\uF900-\uFAFF]/g)
  return hits ? hits.length : 0
}

export function makeUniqueNameWithIndex(rawName, existingNames) {
  const base = String(rawName || '').trim() || '未命名策略'
  if (!existingNames.has(base)) return base
  let idx = 2
  while (existingNames.has(`${base}-${idx}`)) idx += 1
  return `${base}-${idx}`
}

export function mergeSystemDefaults(raw) {
  const out = { ...(raw || {}) }
  const aiModel = String(out.AI_MODEL || '').trim().toLowerCase()
  if (legacyEmptyLikeModels.has(aiModel)) {
    out.AI_MODEL = ''
  }
  for (const f of envFieldDefs) {
    if ((out[f.key] || '').trim() === '' && systemSettingDefaults[f.key] !== undefined) {
      out[f.key] = systemSettingDefaults[f.key]
    }
  }
  return out
}

export function parseStrategies(raw = []) {
  if (!Array.isArray(raw)) return []
  return raw
    .map((x) => String(x || '').trim())
    .filter(Boolean)
}

export function joinFieldMessages(fieldMap) {
  if (!fieldMap || typeof fieldMap !== 'object') return ''
  const parts = Object.entries(fieldMap)
    .filter(([k, v]) => String(k || '').trim() !== '' && String(v || '').trim() !== '')
    .map(([k, v]) => `[${k}] ${v}`)
  return parts.join('；')
}

export function mapBacktestSummary(summary, fallback = {}) {
  const wins = Number(summary?.wins || 0)
  const losses = Number(summary?.losses || 0)
  const rawRatio = Number(summary?.ratio)
  const ratio = Number.isFinite(rawRatio) ? rawRatio : 0
  const ratioInfinite = Boolean(summary?.ratio_infinite) || (losses === 0 && wins > 0)
  return {
    historyId: Number(summary?.history_id || summary?.id || 0),
    createdAt: String(summary?.created_at || ''),
    strategy: summary?.strategy || fallback.btStrategy || '-',
    pair: summary?.pair || fallback.btPair,
    habit: summary?.habit || fallback.habit,
    start: summary?.start || fallback.btStart,
    end: summary?.end || fallback.btEnd,
    bars: Number(summary?.bars || 0),
    totalPnl: Number(summary?.total_pnl || 0),
    initialMargin: Number(summary?.initial_margin || 0),
    finalEquity: Number(summary?.final_equity || 0),
    returnPct: Number(summary?.return_pct || 0),
    leverage: Number(summary?.leverage || 0),
    positionSizingMode: String(summary?.position_sizing_mode || 'contracts'),
    highConfidenceAmount: Number(summary?.high_confidence_amount || 0),
    lowConfidenceAmount: Number(summary?.low_confidence_amount || 0),
    highConfidenceMarginPct: Number(summary?.high_confidence_margin_pct || 0) * 100,
    lowConfidenceMarginPct: Number(summary?.low_confidence_margin_pct || 0) * 100,
    wins,
    losses,
    ratio,
    ratioInfinite,
  }
}
