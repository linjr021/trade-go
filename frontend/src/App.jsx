import { useEffect, useMemo, useState } from 'react'
import {
  getAccount,
  getSignals,
  getStatus,
  runNow,
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

export default function App() {
  const [status, setStatus] = useState({})
  const [account, setAccount] = useState({})
  const [signals, setSignals] = useState([])
  const [loading, setLoading] = useState(false)
  const [runningNow, setRunningNow] = useState(false)
  const [savingSettings, setSavingSettings] = useState(false)
  const [error, setError] = useState('')

  const [settings, setSettings] = useState({
    highConfidenceAmount: 0.01,
    lowConfidenceAmount: 0.005,
    leverage: 10,
  })

  const schedulerRunning = useMemo(
    () => Boolean(status?.scheduler_running),
    [status?.scheduler_running]
  )

  const refreshAll = async () => {
    setLoading(true)
    setError('')
    try {
      const [statusRes, accountRes, signalRes] = await Promise.all([
        getStatus(),
        getAccount(),
        getSignals(30),
      ])

      setStatus(statusRes.data || {})
      setAccount(accountRes.data || {})
      setSignals([...(signalRes?.data?.signals || [])].reverse())

      const cfg = statusRes?.data?.trade_config || {}
      setSettings({
        highConfidenceAmount: Number(cfg.high_confidence_amount || 0.01),
        lowConfidenceAmount: Number(cfg.low_confidence_amount || 0.005),
        leverage: Number(cfg.leverage || 10),
      })
    } catch (e) {
      setError(e?.response?.data?.error || e?.message || '请求失败')
    } finally {
      setLoading(false)
    }
  }

  const saveConfig = async () => {
    setSavingSettings(true)
    setError('')
    try {
      await updateSettings({
        high_confidence_amount: Number(settings.highConfidenceAmount),
        low_confidence_amount: Number(settings.lowConfidenceAmount),
        leverage: Number(settings.leverage),
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
    const timer = setInterval(refreshAll, 15000)
    return () => clearInterval(timer)
  }, [])

  return (
    <div className="page">
      <header className="hero">
        <div className="title-block">
          <h1><span className="ai-word">AI</span><br />交易看板</h1>
        </div>
        <div className="actions">
          <button disabled={loading} onClick={refreshAll}>刷新</button>
          <button disabled={runningNow} onClick={runOneCycle}>手动执行一次</button>
          {!schedulerRunning ? (
            <button disabled={loading} onClick={() => toggleScheduler(true)}>启动调度</button>
          ) : (
            <button className="danger" disabled={loading} onClick={() => toggleScheduler(false)}>停止调度</button>
          )}
        </div>
      </header>

      {error ? <p className="error">{error}</p> : null}

      <section className="cards">
        <article className="card">
          <h2>交易配置</h2>
          <p>交易对：{status?.trade_config?.symbol || '-'}</p>
          <p>周期：{status?.trade_config?.timeframe || '-'}</p>
          <p>杠杆：{status?.trade_config?.leverage || '-'}x</p>
          <p>高信心仓位：{fmtNum(status?.trade_config?.high_confidence_amount, 4)}</p>
          <p>低信心仓位：{fmtNum(status?.trade_config?.low_confidence_amount, 4)}</p>
        </article>

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
          <h2>最新信号</h2>
          <p>方向：{status?.runtime?.last_signal?.signal || '-'}</p>
          <p>信心：{status?.runtime?.last_signal?.confidence || '-'}</p>
          <p>理由：{status?.runtime?.last_signal?.reason || '-'}</p>
        </article>
      </section>

      <section className="card settings">
        <h2>参数设置</h2>
        <div className="form-grid">
          <label>
            <span>高信心开仓数量</span>
            <input
              type="number"
              min="0.0001"
              step="0.0001"
              value={settings.highConfidenceAmount}
              onChange={(e) =>
                setSettings((v) => ({ ...v, highConfidenceAmount: Number(e.target.value) }))
              }
            />
          </label>
          <label>
            <span>低信心开仓数量</span>
            <input
              type="number"
              min="0.0001"
              step="0.0001"
              value={settings.lowConfidenceAmount}
              onChange={(e) =>
                setSettings((v) => ({ ...v, lowConfidenceAmount: Number(e.target.value) }))
              }
            />
          </label>
          <label>
            <span>杠杆</span>
            <input
              type="number"
              min="1"
              step="1"
              value={settings.leverage}
              onChange={(e) => setSettings((v) => ({ ...v, leverage: Number(e.target.value) }))}
            />
          </label>
          <button disabled={savingSettings} onClick={saveConfig}>保存设置</button>
        </div>
      </section>

      <section className="card table-wrap record-card">
        <h2>交易记录</h2>
        <table>
          <thead>
            <tr>
              <th>时间</th>
              <th>信号</th>
              <th>信心</th>
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
                <td>{fmtNum(item.stop_loss)}</td>
                <td>{fmtNum(item.take_profit)}</td>
                <td className="reason">{item.reason}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>
    </div>
  )
}
