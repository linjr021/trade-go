// @ts-nocheck
import { ActionButton } from '@/components/ui/action-button'
import { Space, Tabs } from '@/components/ui/dashboard-primitives'
import { TradeRecordsTable } from '@/modules/trade-tables'

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
    toggleScheduler,
    schedulerRunning,
    savingSettings,
    saveLiveConfig,
    liveViewTab,
    setLiveViewTab,
    renderOverviewCards,
    liveStrategyLabel,
    tradeRecords,
  } = p

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
            <ActionButton className="btn-flat btn-flat-emerald" onClick={() => toggleScheduler(true)} disabled={schedulerRunning}>
              启动调度
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
        <p className="muted">仅本地模拟与记录，不会调用交易所下单，也不会占用真实账户资金。</p>
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
    paperTradeRecords,
  } = p

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
            <span>模拟保证金(USDT)</span>
            <input
              type="number"
              min="0"
              step="1"
              value={paperMargin}
              onChange={(e) => setPaperMargin(Number(e.target.value))}
            />
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
              ),
            },
            {
              key: 'records',
              label: '交易记录',
              children: (
                <div className="builder-pane">
                  <TradeRecordsTable records={paperTradeRecords.filter((r) => !r.symbol || r.symbol === paperPair)} />
                </div>
              ),
            },
          ]}
        />
      </section>
    </section>
  )
}
