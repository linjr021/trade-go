import { defineConfig, devices } from '@playwright/test'

const frontendHost = process.env.PW_FRONTEND_HOST ?? '127.0.0.1'
const frontendPort = Number(process.env.PW_FRONTEND_PORT ?? 4173)
const backendPort = Number(process.env.PW_BACKEND_PORT ?? 18080)
const frontendBaseURL = `http://${frontendHost}:${frontendPort}`
const backendBaseURL = `http://127.0.0.1:${backendPort}`

export default defineConfig({
  testDir: './e2e',
  timeout: 120_000,
  expect: {
    timeout: 15_000,
  },
  fullyParallel: false,
  retries: 0,
  reporter: [['list'], ['html', { open: 'never' }]],
  use: {
    baseURL: frontendBaseURL,
    trace: 'on-first-retry',
    headless: true,
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: [
    {
      command: `cd .. && rm -f /tmp/trade-go-e2e.db /tmp/trade-go-e2e.db-shm /tmp/trade-go-e2e.db-wal && MODE=web HTTP_ADDR=:${backendPort} TRADE_DB_PATH=/tmp/trade-go-e2e.db TEST_MODE=true ENABLE_WS_MARKET=false ADMIN_INITIAL_PASSWORD=admin go run .`,
      url: `${backendBaseURL}/api/auth/bootstrap-status`,
      timeout: 180_000,
      reuseExistingServer: false,
      stdout: 'ignore',
      stderr: 'pipe',
    },
    {
      command: `VITE_API_PROXY_TARGET=${backendBaseURL} npm run dev -- --host ${frontendHost} --port ${frontendPort}`,
      url: frontendBaseURL,
      timeout: 180_000,
      reuseExistingServer: false,
      stdout: 'ignore',
      stderr: 'pipe',
    },
  ],
})
