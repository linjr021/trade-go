// @ts-nocheck
import { useEffect, useMemo, useRef, useState } from 'react'
import { fmtNum, fmtPct } from '@/modules/format'

const BINANCE_REST = 'https://api.binance.com/api/v3'
const BINANCE_WS = 'wss://stream.binance.com:9443/ws'
const OKX_REST = 'https://www.okx.com'
const OKX_WS = 'wss://ws.okx.com:8443/ws/v5/public'
const KLINE_INTERVAL = '1m'
const KLINE_LIMIT = 500
const KLINE_INTERVAL_OPTIONS = [
  '1m', '3m', '5m', '15m', '30m',
  '1h', '2h', '4h', '6h', '8h', '12h',
  '1d', '3d', '1w', '1M',
]
const QUICK_INTERVAL_MAX = 5
const QUICK_INTERVAL_DEFAULT = ['1m', '5m', '15m', '1h', '4h']
const QUICK_INTERVALS_STORAGE_KEY = 'binance-kline-quick-intervals'
const KLINE_HEIGHT_STORAGE_KEY = 'binance-kline-shell-height'
const KLINE_HEIGHT_DEFAULT = 840
const KLINE_HEIGHT_MIN = 620
const KLINE_HEIGHT_MAX = 1600

function clampValue(v, min, max) {
  return Math.max(min, Math.min(max, v))
}

function normalizeSymbol(symbol) {
  const s = String(symbol || '').trim().toUpperCase().replace(/[^A-Z0-9]/g, '')
  return s || 'BTCUSDT'
}

function normalizeExchange(exchange) {
  const e = String(exchange || '').trim().toLowerCase()
  return e === 'okx' ? 'okx' : 'binance'
}

function toOKXInstID(symbol) {
  const s = normalizeSymbol(symbol)
  if (s.endsWith('USDT')) {
    const base = s.slice(0, -4)
    return `${base}-USDT-SWAP`
  }
  if (s.includes('-')) return s
  return `${s}-SWAP`
}

function toOKXBar(interval) {
  const m = {
    '1m': '1m',
    '3m': '3m',
    '5m': '5m',
    '15m': '15m',
    '30m': '30m',
    '1h': '1H',
    '2h': '2H',
    '4h': '4H',
    '6h': '6H',
    '8h': '8H',
    '12h': '12H',
    '1d': '1D',
    '3d': '3D',
    '1w': '1W',
    '1M': '1M',
  }
  return m[String(interval || '1m')] || '1m'
}

function parseBinanceKline(row) {
  return {
    openTime: Number(row?.[0] || 0),
    open: Number(row?.[1] || 0),
    high: Number(row?.[2] || 0),
    low: Number(row?.[3] || 0),
    close: Number(row?.[4] || 0),
    volume: Number(row?.[5] || 0),
    quoteVolume: Number(row?.[7] || 0),
    tradeCount: Number(row?.[8] || 0),
    takerBuyBaseVolume: Number(row?.[9] || 0),
    takerBuyQuoteVolume: Number(row?.[10] || 0),
    closeTime: Number(row?.[6] || 0),
  }
}

function parseOKXKline(row) {
  return {
    openTime: Number(row?.[0] || 0),
    open: Number(row?.[1] || 0),
    high: Number(row?.[2] || 0),
    low: Number(row?.[3] || 0),
    close: Number(row?.[4] || 0),
    volume: Number(row?.[5] || 0),
    quoteVolume: Number(row?.[7] || 0),
    tradeCount: 0,
    takerBuyBaseVolume: 0,
    takerBuyQuoteVolume: 0,
    closeTime: Number(row?.[0] || 0),
  }
}

function buildEMA(values, period) {
  if (!values.length) return []
  const k = 2 / (period + 1)
  const out = Array(values.length).fill(null)
  let prev = Number(values[0] || 0)
  out[0] = prev
  for (let i = 1; i < values.length; i += 1) {
    const price = Number(values[i] || 0)
    prev = price * k + prev * (1 - k)
    out[i] = prev
  }
  return out
}

function buildRSI(values, period = 14) {
  const out = Array(values.length).fill(null)
  if (values.length <= period) return out
  let gainSum = 0
  let lossSum = 0
  for (let i = 1; i <= period; i += 1) {
    const diff = Number(values[i] || 0) - Number(values[i - 1] || 0)
    if (diff >= 0) gainSum += diff
    else lossSum += Math.abs(diff)
  }
  let avgGain = gainSum / period
  let avgLoss = lossSum / period
  out[period] = avgLoss === 0 ? 100 : (100 - (100 / (1 + (avgGain / avgLoss))))
  for (let i = period + 1; i < values.length; i += 1) {
    const diff = Number(values[i] || 0) - Number(values[i - 1] || 0)
    const gain = diff > 0 ? diff : 0
    const loss = diff < 0 ? Math.abs(diff) : 0
    avgGain = ((avgGain * (period - 1)) + gain) / period
    avgLoss = ((avgLoss * (period - 1)) + loss) / period
    out[i] = avgLoss === 0 ? 100 : (100 - (100 / (1 + (avgGain / avgLoss))))
  }
  return out
}

function buildMACD(values, shortPeriod = 12, longPeriod = 26, signalPeriod = 9) {
  const emaShort = buildEMA(values, shortPeriod)
  const emaLong = buildEMA(values, longPeriod)
  const macd = Array(values.length).fill(null)
  const signal = Array(values.length).fill(null)
  const hist = Array(values.length).fill(null)
  for (let i = 0; i < values.length; i += 1) {
    if (Number.isFinite(emaShort[i]) && Number.isFinite(emaLong[i])) {
      macd[i] = Number(emaShort[i]) - Number(emaLong[i])
    }
  }
  const k = 2 / (signalPeriod + 1)
  let prevSignal = null
  for (let i = 0; i < macd.length; i += 1) {
    const v = macd[i]
    if (!Number.isFinite(v)) continue
    if (prevSignal === null) prevSignal = v
    else prevSignal = v * k + prevSignal * (1 - k)
    signal[i] = prevSignal
    hist[i] = v - prevSignal
  }
  return { macd, signal, hist }
}

function buildBOLL(values, period = 20, stdMult = 2) {
  const mid = Array(values.length).fill(null)
  const upper = Array(values.length).fill(null)
  const lower = Array(values.length).fill(null)
  if (values.length < period) return { mid, upper, lower }
  for (let i = period - 1; i < values.length; i += 1) {
    const window = values.slice(i - period + 1, i + 1).map((v) => Number(v || 0))
    const mean = window.reduce((s, v) => s + v, 0) / period
    const variance = window.reduce((s, v) => s + ((v - mean) ** 2), 0) / period
    const std = Math.sqrt(Math.max(0, variance))
    mid[i] = mean
    upper[i] = mean + stdMult * std
    lower[i] = mean - stdMult * std
  }
  return { mid, upper, lower }
}

function buildVWAP(candles) {
  const out = Array(candles.length).fill(null)
  let cumPV = 0
  let cumV = 0
  for (let i = 0; i < candles.length; i += 1) {
    const c = candles[i] || {}
    const high = Number(c.high || 0)
    const low = Number(c.low || 0)
    const close = Number(c.close || 0)
    const vol = Number(c.volume || 0)
    const tp = (high + low + close) / 3
    if (vol > 0 && Number.isFinite(tp)) {
      cumPV += tp * vol
      cumV += vol
      out[i] = cumV > 0 ? (cumPV / cumV) : null
    }
  }
  return out
}

function buildKDJ(candles, period = 9) {
  const k = Array(candles.length).fill(null)
  const d = Array(candles.length).fill(null)
  const j = Array(candles.length).fill(null)
  let prevK = 50
  let prevD = 50
  for (let i = 0; i < candles.length; i += 1) {
    const start = Math.max(0, i - period + 1)
    const scope = candles.slice(start, i + 1)
    const hh = Math.max(...scope.map((x) => Number(x?.high || 0)))
    const ll = Math.min(...scope.map((x) => Number(x?.low || 0)))
    const close = Number(candles[i]?.close || 0)
    const rsv = hh === ll ? 50 : ((close - ll) / Math.max(1e-9, hh - ll)) * 100
    const nextK = (2 * prevK + rsv) / 3
    const nextD = (2 * prevD + nextK) / 3
    const nextJ = 3 * nextK - 2 * nextD
    k[i] = nextK
    d[i] = nextD
    j[i] = nextJ
    prevK = nextK
    prevD = nextD
  }
  return { k, d, j }
}

