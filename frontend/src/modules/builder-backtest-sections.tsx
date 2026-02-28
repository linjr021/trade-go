// @ts-nocheck
import { ActionButton } from '@/components/ui/action-button'
import { Tabs } from '@/components/ui/dashboard-primitives'
import { MonthSelect } from '@/modules/month-select'
import { StrategyBacktestTable } from '@/modules/trade-tables'

export function BuilderPageSection(p) {
  const {
    builderTab,
    setBuilderTab,
    strategyGenMode,
    habit,
    setHabit,
    habitOptions,
    genPair,
    setGenPair,
    pairs,
    genStyle,
    setGenStyle,
    genMinRR,
    setGenMinRR,
    genLowConfAction,
    setGenLowConfAction,
    genDirectionBias,
    setGenDirectionBias,
    genAllowReversal,
    setGenAllowReversal,
    generateStrategy,
    generatingStrategy,
    setUploadFile,
    loadStrategyTemplate,
    loadingTemplate,
    copyStrategyTemplate,
    uploadStrategy,
    uploadingStrategy,
    strategyTemplate,
    generatedStrategies,
    selectedRuleId,
    setSelectedRuleId,
    selectedRule,
    renameRuleName,
    setRenameRuleName,
    renameGeneratedStrategy,
    deleteGeneratedStrategy,
    renameHanCount,
    fmtNum,
  } = p

  return (
    <section className="stack">
      <section className="card">
        <Tabs
          className="dashboard-tabs"
          activeKey={builderTab}
          onChange={setBuilderTab}
          items={[
            { key: 'generate', label: '策略生成' },
            { key: 'rules', label: '策略规则' },
          ]}
        />
        {strategyGenMode === 'llm' ? (
          <div className="strategy-gen-hint llm">
            使用智能体生成
          </div>
        ) : null}
        {strategyGenMode === 'fallback' ? (
          <div className="strategy-gen-hint fallback">
            智能体未接入或调用失败，使用模板回退生成
          </div>
        ) : null}

        {builderTab === 'generate' && (
          <div className="builder-pane">
            <section className="sub-window">
              <h4>策略生成参数</h4>
              <div className="form-grid">
                <label>
                  <span>交易习惯时长</span>
                  <select value={habit} onChange={(e) => setHabit(e.target.value)}>
                    {habitOptions.map((h) => <option key={h} value={h}>{h}</option>)}
                  </select>
                </label>
                <label>
                  <span>交易对</span>
                  <select value={genPair} onChange={(e) => setGenPair(e.target.value)}>
                    {pairs.map((pair) => <option key={`gen-${pair}`} value={pair}>{pair}</option>)}
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
                <ActionButton className="btn-flat btn-flat-purple" onClick={generateStrategy} loading={generatingStrategy}>
                  {generatingStrategy ? '生成中...' : '生成策略'}
                </ActionButton>
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
                <ActionButton className="btn-flat btn-flat-cyan" onClick={loadStrategyTemplate} loading={loadingTemplate}>
                  {loadingTemplate ? '加载中...' : '加载模板策略脚本'}
                </ActionButton>
                <ActionButton className="btn-flat btn-flat-amber" onClick={copyStrategyTemplate}>复制模板</ActionButton>
                <ActionButton className="btn-flat btn-flat-emerald" onClick={uploadStrategy} loading={uploadingStrategy}>
                  {uploadingStrategy ? '上传中...' : '上传策略'}
                </ActionButton>
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
          <div className="builder-pane rule-layout">
            <section className="rule-list-panel">
              <div className="rule-list-head">
                <h4>策略列表</h4>
                <span>{generatedStrategies.length} 条</span>
              </div>
              <div className="rule-list-scroll">
                {generatedStrategies.map((s) => (
                  <div key={s.id} className={`rule-item ${selectedRuleId === s.id ? 'active' : ''}`}>
                    <button
                      type="button"
                      className={`rule-tag ${selectedRuleId === s.id ? 'active' : ''}`}
                      onClick={() => setSelectedRuleId(s.id)}
                      title={s.name}
                    >
                      {s.name}
                    </button>
                  </div>
                ))}
                {!generatedStrategies.length ? (
                  <p className="muted">请先在“策略生成”中创建策略。</p>
                ) : null}
              </div>
            </section>

            <section className="rule-detail-panel">
              <h4>策略详情</h4>
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
                    <ActionButton className="btn-flat btn-flat-blue" onClick={renameGeneratedStrategy}>
                      确认改名
                    </ActionButton>
                    <ActionButton className="btn-flat btn-flat-rose" onClick={() => deleteGeneratedStrategy(selectedRule.id)}>
                      删除规则
                    </ActionButton>
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
              ) : (
                <div className="rule-detail rule-empty">
                  <p className="muted">请选择左侧策略查看详情。</p>
                </div>
              )}
            </section>
          </div>
        )}

      </section>
    </section>
  )
}

