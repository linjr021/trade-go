import { useEffect, useMemo, useState } from 'react'
import {
  AntdApp,
  ConfigProvider,
} from '@/components/ui/dashboard-primitives'
import { OverviewCardsPanel } from '@/modules/overview-cards-panel'
import { TopToast } from '@/modules/top-toast'
import { LLMConfigModal, ExchangeConfigModal } from '@/modules/system-modals'
import { DashboardFrame } from '@/modules/dashboard-frame'
import { DashboardContent } from '@/modules/dashboard-content'
import { ForceChangeCredentialsPage, LoginPage } from '@/modules/login-page'
import { useDashboardController } from '@/modules/use-dashboard-controller'
import {
  changeAuthCredentials,
  clearAuthToken,
  getAuthMe,
  getAuthToken,
  login,
  logout,
  setAuthToken,
} from '@/api'

function extractApiError(err: any, fallback: string) {
  const data = err?.response?.data
  if (typeof data === 'string' && data.trim()) {
    const raw = data.trim()
    if (raw.includes('ECONNREFUSED') || raw.includes('connect') || raw.includes('proxy')) {
      return '后端服务不可达，请先启动后端（默认 :8080）'
    }
    return raw
  }
  if (data && typeof data === 'object' && typeof data.error === 'string' && data.error.trim()) {
    return data.error.trim()
  }
  if (err?.code === 'ERR_NETWORK') {
    return '网络不可达，请检查前后端服务状态'
  }
  if (Number(err?.response?.status || 0) === 500) {
    return '后端返回 500，请查看后端日志定位错误'
  }
  return String(err?.message || fallback)
}

function DashboardAuthed({
  currentUser,
  onLogout,
}: {
  currentUser: Record<string, any>
  onLogout: () => Promise<void> | void
}) {
  const c = useDashboardController()
  const userPermissions = currentUser?.permissions && typeof currentUser.permissions === 'object'
    ? currentUser.permissions
    : {}
  const visibleMenuItems = useMemo(() => {
    if (currentUser?.is_super) return c.sidebarMenuItems
    return c.sidebarMenuItems.filter((item: any) => {
      const key = String(item?.key || '').trim()
      if (!key) return false
      const permModule = String(item?.permModule || key).trim()
      const access = String((userPermissions as any)[permModule] || 'none').toLowerCase()
      return access === 'read' || access === 'edit'
    })
  }, [c.sidebarMenuItems, currentUser?.is_super, userPermissions])

  useEffect(() => {
    if (!visibleMenuItems.length) return
    if (!visibleMenuItems.some((item: any) => String(item?.key || '') === String(c.menu || ''))) {
      c.setMenu(String(visibleMenuItems[0]?.key || 'assets'))
    }
  }, [visibleMenuItems, c.menu, c.setMenu])

  const renderOverviewCards = (
    pair: string,
    _strategyName: string,
    extra: any = null,
    overrides: Record<string, any> = {},
  ) => (
    <OverviewCardsPanel
      pair={pair}
      marketEmotion={overrides.marketEmotion ?? c.marketEmotion}
      totalPnL={overrides.totalPnL ?? c.totalPnL}
      account={overrides.account ?? c.account}
      strategyDurationText={overrides.strategyDurationText ?? c.strategyDurationText}
      pnlRatio={overrides.pnlRatio ?? c.pnlRatio}
      extra={extra}
      resolvedTheme={c.resolvedTheme}
      activeExchangeType={overrides.activeExchangeType ?? c.activeExchangeType}
    />
  )

  return (
    <>
      <TopToast toast={c.toast} />
      <DashboardFrame
        productName={c.productName}
        menu={c.menu}
        setMenu={c.setMenu}
        sidebarMenuItems={visibleMenuItems}
        loading={c.loading}
        error={c.error}
        themeMode={c.themeMode}
        setThemeMode={c.setThemeMode}
        currentUser={currentUser}
        onLogout={onLogout}
        onOpenAuthAdmin={() => c.setMenu('auth_admin')}
      >
        <DashboardContent c={c} renderOverviewCards={renderOverviewCards} />
      </DashboardFrame>
      <LLMConfigModal
        open={c.showLLMModal}
        editingLLMId={c.editingLLMId}
        setShowLLMModal={c.setShowLLMModal}
        setEditingLLMId={c.setEditingLLMId}
        resetLLMModalDraft={c.resetLLMModalDraft}
        setNewLLM={c.setNewLLM}
        newLLM={c.newLLM}
        llmProductCatalog={c.llmProductCatalog}
        selectedLLMPreset={c.selectedLLMPreset}
        llmModelOptions={c.llmModelOptions}
        probingLLMModels={c.probingLLMModels}
        llmProbeMessage={c.llmProbeMessage}
        probeLLMModelOptions={c.probeLLMModelOptions}
        addingLLM={c.addingLLM}
        handleAddLLM={c.handleAddLLM}
      />
      <ExchangeConfigModal
        open={c.showExchangeModal}
        setShowExchangeModal={c.setShowExchangeModal}
        addingExchange={c.addingExchange}
        handleAddExchange={c.handleAddExchange}
        newExchange={c.newExchange}
        setNewExchange={c.setNewExchange}
      />
    </>
  )
}

