import { useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { ActionButton } from '@/components/ui/action-button'
import { Lock, UserRound } from 'lucide-react'

function AuthSplitLayout({
  title,
  subtitle,
  outsideTitle = false,
  cardClassName = '',
  children,
}: {
  title: string
  subtitle: string
  outsideTitle?: boolean
  cardClassName?: string
  children: ReactNode
}) {
  return (
    <div className="login-shell login-shell-split">
      <section className="login-left-panel">
        <div className="login-left-overlay">
          <h1>21xG</h1>
          <p>AI 量化交易控制台<br />策略 · 交易 · 风控</p>
          <div className="flat-shapes">
            <span className="shape arc arc-1" />
            <span className="shape arc arc-2" />
            <span className="shape arc arc-3" />
          </div>
        </div>
      </section>
      <section className="login-right-panel">
        <div className="login-right-stack">
          {outsideTitle ? (
            <div className="login-outside-head">
              <span className="login-outside-kicker">安全入口</span>
              <h2 className="login-outside-title">{title}</h2>
              {String(subtitle || '').trim() ? <p className="login-outside-subtitle">{subtitle}</p> : null}
            </div>
          ) : null}
          <div className={['login-card', cardClassName].filter(Boolean).join(' ')}>
            <header className="login-head">
              {!outsideTitle ? <h2>{title}</h2> : null}
              {String(subtitle || '').trim() ? <p>{subtitle}</p> : null}
            </header>
            {children}
          </div>
        </div>
      </section>
    </div>
  )
}

export function LoginPage({
  loading,
  error,
  onSubmit,
}: {
  loading: boolean
  error: string
  onSubmit: (payload: { username: string; password: string }) => Promise<void> | void
}) {
  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('')
  const disabled = useMemo(
    () => loading || !String(username || '').trim() || !String(password || '').trim(),
    [loading, username, password],
  )

  return (
    <AuthSplitLayout
      title="21xG 登录"
      subtitle="请输入账号和密码进入交易看板"
      cardClassName="login-card-no-frame"
    >
      <label className="login-field">
        <span>账号</span>
        <div className="login-field-box">
          <UserRound size={16} />
          <input
            autoComplete="username"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            placeholder="请输入账号"
          />
        </div>
      </label>
      <label className="login-field">
        <span>密码</span>
        <div className="login-field-box">
          <Lock size={16} />
          <input
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="请输入密码"
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !disabled) {
                void onSubmit({ username, password })
              }
            }}
          />
        </div>
      </label>
      {error ? <p className="login-error">{error}</p> : null}
      <div className="login-actions">
        <ActionButton
          className={`btn-flat btn-flat-blue save-config-btn ${loading ? 'is-saving' : ''}`}
          disabled={disabled}
          loading={loading}
          onClick={() => onSubmit({ username, password })}
        >
          {loading ? '登录中...' : '登录'}
        </ActionButton>
      </div>
      <p className="login-tip">初始账号：admin，初始密码：admin。首次登录后会强制要求修改账号与密码。</p>
    </AuthSplitLayout>
  )
}

