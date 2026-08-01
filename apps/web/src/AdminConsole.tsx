import { useCallback, useEffect, useState } from 'react'
import {
  X, LayoutDashboard, Users, FolderKanban, KeyRound, ScrollText, Search, RefreshCw, Copy,
} from 'lucide-react'
import {
  getAdminStats, getAdminUsers, banUser, unbanUser,
  getAdminProjects, takedownProject, getApiKeys, issueApiKey, revokeApiKey, getAdminAudit,
  type AuthUser, type AdminStats, type AdminUser, type ManagedProject, type ApiKey, type AdminAuditEntry,
} from './api/client'
import './admin.css'

type ModuleID = 'overview' | 'users' | 'projects' | 'apikeys' | 'audit'

const MODULES: { id: ModuleID; label: string; icon: typeof Users }[] = [
  { id: 'overview', label: '概览', icon: LayoutDashboard },
  { id: 'users', label: '用户管理', icon: Users },
  { id: 'projects', label: '项目管理', icon: FolderKanban },
  { id: 'apikeys', label: '开放 API', icon: KeyRound },
  { id: 'audit', label: '审计日志', icon: ScrollText },
]

const errorMessage = (value: unknown, fallback: string) =>
  value instanceof Error ? value.message : fallback

function formatDate(value?: string | null) {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString()
}

export default function AdminConsole({ onClose, currentUser }: { onClose: () => void; currentUser: AuthUser }) {
  const [active, setActive] = useState<ModuleID>('overview')

  return (
    <div className="admin-backdrop" role="presentation" onMouseDown={onClose}>
      <div className="admin-shell" role="dialog" aria-modal="true" aria-label="管理控制台" onMouseDown={(event) => event.stopPropagation()}>
        <nav className="admin-nav">
          <div className="admin-nav-title">管理控制台</div>
          {MODULES.map((module) => {
            const Icon = module.icon
            return (
              <button
                key={module.id}
                className={active === module.id ? 'active' : ''}
                onClick={() => setActive(module.id)}
              >
                <Icon size={16} /> {module.label}
              </button>
            )
          })}
        </nav>
        <div className="admin-main">
          <header className="admin-header">
            <div>
              <h2>{MODULES.find((module) => module.id === active)?.label}</h2>
              <p>已登录：{currentUser.display_name} · {currentUser.email}</p>
            </div>
            <button className="admin-close" onClick={onClose} aria-label="关闭"><X size={20} /></button>
          </header>
          <div className="admin-body">
            {active === 'overview' && <OverviewPanel />}
            {active === 'users' && <UsersPanel currentUser={currentUser} />}
            {active === 'projects' && <ProjectsPanel />}
            {active === 'apikeys' && <ApiKeysPanel />}
            {active === 'audit' && <AuditPanel />}
          </div>
        </div>
      </div>
    </div>
  )
}

function OverviewPanel() {
  const [stats, setStats] = useState<AdminStats | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    getAdminStats(controller.signal)
      .then((response) => setStats(response.data))
      .catch((value) => { if (!controller.signal.aborted) setError(errorMessage(value, '统计加载失败')) })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [])

  if (loading) return <div className="admin-loading">正在加载概览…</div>
  if (error) return <div className="admin-error">{error}</div>
  if (!stats) return <div className="admin-empty">暂无统计数据。</div>

  const cards = [
    { label: '注册用户', value: stats.users },
    { label: '项目总数', value: stats.projects_total },
    { label: '待审核', value: stats.pending_reviews },
    { label: '评论总数', value: stats.comments },
  ]
  return (
    <div>
      <div className="admin-stat-grid">
        {cards.map((card) => (
          <div className="admin-stat" key={card.label}>
            <div className="value">{card.value}</div>
            <div className="label">{card.label}</div>
          </div>
        ))}
      </div>
      <div className="admin-substats">
        {Object.entries(stats.projects_by_status).length === 0
          ? <span className="admin-chip">暂无项目</span>
          : Object.entries(stats.projects_by_status).map(([status, count]) => (
            <span className="admin-chip" key={status}>{status}: {count}</span>
          ))}
      </div>
    </div>
  )
}

