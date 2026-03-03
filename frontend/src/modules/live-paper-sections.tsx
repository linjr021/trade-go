import { useEffect, useMemo, useState } from 'react'
import { ActionButton } from '@/components/ui/action-button'
import { Space, Tabs } from '@/components/ui/dashboard-primitives'
import { TradeRecordsTable } from '@/modules/trade-tables'

function formatBeijingDateTime(value) {
  const raw = String(value || '').trim()
  if (!raw || raw === '-') return '-'
  const d = new Date(raw)
  if (Number.isNaN(d.getTime())) return raw
  const parts = new Intl.DateTimeFormat('zh-CN', {
    timeZone: 'Asia/Shanghai',
    hour12: false,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).formatToParts(d)
  const map: Record<string, string> = {}
  for (const p of parts) map[p.type] = p.value
  return `${map.year || '0000'}-${map.month || '00'}-${map.day || '00'} ${map.hour || '00'}:${map.minute || '00'}:${map.second || '00'}`
}

function formatBeijingClock(value) {
  const full = formatBeijingDateTime(value)
  if (full === '-' || full.length < 19) return full
  return full.slice(11, 19)
}

function getStrategyMeta(name, strategyMetaMap = {}, row = null) {
  const strategyName = String(name || '').trim()
  const fromHistory = row?.meta?.[strategyName]
  const fromMap = strategyMetaMap?.[strategyName]
  const meta = fromHistory || fromMap || {}
  const source = String(meta?.source || '').trim() || '外部/手动'
  const workflowVersion = String(meta?.workflowVersion || meta?.workflow_version || '').trim() || '-'
  const lastUpdatedAt = String(meta?.lastUpdatedAt || meta?.last_updated_at || '').trim() || '-'
  return { source, workflowVersion, lastUpdatedAt }
}

function fmtPrice(value, digits = 2) {
  const n = Number(value || 0)
  if (!Number.isFinite(n) || n <= 0) return '-'
  return n.toFixed(digits)
}

function positionModeText(mode) {
  const m = String(mode || '').trim().toLowerCase()
  if (m === 'contracts') return '按张数'
  return '按保证金百分比'
}

function formatConfidence(value) {
  const v = String(value || '').trim().toUpperCase()
  if (v === 'HIGH') return '高'
  if (v === 'MEDIUM') return '中'
  if (v === 'LOW') return '低'
  return v || '-'
}

function StrategyRuntimeTab({
  currentStrategies = [],
  strategyHistory = [],
  strategyMetaMap = {},
  nextOrderPreview = null,
}) {
  const rows = Array.isArray(strategyHistory) ? strategyHistory.slice(0, 20) : []
  const currentList = Array.isArray(currentStrategies) ? currentStrategies : []
  const [selectedHistoryID, setSelectedHistoryID] = useState('')
  useEffect(() => {
    const firstID = String(rows?.[0]?.id || '')
    if (!rows.length) {
      setSelectedHistoryID('')
      return
    }
    const exists = rows.some((row) => String(row?.id || '') === String(selectedHistoryID || ''))
    if (!exists) {
      setSelectedHistoryID(firstID)
    }
  }, [rows, selectedHistoryID])
  const selectedHistory = useMemo(
    () => rows.find((row) => String(row?.id || '') === String(selectedHistoryID || '')) || rows[0] || null,
    [rows, selectedHistoryID],
  )
  const selectedParams = selectedHistory?.params || {}
  const preview = nextOrderPreview || {}
  const previewSignal = String(preview?.signal || '-').toUpperCase() || '-'
  const previewReason = String(preview?.reason || '').trim() || '-'
  const previewStrategy = String(preview?.strategyCombo || '').trim() || '-'
  const previewPrice = fmtPrice(preview?.price, 2)
  const previewSL = fmtPrice(preview?.stopLoss, 2)
  const previewTP = fmtPrice(preview?.takeProfit, 2)
  const previewApproved = typeof preview?.approved === 'boolean' ? preview.approved : null
  const previewExecuted = typeof preview?.executed === 'boolean' ? preview.executed : null
  const previewApprovedText = previewApproved == null ? '-' : (previewApproved ? '可开仓' : '风控阻断')
  const previewExecutedText = previewExecuted == null ? '-' : (previewExecuted ? '已执行' : '未执行')
  const previewApprovedSize = fmtPrice(preview?.approvedSize, 4)
  const previewSuggestedSize = fmtPrice(preview?.suggestedSize, 4)

  return (
    <div className="builder-pane">
      <section className="sub-window strategy-runtime-window">
        <h3>策略执行</h3>
        <div className="strategy-runtime-current">
          <span>正在执行策略</span>
          {currentList.length ? (
            <div className="strategy-runtime-current-list">
              {currentList.map((name) => {
                const meta = getStrategyMeta(name, strategyMetaMap)
                return (
                  <div key={`current-${name}`} className="strategy-runtime-item">
                    <b title={String(name || '-')}>{String(name || '-')}</b>
                    <p>
                      来源：{meta.source} ｜ 工作流：{meta.workflowVersion} ｜ 最后升级：{formatBeijingDateTime(meta.lastUpdatedAt)}
                    </p>
                  </div>
                )
              })}
            </div>
          ) : (
            <b>-</b>
          )}
        </div>
        <div className="strategy-preview-card">
          <h4>下一单开仓策略预览</h4>
          <div className="strategy-preview-grid">
            <p><span>方向</span><b>{previewSignal}</b></p>
            <p><span>参考价格</span><b>{previewPrice}</b></p>
            <p><span>止损价</span><b>{previewSL}</b></p>
            <p><span>止盈价</span><b>{previewTP}</b></p>
            <p><span>信心</span><b>{formatConfidence(preview?.confidence)}</b></p>
            <p><span>策略组合</span><b>{previewStrategy}</b></p>
            <p><span>开仓判定</span><b>{previewApprovedText}</b></p>
            <p><span>执行状态</span><b>{previewExecutedText}</b></p>
            <p><span>建议数量</span><b>{previewSuggestedSize}</b></p>
            <p><span>风控后数量</span><b>{previewApprovedSize}</b></p>
            <p className="full"><span>理由</span><b>{previewReason}</b></p>
          </div>
        </div>
        <div className="strategy-runtime-history">
          <h4>历史交易策略</h4>
          <div className="strategy-history-mini">
            {rows.length ? rows.map((row) => {
              const ts = String(row?.ts || '').trim()
              const timeText = formatBeijingClock(ts)
              const list = Array.isArray(row?.strategies) ? row.strategies : []
              const name = list.length ? list.join(' / ') : '-'
              const firstMeta = list.length ? getStrategyMeta(list[0], strategyMetaMap, row) : null
              const isActive = String(selectedHistoryID || '') === String(row?.id || '')
              return (
                <button
                  type="button"
                  className={`strategy-history-entry ${isActive ? 'active' : ''}`}
                  key={String(row?.id || `${ts}-${name}`)}
                  onClick={() => setSelectedHistoryID(String(row?.id || ''))}
                >
                  <p>
                    <span>{timeText}</span>
                    <b>{name}</b>
                  </p>
                  {firstMeta ? (
                    <small>来源：{firstMeta.source} ｜ 工作流：{firstMeta.workflowVersion} ｜ 最后升级：{formatBeijingDateTime(firstMeta.lastUpdatedAt)}</small>
                  ) : null}
                </button>
              )
            }) : <p className="muted">暂无历史策略</p>}
          </div>
          {selectedHistory ? (
            <div className="strategy-history-detail">
              <h4>当时生效参数</h4>
              <div className="strategy-history-param-grid">
                <p><span>杠杆</span><b>{Number(selectedParams?.leverage || 0) > 0 ? `${selectedParams.leverage}x` : '-'}</b></p>
                <p><span>仓位模式</span><b>{positionModeText(selectedParams?.positionSizingMode)}</b></p>
                {String(selectedParams?.positionSizingMode || '').toLowerCase() === 'contracts' ? (
                  <>
                    <p><span>高信心张数</span><b>{fmtPrice(selectedParams?.highConfidenceAmount, 2)}</b></p>
                    <p><span>低信心张数</span><b>{fmtPrice(selectedParams?.lowConfidenceAmount, 2)}</b></p>
                  </>
                ) : (
                  <>
                    <p><span>高信心保证金%</span><b>{fmtPrice(selectedParams?.highConfidenceMarginPct, 2)}%</b></p>
                    <p><span>低信心保证金%</span><b>{fmtPrice(selectedParams?.lowConfidenceMarginPct, 2)}%</b></p>
                  </>
                )}
              </div>
            </div>
          ) : null}
        </div>
      </section>
    </div>
  )
}

export function LivePageSection(p) {
  const {
    activePair,
    setActivePair,
    pairs,
    strategyPickerRef,
    strategyPickerOpen,
    setStrategyPickerOpen,
    enabledStrategies,
    setStrategyDraft,
    selectedStrategyText,
    executionStrategyOptions,
    strategyDraft,
    toggleStrategyDraft,
    confirmStrategySelection,
    settings,
    setSettings,
    normalizeDecimal,
    normalizeLeverage,
    refreshCore,
    runningNow,
    runOneCycle,
    startLiveTrading,
    startingLive,
    toggleScheduler,
    schedulerRunning,
    savingSettings,
    saveLiveConfig,
    status,
    liveViewTab,
    setLiveViewTab,
    renderOverviewCards,
    strategyMetaMap,
    liveStrategyLabel,
    liveStrategyHistory,
    liveMarketSnapshot,
    tradeRecords,
  } = p
  const liveNextOrderPreview = useMemo(() => {
    const runtime = status?.runtime || {}
    const sig = runtime?.last_signal || {}
    const priceData = runtime?.last_price || {}
    const marketPrice = Number(liveMarketSnapshot?.price || 0)
    const orderPreview = status?.next_order_preview || {}
    const previewSignal = String(orderPreview?.signal || sig?.signal || '').trim().toUpperCase() || '-'
    const previewConfidence = String(orderPreview?.confidence || sig?.confidence || '').trim().toUpperCase() || '-'
    const previewStrategy = String((orderPreview?.strategy_combo ?? orderPreview?.strategyCombo ?? sig?.strategy_combo ?? sig?.strategyCombo) || '').trim() || '-'
    const previewApproved = orderPreview?.approved
    const previewExecuted = orderPreview?.executed
    const previewApprovedSize = Number(orderPreview?.approved_size ?? orderPreview?.approvedSize ?? 0)
    const previewSuggestedSize = Number(orderPreview?.suggested_size ?? orderPreview?.suggestedSize ?? 0)
    const previewRiskReason = String((orderPreview?.risk_reason ?? orderPreview?.riskReason) || '').trim()
    const previewReasonText = String(orderPreview?.reason || sig?.reason || '').trim()
    const finalReason = previewRiskReason || previewReasonText || (previewSignal === 'HOLD' ? '信号不足，保持观望' : '')
    return {
      signal: previewSignal,
      price: Number(orderPreview?.price || 0) > 0
        ? Number(orderPreview?.price || 0)
        : (marketPrice > 0 ? marketPrice : Number(priceData?.price || 0)),
      stopLoss: Number(orderPreview?.stop_loss ?? orderPreview?.stopLoss ?? sig?.stop_loss ?? sig?.stopLoss ?? 0),
      takeProfit: Number(orderPreview?.take_profit ?? orderPreview?.takeProfit ?? sig?.take_profit ?? sig?.takeProfit ?? 0),
      confidence: previewConfidence,
      reason: finalReason,
      strategyCombo: previewStrategy,
      approved: typeof previewApproved === 'boolean' ? previewApproved : null,
      executed: typeof previewExecuted === 'boolean' ? previewExecuted : null,
      approvedSize: Number.isFinite(previewApprovedSize) ? previewApprovedSize : 0,
      suggestedSize: Number.isFinite(previewSuggestedSize) ? previewSuggestedSize : 0,
    }
  }, [status, liveMarketSnapshot])

  return (
    <section className="stack">
      <section className="card">
        <h3>实盘交易</h3>
        <div className="form-grid wide">
          <label>
            <span>交易对</span>
            <select value={activePair} onChange={(e) => setActivePair(e.target.value)}>
              {pairs.map((pair) => <option key={pair} value={pair}>{pair}</option>)}
            </select>
          </label>
          <label>
            <span>交易策略（多选）</span>
            <div className="strategy-picker" ref={strategyPickerRef}>
              <button
                type="button"
                className="strategy-picker-trigger"
                title={selectedStrategyText}
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
                      className="btn-flat btn-flat-slate"
                      onClick={(e) => {
                        e.preventDefault()
                        setStrategyPickerOpen(false)
                      }}
                    >
                      取消
                    </button>
                    <button
                      type="button"
                      className="btn-flat btn-flat-blue"
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
                  step="0.01"
                  value={settings.highConfidenceAmount}
                  onChange={(e) => setSettings((v) => ({ ...v, highConfidenceAmount: Number(e.target.value) }))}
                  onBlur={() => setSettings((v) => ({
                    ...v,
                    highConfidenceAmount: normalizeDecimal(v.highConfidenceAmount, 0, 1000000),
                  }))}
                />
              </label>
              <label>
                <span>低信心张数</span>
                <input
                  type="number"
                  min="0"
                  step="0.01"
                  value={settings.lowConfidenceAmount}
                  onChange={(e) => setSettings((v) => ({ ...v, lowConfidenceAmount: Number(e.target.value) }))}
                  onBlur={() => setSettings((v) => ({
                    ...v,
                    lowConfidenceAmount: normalizeDecimal(v.lowConfidenceAmount, 0, 1000000),
                  }))}
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
                  step="0.01"
                  value={settings.highConfidenceMarginPct}
                  onChange={(e) => setSettings((v) => ({ ...v, highConfidenceMarginPct: Number(e.target.value) }))}
                  onBlur={() => setSettings((v) => ({
                    ...v,
                    highConfidenceMarginPct: normalizeDecimal(v.highConfidenceMarginPct, 0, 100),
                  }))}
                />
              </label>
              <label>
                <span>低信心保证金%</span>
                <input
                  type="number"
                  min="0"
                  max="100"
                  step="0.01"
                  value={settings.lowConfidenceMarginPct}
                  onChange={(e) => setSettings((v) => ({ ...v, lowConfidenceMarginPct: Number(e.target.value) }))}
                  onBlur={() => setSettings((v) => ({
                    ...v,
                    lowConfidenceMarginPct: normalizeDecimal(v.lowConfidenceMarginPct, 0, 100),
                  }))}
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
              onBlur={() => setSettings((v) => ({ ...v, leverage: normalizeLeverage(v.leverage) }))}
            />
          </label>
        </div>

        <div className="actions-row">
          <Space wrap>
            <ActionButton className="btn-flat btn-flat-sky" onClick={() => refreshCore(false)}>刷新</ActionButton>
            <ActionButton className="btn-flat btn-flat-cyan" loading={runningNow} onClick={runOneCycle}>
              {runningNow ? '执行中...' : '手动执行1次'}
            </ActionButton>
            <ActionButton className="btn-flat btn-flat-emerald" loading={startingLive} onClick={startLiveTrading}>
              {startingLive ? '启动中...' : '开始'}
            </ActionButton>
            <ActionButton className="btn-flat btn-flat-rose" onClick={() => toggleScheduler(false)} disabled={!schedulerRunning}>
              停止
            </ActionButton>
            <ActionButton
              className={`btn-flat btn-flat-blue save-config-btn ${savingSettings ? 'is-saving' : ''}`}
              onClick={saveLiveConfig}
              loading={savingSettings}
            >
              {savingSettings ? '保存中...' : '确认'}
            </ActionButton>
          </Space>
        </div>
      </section>

      <section className="card">
        <Tabs
          className="dashboard-tabs"
          activeKey={liveViewTab}
          onChange={setLiveViewTab}
          items={[
            {
              key: 'overview',
              label: '交易总览',
              children: renderOverviewCards(activePair, liveStrategyLabel),
            },
            {
              key: 'strategy',
              label: '策略执行',
              children: (
                <StrategyRuntimeTab
                  currentStrategies={enabledStrategies}
                  strategyHistory={liveStrategyHistory}
                  strategyMetaMap={strategyMetaMap}
                  nextOrderPreview={liveNextOrderPreview}
                />
              ),
            },
            {
              key: 'records',
              label: '交易记录',
              children: (
                <div className="builder-pane">
                  <TradeRecordsTable records={tradeRecords.filter((r) => !r.symbol || r.symbol === activePair)} />
                </div>
              ),
            },
          ]}
        />
      </section>
    </section>
  )
}

export function PaperPageSection(p) {
  const {
    paperPair,
    setPaperPair,
    pairs,
    paperStrategyPickerRef,
    paperStrategyPickerOpen,
    setPaperStrategyPickerOpen,
    paperStrategySelection,
    setPaperStrategyDraft,
    paperSelectedStrategyText,
    executionStrategyOptions,
    paperStrategyDraft,
    togglePaperStrategyDraft,
    confirmPaperStrategySelection,
    paperMargin,
    setPaperMargin,
    paperSettings,
    setPaperSettings,
    normalizeDecimal,
    normalizeLeverage,
    startPaperSim,
    paperSimLoading,
    paperSimRunning,
    pausePaperSim,
    paperViewTab,
    setPaperViewTab,
    renderOverviewCards,
    strategyMetaMap,
    paperTradeRecords,
    paperLatestDecision,
    paperStrategyHistory,
    paperPnlBaselineMap,
    resetPaperCurrentPnL,
  } = p
  const filteredPaperRecords = paperTradeRecords.filter((r) => !r.symbol || r.symbol === paperPair)
  const paperRawTotalPnL = filteredPaperRecords.reduce((sum, row) => sum + Number(row?.unrealized_pnl || 0), 0)
  const paperPnlBaseline = Number(paperPnlBaselineMap?.[paperPair] || 0)
  const paperTotalPnL = Number(paperRawTotalPnL - paperPnlBaseline)
  const paperWins = filteredPaperRecords.reduce((cnt, row) => cnt + (Number(row?.unrealized_pnl || 0) > 0 ? 1 : 0), 0)
  const paperLosses = filteredPaperRecords.reduce((cnt, row) => cnt + (Number(row?.unrealized_pnl || 0) < 0 ? 1 : 0), 0)
  const paperPnlRatio = paperLosses === 0 ? (paperWins > 0 ? '∞' : '0') : (paperWins / paperLosses).toFixed(2)
  const lastPaperRecord = filteredPaperRecords[0]
  const lastSignal = String(lastPaperRecord?.signal || '').toUpperCase()
  const lastConfidence = String(lastPaperRecord?.confidence || '').toUpperCase()
  const paperMarketEmotion = lastSignal === 'BUY'
    ? (lastConfidence === 'HIGH' ? '强偏多' : '偏多')
    : lastSignal === 'SELL'
      ? (lastConfidence === 'HIGH' ? '强偏空' : '偏空')
      : '中性'
  const oldestPaperRecordTS = filteredPaperRecords.reduce((minTS, row) => {
    const ts = Date.parse(String(row?.ts || ''))
    if (!Number.isFinite(ts) || ts <= 0) return minTS
    return Math.min(minTS, ts)
  }, Number.POSITIVE_INFINITY)
  const paperDurationMinutes = Number.isFinite(oldestPaperRecordTS)
    ? Math.max(0, Math.floor((Date.now() - oldestPaperRecordTS) / 60000))
    : 0
  const paperStrategyDurationText = Number.isFinite(oldestPaperRecordTS)
    ? (paperDurationMinutes >= 60
      ? `${Math.floor(paperDurationMinutes / 60)}h ${paperDurationMinutes % 60}m`
      : `${paperDurationMinutes}m`)
    : (paperSimRunning ? '运行中(<1m)' : '0m')
  const paperAccountSnapshot = {
    balance: Number(paperMargin || 0),
    position: {
      side: lastSignal || '无',
    },
  }
  const paperNextOrderPreview = useMemo(() => {
    const latest = paperLatestDecision && typeof paperLatestDecision === 'object' ? paperLatestDecision : null
    const src = latest || lastPaperRecord || {}
    const signal = String(src?.signal || lastSignal || '').toUpperCase() || '-'
    const confidence = String(src?.confidence || lastConfidence || '').toUpperCase() || '-'
    const reason = String((src?.risk_reason ?? src?.riskReason ?? src?.reason) || '').trim()
    const combo = String((src?.strategy_combo ?? src?.strategyCombo) || '').trim()
    const approvedSize = Number(src?.approved_size ?? src?.approvedSize ?? 0)
    const executedCode = String(src?.execution_code ?? src?.executionCode ?? '').trim().toLowerCase()
    return {
      signal,
      price: Number(src?.price || 0),
      stopLoss: Number(src?.stop_loss ?? src?.stopLoss ?? 0),
      takeProfit: Number(src?.take_profit ?? src?.takeProfit ?? 0),
      confidence,
      reason: reason || '本地模拟规则（价格动量）',
      strategyCombo: combo || paperSelectedStrategyText || '-',
      approved: typeof src?.approved === 'boolean' ? src.approved : null,
      executed: executedCode === ''
        ? null
        : (executedCode === 'paper_simulated' || executedCode === 'simulated'),
      approvedSize: Number.isFinite(approvedSize) ? approvedSize : 0,
      suggestedSize: Number.isFinite(approvedSize) ? approvedSize : 0,
    }
  }, [paperLatestDecision, lastPaperRecord, lastSignal, lastConfidence, paperSelectedStrategyText])

  return (
    <section className="stack">
      <section className="card">
        <h3>模拟交易</h3>
        <div className="form-grid wide">
          <label>
            <span>交易对</span>
            <select value={paperPair} onChange={(e) => setPaperPair(e.target.value)}>
              {pairs.map((pair) => <option key={pair} value={pair}>{pair}</option>)}
            </select>
          </label>
          <label>
            <span>交易策略（多选）</span>
            <div className="strategy-picker" ref={paperStrategyPickerRef}>
              <button
                type="button"
                className="strategy-picker-trigger"
                title={paperSelectedStrategyText}
                onClick={(e) => {
                  e.preventDefault()
                  if (!paperStrategyPickerOpen) setPaperStrategyDraft(paperStrategySelection)
                  setPaperStrategyPickerOpen((v) => !v)
                }}
              >
                {paperSelectedStrategyText}
              </button>
              {paperStrategyPickerOpen ? (
                <div className="strategy-picker-menu">
                  <div className="strategy-picker-list">
                    {executionStrategyOptions.map((s) => (
                      <label key={`paper-strategy-pick-${s}`} className="strategy-picker-item">
                        <input
                          type="checkbox"
                          checked={paperStrategyDraft.includes(s)}
                          disabled={!paperStrategyDraft.includes(s) && paperStrategyDraft.length >= 3}
                          onChange={() => togglePaperStrategyDraft(s)}
                        />
                        <span>{s}</span>
                      </label>
                    ))}
                  </div>
                  <div className="actions-row end">
                    <button
                      type="button"
                      className="btn-flat btn-flat-slate"
                      onClick={(e) => {
                        e.preventDefault()
                        setPaperStrategyPickerOpen(false)
                      }}
                    >
                      取消
                    </button>
                    <button
                      type="button"
                      className="btn-flat btn-flat-blue"
                      onClick={(e) => {
                        e.preventDefault()
                        confirmPaperStrategySelection()
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
              value={paperSettings.positionSizingMode}
              onChange={(e) => setPaperSettings((v) => ({ ...v, positionSizingMode: e.target.value }))}
            >
              <option value="contracts">按张数</option>
              <option value="margin_pct">按保证金百分比</option>
            </select>
          </label>
          {paperSettings.positionSizingMode === 'contracts' ? (
            <>
              <label>
                <span>高信心张数</span>
                <input
                  type="number"
                  min="0"
                  step="0.01"
                  value={paperSettings.highConfidenceAmount}
                  onChange={(e) => setPaperSettings((v) => ({ ...v, highConfidenceAmount: Number(e.target.value) }))}
                  onBlur={() => setPaperSettings((v) => ({
                    ...v,
                    highConfidenceAmount: normalizeDecimal(v.highConfidenceAmount, 0, 1000000),
                  }))}
                />
              </label>
              <label>
                <span>低信心张数</span>
                <input
                  type="number"
                  min="0"
                  step="0.01"
                  value={paperSettings.lowConfidenceAmount}
                  onChange={(e) => setPaperSettings((v) => ({ ...v, lowConfidenceAmount: Number(e.target.value) }))}
                  onBlur={() => setPaperSettings((v) => ({
                    ...v,
                    lowConfidenceAmount: normalizeDecimal(v.lowConfidenceAmount, 0, 1000000),
                  }))}
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
                  step="0.01"
                  value={paperSettings.highConfidenceMarginPct}
                  onChange={(e) => setPaperSettings((v) => ({ ...v, highConfidenceMarginPct: Number(e.target.value) }))}
                  onBlur={() => setPaperSettings((v) => ({
                    ...v,
                    highConfidenceMarginPct: normalizeDecimal(v.highConfidenceMarginPct, 0, 100),
                  }))}
                />
              </label>
              <label>
                <span>低信心保证金%</span>
                <input
                  type="number"
                  min="0"
                  max="100"
                  step="0.01"
                  value={paperSettings.lowConfidenceMarginPct}
                  onChange={(e) => setPaperSettings((v) => ({ ...v, lowConfidenceMarginPct: Number(e.target.value) }))}
                  onBlur={() => setPaperSettings((v) => ({
                    ...v,
                    lowConfidenceMarginPct: normalizeDecimal(v.lowConfidenceMarginPct, 0, 100),
                  }))}
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
              value={paperSettings.leverage}
              onChange={(e) => setPaperSettings((v) => ({ ...v, leverage: Number(e.target.value) }))}
              onBlur={() => setPaperSettings((v) => ({ ...v, leverage: normalizeLeverage(v.leverage) }))}
            />
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
        <div className="actions-row">
          <Space wrap>
            <ActionButton
              className="btn-flat btn-flat-emerald"
              onClick={startPaperSim}
              loading={paperSimLoading}
              disabled={paperSimRunning}
            >
              开始模拟
            </ActionButton>
            <ActionButton className="btn-flat btn-flat-amber" onClick={pausePaperSim} disabled={!paperSimRunning}>
              暂停模拟
            </ActionButton>
            <ActionButton
              className="btn-flat btn-flat-sky"
              onClick={resetPaperCurrentPnL}
              disabled={!filteredPaperRecords.length}
            >
              重置当前盈亏
            </ActionButton>
          </Space>
        </div>
      </section>
      <section className="card">
        <Tabs
          className="dashboard-tabs"
          activeKey={paperViewTab}
          onChange={setPaperViewTab}
          items={[
            {
              key: 'overview',
              label: '交易总览',
              children: renderOverviewCards(
                paperPair,
                paperSelectedStrategyText,
                <section className="sub-window"><h3>模拟交易补充</h3><p>该板块不发真实订单，仅用于策略参数演练与回测准备。</p></section>,
                {
                  marketEmotion: paperMarketEmotion,
                  totalPnL: paperTotalPnL,
                  account: paperAccountSnapshot,
                  strategyDurationText: paperStrategyDurationText,
                  pnlRatio: paperPnlRatio,
                },
              ),
            },
            {
              key: 'strategy',
              label: '策略执行',
              children: (
                <StrategyRuntimeTab
                  currentStrategies={paperStrategySelection}
                  strategyHistory={paperStrategyHistory}
                  strategyMetaMap={strategyMetaMap}
                  nextOrderPreview={paperNextOrderPreview}
                />
              ),
            },
            {
              key: 'records',
              label: '交易记录',
              children: (
                <div className="builder-pane">
                  <TradeRecordsTable records={filteredPaperRecords} />
                </div>
              ),
            },
          ]}
        />
      </section>
    </section>
  )
}