function buildATR(candles, period = 14) {
  const out = Array(candles.length).fill(null)
  if (!candles.length) return out
  const tr = Array(candles.length).fill(0)
  for (let i = 0; i < candles.length; i += 1) {
    const h = Number(candles[i]?.high || 0)
    const l = Number(candles[i]?.low || 0)
    if (i === 0) {
      tr[i] = Math.max(0, h - l)
      continue
    }
    const pc = Number(candles[i - 1]?.close || 0)
    tr[i] = Math.max(
      h - l,
      Math.abs(h - pc),
      Math.abs(l - pc),
    )
  }
  if (candles.length < period) return out
  let seed = 0
  for (let i = 0; i < period; i += 1) seed += tr[i]
  let prev = seed / period
  out[period - 1] = prev
  for (let i = period; i < candles.length; i += 1) {
    prev = ((prev * (period - 1)) + tr[i]) / period
    out[i] = prev
  }
  return out
}

function buildLinePath(values, xAt, yMap) {
  let d = ''
  let started = false
  for (let i = 0; i < values.length; i += 1) {
    const v = values[i]
    if (!Number.isFinite(v)) {
      started = false
      continue
    }
    const x = xAt(i)
    const y = yMap(Number(v))
    d += `${started ? ' L' : ' M'} ${x} ${y}`
    started = true
  }
  return d.trim()
}

function formatTickTime(ts, interval) {
  const d = new Date(Number(ts || 0))
  if (Number.isNaN(d.getTime())) return '--'
  const pad = (n) => String(n).padStart(2, '0')
  if (String(interval).endsWith('m')) return `${pad(d.getHours())}:${pad(d.getMinutes())}`
  if (String(interval).endsWith('h')) return `${pad(d.getMonth() + 1)}/${pad(d.getDate())} ${pad(d.getHours())}:00`
  if (String(interval) === '1M') return `${d.getFullYear()}/${pad(d.getMonth() + 1)}`
  if (String(interval) === '1w') return `${pad(d.getMonth() + 1)}/${pad(d.getDate())}`
  return `${pad(d.getMonth() + 1)}/${pad(d.getDate())}`
}

function priceDigitsFor(value) {
  const n = Math.abs(Number(value || 0))
  if (n >= 1000) return 2
  if (n >= 100) return 3
  if (n >= 1) return 4
  if (n >= 0.1) return 5
  return 6
}

function normalizeQuickIntervals(raw) {
  const src = Array.isArray(raw) ? raw : QUICK_INTERVAL_DEFAULT
  const picked = src
    .map((v) => String(v))
    .filter((v, i, arr) => arr.indexOf(v) === i && KLINE_INTERVAL_OPTIONS.includes(v))
    .slice(0, QUICK_INTERVAL_MAX)
  return picked.length ? picked : [...QUICK_INTERVAL_DEFAULT]
}

