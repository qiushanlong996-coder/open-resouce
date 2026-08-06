import { useCallback, useEffect, useState } from 'react'
import {
  X, LayoutDashboard, Users, FolderKanban, KeyRound, ScrollText, Search, RefreshCw, Copy,
  ClipboardCheck, ExternalLink, Flag,
} from 'lucide-react'
import {
  getAdminStats, getAdminUsers, banUser, unbanUser,
  getAdminProjects, takedownProject, getApiKeys, issueApiKey, revokeApiKey, getAdminAudit,
  getAdminUserStats, getPendingProjectReviews, reviewProject, getAdminReports, resolveReport,
  type AuthUser, type AdminStats, type AdminUser, type ManagedProject, type ApiKey, type AdminAuditEntry,
  type AdminUserStats, type ContentReport,
} from './api/client'
import './admin.css'

type ModuleID = 'overview' | 'reviews' | 'reports' | 'users' | 'projects' | 'apikeys' | 'audit'

const MODULES: { id: ModuleID; label: string; icon: typeof Users }[] = [
  { id: 'overview', label: '概览', icon: LayoutDashboard },
  { id: 'reviews', label: '内容审核', icon: ClipboardCheck },
  { id: 'reports', label: '举报处理', icon: Flag },
  { id: 'users', label: '用户管理', icon: Users },
  { id: 'projects', label: '项目管理', icon: FolderKanban },
  { id: 'apikeys', label: '开放 API', icon: KeyRound },
  { id: 'audit', label: '审计日志', icon: ScrollText },
]

const errorMessage = (value: unknown, fallback: string) =>
  value instanceof Error ? value.message : fallback

function formatDate(value?: string | null) {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '-' : date.toLocaleString()
}

// renderLastLogin 展示最近一次登录的 IP 归属地与时间。
//
// 三种情况要分开表达，不能混成一个「-」：
//   - 从未登录过（本功能上线前的存量用户）→「暂无记录」；
//   - 登录过但归属地判定不出来（内网来源、IP 库无此段）→ 只显示时间；
//   - 两者都有 → 归属地为主、时间为辅。
// 服务端只存归属地不存 IP，所以这里也拿不到、也不该显示原始 IP。
function renderLastLogin(user: AdminUser) {
  if (!user.last_login_at) return <span className="admin-muted">暂无记录</span>
  return (
    <>
      {user.last_login_region || <span className="admin-muted">归属地未知</span>}
      <small className="admin-mono" style={{ display: 'block' }}>{formatDate(user.last_login_at)}</small>
    </>
  )
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
            {active === 'reviews' && <ReviewsPanel />}
            {active === 'reports' && <ReportsPanel />}
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
            <span className="admin-chip" key={status}>{STATUS_LABELS[status] ?? status}: {count}</span>
          ))}
      </div>
      <UserStatsSection />
    </div>
  )
}

const LEVEL_LABELS = ['Lv.1', 'Lv.2', 'Lv.3', 'Lv.4', 'Lv.5', 'Lv.6']

function shortDate(value: string) {
  // value 形如 2026-08-01 → 08-01
  return value.length >= 10 ? value.slice(5) : value
}

