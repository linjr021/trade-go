import { useMemo, useRef } from 'react'
import { ActionButton } from '@/components/ui/action-button'
import { Tabs } from '@/components/ui/dashboard-primitives'

const TIMEFRAME_OPTIONS = [
  '1m',
  '3m',
  '5m',
  '10m',
  '15m',
  '30m',
  '1h',
  '2h',
  '4h',
  '6h',
  '8h',
  '12h',
  '1d',
  '3d',
  '1w',
  '1mo',
]

function JsonCodeEditor({
  value,
  onChange,
}: {
  value: string
  onChange: (next: string) => void
}) {
  const gutterRef = useRef<HTMLDivElement | null>(null)
  const lineCount = useMemo(() => Math.max(1, String(value || '').split('\n').length), [value])
  const lineNumbers = useMemo(() => Array.from({ length: lineCount }, (_, idx) => idx + 1), [lineCount])

  return (
    <div className="json-code-editor">
      <div className="json-code-head">
        <span className="json-code-lang">json</span>
      </div>
      <div className="json-code-body">
        <div className="json-code-gutter" ref={gutterRef} aria-hidden>
          {lineNumbers.map((n) => <span key={`ln-${n}`}>{n}</span>)}
        </div>
        <textarea
          className="json-editor json-code-textarea"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          onScroll={(e) => {
            if (gutterRef.current) gutterRef.current.scrollTop = e.currentTarget.scrollTop
          }}
          spellCheck={false}
        />
      </div>
    </div>
  )
}