export function BinanceAdvancedChart({ symbol, theme, exchange = 'binance' }) {
  const [candles, setCandles] = useState([])
  const [ticker24h, setTicker24h] = useState(null)
  const [loadError, setLoadError] = useState('')
  const [loading, setLoading] = useState(true)
  const [hover, setHover] = useState({ active: false, x: 0, y: 0, sx: 0, sy: 0, cx: 0, cy: 0, i: -1, w: 0, h: 0 })
  const [interval, setInterval] = useState(KLINE_INTERVAL)
  const [isHoveringChart, setIsHoveringChart] = useState(false)
  const [shellHeight, setShellHeight] = useState(() => {
    try {
      const raw = Number(localStorage.getItem(KLINE_HEIGHT_STORAGE_KEY) || KLINE_HEIGHT_DEFAULT)
      if (!Number.isFinite(raw)) return KLINE_HEIGHT_DEFAULT
      return clampValue(Math.round(raw), KLINE_HEIGHT_MIN, KLINE_HEIGHT_MAX)
    } catch {
      return KLINE_HEIGHT_DEFAULT
    }
  })
  const [resizingShell, setResizingShell] = useState(false)
  const [quickIntervals, setQuickIntervals] = useState(() => {
    try {
      const raw = JSON.parse(localStorage.getItem(QUICK_INTERVALS_STORAGE_KEY) || '[]')
      return normalizeQuickIntervals(raw)
    } catch {
      return normalizeQuickIntervals([])
    }
  })
  const [intervalMenuOpen, setIntervalMenuOpen] = useState(false)
  const [quickDragFrom, setQuickDragFrom] = useState(-1)
  const [viewCount, setViewCount] = useState(180)
  const [viewOffset, setViewOffset] = useState(0)
  const [isDragging, setIsDragging] = useState(false)
  const [indicators, setIndicators] = useState({
    ema: true,
    boll: false,
    vwap: false,
    volume: true,
    rsi: true,
    kdj: false,
    macd: true,
    atr: false,
  })
  const dragRef = useRef({ active: false, startX: 0, startOffset: 0 })
  const shellRef = useRef(null)
  const svgRef = useRef(null)
  const intervalMenuRef = useRef(null)
  const viewOffsetRef = useRef(0)
  const viewCountRef = useRef(180)
  const resizeRef = useRef({ active: false, startY: 0, startH: KLINE_HEIGHT_DEFAULT })
  const isDark = theme === 'dark'
  const resolvedSymbol = useMemo(() => normalizeSymbol(symbol), [symbol])
  const resolvedExchange = useMemo(() => normalizeExchange(exchange), [exchange])

  useEffect(() => {
    viewOffsetRef.current = Number(viewOffset || 0)
  }, [viewOffset])

  useEffect(() => {
    viewCountRef.current = Number(viewCount || 180)
  }, [viewCount])

  useEffect(() => {
    setViewOffset(0)
    setHover((h) => ({ ...h, active: false }))
  }, [resolvedSymbol, interval])

  useEffect(() => {
    try {
      localStorage.setItem(QUICK_INTERVALS_STORAGE_KEY, JSON.stringify(quickIntervals))
    } catch {
      // ignore storage write failure
    }
  }, [quickIntervals])

  useEffect(() => {
    try {
      localStorage.setItem(KLINE_HEIGHT_STORAGE_KEY, String(shellHeight))
    } catch {
      // ignore storage write failure
    }
  }, [shellHeight])

  useEffect(() => {
    if (!intervalMenuOpen) return undefined
    const onDocDown = (evt) => {
      const target = evt.target
      if (!(target instanceof Element)) return
      if (intervalMenuRef.current?.contains(target)) return
      setIntervalMenuOpen(false)
    }
    window.addEventListener('mousedown', onDocDown)
    return () => window.removeEventListener('mousedown', onDocDown)
  }, [intervalMenuOpen])

  useEffect(() => {
    const onUp = () => {
      dragRef.current.active = false
      setIsDragging(false)
    }
    window.addEventListener('mouseup', onUp)
    return () => window.removeEventListener('mouseup', onUp)
  }, [])

  useEffect(() => {
    const onMove = (evt) => {
      if (!resizeRef.current.active) return
      evt.preventDefault()
      const raw = Number(resizeRef.current.startH || KLINE_HEIGHT_DEFAULT) + (evt.clientY - Number(resizeRef.current.startY || 0))
      const viewportCap = typeof window !== 'undefined' ? Math.max(KLINE_HEIGHT_MIN, window.innerHeight - 110) : KLINE_HEIGHT_MAX
      const maxH = Math.min(KLINE_HEIGHT_MAX, viewportCap)
      setShellHeight(clampValue(Math.round(raw), KLINE_HEIGHT_MIN, maxH))
    }
    const onUp = () => {
      if (!resizeRef.current.active) return
      resizeRef.current.active = false
      setResizingShell(false)
    }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
    return () => {
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
    }
  }, [])

  const applyZoomByDelta = (deltaY) => {
    setViewCount((old) => {
      const base = Math.max(40, Math.round(Number(old || 180)))
      const next = deltaY > 0 ? base + 14 : base - 14
      return Math.max(40, Math.min(KLINE_LIMIT, next))
    })
  }

  useEffect(() => {
    const onWindowWheel = (evt) => {
      if (!isHoveringChart) return
      const target = evt.target
      if (target instanceof Element && target.closest('.interval-dropdown-menu')) return
      evt.preventDefault()
      evt.stopPropagation()
      applyZoomByDelta(evt.deltaY)
    }
    window.addEventListener('wheel', onWindowWheel, { passive: false })
    return () => window.removeEventListener('wheel', onWindowWheel)
  }, [isHoveringChart])

  useEffect(() => {
    let cancelled = false
    let ws = null
    setLoadError('')
    setLoading(true)

    const loadHistory = async () => {
      let parsed = []
      if (resolvedExchange === 'okx') {
        const instId = toOKXInstID(resolvedSymbol)
        const bar = toOKXBar(interval)
        const okxLimit = Math.min(KLINE_LIMIT, 300)
        const url = `${OKX_REST}/api/v5/market/candles?instId=${encodeURIComponent(instId)}&bar=${encodeURIComponent(bar)}&limit=${okxLimit}`
        const res = await fetch(url)
        if (!res.ok) throw new Error(`OKX K线请求失败(${res.status})`)
        const raw = await res.json()
        if (String(raw?.code || '') !== '0') {
          throw new Error(String(raw?.msg || 'OKX K线返回异常'))
        }
        parsed = Array.isArray(raw?.data) ? raw.data.map(parseOKXKline).filter((k) => k.openTime > 0) : []
        parsed.sort((a, b) => Number(a.openTime || 0) - Number(b.openTime || 0))
      } else {
        const url = `${BINANCE_REST}/klines?symbol=${encodeURIComponent(resolvedSymbol)}&interval=${interval}&limit=${KLINE_LIMIT}`
        const res = await fetch(url)
        if (!res.ok) throw new Error(`币安K线请求失败(${res.status})`)
        const raw = await res.json()
        parsed = Array.isArray(raw) ? raw.map(parseBinanceKline).filter((k) => k.openTime > 0) : []
      }
      if (!parsed.length) throw new Error(`${resolvedExchange === 'okx' ? 'OKX' : '币安'} K线暂无可用数据`)
      if (!cancelled) {
        setCandles(parsed.slice(-KLINE_LIMIT))
        setLoading(false)
      }
    }

    const connectStream = () => {
      if (resolvedExchange === 'okx') {
        const instId = toOKXInstID(resolvedSymbol)
        const channel = `candle${toOKXBar(interval)}`
        ws = new WebSocket(OKX_WS)
        ws.onopen = () => {
          try {
            ws?.send(JSON.stringify({
              op: 'subscribe',
              args: [{ channel, instId }],
            }))
          } catch {
            // ignore
          }
        }
        ws.onmessage = (evt) => {
          if (cancelled) return
          try {
            const msg = JSON.parse(evt.data || '{}')
            if (msg?.event === 'error') {
              setLoadError(`OKX 实时K线连接异常：${String(msg?.msg || '未知错误')}`)
              return
            }
            const row = Array.isArray(msg?.data) && msg.data.length ? msg.data[0] : null
            if (!row) return
            const next = parseOKXKline(row)
            if (!next.openTime) return
            setCandles((prev) => {
              if (!prev.length) return [next]
              const last = prev[prev.length - 1]
              if (last.openTime === next.openTime) {
                return [...prev.slice(0, -1), next]
              }
              if (next.openTime > last.openTime) {
                const merged = [...prev, next].slice(-KLINE_LIMIT)
                if (viewOffsetRef.current > 0) {
                  setViewOffset((old) => {
                    const viewSize = Math.max(40, Math.min(Number(viewCountRef.current || 180), merged.length))
                    const maxOff = Math.max(0, merged.length - viewSize)
                    return Math.max(0, Math.min(maxOff, Number(old || 0) + 1))
                  })
                }
                return merged
              }
              return prev
            })
          } catch {
            // ignore invalid packets
          }
        }
        ws.onerror = () => {
          if (!cancelled) setLoadError('OKX 实时K线连接异常')
        }
        return
      }
      const stream = `${resolvedSymbol.toLowerCase()}@kline_${interval}`
      ws = new WebSocket(`${BINANCE_WS}/${stream}`)
      ws.onmessage = (evt) => {
        if (cancelled) return
        try {
          const msg = JSON.parse(evt.data || '{}')
          const k = msg?.k
          if (!k) return
          const next = {
            openTime: Number(k.t || 0),
            open: Number(k.o || 0),
            high: Number(k.h || 0),
            low: Number(k.l || 0),
            close: Number(k.c || 0),
            volume: Number(k.v || 0),
            quoteVolume: Number(k.q || 0),
            tradeCount: Number(k.n || 0),
            takerBuyBaseVolume: Number(k.V || 0),
            takerBuyQuoteVolume: Number(k.Q || 0),
            closeTime: Number(k.T || 0),
          }
          if (!next.openTime) return
          setCandles((prev) => {
            if (!prev.length) return [next]
            const last = prev[prev.length - 1]
            if (last.openTime === next.openTime) {
              return [...prev.slice(0, -1), next]
            }
            if (next.openTime > last.openTime) {
              const merged = [...prev, next].slice(-KLINE_LIMIT)
              if (viewOffsetRef.current > 0) {
                setViewOffset((old) => {
                  const viewSize = Math.max(40, Math.min(Number(viewCountRef.current || 180), merged.length))
                  const maxOff = Math.max(0, merged.length - viewSize)
                  return Math.max(0, Math.min(maxOff, Number(old || 0) + 1))
                })
              }
              return merged
            }
            return prev
          })
        } catch {
          // ignore invalid packets
        }
      }
      ws.onerror = () => {
        if (!cancelled) setLoadError('币安实时K线连接异常')
      }
    }

    loadHistory()
      .catch((e) => {
        if (!cancelled) {
          setLoadError(String(e?.message || 'K线加载失败'))
          setLoading(false)
        }
      })
      .finally(() => {
        if (!cancelled) connectStream()
      })

    return () => {
      cancelled = true
      if (ws) ws.close()
    }
  }, [resolvedExchange, resolvedSymbol, interval])

  useEffect(() => {
    if (!candles.length) return
    const viewSize = Math.max(1, Math.min(Math.max(40, Math.round(Number(viewCount || 180))), candles.length))
    const maxOff = Math.max(0, candles.length - viewSize)
    setViewOffset((old) => Math.max(0, Math.min(maxOff, Number(old || 0))))
  }, [candles.length, viewCount])

  useEffect(() => {
    let cancelled = false
    let timer = 0
    const loadTicker = async () => {
      try {
        let raw = null
        if (resolvedExchange === 'okx') {
          const instId = toOKXInstID(resolvedSymbol)
          const url = `${OKX_REST}/api/v5/market/ticker?instId=${encodeURIComponent(instId)}`
          const res = await fetch(url)
          if (!res.ok) return
          const json = await res.json()
          const row = Array.isArray(json?.data) && json.data.length ? json.data[0] : null
          if (!row) return
          const last = Number(row?.last || 0)
          const open24h = Number(row?.open24h || 0)
          const change = Number.isFinite(last) && Number.isFinite(open24h) ? last - open24h : 0
          const changePct = open24h > 0 ? (change / open24h) * 100 : 0
          raw = {
            highPrice: Number(row?.high24h || 0),
            lowPrice: Number(row?.low24h || 0),
            volume: Number(row?.vol24h || 0),
            quoteVolume: Number(row?.volCcy24h || 0),
            priceChange: change,
            priceChangePercent: changePct,
          }
        } else {
          const url = `${BINANCE_REST}/ticker/24hr?symbol=${encodeURIComponent(resolvedSymbol)}`
          const res = await fetch(url)
          if (!res.ok) return
          raw = await res.json()
        }
        if (cancelled) return
        setTicker24h(raw || null)
      } catch {
        if (!cancelled) setTicker24h(null)
      }
    }
    loadTicker()
    timer = window.setInterval(loadTicker, 12000)
    return () => {
      cancelled = true
      if (timer) window.clearInterval(timer)
    }
  }, [resolvedExchange, resolvedSymbol])

  if (loading && !candles.length) {
    return <div className={`binance-kline-shell ${isDark ? 'is-dark' : 'is-light'}`}><div className="binance-kline-empty">K线加载中...</div></div>
  }

  if (!candles.length) {
    return (
      <div className={`binance-kline-shell ${isDark ? 'is-dark' : 'is-light'}`}>
        <div className="binance-kline-empty">{loadError || '暂无K线数据'}</div>
      </div>
    )
  }

  const width = 1200
  const height = 680
  const left = 6
  const right = 64
  const top = 12
  const bottom = 22
  const innerW = width - left - right
  const innerH = height - top - bottom
  const rawView = Math.max(40, Math.round(Number(viewCount || 180)))
  const viewSize = Math.max(1, Math.min(rawView, candles.length))
  const maxOffset = Math.max(0, candles.length - viewSize)
  const safeOffset = Math.max(0, Math.min(Number(viewOffset || 0), maxOffset))
  const endIdx = Math.max(1, candles.length - safeOffset)
  const startIdx = Math.max(0, endIdx - viewSize)
  const viewCandles = candles.slice(startIdx, endIdx)
  const gap = 8
  const showVolume = Boolean(indicators.volume)
  const showRSI = Boolean(indicators.rsi)
  const showKDJ = Boolean(indicators.kdj)
  const showMomentum = showRSI || showKDJ
  const showMACD = Boolean(indicators.macd)
  const showATR = Boolean(indicators.atr)
  const volBase = showVolume ? Math.floor(innerH * 0.14) : 0
  const momentumBase = showMomentum ? Math.floor(innerH * 0.12) : 0
  const macdBase = showMACD ? Math.floor(innerH * 0.18) : 0
  const panelCount = Number(showVolume) + Number(showMomentum) + Number(showMACD)
  const priceH = Math.max(180, innerH - volBase - momentumBase - macdBase - (gap * panelCount))
  const priceY0 = top
  let cursorY = priceY0 + priceH
  const volY0 = showVolume ? (cursorY + gap) : 0
  if (showVolume) cursorY = volY0 + volBase
  const momentumY0 = showMomentum ? (cursorY + gap) : 0
  if (showMomentum) cursorY = momentumY0 + momentumBase
  const macdY0 = showMACD ? (cursorY + gap) : 0
  const maxPrice = Math.max(...viewCandles.map((c) => Number(c.high || 0)))
  const minPrice = Math.min(...viewCandles.map((c) => Number(c.low || 0)))
  const paddedMax = maxPrice * 1.003
  const paddedMin = Math.max(0, minPrice * 0.997)
  const span = Math.max(1e-9, paddedMax - paddedMin)
  const xStep = viewCandles.length > 1 ? innerW / (viewCandles.length - 1) : innerW
  const x = (idx) => (viewCandles.length > 1 ? left + xStep * idx : left + innerW / 2)
  const priceY = (price) => priceY0 + ((paddedMax - price) / span) * priceH
  const volumes = viewCandles.map((c) => Number(c.volume || 0))
  const volMax = Math.max(1, ...volumes)
  const volY = (vol) => volY0 + (1 - (Number(vol || 0) / volMax)) * Math.max(1, volBase)
  const momentumY = (value) => momentumY0 + ((100 - Math.max(0, Math.min(100, Number(value || 0)))) / 100) * Math.max(1, momentumBase)
  const bodyWidth = Math.max(2, Math.min(10, xStep * 0.62))
  const closes = viewCandles.map((c) => Number(c.close || 0))
  const ema7 = buildEMA(closes, 7)
  const ema25 = buildEMA(closes, 25)
  const ema99 = buildEMA(closes, 99)
  const bollPack = buildBOLL(closes, 20, 2)
  const vwap = buildVWAP(viewCandles)
  const rsi14 = buildRSI(closes, 14)
  const kdjPack = buildKDJ(viewCandles, 9)
  const macdPack = buildMACD(closes, 12, 26, 9)
  const atr14 = buildATR(viewCandles, 14)
  const macdAbs = Math.max(
    1e-9,
    ...macdPack.macd.map((v) => Math.abs(Number(v || 0))),
    ...macdPack.signal.map((v) => Math.abs(Number(v || 0))),
    ...macdPack.hist.map((v) => Math.abs(Number(v || 0))),
  )
  const macdZeroY = macdY0 + Math.max(1, macdBase) / 2
  const macdY = (v) => macdZeroY - (Number(v || 0) / macdAbs) * (Math.max(1, macdBase) * 0.46)

  const ema7Path = buildLinePath(ema7, x, priceY)
  const ema25Path = buildLinePath(ema25, x, priceY)
  const ema99Path = buildLinePath(ema99, x, priceY)
  const bollMidPath = buildLinePath(bollPack.mid, x, priceY)
  const bollUpperPath = buildLinePath(bollPack.upper, x, priceY)
  const bollLowerPath = buildLinePath(bollPack.lower, x, priceY)
  const vwapPath = buildLinePath(vwap, x, priceY)
  const rsiPath = buildLinePath(rsi14, x, momentumY)
  const kPath = buildLinePath(kdjPack.k, x, momentumY)
  const dPath = buildLinePath(kdjPack.d, x, momentumY)
  const jPath = buildLinePath(kdjPack.j, x, momentumY)
  const macdPath = buildLinePath(macdPack.macd, x, macdY)
  const macdSignalPath = buildLinePath(macdPack.signal, x, macdY)

  const priceTicks = Array.from({ length: 5 }, (_, i) => {
    const ratio = i / 4
    const price = paddedMax - span * ratio
    return { key: `py-${i}`, y: priceY0 + priceH * ratio, price }
  })
  const xTicks = Array.from({ length: 6 }, (_, i) => {
    const ratio = i / 5
    const idx = Math.min(viewCandles.length - 1, Math.max(0, Math.round((viewCandles.length - 1) * ratio)))
    return { key: `x-${i}`, x: x(idx), label: formatTickTime(viewCandles[idx]?.openTime, interval), idx }
  })
  const activeIndex = hover.active && hover.i >= 0 ? hover.i : (viewCandles.length - 1)
  const active = viewCandles[Math.max(0, Math.min(viewCandles.length - 1, activeIndex))]
  const activeEMA7 = Number(ema7?.[activeIndex] || 0)
  const activeEMA25 = Number(ema25?.[activeIndex] || 0)
  const activeEMA99 = Number(ema99?.[activeIndex] || 0)
  const activeBollUpper = Number(bollPack.upper?.[activeIndex] || 0)
  const activeBollMid = Number(bollPack.mid?.[activeIndex] || 0)
  const activeBollLower = Number(bollPack.lower?.[activeIndex] || 0)
  const activeVWAP = Number(vwap?.[activeIndex] || 0)
  const activeRSI = Number(rsi14?.[activeIndex] || 0)
  const activeK = Number(kdjPack.k?.[activeIndex] || 0)
  const activeD = Number(kdjPack.d?.[activeIndex] || 0)
  const activeJ = Number(kdjPack.j?.[activeIndex] || 0)
  const activeMACD = Number(macdPack.macd?.[activeIndex] || 0)
  const activeSignal = Number(macdPack.signal?.[activeIndex] || 0)
  const activeHist = Number(macdPack.hist?.[activeIndex] || 0)
  const activeATR = Number(atr14?.[activeIndex] || 0)
  const activeTime = formatTickTime(active?.openTime, interval)
  const last = candles[candles.length - 1]
  const prevLast = candles[candles.length - 2] || last
  const change = Number(last.close || 0) - Number(prevLast.close || 0)
  const changePct = Number(prevLast.close || 0) > 0 ? (change / Number(prevLast.close || 1)) * 100 : 0
  const priceDigits = priceDigitsFor(last.close)
  const tickerHigh = Number(ticker24h?.highPrice || 0)
  const tickerLow = Number(ticker24h?.lowPrice || 0)
  const tickerVol = Number(ticker24h?.volume || 0)
  const tickerQuoteVol = Number(ticker24h?.quoteVolume || 0)
  const tickerChange = Number(ticker24h?.priceChange)
  const tickerChangePct = Number(ticker24h?.priceChangePercent)
  const headlineChange = Number.isFinite(tickerChange) ? tickerChange : change
  const headlineChangePct = Number.isFinite(tickerChangePct) ? tickerChangePct : changePct
  const intrabarRangePct = Number(active?.open || 0) > 0
    ? ((Number(active?.high || 0) - Number(active?.low || 0)) / Number(active?.open || 1)) * 100
    : 0
  const dayRangePct = tickerLow > 0 ? ((tickerHigh - tickerLow) / tickerLow) * 100 : 0
  const priceVsEMA25Pct = activeEMA25 > 0
    ? ((Number(active?.close || 0) - activeEMA25) / activeEMA25) * 100
    : 0
  const buyVolumeRatio = Number(active?.volume || 0) > 0
    ? (Number(active?.takerBuyBaseVolume || 0) / Number(active?.volume || 1)) * 100
    : 0
  const bollWidthPct = activeBollMid > 0
    ? ((activeBollUpper - activeBollLower) / activeBollMid) * 100
    : 0
  const trendState = activeEMA7 > activeEMA25 && activeEMA25 > activeEMA99
    ? '多头趋势'
    : (activeEMA7 < activeEMA25 && activeEMA25 < activeEMA99 ? '空头趋势' : '震荡整理')
  const hoverLineX = hover.active
    ? Math.max(left, Math.min(width - right, Number(hover.x || 0)))
    : x(activeIndex)
  const hoverLineY = hover.active
    ? Math.max(priceY0, Math.min(height - bottom, Number(hover.y || 0)))
    : priceY(Number(active?.close || 0))
  const hoverPriceMapY = Math.max(priceY0, Math.min(priceY0 + priceH, hoverLineY))
  const hoverPriceValue = paddedMax - ((hoverPriceMapY - priceY0) / Math.max(1, priceH)) * span
  const latestPriceY = priceY(Number(last.close || 0))
  const hoverTimeTagWidth = Math.max(84, String(activeTime).length * 7.4 + 18)
  const hoverTimeTagX = Math.max(left, Math.min(width - right - hoverTimeTagWidth, hoverLineX - hoverTimeTagWidth / 2))
  const hoverTimeTagY = height - bottom + 4
  const tooltipWidth = 250
  const tooltipScreenX = Number(hover.sx || 0)
  const tooltipScreenY = Number(hover.sy || 0)
  const viewportW = typeof window !== 'undefined' ? window.innerWidth : 1920
  const preferTooltipLeft = Number(hover.cx || 0) + tooltipWidth + 24 > viewportW
  const tooltipLeftRaw = preferTooltipLeft ? (tooltipScreenX - tooltipWidth - 12) : (tooltipScreenX + 12)
  const tooltipLeft = Math.max(8, Math.min(Math.max(8, hover.w - tooltipWidth - 8), tooltipLeftRaw))
  const tooltipTop = tooltipScreenY > hover.h - 188 ? Math.max(8, tooltipScreenY - 176) : (tooltipScreenY + 12)
  const providerLabel = resolvedExchange === 'okx' ? 'OKX' : 'Binance'

  const onMove = (e) => {
    const rect = e.currentTarget.getBoundingClientRect()
    const px = e.clientX - rect.left
    const py = e.clientY - rect.top
    let chartX = (px / Math.max(1, rect.width)) * width
    let chartY = (py / Math.max(1, rect.height)) * height
    if (svgRef.current?.createSVGPoint) {
      const pt = svgRef.current.createSVGPoint()
      pt.x = e.clientX
      pt.y = e.clientY
      const ctm = svgRef.current.getScreenCTM?.()
      if (ctm && typeof ctm.inverse === 'function') {
        const mapped = pt.matrixTransform(ctm.inverse())
        chartX = Number(mapped.x || chartX)
        chartY = Number(mapped.y || chartY)
      }
    }
    if (dragRef.current.active) {
      const shift = Math.round((e.clientX - dragRef.current.startX) / Math.max(1, xStep))
      const nextOffset = Math.max(0, Math.min(maxOffset, dragRef.current.startOffset + shift))
      setViewOffset(nextOffset)
      setHover((h) => ({ ...h, active: false }))
      return
    }
    const idx = Math.max(0, Math.min(viewCandles.length - 1, Math.round((chartX - left) / Math.max(1, xStep))))
    setHover({
      active: true,
      x: chartX,
      y: chartY,
      sx: px,
      sy: py,
      cx: e.clientX,
      cy: e.clientY,
      i: idx,
      w: rect.width,
      h: rect.height,
    })
  }

  const chooseInterval = (nextInterval, closeMenu = false) => {
    setInterval(String(nextInterval || KLINE_INTERVAL))
    if (closeMenu) setIntervalMenuOpen(false)
  }

  const toggleQuickInterval = (opt) => {
    const next = String(opt || '')
    setQuickIntervals((prev) => {
      const exists = prev.includes(next)
      if (exists) return normalizeQuickIntervals(prev.filter((v) => v !== next))
      if (prev.length >= QUICK_INTERVAL_MAX) return prev
      return normalizeQuickIntervals([...prev, next])
    })
  }

  const reorderQuickIntervals = (fromIdx, toIdx) => {
    const from = Number(fromIdx)
    const to = Number(toIdx)
    if (!Number.isInteger(from) || !Number.isInteger(to)) return
    setQuickIntervals((prev) => {
      if (from < 0 || to < 0 || from >= prev.length || to >= prev.length || from === to) return prev
      const next = [...prev]
      const [moved] = next.splice(from, 1)
      next.splice(to, 0, moved)
      return normalizeQuickIntervals(next)
    })
  }

  const onStartShellResize = (e) => {
    e.preventDefault()
    resizeRef.current.active = true
    resizeRef.current.startY = e.clientY
    resizeRef.current.startH = Number(shellHeight || KLINE_HEIGHT_DEFAULT)
    setResizingShell(true)
  }

  return (
    <div
      ref={shellRef}
      className={`binance-kline-shell ${isDark ? 'is-dark' : 'is-light'} ${resizingShell ? 'is-resizing' : ''}`}
      style={{ height: `${shellHeight}px` }}
      onMouseEnter={() => setIsHoveringChart(true)}
      onMouseLeave={() => setIsHoveringChart(false)}
    >
      <div className="binance-kline-head">
        <span>{resolvedSymbol} · {providerLabel} · {interval.toUpperCase()}</span>
        <div className="binance-kline-head-right">
          <div className="binance-kline-intervals" ref={intervalMenuRef}>
            <div className="binance-kline-quick-strip">
              {quickIntervals.map((opt) => (
                <button
                  key={opt}
                  type="button"
                  className={`kline-chip ${interval === opt ? 'active' : ''}`}
                  onClick={() => chooseInterval(opt)}
                >
                  {opt.toUpperCase()}
                </button>
              ))}
            </div>
            <div className={`interval-dropdown ${intervalMenuOpen ? 'open' : ''}`}>
              <button
                type="button"
                className={`kline-chip interval-dropdown-trigger ${intervalMenuOpen ? 'active' : ''}`}
                onClick={() => setIntervalMenuOpen((v) => !v)}
              >
                更多周期
              </button>
              {intervalMenuOpen ? (
                <div className="interval-dropdown-menu">
                  <div className="interval-dropdown-head">
                    <b>常用周期 {quickIntervals.length}/{QUICK_INTERVAL_MAX}</b>
                    <button
                      type="button"
                      className="interval-default-btn"
                      onClick={() => setQuickIntervals(normalizeQuickIntervals(QUICK_INTERVAL_DEFAULT))}
                    >
                      恢复默认
                    </button>
                  </div>
                  <span className="interval-dropdown-subtitle">外显顺序（拖拽调整）</span>
                  <div className="quick-order-list">
                    {quickIntervals.map((opt, idx) => (
                      <button
                        key={`quick-order-${opt}-${idx}`}
                        type="button"
                        draggable
                        className={`quick-order-chip ${interval === opt ? 'active' : ''} ${quickDragFrom === idx ? 'dragging' : ''}`}
                        onClick={() => chooseInterval(opt)}
                        onDragStart={(e) => {
                          setQuickDragFrom(idx)
                          e.dataTransfer.effectAllowed = 'move'
                          e.dataTransfer.setData('text/plain', String(idx))
                        }}
                        onDragOver={(e) => {
                          e.preventDefault()
                        }}
                        onDrop={(e) => {
                          e.preventDefault()
                          reorderQuickIntervals(quickDragFrom, idx)
                          setQuickDragFrom(-1)
                        }}
                        onDragEnd={() => setQuickDragFrom(-1)}
                      >
                        <span className="quick-order-chip-index">{idx + 1}</span>
                        <span>{opt.toUpperCase()}</span>
                      </button>
                    ))}
                  </div>
                  <span className="interval-dropdown-subtitle">全部周期（可切换 + 设为常用）</span>
                  <div className="interval-dropdown-list">
                    {KLINE_INTERVAL_OPTIONS.map((opt) => {
                      const checked = quickIntervals.includes(opt)
                      const disabled = !checked && quickIntervals.length >= QUICK_INTERVAL_MAX
                      return (
                        <div key={`interval-opt-${opt}`} className={`interval-dropdown-item ${interval === opt ? 'is-current' : ''}`}>
                          <button
                            type="button"
                            className="interval-select-btn"
                            onClick={() => chooseInterval(opt, true)}
                          >
                            {opt.toUpperCase()}
                          </button>
                          <label className={`interval-fav ${disabled ? 'disabled' : ''}`}>
                            <input
                              type="checkbox"
                              checked={checked}
                              disabled={disabled}
                              onChange={() => toggleQuickInterval(opt)}
                            />
                            常用
                          </label>
                        </div>
                      )
                    })}
                  </div>
                  <p className="interval-dropdown-tip">未置为常用的周期，可在此下拉菜单中直接切换。</p>
                </div>
              ) : null}
            </div>
          </div>
          <b className={headlineChange >= 0 ? 'up' : 'down'}>
            {fmtNum(last.close, priceDigits)} ({headlineChange >= 0 ? '+' : ''}{fmtPct(headlineChangePct)})
          </b>
        </div>
      </div>
      {ticker24h ? (
        <div className="binance-kline-market-stats">
          <span>24h高 {fmtNum(tickerHigh, priceDigits)}</span>
          <span>24h低 {fmtNum(tickerLow, priceDigits)}</span>
          <span>24h量 {fmtNum(tickerVol, 2)}</span>
          <span>24h额 {fmtNum(tickerQuoteVol, 2)} USDT</span>
        </div>
      ) : null}
      <div className="binance-kline-toolbar">
        <div className="binance-kline-tools-right">
          <span>显示 {viewSize} 根K线</span>
          <span>滚轮缩放/拖拽平移/双击回到最新</span>
          <button type="button" className="kline-chip" disabled={safeOffset === 0} onClick={() => setViewOffset(0)}>回到最新</button>
        </div>
      </div>
      <div className="binance-kline-stats">
        <span>时间 {activeTime}</span>
        <span>开 {fmtNum(active?.open, priceDigits)}</span>
        <span>高 {fmtNum(active?.high, priceDigits)}</span>
        <span>低 {fmtNum(active?.low, priceDigits)}</span>
        <span>收 {fmtNum(active?.close, priceDigits)}</span>
        <span>量 {fmtNum(active?.volume, 2)}</span>
        <span>额 {fmtNum(active?.quoteVolume, 2)}</span>
        <span>笔 {fmtNum(active?.tradeCount, 0)}</span>
      </div>
      <div className="binance-kline-main">
        <aside className="binance-kline-side left">
          <h4>技术指标</h4>
          <div className="binance-kline-indicators-vertical">
            <button type="button" className={`kline-chip ${indicators.ema ? 'active' : ''}`} onClick={() => setIndicators((v) => ({ ...v, ema: !v.ema }))}>EMA</button>
            <button type="button" className={`kline-chip ${indicators.boll ? 'active' : ''}`} onClick={() => setIndicators((v) => ({ ...v, boll: !v.boll }))}>BOLL</button>
            <button type="button" className={`kline-chip ${indicators.vwap ? 'active' : ''}`} onClick={() => setIndicators((v) => ({ ...v, vwap: !v.vwap }))}>VWAP</button>
            <button type="button" className={`kline-chip ${indicators.volume ? 'active' : ''}`} onClick={() => setIndicators((v) => ({ ...v, volume: !v.volume }))}>成交量</button>
            <button type="button" className={`kline-chip ${indicators.rsi ? 'active' : ''}`} onClick={() => setIndicators((v) => ({ ...v, rsi: !v.rsi }))}>RSI</button>
            <button type="button" className={`kline-chip ${indicators.kdj ? 'active' : ''}`} onClick={() => setIndicators((v) => ({ ...v, kdj: !v.kdj }))}>KDJ</button>
            <button type="button" className={`kline-chip ${indicators.macd ? 'active' : ''}`} onClick={() => setIndicators((v) => ({ ...v, macd: !v.macd }))}>MACD</button>
            <button type="button" className={`kline-chip ${indicators.atr ? 'active' : ''}`} onClick={() => setIndicators((v) => ({ ...v, atr: !v.atr }))}>ATR</button>
          </div>
        </aside>

        <div className="binance-kline-center">
          <svg
        ref={svgRef}
        viewBox={`0 0 ${width} ${height}`}
        preserveAspectRatio="none"
        className={`binance-kline-svg ${isDragging ? 'is-dragging' : ''}`}
        role="img"
        aria-label={`${resolvedSymbol}-kline`}
        onMouseMove={onMove}
        onMouseLeave={() => {
          setHover((h) => ({ ...h, active: false }))
          dragRef.current.active = false
          setIsDragging(false)
        }}
        onMouseDown={(e) => {
          if (e.button !== 0) return
          dragRef.current.active = true
          setIsDragging(true)
          dragRef.current.startX = e.clientX
          dragRef.current.startOffset = safeOffset
          setHover((h) => ({ ...h, active: false }))
        }}
        onDoubleClick={() => setViewOffset(0)}
      >
        <rect x="0" y="0" width={width} height={height} rx="12" className="binance-kline-bg" />
        {priceTicks.map((t) => (
          <line key={`${t.key}-line`} x1={left} y1={t.y} x2={width - right} y2={t.y} className="binance-kline-grid" />
        ))}
        <line x1={left} y1={priceY0} x2={left} y2={height - bottom} className="binance-kline-axis" />
        <line x1={left} y1={priceY0 + priceH} x2={width - right} y2={priceY0 + priceH} className="binance-kline-axis" />
        {showVolume ? <line x1={left} y1={volY0 + volBase} x2={width - right} y2={volY0 + volBase} className="binance-kline-axis subtle" /> : null}
        {showMomentum ? <line x1={left} y1={momentumY0 + momentumBase} x2={width - right} y2={momentumY0 + momentumBase} className="binance-kline-axis subtle" /> : null}
        {showMACD ? <line x1={left} y1={macdY0 + macdBase} x2={width - right} y2={macdY0 + macdBase} className="binance-kline-axis subtle" /> : null}
        {priceTicks.map((t) => (
          <text key={`${t.key}-txt`} x={width - right + 6} y={t.y + 4} textAnchor="start" className="binance-kline-tick">
            {fmtNum(t.price, priceDigits)}
          </text>
        ))}
        {xTicks.map((t) => (
          <text key={t.key} x={t.x} y={height - 10} textAnchor="middle" className="binance-kline-tick">
            {t.label}
          </text>
        ))}
        {showVolume ? <text x={left + 8} y={volY0 + 14} className="binance-kline-panel-label">VOL</text> : null}
        {showMomentum ? <text x={left + 8} y={momentumY0 + 14} className="binance-kline-panel-label">MOMENTUM</text> : null}
        {showMACD ? <text x={left + 8} y={macdY0 + 14} className="binance-kline-panel-label">MACD(12,26,9)</text> : null}
        {showMomentum ? <line x1={left} y1={momentumY(70)} x2={width - right} y2={momentumY(70)} className="binance-kline-grid subtle" /> : null}
        {showMomentum ? <line x1={left} y1={momentumY(30)} x2={width - right} y2={momentumY(30)} className="binance-kline-grid subtle" /> : null}
        {showMACD ? <line x1={left} y1={macdZeroY} x2={width - right} y2={macdZeroY} className="binance-kline-grid subtle" /> : null}
        {showVolume ? viewCandles.map((c, i) => {
          const up = Number(c.close || 0) >= Number(c.open || 0)
          const xCenter = x(i)
          const topY = volY(c.volume || 0)
          const h = Math.max(1, (volY0 + volBase) - topY)
          return (
            <rect
              key={`v-${c.openTime}-${i}`}
              x={xCenter - bodyWidth / 2}
              y={topY}
              width={bodyWidth}
              height={h}
              className={up ? 'binance-kline-vol up' : 'binance-kline-vol down'}
            />
          )
        }) : null}
        {viewCandles.map((c, i) => {
          const up = Number(c.close || 0) >= Number(c.open || 0)
          const wickX = x(i)
          const openY = priceY(Number(c.open || 0))
          const closeY = priceY(Number(c.close || 0))
          const bodyTop = Math.min(openY, closeY)
          const bodyHeight = Math.max(1, Math.abs(closeY - openY))
          return (
            <g key={`c-${c.openTime}-${i}`}>
              <line x1={wickX} y1={priceY(Number(c.high || 0))} x2={wickX} y2={priceY(Number(c.low || 0))} className={up ? 'binance-kline-wick up' : 'binance-kline-wick down'} />
              <rect
                x={wickX - bodyWidth / 2}
                y={bodyTop}
                width={bodyWidth}
                height={bodyHeight}
                rx="1"
                className={up ? 'binance-kline-candle up' : 'binance-kline-candle down'}
              />
            </g>
          )
        })}
        {indicators.ema ? <path d={ema7Path} fill="none" className="binance-kline-ema ema7" /> : null}
        {indicators.ema ? <path d={ema25Path} fill="none" className="binance-kline-ema ema25" /> : null}
        {indicators.ema ? <path d={ema99Path} fill="none" className="binance-kline-ema ema99" /> : null}
        {indicators.boll ? <path d={bollUpperPath} fill="none" className="binance-kline-ema boll-up" /> : null}
        {indicators.boll ? <path d={bollMidPath} fill="none" className="binance-kline-ema boll-mid" /> : null}
        {indicators.boll ? <path d={bollLowerPath} fill="none" className="binance-kline-ema boll-low" /> : null}
        {indicators.vwap ? <path d={vwapPath} fill="none" className="binance-kline-ema vwap" /> : null}
        {showRSI ? <path d={rsiPath} fill="none" className="binance-kline-ema rsi" /> : null}
        {showKDJ ? <path d={kPath} fill="none" className="binance-kline-ema k-line" /> : null}
        {showKDJ ? <path d={dPath} fill="none" className="binance-kline-ema d-line" /> : null}
        {showKDJ ? <path d={jPath} fill="none" className="binance-kline-ema j-line" /> : null}
        {showMACD ? macdPack.hist.map((v, i) => {
          if (!Number.isFinite(v)) return null
          const xCenter = x(i)
          const yVal = macdY(v)
          const yZero = macdZeroY
          const barTop = Math.min(yVal, yZero)
          const barH = Math.max(1, Math.abs(yVal - yZero))
          return (
            <rect
              key={`m-h-${viewCandles[i]?.openTime || i}`}
              x={xCenter - Math.max(1.3, bodyWidth * 0.38)}
              y={barTop}
              width={Math.max(2, bodyWidth * 0.76)}
              height={barH}
              className={Number(v) >= 0 ? 'binance-kline-macd-bar up' : 'binance-kline-macd-bar down'}
            />
          )
        }) : null}
        {showMACD ? <path d={macdPath} fill="none" className="binance-kline-ema macd" /> : null}
        {showMACD ? <path d={macdSignalPath} fill="none" className="binance-kline-ema signal" /> : null}
        <line x1={left} y1={latestPriceY} x2={width - right} y2={latestPriceY} className="binance-kline-last-price-line" />
        <rect x={width - right + 6} y={latestPriceY - 9} width={right - 12} height={18} rx="4" className="binance-kline-last-price-tag" />
        <text x={width - right + 10} y={latestPriceY + 4} className="binance-kline-last-price-text">{fmtNum(last.close, priceDigits)}</text>
        {hover.active ? (
          <>
            <line x1={hoverLineX} y1={priceY0} x2={hoverLineX} y2={height - bottom} className="binance-kline-crosshair" />
            <line x1={left} y1={hoverLineY} x2={width - right} y2={hoverLineY} className="binance-kline-crosshair" />
            <rect
              x={width - right + 6}
              y={hoverLineY - 9}
              width={right - 12}
              height={18}
              rx="4"
              className="binance-kline-hover-price-tag"
            />
            <text x={width - right + 10} y={hoverLineY + 4} className="binance-kline-hover-price-text">
              {fmtNum(hoverPriceValue, priceDigits)}
            </text>
            <rect
              x={hoverTimeTagX}
              y={hoverTimeTagY}
              width={hoverTimeTagWidth}
              height={18}
              rx="4"
              className="binance-kline-hover-time-tag"
            />
            <text
              x={hoverTimeTagX + hoverTimeTagWidth / 2}
              y={hoverTimeTagY + 12}
              textAnchor="middle"
              className="binance-kline-hover-time-text"
            >
              {activeTime}
            </text>
          </>
        ) : null}
          </svg>
          {hover.active ? (
            <div className="binance-kline-tooltip" style={{ left: `${tooltipLeft}px`, top: `${tooltipTop}px` }}>
              <p>{resolvedSymbol} · {activeTime}</p>
              <p>开 {fmtNum(active?.open, priceDigits)} / 高 {fmtNum(active?.high, priceDigits)}</p>
              <p>低 {fmtNum(active?.low, priceDigits)} / 收 {fmtNum(active?.close, priceDigits)}</p>
              <p>量 {fmtNum(active?.volume, 2)} / 额 {fmtNum(active?.quoteVolume, 2)}</p>
              {indicators.ema ? <p>EMA7 {fmtNum(activeEMA7, priceDigits)} · EMA25 {fmtNum(activeEMA25, priceDigits)}</p> : null}
              {indicators.boll ? <p>BOLL U/M/L {fmtNum(activeBollUpper, priceDigits)} / {fmtNum(activeBollMid, priceDigits)} / {fmtNum(activeBollLower, priceDigits)}</p> : null}
              {indicators.vwap ? <p>VWAP {fmtNum(activeVWAP, priceDigits)}</p> : null}
              {showRSI ? <p>RSI {fmtNum(activeRSI, 2)}</p> : null}
              {showKDJ ? <p>KDJ {fmtNum(activeK, 2)} / {fmtNum(activeD, 2)} / {fmtNum(activeJ, 2)}</p> : null}
              {showMACD ? <p>MACD {fmtNum(activeMACD, 4)} · Signal {fmtNum(activeSignal, 4)}</p> : null}
              {showATR ? <p>ATR14 {fmtNum(activeATR, priceDigits)}</p> : null}
            </div>
          ) : null}
        </div>

        <aside className="binance-kline-side right">
          <h4>实时分析</h4>
          <div className="kline-side-metrics">
            <p><span>市场结构</span><b>{trendState}</b></p>
            <p><span>单根振幅</span><b>{fmtPct(intrabarRangePct)}</b></p>
            <p><span>24h振幅</span><b>{fmtPct(dayRangePct)}</b></p>
            <p><span>价离EMA25</span><b className={priceVsEMA25Pct >= 0 ? 'up' : 'down'}>{priceVsEMA25Pct >= 0 ? '+' : ''}{fmtPct(priceVsEMA25Pct)}</b></p>
            <p><span>主动买量占比</span><b>{fmtPct(buyVolumeRatio)}</b></p>
            <p><span>ATR14</span><b>{fmtNum(activeATR, priceDigits)}</b></p>
          </div>
        </aside>
      </div>
      <div className="binance-kline-legend">
        {indicators.ema ? <span><i className="ema-dot ema7" />EMA7 {fmtNum(activeEMA7, priceDigits)}</span> : null}
        {indicators.ema ? <span><i className="ema-dot ema25" />EMA25 {fmtNum(activeEMA25, priceDigits)}</span> : null}
        {indicators.ema ? <span><i className="ema-dot ema99" />EMA99 {fmtNum(activeEMA99, priceDigits)}</span> : null}
        {indicators.boll ? <span><i className="ema-dot boll" />BOLL宽度 {fmtPct(bollWidthPct)}</span> : null}
        {indicators.vwap ? <span><i className="ema-dot vwap" />VWAP {fmtNum(activeVWAP, priceDigits)}</span> : null}
        {showRSI ? <span><i className="ema-dot rsi" />RSI {fmtNum(activeRSI, 2)}</span> : null}
        {showKDJ ? <span><i className="ema-dot kdj" />KDJ {fmtNum(activeK, 2)}/{fmtNum(activeD, 2)}/{fmtNum(activeJ, 2)}</span> : null}
        {showMACD ? <span><i className="ema-dot macd" />MACD {fmtNum(activeMACD, 4)}</span> : null}
        {showMACD ? <span><i className="ema-dot signal" />Signal {fmtNum(activeSignal, 4)}</span> : null}
        {showMACD ? <span><i className="ema-dot hist" />Hist {fmtNum(activeHist, 4)}</span> : null}
        {showATR ? <span><i className="ema-dot atr" />ATR14 {fmtNum(activeATR, priceDigits)}</span> : null}
        {loadError ? <span className="warn">{loadError}</span> : null}
      </div>
      <div
        className="binance-kline-resizer"
        role="separator"
        aria-label="调整K线高度"
        onMouseDown={onStartShellResize}
      />
    </div>
  )
}

