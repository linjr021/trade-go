import { useEffect, useMemo, useState } from 'react'
import { ActionButton } from '@/components/ui/action-button'
import { Modal, Select, Space, Table, Tabs } from '@/components/ui/dashboard-primitives'
import {
  createAuthRole,
  createAuthUser,
  deleteAuthRole,
  deleteAuthUser,
  getAuthAuditLogs,
  getAuthRoles,
  getAuthUsers,
  updateAuthRole,
  updateAuthUserPassword,
  updateAuthUserRole,
} from '@/api'
import { resolveRequestError } from '@/modules/trade-utils'

type RoleRow = Record<string, any>
type UserRow = Record<string, any>

const ACCESS_OPTIONS = [
  { value: 'none', label: '不可见' },
  { value: 'read', label: '只读' },
  { value: 'edit', label: '编辑' },
]

const MODULE_LABELS: Record<string, string> = {
  assets: '资产详情',
  live: '实盘交易',
  paper: '模拟交易',
  skill_workflow: 'AI 工作流',
  builder: '策略生成',
  backtest: '历史回测',
  auth_admin: '权限审计',
  system: '系统设置',
}

function moduleLabelCN(moduleKey: string) {
  const key = String(moduleKey || '').trim()
  return MODULE_LABELS[key] || key || '未知模块'
}

function normalizePermMap(modules: string[] = [], raw: Record<string, any> = {}) {
  const out: Record<string, string> = {}
  for (const m of modules) out[m] = 'none'
  for (const [k, v] of Object.entries(raw || {})) {
    const key = String(k || '').trim()
    if (!key) continue
    const val = String(v || '').trim().toLowerCase()
    out[key] = ['none', 'read', 'edit'].includes(val) ? val : 'none'
  }
  return out
}

