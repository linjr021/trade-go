import { expect, test, type Page } from '@playwright/test'

const DEFAULT_ADMIN_PASSWORD = 'admin'
const RESET_ADMIN_PASSWORD = 'Admin@123456'

async function submitLogin(page: Page, password: string) {
  const usernameInput = page.getByPlaceholder('请输入账号')
  if (await usernameInput.count()) {
    await usernameInput.fill('admin')
  }
  const passwordInput = page.getByPlaceholder('请输入密码')
  await expect(passwordInput).toBeVisible()
  await passwordInput.fill(password)

  const loginBtn = page.getByRole('button', { name: '登录' })
  await expect(loginBtn).toBeEnabled()
  const loginResp = page.waitForResponse((resp) => {
    return resp.request().method() === 'POST' && resp.url().includes('/api/auth/login')
  })
  await loginBtn.click()
  return loginResp
}

async function loginToDashboard(page: Page) {
  await page.goto('/')
  await expect(page.getByText('21xG 登录')).toBeVisible()

  let loginResponse = await submitLogin(page, DEFAULT_ADMIN_PASSWORD)
  if (loginResponse.status() === 401) {
    loginResponse = await submitLogin(page, RESET_ADMIN_PASSWORD)
  }
  expect(loginResponse.status()).toBe(200)

  const forceChangeTitle = page.getByText('首次登录安全设置')
  if (await forceChangeTitle.isVisible({ timeout: 4000 }).catch(() => false)) {
    await page.getByPlaceholder('请输入当前密码').fill(DEFAULT_ADMIN_PASSWORD)
    await page.getByPlaceholder('请输入新密码').fill(RESET_ADMIN_PASSWORD)
    await page.getByPlaceholder('请再次输入新密码').fill(RESET_ADMIN_PASSWORD)

    const changeResp = page.waitForResponse((resp) => {
      return resp.request().method() === 'POST' && resp.url().includes('/api/auth/change-credentials')
    })
    await page.getByRole('button', { name: '确认并进入系统' }).click()
    expect((await changeResp).status()).toBe(200)
  }

  await expect(page.locator('.sidebar')).toContainText('资产详情')
  await expect(page.locator('.brand')).toContainText('21xG')
}

test.describe('Dashboard smoke', () => {
  test('login flow and main frame render', async ({ page }) => {
    await loginToDashboard(page)
    await expect(page.locator('.content-head h1')).toHaveText('资产详情')
    await expect(page.locator('.sidebar')).toContainText('AI 工作流')
    await expect(page.locator('.sidebar')).toContainText('策略生成')
  })

  test('navigation and ai-workflow single source check', async ({ page }) => {
    await loginToDashboard(page)

    await page.locator('.sidebar').getByRole('button', { name: 'AI 工作流' }).click()
    await expect(page.locator('.content-head h1')).toHaveText('AI 工作流')
    await expect(page.getByText('流程图')).toBeVisible()
    await expect(page.getByText('规格构建').first()).toBeVisible()

    const token = await page.evaluate(() => window.localStorage.getItem('auth_token') || '')
    expect(token.length).toBeGreaterThan(10)

    const resp = await page.request.get('/api/skill-workflow', {
      headers: { Authorization: `Bearer ${token}` },
    })
    expect(resp.ok()).toBeTruthy()
    const data = await resp.json()

    expect(String(data?.ai_settings_path || '')).toContain('skills/trading-strategy-pipeline/references/ai-settings.json')
    expect(Array.isArray(data?.habit_profiles)).toBeTruthy()
    expect(Array.isArray(data?.workflow?.steps)).toBeTruthy()
    expect((data?.workflow?.steps || []).length).toBeGreaterThan(0)

    await page.locator('.sidebar').getByRole('button', { name: '策略生成' }).click()
    await expect(page.locator('.content-head h1')).toHaveText('策略生成')
    await expect(page.getByText('策略生成参数')).toBeVisible()

    const habitsFromAPI = (data?.habit_profiles || [])
      .map((x: any) => String(x?.habit || '').trim())
      .filter(Boolean)
    const habitOptions = await page
      .getByLabel('交易习惯时长（核心输入）')
      .locator('option')
      .allTextContents()

    for (const h of habitsFromAPI) {
      expect(habitOptions).toContain(h)
    }
  })

  test('system page key modules render', async ({ page }) => {
    await loginToDashboard(page)

    await page.locator('.sidebar').getByRole('button', { name: '系统设置' }).click()
    await expect(page.locator('.content-head h1')).toHaveText('系统设置')

    await page.getByRole('tab', { name: '系统状态' }).click()
    await expect(page.getByText('服务器各组件状态')).toBeVisible()
    await expect(page.getByRole('button', { name: '刷新状态' })).toBeVisible()

    await page.getByRole('tab', { name: '运行配置' }).click()
    await expect(page.getByRole('button', { name: '保存系统设置' })).toBeVisible()

    await page.getByRole('tab', { name: '智能体参数' }).click()
    await expect(page.getByText('智能体参数列表')).toBeVisible()

    await page.getByRole('tab', { name: '交易所参数' }).click()
    await expect(page.getByText('账号绑定状态')).toBeVisible()
  })
})