export function AssetTrendChart({ points, range = '30D' }) {
  if (!points.length) return <p className="muted">暂无资产趋势数据</p>
  const rangeDays = (() => {
    switch (String(range || '30D')) {
      case '7D':
        return 7
      case '3M':
        return 90
      case '6M':
        return 180
      case '1Y':
        return 365
      case '30D':
      default:
        return 30
    }
  })()
  const width = 920
  const height = 280
  const left = 58
  const right = 16
  const top = 18
  const bottom = 34
  const innerW = width - left - right
  const innerH = height - top - bottom
  const safePoints = points.map((p) => {
    const n = Number(p?.equity || 0)
    return {
      ts: p?.ts || '',
      equity: Number.isFinite(n) ? Math.max(0, n) : 0,
    }
  })
  const maxV = Math.max(1, ...safePoints.map((x) => x.equity))
  const minV = 0
  const span = Math.max(maxV - minV, 1e-9)
  const step = safePoints.length > 1 ? innerW / (safePoints.length - 1) : 0
  const x = (idx) => (safePoints.length > 1 ? (left + step * idx) : (width - right))
  const y = (v) => top + ((maxV - v) / span) * innerH
  const fmtAxisNum = (v) => {
    const n = Number(v || 0)
    if (!Number.isFinite(n)) return '0'
    return n.toLocaleString('zh-CN', {
      minimumFractionDigits: 0,
      maximumFractionDigits: 2,
    })
  }
  const now = new Date()
  const endTime = now.getTime()
  const start = new Date(now)
  start.setHours(0, 0, 0, 0)
  start.setDate(start.getDate() - (rangeDays - 1))
  const startTime = start.getTime()
  const pad2 = (n) => String(n).padStart(2, '0')
  const fmtXAxisTime = (ts) => {
    const d = new Date(ts)
    if (String(range) === '3M' || String(range) === '6M' || String(range) === '1Y') {
      return `${d.getFullYear()}/${pad2(d.getMonth() + 1)}`
    }
    return `${pad2(d.getMonth() + 1)}/${pad2(d.getDate())}`
  }
  const linePath = safePoints
    .map((p, i) => `${i === 0 ? 'M' : 'L'} ${x(i)} ${y(p.equity)}`)
    .join(' ')
  const areaBaseY = y(0)
  const areaPath = `${linePath} L ${x(safePoints.length - 1)} ${areaBaseY} L ${x(0)} ${areaBaseY} Z`
  const firstEquity = safePoints[0]?.equity || 0
  const lastEquity = safePoints[safePoints.length - 1]?.equity || 0
  const delta = lastEquity - firstEquity
  const deltaPct = firstEquity === 0 ? 0 : (delta / Math.abs(firstEquity)) * 100
  const lastY = y(lastEquity)
  const yTicks = [0, 0.25, 0.5, 0.75, 1].map((r, i) => {
    const value = maxV * (1 - r)
    return {
      key: `y-${i}`,
      y: top + innerH * r,
      value,
    }
  })
  const xTickIndexes = (() => {
    if (safePoints.length <= 1) return [0]
    const maxTicks = 6
    const stepIdx = Math.max(1, Math.floor((safePoints.length - 1) / (maxTicks - 1)))
    const idxs = []
    for (let i = 0; i < safePoints.length; i += stepIdx) idxs.push(i)
    if (idxs[idxs.length - 1] !== safePoints.length - 1) idxs.push(safePoints.length - 1)
    return Array.from(new Set(idxs))
  })()
  const xTicks = xTickIndexes.map((idx) => {
    const ratio = safePoints.length > 1 ? (idx / (safePoints.length - 1)) : 1
    const tickTs = new Date(startTime + (endTime - startTime) * ratio)
    return {
      key: `x-${idx}`,
      x: x(idx),
      label: fmtXAxisTime(tickTs),
    }
  })

  return (
    <div className="asset-trend-shell">
      <div className="asset-trend-meta">
        <span>区间变化</span>
        <b className={delta >= 0 ? 'up' : 'down'}>
          {delta > 0 ? '+' : ''}{fmtNum(delta, 2)} ({deltaPct > 0 ? '+' : ''}{fmtPct(deltaPct)})
        </b>
      </div>
      <svg viewBox={`0 0 ${width} ${height}`} className="asset-trend-svg" role="img" aria-label="equity-trend">
        <defs>
          <linearGradient id="assetTrendFill" x1="0" x2="0" y1="0" y2="1">
            <stop offset="0%" stopColor="rgba(47,107,255,0.34)" />
            <stop offset="100%" stopColor="rgba(47,107,255,0.02)" />
          </linearGradient>
        </defs>
        <rect x="0" y="0" width={width} height={height} rx="14" className="asset-trend-bg" />
        {yTicks.map((g) => (
          <line
            key={g.key}
            x1={left}
            x2={width - right}
            y1={g.y}
            y2={g.y}
            className="asset-trend-grid"
          />
        ))}
        <line x1={left} y1={top} x2={left} y2={height - bottom} className="asset-trend-axis" />
        <line x1={left} y1={height - bottom} x2={width - right} y2={height - bottom} className="asset-trend-axis" />
        {yTicks.map((t) => (
          <text key={`${t.key}-label`} x={left - 8} y={t.y + 4} textAnchor="end" className="asset-trend-tick-label">
            {fmtAxisNum(t.value)}
          </text>
        ))}
        {xTicks.map((tick) => (
          <text key={tick.key} x={tick.x} y={height - 10} textAnchor="middle" className="asset-trend-tick-label">
            {tick.label}
          </text>
        ))}
        <text x={left} y={top - 6} className="asset-trend-axis-label">资金数量 (USDT)</text>
        <text x={(left + (width - right)) / 2} y={height - 2} textAnchor="middle" className="asset-trend-axis-label">时间</text>
        <path d={areaPath} fill="url(#assetTrendFill)" />
        <path d={linePath} fill="none" stroke="#2f6bff" strokeWidth="2.6" />
        <circle cx={x(safePoints.length - 1)} cy={lastY} r="4.4" fill="#2f6bff" />
      </svg>
    </div>
  )
}