export function BootstrapAdminPage({
  loading,
  error,
  onSubmit,
}: {
  loading: boolean
  error: string
  onSubmit: (payload: { password: string }) => Promise<void> | void
}) {
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const localError = useMemo(() => {
    if (!confirmPassword) return ''
    return password === confirmPassword ? '' : '两次输入的密码不一致'
  }, [password, confirmPassword])
  const disabled = useMemo(() => {
    return loading ||
      !String(password || '').trim() ||
      !String(confirmPassword || '').trim() ||
      Boolean(localError)
  }, [loading, password, confirmPassword, localError])

  return (
    <AuthSplitLayout
      title="初始化管理员密码"
      subtitle="首次启动请先为 admin 账号设置密码"
      cardClassName="bootstrap-admin-card"
    >
      <p className="login-tip">管理员账号固定为 `admin`，密码需 8-16 位且包含英文、数字、符号。</p>
      <label className="login-field">
        <span>新密码</span>
        <input
          type="password"
          autoComplete="new-password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          placeholder="请输入新密码"
        />
      </label>
      <label className="login-field">
        <span>确认密码</span>
        <input
          type="password"
          autoComplete="new-password"
          value={confirmPassword}
          onChange={(e) => setConfirmPassword(e.target.value)}
          placeholder="请再次输入新密码"
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !disabled) {
              void onSubmit({ password })
            }
          }}
        />
      </label>
      {localError ? <p className="login-error">{localError}</p> : null}
      {error ? <p className="login-error">{error}</p> : null}
      <div className="login-actions">
        <ActionButton
          className={`btn-flat btn-flat-blue save-config-btn ${loading ? 'is-saving' : ''}`}
          disabled={disabled}
          loading={loading}
          onClick={() => onSubmit({ password })}
        >
          {loading ? '设置中...' : '确认并进入系统'}
        </ActionButton>
      </div>
    </AuthSplitLayout>
  )
}

export function ForceChangeCredentialsPage({
  username,
  loading,
  error,
  onSubmit,
}: {
  username: string
  loading: boolean
  error: string
  onSubmit: (payload: { currentPassword: string; newUsername: string; newPassword: string }) => Promise<void> | void
}) {
  const [currentPassword, setCurrentPassword] = useState('')
  const currentUsername = String(username || '').trim()
  const [newPassword, setNewPassword] = useState('')
  const [confirmNewPassword, setConfirmNewPassword] = useState('')
  const localError = useMemo(() => {
    if (!String(confirmNewPassword || '').trim()) return ''
    return newPassword === confirmNewPassword ? '' : '两次输入的新密码不一致'
  }, [newPassword, confirmNewPassword])
  const disabled = useMemo(() => {
    return loading ||
      !currentUsername ||
      !String(currentPassword || '').trim() ||
      !String(newPassword || '').trim() ||
      !String(confirmNewPassword || '').trim() ||
      Boolean(localError)
  }, [loading, currentUsername, currentPassword, newPassword, confirmNewPassword, localError])

  return (
    <AuthSplitLayout
      title="首次登录安全设置"
      subtitle="请先修改账号和密码后再进入系统"
    >
      <label className="login-field">
        <span>当前账号</span>
        <input
          autoComplete="username"
          value={currentUsername}
          readOnly
        />
      </label>
      <label className="login-field">
        <span>当前密码</span>
        <input
          type="password"
          autoComplete="current-password"
          value={currentPassword}
          onChange={(e) => setCurrentPassword(e.target.value)}
          placeholder="请输入当前密码"
        />
      </label>
      <label className="login-field">
        <span>新密码（8-16位，英文+数字+符号）</span>
        <input
          type="password"
          autoComplete="new-password"
          value={newPassword}
          onChange={(e) => setNewPassword(e.target.value)}
          placeholder="请输入新密码"
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !disabled) {
              void onSubmit({ currentPassword, newUsername: currentUsername, newPassword })
            }
          }}
        />
      </label>
      <label className="login-field">
        <span>重复输入新密码</span>
        <input
          type="password"
          autoComplete="new-password"
          value={confirmNewPassword}
          onChange={(e) => setConfirmNewPassword(e.target.value)}
          placeholder="请再次输入新密码"
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !disabled) {
              void onSubmit({ currentPassword, newUsername: currentUsername, newPassword })
            }
          }}
        />
      </label>
      {localError ? <p className="login-error">{localError}</p> : null}
      {error ? <p className="login-error">{error}</p> : null}
      <div className="login-actions">
        <ActionButton
          className={`btn-flat btn-flat-blue save-config-btn ${loading ? 'is-saving' : ''}`}
          disabled={disabled}
          loading={loading}
          onClick={() => onSubmit({ currentPassword, newUsername: currentUsername, newPassword })}
        >
          {loading ? '保存中...' : '确认并进入系统'}
        </ActionButton>
      </div>
    </AuthSplitLayout>
  )
}
