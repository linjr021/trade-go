import { Badge } from '@/components/ui/badge'
import {
  Layout,
  Menu,
  Select,
} from '@/components/ui/dashboard-primitives'
import { ActionButton } from '@/components/ui/action-button'
import { MENU_ITEMS } from '@/modules/constants'

export function DashboardFrame({
  productName,
  menu,
  setMenu,
  sidebarMenuItems,
  loading,
  error,
  themeMode,
  setThemeMode,
  currentUser,
  onLogout,
  onOpenAuthAdmin,
  children,
}) {
  return (
    <Layout className="app-shell">
      <Layout.Sider className="sidebar" width={250} breakpoint="lg" collapsedWidth={0}>
        <div className="sidebar-brand-wrap">
          <div className="brand">{productName}</div>
        </div>
        <div className="sidebar-menu-wrap">
          <Menu
            className="dir-menu dashboard-dir-menu"
            mode="inline"
            theme="dark"
            selectedKeys={[menu]}
            items={sidebarMenuItems}
            onClick={({ key }) => setMenu(String(key))}
          />
        </div>
      </Layout.Sider>

      <Layout className="content-layout">
        <Layout.Header className="content-head">
          <div className="content-head-left">
            <h1>{MENU_ITEMS.find((m) => m.key === menu)?.label}</h1>
            {loading ? <p><Badge variant="secondary">加载中</Badge></p> : null}
            {error ? <p className="head-error">{error}</p> : null}
          </div>
          <div className="theme-switcher">
            {currentUser ? (
              <div className="auth-user-box">
                <div className="auth-user-meta">
                  <span className="auth-user-name">{currentUser.username}</span>
                  <span className="auth-user-role">{currentUser.role_name || '-'}</span>
                </div>
                {typeof onOpenAuthAdmin === 'function' ? (
                  <ActionButton className="btn-flat btn-flat-indigo btn-sm" onClick={onOpenAuthAdmin}>
                    权限审计
                  </ActionButton>
                ) : null}
                <ActionButton className="btn-flat btn-flat-gray btn-sm" onClick={onLogout}>
                  退出登录
                </ActionButton>
              </div>
            ) : null}
            <span>主题</span>
            <Select
              className="theme-mode-select"
              size="small"
              value={themeMode}
              options={[
                { value: 'light', label: '浅色模式' },
                { value: 'dark', label: '深色模式' },
                { value: 'system', label: '跟随系统' },
              ]}
              onChange={(value) => setThemeMode(value)}
            />
          </div>
        </Layout.Header>
        <Layout.Content className="content">
          {children}
        </Layout.Content>
      </Layout>
    </Layout>
  )
}
