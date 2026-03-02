// @ts-nocheck
import { ActionButton } from '@/components/ui/action-button'
import { Tabs } from '@/components/ui/dashboard-primitives'
import { MonthSelect } from '@/modules/month-select'
import { StrategyBacktestTable } from '@/modules/trade-tables'

const WORKFLOW_STEP_LABEL_MAP = {
  'spec-builder': '规格构建',
  'strategy-draft': '策略草案',
  optimizer: '参数优化',
  'risk-reviewer': '风险复核',
  'release-packager': '发布打包',
  SpecBuilder: '规格构建',
  StrategyDraft: '策略草案',
  Optimizer: '参数优化',
  RiskReviewer: '风险复核',
  ReleasePackager: '发布打包',
}

const WORKFLOW_CHANNEL_LABEL_MAP = {
  strategy_generator: '策略生成',
  chat_assistant: '参数助手',
  default: '默认',
}

function workflowLabel(value) {
  const raw = String(value || '').trim()
  if (!raw) return '-'
  return WORKFLOW_STEP_LABEL_MAP[raw] || raw
}

function workflowChainText(steps) {
  if (!Array.isArray(steps) || !steps.length) return '-'
  return steps.map((step) => workflowLabel(step)).join(' -> ')
}

function workflowChannelText(channel) {
  const raw = String(channel || '').trim()
  if (!raw) return '-'
  return WORKFLOW_CHANNEL_LABEL_MAP[raw] || raw
}

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
                  <span>交易习惯时长（核心输入）</span>
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
                  <p><b>最小盈亏比（请求值）：</b>{fmtNum(selectedRule.minRR, 2)}</p>
                  <p><b>最小盈亏比（最终生效）：</b>{
                    selectedRule?.skillPackage?.spec_builder?.hard_constraints?.min_profit_loss_ratio != null
                      ? fmtNum(Number(selectedRule.skillPackage.spec_builder.hard_constraints.min_profit_loss_ratio), 2)
                      : fmtNum(selectedRule.minRR, 2)
                  }</p>
                  <p><b>允许反转：</b>{selectedRule.allowReversal ? '是' : '否'}</p>
                  <p><b>低信心处理：</b>{selectedRule.lowConfAction || '-'}</p>
                  <p><b>方向偏好：</b>{selectedRule.directionBias || '-'}</p>
                  <p><b>工作流版本：</b>{selectedRule?.skillPackage?.version || '-'}</p>
                  <p><b>工作流链路：</b>{workflowChainText(selectedRule?.skillPackage?.workflow)}</p>
                  <p><b>交易习惯画像：</b>{selectedRule?.skillPackage?.habit_profile?.label || '-'} / 周期 {selectedRule?.skillPackage?.habit_profile?.timeframe || '-'}</p>
                  <p><b>硬边界：</b>
                    最大杠杆 {selectedRule?.skillPackage?.spec_builder?.hard_constraints?.max_leverage ?? '-'}，
                    最大回撤 {selectedRule?.skillPackage?.spec_builder?.hard_constraints?.max_drawdown_pct != null ? `${fmtNum(Number(selectedRule.skillPackage.spec_builder.hard_constraints.max_drawdown_pct) * 100, 2)}%` : '-'}，
                    单笔风险 {selectedRule?.skillPackage?.spec_builder?.hard_constraints?.max_risk_per_trade_pct != null ? `${fmtNum(Number(selectedRule.skillPackage.spec_builder.hard_constraints.max_risk_per_trade_pct) * 100, 2)}%` : '-'}
                  </p>
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