export function AdvancedSettingsSection(p) {
  const {
    advancedTab,
    setAdvancedTab,
    systemSettings,
    setSystemSettings,
    savingAdvancedEnvSettings,
    saveAdvancedEnvSettings,
    advancedEnvSaveHint,
    habitProfilesJSON,
    setHabitProfilesJSON,
    savingHabitProfiles,
    saveHabitProfilesConfig,
    resetHabitProfilesToDefault,
    strategySchemaJSON,
    setStrategySchemaJSON,
    savingStrategySchema,
    saveStrategySchemaConfig,
    resetStrategySchemaToDefault,
    loadAdvancedContent,
  } = p

  return (
    <section className="stack">
      <section className="card">
        <h3>高级参数</h3>
        <Tabs
          className="dashboard-tabs"
          activeKey={advancedTab}
          onChange={setAdvancedTab}
          items={[
            { key: 'env', label: '高级环境变量' },
            { key: 'habit', label: '交易习惯画像' },
            { key: 'schema', label: '策略包 Schema' },
          ]}
        />
        <div className="actions-row end">
          <ActionButton className="btn-flat btn-flat-sky" onClick={loadAdvancedContent}>
            刷新当前内容
          </ActionButton>
        </div>

        {advancedTab === 'env' && (
          <div className="builder-pane">
            <section className="sub-window">
              <h4>执行引擎参数</h4>
              <div className="form-grid workflow-constraints-grid">
                <label>
                  <span>数据库路径</span>
                  <input
                    type="text"
                    value={String(systemSettings?.TRADE_DB_PATH || '')}
                    onChange={(e) => setSystemSettings((old) => ({ ...old, TRADE_DB_PATH: e.target.value }))}
                  />
                </label>
                <label>
                  <span>K线周期</span>
                  <select
                    value={String(systemSettings?.TIMEFRAME || '15m')}
                    onChange={(e) => setSystemSettings((old) => ({ ...old, TIMEFRAME: e.target.value }))}
                  >
                    {TIMEFRAME_OPTIONS.map((tf) => (
                      <option key={`adv-tf-${tf}`} value={tf}>{tf}</option>
                    ))}
                  </select>
                </label>
                <label>
                  <span>采样K线数量(30-5000)</span>
                  <input
                    type="number"
                    min="30"
                    max="5000"
                    step="1"
                    value={String(systemSettings?.DATA_POINTS || '96')}
                    onChange={(e) => setSystemSettings((old) => ({ ...old, DATA_POINTS: e.target.value }))}
                  />
                </label>
                <label>
                  <span>启用行情WS</span>
                  <select
                    value={String(systemSettings?.ENABLE_WS_MARKET || 'true').toLowerCase() === 'false' ? 'false' : 'true'}
                    onChange={(e) => setSystemSettings((old) => ({ ...old, ENABLE_WS_MARKET: e.target.value }))}
                  >
                    <option value="true">true</option>
                    <option value="false">false</option>
                  </select>
                </label>
                <label>
                  <span>最小轮询间隔秒(1-300)</span>
                  <input
                    type="number"
                    min="1"
                    max="300"
                    step="1"
                    value={String(systemSettings?.REALTIME_MIN_INTERVAL_SEC || '5')}
                    onChange={(e) => setSystemSettings((old) => ({ ...old, REALTIME_MIN_INTERVAL_SEC: e.target.value }))}
                  />
                </label>
                <label>
                  <span>启用策略LLM调用</span>
                  <select
                    value={String(systemSettings?.STRATEGY_LLM_ENABLED || 'true').toLowerCase() === 'false' ? 'false' : 'true'}
                    onChange={(e) => setSystemSettings((old) => ({ ...old, STRATEGY_LLM_ENABLED: e.target.value }))}
                  >
                    <option value="true">true</option>
                    <option value="false">false</option>
                  </select>
                </label>
                <label>
                  <span>策略LLM超时秒(1-300)</span>
                  <input
                    type="number"
                    min="1"
                    max="300"
                    step="1"
                    value={String(systemSettings?.STRATEGY_LLM_TIMEOUT_SEC || '60')}
                    onChange={(e) => setSystemSettings((old) => ({ ...old, STRATEGY_LLM_TIMEOUT_SEC: e.target.value }))}
                  />
                </label>
                <label>
                  <span>TEST_MODE</span>
                  <select
                    value={String(systemSettings?.TEST_MODE || 'false').toLowerCase() === 'true' ? 'true' : 'false'}
                    onChange={(e) => setSystemSettings((old) => ({ ...old, TEST_MODE: e.target.value }))}
                  >
                    <option value="false">false</option>
                    <option value="true">true</option>
                  </select>
                </label>
              </div>
              <div className="actions-row end">
                {advancedEnvSaveHint ? <span className="save-hint">{advancedEnvSaveHint}</span> : null}
                <ActionButton
                  className={`btn-flat btn-flat-blue save-config-btn ${savingAdvancedEnvSettings ? 'is-saving' : ''}`}
                  onClick={saveAdvancedEnvSettings}
                  loading={savingAdvancedEnvSettings}
                >
                  {savingAdvancedEnvSettings ? '保存中...' : '保存高级环境变量'}
                </ActionButton>
              </div>
            </section>
          </div>
        )}

        {advancedTab === 'habit' && (
          <div className="builder-pane">
            <section className="sub-window">
              <h4>交易习惯画像（JSON）</h4>
              <p className="muted">修改后将直接写入 AI 设置源并用于策略生成。</p>
              <JsonCodeEditor value={habitProfilesJSON} onChange={setHabitProfilesJSON} />
              <div className="actions-row end">
                <ActionButton className="btn-flat btn-flat-slate" onClick={resetHabitProfilesToDefault}>
                  恢复默认
                </ActionButton>
                <ActionButton
                  className={`btn-flat btn-flat-blue save-config-btn ${savingHabitProfiles ? 'is-saving' : ''}`}
                  onClick={saveHabitProfilesConfig}
                  loading={savingHabitProfiles}
                >
                  {savingHabitProfiles ? '保存中...' : '保存交易习惯画像'}
                </ActionButton>
              </div>
            </section>
          </div>
        )}

        {advancedTab === 'schema' && (
          <div className="builder-pane">
            <section className="sub-window">
              <h4>策略包 Schema（JSON）</h4>
              <p className="muted">用于规范策略包结构与字段约束。</p>
              <JsonCodeEditor value={strategySchemaJSON} onChange={setStrategySchemaJSON} />
              <div className="actions-row end">
                <ActionButton className="btn-flat btn-flat-slate" onClick={resetStrategySchemaToDefault}>
                  恢复默认
                </ActionButton>
                <ActionButton
                  className={`btn-flat btn-flat-blue save-config-btn ${savingStrategySchema ? 'is-saving' : ''}`}
                  onClick={saveStrategySchemaConfig}
                  loading={savingStrategySchema}
                >
                  {savingStrategySchema ? '保存中...' : '保存策略包 Schema'}
                </ActionButton>
              </div>
            </section>
          </div>
        )}
      </section>
    </section>
  )
}
