import { useEffect, useMemo, useState } from 'react'
import {
  getAccount,
  getSignals,
  getStatus,
  getStrategyScores,
  getSystemSettings,
  runNow,
  saveSystemSettings,
  startScheduler,
  stopScheduler,
  updateSettings,
} from './api'

function fmtTime(value) {
  if (!value) return '-'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return '-'
  return d.toLocaleString()
}

function fmtNum(value, digits = 2) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '-'
  return Number(value).toFixed(digits)
}

function clamp(value, min, max) {
  const n = Number(value)
  if (Number.isNaN(n)) return min
  return Math.min(max, Math.max(min, n))
}

const envFieldDefs = [
  { key: 'AI_API_KEY', label: 'AI API Key', type: 'password' },
  { key: 'AI_BASE_URL', label: 'AI Base URL' },
  { key: 'AI_MODEL', label: 'AI Model' },
  { key: 'PY_STRATEGY_URL', label: 'Python 策略服务 URL' },
  { key: 'BINANCE_API_KEY', label: 'Binance API Key', type: 'password' },
  { key: 'BINANCE_SECRET', label: 'Binance Secret', type: 'password' },
  { key: 'MODE', label: '运行模式 (cli/web)' },
  { key: 'HTTP_ADDR', label: 'HTTP 监听地址' },
  { key: 'TRADE_DB_PATH', label: '数据库路径' },
  { key: 'ENABLE_WS_MARKET', label: '启用WS行情 (true/false)' },
  { key: 'REALTIME_MIN_INTERVAL_SEC', label: '实时最小间隔(秒)' },
  { key: 'STRATEGY_LLM_ENABLED', label: '启用LLM调参 (true/false)' },
  { key: 'STRATEGY_LLM_TIMEOUT_SEC', label: 'LLM超时(秒)' },
]

const systemSettingDefaults = {
  AI_MODEL: 'chat-model',
  PY_STRATEGY_URL: 'http://127.0.0.1:9000',
  MODE: 'web',
  HTTP_ADDR: ':8080',
  TRADE_DB_PATH: 'data/trade.db',
  ENABLE_WS_MARKET: 'true',
  REALTIME_MIN_INTERVAL_SEC: '10',
  STRATEGY_LLM_ENABLED: 'false',
  STRATEGY_LLM_TIMEOUT_SEC: '8',
}

function mergeSystemDefaults(raw) {
  const out = { ...(raw || {}) }
  for (const f of envFieldDefs) {
    if ((out[f.key] || '').trim() === '' && systemSettingDefaults[f.key] !== undefined) {
      out[f.key] = systemSettingDefaults[f.key]
    }
  }
  return out
}

function pickNum(obj, keys, fallback = 0) {
  for (const k of keys) {
    if (obj && obj[k] !== undefined && obj[k] !== null) {
      const n = Number(obj[k])
      if (!Number.isNaN(n)) return n
    }
  }
  return fallback
}

function normalizeKlines(raw) {
  if (!Array.isArray(raw)) return []
  return raw
    .map((item) => ({
      open: pickNum(item, ['open', 'Open']),
      high: pickNum(item, ['high', 'High']),
      low: pickNum(item, ['low', 'Low']),
      close: pickNum(item, ['close', 'Close']),
      ts: item?.timestamp || item?.Timestamp || '',
    }))
    .filter((k) => k.high > 0 && k.low > 0 && k.open > 0 && k.close > 0)
}

function KlineChart({ data }) {
  const klines = normalizeKlines(data)
  if (!klines.length) {
    return <p className="kline-empty">暂无 K 线数据</p>
  }

  const width = 760
  const height = 220
  const pad = 16
  const innerW = width - pad * 2
  const innerH = height - pad * 2
  const highMax = Math.max(...klines.map((k) => k.high))
  const lowMin = Math.min(...klines.map((k) => k.low))
  const priceSpan = Math.max(highMax - lowMin, 1e-9)
  const step = innerW / klines.length
  const bodyW = Math.max(3, step * 0.58)
  const xOf = (i) => pad + step * i + step / 2
  const yOf = (price) => pad + ((highMax - price) / priceSpan) * innerH

  return (
    <svg viewBox={`0 0 ${width} ${height}`} className="kline-svg" role="img" aria-label="realtime-kline">
      <rect x="0" y="0" width={width} height={height} rx="10" fill="#fbfcfe" />
      {klines.map((k, i) => {
        const x = xOf(i)
        const yHigh = yOf(k.high)
        const yLow = yOf(k.low)
        const yOpen = yOf(k.open)
        const yClose = yOf(k.close)
        const up = k.close >= k.open
        const bodyTop = Math.min(yOpen, yClose)
        const bodyH = Math.max(2, Math.abs(yClose - yOpen))
        const color = up ? '#0f9d77' : '#d04949'
        return (
          <g key={`${k.ts}-${i}`}>
            <line x1={x} y1={yHigh} x2={x} y2={yLow} stroke={color} strokeWidth="1.5" />
            <rect x={x - bodyW / 2} y={bodyTop} width={bodyW} height={bodyH} fill={color} rx="1.5" />
          </g>
        )
      })}
    </svg>
  )
}