function UsersPanel({ currentUser }: { currentUser: AuthUser }) {
  const [users, setUsers] = useState<AdminUser[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [search, setSearch] = useState('')
  const [query, setQuery] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState<string | null>(null)
  const [banning, setBanning] = useState<string | null>(null)
  const [banReason, setBanReason] = useState('')
  const pageSize = 20

  const load = useCallback((signal?: AbortSignal) => {
    setLoading(true)
    setError('')
    getAdminUsers({ search: query, page, page_size: pageSize }, signal)
      .then((response) => { setUsers(response.data); setTotal(response.total) })
      .catch((value) => { if (!signal?.aborted) setError(errorMessage(value, '用户加载失败')) })
      .finally(() => { if (!signal?.aborted) setLoading(false) })
  }, [query, page])

  useEffect(() => {
    const controller = new AbortController()
    load(controller.signal)
    return () => controller.abort()
  }, [load])

  const submitSearch = () => { setPage(1); setQuery(search.trim()) }

  const doBan = async (user: AdminUser) => {
    setBusy(user.id)
    try {
      await banUser(user.id, banReason.trim())
      setBanning(null); setBanReason('')
      load()
    } catch (value) {
      setError(errorMessage(value, '封禁失败'))
    } finally {
      setBusy(null)
    }
  }

  const doUnban = async (user: AdminUser) => {
    setBusy(user.id)
    try {
      await unbanUser(user.id)
      load()
    } catch (value) {
      setError(errorMessage(value, '解封失败'))
    } finally {
      setBusy(null)
    }
  }

  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  return (
    <div>
      <div className="admin-toolbar">
        <div className="admin-search">
          <Search size={15} />
          <input
            placeholder="搜索邮箱或昵称"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            onKeyDown={(event) => { if (event.key === 'Enter') submitSearch() }}
          />
        </div>
        <button className="admin-btn" onClick={submitSearch}>搜索</button>
        <button className="admin-btn" onClick={() => load()} aria-label="刷新"><RefreshCw size={15} /></button>
      </div>
      {error && <div className="admin-error">{error}</div>}
      {loading ? <div className="admin-loading">正在加载…</div>
        : users.length === 0 ? <div className="admin-empty">没有匹配的用户。</div>
          : <table className="admin-table">
            <thead>
              <tr><th>邮箱</th><th>昵称</th><th>等级</th><th>经验</th><th>注册时间</th><th>状态</th><th>操作</th></tr>
            </thead>
            <tbody>
              {users.map((user) => (
                <tr key={user.id}>
                  <td className="admin-mono">{user.email}{user.is_admin && <span className="admin-badge muted" style={{ marginLeft: 6 }}>管理员</span>}</td>
                  <td>{user.display_name}</td>
                  <td>Lv.{user.level}</td>
                  <td>{user.experience}</td>
                  <td>{formatDate(user.created_at)}</td>
                  <td>{user.banned ? <span className="admin-badge danger">已封禁</span> : <span className="admin-badge ok">正常</span>}</td>
                  <td>
                    {user.id === currentUser.id ? <span className="admin-badge muted">本人</span>
                      : user.banned
                        ? <button className="admin-btn" disabled={busy === user.id} onClick={() => void doUnban(user)}>解封</button>
                        : banning === user.id
                          ? <span className="admin-inline-confirm">
                            <input placeholder="封禁原因（可选）" value={banReason} onChange={(event) => setBanReason(event.target.value)} />
                            <button className="admin-btn danger" disabled={busy === user.id} onClick={() => void doBan(user)}>确认封禁</button>
                            <button className="admin-btn" onClick={() => { setBanning(null); setBanReason('') }}>取消</button>
                          </span>
                          : <button className="admin-btn danger" onClick={() => { setBanning(user.id); setBanReason('') }}>封禁</button>}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>}
      <div className="admin-pagination">
        <span>共 {total} 人 · 第 {page}/{totalPages} 页</span>
        <button className="admin-btn" disabled={page <= 1} onClick={() => setPage((value) => value - 1)}>上一页</button>
        <button className="admin-btn" disabled={page >= totalPages} onClick={() => setPage((value) => value + 1)}>下一页</button>
      </div>
    </div>
  )
}

const STATUS_LABELS: Record<string, string> = {
  draft: '草稿', pending_review: '待审核', published: '已发布', rejected: '已驳回', archived: '已下架',
}

function statusBadgeClass(status: string) {
  if (status === 'published') return 'ok'
  if (status === 'pending_review') return 'warn'
  if (status === 'archived' || status === 'rejected') return 'danger'
  return 'muted'
}

function ProjectsPanel() {
  const [projects, setProjects] = useState<ManagedProject[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [status, setStatus] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState<string | null>(null)
  const [confirming, setConfirming] = useState<string | null>(null)
  const [reason, setReason] = useState('')
  const pageSize = 20

  const load = useCallback((signal?: AbortSignal) => {
    setLoading(true)
    setError('')
    getAdminProjects({ status, page, page_size: pageSize }, signal)
      .then((response) => { setProjects(response.data); setTotal(response.total) })
      .catch((value) => { if (!signal?.aborted) setError(errorMessage(value, '项目加载失败')) })
      .finally(() => { if (!signal?.aborted) setLoading(false) })
  }, [status, page])

  useEffect(() => {
    const controller = new AbortController()
    load(controller.signal)
    return () => controller.abort()
  }, [load])

  const doTakedown = async (project: ManagedProject) => {
    setBusy(project.id)
    try {
      await takedownProject(project.id, reason.trim())
      setConfirming(null); setReason('')
      load()
    } catch (value) {
      setError(errorMessage(value, '下架失败'))
    } finally {
      setBusy(null)
    }
  }

  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  return (
    <div>
      <div className="admin-toolbar">
        <select
          className="admin-btn"
          value={status}
          onChange={(event) => { setPage(1); setStatus(event.target.value) }}
        >
          <option value="">全部状态</option>
          {Object.entries(STATUS_LABELS).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
        </select>
        <button className="admin-btn" onClick={() => load()} aria-label="刷新"><RefreshCw size={15} /></button>
      </div>
      {error && <div className="admin-error">{error}</div>}
      {loading ? <div className="admin-loading">正在加载…</div>
        : projects.length === 0 ? <div className="admin-empty">没有项目。</div>
          : <table className="admin-table">
            <thead>
              <tr><th>名称</th><th>标识</th><th>分类</th><th>状态</th><th>更新时间</th><th>操作</th></tr>
            </thead>
            <tbody>
              {projects.map((project) => (
                <tr key={project.id}>
                  <td>{project.name}</td>
                  <td className="admin-mono">{project.slug}</td>
                  <td>{project.category}</td>
                  <td><span className={`admin-badge ${statusBadgeClass(project.status)}`}>{STATUS_LABELS[project.status] ?? project.status}</span></td>
                  <td>{formatDate(project.updated_at)}</td>
                  <td>
                    {project.status === 'archived' ? <span className="admin-badge muted">已下架</span>
                      : confirming === project.id
                        ? <span className="admin-inline-confirm">
                          <input placeholder="下架原因（可选）" value={reason} onChange={(event) => setReason(event.target.value)} />
                          <button className="admin-btn danger" disabled={busy === project.id} onClick={() => void doTakedown(project)}>确认下架</button>
                          <button className="admin-btn" onClick={() => { setConfirming(null); setReason('') }}>取消</button>
                        </span>
                        : <button className="admin-btn danger" onClick={() => { setConfirming(project.id); setReason('') }}>下架</button>}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>}
      <div className="admin-pagination">
        <span>共 {total} 个 · 第 {page}/{totalPages} 页</span>
        <button className="admin-btn" disabled={page <= 1} onClick={() => setPage((value) => value - 1)}>上一页</button>
        <button className="admin-btn" disabled={page >= totalPages} onClick={() => setPage((value) => value + 1)}>下一页</button>
      </div>
    </div>
  )
}

function ApiKeysPanel() {
  const [keys, setKeys] = useState<ApiKey[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [name, setName] = useState('')
  const [issuing, setIssuing] = useState(false)
  const [issued, setIssued] = useState('')
  const [busy, setBusy] = useState<string | null>(null)

  const load = useCallback((signal?: AbortSignal) => {
    setLoading(true)
    setError('')
    getApiKeys(signal)
      .then((response) => setKeys(response.data))
      .catch((value) => { if (!signal?.aborted) setError(errorMessage(value, '密钥加载失败')) })
      .finally(() => { if (!signal?.aborted) setLoading(false) })
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    load(controller.signal)
    return () => controller.abort()
  }, [load])

  const doIssue = async () => {
    if (!name.trim()) { setError('请填写密钥名称'); return }
    setIssuing(true)
    setError('')
    try {
      const response = await issueApiKey(name.trim())
      setIssued(response.data.plaintext)
      setName('')
      load()
    } catch (value) {
      setError(errorMessage(value, '签发失败'))
    } finally {
      setIssuing(false)
    }
  }

  const doRevoke = async (key: ApiKey) => {
    setBusy(key.id)
    try {
      await revokeApiKey(key.id)
      load()
    } catch (value) {
      setError(errorMessage(value, '撤销失败'))
    } finally {
      setBusy(null)
    }
  }

  return (
    <div>
      {issued && (
        <div className="admin-notice">
          <span>密钥已生成，仅此一次可见：<strong className="admin-mono">{issued}</strong></span>
          <button className="admin-btn" onClick={() => { void navigator.clipboard?.writeText(issued) }}><Copy size={14} /> 复制</button>
          <button className="admin-btn" onClick={() => setIssued('')}>我已保存</button>
        </div>
      )}
      <div className="admin-toolbar">
        <div className="admin-search">
          <KeyRound size={15} />
          <input placeholder="新密钥名称" value={name} onChange={(event) => setName(event.target.value)} />
        </div>
        <button className="admin-btn primary" disabled={issuing} onClick={() => void doIssue()}>{issuing ? '签发中…' : '签发密钥'}</button>
        <button className="admin-btn" onClick={() => load()} aria-label="刷新"><RefreshCw size={15} /></button>
      </div>
      {error && <div className="admin-error">{error}</div>}
      {loading ? <div className="admin-loading">正在加载…</div>
        : keys.length === 0 ? <div className="admin-empty">还没有签发任何密钥。</div>
          : <table className="admin-table">
            <thead>
              <tr><th>名称</th><th>前缀</th><th>创建时间</th><th>状态</th><th>操作</th></tr>
            </thead>
            <tbody>
              {keys.map((key) => (
                <tr key={key.id}>
                  <td>{key.name}</td>
                  <td className="admin-mono">{key.prefix}…</td>
                  <td>{formatDate(key.created_at)}</td>
                  <td>{key.revoked_at ? <span className="admin-badge danger">已撤销</span> : <span className="admin-badge ok">有效</span>}</td>
                  <td>
                    {key.revoked_at ? <span className="admin-badge muted">—</span>
                      : <button className="admin-btn danger" disabled={busy === key.id} onClick={() => void doRevoke(key)}>撤销</button>}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>}
    </div>
  )
}

function AuditPanel() {
  const [entries, setEntries] = useState<AdminAuditEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const load = useCallback((signal?: AbortSignal) => {
    setLoading(true)
    setError('')
    getAdminAudit(100, signal)
      .then((response) => setEntries(response.data))
      .catch((value) => { if (!signal?.aborted) setError(errorMessage(value, '审计日志加载失败')) })
      .finally(() => { if (!signal?.aborted) setLoading(false) })
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    load(controller.signal)
    return () => controller.abort()
  }, [load])

  return (
    <div>
      <div className="admin-toolbar">
        <button className="admin-btn" onClick={() => load()}><RefreshCw size={15} /> 刷新</button>
      </div>
      {error && <div className="admin-error">{error}</div>}
      {loading ? <div className="admin-loading">正在加载…</div>
        : entries.length === 0 ? <div className="admin-empty">暂无审计记录。</div>
          : <table className="admin-table">
            <thead>
              <tr><th>时间</th><th>操作</th><th>操作者</th><th>目标</th><th>详情</th></tr>
            </thead>
            <tbody>
              {entries.map((entry) => (
                <tr key={entry.id}>
                  <td>{formatDate(entry.created_at)}</td>
                  <td><span className="admin-badge muted">{entry.action}</span></td>
                  <td className="admin-mono">{entry.actor_email || entry.actor_id || '—'}</td>
                  <td className="admin-mono">{entry.target || '—'}</td>
                  <td>{entry.detail || '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>}
    </div>
  )
}
