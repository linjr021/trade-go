import { useEffect, useMemo, useRef, useState } from 'react'
import {
  getAccount,
  getAssetOverview,
  getAssetPnLCalendar,
  getAssetDistribution,
  getAssetTrend,
  getIntegrations,
  addExchangeIntegration,
  addLLMIntegration,
  getSignals,
  getTradeRecords,
  getStatus,
  getStrategyTemplate,
  getStrategies,
  getStrategyScores,
  getPromptSettings,
  getSystemSettings,
  generateStrategyPreference,
  resetPromptSettings,
  runBacktestApi,
  getBacktestHistory,
  getBacktestHistoryDetail,
  deleteBacktestHistory,
  runNow,
  savePromptSettings,
  saveSystemSettings,
  startScheduler,
  stopScheduler,
  uploadStrategyFile,
  updateSettings,
} from './api'

const MENU_ITEMS = [
  { key: 'assets', label: '资产详情' },
  { key: 'live', label: '实盘交易' },
  { key: 'paper', label: '模拟交易' },
  { key: 'builder', label: '策略生成' },
  { key: 'backtest', label: '历史回测' },
  { key: 'system', label: '系统设置' },
]

const HABIT_OPTIONS = ['10m', '1h', '4h', '1D', '5D', '30D', '90D']
const PAIRS = ['BTCUSDT', 'ETHUSDT', 'BNBUSDT', 'SOLUSDT', 'XRPUSDT']

const envFieldGroups = [
  {
    title: '运行配置',
    fields: [
      { key: 'PRODUCT_NAME', label: '产品名称' },
      { key: 'PY_STRATEGY_URL', label: 'Python 策略服务 URL' },
      { key: 'MODE', label: '运行模式（prod/test/dev）' },
      { key: 'HTTP_ADDR', label: '服务地址（:8080）' },
    ],
  },
]

const envFieldDefs = envFieldGroups.flatMap((group) => group.fields)

const systemSettingDefaults = {
  PRODUCT_NAME: 'AI 交易看板',
  PY_STRATEGY_URL: 'http://127.0.0.1:9000',
  PY_STRATEGY_ENABLED: 'ai_assisted,trend_following,mean_reversion,breakout',
}

const legacyEmptyLikeModels = new Set(['chat-model'])

const promptSettingDefaults = {
  trading_ai_system_prompt: `你是加密永续量化交易决策引擎，交易标的默认 ${'${symbol}'}。
你必须遵守：
1) 仅输出严格JSON；
2) 先判断市场状态（trend/range/breakout）再给信号；
3) 信号不充分或冲突时优先HOLD；
4) 你只负责方向、入场区、止损、止盈、盈亏比（盈利/亏损）；仓位由风控引擎执行；
5) 不得决定固定下单金额、固定仓位和固定杠杆；这些均由实盘设置与风控引擎执行。`,
  trading_ai_policy_prompt: `硬边界：
1) 只能输出方向/入场区间/止损/止盈/盈亏比（盈利/亏损），不输出固定开仓金额或固定杠杆。
2) 最低盈亏比（盈利/亏损）门槛：盈亏比 = |TP-Entry| / |Entry-SL| >= 1.5，推荐>=2.0。
3) 若关键条件不足或冲突，必须输出HOLD并给出触发条件。
4) 非高信心不允许频繁反转，优先顺势交易。
5) 输出必须包含：首选策略、备选策略、入场区间、止损、目标、盈亏比估算。`,
  strategy_generator_prompt_template: `你是资深量化策略研究员。请为 ${'${symbol}'} 在 ${'${habit}'} 交易习惯下生成一套可执行自动策略。
按以下结构输出：

1) 市场状态识别
- 趋势/震荡/突破判定规则（必须可量化）

2) 关键位定义
- 支撑/阻力计算逻辑与技术含义

3) 入场与出场
- 首选策略（触发条件、入场区间、SL、TP）
- 备选策略（触发条件、入场区间、SL、TP）

4) 风险管理（硬约束）
- 仓位/杠杆由实盘执行参数与风控引擎统一决定（策略中不固定金额）
- 可给出风险预算公式，但不得写死固定金额阈值
- 最小盈亏比（盈利/亏损）要求：目标>=2.0（最低1.5）

5) 观望与失效条件
- 明确“什么情况下不交易”
- 明确“策略何时失效需停用”

6) 回测建议
- 推荐回测区间、周期、指标、评估口径（总盈亏、胜率、盈亏比、回撤）`,
}

const strategyTemplateFallback = `"""Sample custom strategy.

Rename/copy this file and adjust logic.
"""

STRATEGY_ID = "sample_custom"


def analyze(payload, features=None):
    # payload: raw request body from Go
    # features: extracted metrics (price/rsi/macd/atr_ratio/...)
    price = 0.0
    if isinstance(features, dict):
        price = float(features.get("price", 0.0) or 0.0)

    if price <= 0:
        return {
            "signal": "HOLD",
            "reason": "invalid price",
            "stop_loss": 0,
            "take_profit": 0,
            "confidence": "LOW",
            "strategy_combo": STRATEGY_ID,
        }

    # Replace with your own logic
    return {
        "signal": "HOLD",
        "reason": "template strategy: no entry",
        "stop_loss": round(price * 0.99, 4),
        "take_profit": round(price * 1.01, 4),
        "confidence": "LOW",
        "strategy_combo": STRATEGY_ID,
    }
`

function fmtNum(v, digits = 2) {
  const n = Number(v)
  if (Number.isNaN(n)) return '-'
  return n.toFixed(digits)
}

function fmtPct(v) {
  const n = Number(v)
  if (Number.isNaN(n)) return '-'
  return `${n.toFixed(2)}%`
}

function fmtTime(value) {
  if (!value) return '-'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return '-'
  return d.toLocaleString()
}

function clamp(value, min, max) {
  const n = Number(value)
  if (Number.isNaN(n)) return min
  return Math.min(max, Math.max(min, n))
}

function countHanChars(value) {
  const text = String(value || '')
  const hits = text.match(/[\u3400-\u4DBF\u4E00-\u9FFF\uF900-\uFAFF]/g)
  return hits ? hits.length : 0
}

function MonthSelect({ value, onChange, min = '2018-01', max = '2025-12' }) {
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
      <select
        className="month-select year"
        value={String(curY)}
        onChange={(e) => apply(Number(e.target.value), curM)}
      >
        {years.map((y) => (
          <option key={`y-${y}`} value={String(y)}>{y}年</option>
        ))}
      </select>
      <select
        className="month-select month"
        value={String(curM)}
        onChange={(e) => apply(curY, Number(e.target.value))}
      >
        {Array.from({ length: endM - startM + 1 }, (_, i) => startM + i).map((m) => (
          <option key={`m-${m}`} value={String(m)}>{m}月</option>
        ))}
      </select>
    </div>
  )
}