export default function App() {
  const [status, setStatus] = useState({})
  const [account, setAccount] = useState({})
  const [signals, setSignals] = useState([])
  const [strategyScores, setStrategyScores] = useState([])
  const [loading, setLoading] = useState(false)
  const [runningNow, setRunningNow] = useState(false)
  const [savingSettings, setSavingSettings] = useState(false)
  const [savingSystemSettings, setSavingSystemSettings] = useState(false)
  const [error, setError] = useState('')
  const [activeTab, setActiveTab] = useState('system')
  const [initialized, setInitialized] = useState(false)

  const [settings, setSettings] = useState({
    positionSizingMode: 'contracts',
    highConfidenceAmount: 0.01,
    lowConfidenceAmount: 0.005,
    highConfidenceMarginPct: 10,
    lowConfidenceMarginPct: 5,
    leverage: 10,
  })
  const [systemSettings, setSystemSettings] = useState({ ...systemSettingDefaults })

  const schedulerRunning = useMemo(
    () => Boolean(status?.scheduler_running),
    [status?.scheduler_running]
  )

  const refreshAll = async (opts = {}) => {
    const silent = Boolean(opts.silent)
    if (!silent) {
      setLoading(true)
      setError('')
    }
    try {
      const [statusRes, accountRes, signalRes, scoreRes] = await Promise.all([
        getStatus(),
        getAccount(),
        getSignals(30),
        getStrategyScores(20),
      ])

      setStatus(statusRes.data || {})
      setAccount(accountRes.data || {})
      setSignals([...(signalRes?.data?.signals || [])].reverse())
      setStrategyScores(scoreRes?.data?.scores || statusRes?.data?.strategy_scores || [])

      const cfg = statusRes?.data?.trade_config || {}
      if (!silent || !initialized) {
        setSettings({
          positionSizingMode: String(cfg.position_sizing_mode || 'contracts'),
          highConfidenceAmount: Number(cfg.high_confidence_amount || 0.01),
          lowConfidenceAmount: Number(cfg.low_confidence_amount || 0.005),
          highConfidenceMarginPct: Number(cfg.high_confidence_margin_pct || 0.1) * 100,
          lowConfidenceMarginPct: Number(cfg.low_confidence_margin_pct || 0.05) * 100,
          leverage: Number(cfg.leverage || 10),
        })
      }

      if (!initialized) {
        try {
          const systemRes = await getSystemSettings()
          setSystemSettings(mergeSystemDefaults(systemRes?.data?.settings || {}))
        } catch {
          setSystemSettings((v) => mergeSystemDefaults(v))
        }
        setInitialized(true)
      }
    } catch (e) {
      if (!silent) {
        setError(e?.response?.data?.error || e?.message || '请求失败')
      }
    } finally {
      if (!silent) {
        setLoading(false)
      }
    }
  }

  const saveConfig = async () => {
    setSavingSettings(true)
    setError('')
    try {
      await updateSettings({
        position_sizing_mode: String(settings.positionSizingMode || 'contracts'),
        high_confidence_amount: clamp(settings.highConfidenceAmount, 0, 1000000),
        low_confidence_amount: clamp(settings.lowConfidenceAmount, 0, 1000000),
        high_confidence_margin_pct: clamp(settings.highConfidenceMarginPct, 0, 100) / 100,
        low_confidence_margin_pct: clamp(settings.lowConfidenceMarginPct, 0, 100) / 100,
        leverage: Math.round(clamp(settings.leverage, 1, 150)),
      })
      await refreshAll()
    } catch (e) {
      setError(e?.response?.data?.error || e?.message || '保存失败')
    } finally {
      setSavingSettings(false)
    }
  }

  const runOneCycle = async () => {
    setRunningNow(true)
    setError('')
    try {
      await runNow()
      await refreshAll()
    } catch (e) {
      setError(e?.response?.data?.error || e?.message || '执行失败')
    } finally {
      setRunningNow(false)
    }
  }

  const saveSystemEnv = async () => {
    setSavingSystemSettings(true)
    setError('')
    try {
      const payload = {}
      for (const f of envFieldDefs) {
        payload[f.key] = String(systemSettings?.[f.key] || '')
      }
      const res = await saveSystemSettings(payload)
      setSystemSettings(mergeSystemDefaults(res?.data?.settings || payload))
    } catch (e) {
      setError(e?.response?.data?.error || e?.message || '系统设置保存失败')
    } finally {
      setSavingSystemSettings(false)
    }
  }

  const toggleScheduler = async (start) => {
    setError('')
    try {
      if (start) {
        await startScheduler()
      } else {
        await stopScheduler()
      }
      await refreshAll()
    } catch (e) {
      setError(e?.response?.data?.error || e?.message || '调度操作失败')
    }
  }

  useEffect(() => {
    refreshAll()
    const timer = setInterval(() => refreshAll({ silent: true }), 15000)
    return () => clearInterval(timer)
  }, [initialized])

  return (
    <div className="page">
      <header className="hero">
        <div className="title-block">
          <h1><span className="ai-word">21XG</span><br />AI 合约</h1>
        </div>
      </header>

      {error ? <p className="error">{error}</p> : null}

      <section className="layout">
        <section className="card settings settings-wide">
          <h2>参数设置</h2>
          <div className="settings-layout">
            <div>
              <div className="form-grid">
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
                        onChange={(e) =>
                          setSettings((v) => ({ ...v, highConfidenceAmount: Number(e.target.value) }))
                        }
                      />
                    </label>
                    <label>
                      <span>低信心张数</span>
                      <input
                        type="number"
                        min="0"
                        step="0.0001"
                        value={settings.lowConfidenceAmount}
                        onChange={(e) =>
                          setSettings((v) => ({ ...v, lowConfidenceAmount: Number(e.target.value) }))
                        }
                      />
                    </label>
                  </>
                ) : (
                  <>
                    <label>
                      <span>高信心占比%</span>
                      <input
                        type="number"
                        min="0"
                        max="100"
                        step="0.1"
                        value={settings.highConfidenceMarginPct}
                        onChange={(e) =>
                          setSettings((v) => ({ ...v, highConfidenceMarginPct: Number(e.target.value) }))
                        }
                      />
                    </label>
                    <label>
                      <span>低信心占比%</span>
                      <input
                        type="number"
                        min="0"
                        max="100"
                        step="0.1"
                        value={settings.lowConfidenceMarginPct}
                        onChange={(e) =>
                          setSettings((v) => ({ ...v, lowConfidenceMarginPct: Number(e.target.value) }))
                        }
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
              <div className="settings-actions">
                <button disabled={savingSettings} onClick={saveConfig}>确认</button>
              </div>
            </div>
          </div>
        </section>

        <aside className="tab-sidebar card">
          <h2>视图</h2>
          <div className="tabs">
            <button className={activeTab === 'system' ? 'tab active' : 'tab'} onClick={() => setActiveTab('system')}>系统状态</button>
            <button className={activeTab === 'records' ? 'tab active' : 'tab'} onClick={() => setActiveTab('records')}>交易记录</button>
            <button className={activeTab === 'scores' ? 'tab active' : 'tab'} onClick={() => setActiveTab('scores')}>策略评分</button>
            <button className={activeTab === 'syscfg' ? 'tab active' : 'tab'} onClick={() => setActiveTab('syscfg')}>系统设置</button>
          </div>
        </aside>

        <div className="middle-panel">
          {activeTab === 'system' ? (
            <section className="cards">
              <article className="card">
                <h2>系统状态</h2>
                <p>调度器：{schedulerRunning ? '运行中' : '已停止'}</p>
                <p>下次执行：{fmtTime(status?.next_run_at)}</p>
                <p>上次执行：{fmtTime(status?.runtime?.last_run_at)}</p>
                <p>上次错误：{status?.runtime?.last_error || '无'}</p>
              </article>
              <article className="card">
                <h2>账户信息</h2>
                <p>余额：{fmtNum(account?.balance)} USDT</p>
                <p>持仓方向：{account?.position?.side || '无'}</p>
                <p>持仓数量：{fmtNum(account?.position?.size, 4)}</p>
                <p>未实现盈亏：{fmtNum(account?.position?.unrealizedPnL)} USDT</p>
              </article>
              <article className="card">
                <h2>交易配置</h2>
                <p>交易对：{status?.trade_config?.symbol || '-'}</p>
                <p>周期：{status?.trade_config?.timeframe || '-'}</p>
                <p>仓位模式：{status?.trade_config?.position_sizing_mode === 'margin_pct' ? '保证金百分比' : '按张数'}</p>
                <p>杠杆：{status?.trade_config?.leverage || '-'}x</p>
                <p>高信心仓位：{fmtNum(status?.trade_config?.high_confidence_amount, 4)}</p>
                <p>低信心仓位：{fmtNum(status?.trade_config?.low_confidence_amount, 4)}</p>
              </article>
              <article className="card">
                <h2>最新信号</h2>
                <p>方向：{status?.runtime?.last_signal?.signal || '-'}</p>
                <p>信心：{status?.runtime?.last_signal?.confidence || '-'}</p>
                <p>策略组合：{status?.runtime?.last_signal?.strategy_combo || '-'}</p>
                <p>组合评分：{fmtNum(status?.runtime?.last_signal?.strategy_score)} / 10</p>
                <p>理由：{status?.runtime?.last_signal?.reason || '-'}</p>
              </article>
              <article className="card kline-card">
                <h2>实时 K 线图</h2>
                <KlineChart data={status?.runtime?.last_price?.KlineData || status?.runtime?.last_price?.kline_data} />
                <p className="kline-meta">最近刷新：{fmtTime(status?.runtime?.last_price?.Timestamp || status?.runtime?.last_price?.timestamp)}</p>
              </article>
            </section>
          ) : null}

          {activeTab === 'records' ? (
            <section className="card table-wrap record-card">
              <h2>交易记录</h2>
              <table>
                <thead>
                  <tr>
                    <th>时间</th>
                    <th>信号</th>
                    <th>信心</th>
                    <th>策略组合</th>
                    <th>盈亏</th>
                    <th>止损</th>
                    <th>止盈</th>
                    <th>原因</th>
                  </tr>
                </thead>
                <tbody>
                  {signals.map((item) => (
                    <tr key={`${item.timestamp}-${item.signal}-${item.reason}`}>
                      <td>{fmtTime(item.timestamp)}</td>
                      <td>{item.signal}</td>
                      <td>{item.confidence}</td>
                      <td>{item.strategy_combo || '-'}</td>
                      <td>{item.pnl === undefined || item.pnl === null ? '-' : `${fmtNum(item.pnl)} USDT`}</td>
                      <td>{fmtNum(item.stop_loss)}</td>
                      <td>{fmtNum(item.take_profit)}</td>
                      <td className="reason">{item.reason}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </section>
          ) : null}

          {activeTab === 'scores' ? (
            <section className="card table-wrap">
              <h2>策略组合评分（最高10分）</h2>
              <table>
                <thead>
                  <tr>
                    <th>策略组合</th>
                    <th>评分</th>
                    <th>累计盈亏(USDT)</th>
                    <th>样本数</th>
                    <th>胜/负</th>
                  </tr>
                </thead>
                <tbody>
                  {strategyScores.map((item) => (
                    <tr key={`${item.combo}-${item.updated_at}`}>
                      <td>{item.combo}</td>
                      <td>{fmtNum(item.score)} / 10</td>
                      <td>{fmtNum(item.total_pnl)}</td>
                      <td>{item.observations}</td>
                      <td>{item.wins}/{item.losses}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </section>
          ) : null}

          {activeTab === 'syscfg' ? (
            <section className="card table-wrap">
              <h2>系统设置</h2>
              <div className="env-grid">
                {envFieldDefs.map((f) => (
                  <label key={f.key}>
                    <span>{f.label}</span>
                    <input
                      type={f.type || 'text'}
                      value={systemSettings?.[f.key] || ''}
                      onChange={(e) => setSystemSettings((v) => ({ ...v, [f.key]: e.target.value }))}
                    />
                  </label>
                ))}
              </div>
              <div className="settings-actions">
                <button disabled={savingSystemSettings} onClick={saveSystemEnv}>保存系统设置</button>
              </div>
            </section>
          ) : null}
        </div>

        <aside className="right-panel card">
          <h2>操作</h2>
          <div className="actions action-stack">
            <button disabled={loading} onClick={refreshAll}>刷新</button>
            <button disabled={runningNow} onClick={runOneCycle}>手动执行1次</button>
            <button disabled={loading || schedulerRunning} onClick={() => toggleScheduler(true)}>启动调度</button>
            <button className="danger" disabled={loading || !schedulerRunning} onClick={() => toggleScheduler(false)}>停止</button>
          </div>
        </aside>

      </section>
    </div>
  )
}