// UserStatsSection 展示注册趋势与等级分布。图表为纯 CSS 柱状条（无第三方依赖），
// 单一色相表达量级，配合数值标签与可读结构，兼顾无障碍。
function UserStatsSection() {
  const [stats, setStats] = useState<AdminUserStats | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    getAdminUserStats(14, controller.signal)
      .then((response) => setStats(response.data))
      .catch((value) => { if (!controller.signal.aborted) setError(errorMessage(value, '用户统计加载失败')) })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [])

  if (loading) return <div className="admin-loading">正在加载用户统计…</div>
  if (error) return <div className="admin-error">{error}</div>
  if (!stats) return null

  const trendMax = Math.max(1, ...stats.registrations.map((point) => point.count))
  const newInWindow = stats.registrations.reduce((sum, point) => sum + point.count, 0)
  const levelMax = Math.max(1, ...stats.level_histogram.map((bucket) => bucket.count))

  return (
    <div className="admin-userstats">
      <div className="admin-stat-grid">
        <div className="admin-stat"><div className="value">{stats.total_users}</div><div className="label">用户总数</div></div>
        <div className="admin-stat"><div className="value">{newInWindow}</div><div className="label">近 {stats.days} 天新增</div></div>
        <div className="admin-stat"><div className="value">{stats.banned}</div><div className="label">已封禁</div></div>
      </div>

      <figure className="admin-chart" aria-label={`近 ${stats.days} 天每日注册用户数`}>
        <figcaption>近 {stats.days} 天注册趋势</figcaption>
        <div className="admin-vbars" role="img" aria-label={`近 ${stats.days} 天每日注册数，最高 ${trendMax}`}>
          {stats.registrations.map((point) => (
            <div className="admin-vbar" key={point.date} title={`${point.date}：${point.count} 人`}>
              <div className="admin-vbar-track">
                <div
                  className="admin-vbar-fill"
                  style={{ height: `${Math.round((point.count / trendMax) * 100)}%` }}
                />
              </div>
              <span className="admin-vbar-label">{shortDate(point.date)}</span>
            </div>
          ))}
        </div>
      </figure>

      <figure className="admin-chart" aria-label="用户等级分布">
        <figcaption>等级分布</figcaption>
        <div className="admin-hbars">
          {stats.level_histogram.map((bucket) => (
            <div className="admin-hbar" key={bucket.level}>
              <span className="admin-hbar-name">{LEVEL_LABELS[bucket.level - 1] ?? `Lv.${bucket.level}`}</span>
              <div className="admin-hbar-track">
                <div
                  className="admin-hbar-fill"
                  style={{ width: `${Math.round((bucket.count / levelMax) * 100)}%` }}
                />
              </div>
              <span className="admin-hbar-value">{bucket.count}</span>
            </div>
          ))}
        </div>
      </figure>
    </div>
  )
}

// ReviewsPanel 把原「项目审核中心」并入控制台：列出待审核项目、预览关键内容
// （名称/简介/详情、文档与代码入口），并通过既有的 /admin/reviews 端点通过/驳回。
function ReviewsPanel() {
  const [projects, setProjects] = useState<ManagedProject[]>([])
  const [reason, setReason] = useState<Record<string, string>>({})
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [processing, setProcessing] = useState<string | null>(null)

  const load = useCallback((signal?: AbortSignal) => {
    setLoading(true)
    setError('')
    getPendingProjectReviews(signal)
      .then((response) => setProjects(response.data))
      .catch((value) => { if (!signal?.aborted) setError(errorMessage(value, '审核列表加载失败')) })
      .finally(() => { if (!signal?.aborted) setLoading(false) })
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    load(controller.signal)
    return () => controller.abort()
  }, [load])

  const review = async (project: ManagedProject, action: 'approve' | 'reject') => {
    const trimmed = (reason[project.id] ?? '').trim()
    if (action === 'reject' && !trimmed) {
      setError('驳回项目时必须填写审核意见')
      return
    }
    setProcessing(project.id)
    setError('')
    try {
      await reviewProject(project.id, action, trimmed)
      setProjects((current) => current.filter((item) => item.id !== project.id))
    } catch (value) {
      setError(errorMessage(value, '审核操作失败'))
    } finally {
      setProcessing(null)
    }
  }

  return (
    <div>
      <div className="admin-toolbar">
        <span className="admin-toolbar-note">只有审核通过的项目才会进入公开目录与搜索。</span>
        <button className="admin-btn" onClick={() => load()} aria-label="刷新"><RefreshCw size={15} /></button>
      </div>
      {error && <div className="admin-error">{error}</div>}
      {loading ? <div className="admin-loading">正在加载…</div>
        : projects.length === 0 ? <div className="admin-empty">当前没有待审核项目。</div>
          : <div className="admin-review-list">
            {projects.map((project) => (
              <article className="admin-review-card" key={project.id}>
                <div className="admin-review-head">
                  <div>
                    <strong>{project.name}</strong>
                    <small className="admin-mono">{project.slug} · {project.category} · v{project.current_version}</small>
                  </div>
                  <span className="admin-badge warn">待审核</span>
                </div>
                <p className="admin-review-summary">{project.summary || '（无简介）'}</p>
                <details className="admin-review-detail">
                  <summary>查看完整资料</summary>
                  <p>{project.description || '（无详细介绍）'}</p>
                  <div className="admin-review-meta">
                    {project.tech_stack.length > 0 && <span className="admin-chip">{project.tech_stack.join(' · ')}</span>}
                    {project.license && <span className="admin-chip">{project.license}</span>}
                  </div>
                </details>
                <div className="admin-review-links">
                  {project.repository_url && (
                    <a className="admin-btn" href={project.repository_url} target="_blank" rel="noreferrer">
                      <ExternalLink size={13} /> 代码仓库
                    </a>
                  )}
                  <a className="admin-btn" href={`/projects/${encodeURIComponent(project.slug)}`} target="_blank" rel="noreferrer">
                    <ExternalLink size={13} /> 文档预览
                  </a>
                  {project.document_object_key && <span className="admin-badge muted">含文档包</span>}
                  {project.code_object_key && <span className="admin-badge muted">含代码包</span>}
                </div>
                <textarea
                  className="admin-review-reason"
                  maxLength={500}
                  placeholder="审核意见；驳回时必填"
                  value={reason[project.id] ?? ''}
                  onChange={(event) => setReason((current) => ({ ...current, [project.id]: event.target.value }))}
                />
                <div className="admin-review-actions">
                  <button className="admin-btn danger" disabled={processing !== null} onClick={() => void review(project, 'reject')}>驳回</button>
                  <button className="admin-btn primary" disabled={processing !== null} onClick={() => void review(project, 'approve')}>
                    {processing === project.id ? '处理中…' : '审核通过'}
                  </button>
                </div>
              </article>
            ))}
          </div>}
    </div>
  )
}