export default function App() {
  const [checking, setChecking] = useState(true)
  const [authLoading, setAuthLoading] = useState(false)
  const [authError, setAuthError] = useState('')
  const [changingCreds, setChangingCreds] = useState(false)
  const [changeCredsError, setChangeCredsError] = useState('')
  const [currentUser, setCurrentUser] = useState<Record<string, any> | null>(null)

  useEffect(() => {
    let dead = false
    const checkLogin = async () => {
      const token = getAuthToken()
      if (token) {
        try {
          const res = await getAuthMe()
          if (dead) return
          setCurrentUser(res?.data?.user || null)
        } catch (_err) {
          clearAuthToken()
          if (!dead) setCurrentUser(null)
        }
      } else {
        setCurrentUser(null)
      }
      if (!dead) {
        setChecking(false)
      }
    }
    const onUnauthorized = () => {
      clearAuthToken()
      setCurrentUser(null)
      setAuthError('登录已过期，请重新登录')
    }
    window.addEventListener('auth:unauthorized', onUnauthorized as EventListener)
    void checkLogin()
    return () => {
      dead = true
      window.removeEventListener('auth:unauthorized', onUnauthorized as EventListener)
    }
  }, [])

  const handleLogin = async (payload: { username: string; password: string }) => {
    if (authLoading) return
    setAuthLoading(true)
    setAuthError('')
    try {
      const res = await login(payload)
      const token = String(res?.data?.token || '').trim()
      const user = res?.data?.user || null
      if (!token || !user) {
        throw new Error('登录响应缺少 token 或用户信息')
      }
      setAuthToken(token)
      setCurrentUser(user)
    } catch (err) {
      setAuthError(extractApiError(err, '登录失败'))
    } finally {
      setAuthLoading(false)
    }
  }

  const handleLogout = async () => {
    try {
      await logout()
    } catch (_err) {
      // ignore logout network error
    }
    clearAuthToken()
    setCurrentUser(null)
  }

  const handleChangeCredentials = async (payload: {
    currentPassword: string
    newUsername: string
    newPassword: string
  }) => {
    if (changingCreds) return
    setChangingCreds(true)
    setChangeCredsError('')
    try {
      const res = await changeAuthCredentials({
        current_password: payload.currentPassword,
        new_username: payload.newUsername,
        new_password: payload.newPassword,
      })
      const nextUser = res?.data?.user || null
      if (!nextUser) throw new Error('更新后未返回用户信息')
      setCurrentUser(nextUser)
    } catch (err) {
      setChangeCredsError(extractApiError(err, '修改失败'))
    } finally {
      setChangingCreds(false)
    }
  }

  return (
    <ConfigProvider>
      <AntdApp>
        {checking ? (
          <div className="login-shell"><p className="muted">正在检查登录状态...</p></div>
        ) : currentUser ? (
          currentUser?.must_change_credentials ? (
            <ForceChangeCredentialsPage
              username={String(currentUser?.username || '')}
              loading={changingCreds}
              error={changeCredsError}
              onSubmit={handleChangeCredentials}
            />
          ) : (
            <DashboardAuthed currentUser={currentUser} onLogout={handleLogout} />
          )
        ) : (
          <LoginPage loading={authLoading} error={authError} onSubmit={handleLogin} />
        )}
      </AntdApp>
    </ConfigProvider>
  )
}