export function AuthAdminPanel({
  open = true,
  onClose = () => {},
  inline = false,
}: {
  open?: boolean
  onClose?: () => void
  inline?: boolean
}) {
  const [tab, setTab] = useState('users')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [roles, setRoles] = useState<RoleRow[]>([])
  const [users, setUsers] = useState<UserRow[]>([])
  const [auditLogs, setAuditLogs] = useState<Record<string, any>[]>([])
  const [modules, setModules] = useState<string[]>([])
  const [creatingRole, setCreatingRole] = useState(false)
  const [creatingUser, setCreatingUser] = useState(false)
  const [savingRoleId, setSavingRoleId] = useState(0)
  const [savingUserRoleId, setSavingUserRoleId] = useState(0)
  const [resetPwdUserId, setResetPwdUserId] = useState(0)
  const [deletingUserId, setDeletingUserId] = useState(0)
  const [deletingRoleId, setDeletingRoleId] = useState(0)

  const [newRoleName, setNewRoleName] = useState('')
  const [newRolePerms, setNewRolePerms] = useState<Record<string, string>>({})
  const [rolePermEdits, setRolePermEdits] = useState<Record<string, Record<string, string>>>({})
  const [newUsername, setNewUsername] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [newUserRoleId, setNewUserRoleId] = useState('')
  const [passwordResetMap, setPasswordResetMap] = useState<Record<string, string>>({})

  const roleOptions = useMemo(
    () => roles.map((r) => ({ value: String(r.id), label: `${r.name}${r.is_super ? '（超级）' : ''}` })),
    [roles],
  )

  const loadAll = async (silent = false) => {
    if (!silent) setLoading(true)
    setError('')
    try {
      const [roleRes, userRes, logRes] = await Promise.all([
        getAuthRoles(),
        getAuthUsers(),
        getAuthAuditLogs({ limit: 200 }),
      ])
      const roleRows = Array.isArray(roleRes?.data?.roles) ? roleRes.data.roles : []
      const modRows = Array.isArray(roleRes?.data?.modules) ? roleRes.data.modules.map((x: any) => String(x || '').trim()).filter(Boolean) : []
      const userRows = Array.isArray(userRes?.data?.users) ? userRes.data.users : []
      const logs = Array.isArray(logRes?.data?.logs) ? logRes.data.logs : []

      setRoles(roleRows)
      setModules(modRows)
      setUsers(userRows)
      setAuditLogs(logs)

      const roleEditMap: Record<string, Record<string, string>> = {}
      roleRows.forEach((r: any) => {
        roleEditMap[String(r.id)] = normalizePermMap(modRows, r.permissions || {})
      })
      setRolePermEdits(roleEditMap)
      setNewRolePerms((old) => (Object.keys(old).length ? old : normalizePermMap(modRows, {})))
      setNewUserRoleId((old) => old || (roleRows[0] ? String(roleRows[0].id) : ''))
    } catch (err) {
      setError(resolveRequestError(err, '加载权限审计数据失败'))
    } finally {
      if (!silent) setLoading(false)
    }
  }

  useEffect(() => {
    if (inline || open) {
      void loadAll(false)
    }
  }, [inline, open])

  const createRole = async () => {
    if (creatingRole) return
    setCreatingRole(true)
    setError('')
    try {
      await createAuthRole({
        name: String(newRoleName || '').trim(),
        permissions: normalizePermMap(modules, newRolePerms),
      })
      setNewRoleName('')
      setNewRolePerms(normalizePermMap(modules, {}))
      await loadAll(true)
    } catch (err) {
      setError(resolveRequestError(err, '创建角色失败'))
    } finally {
      setCreatingRole(false)
    }
  }

  const saveRolePerm = async (role: RoleRow) => {
    const id = Number(role?.id || 0)
    if (!id || savingRoleId === id) return
    setSavingRoleId(id)
    setError('')
    try {
      await updateAuthRole({
        id,
        name: String(role?.name || '').trim(),
        permissions: normalizePermMap(modules, rolePermEdits[String(id)] || {}),
      })
      await loadAll(true)
    } catch (err) {
      setError(resolveRequestError(err, '更新角色权限失败'))
    } finally {
      setSavingRoleId(0)
    }
  }

  const createUser = async () => {
    if (creatingUser) return
    setCreatingUser(true)
    setError('')
    try {
      await createAuthUser({
        username: String(newUsername || '').trim(),
        password: String(newPassword || ''),
        role_id: Number(newUserRoleId || 0),
      })
      setNewUsername('')
      setNewPassword('')
      await loadAll(true)
    } catch (err) {
      setError(resolveRequestError(err, '创建用户失败'))
    } finally {
      setCreatingUser(false)
    }
  }

  const changeUserRole = async (user: UserRow, roleId: string) => {
    const uid = Number(user?.id || 0)
    if (!uid || savingUserRoleId === uid) return
    setSavingUserRoleId(uid)
    setError('')
    try {
      await updateAuthUserRole({ user_id: uid, role_id: Number(roleId || 0) })
      await loadAll(true)
    } catch (err) {
      setError(resolveRequestError(err, '更新用户角色失败'))
    } finally {
      setSavingUserRoleId(0)
    }
  }

  const resetUserPassword = async (user: UserRow) => {
    const uid = Number(user?.id || 0)
    if (!uid || resetPwdUserId === uid) return
    const nextPwd = String(passwordResetMap[String(uid)] || '')
    if (!nextPwd) return
    setResetPwdUserId(uid)
    setError('')
    try {
      await updateAuthUserPassword({ user_id: uid, password: nextPwd })
      setPasswordResetMap((old) => ({ ...old, [String(uid)]: '' }))
      await loadAll(true)
    } catch (err) {
      setError(resolveRequestError(err, '重置密码失败'))
    } finally {
      setResetPwdUserId(0)
    }
  }

  const removeUser = async (user: UserRow) => {
    const uid = Number(user?.id || 0)
    if (!uid || deletingUserId === uid) return
    const username = String(user?.username || '').trim() || `ID=${uid}`
    if (!window.confirm(`确认删除用户「${username}」吗？`)) return
    setDeletingUserId(uid)
    setError('')
    try {
      await deleteAuthUser({ user_id: uid })
      setPasswordResetMap((old) => ({ ...old, [String(uid)]: '' }))
      await loadAll(true)
    } catch (err) {
      setError(resolveRequestError(err, '删除用户失败'))
    } finally {
      setDeletingUserId(0)
    }
  }

  const removeRole = async (role: RoleRow) => {
    const rid = Number(role?.id || 0)
    if (!rid || deletingRoleId === rid) return
    const name = String(role?.name || '').trim() || `ID=${rid}`
    if (!window.confirm(`确认删除权限组「${name}」吗？`)) return
    setDeletingRoleId(rid)
    setError('')
    try {
      await deleteAuthRole({ role_id: rid })
      await loadAll(true)
    } catch (err) {
      setError(resolveRequestError(err, '删除权限组失败'))
    } finally {
      setDeletingRoleId(0)
    }
  }

  const content = (
    <div className="auth-admin-panel">
      <Tabs
        className="dashboard-tabs"
        activeKey={tab}
        onChange={setTab}
        items={[
          { key: 'users', label: '用户管理' },
          { key: 'roles', label: '权限组' },
          { key: 'audit', label: '审计日志' },
        ]}
      />
      {error ? <p className="login-error">{error}</p> : null}
      {loading ? <p className="muted">加载中...</p> : null}

      {tab === 'users' && (
        <section className="stack">
          <section className="sub-window auth-create-window">
            <div className="card-head">
              <h4>新增用户</h4>
            </div>
            <div className="auth-grid">
              <label>
                <span>账号（5-10位英数）</span>
                <input value={newUsername} onChange={(e) => setNewUsername(e.target.value)} placeholder="如 trader01" />
              </label>
              <label>
                <span>密码（8-16位，英文+数字+符号）</span>
                <input type="password" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} placeholder="请输入密码" />
              </label>
              <label>
                <span>权限组</span>
                <Select
                  className="auth-admin-select"
                  value={newUserRoleId}
                  onChange={setNewUserRoleId}
                  options={roleOptions}
                />
              </label>
            </div>
            <div className="actions-row end">
              <ActionButton className="btn-flat btn-flat-blue" loading={creatingUser} onClick={createUser}>
                {creatingUser ? '创建中...' : '创建用户'}
              </ActionButton>
            </div>
          </section>

          <section className="sub-window auth-list-window">
            <div className="card-head">
              <h4>用户列表</h4>
            </div>
            <Table
              className="auth-list-table"
              dataSource={users.map((u: any) => ({ ...u, key: `user-${u.id}` }))}
              columns={[
                { title: 'ID', dataIndex: 'id' },
                { title: '账号', dataIndex: 'username' },
                { title: '最后登录', dataIndex: 'last_login_at' },
                {
                  title: '权限组',
                  key: 'roleEdit',
                  render: (_: any, row: any) => (
                    <Space>
                      <Select
                        className="auth-admin-select"
                        value={String(row.role_id || '')}
                        options={roleOptions}
                        onChange={(value) => changeUserRole(row, value)}
                      />
                    </Space>
                  ),
                },
                {
                  title: '密码重置',
                  key: 'pwdReset',
                  render: (_: any, row: any) => {
                    const id = String(row.id || '')
                    return (
                      <Space>
                        <input
                          className="auth-inline-input"
                          type="password"
                          value={passwordResetMap[id] || ''}
                          onChange={(e) => setPasswordResetMap((old) => ({ ...old, [id]: e.target.value }))}
                          placeholder="输入新密码"
                        />
                        <ActionButton
                          className="btn-flat btn-flat-indigo btn-sm"
                          loading={resetPwdUserId === Number(row.id)}
                          onClick={() => resetUserPassword(row)}
                        >
                          保存
                        </ActionButton>
                      </Space>
                    )
                  },
                },
                {
                  title: '操作',
                  key: 'actions',
                  render: (_: any, row: any) => (
                    row?.built_in ? (
                      <span className="muted">内置</span>
                    ) : (
                      <ActionButton
                        className="btn-flat btn-flat-rose btn-sm"
                        loading={deletingUserId === Number(row.id)}
                        onClick={() => removeUser(row)}
                      >
                        删除
                      </ActionButton>
                    )
                  ),
                },
              ]}
              pagination={{ pageSize: 20 }}
              size="small"
            />
          </section>
        </section>
      )}

      {tab === 'roles' && (
        <section className="stack">
          <section className="sub-window auth-create-window">
            <div className="card-head"><h4>新增权限组</h4></div>
            <div className="auth-grid">
              <label>
                <span>权限组名（支持中文）</span>
                <input value={newRoleName} onChange={(e) => setNewRoleName(e.target.value)} placeholder="如 超短线只读组" />
              </label>
            </div>
            <div className="role-perm-grid">
              {modules.map((mod) => (
                <label key={`new-role-mod-${mod}`}>
                  <span>{moduleLabelCN(mod)}</span>
                  <Select
                    className="auth-admin-select"
                    value={newRolePerms[mod] || 'none'}
                    onChange={(value) => setNewRolePerms((old) => ({ ...old, [mod]: value }))}
                    options={ACCESS_OPTIONS}
                  />
                </label>
              ))}
            </div>
            <div className="actions-row end">
              <ActionButton className="btn-flat btn-flat-blue" loading={creatingRole} onClick={createRole}>
                {creatingRole ? '创建中...' : '创建权限组'}
              </ActionButton>
            </div>
          </section>

          <section className="sub-window auth-list-window">
            <div className="card-head">
              <h4>权限列表</h4>
            </div>
            {roles.map((role) => (
              <section className="sub-window auth-role-item-window" key={`role-${role.id}`}>
                <div className="card-head">
                  <h4>{role.name}{role.is_super ? '（超级）' : ''}{role.built_in ? '（内置）' : ''}</h4>
                </div>
                <div className="role-perm-grid">
                  {modules.map((mod) => (
                    <label key={`role-${role.id}-${mod}`}>
                      <span>{moduleLabelCN(mod)}</span>
                      <Select
                        className="auth-admin-select"
                        disabled={Boolean(role?.built_in)}
                        value={(rolePermEdits[String(role.id)] || {})[mod] || 'none'}
                        onChange={(value) => setRolePermEdits((old) => ({
                          ...old,
                          [String(role.id)]: {
                            ...(old[String(role.id)] || {}),
                            [mod]: value,
                          },
                        }))}
                        options={ACCESS_OPTIONS}
                      />
                    </label>
                  ))}
                </div>
                {!role?.built_in ? (
                  <div className="actions-row end">
                    <ActionButton
                      className="btn-flat btn-flat-indigo btn-sm"
                      loading={savingRoleId === Number(role.id)}
                      onClick={() => saveRolePerm(role)}
                    >
                      保存权限
                    </ActionButton>
                    <ActionButton
                      className="btn-flat btn-flat-rose btn-sm"
                      loading={deletingRoleId === Number(role.id)}
                      onClick={() => removeRole(role)}
                    >
                      删除权限组
                    </ActionButton>
                  </div>
                ) : null}
              </section>
            ))}
          </section>
        </section>
      )}

      {tab === 'audit' && (
        <Table
          dataSource={auditLogs.map((x: any) => ({ ...x, key: `audit-${x.id}` }))}
          columns={[
            { title: '时间', dataIndex: 'ts' },
            { title: '用户', dataIndex: 'username' },
            { title: '动作', dataIndex: 'action' },
            {
              title: '模块',
              dataIndex: 'module',
              render: (value: any) => moduleLabelCN(String(value || '')),
            },
            { title: '目标', dataIndex: 'target' },
            { title: '结果', dataIndex: 'result' },
            { title: 'IP', dataIndex: 'ip' },
          ]}
          pagination={{ pageSize: 30 }}
          scroll={{ y: 360 }}
          size="small"
        />
      )}
    </div>
  )

  if (inline) {
    return (
      <div className="builder-pane">
        <section className="sub-window">
          <div className="card-head">
            <h4>权限审计 + 账号管理</h4>
          </div>
          {content}
        </section>
      </div>
    )
  }

  return (
    <Modal open={open} title="权限审计" onCancel={onClose}>
      {content}
    </Modal>
  )
}
