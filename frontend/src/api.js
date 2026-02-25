import axios from 'axios'

const http = axios.create({
  baseURL: '/api',
  timeout: 15000,
})

export const getStatus = () => http.get('/status')
export const getAccount = () => http.get('/account')
export const getSignals = (limit = 20) => http.get('/signals', { params: { limit } })
export const getStrategyScores = (limit = 20) =>
  http.get('/strategy-scores', { params: { limit } })
export const getSystemSettings = () => http.get('/system-settings')
export const saveSystemSettings = (settings) => http.post('/system-settings', { settings })

export const updateSettings = (payload) => http.post('/settings', payload)
export const runNow = () => http.post('/run')
export const startScheduler = () => http.post('/scheduler/start')
export const stopScheduler = () => http.post('/scheduler/stop')