function mergeSystemDefaults(raw) {
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

function parseStrategies(raw = []) {
  if (!Array.isArray(raw)) return []
  return raw
    .map((x) => String(x || '').trim())
    .filter(Boolean)
}

function parseBinanceKlines(raw) {
  if (!Array.isArray(raw)) return []
  return raw
    .map((row) => {
      if (!Array.isArray(row) || row.length < 6) return null
      return {
        ts: Number(row[0]),
        open: Number(row[1]),
        high: Number(row[2]),
        low: Number(row[3]),
        close: Number(row[4]),
        volume: Number(row[5]),
      }
    })
    .filter((x) => x && x.high > 0 && x.low > 0)
}

function KlineChart({ data }) {
  if (!data.length) return <p className="muted">暂无K线</p>

  const width = 920
  const height = 260
  const pad = 16
  const innerW = width - pad * 2
  const innerH = height - pad * 2
  const highMax = Math.max(...data.map((k) => k.high))
  const lowMin = Math.min(...data.map((k) => k.low))
  const span = Math.max(highMax - lowMin, 1e-9)
  const step = innerW / data.length
  const bodyW = Math.max(2, step * 0.58)

  const x = (i) => pad + step * i + step / 2
  const y = (p) => pad + ((highMax - p) / span) * innerH

  return (
    <svg viewBox={`0 0 ${width} ${height}`} className="kline-svg" role="img" aria-label="kline">
      <rect x="0" y="0" width={width} height={height} rx="12" fill="#f9fafc" />
      {data.map((k, i) => {
        const color = k.close >= k.open ? '#0f996e' : '#d04d4d'
        const bodyTop = Math.min(y(k.open), y(k.close))
        const bodyH = Math.max(2, Math.abs(y(k.close) - y(k.open)))
        return (
          <g key={`${k.ts}-${i}`}>
            <line x1={x(i)} y1={y(k.high)} x2={x(i)} y2={y(k.low)} stroke={color} strokeWidth="1.2" />
            <rect x={x(i) - bodyW / 2} y={bodyTop} width={bodyW} height={bodyH} fill={color} rx="1" />
          </g>
        )
      })}
    </svg>
  )
}

function AssetTrendChart({ points }) {
  if (!points.length) return <p className="muted">暂无资产趋势数据</p>
  const width = 920
  const height = 240
  const pad = 18
  const innerW = width - pad * 2
  const innerH = height - pad * 2
  const maxV = Math.max(...points.map((x) => Number(x.equity || 0)))
  const minV = Math.min(...points.map((x) => Number(x.equity || 0)))
  const span = Math.max(maxV - minV, 1e-9)
  const step = points.length > 1 ? innerW / (points.length - 1) : innerW
  const y = (v) => pad + ((maxV - v) / span) * innerH
  const path = points
    .map((p, i) => `${i === 0 ? 'M' : 'L'} ${pad + step * i} ${y(Number(p.equity || 0))}`)
    .join(' ')
  return (
    <svg viewBox={`0 0 ${width} ${height}`} className="kline-svg" role="img" aria-label="equity-trend">
      <rect x="0" y="0" width={width} height={height} rx="12" fill="#f9fafc" />
      <path d={path} fill="none" stroke="#1f5fbe" strokeWidth="2.2" />
    </svg>
  )
}

function PnLCalendar({ month, days }) {
  const [y, m] = month.split('-').map((x) => Number(x))
  if (!y || !m) return <p className="muted">月份格式错误</p>
  const totalDays = new Date(y, m, 0).getDate()
  const firstWeekday = new Date(y, m - 1, 1).getDay()
  const map = new Map((days || []).map((d) => [d.date, d]))
  const cells = []
  for (let i = 0; i < firstWeekday; i += 1) cells.push(null)
  for (let d = 1; d <= totalDays; d += 1) {
    const date = `${y}-${String(m).padStart(2, '0')}-${String(d).padStart(2, '0')}`
    cells.push({ day: d, item: map.get(date) || null })
  }
  return (
    <div className="calendar-grid">
      {['日', '一', '二', '三', '四', '五', '六'].map((w) => (
        <div key={w} className="calendar-head">{w}</div>
      ))}
      {cells.map((c, i) => (
        <div key={`cell-${i}`} className={`calendar-cell ${!c ? 'empty' : ''}`}>
          {c ? (
            <>
              <div className="calendar-day">{c.day}</div>
              <div className={Number(c.item?.pnl_amount || 0) >= 0 ? 'calendar-pnl up' : 'calendar-pnl down'}>
                {c.item ? fmtNum(c.item.pnl_amount, 2) : '-'}
              </div>
            </>
          ) : null}
        </div>
      ))}
    </div>
  )
}

function AssetDistributionChart({ items }) {
  const safeItems = (items || []).filter((x) => Number(x?.value || 0) > 0)
  if (!safeItems.length) return <p className="muted">暂无资产分布数据</p>
  const total = safeItems.reduce((s, x) => s + Number(x.value || 0), 0) || 1
  let start = -Math.PI / 2
  const cx = 120
  const cy = 120
  const r = 86
  const stroke = 28
  const toPoint = (angle) => ({ x: cx + r * Math.cos(angle), y: cy + r * Math.sin(angle) })
  const arcs = safeItems.map((it) => {
    const frac = Number(it.value || 0) / total
    const end = start + frac * Math.PI * 2
    const p1 = toPoint(start)
    const p2 = toPoint(end)
    const large = end-start > Math.PI ? 1 : 0
    const d = `M ${p1.x} ${p1.y} A ${r} ${r} 0 ${large} 1 ${p2.x} ${p2.y}`
    const out = { d, color: it.color || '#1f5fbe', label: it.label, value: it.value }
    start = end
    return out
  })
  return (
    <div className="distribution-wrap">
      <svg viewBox="0 0 240 240" className="distribution-svg">
        <circle cx={cx} cy={cy} r={r} fill="none" stroke="#e5ecf4" strokeWidth={stroke} />
        {arcs.map((a, i) => (
          <path key={`arc-${i}`} d={a.d} fill="none" stroke={a.color} strokeWidth={stroke} strokeLinecap="round" />
        ))}
      </svg>
      <div className="distribution-legend">
        {safeItems.map((it, i) => {
          const pct = (Number(it.value || 0) / total) * 100
          return (
            <div key={`legend-${i}`} className="legend-item">
              <span className="legend-dot" style={{ backgroundColor: it.color || '#1f5fbe' }} />
              <span>{it.label}</span>
              <b>{fmtNum(it.value, 2)} ({fmtPct(pct)})</b>
            </div>
          )
        })}
      </div>
    </div>
  )
}

function StrategyBacktestTable({ records }) {
  if (!records.length) return <p className="muted">暂无模拟记录</p>
  return (
    <div className="table-wrap">
      <table>
        <thead>
          <tr>
            <th>时间</th>
            <th>方向</th>
            <th>信心</th>
            <th>张数</th>
            <th>杠杆</th>
            <th>入场</th>
            <th>出场</th>
            <th>盈亏</th>
          </tr>
        </thead>
        <tbody>
          {records.map((r) => (
            <tr key={r.id}>
              <td>{fmtTime(r.ts)}</td>
              <td>{r.side}</td>
              <td>{r.confidence || '-'}</td>
              <td>{r.size === undefined || r.size === null ? '-' : fmtNum(r.size, 4)}</td>
              <td>{r.leverage ? `${r.leverage}x` : '-'}</td>
              <td>{fmtNum(r.entry, 2)}</td>
              <td>{fmtNum(r.exit, 2)}</td>
              <td className={r.pnl >= 0 ? 'up' : 'down'}>{fmtNum(r.pnl, 2)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function TradeRecordsTable({ records }) {
  if (!records.length) return <p className="muted">暂无交易记录</p>
  return (
    <div className="table-wrap">
      <table>
        <thead>
          <tr>
            <th>时间</th>
            <th>交易对</th>
            <th>方向</th>
            <th>开单</th>
            <th>数量</th>
            <th>价格</th>
            <th>盈亏</th>
          </tr>
        </thead>
        <tbody>
          {records.map((r) => (
            <tr key={r.id || `${r.ts}-${r.symbol}-${r.signal}`}>
              <td>{fmtTime(r.ts)}</td>
              <td>{r.symbol || '-'}</td>
              <td>{String(r.signal || '-').toUpperCase()}</td>
              <td>{r.approved ? '已开单' : '未开单'}</td>
              <td>{fmtNum(r.approved_size, 4)}</td>
              <td>{fmtNum(r.price, 2)}</td>
              <td className={Number(r.unrealized_pnl || 0) >= 0 ? 'up' : 'down'}>
                {fmtNum(r.unrealized_pnl, 2)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

export default function App() {
  const [menu, setMenu] = useState('live')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const [status, setStatus] = useState({})
  const [account, setAccount] = useState({})
  const [signals, setSignals] = useState([])
  const [tradeRecords, setTradeRecords] = useState([])
  const [strategyScores, setStrategyScores] = useState([])

  const [strategyOptions, setStrategyOptions] = useState(['ai_assisted', 'trend_following', 'mean_reversion', 'breakout'])
  const [enabledStrategies, setEnabledStrategies] = useState(['ai_assisted'])
  const [activeStrategy, setActiveStrategy] = useState('ai_assisted')
  const [paperStrategy, setPaperStrategy] = useState('ai_assisted')
  const [activePair, setActivePair] = useState('BTCUSDT')
  const [paperPair, setPaperPair] = useState('BTCUSDT')

  const [liveKline, setLiveKline] = useState([])
  const [paperKline, setPaperKline] = useState([])
  const [klineUpdatedAt, setKlineUpdatedAt] = useState('')
  const [liveViewTab, setLiveViewTab] = useState('overview')
  const [paperViewTab, setPaperViewTab] = useState('overview')
  const [strategyPickerOpen, setStrategyPickerOpen] = useState(false)
  const [strategyDraft, setStrategyDraft] = useState([])
  const strategyPickerRef = useRef(null)

  const [settings, setSettings] = useState({
    positionSizingMode: 'contracts',
    highConfidenceAmount: 0.01,
    lowConfidenceAmount: 0.005,
    highConfidenceMarginPct: 10,
    lowConfidenceMarginPct: 5,
    leverage: 10,
  })
  const [systemSettings, setSystemSettings] = useState({ ...systemSettingDefaults })
  const [systemSubTab, setSystemSubTab] = useState('env')
  const [llmConfigs, setLlmConfigs] = useState([])
  const [exchangeConfigs, setExchangeConfigs] = useState([])
  const [addingLLM, setAddingLLM] = useState(false)
  const [addingExchange, setAddingExchange] = useState(false)
  const [showLLMModal, setShowLLMModal] = useState(false)
  const [showExchangeModal, setShowExchangeModal] = useState(false)
  const [newLLM, setNewLLM] = useState({ name: '', base_url: '', api_key: '', model: '' })
  const [newExchange, setNewExchange] = useState({
    name: '',
    exchange: 'binance',
    api_key: '',
    secret: '',
    passphrase: '',
  })
  const [promptSettings, setPromptSettings] = useState({
    trading_ai_system_prompt: '',
    trading_ai_policy_prompt: '',
    strategy_generator_prompt_template: '',
  })
  const [paperMargin, setPaperMargin] = useState(200)

  const [runningNow, setRunningNow] = useState(false)
  const [savingSettings, setSavingSettings] = useState(false)
  const [savingSystemSettings, setSavingSystemSettings] = useState(false)
  const [systemSaveHint, setSystemSaveHint] = useState('')
  const [toast, setToast] = useState({ visible: false, type: 'success', message: '' })
  const [savingPrompts, setSavingPrompts] = useState(false)
  const [resettingPrompts, setResettingPrompts] = useState(false)

  const [builderTab, setBuilderTab] = useState('generate')
  const [habit, setHabit] = useState('1h')
  const [genPair, setGenPair] = useState('BTCUSDT')
  const [genStyle, setGenStyle] = useState('hybrid')
  const [genMinRR, setGenMinRR] = useState(2.0)
  const [genAllowReversal, setGenAllowReversal] = useState(false)
  const [genLowConfAction, setGenLowConfAction] = useState('hold')
  const [genDirectionBias, setGenDirectionBias] = useState('balanced')
  const [generatedStrategies, setGeneratedStrategies] = useState([])
  const [selectedRuleId, setSelectedRuleId] = useState('')
  const [renameRuleName, setRenameRuleName] = useState('')
  const [uploadFile, setUploadFile] = useState(null)
  const [uploadingStrategy, setUploadingStrategy] = useState(false)
  const [strategyTemplate, setStrategyTemplate] = useState('')
  const [loadingTemplate, setLoadingTemplate] = useState(false)
  const [generatingStrategy, setGeneratingStrategy] = useState(false)

  const [btStrategy, setBtStrategy] = useState('')
  const [btPair, setBtPair] = useState('BTCUSDT')
  const [btInitialMargin, setBtInitialMargin] = useState(1000)
  const [btLeverage, setBtLeverage] = useState(10)
  const [btPositionSizingMode, setBtPositionSizingMode] = useState('contracts')
  const [btHighConfidenceAmount, setBtHighConfidenceAmount] = useState(0.01)
  const [btLowConfidenceAmount, setBtLowConfidenceAmount] = useState(0.005)
  const [btHighConfidenceMarginPct, setBtHighConfidenceMarginPct] = useState(10)
  const [btLowConfidenceMarginPct, setBtLowConfidenceMarginPct] = useState(5)
  const [btStart, setBtStart] = useState('2021-01')
  const [btEnd, setBtEnd] = useState('2024-12')
  const [btRunning, setBtRunning] = useState(false)
  const [btSummary, setBtSummary] = useState(null)
  const [btRecords, setBtRecords] = useState([])
  const [btHistory, setBtHistory] = useState([])
  const [btHistoryLoading, setBtHistoryLoading] = useState(false)
  const [btHistoryDeletingId, setBtHistoryDeletingId] = useState(0)
  const [btHistorySelectedId, setBtHistorySelectedId] = useState(0)

  const [liveStrategyStartedAt, setLiveStrategyStartedAt] = useState(Date.now())
  const [assetRange, setAssetRange] = useState('30D')
  const [assetMonth, setAssetMonth] = useState(new Date().toISOString().slice(0, 7))
  const [assetOverview, setAssetOverview] = useState({})
  const [assetTrend, setAssetTrend] = useState([])
  const [assetCalendar, setAssetCalendar] = useState([])
  const [assetDistribution, setAssetDistribution] = useState([])

  const schedulerRunning = Boolean(status?.scheduler_running)
  const productName = String(systemSettings?.PRODUCT_NAME || '').trim() || 'AI 交易看板'
  const generatedStrategyNames = useMemo(
    () => generatedStrategies.map((s) => String(s?.name || '').trim()).filter(Boolean),
    [generatedStrategies],
  )
  const executionStrategyOptions = useMemo(
    () => Array.from(new Set([...strategyOptions, ...generatedStrategyNames])),
    [strategyOptions, generatedStrategyNames],
  )
  const selectedStrategyText = enabledStrategies.length ? enabledStrategies.join(', ') : '请选择策略'
  const liveStrategyLabel = enabledStrategies.length ? enabledStrategies.join(' / ') : (activeStrategy || '-')

  const refreshCore = async (silent = false) => {
    if (!silent) {
      setLoading(true)
      setError('')
    }
    try {
      const [tradeRes, statusRes, accountRes, signalRes, scoreRes] = await Promise.all([
        getTradeRecords(60),
        getStatus(),
        getAccount(),
        getSignals(40),
        getStrategyScores(40),
      ])
      setTradeRecords(tradeRes?.data?.records || [])
      const st = statusRes.data || {}
      setStatus(st)
      setAccount(accountRes.data || {})
      setSignals([...(signalRes?.data?.signals || [])].reverse())
      setStrategyScores(scoreRes?.data?.scores || st?.strategy_scores || [])

      const cfg = st?.trade_config || {}
      setSettings((old) => ({
        ...old,
        positionSizingMode: String(cfg.position_sizing_mode || old.positionSizingMode || 'contracts'),
        highConfidenceAmount: Number(cfg.high_confidence_amount || old.highConfidenceAmount || 0.01),
        lowConfidenceAmount: Number(cfg.low_confidence_amount || old.lowConfidenceAmount || 0.005),
        highConfidenceMarginPct: Number(cfg.high_confidence_margin_pct || 0.1) * 100,
        lowConfidenceMarginPct: Number(cfg.low_confidence_margin_pct || 0.05) * 100,
        leverage: Number(cfg.leverage || old.leverage || 10),
      }))
      if (cfg?.symbol) {
        const symbol = String(cfg.symbol).toUpperCase()
        setActivePair(symbol)
        setPaperPair((p) => p || symbol)
      }
    } catch (e) {
      if (!silent) setError(e?.response?.data?.error || e?.message || '请求失败')
    } finally {
      if (!silent) setLoading(false)
    }
  }

  const paperTradeRecords = useMemo(() => {
    if (Array.isArray(btRecords) && btRecords.length) {
      return btRecords.map((r, i) => ({
        id: `bt-${r.id || i}`,
        ts: r.ts,
        symbol: paperPair,
        signal: r.side,
        approved: true,
        approved_size: 0,
        price: r.entry,
        unrealized_pnl: r.pnl,
      }))
    }
    return tradeRecords.map((r) => ({
      ...r,
      symbol: r.symbol || paperPair,
    }))
  }, [btRecords, tradeRecords, paperPair])

  useEffect(() => {
    setStrategyDraft(enabledStrategies)
  }, [enabledStrategies])

  useEffect(() => {
    if (!strategyPickerOpen) return undefined
    const onDocMouseDown = (e) => {
      if (strategyPickerRef.current && !strategyPickerRef.current.contains(e.target)) {
        setStrategyPickerOpen(false)
      }
    }
    const onDocKeyDown = (e) => {
      if (e.key === 'Escape') setStrategyPickerOpen(false)
    }
    document.addEventListener('mousedown', onDocMouseDown)
    document.addEventListener('keydown', onDocKeyDown)
    return () => {
      document.removeEventListener('mousedown', onDocMouseDown)
      document.removeEventListener('keydown', onDocKeyDown)
    }
  }, [strategyPickerOpen])

  const loadSystemAndStrategies = async () => {
    const [sysRes, strategyRes, promptRes, integrationRes] = await Promise.allSettled([
      getSystemSettings(),
      getStrategies(),
      getPromptSettings(),
      getIntegrations(),
    ])

    let merged = mergeSystemDefaults(systemSettings)
    if (sysRes.status === 'fulfilled') {
      merged = mergeSystemDefaults(sysRes.value?.data?.settings || {})
    }
    setSystemSettings(merged)

    if (promptRes.status === 'fulfilled') {
      const prompts = promptRes.value?.data?.prompts || {}
      setPromptSettings({
        trading_ai_system_prompt: String(
          prompts.trading_ai_system_prompt || promptSettingDefaults.trading_ai_system_prompt
        ),
        trading_ai_policy_prompt: String(
          prompts.trading_ai_policy_prompt || promptSettingDefaults.trading_ai_policy_prompt
        ),
        strategy_generator_prompt_template: String(
          prompts.strategy_generator_prompt_template || promptSettingDefaults.strategy_generator_prompt_template
        ),
      })
    } else {
      setPromptSettings((v) => ({
        trading_ai_system_prompt:
          String(v.trading_ai_system_prompt || '').trim() || promptSettingDefaults.trading_ai_system_prompt,
        trading_ai_policy_prompt:
          String(v.trading_ai_policy_prompt || '').trim() || promptSettingDefaults.trading_ai_policy_prompt,
        strategy_generator_prompt_template:
          String(v.strategy_generator_prompt_template || '').trim() ||
          promptSettingDefaults.strategy_generator_prompt_template,
      }))
    }

    if (strategyRes.status === 'fulfilled') {
      const available = parseStrategies(strategyRes.value?.data?.available)
      const enabled = parseStrategies(strategyRes.value?.data?.enabled)
      const byEnv = parseStrategies(String(merged.PY_STRATEGY_ENABLED || '').split(','))
      const mergedSet = Array.from(new Set([...available, ...enabled, ...byEnv]))
      const mergedExecution = Array.from(new Set([...mergedSet, ...generatedStrategyNames]))
      const enabledMerged = Array.from(new Set([...enabled, ...byEnv])).filter((x) => mergedExecution.includes(x))
      if (mergedSet.length) {
        setStrategyOptions(mergedSet)
        if (!mergedExecution.includes(activeStrategy)) setActiveStrategy(mergedExecution[0])
        if (!mergedExecution.includes(paperStrategy)) setPaperStrategy(mergedExecution[0])
        setEnabledStrategies((enabledMerged.length ? enabledMerged : [mergedExecution[0]]).slice(0, 3))
      }
    }
    if (integrationRes.status === 'fulfilled') {
      setLlmConfigs(Array.isArray(integrationRes.value?.data?.llms) ? integrationRes.value.data.llms : [])
      setExchangeConfigs(Array.isArray(integrationRes.value?.data?.exchanges) ? integrationRes.value.data.exchanges : [])
    }
  }

  async function fetchKline(symbol, setter) {
    try {
      const res = await fetch(`https://api.binance.com/api/v3/klines?symbol=${symbol}&interval=1m&limit=90`)
      const raw = await res.json()
      setter(parseBinanceKlines(raw))
      setKlineUpdatedAt(new Date().toISOString())
    } catch {
      // ignore transient kline errors
    }
  }

  useEffect(() => {
    refreshCore(false)
    loadSystemAndStrategies()
    loadStrategyTemplate()
    const timer = setInterval(() => refreshCore(true), 15000)
    return () => clearInterval(timer)
  }, [])

  useEffect(() => {
    fetchKline(activePair, setLiveKline)
    const timer = setInterval(() => fetchKline(activePair, setLiveKline), 1000)
    return () => clearInterval(timer)
  }, [activePair])

  useEffect(() => {
    fetchKline(paperPair, setPaperKline)
    const timer = setInterval(() => fetchKline(paperPair, setPaperKline), 1000)
    return () => clearInterval(timer)
  }, [paperPair])

  useEffect(() => {
    if (!toast.visible) return undefined
    const timer = setTimeout(() => {
      setToast((t) => ({ ...t, visible: false }))
    }, 2600)
    return () => clearTimeout(timer)
  }, [toast.visible, toast.message, toast.type])

  const showToast = (type, message) => {
    setToast({ visible: true, type, message: String(message || '') })
  }

  const loadAssetOverview = async () => {
    try {
      const res = await getAssetOverview()
      setAssetOverview(res?.data?.overview || {})
    } catch {
      // keep previous data
    }
  }

  const loadAssetTrend = async (range) => {
    try {
      const res = await getAssetTrend(range)
      setAssetTrend(Array.isArray(res?.data?.points) ? res.data.points : [])
    } catch {
      setAssetTrend([])
    }
  }

  const loadAssetCalendar = async (month) => {
    try {
      const res = await getAssetPnLCalendar(month)
      setAssetCalendar(Array.isArray(res?.data?.days) ? res.data.days : [])
    } catch {
      setAssetCalendar([])
    }
  }

  const loadAssetDistribution = async () => {
    try {
      const res = await getAssetDistribution()
      setAssetDistribution(Array.isArray(res?.data?.items) ? res.data.items : [])
    } catch {
      setAssetDistribution([])
    }
  }

  const joinFieldMessages = (fieldMap) => {
    if (!fieldMap || typeof fieldMap !== 'object') return ''
    const parts = Object.entries(fieldMap)
      .filter(([k, v]) => String(k || '').trim() !== '' && String(v || '').trim() !== '')
      .map(([k, v]) => `[${k}] ${v}`)
    return parts.join('；')
  }

  const mapBacktestSummary = (summary) => {
    const wins = Number(summary?.wins || 0)
    const losses = Number(summary?.losses || 0)
    const rawRatio = Number(summary?.ratio)
    const ratio = Number.isFinite(rawRatio) ? rawRatio : 0
    const ratioInfinite = Boolean(summary?.ratio_infinite) || (losses === 0 && wins > 0)
    return {
      historyId: Number(summary?.history_id || summary?.id || 0),
      createdAt: String(summary?.created_at || ''),
      strategy: summary?.strategy || btStrategy || '-',
      pair: summary?.pair || btPair,
      habit: summary?.habit || habit,
      start: summary?.start || btStart,
      end: summary?.end || btEnd,
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

  const loadBacktestHistory = async (silent = false) => {
    if (!silent) setBtHistoryLoading(true)
    try {
      const res = await getBacktestHistory(120)
      setBtHistory(Array.isArray(res?.data?.runs) ? res.data.runs : [])
    } catch {
      if (!silent) setBtHistory([])
    } finally {
      if (!silent) setBtHistoryLoading(false)
    }
  }

  useEffect(() => {
    if (menu !== 'assets') return
    loadAssetOverview()
    loadAssetTrend(assetRange)
    loadAssetCalendar(assetMonth)
    loadAssetDistribution()
  }, [menu])

  useEffect(() => {
    if (menu !== 'assets') return
    loadAssetTrend(assetRange)
  }, [assetRange, menu])

  useEffect(() => {
    if (menu !== 'assets') return
    loadAssetCalendar(assetMonth)
  }, [assetMonth, menu])

  useEffect(() => {
    if (menu !== 'backtest') return
    loadBacktestHistory(false)
  }, [menu])

  const marketEmotion = useMemo(() => {
    const confidence = String(status?.runtime?.last_signal?.confidence || 'LOW').toUpperCase()
    const change = Number(status?.runtime?.last_price?.price_change || 0)
    if (confidence === 'HIGH' && change >= 0) return '偏多'
    if (confidence === 'HIGH' && change < 0) return '偏空'
    if (Math.abs(change) < 0.15) return '中性'
    return change > 0 ? '温和偏多' : '温和偏空'
  }, [status])

  const totalPnL = Number(account?.position?.unrealized_pnl || 0)

  const pnlRatio = useMemo(() => {
    const wins = strategyScores.reduce((a, b) => a + Number(b?.wins || 0), 0)
    const losses = strategyScores.reduce((a, b) => a + Number(b?.losses || 0), 0)
    if (losses === 0) return wins > 0 ? '∞' : '0'
    return (wins / losses).toFixed(2)
  }, [strategyScores])

  const strategyDurationText = useMemo(() => {
    const diff = Math.max(0, Date.now() - liveStrategyStartedAt)
    const mins = Math.floor(diff / 60000)
    const hours = Math.floor(mins / 60)
    const rem = mins % 60
    if (hours > 0) return `${hours}h ${rem}m`
    return `${mins}m`
  }, [liveStrategyStartedAt, status?.runtime?.last_run_at])

  const saveLiveConfig = async () => {
    setSavingSettings(true)
    setError('')
    try {
      await updateSettings({
        symbol: String(activePair || 'BTCUSDT').toUpperCase(),
        position_sizing_mode: String(settings.positionSizingMode || 'contracts'),
        high_confidence_amount: clamp(settings.highConfidenceAmount, 0, 1000000),
        low_confidence_amount: clamp(settings.lowConfidenceAmount, 0, 1000000),
        high_confidence_margin_pct: clamp(settings.highConfidenceMarginPct, 0, 100) / 100,
        low_confidence_margin_pct: clamp(settings.lowConfidenceMarginPct, 0, 100) / 100,
        leverage: Math.round(clamp(settings.leverage, 1, 150)),
      })

      const normalized = enabledStrategies.filter((x) => executionStrategyOptions.includes(x))
      const fallbackStrategy = executionStrategyOptions.includes(activeStrategy)
        ? activeStrategy
        : (executionStrategyOptions[0] || '')
      const withFallback = normalized.length
        ? normalized
        : (fallbackStrategy ? [fallbackStrategy] : [])
      if (fallbackStrategy && !withFallback.includes(fallbackStrategy)) withFallback.push(fallbackStrategy)
      const nextEnabled = Array.from(new Set(withFallback)).slice(0, 3)
      const merged = { ...systemSettings, PY_STRATEGY_ENABLED: nextEnabled.join(',') }
      await saveSystemSettings(merged)
      setSystemSettings(merged)
      setLiveStrategyStartedAt(Date.now())
      await refreshCore(false)
      showToast('success', '实盘配置保存成功')
    } catch (e) {
      const reason = e?.response?.data?.error || e?.message || '保存失败'
      setError(reason)
      showToast('error', `保存失败：${reason}`)
    } finally {
      setSavingSettings(false)
    }
  }

  const saveSystemEnv = async () => {
    setSavingSystemSettings(true)
    setSystemSaveHint('')
    setError('')
    try {
      const payload = {}
      for (const f of envFieldDefs) payload[f.key] = String(systemSettings?.[f.key] || '')
      const res = await saveSystemSettings(payload)
      setSystemSettings(mergeSystemDefaults(res?.data?.settings || payload))
      setSystemSaveHint(`已保存 ${new Date().toLocaleTimeString()}`)
      const warnMsg = joinFieldMessages(res?.data?.warnings)
      if (warnMsg) {
        showToast('warning', `系统设置已保存，但存在告警：${warnMsg}`)
      } else {
        showToast('success', '系统设置保存成功')
      }
    } catch (e) {
      const base = e?.response?.data?.error || e?.message || '系统设置保存失败'
      const detail = joinFieldMessages(e?.response?.data?.field_errors)
      const reason = detail ? `${base}：${detail}` : base
      setSystemSaveHint('')
      setError(reason)
      showToast('error', `保存失败：${reason}`)
    } finally {
      setSavingSystemSettings(false)
    }
  }

  const savePromptConfig = async () => {
    setSavingPrompts(true)
    setError('')
    try {
      const payload = {
        trading_ai_system_prompt: String(promptSettings.trading_ai_system_prompt || ''),
        trading_ai_policy_prompt: String(promptSettings.trading_ai_policy_prompt || ''),
        strategy_generator_prompt_template: String(promptSettings.strategy_generator_prompt_template || ''),
      }
      const res = await savePromptSettings(payload)
      const prompts = res?.data?.prompts || payload
      setPromptSettings({
        trading_ai_system_prompt: String(prompts.trading_ai_system_prompt || ''),
        trading_ai_policy_prompt: String(prompts.trading_ai_policy_prompt || ''),
        strategy_generator_prompt_template: String(prompts.strategy_generator_prompt_template || ''),
      })
      showToast('success', '提示词保存成功')
    } catch (e) {
      const reason = e?.response?.data?.error || e?.message || '提示词保存失败'
      setError(reason)
      showToast('error', `保存失败：${reason}`)
    } finally {
      setSavingPrompts(false)
    }
  }

  const handleAddLLM = async () => {
    setAddingLLM(true)
    setError('')
    try {
      const payload = {
        name: String(newLLM.name || '').trim(),
        base_url: String(newLLM.base_url || '').trim(),
        api_key: String(newLLM.api_key || '').trim(),
        model: String(newLLM.model || '').trim(),
      }
      const res = await addLLMIntegration(payload)
      setLlmConfigs(Array.isArray(res?.data?.llms) ? res.data.llms : [])
      setNewLLM({ name: '', base_url: '', api_key: '', model: '' })
      setShowLLMModal(false)
      showToast('success', `LLM 参数添加并验证成功（ID=${res?.data?.added?.id || '-'})`)
    } catch (e) {
      const reason = e?.response?.data?.error || e?.message || 'LLM 参数添加失败'
      setError(reason)
      showToast('error', `添加失败：${reason}`)
    } finally {
      setAddingLLM(false)
    }
  }

  const handleAddExchange = async () => {
    setAddingExchange(true)
    setError('')
    try {
      const payload = {
        name: String(newExchange.name || '').trim(),
        exchange: String(newExchange.exchange || 'binance').trim(),
        api_key: String(newExchange.api_key || '').trim(),
        secret: String(newExchange.secret || '').trim(),
        passphrase: String(newExchange.passphrase || '').trim(),
      }
      const res = await addExchangeIntegration(payload)
      setExchangeConfigs(Array.isArray(res?.data?.exchanges) ? res.data.exchanges : [])
      setNewExchange({
        name: '',
        exchange: 'binance',
        api_key: '',
        secret: '',
        passphrase: '',
      })
      setShowExchangeModal(false)
      showToast('success', `交易所参数添加并验证成功（ID=${res?.data?.added?.id || '-'})`)
    } catch (e) {
      const reason = e?.response?.data?.error || e?.message || '交易所参数添加失败'
      setError(reason)
      showToast('error', `添加失败：${reason}`)
    } finally {
      setAddingExchange(false)
    }
  }

  const resetPromptConfig = async () => {
    setResettingPrompts(true)
    setError('')
    try {
      const res = await resetPromptSettings()
      const prompts = res?.data?.prompts || {}
      setPromptSettings({
        trading_ai_system_prompt: String(prompts.trading_ai_system_prompt || ''),
        trading_ai_policy_prompt: String(prompts.trading_ai_policy_prompt || ''),
        strategy_generator_prompt_template: String(prompts.strategy_generator_prompt_template || ''),
      })
    } catch (e) {
      setError(e?.response?.data?.error || e?.message || '恢复默认失败')
    } finally {
      setResettingPrompts(false)
    }
  }

  const runOneCycle = async () => {
    setRunningNow(true)
    setError('')
    try {
      await runNow()
      await refreshCore(false)
    } catch (e) {
      setError(e?.response?.data?.error || e?.message || '执行失败')
    } finally {
      setRunningNow(false)
    }
  }

  const toggleScheduler = async (start) => {
    setError('')
    try {
      if (start) await startScheduler()
      else await stopScheduler()
      await refreshCore(false)
    } catch (e) {
      setError(e?.response?.data?.error || e?.message || '调度操作失败')
    }
  }

  const generateStrategy = async () => {
    setGeneratingStrategy(true)
    setError('')
    const id = `st_${Date.now()}`
    try {
      const res = await generateStrategyPreference({
        symbol: genPair,
        habit,
        strategy_style: genStyle,
        min_rr: clamp(genMinRR, 1.0, 10),
        allow_reversal: Boolean(genAllowReversal),
        low_conf_action: genLowConfAction,
        direction_bias: genDirectionBias,
      })
      const generated = res?.data?.generated || {}
      const name =
        String(generated.strategy_name || '').trim() ||
        `AI_${genPair}_${habit}_${new Date().toISOString().slice(0, 10)}`
      const preferencePrompt = String(generated.preference_prompt || '').trim()
      const generatorPrompt = String(generated.generator_prompt || '').trim()
      const logic = String(generated.logic || '').trim()
      const basis = String(generated.basis || '').trim()

      const rule = {
        id,
        name,
        habit,
        symbol: genPair,
        style: genStyle,
        minRR: clamp(genMinRR, 1.0, 10),
        allowReversal: Boolean(genAllowReversal),
        lowConfAction: genLowConfAction,
        directionBias: genDirectionBias,
        createdAt: new Date().toISOString(),
        prompt: generatorPrompt || promptSettings.strategy_generator_prompt_template,
        preferencePrompt: preferencePrompt || '',
        logic: logic || '按市场状态识别 -> 多因子确认 -> 风控过滤 -> 执行建议生成。',
        basis: basis || '基于实时行情与技术指标综合生成。',
      }
      setGeneratedStrategies((arr) => [rule, ...arr])
      setSelectedRuleId(id)
      setBuilderTab('rules')
      if (!btStrategy) setBtStrategy(name)

    } catch (e) {
      setError(e?.response?.data?.error || e?.message || '策略生成失败')
    } finally {
      setGeneratingStrategy(false)
    }
  }

  const selectedRule = generatedStrategies.find((s) => s.id === selectedRuleId)
  const renameHanCount = useMemo(() => countHanChars(renameRuleName), [renameRuleName])

  useEffect(() => {
    setRenameRuleName(selectedRule?.name || '')
  }, [selectedRuleId, selectedRule?.name])

  const toggleStrategyDraft = (id) => {
    setStrategyDraft((prev) => {
      if (prev.includes(id)) return prev.filter((x) => x !== id)
      if (prev.length >= 3) {
        setError('最多同时选择 3 条策略')
        return prev
      }
      return [...prev, id]
    })
  }

  const confirmStrategySelection = () => {
    const normalized = strategyDraft.filter((x) => executionStrategyOptions.includes(x)).slice(0, 3)
    const next = normalized.length ? normalized : (executionStrategyOptions[0] ? [executionStrategyOptions[0]] : [])
    setEnabledStrategies(next)
    if (next.length && !next.includes(activeStrategy)) {
      setActiveStrategy(next[0])
      setLiveStrategyStartedAt(Date.now())
    }
    setStrategyPickerOpen(false)
  }

  const renameGeneratedStrategy = () => {
    if (!selectedRule) {
      setError('请先选择要改名的策略')
      return
    }
    const oldName = String(selectedRule.name || '').trim()
    const nextName = String(renameRuleName || '').trim()
    if (!nextName) {
      setError('策略名称不能为空')
      showToast('error', '重命名失败：策略名称不能为空')
      return
    }
    if (countHanChars(nextName) > 10) {
      setError('策略名称最多 10 个汉字')
      showToast('error', '重命名失败：策略名称最多 10 个汉字')
      return
    }
    const hasDupGenerated = generatedStrategies.some(
      (s) => s.id !== selectedRule.id && String(s.name || '').trim() === nextName,
    )
    const hasDupBuiltin = strategyOptions.includes(nextName)
    if (hasDupGenerated || hasDupBuiltin) {
      setError('策略名称重复，请换一个名称')
      showToast('error', '重命名失败：策略名称重复')
      return
    }
    if (nextName === oldName) {
      showToast('warning', '名称未变化')
      return
    }

    setGeneratedStrategies((arr) => arr.map((s) => (s.id === selectedRule.id ? { ...s, name: nextName } : s)))
    setEnabledStrategies((arr) => arr.map((x) => (x === oldName ? nextName : x)))
    setStrategyDraft((arr) => arr.map((x) => (x === oldName ? nextName : x)))
    setActiveStrategy((v) => (v === oldName ? nextName : v))
    setPaperStrategy((v) => (v === oldName ? nextName : v))
    setBtStrategy((v) => (v === oldName ? nextName : v))
    setRenameRuleName(nextName)
    setError('')
    showToast('success', '策略重命名成功')
  }

  const uploadStrategy = async () => {
    if (!uploadFile) {
      setError('请先选择 .py 策略文件')
      return
    }
    if (!uploadFile.name.toLowerCase().endsWith('.py')) {
      setError('仅支持上传 .py 文件')
      return
    }
    setUploadingStrategy(true)
    setError('')
    try {
      const res = await uploadStrategyFile(uploadFile)
      setUploadFile(null)
      const available = parseStrategies(res?.data?.available || [])
      if (available.length) {
        setStrategyOptions(available)
      }
      await loadSystemAndStrategies()
    } catch (e) {
      setError(e?.response?.data?.error || e?.message || '上传策略失败')
    } finally {
      setUploadingStrategy(false)
    }
  }

  const loadStrategyTemplate = async () => {
    setLoadingTemplate(true)
    setError('')
    try {
      const res = await getStrategyTemplate()
      const content = String(res?.data?.content || '').trim()
      if (!content) {
        setStrategyTemplate(strategyTemplateFallback)
        setError('模板接口未返回内容，已加载内置模板。请重启 Go 后端后重试。')
        return
      }
      setStrategyTemplate(content)
    } catch (e) {
      setStrategyTemplate(strategyTemplateFallback)
      setError((e?.response?.data?.error || e?.message || '加载模板失败') + '，已回退内置模板')
    } finally {
      setLoadingTemplate(false)
    }
  }

  const copyStrategyTemplate = async () => {
    if (!strategyTemplate.trim()) {
      setError('模板为空，请先加载模板')
      return
    }
    try {
      await navigator.clipboard.writeText(strategyTemplate)
    } catch {
      setError('复制失败，请手动复制')
    }
  }

  async function runBacktest() {
    setBtRunning(true)
    setError('')
    try {
      if (btEnd < btStart) {
        throw new Error('结束时间不能早于开始时间')
      }
      const res = await runBacktestApi({
        strategy_name: btStrategy || activeStrategy || 'default_strategy',
        pair: btPair,
        habit,
        start_month: btStart,
        end_month: btEnd,
        initial_margin: clamp(btInitialMargin, 1, 1000000000),
        leverage: Math.round(clamp(btLeverage, 1, 150)),
        position_sizing_mode: String(btPositionSizingMode || 'contracts'),
        high_confidence_amount: clamp(btHighConfidenceAmount, 0, 1000000),
        low_confidence_amount: clamp(btLowConfidenceAmount, 0, 1000000),
        high_confidence_margin_pct: clamp(btHighConfidenceMarginPct, 0, 100) / 100,
        low_confidence_margin_pct: clamp(btLowConfidenceMarginPct, 0, 100) / 100,
        paper_margin: clamp(paperMargin, 0, 1000000),
      })
      const summary = res?.data?.summary || null
      const records = Array.isArray(res?.data?.records) ? res.data.records : []
      if (!summary) {
        setError('回测返回为空')
        return
      }
      const mappedSummary = mapBacktestSummary(summary)
      setBtSummary(mappedSummary)
      setBtRecords(records.slice().reverse())
      if (mappedSummary.historyId > 0) {
        setBtHistorySelectedId(mappedSummary.historyId)
      }
      await loadBacktestHistory(true)
      if (res?.data?.history_warning) {
        showToast('warning', String(res.data.history_warning))
      }
    } catch (e) {
      setError(e?.response?.data?.error || e?.message || '回测失败')
    } finally {
      setBtRunning(false)
    }
  }

  const viewBacktestHistoryDetail = async (id) => {
    const historyID = Number(id || 0)
    if (historyID <= 0) return
    setBtHistoryLoading(true)
    setError('')
    try {
      const res = await getBacktestHistoryDetail(historyID)
      const summary = res?.data?.summary || null
      const records = Array.isArray(res?.data?.records) ? res.data.records : []
      if (!summary) {
        setError('回测详情为空')
        return
      }
      setBtSummary(mapBacktestSummary(summary))
      setBtRecords(records.slice().reverse())
      setBtHistorySelectedId(historyID)
    } catch (e) {
      setError(e?.response?.data?.error || e?.message || '加载回测详情失败')
      showToast('error', '加载回测详情失败')
    } finally {
      setBtHistoryLoading(false)
    }
  }

  const removeBacktestHistory = async (id) => {
    const historyID = Number(id || 0)
    if (historyID <= 0) return
    if (!window.confirm(`确认删除回测记录 #${historyID} 吗？`)) return
    setBtHistoryDeletingId(historyID)
    setError('')
    try {
      await deleteBacktestHistory(historyID)
      await loadBacktestHistory(true)
      setBtHistory((arr) => arr.filter((x) => Number(x?.id || 0) !== historyID))
      if (Number(btHistorySelectedId || 0) === historyID) {
        setBtSummary(null)
        setBtRecords([])
        setBtHistorySelectedId(0)
      }
      showToast('success', `回测记录 #${historyID} 已删除`)
    } catch (e) {
      const reason = e?.response?.data?.error || e?.message || '删除失败'
      setError(reason)
      showToast('error', `删除失败：${reason}`)
    } finally {
      setBtHistoryDeletingId(0)
    }
  }

  const renderOverviewCards = (klineData, pair, strategyName, extra = null) => (
    <div className="builder-pane">
      <div className="overview-grid">
        <article className="metric-card"><h4>今日市场情绪</h4><p>{marketEmotion}</p></article>
        <article className="metric-card"><h4>总盈亏</h4><p className={totalPnL >= 0 ? 'up' : 'down'}>{fmtNum(totalPnL, 2)} USDT</p></article>
        <article className="metric-card"><h4>账户信息</h4><p>余额 {fmtNum(account?.balance, 2)} / 持仓 {account?.position?.side || '无'}</p></article>
        <article className="metric-card"><h4>当前策略交易时长</h4><p>{strategyDurationText}</p></article>
        <article className="metric-card"><h4>盈亏比</h4><p>{pnlRatio}</p></article>
        <article className="metric-card"><h4>当前策略</h4><p>{strategyName || '-'}</p></article>
      </div>
      {extra}
      <section className="sub-window kline-card">
        <div className="card-head">
          <h3>K 线图</h3>
          <span>{pair} · 1s刷新 · {fmtTime(klineUpdatedAt)}</span>
        </div>
        <KlineChart data={klineData} />
      </section>
    </div>
  )

  return (
    <>
      {toast.visible ? (
        <div className={`top-toast ${toast.type}`} role="status" aria-live="polite">
          {toast.message}
        </div>
      ) : null}
      <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">{productName}</div>
        <nav className="dir-menu">
          {MENU_ITEMS.map((item) => (
            <button
              key={item.key}
              className={`dir-item ${menu === item.key ? 'active' : ''}`}
              onClick={() => setMenu(item.key)}
            >
              {item.label}
            </button>
          ))}
        </nav>
      </aside>

      <main className="content">
        <header className="content-head">
          <h1>{MENU_ITEMS.find((m) => m.key === menu)?.label}</h1>
          <p>状态：{loading ? '加载中' : '就绪'} {error ? `| ${error}` : ''}</p>
        </header>

        {menu === 'live' && (
          <section className="stack">
            <section className="card">
              <h3>实盘交易</h3>
              <div className="form-grid wide">
                <label>
                  <span>交易对</span>
                  <select value={activePair} onChange={(e) => setActivePair(e.target.value)}>
                    {PAIRS.map((p) => <option key={p} value={p}>{p}</option>)}
                  </select>
                </label>
                <label>
                  <span>交易策略（多选）</span>
                  <div className="strategy-picker" ref={strategyPickerRef}>
                    <button
                      type="button"
                      className="strategy-picker-trigger"
                      onClick={(e) => {
                        e.preventDefault()
                        if (!strategyPickerOpen) {
                          setStrategyDraft(enabledStrategies)
                        }
                        setStrategyPickerOpen((v) => !v)
                      }}
                    >
                      {selectedStrategyText}
                    </button>
                    {strategyPickerOpen ? (
                      <div className="strategy-picker-menu">
                        <div className="strategy-picker-list">
                          {executionStrategyOptions.map((s) => (
                            <label key={`strategy-pick-${s}`} className="strategy-picker-item">
                              <input
                                type="checkbox"
                                checked={strategyDraft.includes(s)}
                                disabled={!strategyDraft.includes(s) && strategyDraft.length >= 3}
                                onChange={() => toggleStrategyDraft(s)}
                              />
                              <span>{s}</span>
                            </label>
                          ))}
                        </div>
                        <div className="actions-row end">
                          <button
                            type="button"
                            onClick={(e) => {
                              e.preventDefault()
                              setStrategyPickerOpen(false)
                            }}
                          >
                            取消
                          </button>
                          <button
                            type="button"
                            className="primary"
                            onClick={(e) => {
                              e.preventDefault()
                              confirmStrategySelection()
                            }}
                          >
                            确认
                          </button>
                        </div>
                      </div>
                    ) : null}
                  </div>
                </label>
                <label>
                  <span>仓位模式</span>
                  <select
                    value={settings.positionSizingMode}
                    onChange={(e) => setSettings((v) => ({ ...v, positionSizingMode: e.target.value }))}
                  >
                    <option value="contracts">按张数</option>
                    <option value="margin_pct">按保证金百分比</option>
                  </select>
                </label>
                {settings.positionSizingMode === 'contracts' ? (
                  <>
                    <label>
                      <span>高信心张数</span>
                      <input
                        type="number"
                        min="0"
                        step="0.0001"
                        value={settings.highConfidenceAmount}
                        onChange={(e) => setSettings((v) => ({ ...v, highConfidenceAmount: Number(e.target.value) }))}
                      />
                    </label>
                    <label>
                      <span>低信心张数</span>
                      <input
                        type="number"
                        min="0"
                        step="0.0001"
                        value={settings.lowConfidenceAmount}
                        onChange={(e) => setSettings((v) => ({ ...v, lowConfidenceAmount: Number(e.target.value) }))}
                      />
                    </label>
                  </>
                ) : (
                  <>
                    <label>
                      <span>高信心保证金%</span>
                      <input
                        type="number"
                        min="0"
                        max="100"
                        step="0.1"
                        value={settings.highConfidenceMarginPct}
                        onChange={(e) => setSettings((v) => ({ ...v, highConfidenceMarginPct: Number(e.target.value) }))}
                      />
                    </label>
                    <label>
                      <span>低信心保证金%</span>
                      <input
                        type="number"
                        min="0"
                        max="100"
                        step="0.1"
                        value={settings.lowConfidenceMarginPct}
                        onChange={(e) => setSettings((v) => ({ ...v, lowConfidenceMarginPct: Number(e.target.value) }))}
                      />
                    </label>
                  </>
                )}
                <label>
                  <span>杠杆(1-150)</span>
                  <input
                    type="number"
                    min="1"
                    max="150"
                    step="1"
                    value={settings.leverage}
                    onChange={(e) => setSettings((v) => ({ ...v, leverage: Number(e.target.value) }))}
                  />
                </label>
              </div>

              <div className="actions-row">
                <button onClick={() => refreshCore(false)}>刷新</button>
                <button onClick={runOneCycle} disabled={runningNow}>{runningNow ? '执行中...' : '手动执行1次'}</button>
                <button onClick={() => toggleScheduler(true)} disabled={schedulerRunning}>启动调度</button>
                <button onClick={() => toggleScheduler(false)} disabled={!schedulerRunning} className="danger">停止</button>
                <button
                  onClick={saveLiveConfig}
                  disabled={savingSettings}
                  className={`primary save-config-btn ${savingSettings ? 'is-saving' : ''}`}
                >
                  {savingSettings ? '保存中...' : '确认'}
                </button>
              </div>
            </section>

            <section className="card">
              <div className="tab-row">
                <button
                  className={liveViewTab === 'overview' ? 'tab active' : 'tab'}
                  onClick={() => setLiveViewTab('overview')}
                >
                  交易总览
                </button>
                <button
                  className={liveViewTab === 'records' ? 'tab active' : 'tab'}
                  onClick={() => setLiveViewTab('records')}
                >
                  交易记录
                </button>
              </div>
              {liveViewTab === 'overview' ? (
                renderOverviewCards(liveKline, activePair, liveStrategyLabel)
              ) : (
                <div className="builder-pane">
                  <TradeRecordsTable records={tradeRecords.filter((r) => !r.symbol || r.symbol === activePair)} />
                </div>
              )}
            </section>
          </section>
        )}

        {menu === 'paper' && (
          <section className="stack">
            <section className="card">
              <h3>模拟交易</h3>
              <div className="form-grid wide">
                <label>
                  <span>交易对</span>
                  <select value={paperPair} onChange={(e) => setPaperPair(e.target.value)}>
                    {PAIRS.map((p) => <option key={p} value={p}>{p}</option>)}
                  </select>
                </label>
                <label>
                  <span>交易策略</span>
                  <select value={paperStrategy} onChange={(e) => setPaperStrategy(e.target.value)}>
                    {executionStrategyOptions.map((s) => <option key={s} value={s}>{s}</option>)}
                  </select>
                </label>
                <label>
                  <span>模拟保证金(USDT)</span>
                  <input
                    type="number"
                    min="0"
                    step="1"
                    value={paperMargin}
                    onChange={(e) => setPaperMargin(Number(e.target.value))}
                  />
                </label>
              </div>
            </section>
            <section className="card">
              <div className="tab-row">
                <button
                  className={paperViewTab === 'overview' ? 'tab active' : 'tab'}
                  onClick={() => setPaperViewTab('overview')}
                >
                  交易总览
                </button>
                <button
                  className={paperViewTab === 'records' ? 'tab active' : 'tab'}
                  onClick={() => setPaperViewTab('records')}
                >
                  交易记录
                </button>
              </div>
              {paperViewTab === 'overview' ? (
                renderOverviewCards(
                  paperKline,
                  paperPair,
                  paperStrategy,
                  <section className="sub-window"><h3>模拟交易补充</h3><p>该板块不发真实订单，仅用于策略参数演练与回测准备。</p></section>
                )
              ) : (
                <div className="builder-pane">
                  <TradeRecordsTable records={paperTradeRecords.filter((r) => !r.symbol || r.symbol === paperPair)} />
                </div>
              )}
            </section>
          </section>
        )}

        {menu === 'assets' && (
          <section className="stack">
            <div className="asset-layout">
              <section className="card asset-total-card">
                <h3>交易所账户总资金</h3>
                <div className="asset-convert-label">资产折合</div>
                <div className="asset-convert-value">≈{fmtNum(assetOverview.total_funds, 2)} USDT</div>
                <div className="asset-total-row">
                  <span>今日盈亏</span>
                  <b className={Number(assetOverview.today_pnl_amount || 0) >= 0 ? 'up' : 'down'}>
                    {fmtNum(assetOverview.today_pnl_amount, 2)} ({fmtPct(assetOverview.today_pnl_pct)})
                  </b>
                </div>
                <div className="asset-total-row">
                  <span>累计盈亏</span>
                  <b className={Number(assetOverview.cumulative_pnl || 0) >= 0 ? 'up' : 'down'}>
                    {fmtNum(assetOverview.cumulative_pnl, 2)} ({fmtPct(assetOverview.cumulative_pnl_pct)})
                  </b>
                </div>
              </section>
              <section className="card">
                <h3>资产分布图</h3>
                <AssetDistributionChart items={assetDistribution} />
              </section>
              <section className="card asset-equal-card">
                <div className="card-head">
                  <h3>盈亏日历</h3>
                  <MonthSelect value={assetMonth} min="2018-01" max="2025-12" onChange={setAssetMonth} />
                </div>
                <PnLCalendar month={assetMonth} days={assetCalendar} />
              </section>
              <section className="card asset-equal-card">
                <div className="card-head">
                  <h3>资产趋势</h3>
                  <div className="tab-row">
                    {['7D', '30D', '3M', '6M', '1Y'].map((r) => (
                      <button
                        key={r}
                        className={assetRange === r ? 'tab active' : 'tab'}
                        onClick={() => setAssetRange(r)}
                      >
                        {r}
                      </button>
                    ))}
                  </div>
                </div>
                <AssetTrendChart points={assetTrend} />
              </section>
            </div>
          </section>
        )}

        {menu === 'builder' && (
          <section className="stack">
            <section className="card">
              <div className="tab-row">
                <button className={builderTab === 'generate' ? 'tab active' : 'tab'} onClick={() => setBuilderTab('generate')}>策略生成</button>
                <button className={builderTab === 'rules' ? 'tab active' : 'tab'} onClick={() => setBuilderTab('rules')}>策略规则</button>
              </div>

              {builderTab === 'generate' && (
                <div className="builder-pane">
                  <section className="sub-window">
                    <h4>策略生成参数</h4>
                    <div className="form-grid">
                      <label>
                        <span>交易习惯时长</span>
                        <select value={habit} onChange={(e) => setHabit(e.target.value)}>
                          {HABIT_OPTIONS.map((h) => <option key={h} value={h}>{h}</option>)}
                        </select>
                      </label>
                      <label>
                        <span>交易对</span>
                        <select value={genPair} onChange={(e) => setGenPair(e.target.value)}>
                          {PAIRS.map((p) => <option key={`gen-${p}`} value={p}>{p}</option>)}
                        </select>
                      </label>
                      <label>
                        <span>策略样式</span>
                        <select value={genStyle} onChange={(e) => setGenStyle(e.target.value)}>
                          <option value="hybrid">混合</option>
                          <option value="trend_following">趋势</option>
                          <option value="breakout">突破</option>
                          <option value="mean_reversion">均值回归</option>
                        </select>
                      </label>
                      <label>
                        <span>最小盈亏比（盈利/亏损）</span>
                        <input
                          type="number"
                          min="1"
                          max="10"
                          step="0.1"
                          value={genMinRR}
                          onChange={(e) => setGenMinRR(Number(e.target.value))}
                        />
                      </label>
                      <label>
                        <span>低信心处理</span>
                        <select value={genLowConfAction} onChange={(e) => setGenLowConfAction(e.target.value)}>
                          <option value="hold">直接观望</option>
                          <option value="reduce_size">缩小仓位</option>
                          <option value="strict_filter">加强过滤后再评估</option>
                        </select>
                      </label>
                      <label>
                        <span>方向偏好</span>
                        <select value={genDirectionBias} onChange={(e) => setGenDirectionBias(e.target.value)}>
                          <option value="balanced">平衡</option>
                          <option value="long_bias">偏多</option>
                          <option value="short_bias">偏空</option>
                        </select>
                      </label>
                      <label>
                        <span>允许反转</span>
                        <select
                          value={genAllowReversal ? 'true' : 'false'}
                          onChange={(e) => setGenAllowReversal(e.target.value === 'true')}
                        >
                          <option value="false">否</option>
                          <option value="true">是</option>
                        </select>
                      </label>
                    </div>
                    <div className="actions-row end">
                      <button onClick={generateStrategy} disabled={generatingStrategy}>
                        {generatingStrategy ? '生成中...' : '生成策略'}
                      </button>
                    </div>
                  </section>

                  <section className="sub-window template-window">
                    <h4>模板策略脚本</h4>
                    <div className="form-grid">
                      <label>
                        <span>上传 Python 策略脚本</span>
                        <input
                          type="file"
                          accept=".py,text/x-python"
                          onChange={(e) => setUploadFile(e.target.files?.[0] || null)}
                        />
                      </label>
                    </div>
                    <div className="actions-row">
                      <button onClick={loadStrategyTemplate} disabled={loadingTemplate}>
                        {loadingTemplate ? '加载中...' : '加载模板策略脚本'}
                      </button>
                      <button onClick={copyStrategyTemplate}>复制模板</button>
                      <button onClick={uploadStrategy} disabled={uploadingStrategy}>
                        {uploadingStrategy ? '上传中...' : '上传策略'}
                      </button>
                    </div>
                    <label>
                      <span>模板策略脚本（可复制后修改再上传）</span>
                      <textarea
                        value={strategyTemplate}
                        readOnly
                        placeholder="点击“加载模板策略脚本”查看 sample_template.py"
                      />
                    </label>
                  </section>
                </div>
              )}

              {builderTab === 'rules' && (
                <div className="builder-pane">
                  <div className="rule-tags">
                    {generatedStrategies.map((s) => (
                      <button
                        key={s.id}
                        className={`rule-tag ${selectedRuleId === s.id ? 'active' : ''}`}
                        onClick={() => setSelectedRuleId(s.id)}
                      >
                        {s.name}
                      </button>
                    ))}
                    {!generatedStrategies.length && <p className="muted">请先在“策略生成”中创建策略。</p>}
                  </div>
                  {selectedRule ? (
                    <div className="rule-detail">
                      <p><b>策略名称：</b>{selectedRule.name}</p>
                      <div className="actions-row">
                        <input
                          type="text"
                          value={renameRuleName}
                          maxLength={24}
                          placeholder="支持中文，最多10个汉字"
                          onChange={(e) => setRenameRuleName(e.target.value)}
                        />
                        <button type="button" className="primary" onClick={renameGeneratedStrategy}>
                          确认改名
                        </button>
                      </div>
                      <p className="muted">汉字长度：{renameHanCount}/10</p>
                      <p><b>交易对：</b>{selectedRule.symbol || '-'}</p>
                      <p><b>策略样式：</b>{selectedRule.style || '-'}</p>
                      <p><b>最小盈亏比（盈利/亏损）：</b>{fmtNum(selectedRule.minRR, 2)}</p>
                      <p><b>允许反转：</b>{selectedRule.allowReversal ? '是' : '否'}</p>
                      <p><b>低信心处理：</b>{selectedRule.lowConfAction || '-'}</p>
                      <p><b>方向偏好：</b>{selectedRule.directionBias || '-'}</p>
                      <p><b>策略偏好：</b>{selectedRule.preferencePrompt || '-'}</p>
                      <p><b>提示词：</b>{selectedRule.prompt}</p>
                      <p><b>AI 生成逻辑：</b>{selectedRule.logic}</p>
                      <p><b>依据：</b>{selectedRule.basis}</p>
                    </div>
                  ) : null}
                </div>
              )}

            </section>
          </section>
        )}

        {menu === 'backtest' && (
          <section className="stack">
            <section className="card">
              <h3>历史回测</h3>
              <div className="builder-pane">
                <div className="form-grid backtest-grid">
                  <label className="bt-strategy">
                    <span>策略</span>
                    <select value={btStrategy} onChange={(e) => setBtStrategy(e.target.value)}>
                      <option value="">请选择</option>
                      {executionStrategyOptions.map((s) => <option key={`bt-${s}`} value={s}>{s}</option>)}
                    </select>
                  </label>
                  <label className="bt-pair">
                    <span>交易对</span>
                    <select value={btPair} onChange={(e) => setBtPair(e.target.value)}>
                      {PAIRS.map((p) => <option key={p} value={p}>{p}</option>)}
                    </select>
                  </label>
                  <label className="bt-initial-margin">
                    <span>初始保证金(USDT)</span>
                    <input
                      type="number"
                      min="1"
                      step="1"
                      value={btInitialMargin}
                      onChange={(e) => setBtInitialMargin(Number(e.target.value))}
                    />
                  </label>

                  <label className="bt-leverage">
                    <span>合约倍率(1-150)</span>
                    <input
                      type="number"
                      min="1"
                      max="150"
                      step="1"
                      value={btLeverage}
                      onChange={(e) => setBtLeverage(Number(e.target.value))}
                    />
                  </label>

                  <div className="bt-position-panel">
                    <div className="bt-position-grid">
                      <label>
                        <span>仓位模式</span>
                        <select
                          value={btPositionSizingMode}
                          onChange={(e) => setBtPositionSizingMode(e.target.value)}
                        >
                          <option value="contracts">按张数</option>
                          <option value="margin_pct">按百分比仓位</option>
                        </select>
                      </label>
                      {btPositionSizingMode === 'contracts' ? (
                        <>
                          <label>
                            <span>高信心张数</span>
                            <input
                              type="number"
                              min="0"
                              step="0.0001"
                              value={btHighConfidenceAmount}
                              onChange={(e) => setBtHighConfidenceAmount(Number(e.target.value))}
                            />
                          </label>
                          <label>
                            <span>低信心张数</span>
                            <input
                              type="number"
                              min="0"
                              step="0.0001"
                              value={btLowConfidenceAmount}
                              onChange={(e) => setBtLowConfidenceAmount(Number(e.target.value))}
                            />
                          </label>
                        </>
                      ) : (
                        <>
                          <label>
                            <span>高信心仓位%</span>
                            <input
                              type="number"
                              min="0"
                              max="100"
                              step="0.1"
                              value={btHighConfidenceMarginPct}
                              onChange={(e) => setBtHighConfidenceMarginPct(Number(e.target.value))}
                            />
                          </label>
                          <label>
                            <span>低信心仓位%</span>
                            <input
                              type="number"
                              min="0"
                              max="100"
                              step="0.1"
                              value={btLowConfidenceMarginPct}
                              onChange={(e) => setBtLowConfidenceMarginPct(Number(e.target.value))}
                            />
                          </label>
                        </>
                      )}
                    </div>
                  </div>

                  <div className="simulate-date-range bt-time-range">
                    <label className="date-range-field">
                      <span>开始时间</span>
                      <MonthSelect
                        value={btStart}
                        min="2018-01"
                        max="2025-12"
                        onChange={(v) => {
                          setBtStart(v)
                          if (v > btEnd) setBtEnd(v)
                        }}
                      />
                    </label>
                    <div className="date-range-sep">-</div>
                    <label className="date-range-field">
                      <span>结束时间</span>
                      <MonthSelect
                        value={btEnd}
                        min="2018-01"
                        max="2025-12"
                        onChange={(v) => {
                          setBtEnd(v < btStart ? btStart : v)
                        }}
                      />
                    </label>
                  </div>
                </div>
                <div className="actions-row end"><button onClick={runBacktest} disabled={btRunning}>{btRunning ? '回测中...' : '开始回测'}</button></div>

                {btSummary ? (
                  <div className="summary-grid">
                    <article className="metric-card"><h4>策略</h4><p>{btSummary.strategy}</p></article>
                    <article className="metric-card"><h4>初始保证金</h4><p>{fmtNum(btSummary.initialMargin, 2)} USDT</p></article>
                    <article className="metric-card"><h4>合约倍率</h4><p>{btSummary.leverage || '-'}x</p></article>
                    <article className="metric-card"><h4>仓位模式</h4><p>{btSummary.positionSizingMode === 'margin_pct' ? '按百分比仓位' : '按张数'}</p></article>
                    {btSummary.positionSizingMode === 'margin_pct' ? (
                      <article className="metric-card"><h4>高/低信心仓位%</h4><p>{fmtNum(btSummary.highConfidenceMarginPct, 2)} / {fmtNum(btSummary.lowConfidenceMarginPct, 2)}</p></article>
                    ) : (
                      <article className="metric-card"><h4>高/低信心张数</h4><p>{fmtNum(btSummary.highConfidenceAmount, 4)} / {fmtNum(btSummary.lowConfidenceAmount, 4)}</p></article>
                    )}
                    <article className="metric-card"><h4>总盈亏</h4><p className={btSummary.totalPnl >= 0 ? 'up' : 'down'}>{fmtNum(btSummary.totalPnl, 2)} USDT</p></article>
                    <article className="metric-card"><h4>期末权益</h4><p className={btSummary.finalEquity >= 0 ? 'up' : 'down'}>{fmtNum(btSummary.finalEquity, 2)} USDT</p></article>
                    <article className="metric-card"><h4>收益率</h4><p className={btSummary.returnPct >= 0 ? 'up' : 'down'}>{fmtPct(btSummary.returnPct)}</p></article>
                    <article className="metric-card"><h4>回测时长</h4><p>{btSummary.start} - {btSummary.end}</p></article>
                    <article className="metric-card"><h4>盈亏比</h4><p>{btSummary.ratioInfinite ? '∞' : fmtNum(btSummary.ratio, 2)}</p></article>
                    <article className="metric-card"><h4>胜/负</h4><p>{btSummary.wins}/{btSummary.losses}</p></article>
                    <article className="metric-card"><h4>数据条数</h4><p>{btSummary.bars}</p></article>
                  </div>
                ) : null}

                <StrategyBacktestTable records={btRecords} />

                <section className="sub-window">
                  <div className="card-head">
                    <h4>回测记录</h4>
                    <span>{btHistoryLoading ? '加载中...' : `共 ${btHistory.length} 条`}</span>
                  </div>
                  {!btHistory.length ? (
                    <p className="muted">暂无回测记录</p>
                  ) : (
                    <div className="table-wrap">
                      <table className="backtest-history-table">
                        <thead>
                          <tr>
                            <th>ID</th>
                            <th>创建时间</th>
                            <th>策略</th>
                            <th>交易对</th>
                            <th>区间</th>
                            <th>总盈亏</th>
                            <th>收益率</th>
                            <th>操作</th>
                          </tr>
                        </thead>
                        <tbody>
                          {btHistory.map((row) => {
                            const rowID = Number(row?.id || 0)
                            const selected = rowID > 0 && rowID === Number(btHistorySelectedId || 0)
                            return (
                              <tr key={`bth-${rowID}`} className={selected ? 'history-row active' : 'history-row'}>
                                <td>{rowID || '-'}</td>
                                <td>{fmtTime(row?.created_at)}</td>
                                <td>{row?.strategy || '-'}</td>
                                <td>{row?.pair || '-'}</td>
                                <td>{row?.start || '-'} - {row?.end || '-'}</td>
                                <td className={Number(row?.total_pnl || 0) >= 0 ? 'up' : 'down'}>
                                  {fmtNum(row?.total_pnl, 2)}
                                </td>
                                <td className={Number(row?.return_pct || 0) >= 0 ? 'up' : 'down'}>
                                  {fmtPct(row?.return_pct)}
                                </td>
                                <td>
                                  <div className="inline-actions">
                                    <button type="button" onClick={() => viewBacktestHistoryDetail(rowID)}>查看明细</button>
                                    <button
                                      type="button"
                                      className="danger"
                                      disabled={btHistoryDeletingId === rowID}
                                      onClick={() => removeBacktestHistory(rowID)}
                                    >
                                      {btHistoryDeletingId === rowID ? '删除中...' : '删除'}
                                    </button>
                                  </div>
                                </td>
                              </tr>
                            )
                          })}
                        </tbody>
                      </table>
                    </div>
                  )}
                </section>
              </div>
            </section>
          </section>
        )}

        {menu === 'system' && (
          <section className="stack">
            <section className="card">
              <h3>系统设置</h3>
              <div className="tab-row">
                <button
                  className={systemSubTab === 'env' ? 'tab active' : 'tab'}
                  onClick={() => setSystemSubTab('env')}
                >
                  运行配置
                </button>
                <button
                  className={systemSubTab === 'llm' ? 'tab active' : 'tab'}
                  onClick={() => setSystemSubTab('llm')}
                >
                  LLM 参数
                </button>
                <button
                  className={systemSubTab === 'exchange' ? 'tab active' : 'tab'}
                  onClick={() => setSystemSubTab('exchange')}
                >
                  交易所参数
                </button>
                <button
                  className={systemSubTab === 'prompts' ? 'tab active' : 'tab'}
                  onClick={() => setSystemSubTab('prompts')}
                >
                  提示词管理
                </button>
              </div>

              {systemSubTab === 'env' && (
                <div className="builder-pane">
                  <div className="env-groups">
                    {envFieldGroups.map((group) => (
                      <section key={group.title} className="env-group-card">
                        <h4>{group.title}</h4>
                        <div className="env-grid">
                          {group.fields.map((f) => (
                            <label key={f.key}>
                              <span>{f.label}</span>
                              <input
                                type={f.type || 'text'}
                                value={systemSettings[f.key] || ''}
                                placeholder={f.placeholder || ''}
                                onChange={(e) => setSystemSettings((v) => ({ ...v, [f.key]: e.target.value }))}
                              />
                            </label>
                          ))}
                        </div>
                      </section>
                    ))}
                  </div>
                  <div className="actions-row end">
                    {systemSaveHint ? <span className="save-hint">{systemSaveHint}</span> : null}
                    <button
                      onClick={saveSystemEnv}
                      disabled={savingSystemSettings}
                      className={`primary save-config-btn ${savingSystemSettings ? 'is-saving' : ''}`}
                    >
                      {savingSystemSettings ? '保存中...' : '保存系统设置'}
                    </button>
                  </div>
                </div>
              )}

              {systemSubTab === 'llm' && (
                <div className="builder-pane">
                  <div className="actions-row end">
                    <button className="primary" onClick={() => setShowLLMModal(true)}>添加 LLM 模型</button>
                  </div>
                  <div className="table-wrap">
                    <table className="centered-list-table">
                      <thead>
                        <tr>
                          <th>ID</th>
                          <th>名称</th>
                          <th>Base URL</th>
                          <th>模型</th>
                        </tr>
                      </thead>
                      <tbody>
                        {llmConfigs.map((x) => (
                          <tr key={x.id}>
                            <td>{x.id}</td>
                            <td>{x.name}</td>
                            <td>{x.base_url}</td>
                            <td>{x.model}</td>
                          </tr>
                        ))}
                        {!llmConfigs.length ? (
                          <tr><td colSpan="4" className="muted">暂无 LLM 参数</td></tr>
                        ) : null}
                      </tbody>
                    </table>
                  </div>
                </div>
              )}

              {systemSubTab === 'exchange' && (
                <div className="builder-pane">
                  <div className="actions-row end">
                    <button className="primary" onClick={() => setShowExchangeModal(true)}>添加交易所参数</button>
                  </div>
                  <div className="table-wrap">
                    <table className="centered-list-table">
                      <thead>
                        <tr>
                          <th>ID</th>
                          <th>交易所</th>
                          <th>API Key</th>
                        </tr>
                      </thead>
                      <tbody>
                        {exchangeConfigs.map((x) => (
                          <tr key={x.id}>
                            <td>{x.id}</td>
                            <td>{x.exchange}</td>
                            <td>{x.api_key ? `${String(x.api_key).slice(0, 6)}***` : '-'}</td>
                          </tr>
                        ))}
                        {!exchangeConfigs.length ? (
                          <tr><td colSpan="3" className="muted">暂无交易所参数</td></tr>
                        ) : null}
                      </tbody>
                    </table>
                  </div>
                </div>
              )}

              {systemSubTab === 'prompts' && (
                <div className="builder-pane">
                  <div className="prompt-grid">
                    <label>
                      <span>硬边界一：角色与输出约束</span>
                      <textarea
                        value={promptSettings.trading_ai_system_prompt}
                        onChange={(e) =>
                          setPromptSettings((v) => ({ ...v, trading_ai_system_prompt: e.target.value }))
                        }
                      />
                    </label>
                    <label>
                      <span>硬边界二：风险与执行约束</span>
                      <textarea
                        value={promptSettings.trading_ai_policy_prompt}
                        onChange={(e) =>
                          setPromptSettings((v) => ({ ...v, trading_ai_policy_prompt: e.target.value }))
                        }
                      />
                    </label>
                    <label>
                      <span>硬边界三：策略生成结构模板（支持 ${'{habit}'} / ${'{symbol}'} 变量）</span>
                      <textarea
                        value={promptSettings.strategy_generator_prompt_template}
                        onChange={(e) =>
                          setPromptSettings((v) => ({
                            ...v,
                            strategy_generator_prompt_template: e.target.value,
                          }))
                        }
                      />
                    </label>
                  </div>
                  <div className="actions-row end">
                    <button onClick={resetPromptConfig} disabled={resettingPrompts}>
                      {resettingPrompts ? '恢复中...' : '恢复默认预设'}
                    </button>
                    <button
                      onClick={savePromptConfig}
                      disabled={savingPrompts}
                      className={`primary save-config-btn ${savingPrompts ? 'is-saving' : ''}`}
                    >
                      {savingPrompts ? '保存中...' : '保存提示词'}
                    </button>
                  </div>
                </div>
              )}
            </section>
          </section>
        )}
      </main>
      </div>
      {showLLMModal ? (
        <div className="modal-mask" onClick={() => setShowLLMModal(false)}>
          <section className="modal-card" onClick={(e) => e.stopPropagation()}>
            <h3>添加 LLM 模型</h3>
            <p className="muted">ID 将自动按 1,2,3... 递增分配。</p>
            <div className="form-grid modal-form">
              <label><span>名称</span><input value={newLLM.name} onChange={(e) => setNewLLM((v) => ({ ...v, name: e.target.value }))} /></label>
              <label><span>Base URL</span><input value={newLLM.base_url} onChange={(e) => setNewLLM((v) => ({ ...v, base_url: e.target.value }))} /></label>
              <label><span>API Key</span><input type="password" value={newLLM.api_key} onChange={(e) => setNewLLM((v) => ({ ...v, api_key: e.target.value }))} /></label>
              <label><span>模型</span><input value={newLLM.model} onChange={(e) => setNewLLM((v) => ({ ...v, model: e.target.value }))} /></label>
            </div>
            <div className="actions-row end">
              <button onClick={() => setShowLLMModal(false)}>取消</button>
              <button
                onClick={handleAddLLM}
                disabled={addingLLM}
                className={`primary save-config-btn ${addingLLM ? 'is-saving' : ''}`}
              >
                {addingLLM ? '校验中...' : '确认添加'}
              </button>
            </div>
          </section>
        </div>
      ) : null}
      {showExchangeModal ? (
        <div className="modal-mask" onClick={() => setShowExchangeModal(false)}>
          <section className="modal-card" onClick={(e) => e.stopPropagation()}>
            <h3>添加交易所参数</h3>
            <p className="muted">ID 将自动按 1,2,3... 递增分配。</p>
            <div className="form-grid modal-form">
              <label>
                <span>交易所</span>
                <select value={newExchange.exchange} onChange={(e) => setNewExchange((v) => ({ ...v, exchange: e.target.value }))}>
                  <option value="binance">binance</option>
                </select>
              </label>
              <label><span>API Key</span><input value={newExchange.api_key} onChange={(e) => setNewExchange((v) => ({ ...v, api_key: e.target.value }))} /></label>
              <label><span>Secret</span><input type="password" value={newExchange.secret} onChange={(e) => setNewExchange((v) => ({ ...v, secret: e.target.value }))} /></label>
              <label><span>Passphrase(可选)</span><input value={newExchange.passphrase} onChange={(e) => setNewExchange((v) => ({ ...v, passphrase: e.target.value }))} /></label>
            </div>
            <div className="actions-row end">
              <button onClick={() => setShowExchangeModal(false)}>取消</button>
              <button
                onClick={handleAddExchange}
                disabled={addingExchange}
                className={`primary save-config-btn ${addingExchange ? 'is-saving' : ''}`}
              >
                {addingExchange ? '校验中...' : '确认添加'}
              </button>
            </div>
          </section>
        </div>
      ) : null}
    </>
  )
}
