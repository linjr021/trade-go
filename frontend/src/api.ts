import axios, { AxiosHeaders } from 'axios'

const AUTH_TOKEN_KEY = 'auth_token'

export const getAuthToken = () => localStorage.getItem(AUTH_TOKEN_KEY) || ''
export const setAuthToken = (token: string) => {
  const v = String(token || '').trim()
  if (!v) {
    localStorage.removeItem(AUTH_TOKEN_KEY)
    return
  }
  localStorage.setItem(AUTH_TOKEN_KEY, v)
}
export const clearAuthToken = () => localStorage.removeItem(AUTH_TOKEN_KEY)

const http = axios.create({
  baseURL: '/api',
  timeout: 20000,
})

http.interceptors.request.use((cfg) => {
  const token = getAuthToken()
  if (token) {
    const headers = AxiosHeaders.from(cfg.headers || {})
    headers.set('Authorization', `Bearer ${token}`)
    cfg.headers = headers
  }
  return cfg
})

http.interceptors.response.use(
  (resp) => resp,
  (err) => {
    const reqUrl = String(err?.config?.url || '')
    if (err?.response?.status === 401 && !reqUrl.includes('/auth/login')) {
      clearAuthToken()
      window.dispatchEvent(new Event('auth:unauthorized'))
    }
    return Promise.reject(err)
  },
)

export const login = (payload: { username: string; password: string }) =>
  http.post('/auth/login', payload, { timeout: 30000 })
export const getAuthBootstrapStatus = () => http.get('/auth/bootstrap-status')
export const bootstrapAdminPassword = (payload: { password: string }) =>
  http.post('/auth/bootstrap-admin', payload, { timeout: 30000 })
export const logout = () => http.post('/auth/logout')
export const getAuthMe = () => http.get('/auth/me')
export const changeAuthCredentials = (payload: {
  current_password: string
  new_username: string
  new_password: string
}) => http.post('/auth/change-credentials', payload, { timeout: 30000 })
export const getAuthRoles = () => http.get('/auth/roles')
export const createAuthRole = (payload: Record<string, any>) => http.post('/auth/roles', payload)
export const updateAuthRole = (payload: Record<string, any>) => http.post('/auth/roles/update', payload)
export const deleteAuthRole = (payload: { role_id: number }) => http.post('/auth/roles/delete', payload)
export const getAuthUsers = () => http.get('/auth/users')
export const createAuthUser = (payload: Record<string, any>) => http.post('/auth/users', payload)
export const updateAuthUserRole = (payload: Record<string, any>) => http.post('/auth/users/role', payload)
export const updateAuthUserPassword = (payload: Record<string, any>) => http.post('/auth/users/password', payload)
export const deleteAuthUser = (payload: { user_id: number }) => http.post('/auth/users/delete', payload)
export const getAuthAuditLogs = (params: Record<string, any> = {}) => http.get('/auth/audit-logs', { params })

export const getStatus = () => http.get('/status')
export const getAccount = () => http.get('/account')
export const getAssetOverview = () => http.get('/assets/overview')
export const getAssetTrend = (range: string = '30D') => http.get('/assets/trend', { params: { range } })
export const getAssetPnLCalendar = (month: string) => http.get('/assets/pnl-calendar', { params: { month } })
export const getAssetDistribution = () => http.get('/assets/distribution')
export const getMarketSnapshot = (params: Record<string, any> = {}) => http.get('/market/snapshot', { params })
export const getTradeRecords = (limit: number = 40) => http.get('/trade-records', { params: { limit } })
export const getStrategyScores = (limit: number = 20) => http.get('/strategy-scores', { params: { limit } })
export const getStrategies = () => http.get('/strategies')
export const getGeneratedStrategies = () => http.get('/generated-strategies')
export const syncGeneratedStrategies = (strategies: any[]) =>
  http.post('/generated-strategies', { strategies }, { timeout: 30000 })
export const generateStrategyPreference = (payload: Record<string, any>) =>
  http.post('/strategy-preference/generate', payload, { timeout: 120000 })
export const getSkillWorkflow = () => http.get('/skill-workflow')
export const saveSkillWorkflow = (workflow: Record<string, any>) => http.post('/skill-workflow', { workflow })
export const resetSkillWorkflow = () => http.post('/skill-workflow', { reset_default: true })
export const getLLMUsageLogs = (params: Record<string, any> = {}) => http.get('/llm-usage/logs', { params })
export const runAutoStrategyRegenNow = (payload: Record<string, any> = {}) =>
  http.post('/auto-strategy/regen-now', payload, { timeout: 90000 })
export const resetRiskBaseline = (payload: Record<string, any> = {}) =>
  http.post('/risk/reset', payload, { timeout: 30000 })
export const runBacktestApi = (payload: Record<string, any>) => http.post('/backtest', payload, { timeout: 120000 })
export const getBacktestHistory = (limit: number = 80) => http.get('/backtest-history', { params: { limit } })
export const getBacktestHistoryDetail = (id: number) => http.get('/backtest-history/detail', { params: { id } })
export const deleteBacktestHistory = (id: number) => http.post('/backtest-history/delete', { id })
export const getSystemSettings = () => http.get('/system-settings')
export const saveSystemSettings = (settings: Record<string, any>) => http.post('/system-settings', { settings })
export const getSystemRuntimeStatus = () => http.get('/system/runtime')
export const restartSystemRuntime = () => http.post('/system/restart', {}, { timeout: 90000 })
export const getIntegrations = () => http.get('/integrations')
export const addLLMIntegration = (payload: Record<string, any>) => http.post('/integrations/llm', payload, { timeout: 30000 })
export const testLLMIntegration = (id: string) => http.post('/integrations/llm/test', { id }, { timeout: 30000 })
export const probeLLMModels = (payload: Record<string, any>) => http.post('/integrations/llm/models', payload, { timeout: 45000 })
export const updateLLMIntegration = (payload: Record<string, any>) => http.post('/integrations/llm/update', payload, { timeout: 30000 })
export const deleteLLMIntegration = (id: string) => http.post('/integrations/llm/delete', { id }, { timeout: 30000 })
export const addExchangeIntegration = (payload: Record<string, any>) =>
  http.post('/integrations/exchange', payload, { timeout: 30000 })
export const activateExchangeIntegration = (id: string) =>
  http.post('/integrations/exchange/activate', { id }, { timeout: 30000 })
export const deleteExchangeIntegration = (id: string) =>
  http.post('/integrations/exchange/delete', { id }, { timeout: 30000 })

export const updateSettings = (payload: Record<string, any>) => http.post('/settings', payload)
export const runNow = () => http.post('/run')
export const startScheduler = () => http.post('/scheduler/start')
export const stopScheduler = () => http.post('/scheduler/stop')
export const runPaperSimulateStep = (payload: Record<string, any>) =>
  http.post('/paper/simulate-step', payload, { timeout: 90000 })
export const getPaperState = (params: Record<string, any> = {}) => http.get('/paper/state', { params })
export const updatePaperConfig = (payload: Record<string, any>) => http.post('/paper/config', payload)
export const startPaperSimulation = (payload: Record<string, any> = {}) =>
  http.post('/paper/start', payload, { timeout: 90000 })
export const stopPaperSimulation = () => http.post('/paper/stop')
export const resetPaperPnL = (payload: Record<string, any> = {}) => http.post('/paper/reset-pnl', payload)
export const resetPaperRiskBaseline = (payload: Record<string, any> = {}) =>
  http.post('/paper/risk/reset', payload, { timeout: 30000 })
