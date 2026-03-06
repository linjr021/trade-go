import { ActionButton } from '@/components/ui/action-button'
import { Space, Tabs } from '@/components/ui/dashboard-primitives'

export function SystemPageSection(p) {
  const {
    systemSubTab,
    setSystemSubTab,
    loadingSystemRuntime,
    loadSystemRuntime,
    restartingBackend,
    restartBackend,
    runtimeComponents,
    systemRuntime,
    fmtNum,
    fmtTime,
    envFieldGroups,
    systemSettings,
    setSystemSettings,
    systemSaveHint,
    savingSystemSettings,
    saveSystemEnv,
    setEditingLLMId,
    resetLLMModalDraft,
    setShowLLMModal,
    llmProductCatalog,
    llmConfigs,
    activeLLMId,
    activatingLLMId,
    bindLLMAccount,
    testingLLMId,
    llmStatusMap,
    testLLMConfigReachability,
    openEditLLMModal,
    deletingLLMId,
    removeLLMConfig,
    setShowExchangeModal,
    exchangeBound,
    activeExchangeId,
    exchangeConfigs,
    activatingExchangeId,
    bindExchangeAccount,
    deletingExchangeId,
    removeExchangeAccount,
  } = p
  const tokenUsageByChannel = (systemRuntime?.integration?.agent?.token_usage?.by_channel || {}) as Record<string, any>

  return (
    <section className="stack">
      <section className="card">
        <h3>系统设置</h3>
        <Tabs
          className="dashboard-tabs"
          activeKey={systemSubTab}
          onChange={setSystemSubTab}
          items={[
            { key: 'status', label: '系统状态' },
            { key: 'env', label: '运行配置' },
            { key: 'llm', label: '智能体参数' },
            { key: 'exchange', label: '交易所参数' },
          ]}
        />

        {systemSubTab === 'status' && (
          <div className="builder-pane system-status-pane">
            <section className="sub-window">
              <div className="card-head">
                <h4>系统运行状态</h4>
                <span>{loadingSystemRuntime ? '更新中...' : '已更新'}</span>
              </div>
              <div className="actions-row end">
                <Space>
                  <ActionButton className="btn-flat btn-flat-sky" onClick={() => loadSystemRuntime(false)} loading={loadingSystemRuntime}>
                    {loadingSystemRuntime ? '刷新中...' : '刷新状态'}
                  </ActionButton>
                  <ActionButton
                    className={`btn-flat btn-flat-rose save-config-btn ${restartingBackend ? 'is-saving' : ''}`}
                    onClick={restartBackend}
                    loading={restartingBackend}
                  >
                    {restartingBackend ? '重启中...' : '重启后台'}
                  </ActionButton>
                </Space>
              </div>
            </section>

            <section className="sub-window">
              <h4>服务器各组件状态</h4>
              <div className="status-grid">
                {runtimeComponents.map((c, idx) => (
                  <article key={`comp-${idx}`} className={`status-chip status-${c.status || 'unknown'}`}>
                    <div className="status-chip-title">{c.name || '-'}</div>
                    <div className="status-chip-text">{c.message || '-'}</div>
                  </article>
                ))}
                {!runtimeComponents.length ? (
                  <p className="muted">暂无组件状态数据</p>
                ) : null}
              </div>
            </section>

            <section className="sub-window">
              <h4>后台服务器资源状态</h4>
              <div className="summary-grid">
                <article className="metric-card"><h4>主机名</h4><p>{systemRuntime?.server?.hostname || '-'}</p></article>
                <article className="metric-card"><h4>运行时长</h4><p>{fmtNum((Number(systemRuntime?.server?.uptime_sec || 0) / 3600), 2)} h</p></article>
                <article className="metric-card"><h4>重启次数</h4><p>{systemRuntime?.server?.restart_count ?? 0}</p></article>
                <article className="metric-card"><h4>Goroutines</h4><p>{systemRuntime?.resources?.goroutines ?? 0}</p></article>
                <article className="metric-card"><h4>Heap(MB)</h4><p>{fmtNum(systemRuntime?.resources?.heap_alloc_mb, 2)}</p></article>
                <article className="metric-card"><h4>Sys(MB)</h4><p>{fmtNum(systemRuntime?.resources?.sys_memory_mb, 2)}</p></article>
              </div>
            </section>

            <section className="sub-window">
              <h4>智能体 / 交易所对接状态</h4>
              <div className="summary-grid">
                <article className="metric-card">
                  <h4>交易所状态</h4>
                  <p>{systemRuntime?.integration?.exchange?.ready ? '已连接' : '未连接'}</p>
                </article>
                <article className="metric-card">
                  <h4>当前交易所ID</h4>
                  <p>{systemRuntime?.integration?.exchange?.active_exchange_id || '-'}</p>
                </article>
                <article className="metric-card">
                  <h4>智能体状态</h4>
                  <p>
                    {systemRuntime?.integration?.agent?.status === 'connected'
                      ? '可达'
                      : systemRuntime?.integration?.agent?.status === 'warning'
                        ? '不可达'
                        : systemRuntime?.integration?.agent?.configured
                          ? '已配置（未验证）'
                          : '未配置'}
                  </p>
                </article>
                <article className="metric-card">
                  <h4>智能体检测信息</h4>
                  <p>{systemRuntime?.integration?.agent?.message || '-'}</p>
                </article>
                <article className="metric-card">
                  <h4>模型</h4>
                  <p>{systemRuntime?.integration?.agent?.model || '-'}</p>
                </article>
                <article className="metric-card">
                  <h4>Token 总消耗</h4>
                  <p>{systemRuntime?.integration?.agent?.token_usage?.total_tokens ?? 0}</p>
                </article>
                <article className="metric-card">
                  <h4>Token 请求数</h4>
                  <p>{systemRuntime?.integration?.agent?.token_usage?.requests ?? 0}</p>
                </article>
              </div>
              <div className="table-wrap">
                <table className="centered-list-table">
                  <thead>
                    <tr>
                      <th>场景</th>
                      <th>请求数</th>
                      <th>输入Token</th>
                      <th>输出Token</th>
                      <th>总Token</th>
                      <th>最近使用</th>
                    </tr>
                  </thead>
                  <tbody>
                    {Object.entries(tokenUsageByChannel).map(([k, v]) => (
                      <tr key={`token-${k}`}>
                        <td>{k}</td>
                        <td>{v?.requests ?? 0}</td>
                        <td>{v?.prompt_tokens ?? 0}</td>
                        <td>{v?.completion_tokens ?? 0}</td>
                        <td>{v?.total_tokens ?? 0}</td>
                        <td>{fmtTime(v?.last_used_at)}</td>
                      </tr>
                    ))}
                    {!Object.keys(tokenUsageByChannel).length ? (
                      <tr><td colSpan={6} className="muted">暂无 token 使用数据</td></tr>
                    ) : null}
                  </tbody>
                </table>
              </div>
            </section>
          </div>
        )}

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
              <ActionButton
                className={`btn-flat btn-flat-blue save-config-btn ${savingSystemSettings ? 'is-saving' : ''}`}
                onClick={saveSystemEnv}
                loading={savingSystemSettings}
              >
                {savingSystemSettings ? '保存中...' : '保存系统设置'}
              </ActionButton>
            </div>
          </div>
        )}

        {systemSubTab === 'llm' && (
          <div className="builder-pane">
            <section className="sub-window">
              <div className="card-head">
                <h4>智能体参数列表</h4>
                <ActionButton
                  className="btn-flat btn-flat-blue"
                  onClick={() => {
                    setEditingLLMId('')
                    resetLLMModalDraft()
                    setShowLLMModal(true)
                  }}
                >
                  添加智能体参数
                </ActionButton>
              </div>
              <div className="table-wrap">
                <table className="centered-list-table">
                  <thead>
                    <tr>
                      <th>ID</th>
                      <th>名称</th>
                      <th>产品</th>
                      <th>模型</th>
                      <th>状态</th>
                      <th>操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {llmConfigs.map((x) => {
                      const id = String(x.id || '')
                      const isActive = id === String(activeLLMId || '')
                      const isActivating = activatingLLMId === id
                      const isTesting = testingLLMId === id
                      const rowStatus = llmStatusMap[id] || { state: 'unknown', message: '未检测' }
                      const statusState = isTesting ? 'testing' : String(rowStatus.state || 'unknown')
                      const statusText = isTesting
                        ? '检测中'
                        : statusState === 'reachable'
                          ? '可达'
                          : statusState === 'unreachable'
                            ? '不可达'
                            : '未检测'
                      const statusTitle = String(rowStatus.message || statusText)
                      return (
                        <tr key={x.id} className={isActive ? 'exchange-row-active' : ''}>
                          <td>{x.id}</td>
                          <td>{x.name}</td>
                          <td>{llmProductCatalog.find((item) => item.product === x.product)?.label || llmProductCatalog.find((item) => item.product === x.product)?.name || x.product || '-'}</td>
                          <td>{x.model}</td>
                          <td>
                            <span className={`llm-status-badge is-${statusState}`} title={statusTitle}>
                              {statusText}
                            </span>
                          </td>
                          <td>
                            <div className="inline-actions">
                              <ActionButton
                                className="btn-flat btn-flat-emerald"
                                loading={isActivating}
                                disabled={isActive || isActivating}
                                onClick={() => bindLLMAccount(x.id)}
                              >
                                {isActivating ? '激活中...' : (isActive ? '已激活' : '激活')}
                              </ActionButton>
                              <ActionButton
                                className="btn-flat btn-flat-cyan"
                                loading={isTesting}
                                onClick={() => testLLMConfigReachability(x.id)}
                              >
                                {isTesting ? '测试中...' : '测试'}
                              </ActionButton>
                              <ActionButton className="btn-flat btn-flat-slate" onClick={() => openEditLLMModal(x)}>编辑</ActionButton>
                              <ActionButton
                                className="btn-flat btn-flat-rose"
                                loading={deletingLLMId === String(x.id)}
                                onClick={() => removeLLMConfig(x.id)}
                              >
                                {deletingLLMId === String(x.id) ? '删除中...' : '删除'}
                              </ActionButton>
                            </div>
                          </td>
                        </tr>
                      )
                    })}
                    {!llmConfigs.length ? (
                      <tr><td colSpan={6} className="muted">暂无智能体参数</td></tr>
                    ) : null}
                  </tbody>
                </table>
              </div>
            </section>
          </div>
        )}

        {systemSubTab === 'exchange' && (
          <div className="builder-pane">
            <div className="actions-row end">
              <ActionButton className="btn-flat btn-flat-blue" onClick={() => setShowExchangeModal(true)}>添加交易所参数</ActionButton>
            </div>
            <section className={`sub-window exchange-bind-status ${exchangeBound ? 'is-bound' : 'is-unbound'}`}>
              <h4>账号绑定状态</h4>
              {exchangeBound ? (
                <p>已绑定交易账号，当前 ID：{activeExchangeId || '-'}</p>
              ) : (
                <p>未绑定交易账号，请在下方列表中选择一个账号进行绑定。</p>
              )}
            </section>
            <div className="table-wrap">
              <table className="centered-list-table">
                <thead>
                  <tr>
                    <th>ID</th>
                    <th>交易所</th>
                    <th>API Key</th>
                    <th>状态</th>
                    <th>操作</th>
                  </tr>
                </thead>
                <tbody>
                  {exchangeConfigs.map((x) => (
                    <tr key={x.id} className={String(x.id) === activeExchangeId ? 'exchange-row-active' : ''}>
                      <td>{x.id}</td>
                      <td>{x.exchange}</td>
                      <td>{x.api_key ? `${String(x.api_key).slice(0, 6)}***` : '-'}</td>
                      <td>{String(x.id) === activeExchangeId ? '已绑定' : '未绑定'}</td>
                      <td>
                        <div className="inline-actions">
                          <ActionButton
                            className="btn-flat btn-flat-emerald"
                            loading={activatingExchangeId === String(x.id)}
                            disabled={String(x.id) === activeExchangeId || activatingExchangeId === String(x.id)}
                            onClick={() => bindExchangeAccount(x.id)}
                          >
                            {activatingExchangeId === String(x.id) ? '绑定中...' : '绑定此账号'}
                          </ActionButton>
                          <ActionButton
                            className="btn-flat btn-flat-rose"
                            loading={deletingExchangeId === String(x.id)}
                            onClick={() => removeExchangeAccount(x.id)}
                          >
                            {deletingExchangeId === String(x.id) ? '删除中...' : '删除'}
                          </ActionButton>
                        </div>
                      </td>
                    </tr>
                  ))}
                  {!exchangeConfigs.length ? (
                    <tr><td colSpan={5} className="muted">暂无交易所参数</td></tr>
                  ) : null}
                </tbody>
              </table>
            </div>
          </div>
        )}

      </section>
    </section>
  )
}