export function PnLCalendar({ month, days }) {
  const toDayInt = (value) => {
    const n = Number(value)
    if (!Number.isFinite(n)) return 0
    return Math.max(0, Math.trunc(n))
  }

  const fmtPnlCompact = (value) => {
    const n = Number(value || 0)
    if (!Number.isFinite(n)) return '0'
    const rounded = Math.round(n * 100) / 100
    return rounded.toLocaleString('zh-CN', {
      minimumFractionDigits: 0,
      maximumFractionDigits: 2,
    })
  }

  const now = new Date()
  const monthMatch = String(month || '').match(/^(\d{4})-(\d{2})$/)
  const y = monthMatch ? Number(monthMatch[1]) : now.getFullYear()
  const m = monthMatch ? Number(monthMatch[2]) : (now.getMonth() + 1)
  const firstDay = new Date(y, Math.max(0, m - 1), 1)
  const firstWeekday = firstDay.getDay()
  const totalDays = new Date(y, m, 0).getDate()
  const map = {}
  const hasData = new Set()

  for (const row of days || []) {
    const dayRaw = row?.day ?? row?.date ?? row?.Date
    let dayNum = toDayInt(dayRaw)
    if (!dayNum || dayNum < 1 || dayNum > 31) {
      const text = String(dayRaw || '')
      const m2 = text.match(/(\d{4})-(\d{2})-(\d{2})/)
      if (m2) dayNum = toDayInt(m2[3])
    }
    if (!dayNum || dayNum < 1 || dayNum > totalDays) continue

    const pnlRaw = row?.pnl ?? row?.pnl_amount ?? row?.pnlAmount ?? row?.PnLAmount
    const pnl = Number(pnlRaw)
    map[dayNum] = Number.isFinite(pnl) ? pnl : 0
    hasData.add(dayNum)
  }

  const cells = []
  for (let i = 0; i < firstWeekday; i += 1) cells.push(null)
  for (let d = 1; d <= totalDays; d += 1) {
    cells.push({
      day: d,
      pnl: Number(map[d] || 0),
      hasData: hasData.has(d),
    })
  }
  while (cells.length % 7 !== 0) cells.push(null)

  const wds = ['日', '一', '二', '三', '四', '五', '六']

  return (
    <div className="pnl-calendar">
      <div className="calendar-grid head">
        {wds.map((w) => (
          <div key={w} className="calendar-head">{w}</div>
        ))}
        {cells.map((c, i) => (
          <div
            key={`cell-${i}`}
            className={`calendar-cell ${
              !c
                ? 'empty'
                : (!c.hasData ? 'nodata' : (c.pnl > 0 ? 'profit' : (c.pnl < 0 ? 'loss' : 'flat')))
            }`}
          >
            {c ? (
              <>
                <span className="calendar-day">{c.day}</span>
                <span className={`calendar-pnl ${c.pnl >= 0 ? 'up' : 'down'}`}>
                  {c.hasData ? `${c.pnl > 0 ? '+' : ''}${fmtPnlCompact(c.pnl)}` : ''}
                </span>
              </>
            ) : null}
          </div>
        ))}
      </div>
    </div>
  )
}

