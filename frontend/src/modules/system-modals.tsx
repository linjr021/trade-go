// @ts-nocheck
import { ActionButton } from '@/components/ui/action-button'
import { Modal } from '@/components/ui/dashboard-primitives'

export function LLMConfigModal(p) {
  const {
    open,
    editingLLMId,
    setShowLLMModal,
    setEditingLLMId,
    resetLLMModalDraft,
    setNewLLM,
    newLLM,
    llmProductCatalog,
    selectedLLMPreset,
    llmModelOptions,
    probingLLMModels,
    llmProbeMessage,
    probeLLMModelOptions,
    addingLLM,
    handleAddLLM,
  } = p

  const closeModal = () => {
    setShowLLMModal(false)
    setEditingLLMId('')
    resetLLMModalDraft()
  }

  return (
    <>
      <Modal
        open={open}
        title={editingLLMId ? `编辑智能体参数 #${editingLLMId}` : '添加智能体参数'}
        onCancel={closeModal}
        footer={[
          <ActionButton
            key="cancel"
            className="btn-flat btn-flat-slate"
            onClick={closeModal}
          >
            取消
          </ActionButton>,
          <ActionButton
            key="submit"
            className={`btn-flat btn-flat-blue save-config-btn ${addingLLM ? 'is-saving' : ''}`}
            loading={addingLLM}
            onClick={handleAddLLM}
          >
            {addingLLM ? (editingLLMId ? '更新中...' : '校验中...') : (editingLLMId ? '确认更新' : '确认添加')}
          </ActionButton>,
        ]}
        destroyOnClose
      >
        <p className="muted">{editingLLMId ? '修改后将重新校验连通性。' : 'ID 将自动按 1,2,3... 递增分配。'}</p>
        <div className="form-grid modal-form">
          <label><span>名称</span><input value={newLLM.name} onChange={(e) => setNewLLM((v) => ({ ...v, name: e.target.value }))} /></label>
          <label>
            <span>智能体产品</span>
            <select
              value={String(newLLM.product || '')}
              onChange={(e) => {
                const product = String(e.target.value || '')
                const preset = (llmProductCatalog || []).find((x) => String(x?.product || '') === product)
                setNewLLM((v) => ({
                  ...v,
                  product: String(preset?.product || 'chatgpt'),
                  base_url: String(preset?.base_url || v?.base_url || ''),
                  model: '',
                }))
              }}
            >
              {(llmProductCatalog || []).map((x) => (
                <option key={`llm-product-option-${x.product}`} value={x.product}>
                  {x.label || x.name || x.product}
                </option>
              ))}
            </select>
          </label>
          <label>
            <span>Base URL（来自产品）</span>
            <input value={String(selectedLLMPreset?.base_url || '')} readOnly />
          </label>
          <label><span>API Key</span><input type="password" value={newLLM.api_key} onChange={(e) => setNewLLM((v) => ({ ...v, api_key: e.target.value, model: '' }))} /></label>
          <label>
            <span>模型（自动检测）</span>
            <select
              value={String(newLLM.model || '')}
              disabled={!llmModelOptions.length || probingLLMModels}
              onChange={(e) => setNewLLM((v) => ({ ...v, model: e.target.value }))}
            >
              {!llmModelOptions.length ? (
                <option value="">{probingLLMModels ? '模型检测中...' : '请先选择产品并填写 API Key'}</option>
              ) : null}
              {llmModelOptions.map((m) => (
                <option key={m} value={m}>{m}</option>
              ))}
            </select>
          </label>
        </div>
        <div className="actions-row end">
          <ActionButton
            className={`btn-flat btn-flat-sky save-config-btn ${probingLLMModels ? 'is-saving' : ''}`}
            loading={probingLLMModels}
            onClick={() =>
              probeLLMModelOptions()
            }
          >
            {probingLLMModels ? '检测中...' : '重新检测模型'}
          </ActionButton>
        </div>
      </Modal>
      <Modal
        open={Boolean(probingLLMModels)}
        title="智能体检测中"
        onCancel={() => {}}
        footer={null}
      >
        <div className="llm-probe-progress">
          <div className="llm-probe-spinner" />
          <p>{llmProbeMessage || '正在检测，请稍候...'}</p>
        </div>
      </Modal>
    </>
  )
}

export function ExchangeConfigModal(p) {
  const {
    open,
    setShowExchangeModal,
    addingExchange,
    handleAddExchange,
    newExchange,
    setNewExchange,
  } = p

  return (
    <Modal
      open={open}
      title="添加交易所参数"
      onCancel={() => setShowExchangeModal(false)}
      footer={[
        <ActionButton key="cancel" className="btn-flat btn-flat-slate" onClick={() => setShowExchangeModal(false)}>取消</ActionButton>,
        <ActionButton
          key="submit"
          className={`btn-flat btn-flat-emerald save-config-btn ${addingExchange ? 'is-saving' : ''}`}
          loading={addingExchange}
          onClick={handleAddExchange}
        >
          {addingExchange ? '校验中...' : '确认添加'}
        </ActionButton>,
      ]}
      destroyOnClose
    >
      <p className="muted">ID 将自动按 1,2,3... 递增分配。</p>
      <div className="form-grid modal-form">
        <label>
          <span>交易所</span>
          <select value={newExchange.exchange} onChange={(e) => setNewExchange((v) => ({ ...v, exchange: e.target.value }))}>
            <option value="binance">binance</option>
            <option value="okx">okx</option>
          </select>
        </label>
        <label><span>API Key</span><input value={newExchange.api_key} onChange={(e) => setNewExchange((v) => ({ ...v, api_key: e.target.value }))} /></label>
        <label><span>Secret</span><input type="password" value={newExchange.secret} onChange={(e) => setNewExchange((v) => ({ ...v, secret: e.target.value }))} /></label>
        <label><span>{newExchange.exchange === 'okx' ? 'Passphrase(必填)' : 'Passphrase(可选)'}</span><input value={newExchange.passphrase} onChange={(e) => setNewExchange((v) => ({ ...v, passphrase: e.target.value }))} /></label>
      </div>
    </Modal>
  )
}