const REPORT_STATUS_LABELS: Record<string, string> = {
  open: '未处理', resolved: '已处理', dismissed: '已驳回',
}

const REPORT_TARGET_LABELS: Record<string, string> = {
  project: '项目', comment: '评论',
}

function reportStatusBadgeClass(status: string) {
  if (status === 'resolved') return 'ok'
  if (status === 'dismissed') return 'muted'
  return 'warn'
}

// ReportsPanel 内容举报处理：按状态筛选举报，处理（resolve）或驳回（dismiss），
// 操作后刷新列表。复用 admin.css 的表格与徽章样式。
function ReportsPanel() {
  const [reports, setReports] = useState<ContentReport[]>([])
  const [status, setStatus] = useState('open')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState<string | null>(null)

  const load = useCallback((signal?: AbortSignal) => {
    setLoading(true)
    setError('')
    getAdminReports(status || undefined, signal)
      .then((response) => setReports(response.data))
      .catch((value) => { if (!signal?.aborted) setError(errorMessage(value, '举报列表加载失败')) })
      .finally(() => { if (!signal?.aborted) setLoading(false) })
  }, [status])

  useEffect(() => {
    const controller = new AbortController()
    load(controller.signal)
    return () => controller.abort()
  }, [load])

  const handle = async (report: ContentReport, action: 'resolve' | 'dismiss') => {
    setBusy(report.id)
    setError('')
    try {
      await resolveReport(report.id, action)
      load()
    } catch (value) {
      setError(errorMessage(value, '举报处理失败'))
    } finally {
      setBusy(null)
    }
  }

  return (
    <div>
      <div className="admin-toolbar">
        <select className="admin-btn" value={status} onChange={(event) => setStatus(event.target.value)}>
          <option value="">全部状态</option>
          {Object.entries(REPORT_STATUS_LABELS).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
        </select>
        <button className="admin-btn" onClick={() => load()} aria-label="刷新"><RefreshCw size={15} /></button>
        <span className="admin-toolbar-note">处理后举报将从未处理队列移出。</span>
      </div>
      {error && <div className="admin-error">{error}</div>}
      {loading ? <div className="admin-loading">正在加载…</div>
        : reports.length === 0 ? <div className="admin-empty">没有匹配的举报。</div>
          : <table className="admin-table">
            <thead>
              <tr><th>时间</th><th>举报人</th><th>类型</th><th>目标</th><th>原因</th><th>状态</th><th>操作</th></tr>
            </thead>
            <tbody>
              {reports.map((report) => (
                <tr key={report.id}>
                  <td>{formatDate(report.created_at)}</td>
                  <td className="admin-mono">{report.reporter_email || report.reporter_name || report.reporter_id}</td>
                  <td>{REPORT_TARGET_LABELS[report.target_type] ?? report.target_type}</td>
                  <td className="admin-mono" title={report.target_id}>{report.target_id}</td>
                  <td>{report.reason}{report.detail && <small className="admin-mono" style={{ display: 'block' }}>{report.detail}</small>}</td>
                  <td><span className={`admin-badge ${reportStatusBadgeClass(report.status)}`}>{REPORT_STATUS_LABELS[report.status] ?? report.status}</span></td>
                  <td>
                    {report.status === 'open'
                      ? <span className="admin-inline-confirm">
                        <button className="admin-btn primary" disabled={busy === report.id} onClick={() => void handle(report, 'resolve')}>处理</button>
                        <button className="admin-btn" disabled={busy === report.id} onClick={() => void handle(report, 'dismiss')}>驳回</button>
                      </span>
                      : <span className="admin-badge muted">已归档</span>}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>}
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
              <tr><th>邮箱</th><th>昵称</th><th>等级</th><th>经验</th><th>注册时间</th><th>上次登录</th><th>状态</th><th>操作</th></tr>
            </thead>
            <tbody>
              {users.map((user) => (
                <tr key={user.id}>
                  <td className="admin-mono">{user.email}{user.is_admin && <span className="admin-badge muted" style={{ marginLeft: 6 }}>管理员</span>}</td>
                  <td>{user.display_name}</td>
                  <td>Lv.{user.level}</td>
                  <td>{user.experience}</td>
                  <td>{formatDate(user.created_at)}</td>
                  {/* 上次登录：归属地 + 时间。本功能上线前的存量用户尚无记录，
                      显示「暂无记录」而不是编造一个位置。 */}
                  <td>{renderLastLogin(user)}</td>
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
                    {key.revoked_at ? <span className="admin-badge muted">-</span>
                      : <button className="admin-btn danger" disabled={busy === key.id} onClick={() => void doRevoke(key)}>撤销</button>}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>}
    </div>
  )
}

// 已知的审计动作类型；用于日志筛选下拉。后端 action 取值来自 recordAdminAudit 调用点。
const AUDIT_ACTIONS: { value: string; label: string }[] = [
  { value: 'user_banned', label: '封禁用户' },
  { value: 'user_unbanned', label: '解封用户' },
  { value: 'project_takedown', label: '下架项目' },
  { value: 'project_review_approve', label: '审核通过' },
  { value: 'project_review_reject', label: '审核驳回' },
  { value: 'api_key_issued', label: '签发密钥' },
  { value: 'api_key_revoked', label: '撤销密钥' },
]

const AUDIT_LIMIT = 200

function AuditPanel() {
  const [entries, setEntries] = useState<AdminAuditEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [action, setAction] = useState('')

  const load = useCallback((signal?: AbortSignal) => {
    setLoading(true)
    setError('')
    getAdminAudit({ limit: AUDIT_LIMIT, action: action || undefined }, signal)
      .then((response) => setEntries(response.data))
      .catch((value) => { if (!signal?.aborted) setError(errorMessage(value, '审计日志加载失败')) })
      .finally(() => { if (!signal?.aborted) setLoading(false) })
  }, [action])

  useEffect(() => {
    const controller = new AbortController()
    load(controller.signal)
    return () => controller.abort()
  }, [load])

  const actionLabel = (value: string) => AUDIT_ACTIONS.find((item) => item.value === value)?.label ?? value

  return (
    <div>
      <div className="admin-toolbar">
        <select className="admin-btn" value={action} onChange={(event) => setAction(event.target.value)}>
          <option value="">全部动作</option>
          {AUDIT_ACTIONS.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}
        </select>
        <button className="admin-btn" onClick={() => load()}><RefreshCw size={15} /> 刷新</button>
        <span className="admin-toolbar-note">最多显示最近 {AUDIT_LIMIT} 条</span>
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
                  <td><span className="admin-badge muted" title={entry.action}>{actionLabel(entry.action)}</span></td>
                  <td className="admin-mono">{entry.actor_email || entry.actor_id || '-'}</td>
                  <td className="admin-mono">{entry.target || '-'}</td>
                  <td>{entry.detail || '-'}</td>
                </tr>
              ))}
            </tbody>
          </table>}
    </div>
  )
}