export function SkillWorkflowPageSection(p) {
  const {
    skillWorkflow,
    loadingSkillWorkflow,
    savingSkillWorkflow,
    aiWorkflowTab,
    setAiWorkflowTab,
    aiWorkflowLogs,
    aiWorkflowLogsLoading,
    aiWorkflowLogChannel,
    setAiWorkflowLogChannel,
    aiWorkflowLogLimit,
    setAiWorkflowLogLimit,
    autoReviewFields,
    systemSettings,
    setSystemSettings,
    autoReviewSaveHint,
    savingAutoReviewSettings,
    saveAutoReviewEnv,
    updateSkillStepField,
    updateSkillConstraintField,
    updateSkillPromptField,
    saveSkillWorkflowConfig,
    resetSkillWorkflowConfig,
    loadSkillWorkflowConfig,
    loadAIWorkflowLogs,
    fmtNum,
    fmtTime,
  } = p

  return (
    <section className="stack">
      <section className="card">
        <h3>AI 工作流</h3>
        <Tabs
          className="dashboard-tabs"
          activeKey={aiWorkflowTab}
          onChange={setAiWorkflowTab}
          items={[
            { key: 'config', label: '流程配置' },
            { key: 'auto_review', label: '自动评估' },
            { key: 'prompts', label: '提示词' },
            { key: 'logs', label: '执行记录' },
          ]}
        />
        {aiWorkflowTab === 'config' && (
          <div className="builder-pane workflow-pane">
            <section className="sub-window">
              <div className="card-head">
                <h4>流程图</h4>
                <div className="inline-actions">
                  <ActionButton
                    className="btn-flat btn-flat-sky"
                    loading={loadingSkillWorkflow}
                    onClick={() => loadSkillWorkflowConfig(false)}
                  >
                    {loadingSkillWorkflow ? '刷新中...' : '刷新'}
                  </ActionButton>
                  <ActionButton
                    className="btn-flat btn-flat-amber"
                    loading={savingSkillWorkflow}
                    onClick={resetSkillWorkflowConfig}
                  >
                    {savingSkillWorkflow ? '处理中...' : '恢复默认'}
                  </ActionButton>
                  <ActionButton
                    className="btn-flat btn-flat-purple"
                    loading={savingSkillWorkflow}
                    onClick={saveSkillWorkflowConfig}
                  >
                    {savingSkillWorkflow ? '保存中...' : '保存流程参数'}
                  </ActionButton>
                </div>
              </div>
              <div className="workflow-flow">
                {(skillWorkflow?.steps || []).map((step, idx, arr) => (
                  <div key={`wf-${step.id}-${idx}`} className="workflow-chain-item">
                    <article className={`workflow-node ${step.enabled ? 'is-enabled' : 'is-disabled'}`}>
                      <div className="workflow-node-head">
                        <b>{workflowLabel(step.name || step.id)}</b>
                        <span>标识：{step.id}</span>
                      </div>
                      <p>{step.description || '-'}</p>
                      <div className="workflow-node-meta">
                        <span>超时 {step.timeout_sec}s</span>
                        <span>重试 {step.max_retry}</span>
                        <span>{step.on_fail === 'hard_fail' ? '失败即中断' : '失败回退HOLD'}</span>
                      </div>
                    </article>
                    {idx < arr.length - 1 ? <div className="workflow-arrow">→</div> : null}
                  </div>
                ))}
                {!Array.isArray(skillWorkflow?.steps) || !skillWorkflow.steps.length ? (
                  <p className="muted">暂无工作流步骤。</p>
                ) : null}
              </div>
            </section>

            <section className="sub-window">
              <h4>步骤参数</h4>
              <div className="workflow-steps-grid">
                {(skillWorkflow?.steps || []).map((step) => (
                  <article key={`wf-edit-${step.id}`} className="workflow-step-editor">
                    <div className="workflow-step-editor-title">
                      <b>{workflowLabel(step.name || step.id)}</b>
                      <span>标识：{step.id}</span>
                    </div>
                    <div className="workflow-step-fields">
                      <label className="workflow-check">
                        <input
                          type="checkbox"
                          checked={Boolean(step.enabled)}
                          onChange={(e) => updateSkillStepField(step.id, 'enabled', e.target.checked)}
                        />
                        <span>启用</span>
                      </label>
                      <label>
                        <span>超时(秒)</span>
                        <input
                          type="number"
                          min="1"
                          max="300"
                          step="1"
                          value={Number(step.timeout_sec || 1)}
                          onChange={(e) => updateSkillStepField(step.id, 'timeout_sec', Number(e.target.value))}
                        />
                      </label>
                      <label>
                        <span>重试次数</span>
                        <input
                          type="number"
                          min="0"
                          max="5"
                          step="1"
                          value={Number(step.max_retry || 0)}
                          onChange={(e) => updateSkillStepField(step.id, 'max_retry', Number(e.target.value))}
                        />
                      </label>
                      <label>
                        <span>失败动作</span>
                        <select
                          value={String(step.on_fail || 'hold')}
                          onChange={(e) => updateSkillStepField(step.id, 'on_fail', e.target.value)}
                        >
                          <option value="hold">回退 HOLD</option>
                          <option value="hard_fail">阻断执行</option>
                        </select>
                      </label>
                    </div>
                  </article>
                ))}
              </div>
            </section>

            <section className="sub-window">
              <h4>硬边界参数</h4>
              <div className="form-grid workflow-constraints-grid">
                <label>
                  <span>最大杠杆上限</span>
                  <input
                    type="number"
                    min="1"
                    max="150"
                    step="1"
                    value={Number(skillWorkflow?.constraints?.max_leverage_cap || 150)}
                    onChange={(e) => updateSkillConstraintField('max_leverage_cap', Number(e.target.value))}
                  />
                </label>
                <label>
                  <span>最大回撤上限(%)</span>
                  <input
                    type="number"
                    min="1"
                    max="80"
                    step="0.1"
                    value={fmtNum(Number(skillWorkflow?.constraints?.max_drawdown_cap_pct || 0) * 100, 2)}
                    onChange={(e) => updateSkillConstraintField('max_drawdown_cap_pct', Number(e.target.value) / 100)}
                  />
                </label>
                <label>
                  <span>单笔风险上限(%)</span>
                  <input
                    type="number"
                    min="0.1"
                    max="20"
                    step="0.1"
                    value={fmtNum(Number(skillWorkflow?.constraints?.max_risk_per_trade_cap_pct || 0) * 100, 2)}
                    onChange={(e) => updateSkillConstraintField('max_risk_per_trade_cap_pct', Number(e.target.value) / 100)}
                  />
                </label>
                <label>
                  <span>最小盈亏比下限</span>
                  <input
                    type="number"
                    min="1"
                    max="10"
                    step="0.1"
                    value={Number(skillWorkflow?.constraints?.min_profit_loss_floor || 1.5)}
                    onChange={(e) => updateSkillConstraintField('min_profit_loss_floor', Number(e.target.value))}
                  />
                </label>
                <label>
                  <span>步骤失败后禁止下单</span>
                  <select
                    value={skillWorkflow?.constraints?.block_trade_on_skill_fail ? 'true' : 'false'}
                    onChange={(e) => updateSkillConstraintField('block_trade_on_skill_fail', e.target.value === 'true')}
                  >
                    <option value="true">是</option>
                    <option value="false">否</option>
                  </select>
                </label>
              </div>
            </section>
          </div>
        )}
        {aiWorkflowTab === 'auto_review' && (
          <div className="builder-pane workflow-pane">
            <section className="sub-window">
              <div className="card-head">
                <h4>自动评估参数</h4>
                <div className="inline-actions">
                  <ActionButton
                    className={`btn-flat btn-flat-blue save-config-btn ${savingAutoReviewSettings ? 'is-saving' : ''}`}
                    onClick={saveAutoReviewEnv}
                    loading={savingAutoReviewSettings}
                  >
                    {savingAutoReviewSettings ? '保存中...' : '保存自动评估参数'}
                  </ActionButton>
                </div>
              </div>
              <div className="form-grid workflow-constraints-grid">
                {(autoReviewFields || []).map((f) => {
                  const key = String(f?.key || '').trim()
                  const isBool = key === 'AUTO_REVIEW_ENABLED' || key === 'AUTO_REVIEW_AFTER_ORDER_ONLY' || key === 'AUTO_STRATEGY_REGEN_ENABLED'
                  const rawValue = String(systemSettings?.[key] || '')
                  return (
                    <label key={`auto-review-${key}`}>
                      <span>{f.label}</span>
                      {isBool ? (
                        <select
                          value={rawValue === 'false' ? 'false' : 'true'}
                          onChange={(e) => setSystemSettings((prev) => ({ ...prev, [key]: e.target.value }))}
                        >
                          <option value="true">true</option>
                          <option value="false">false</option>
                        </select>
                      ) : (
                        <input
                          type="number"
                          step="any"
                          value={rawValue}
                          onChange={(e) => setSystemSettings((prev) => ({ ...prev, [key]: e.target.value }))}
                        />
                      )}
                    </label>
                  )
                })}
              </div>
              <div className="actions-row end">
                {autoReviewSaveHint ? <span className="save-hint">{autoReviewSaveHint}</span> : null}
              </div>
            </section>
          </div>
        )}
        {aiWorkflowTab === 'prompts' && (
          <div className="builder-pane workflow-pane">
            <section className="sub-window">
              <div className="card-head">
                <h4>工作流提示词</h4>
                <div className="inline-actions">
                  <ActionButton
                    className="btn-flat btn-flat-sky"
                    loading={loadingSkillWorkflow}
                    onClick={() => loadSkillWorkflowConfig(false)}
                  >
                    {loadingSkillWorkflow ? '刷新中...' : '刷新'}
                  </ActionButton>
                  <ActionButton
                    className="btn-flat btn-flat-amber"
                    loading={savingSkillWorkflow}
                    onClick={resetSkillWorkflowConfig}
                  >
                    {savingSkillWorkflow ? '处理中...' : '恢复默认'}
                  </ActionButton>
                  <ActionButton
                    className="btn-flat btn-flat-purple"
                    loading={savingSkillWorkflow}
                    onClick={saveSkillWorkflowConfig}
                  >
                    {savingSkillWorkflow ? '保存中...' : '保存提示词'}
                  </ActionButton>
                </div>
              </div>
              <div className="workflow-prompts-grid">
                <label>
                  <span>策略生成系统提示词</span>
                  <textarea
                    value={String(skillWorkflow?.prompts?.strategy_generator_system_prompt || '')}
                    onChange={(e) => updateSkillPromptField('strategy_generator_system_prompt', e.target.value)}
                  />
                </label>
                <label>
                  <span>策略生成任务提示词</span>
                  <textarea
                    value={String(skillWorkflow?.prompts?.strategy_generator_task_prompt || '')}
                    onChange={(e) => updateSkillPromptField('strategy_generator_task_prompt', e.target.value)}
                  />
                </label>
                <label>
                  <span>策略生成约束清单（每行1条）</span>
                  <textarea
                    value={Array.isArray(skillWorkflow?.prompts?.strategy_generator_requirements)
                      ? skillWorkflow.prompts.strategy_generator_requirements.join('\n')
                      : ''}
                    onChange={(e) => updateSkillPromptField('strategy_generator_requirements', e.target.value)}
                  />
                </label>
                <label>
                  <span>实盘决策系统提示词</span>
                  <textarea
                    value={String(skillWorkflow?.prompts?.decision_system_prompt || '')}
                    onChange={(e) => updateSkillPromptField('decision_system_prompt', e.target.value)}
                  />
                </label>
                <label>
                  <span>实盘决策策略提示词</span>
                  <textarea
                    value={String(skillWorkflow?.prompts?.decision_policy_prompt || '')}
                    onChange={(e) => updateSkillPromptField('decision_policy_prompt', e.target.value)}
                  />
                </label>
              </div>
            </section>
          </div>
        )}
        {aiWorkflowTab === 'logs' && (
          <div className="builder-pane workflow-pane">
            <section className="sub-window">
              <div className="card-head">
                <h4>执行记录与令牌消耗</h4>
                <div className="inline-actions">
                  <ActionButton
                    className="btn-flat btn-flat-sky"
                    loading={aiWorkflowLogsLoading}
                    onClick={() => loadAIWorkflowLogs(false)}
                  >
                    {aiWorkflowLogsLoading ? '加载中...' : '刷新'}
                  </ActionButton>
                </div>
              </div>
              <div className="workflow-log-toolbar">
                <label>
                  <span>通道</span>
                  <select value={aiWorkflowLogChannel} onChange={(e) => setAiWorkflowLogChannel(e.target.value)}>
                    <option value="strategy_generator">策略生成</option>
                    <option value="chat_assistant">参数助手</option>
                    <option value="default">默认</option>
                    <option value="all">全部</option>
                  </select>
                </label>
                <label>
                  <span>记录条数</span>
                  <input
                    type="number"
                    min="1"
                    max="500"
                    step="1"
                    value={Number(aiWorkflowLogLimit || 50)}
                    onChange={(e) => setAiWorkflowLogLimit(Number(e.target.value))}
                  />
                </label>
              </div>
              {!aiWorkflowLogs.length ? (
                <p className="muted">暂无执行记录。</p>
              ) : (
                <div className="workflow-log-list">
                  {aiWorkflowLogs.map((log) => (
                    <article key={`wf-log-${log.id}-${log.created_at}`} className="workflow-log-item">
                      <div className="workflow-log-head">
                        <div className="workflow-log-meta">
                          <b>#{log.id} · {workflowChannelText(log.channel)}</b>
                          <span>{fmtTime(log.created_at)}</span>
                          <span>模型：{log.model || '-'}</span>
                        </div>
                        <div className="workflow-log-token">
                          <span>总令牌 {log.total_tokens ?? 0}</span>
                          <span>输入令牌 {log.prompt_tokens ?? 0}</span>
                          <span>输出令牌 {log.completion_tokens ?? 0}</span>
                        </div>
                      </div>
                      <details className="workflow-log-detail">
                        <summary>查看执行内容</summary>
                        <div className="workflow-log-content">
                          <div className="workflow-log-block">
                            <h5>输入内容</h5>
                            <pre>{log.prompt || '-'}</pre>
                          </div>
                          <div className="workflow-log-block">
                            <h5>输出内容</h5>
                            <pre>{log.completion || '-'}</pre>
                          </div>
                        </div>
                      </details>
                    </article>
                  ))}
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
                  title={btSelectedStrategyText}
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