export function BacktestPageSection(p) {
  const {
    btStrategyPickerRef,
    btStrategyPickerOpen,
    setBtStrategyPickerOpen,
    btStrategySelection,
    setBtStrategyDraft,
    btSelectedStrategyText,
    executionStrategyOptions,
    btStrategyDraft,
    toggleBtStrategyDraft,
    confirmBtStrategySelection,
    btPair,
    setBtPair,
    pairs,
    btInitialMargin,
    setBtInitialMargin,
    btLeverage,
    setBtLeverage,
    normalizeLeverage,
    btPositionSizingMode,
    setBtPositionSizingMode,
    btHighConfidenceAmount,
    setBtHighConfidenceAmount,
    btLowConfidenceAmount,
    setBtLowConfidenceAmount,
    btHighConfidenceMarginPct,
    setBtHighConfidenceMarginPct,
    btLowConfidenceMarginPct,
    setBtLowConfidenceMarginPct,
    normalizeDecimal,
    btStart,
    setBtStart,
    btEnd,
    setBtEnd,
    backtestMinMonth,
    backtestMaxMonth,
    runBacktest,
    btRunning,
    btHistoryLoading,
    btHistory,
    btHistorySelectedId,
    viewBacktestHistoryDetail,
    fmtNum,
    fmtTime,
    selectedBacktestHistory,
    fmtPct,
    btSummary,
    btHistoryDeletingId,
    removeBacktestHistory,
    btRecords,
  } = p

  return (
    <section className="stack">
      <section className="card">
        <h3>历史回测</h3>
        <div className="builder-pane">
          <div className="form-grid backtest-grid">
            <label className="bt-strategy">
              <span>交易策略（多选）</span>
              <div className="strategy-picker" ref={btStrategyPickerRef}>
                <button
                  type="button"
                  className="strategy-picker-trigger"
                  onClick={(e) => {
                    e.preventDefault()
                    if (!btStrategyPickerOpen) setBtStrategyDraft(btStrategySelection)
                    setBtStrategyPickerOpen((v) => !v)
                  }}
                >
                  {btSelectedStrategyText}
                </button>
                {btStrategyPickerOpen ? (
                  <div className="strategy-picker-menu">
                    <div className="strategy-picker-list">
                      {executionStrategyOptions.map((s) => (
                        <label key={`bt-strategy-pick-${s}`} className="strategy-picker-item">
                          <input
                            type="checkbox"
                            checked={btStrategyDraft.includes(s)}
                            disabled={!btStrategyDraft.includes(s) && btStrategyDraft.length >= 3}
                            onChange={() => toggleBtStrategyDraft(s)}
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
                          setBtStrategyPickerOpen(false)
                        }}
                      >
                        取消
                      </button>
                      <button
                        type="button"
                        className="btn-flat btn-flat-blue"
                        onClick={(e) => {
                          e.preventDefault()
                          confirmBtStrategySelection()
                        }}
                      >
                        确认
                      </button>
                    </div>
                  </div>
                ) : null}
              </div>
            </label>
            <label className="bt-pair">
              <span>交易对</span>
              <select value={btPair} onChange={(e) => setBtPair(e.target.value)}>
                {pairs.map((pair) => <option key={pair} value={pair}>{pair}</option>)}
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
                onBlur={() => setBtLeverage((v) => normalizeLeverage(v))}
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
                        step="0.01"
                        value={btHighConfidenceAmount}
                        onChange={(e) => setBtHighConfidenceAmount(Number(e.target.value))}
                        onBlur={() => setBtHighConfidenceAmount((v) => normalizeDecimal(v, 0, 1000000))}
                      />
                    </label>
                    <label>
                      <span>低信心张数</span>
                      <input
                        type="number"
                        min="0"
                        step="0.01"
                        value={btLowConfidenceAmount}
                        onChange={(e) => setBtLowConfidenceAmount(Number(e.target.value))}
                        onBlur={() => setBtLowConfidenceAmount((v) => normalizeDecimal(v, 0, 1000000))}
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
                        step="0.01"
                        value={btHighConfidenceMarginPct}
                        onChange={(e) => setBtHighConfidenceMarginPct(Number(e.target.value))}
                        onBlur={() => setBtHighConfidenceMarginPct((v) => normalizeDecimal(v, 0, 100))}
                      />
                    </label>
                    <label>
                      <span>低信心仓位%</span>
                      <input
                        type="number"
                        min="0"
                        max="100"
                        step="0.01"
                        value={btLowConfidenceMarginPct}
                        onChange={(e) => setBtLowConfidenceMarginPct(Number(e.target.value))}
                        onBlur={() => setBtLowConfidenceMarginPct((v) => normalizeDecimal(v, 0, 100))}
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
                  min={backtestMinMonth}
                  max={backtestMaxMonth}
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
                  min={backtestMinMonth}
                  max={backtestMaxMonth}
                  onChange={(v) => {
                    setBtEnd(v < btStart ? btStart : v)
                  }}
                />
              </label>
            </div>
          </div>
          <div className="actions-row end">
            <ActionButton className="btn-flat btn-flat-orange" onClick={runBacktest} loading={btRunning}>
              {btRunning ? '回测中...' : '开始回测'}
            </ActionButton>
          </div>

          <section className="sub-window">
            <div className="card-head">
              <h4>回测记录</h4>
              <span>{btHistoryLoading ? '加载中...' : `共 ${btHistory.length} 条`}</span>
            </div>
            {!btHistory.length ? (
              <p className="muted">暂无回测记录</p>
            ) : (
              <div className="rule-layout history-layout">
                <section className="rule-list-panel history-list-panel">
                  <div className="rule-list-head">
                    <h4>回测列表</h4>
                    <span>{btHistory.length} 条</span>
                  </div>
                  <div className="rule-list-scroll history-list-scroll">
                    {btHistory.map((row) => {
                      const rowID = Number(row?.id || 0)
                      const selected = rowID > 0 && rowID === Number(btHistorySelectedId || 0)
                      return (
                        <button
                          key={`bth-${rowID}`}
                          type="button"
                          className={`rule-tag history-tag ${selected ? 'active' : ''}`}
                          onClick={() => viewBacktestHistoryDetail(rowID)}
                          title={`#${rowID || '-'} · ${row?.strategy || '-'} · ${row?.pair || '-'} · ${row?.start || '-'} - ${row?.end || '-'}`}
                        >
                          <span className="history-tag-head">
                            <span className="history-tag-title">#{rowID || '-'} · {row?.strategy || '-'}</span>
                            <span className={`history-tag-pnl ${Number(row?.total_pnl || 0) >= 0 ? 'up' : 'down'}`}>
                              {fmtNum(row?.total_pnl, 2)} USDT
                            </span>
                          </span>
                          <span className="history-tag-sub">{fmtTime(row?.created_at)}</span>
                        </button>
                      )
                    })}
                  </div>
                </section>

                <section className="rule-detail-panel history-detail-panel">
                  {selectedBacktestHistory ? (
                    <div className="rule-detail history-detail">
                      <h4>回测详情</h4>
                      <div className="summary-grid history-meta-grid">
                        <article className="metric-card"><h4>记录 ID</h4><p>#{Number(selectedBacktestHistory?.id || 0)}</p></article>
                        <article className="metric-card"><h4>创建时间</h4><p>{fmtTime(selectedBacktestHistory?.created_at)}</p></article>
                        <article className="metric-card"><h4>策略</h4><p>{selectedBacktestHistory?.strategy || '-'}</p></article>
                        <article className="metric-card"><h4>交易对</h4><p>{selectedBacktestHistory?.pair || '-'}</p></article>
                        <article className="metric-card"><h4>总盈亏</h4><p className={Number(selectedBacktestHistory?.total_pnl || 0) >= 0 ? 'up' : 'down'}>{fmtNum(selectedBacktestHistory?.total_pnl, 2)} USDT</p></article>
                        <article className="metric-card"><h4>总收益</h4><p className={Number(selectedBacktestHistory?.return_pct || 0) >= 0 ? 'up' : 'down'}>{fmtPct(selectedBacktestHistory?.return_pct)}</p></article>
                      </div>
                      <div className="inline-actions">
                        <ActionButton
                          className="btn-flat btn-flat-slate"
                          loading={btHistoryLoading}
                          onClick={() => viewBacktestHistoryDetail(Number(selectedBacktestHistory?.id || 0))}
                        >
                          {btHistoryLoading ? '加载中...' : '查看明细'}
                        </ActionButton>
                        <ActionButton
                          className="btn-flat btn-flat-rose"
                          loading={btHistoryDeletingId === Number(selectedBacktestHistory?.id || 0)}
                          onClick={() => removeBacktestHistory(Number(selectedBacktestHistory?.id || 0))}
                        >
                          {btHistoryDeletingId === Number(selectedBacktestHistory?.id || 0) ? '删除中...' : '删除'}
                        </ActionButton>
                      </div>
                      {Number(btSummary?.historyId || 0) === Number(selectedBacktestHistory?.id || 0) ? (
                        <>
                          <div className="summary-grid history-summary-grid">
                            <article className="metric-card"><h4>初始保证金</h4><p>{fmtNum(btSummary.initialMargin, 2)} USDT</p></article>
                            <article className="metric-card"><h4>合约倍率</h4><p>{btSummary.leverage || '-'}x</p></article>
                            <article className="metric-card"><h4>仓位模式</h4><p>{btSummary.positionSizingMode === 'margin_pct' ? '按百分比仓位' : '按张数'}</p></article>
                            {btSummary.positionSizingMode === 'margin_pct' ? (
                              <article className="metric-card"><h4>高/低信心仓位%</h4><p>{fmtNum(btSummary.highConfidenceMarginPct, 2)} / {fmtNum(btSummary.lowConfidenceMarginPct, 2)}</p></article>
                            ) : (
                              <article className="metric-card"><h4>高/低信心张数</h4><p>{fmtNum(btSummary.highConfidenceAmount, 2)} / {fmtNum(btSummary.lowConfidenceAmount, 2)}</p></article>
                            )}
                            <article className="metric-card"><h4>期末权益</h4><p className={btSummary.finalEquity >= 0 ? 'up' : 'down'}>{fmtNum(btSummary.finalEquity, 2)} USDT</p></article>
                            <article className="metric-card"><h4>回测时长</h4><p>{btSummary.start} - {btSummary.end}</p></article>
                            <article className="metric-card"><h4>盈亏比</h4><p>{btSummary.ratioInfinite ? '∞' : fmtNum(btSummary.ratio, 2)}</p></article>
                            <article className="metric-card"><h4>胜/负</h4><p>{btSummary.wins}/{btSummary.losses}</p></article>
                            <article className="metric-card"><h4>数据条数</h4><p>{btSummary.bars}</p></article>
                          </div>
                          <StrategyBacktestTable records={btRecords} />
                        </>
                      ) : (
                        <p className="muted">点击“查看明细”加载该回测记录的交易明细。</p>
                      )}
                    </div>
                  ) : (
                    <div className="rule-detail rule-empty">
                      <p className="muted">请选择左侧回测记录查看详情。</p>
                    </div>
                  )}
                </section>
              </div>
            )}
          </section>
        </div>
      </section>
    </section>
  )
}