export function AssetDistributionChart({ items }) {
  const safeItems = Array.isArray(items) && items.length
    ? items
    : [
      { label: 'USDT', value: 100, color: '#2f6bff' },
      { label: 'BTC', value: 0, color: '#22c55e' },
      { label: 'ETH', value: 0, color: '#f59e0b' },
    ]
  const total = Math.max(1, safeItems.reduce((acc, it) => acc + Number(it.value || 0), 0))
  const cx = 120
  const cy = 120
  const r = 86
  const stroke = 28
  let start = -Math.PI / 2
  const toPoint = (angle) => ({
    x: cx + r * Math.cos(angle),
    y: cy + r * Math.sin(angle),
  })
  const arcs = safeItems.map((it) => {
    const frac = Math.max(0, Number(it.value || 0) / total)
    if (frac <= 0) {
      return {
        type: 'none',
        color: it.color || '#1f5fbe',
        label: it.label,
        value: it.value,
      }
    }
    if (frac >= 0.999999) {
      return {
        type: 'full',
        color: it.color || '#1f5fbe',
        label: it.label,
        value: it.value,
      }
    }
    const end = start + frac * Math.PI * 2
    const p1 = toPoint(start)
    const p2 = toPoint(end)
    const large = end - start > Math.PI ? 1 : 0
    const d = `M ${p1.x} ${p1.y} A ${r} ${r} 0 ${large} 1 ${p2.x} ${p2.y}`
    const out = { type: 'arc', d, color: it.color || '#1f5fbe', label: it.label, value: it.value }
    start = end
    return out
  })
  return (
    <div className="distribution-wrap">
      <div className="distribution-donut">
        <svg viewBox="0 0 240 240" className="distribution-svg">
          <circle cx={cx} cy={cy} r={r} fill="none" className="distribution-track" strokeWidth={stroke} />
          {arcs.map((a, i) => (
            a.type === 'full'
              ? (
                <circle
                  key={`arc-full-${i}`}
                  cx={cx}
                  cy={cy}
                  r={r}
                  fill="none"
                  stroke={a.color}
                  strokeWidth={stroke}
                  strokeLinecap="round"
                />
              )
              : (a.type === 'arc'
                ? (
                  <path key={`arc-${i}`} d={a.d} fill="none" stroke={a.color} strokeWidth={stroke} strokeLinecap="round" />
                )
                : null)
          ))}
        </svg>
        <div className="distribution-center">
          <small>总资产</small>
          <b>{fmtNum(total, 2)}</b>
          <span>USDT</span>
        </div>
      </div>
      <div className="distribution-legend">
        {safeItems.map((it, i) => {
          const pct = (Number(it.value || 0) / total) * 100
          return (
            <div key={`legend-${i}`} className="legend-item">
              <span className="legend-dot" style={{ backgroundColor: it.color || '#1f5fbe' }} />
              <div className="legend-main">
                <span>{it.label}</span>
                <b>{fmtNum(it.value, 2)} ({fmtPct(pct)})</b>
                <div className="legend-bar">
                  <i style={{ width: `${Math.max(pct, 2)}%`, backgroundColor: it.color || '#1f5fbe' }} />
                </div>
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
