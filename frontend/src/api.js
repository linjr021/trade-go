import axios from 'axios'

const http = axios.create({
  baseURL: '/api',
  timeout: 20000,
})

export const getStatus = () => http.get('/status')
export const getAccount = () => http.get('/account')
export const getAssetOverview = () => http.get('/assets/overview')
export const getAssetTrend = (range = '30D') => http.get('/assets/trend', { params: { range } })
export const getAssetPnLCalendar = (month) => http.get('/assets/pnl-calendar', { params: { month } })
export const getAssetDistribution = () => http.get('/assets/distribution')
export const getSignals = (limit = 20) => http.get('/signals', { params: { limit } })
export const getTradeRecords = (limit = 40) => http.get('/trade-records', { params: { limit } })
export const getStrategyScores = (limit = 20) => http.get('/strategy-scores', { params: { limit } })
export const getStrategies = () => http.get('/strategies')
export const getStrategyTemplate = () => http.get('/strategies/template')
export const uploadStrategyFile = (file) => {
  const form = new FormData()
  form.append('file', file)
  return http.post('/strategies/upload', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
    timeout: 120000,
  })
}
export const generateStrategyPreference = (payload) =>
  http.post('/strategy-preference/generate', payload, { timeout: 120000 })
export const runBacktestApi = (payload) => http.post('/backtest', payload, { timeout: 120000 })
export const getBacktestHistory = (limit = 80) => http.get('/backtest-history', { params: { limit } })
export const getBacktestHistoryDetail = (id) => http.get('/backtest-history/detail', { params: { id } })
export const deleteBacktestHistory = (id) => http.post('/backtest-history/delete', { id })
export const getSystemSettings = () => http.get('/system-settings')
export const saveSystemSettings = (settings) => http.post('/system-settings', { settings })
export const getSystemRuntimeStatus = () => http.get('/system/runtime')
export const restartSystemRuntime = () => http.post('/system/restart', {}, { timeout: 90000 })
export const getIntegrations = () => http.get('/integrations')
export const addLLMIntegration = (payload) => http.post('/integrations/llm', payload, { timeout: 30000 })
export const testLLMIntegration = (id) => http.post('/integrations/llm/test', { id }, { timeout: 30000 })
export const updateLLMIntegration = (payload) => http.post('/integrations/llm/update', payload, { timeout: 30000 })
export const deleteLLMIntegration = (id) => http.post('/integrations/llm/delete', { id }, { timeout: 30000 })
export const addExchangeIntegration = (payload) =>
  http.post('/integrations/exchange', payload, { timeout: 30000 })
export const activateExchangeIntegration = (id) =>
  http.post('/integrations/exchange/activate', { id }, { timeout: 30000 })
export const deleteExchangeIntegration = (id) =>
  http.post('/integrations/exchange/delete', { id }, { timeout: 30000 })
export const getPromptSettings = () => http.get('/prompt-settings')
export const savePromptSettings = (prompts) => http.post('/prompt-settings', { prompts })
export const resetPromptSettings = () => http.post('/prompt-settings', { reset_default: true })

export const updateSettings = (payload) => http.post('/settings', payload)
export const runNow = () => http.post('/run')
export const startScheduler = () => http.post('/scheduler/start')
export const stopScheduler = () => http.post('/scheduler/stop')
