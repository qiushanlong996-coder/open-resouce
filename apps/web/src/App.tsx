import { isValidElement, lazy, Suspense, useEffect, useMemo, useRef, useState, type FormEvent, type KeyboardEvent as ReactKeyboardEvent, type ReactNode } from 'react'
import ReactMarkdown, { defaultUrlTransform } from 'react-markdown'
import rehypeHighlight from 'rehype-highlight'
import remarkGfm from 'remark-gfm'
import {
  ArrowLeft,
  ArrowUpRight,
  Bell,
  BookOpen,
  Check,
  ChevronDown,
  ChevronRight,
  CircleUserRound,
  Code2,
  Copy,
  Download,
  Eye,
  FileCode2,
  FileText,
  Flag,
  GitBranch,
  Heart,
  History,
  Home as HomeIcon,
  Link2,
  ThumbsUp,
  Menu,
  WrapText,
  MessageSquare,
  MonitorCog,
  Moon,
  MoreHorizontal,
  Palette,
  Pencil,
  Play,
  RefreshCw,
  Rocket,
  Search,
  Send,
  Share2,
  ShieldCheck,
  Star,
  Sun,
  Tag,
  Trash2,
  Upload,
  UserPlus,
  X,
} from 'lucide-react'
import {
  ApiError,
  changePassword,
  confirmPasswordReset,
  createAuthorProject,
  createDocumentComment,
  createDocumentCommentReply,
  deleteDocumentComment,
  deleteDocumentCommentReply,
  deleteProjectCollaborator,
  getDocument,
  getDocumentCommentEventsURL,
  getDocumentComments,
  getAuthSessions,
  getAuthorProjects,
  getCurrentUser,
  getDocuments,
  getFavorites,
  getFollows,
  getNotificationEventsURL,
  getNotifications,
  getProject,
  recordProjectView,
  getProjectCodeArchiveURL,
  getProjectCodeFileDownloadURL,
  getProjectCodeFile,
  getProjectCodeTree,
  getProjectCollaborationAccess,
  getProjectCollaborators,
  getProjectCollaborationWebSocketURL,
  getAuthorProjectComments,
  getAuthorProjectMetrics,
  getProjects,
  getServiceInfo,
  getSiteStats,
  getLeaderboard,
  getFeaturedProjects,
  getHotTags,
  getActivity,
  login,
  logout,
  logoutAll,
  markAllNotificationsRead,
  markNotificationRead,
  register,
  requestPasswordReset,
  revokeAuthSession,
  setCommentLike,
  setProjectFavorite,
  setProjectFollow,
  shareProject,
  setProjectCollaborator,
  submitAuthorProject,
  uploadProjectFile,
  resolveDocumentComment,
  updateDocumentComment,
  updateDocumentCommentReply,
  updateCurrentUser,
  updateAuthorProject,
  updateAuthorProjectDocument,
  type DocumentComment as APIDocumentComment,
  type DocumentDetail,
  type DocumentNode,
  type AppNotification,
  type AuthSession,
  type AuthUser,
  type CodeEntry,
  type CodeFile,
  type ManagedProject,
  type ManagedProjectInput,
  type ProjectDocument,
  type AuthorCommentItem,
  type ProjectMetrics,
  type CollaborationAccess,
  type ProjectCollaborator,
  type ProjectDetail as APIProjectDetail,
  type ProjectSummary,
  type ServiceInfo,
  type SiteStats,
  type LeaderboardUser,
  type TagCount,
  type ActivityItem,
} from './api/client'
import type { RichMarkdownEditorHandle } from './RichMarkdownEditor'
import {
  BilibiliEmbed,
  CodeBlock,
  DocumentSearchBox,
  HeadingAnchor,
  ImageLightbox,
  MermaidDiagram,
} from './DocumentReader'
import ErrorBoundary from './ErrorBoundary'
import { BrandMark } from './BrandMark'
import { LevelAvatar, LevelBadge } from './LevelAvatar'
import type { ReportTarget } from './ReportDialog'
import { bilibiliEmbedURL, useDocumentSearch } from './documentReaderUtils'
import {
  BackToTopButton,
  DocumentPager,
  ReaderSettingsControl,
  ReadingProgressBar,
} from './ReaderExperience'
import {
  flattenDocumentTree,
  useReaderPreferences,
  useScrollMetrics,
  useScrollSpy,
} from './readerExperienceUtils'
import { useHighlightedLines } from './codeHighlight'
import { themes, applyTheme, isThemeId, type ThemeId } from './themes'
import './App.css'
import './editorEnhancements.css'
import './codeView.css'

const EmojiMartPicker = lazy(() => import('./EmojiMartPicker'))
const RichMarkdownEditor = lazy(() => import('./RichMarkdownEditor'))
const CollaborativeMarkdownEditor = lazy(() => import('./CollaborativeMarkdownEditor'))
const ProjectDocumentTree = lazy(() => import('./ProjectDocumentTree'))
const DocumentSearchPanel = lazy(() => import('./DocumentSearchPanel'))
const DocumentRevisionPanel = lazy(() => import('./DocumentRevisionPanel'))
const AiAssistant = lazy(() => import('./AiAssistant'))
const ReportDialog = lazy(() => import('./ReportDialog'))
const AdminConsole = lazy(() => import('./AdminConsole'))
const AccessKeyManager = lazy(() => import('./AccessKeyManager'))
const OpenApiDocs = lazy(() => import('./OpenApiDocs'))
const UserProfile = lazy(() => import('./UserProfile').then((module) => ({ default: module.UserProfile })))
const AvatarFramePicker = lazy(() => import('./AvatarFramePicker').then((module) => ({ default: module.AvatarFramePicker })))

// 顶栏浮层是互斥的：同一时刻最多只有一个下拉面板可见，null 表示全部关闭。
type HeaderPopover = 'theme' | 'notification' | 'account' | 'mobileMenu' | null

// 快捷键提示要跟平台一致：Mac 上是 ⌘，其他平台是 Ctrl。
// 之前顶栏固定写死 ⌘ K，在 Windows 上是错的，而且当时根本没有对应的处理器。
const isAppleShortcutPlatform = /Mac|iPhone|iPad|iPod/i.test(navigator.platform || navigator.userAgent)
const shortcutModifierLabel = isAppleShortcutPlatform ? '⌘' : 'Ctrl'

// shouldIgnoreShortcut 判断当前焦点是否在可编辑区域。
// 在输入框、textarea 或富文本编辑器里打字时不能劫持快捷键。
function shouldIgnoreShortcut(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) return false
  if (target.isContentEditable) return true
  const tag = target.tagName
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT'
}

type Project = {
  id: string
  name: string
  slug: string
  summary: string
  description: string
  category: string
  tags: string[]
  stack: string[]
  license: string
  updated: string
  views: string
  downloads: string
  stars: string
  comments: number
  maintainer: string
  initials: string
  accent: string
  status: string
  repo: string
  // 用于「趋势 / 最新更新」排序的原始值；种子演示项目缺失时回退。
  updatedAt?: string
  viewsValue?: number
  starsValue?: number
  highlights?: string[]
  useCases?: string[]
  currentVersion?: string
  resources?: { cover: boolean; document: boolean; code: boolean }
  // 已发布项目的作者信息，用于把「作者」链接到其公开主页；种子项目为空。
  ownerId?: string
  authorName?: string
  hasCover?: boolean
}

type CommentItem = {
  id: string
  blockId: string
  authorId: string
  authorLevel: number
  authorAvatar: string
  authorFrame: string
  // authorRegion 为空时不渲染（历史评论、内网来源、未配置 IP 库）。
  authorRegion: string
  user: string
  initials: string
  time: string
  quote: string
  text: string
  status: 'open' | 'resolved'
  replies: CommentItem[]
  replyCount: number
  likeCount: number
  liked: boolean
  updatedAt: string
  edited: boolean
}

type ThemeMode = 'light' | 'dark' | 'system'
type Skin = ThemeId
type GatewayState =
  | { status: 'checking' }
  | { status: 'online'; info: ServiceInfo }
  | { status: 'offline' }
type CatalogState = 'checking' | 'online' | 'offline'

const categories = ['全部项目', 'Multi-Agent', 'RAG Agent', 'Coding Agent', 'Workflow Agent', 'Agent Framework']

const projects: Project[] = [
  {
    id: 'atlas',
    name: 'Atlas Agent',
    slug: 'atlas-agent',
    summary: '面向复杂任务的多 Agent 协作运行时',
    description: '把规划、检索、执行和复盘拆成可观察的 Agent 节点，让复杂任务保持清晰、可控、可复用。',
    category: 'Multi-Agent',
    tags: ['多智能体', '工作流', '可观测'],
    stack: ['Python', 'LangGraph', 'OpenAI'],
    license: 'Apache-2.0',
    updated: '2 小时前',
    views: '46k',
    downloads: '12.8k',
    stars: '2.4k',
    comments: 18,
    maintainer: '北岛实验室',
    initials: 'B',
    accent: 'blue',
    status: '活跃维护',
    repo: 'github.com/atlas-lab/agent',
  },
  {
    id: 'paperclip',
    name: 'Paperclip RAG',
    slug: 'paperclip-rag',
    summary: '为团队知识库设计的轻量 RAG Agent',
    description: '从文档清洗、切分、检索到带引用回答，提供一条适合内部知识库的可视化链路。',
    category: 'RAG Agent',
    tags: ['知识库', '引用回答', '中文'],
    stack: ['TypeScript', 'LlamaIndex', 'Elasticsearch'],
    license: 'MIT',
    updated: '昨天',
    views: '29k',
    downloads: '8.1k',
    stars: '1.8k',
    comments: 12,
    maintainer: 'Paperclip',
    initials: 'P',
    accent: 'orange',
    status: '生产可用',
    repo: 'github.com/paperclip-ai/rag',
  },
  {
    id: 'forge',
    name: 'Forge Runner',
    slug: 'forge-runner',
    summary: '让 Coding Agent 安全执行真实工程任务',
    description: '提供沙箱、工具调用、补丁预览和测试回放，让编码 Agent 的每一步都能被开发者检查。',
    category: 'Coding Agent',
    tags: ['代码 Agent', '沙箱', '工具调用'],
    stack: ['Go', 'React', 'Docker'],
    license: 'MIT',
    updated: '3 天前',
    views: '21k',
    downloads: '5.6k',
    stars: '960',
    comments: 9,
    maintainer: 'Forge Team',
    initials: 'F',
    accent: 'green',
    status: '实验项目',
    repo: 'github.com/forge-runner/core',
  },
  {
    id: 'relay',
    name: 'Relay MCP',
    slug: 'relay-mcp',
    summary: '面向工具调用的 MCP 服务编排层',
    description: '把分散的工具能力整理成可发现、可授权、可审计的服务目录，降低 Agent 接入成本。',
    category: 'Agent Framework',
    tags: ['MCP', '工具调用', '服务治理'],
    stack: ['Go', 'MCP', 'Redis'],
    license: 'Apache-2.0',
    updated: '上周',
    views: '15k',
    downloads: '4.2k',
    stars: '742',
    comments: 7,
    maintainer: 'Relay Open',
    initials: 'R',
    accent: 'purple',
    status: '活跃维护',
    repo: 'github.com/relay-open/mcp',
  },
]

const compactNumber = new Intl.NumberFormat('en', { notation: 'compact', maximumFractionDigits: 1 })

function mapProjectSummary(project: ProjectSummary): Project {
  const demoProject = projects.find((item) => item.id === project.id)
  return {
    id: project.id,
    name: project.name,
    slug: project.slug,
    summary: project.summary,
    description: demoProject?.description ?? project.summary,
    category: project.category,
    tags: project.tags,
    stack: project.stack,
    license: project.license,
    updated: new Date(project.updated_at).toLocaleDateString('zh-CN'),
    views: compactNumber.format(project.metrics.views ?? 0).toLowerCase(),
    downloads: compactNumber.format(project.metrics.downloads).toLowerCase(),
    stars: compactNumber.format(project.metrics.stars).toLowerCase(),
    comments: project.metrics.comments,
    maintainer: project.maintainer,
    initials: demoProject?.initials ?? project.name.slice(0, 1).toUpperCase(),
    accent: demoProject?.accent ?? 'blue',
    status: project.status,
    repo: demoProject?.repo ?? '',
    updatedAt: project.updated_at,
    viewsValue: project.metrics.views ?? 0,
    starsValue: project.metrics.stars ?? 0,
    highlights: demoProject?.highlights,
    useCases: demoProject?.useCases,
    currentVersion: demoProject?.currentVersion,
    hasCover: project.has_cover,
  }
}

function mapProjectDetail(project: APIProjectDetail): Project {
  return {
    ...mapProjectSummary(project),
    description: project.description,
    highlights: project.highlights,
    useCases: project.use_cases,
    repo: project.repository,
    currentVersion: project.current_version,
    resources: project.resources,
    ownerId: project.owner_id,
    authorName: project.author_name,
  }
}

function mapDocumentComment(comment: APIDocumentComment): CommentItem {
  return {
    id: comment.id,
    blockId: comment.block_id,
    authorId: comment.author_id ?? '',
    authorLevel: comment.author_level ?? 1,
    authorAvatar: comment.author_avatar ?? '',
    authorFrame: comment.author_frame ?? '',
    authorRegion: comment.author_region ?? '',
    user: comment.author,
    initials: comment.author.slice(0, 1),
    time: new Intl.DateTimeFormat('zh-CN', { dateStyle: 'short', timeStyle: 'short' }).format(new Date(comment.created_at)),
    quote: comment.quote,
    text: comment.body,
    status: comment.status,
    // 单条评论响应可能不包含 replies，这里容错以免渲染阶段抛异常导致白屏。
    replies: (comment.replies ?? []).map(mapDocumentComment),
    replyCount: comment.reply_count ?? 0,
    likeCount: comment.like_count ?? 0,
    liked: comment.liked ?? false,
    updatedAt: comment.updated_at,
    edited: comment.updated_at !== comment.created_at,
  }
}

// firstDocumentSlug 深度优先取目录中的首篇文档，用于打开项目时默认展示。
function firstDocumentSlug(nodes: DocumentNode[]): string {
  for (const node of nodes) {
    if (node.slug) return node.slug
    const child = firstDocumentSlug(node.children ?? [])
    if (child) return child
  }
  return ''
}

function reactNodeText(node: ReactNode): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node)
  if (Array.isArray(node)) return node.map(reactNodeText).join('')
  if (isValidElement<{ children?: ReactNode }>(node)) return reactNodeText(node.props.children)
  return ''
}

function App() {
  const [activeTab, setActiveTab] = useState('探索')
  const [activeCategory, setActiveCategory] = useState('全部项目')
  const [selectedProject, setSelectedProject] = useState<Project | null>(null)
  const [profileUserId, setProfileUserId] = useState<string | null>(null)
  const [detailTab, setDetailTab] = useState('文档阅读')
  const [saved, setSaved] = useState<string[]>([])
  const [followed, setFollowed] = useState<string[]>([])
  const [comments, setComments] = useState<CommentItem[]>([])
  const [commentsState, setCommentsState] = useState<CatalogState>('online')
  const [commentSubmitting, setCommentSubmitting] = useState(false)
  const [resolvingCommentID, setResolvingCommentID] = useState<string | null>(null)
  const [deletingCommentID, setDeletingCommentID] = useState<string | null>(null)
  const [draftComment, setDraftComment] = useState('')
  const [commentComposerOpen, setCommentComposerOpen] = useState(false)
  const [selectedQuote, setSelectedQuote] = useState('每个节点都需要声明输入、输出和失败策略。')
  const [selectedBlockID, setSelectedBlockID] = useState('block-atlas-collaboration')
  // 选区评论框的锚点：null 表示回退到底部固定位置（例如从侧边栏直接新建评论）。
  const [composerAnchor, setComposerAnchor] = useState<{ top: number } | null>(null)
  const [toast, setToast] = useState('')
  const [loginOpen, setLoginOpen] = useState(() => new URLSearchParams(window.location.search).has('reset_token'))
  const [currentUser, setCurrentUser] = useState<AuthUser | null>(null)
  const [authChecked, setAuthChecked] = useState(false)
  // 顶栏浮层同时只允许打开一个，避免主题、通知、账号面板互相叠加。
  const [headerPopover, setHeaderPopover] = useState<HeaderPopover>(null)
  const [notifications, setNotifications] = useState<AppNotification[]>([])
  const [unreadNotificationCount, setUnreadNotificationCount] = useState(0)
  const [notificationsLoading, setNotificationsLoading] = useState(false)
  const [markingNotifications, setMarkingNotifications] = useState(false)
  const [authorCenterOpen, setAuthorCenterOpen] = useState(false)
  const [adminConsoleOpen, setAdminConsoleOpen] = useState(
    () => new URLSearchParams(window.location.search).get('admin') === '1',
  )
  const [accessKeyOpen, setAccessKeyOpen] = useState(false)
  const [avatarFramePickerOpen, setAvatarFramePickerOpen] = useState(false)
  const [reportTarget, setReportTarget] = useState<ReportTarget | null>(null)
  const [logoutSubmitting, setLogoutSubmitting] = useState(false)
  const [authSessions, setAuthSessions] = useState<AuthSession[]>([])
  const [sessionsLoading, setSessionsLoading] = useState(false)
  const [revokingSessionID, setRevokingSessionID] = useState<string | null>(null)
  const [passwordEditing, setPasswordEditing] = useState(false)
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [passwordSubmitting, setPasswordSubmitting] = useState(false)
  const [profileEditing, setProfileEditing] = useState(false)
  const [profileDisplayName, setProfileDisplayName] = useState('')
  const [profileSubmitting, setProfileSubmitting] = useState(false)
  const [themeMode, setThemeMode] = useState<ThemeMode>(() => {
    const stored = window.localStorage.getItem('xinyuan-theme-mode')
    return stored === 'light' || stored === 'dark' || stored === 'system' ? stored : 'system'
  })
  const [skin, setSkin] = useState<Skin>(() => {
    const stored = window.localStorage.getItem('xinyuan-skin')
    return isThemeId(stored) ? stored : 'ocean'
  })
  // 跟随系统模式时，需要知道当前系统偏好明暗，才能应用对应的一套主题变量。
  const [systemDark, setSystemDark] = useState(() => window.matchMedia('(prefers-color-scheme: dark)').matches)
  const [searchPanelOpen, setSearchPanelOpen] = useState(false)
  const [openDocsOpen, setOpenDocsOpen] = useState(false)
  const [gatewayState, setGatewayState] = useState<GatewayState>({ status: 'checking' })
  const [catalogProjects, setCatalogProjects] = useState<Project[]>(projects)
  const [catalogState, setCatalogState] = useState<CatalogState>('checking')
  const [detailState, setDetailState] = useState<CatalogState>('online')
  const [documentState, setDocumentState] = useState<CatalogState>('online')
  const [documentTree, setDocumentTree] = useState<DocumentNode[]>([])
  const [activeDocument, setActiveDocument] = useState<DocumentDetail | null>(null)
  const selectedProjectSlug = useRef<string | null>(null)
  const documentRequestSequence = useRef(0)
  // 从通知跳转过来时，记录要定位的评论线程 ID，等评论加载后滚动高亮。
  const pendingCommentFocus = useRef<string | null>(null)
  const headerRef = useRef<HTMLElement | null>(null)

  const themePanelOpen = headerPopover === 'theme'
  const notificationPanelOpen = headerPopover === 'notification'
  const accountPanelOpen = headerPopover === 'account'
  const mobileMenuOpen = headerPopover === 'mobileMenu'
  const closeHeaderPopover = () => setHeaderPopover(null)
  const toggleHeaderPopover = (panel: Exclude<HeaderPopover, null>) =>
    setHeaderPopover((current) => (current === panel ? null : panel))

  // ⌘K / Ctrl+K 打开统一搜索面板。
  // 顶栏那个 <kbd>⌘ K</kbd> 之前是纯装饰，没有任何处理器绑定它。
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key.toLowerCase() !== 'k') return
      if (!(event.metaKey || event.ctrlKey) || event.altKey || event.shiftKey) return
      if (shouldIgnoreShortcut(event.target)) return
      // 阻止浏览器自身的 ⌘K（地址栏搜索）。
      event.preventDefault()
      closeHeaderPopover()
      setSearchPanelOpen(true)
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [])

  // 点击顶栏之外的任意位置，或按 Esc，都要收起当前浮层。
  useEffect(() => {
    if (!headerPopover) return
    const handlePointerDown = (event: PointerEvent) => {
      const target = event.target
      if (target instanceof Node && headerRef.current?.contains(target)) return
      closeHeaderPopover()
    }
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') closeHeaderPopover()
    }
    document.addEventListener('pointerdown', handlePointerDown)
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('pointerdown', handlePointerDown)
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [headerPopover])

  useEffect(() => {
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = (event: MediaQueryListEvent) => setSystemDark(event.matches)
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [])

  useEffect(() => {
    const root = document.documentElement
    root.dataset.themeMode = themeMode
    root.dataset.skin = skin
    // 结算出最终明暗，并把对应主题的整套 CSS 变量（含 --app-bg）内联到 <html>。
    const dark = themeMode === 'dark' || (themeMode === 'system' && systemDark)
    applyTheme(root, skin, dark)
    window.localStorage.setItem('xinyuan-theme-mode', themeMode)
    window.localStorage.setItem('xinyuan-skin', skin)
  }, [themeMode, skin, systemDark])

  // 按当前页面动态更新浏览器标题，利于分享与多标签识别。
  useEffect(() => {
    if (selectedProject) {
      document.title = `${selectedProject.name} · 新猿译码`
    } else if (activeTab && activeTab !== '探索') {
      document.title = `${activeTab} · 新猿译码`
    } else {
      document.title = '新猿译码 · Agent 开源项目分享平台'
    }
  }, [selectedProject, activeTab])

  // 直达链接：/projects/{slug}[?document=...] 可直达项目/文档，后退可返回首页。
  useEffect(() => {
    const openFromURL = () => {
      const match = /^\/projects\/([^/?#]+)/.exec(window.location.pathname)
      if (!match) {
        setSelectedProject(null)
        selectedProjectSlug.current = null
        return
      }
      const documentSlug = new URLSearchParams(window.location.search).get('document')
      openProjectBySlug(
        decodeURIComponent(match[1]),
        documentSlug ? () => openDocument(documentSlug) : undefined,
      )
    }
    openFromURL()
    window.addEventListener('popstate', openFromURL)
    return () => window.removeEventListener('popstate', openFromURL)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    getServiceInfo(controller.signal)
      .then((info) => setGatewayState({ status: 'online', info }))
      .catch((error: unknown) => {
        if (error instanceof DOMException && error.name === 'AbortError') return
        setGatewayState({ status: 'offline' })
      })
    return () => controller.abort()
  }, [])

  useEffect(() => {
    if (!currentUser) {
      setSaved([])
      return
    }
    const controller = new AbortController()
    getFavorites(controller.signal)
      .then((response) => setSaved(response.data))
      .catch((error: unknown) => {
        if (error instanceof DOMException && error.name === 'AbortError') return
        showToast('收藏状态加载失败，请稍后重试')
      })
    return () => controller.abort()
  }, [currentUser])

  useEffect(() => {
    if (!currentUser) {
      setFollowed([])
      return
    }
    const controller = new AbortController()
    getFollows(controller.signal)
      .then((response) => setFollowed(response.data))
      .catch((error: unknown) => {
        if (error instanceof DOMException && error.name === 'AbortError') return
        showToast('关注状态加载失败，请稍后重试')
      })
    return () => controller.abort()
  }, [currentUser])

  useEffect(() => {
    const controller = new AbortController()
    getCurrentUser(controller.signal)
      .then((response) => setCurrentUser(response.data))
      .catch(() => setCurrentUser(null))
      .finally(() => setAuthChecked(true))
    return () => controller.abort()
  }, [])

  useEffect(() => {
    if (!authChecked || !adminConsoleOpen) return
    if (!currentUser) {
      setLoginOpen(true)
      return
    }
    if (!currentUser.is_admin) {
      setAdminConsoleOpen(false)
      const url = new URL(window.location.href)
      url.searchParams.delete('admin')
      window.history.replaceState(window.history.state, '', url.toString())
      showToast('当前账号没有管理权限')
    }
  }, [adminConsoleOpen, authChecked, currentUser])

  useEffect(() => {
    if (!currentUser) {
      setNotifications([])
      setUnreadNotificationCount(0)
      closeHeaderPopover()
      return
    }
    const controller = new AbortController()
    setNotificationsLoading(true)
    getNotifications(controller.signal)
      .then((response) => {
        setNotifications(response.data)
        setUnreadNotificationCount(response.unread_count)
      })
      .catch((error: unknown) => {
        if (error instanceof DOMException && error.name === 'AbortError') return
      })
      .finally(() => setNotificationsLoading(false))

    const eventSource = new EventSource(getNotificationEventsURL(), { withCredentials: true })
    let refreshController: AbortController | null = null
    const refreshNotifications = () => {
      refreshController?.abort()
      refreshController = new AbortController()
      getNotifications(refreshController.signal)
        .then((response) => {
          setNotifications(response.data)
          setUnreadNotificationCount(response.unread_count)
        })
        .catch(() => {})
    }
    eventSource.addEventListener('notification', refreshNotifications)

    return () => {
      controller.abort()
      refreshController?.abort()
      eventSource.removeEventListener('notification', refreshNotifications)
      eventSource.close()
    }
  }, [currentUser])

  useEffect(() => {
    if (!currentUser || !accountPanelOpen) return
    const controller = new AbortController()
    setSessionsLoading(true)
    getAuthSessions(controller.signal)
      .then((response) => setAuthSessions(response.data))
      .catch((error: unknown) => {
        if (error instanceof DOMException && error.name === 'AbortError') return
        showToast('登录会话加载失败，请稍后重试')
      })
      .finally(() => setSessionsLoading(false))
    return () => controller.abort()
  }, [accountPanelOpen, currentUser])

  useEffect(() => {
    const controller = new AbortController()
    const timer = window.setTimeout(() => {
      setCatalogState('checking')
      getProjects({
        category: activeCategory === '全部项目' ? undefined : activeCategory,
        pageSize: 50,
        sort: 'updated',
      }, controller.signal)
        .then((response) => {
          setCatalogProjects(response.data.map(mapProjectSummary))
          setCatalogState('online')
        })
        .catch((error: unknown) => {
          if (error instanceof DOMException && error.name === 'AbortError') return
          setCatalogProjects(projects)
          setCatalogState('offline')
        })
    }, 250)

    return () => {
      window.clearTimeout(timer)
      controller.abort()
    }
  }, [activeCategory])

  useEffect(() => {
    if (!selectedProject || !activeDocument) return
    setSelectedBlockID(activeDocument.blocks[0]?.id ?? '')
    setCommentsState('checking')
    const controller = new AbortController()
    getDocumentComments(selectedProject.slug, activeDocument.slug, controller.signal)
      .then((response) => {
        setComments(response.data.map(mapDocumentComment))
        setCommentsState('online')
      })
      .catch((error: unknown) => {
        if (error instanceof DOMException && error.name === 'AbortError') return
        setCommentsState('offline')
        setComments([])
      })
    return () => controller.abort()
  }, [activeDocument, selectedProject])

  // 从通知跳转过来时，等目标文档的评论加载完再滚动定位并短暂高亮。
  useEffect(() => {
    const targetId = pendingCommentFocus.current
    if (!targetId) return
    if (!comments.some((comment) => comment.id === targetId)) return
    pendingCommentFocus.current = null
    // 评论卡片渲染完成后再定位。
    window.setTimeout(() => {
      const element = document.getElementById(`comment-${targetId}`)
      if (!element) return
      element.scrollIntoView({ behavior: 'smooth', block: 'center' })
      element.classList.add('is-focused')
      window.setTimeout(() => element.classList.remove('is-focused'), 2400)
    }, 80)
  }, [comments])

  useEffect(() => {
    if (!selectedProject || !activeDocument) return
    const projectSlug = selectedProject.slug
    const documentSlug = activeDocument.slug
    const eventSource = new EventSource(getDocumentCommentEventsURL(projectSlug, documentSlug))
    let refreshController: AbortController | null = null

    eventSource.onopen = () => setCommentsState('online')
    eventSource.onerror = () => setCommentsState('offline')
    const refreshComments = () => {
      refreshController?.abort()
      refreshController = new AbortController()
      getDocumentComments(projectSlug, documentSlug, refreshController.signal)
        .then((response) => {
          setComments(response.data.map(mapDocumentComment))
          setCommentsState('online')
        })
        .catch((error: unknown) => {
          if (error instanceof DOMException && error.name === 'AbortError') return
          setCommentsState('offline')
        })
    }
    eventSource.addEventListener('comment', refreshComments)

    return () => {
      refreshController?.abort()
      eventSource.removeEventListener('comment', refreshComments)
      eventSource.close()
    }
  }, [activeDocument, selectedProject])

  // 关键词检索统一交给搜索面板（ES），这里只做分类与收藏的本地筛选。
  const filteredProjects = useMemo(() => catalogProjects.filter((project) => {
    if (activeTab === '我的收藏' && !saved.includes(project.id)) return false
    if (activeTab === '我的关注' && !followed.includes(project.id)) return false
    return activeCategory === '全部项目' || project.category === activeCategory
  }), [activeCategory, activeTab, catalogProjects, saved, followed])

  const showToast = (message: string) => {
    setToast(message)
    window.setTimeout(() => setToast(''), 2400)
  }

  const setAdminConsoleLocation = (open: boolean) => {
    const url = new URL(window.location.href)
    if (open) url.searchParams.set('admin', '1')
    else url.searchParams.delete('admin')
    window.history.replaceState(window.history.state, '', url.toString())
  }

  const openAdminConsole = () => {
    setAdminConsoleOpen(true)
    closeHeaderPopover()
    setAdminConsoleLocation(true)
  }

  const closeAdminConsole = () => {
    setAdminConsoleOpen(false)
    setAdminConsoleLocation(false)
  }

  // openReport 打开举报弹窗，未登录时先引导登录（登录门禁在入口处统一处理）。
  const openReport = (target: ReportTarget) => {
    if (!currentUser) {
      showToast('登录后才能举报')
      setLoginOpen(true)
      return
    }
    setReportTarget(target)
  }

  const markAllRead = async () => {
    setMarkingNotifications(true)
    try {
      await markAllNotificationsRead()
      const readAt = new Date().toISOString()
      setNotifications((current) => current.map((entry) => entry.read_at ? entry : { ...entry, read_at: readAt }))
      setUnreadNotificationCount(0)
    } catch {
      showToast('通知标记失败，请稍后重试')
    } finally {
      setMarkingNotifications(false)
    }
  }

  // navigateToNotification 按通知类型跳转到来源。
  const navigateToNotification = (entry: AppNotification) => {
    // 驳回的项目未发布，公开详情会 404，跳到作者项目中心查看驳回原因。
    if (entry.type === 'project.rejected') {
      setAuthorCenterOpen(true)
      return
    }
    if (!entry.project_slug) return
    if (entry.document_slug) {
      // 回复通知：打开对应文档，评论加载后再定位到该评论线程。
      pendingCommentFocus.current = entry.comment_id ?? null
      openSearchResult(entry.project_slug, entry.document_slug)
      return
    }
    // 审核通过等仅带项目的通知：打开已发布项目详情。
    openProjectBySlug(entry.project_slug)
  }

  const openNotification = async (entry: AppNotification) => {
    closeHeaderPopover()
    navigateToNotification(entry)
    if (entry.read_at) return
    try {
      await markNotificationRead(entry.id)
      const readAt = new Date().toISOString()
      setNotifications((current) => current.map((item) => item.id === entry.id ? { ...item, read_at: readAt } : item))
      setUnreadNotificationCount((count) => Math.max(0, count - 1))
    } catch {
      showToast('通知标记失败，请稍后重试')
    }
  }

  // updatePublishedDocument 把协作保存的正文回写到前端状态。
  // 当前打开了具体文档时只更新该文档，不能拿文档正文覆盖项目简介。
  const updatePublishedDocument = (markdown: string) => {
    const slug = selectedProjectSlug.current
    if (!slug) return
    if (activeDocument) {
      setActiveDocument((current) => current ? { ...current, markdown } : current)
      return
    }
    setSelectedProject((current) => current?.slug === slug ? { ...current, description: markdown } : current)
    setCatalogProjects((current) => current.map((project) =>
      project.slug === slug ? { ...project, description: markdown } : project))
  }

  const closeProject = () => {
    selectedProjectSlug.current = null
    documentRequestSequence.current += 1
    setSelectedProject(null)
    // 关闭详情时 URL 回到首页，保证「复制链接」与地址栏一致。
    const url = new URL(window.location.href)
    url.pathname = '/'
    url.search = ''
    window.history.replaceState(window.history.state, '', url.toString())
  }

  // openProfile 打开某个用户的公开主页弹窗（评论作者、项目作者点击进入）。
  const openProfile = (userId: string) => {
    if (userId) setProfileUserId(userId)
  }

  // openProjectBySlug 只有 slug 时打开项目详情。目录里没有（比如列表分页未
  // 加载到）时直接拉详情。afterOpen 在项目打开后执行，用于继续定位文档。
  const openProjectBySlug = (projectSlug: string, afterOpen?: () => void) => {
    const known = catalogProjects.find((project) => project.slug === projectSlug)
    if (known) {
      openProject(known)
      if (afterOpen) window.setTimeout(afterOpen, 0)
      return
    }
    getProject(projectSlug)
      .then((response) => {
        openProject(mapProjectDetail(response.data))
        if (afterOpen) window.setTimeout(afterOpen, 0)
      })
      .catch(() => showToast('项目打开失败'))
  }

  // openSearchResult 从搜索结果或通知跳转。打开项目后再切到命中的那篇文档
  //（目录加载完会默认打开首篇）。
  const openSearchResult = (projectSlug: string, documentSlug: string) => {
    openProjectBySlug(projectSlug, () => openDocument(documentSlug))
  }

  const openProject = (project: Project) => {
    selectedProjectSlug.current = project.slug
    setSelectedProject(project)
    // 同步到 URL：/projects/{slug} 可分享、可直达、可后退。
    const url = new URL(window.location.href)
    url.pathname = `/projects/${encodeURIComponent(project.slug)}`
    url.search = ''
    window.history.pushState({ projectSlug: project.slug }, '', url.toString())
    // 记录一次浏览（fire-and-forget，失败不影响打开）。
    void recordProjectView(project.slug).catch(() => {})
    setDetailTab('文档阅读')
    setDetailState('checking')
    setDocumentState('checking')
    setDocumentTree([])
    setActiveDocument(null)
    const requestSequence = ++documentRequestSequence.current
    getProject(project.slug)
      .then((response) => {
        if (selectedProjectSlug.current !== project.slug) return
        setSelectedProject((current) => current?.slug === project.slug ? mapProjectDetail(response.data) : current)
        setDetailState('online')
      })
      .catch(() => {
        if (selectedProjectSlug.current === project.slug) setDetailState('offline')
      })
    // 先取目录再取首篇正文：不同项目的文档 slug 不同（已发布项目为 overview，
    // 种子项目为 quick-start），硬编码 slug 会导致真实项目取不到文档。
    getDocuments(project.slug)
      .then((treeResponse) => {
        if (selectedProjectSlug.current !== project.slug || documentRequestSequence.current !== requestSequence) return
        setDocumentTree(treeResponse.data)
        const firstSlug = firstDocumentSlug(treeResponse.data)
        if (!firstSlug) {
          setDocumentState('online')
          return
        }
        return getDocument(project.slug, firstSlug).then((documentResponse) => {
          if (selectedProjectSlug.current !== project.slug || documentRequestSequence.current !== requestSequence) return
          setActiveDocument(documentResponse.data)
          setDocumentState('online')
        })
      })
      .catch(() => {
        if (selectedProjectSlug.current === project.slug && documentRequestSequence.current === requestSequence) setDocumentState('offline')
      })
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  const openDocument = (documentSlug: string) => {
    const projectSlug = selectedProjectSlug.current
    if (!projectSlug || activeDocument?.slug === documentSlug) return
    // 把当前文档写进 ?document=，深链可直接定位到具体文档。
    const url = new URL(window.location.href)
    url.searchParams.set('document', documentSlug)
    window.history.replaceState(window.history.state, '', url.toString())
    const requestSequence = ++documentRequestSequence.current
    setDocumentState('checking')
    getDocument(projectSlug, documentSlug)
      .then((response) => {
        if (selectedProjectSlug.current !== projectSlug || documentRequestSequence.current !== requestSequence) return
        setActiveDocument(response.data)
        setDocumentState('online')
        window.scrollTo({ top: 0, behavior: 'smooth' })
      })
      .catch(() => {
        if (selectedProjectSlug.current === projectSlug && documentRequestSequence.current === requestSequence) setDocumentState('offline')
      })
  }

  const toggleSaved = async (projectId: string) => {
    if (!currentUser) {
      setLoginOpen(true)
      showToast('登录后才能收藏项目')
      return
    }
    const project = catalogProjects.find((item) => item.id === projectId)
    if (!project) return
    const wasSaved = saved.includes(projectId)
    setSaved((current) => wasSaved ? current.filter((id) => id !== projectId) : [...current, projectId])
    try {
      await setProjectFavorite(project.slug, !wasSaved)
      showToast(wasSaved ? '已取消收藏' : '已收藏到个人中心')
    } catch {
      setSaved((current) => wasSaved ? [...current, projectId] : current.filter((id) => id !== projectId))
      showToast('收藏状态更新失败，请稍后重试')
    }
  }

  const toggleFollowed = async (projectId: string) => {
    if (!currentUser) {
      setLoginOpen(true)
      showToast('登录后才能关注项目')
      return
    }
    const project = catalogProjects.find((item) => item.id === projectId)
    if (!project) return
    const wasFollowed = followed.includes(projectId)
    setFollowed((current) => wasFollowed ? current.filter((id) => id !== projectId) : [...current, projectId])
    try {
      await setProjectFollow(project.slug, !wasFollowed)
      showToast(wasFollowed ? '已取消关注' : '已关注，有更新时会通知你')
    } catch {
      setFollowed((current) => wasFollowed ? [...current, projectId] : current.filter((id) => id !== projectId))
      showToast('关注状态更新失败，请稍后重试')
    }
  }

  const handleSelection = () => {
    const browserSelection = window.getSelection()
    const selection = browserSelection?.toString().trim()
    if (selection && browserSelection) {
      setSelectedQuote(selection)
      const anchorElement = browserSelection.anchorNode instanceof Element
        ? browserSelection.anchorNode
        : browserSelection.anchorNode?.parentElement
      const stableBlockID = anchorElement?.closest<HTMLElement>('[data-block-id]')?.dataset.blockId
      const matchedBlock = activeDocument?.blocks.find((item) => item.text.includes(selection) || selection.includes(item.text))
      if (stableBlockID) setSelectedBlockID(stableBlockID)
      else if (matchedBlock) setSelectedBlockID(matchedBlock.id)
      // 记录选区底部相对文章容器的偏移，让评论框紧贴选中内容下方出现。
      const article = anchorElement?.closest<HTMLElement>('.document-article')
      const range = browserSelection.rangeCount > 0 ? browserSelection.getRangeAt(0) : null
      if (article && range) {
        const selectionRect = range.getBoundingClientRect()
        const articleRect = article.getBoundingClientRect()
        if (selectionRect.height > 0 || selectionRect.width > 0) {
          setComposerAnchor({ top: selectionRect.bottom - articleRect.top + article.scrollTop + 10 })
        } else {
          setComposerAnchor(null)
        }
      } else {
        setComposerAnchor(null)
      }
      setCommentComposerOpen(true)
    }
  }

  const submitComment = async () => {
    if (!draftComment.trim() || !selectedProject || !activeDocument) return
    if (!currentUser) {
      setLoginOpen(true)
      showToast('登录后才能发表评论')
      return
    }
    setCommentSubmitting(true)
    try {
      const response = await createDocumentComment(selectedProject.slug, activeDocument.slug, {
        block_id: selectedBlockID,
        quote: selectedQuote,
        body: draftComment.trim(),
      })
      const created = mapDocumentComment(response.data)
      setComments((current) => [created, ...current])
      setDraftComment('')
      setCommentComposerOpen(false)
      showToast('评论已发布，已同步到当前文档')
    } catch {
      showToast('评论发布失败，请稍后重试')
    } finally {
      setCommentSubmitting(false)
    }
  }

  // toggleCommentLike 点赞/取消点赞评论或回复。乐观更新，失败回滚。
  const adjustCommentLike = (list: CommentItem[], commentId: string, liked: boolean, delta: number): CommentItem[] =>
    list.map((comment) => {
      if (comment.id === commentId) {
        return { ...comment, liked, likeCount: Math.max(0, comment.likeCount + delta) }
      }
      if (comment.replies.length) {
        return { ...comment, replies: adjustCommentLike(comment.replies, commentId, liked, delta) }
      }
      return comment
    })

  const toggleCommentLike = async (commentId: string, currentlyLiked: boolean) => {
    if (!currentUser) {
      setLoginOpen(true)
      showToast('登录后才能点赞')
      return
    }
    if (!selectedProject || !activeDocument) return
    const like = !currentlyLiked
    setComments((current) => adjustCommentLike(current, commentId, like, like ? 1 : -1))
    try {
      await setCommentLike(selectedProject.slug, activeDocument.slug, commentId, like)
    } catch {
      // 回滚乐观更新。
      setComments((current) => adjustCommentLike(current, commentId, currentlyLiked, like ? -1 : 1))
      showToast('点赞失败，请稍后重试')
    }
  }

  const resolveComment = async (commentId: string) => {
    if (!selectedProject || !activeDocument) return
    if (!currentUser) {
      setLoginOpen(true)
      return
    }
    setResolvingCommentID(commentId)
    try {
      const response = await resolveDocumentComment(selectedProject.slug, activeDocument.slug, commentId)
      // 单条响应不携带回复，合并时保留本地已有回复，避免解决后回复消失。
      const resolved = mapDocumentComment(response.data)
      setComments((current) => current.map((comment) => (
        comment.id === commentId
          ? { ...resolved, replies: comment.replies, replyCount: comment.replyCount }
          : comment
      )))
      showToast('评论已标记为已解决')
    } catch {
      showToast('评论状态更新失败，请稍后重试')
    } finally {
      setResolvingCommentID(null)
    }
  }

  const replyToComment = async (commentId: string, body: string) => {
    if (!selectedProject || !activeDocument || !body.trim()) return false
    if (!currentUser) {
      setLoginOpen(true)
      showToast('登录后才能回复')
      return false
    }
    try {
      const response = await createDocumentCommentReply(
        selectedProject.slug,
        activeDocument.slug,
        commentId,
        { body: body.trim() },
      )
      const reply = mapDocumentComment(response.data)
      setComments((current) => current.map((comment) => (
        comment.id === commentId
          ? { ...comment, replies: [...comment.replies, reply], replyCount: comment.replyCount + 1 }
          : comment
      )))
      showToast('回复已发布')
      return true
    } catch {
      showToast('回复发布失败，请稍后重试')
      return false
    }
  }

  const deleteReply = async (commentId: string, replyId: string) => {
    if (!selectedProject || !activeDocument) return false
    if (!currentUser) {
      setLoginOpen(true)
      return false
    }
    try {
      await deleteDocumentCommentReply(selectedProject.slug, activeDocument.slug, commentId, replyId)
      setComments((current) => current.map((comment) => (
        comment.id === commentId
          ? {
              ...comment,
              replies: comment.replies.filter((reply) => reply.id !== replyId),
              replyCount: Math.max(0, comment.replyCount - 1),
            }
          : comment
      )))
      showToast('回复已删除')
      return true
    } catch {
      showToast('回复删除失败，请稍后重试')
      return false
    }
  }

  const deleteComment = async (commentId: string) => {
    if (!selectedProject || !activeDocument || !currentUser) return false
    setDeletingCommentID(commentId)
    try {
      await deleteDocumentComment(selectedProject.slug, activeDocument.slug, commentId)
      setComments((current) => current.filter((comment) => comment.id !== commentId))
      showToast('评论线程已删除')
      return true
    } catch {
      showToast('评论删除失败，请稍后重试')
      return false
    } finally {
      setDeletingCommentID(null)
    }
  }

  const editComment = async (commentId: string, body: string) => {
    if (!selectedProject || !activeDocument || !body.trim()) return false
    if (!currentUser) {
      setLoginOpen(true)
      return false
    }
    try {
      const response = await updateDocumentComment(
        selectedProject.slug, activeDocument.slug, commentId, body.trim(),
      )
      // 编辑响应同样不携带回复，需保留本地回复列表。
      const updated = mapDocumentComment(response.data)
      setComments((current) => current.map((comment) => (
        comment.id === commentId
          ? { ...updated, replies: comment.replies, replyCount: comment.replyCount }
          : comment
      )))
      showToast('评论已更新')
      return true
    } catch {
      showToast('评论更新失败，请稍后重试')
      return false
    }
  }

  const editReply = async (commentId: string, replyId: string, body: string) => {
    if (!selectedProject || !activeDocument || !body.trim()) return false
    if (!currentUser) {
      setLoginOpen(true)
      return false
    }
    try {
      const response = await updateDocumentCommentReply(
        selectedProject.slug, activeDocument.slug, commentId, replyId, body.trim(),
      )
      const updatedReply = mapDocumentComment(response.data)
      setComments((current) => current.map((comment) => (
        comment.id === commentId
          ? {
              ...comment,
              replies: comment.replies.map((reply) => reply.id === replyId ? updatedReply : reply),
            }
          : comment
      )))
      showToast('回复已更新')
      return true
    } catch {
      showToast('回复更新失败，请稍后重试')
      return false
    }
  }

  const performLogout = async (allDevices: boolean) => {
    if (logoutSubmitting) return
    setLogoutSubmitting(true)
    try {
      if (allDevices) await logoutAll()
      else await logout()
      setCurrentUser(null)
      setAuthSessions([])
      setSaved([])
      setFollowed([])
      closeHeaderPopover()
      showToast(allDevices ? '所有设备已退出登录' : '已退出当前设备')
    } catch {
      showToast('退出失败，请稍后重试')
    } finally {
      setLogoutSubmitting(false)
    }
  }

  const revokeSession = async (session: AuthSession) => {
    if (revokingSessionID) return
    setRevokingSessionID(session.id)
    try {
      await revokeAuthSession(session.id)
      if (session.current) {
        setCurrentUser(null)
        setAuthSessions([])
        closeHeaderPopover()
        showToast('已退出当前设备')
      } else {
        setAuthSessions((current) => current.filter((item) => item.id !== session.id))
        showToast('指定设备已退出登录')
      }
    } catch {
      showToast('会话撤销失败，请稍后重试')
    } finally {
      setRevokingSessionID(null)
    }
  }

  const submitPasswordChange = async () => {
    if (passwordSubmitting || !currentPassword || newPassword.length < 8) return
    setPasswordSubmitting(true)
    try {
      await changePassword({ current_password: currentPassword, new_password: newPassword })
      setCurrentPassword('')
      setNewPassword('')
      setPasswordEditing(false)
      setAuthSessions((current) => current.filter((session) => session.current))
      showToast('密码已修改，其他设备已退出登录')
    } catch {
      showToast('密码修改失败，请检查当前密码和新密码')
    } finally {
      setPasswordSubmitting(false)
    }
  }

  const submitProfileUpdate = async () => {
    const displayName = profileDisplayName.trim()
    if (profileSubmitting || displayName.length < 2) return
    setProfileSubmitting(true)
    try {
      const response = await updateCurrentUser({ display_name: displayName })
      setCurrentUser(response.data)
      setProfileEditing(false)
      showToast('昵称已更新')
    } catch {
      showToast('昵称更新失败，请稍后重试')
    } finally {
      setProfileSubmitting(false)
    }
  }

  return (
    <div className="app-shell" data-theme-mode={themeMode} data-skin={skin}>
      <header className="site-header" ref={headerRef}>
        <button className="brand" onClick={() => { closeProject(); closeHeaderPopover() }} aria-label="返回首页">
          <BrandMark className="brand-mark-svg" size={32} title="新猿译码" />
          <span>
            <strong>新猿译码</strong>
            <small>AGENT OPEN SOURCE HUB</small>
          </span>
        </button>

        <nav className={`main-nav ${mobileMenuOpen ? 'is-open' : ''}`}>
          {['探索', '趋势', '最新更新', '社区'].map((item) => (
            <button key={item} className={activeTab === item ? 'active' : ''} onClick={() => { setActiveTab(item); closeProject(); closeHeaderPopover() }}>
              {item}
            </button>
          ))}
        </nav>

        <div className="header-actions">
          <button
            type="button"
            className="global-search"
            title="搜索项目与文档"
            onClick={() => { closeHeaderPopover(); setSearchPanelOpen(true) }}
          >
            <Search size={16} />
            <span className="global-search-placeholder">搜索项目、文档或技术栈</span>
            <kbd>{shortcutModifierLabel} K</kbd>
          </button>
          <div className="theme-control">
            <button className="icon-button quiet" title="切换主题" aria-label="切换主题" aria-expanded={themePanelOpen} onClick={() => toggleHeaderPopover('theme')}>
              {themeMode === 'dark' ? <Moon size={18} /> : themeMode === 'light' ? <Sun size={18} /> : <MonitorCog size={18} />}
            </button>
            {themePanelOpen && <ThemePanel themeMode={themeMode} skin={skin} onModeChange={(mode) => { setThemeMode(mode); closeHeaderPopover() }} onSkinChange={setSkin} />}
          </div>
          <div className="notification-control">
            <button
              className="icon-button quiet notification-button"
              title="通知"
              aria-label={unreadNotificationCount > 0 ? `通知，${unreadNotificationCount} 条未读` : '通知'}
              aria-expanded={currentUser ? notificationPanelOpen : undefined}
              onClick={() => {
                if (!currentUser) {
                  setLoginOpen(true)
                  showToast('登录后可查看通知')
                  return
                }
                toggleHeaderPopover('notification')
              }}
            >
              <Bell size={18} />
              {unreadNotificationCount > 0 && (
                <span className="notification-badge">{unreadNotificationCount > 99 ? '99+' : unreadNotificationCount}</span>
              )}
            </button>
            {currentUser && notificationPanelOpen && (
              <div className="notification-popover" aria-label="站内通知">
                <div className="notification-heading">
                  <strong>通知</strong>
                  <button
                    disabled={markingNotifications || unreadNotificationCount === 0}
                    onClick={() => void markAllRead()}
                  >
                    {markingNotifications ? '处理中…' : '全部已读'}
                  </button>
                </div>
                {notificationsLoading ? (
                  <div className="notification-empty">正在加载通知…</div>
                ) : notifications.length === 0 ? (
                  <div className="notification-empty">暂时没有通知</div>
                ) : (
                  <div className="notification-list">
                    {notifications.map((entry) => (
                      <button
                        key={entry.id}
                        className={`notification-row ${entry.read_at ? '' : 'is-unread'}`}
                        onClick={() => void openNotification(entry)}
                      >
                        <span className="notification-row-title">{entry.title}</span>
                        {entry.body && <span className="notification-row-body">{entry.body}</span>}
                        <span className="notification-row-time">{new Date(entry.created_at).toLocaleString('zh-CN')}</span>
                      </button>
                    ))}
                  </div>
                )}
              </div>
            )}
          </div>
          {currentUser?.is_admin && (
            <button
              className="admin-entry-button"
              title="打开管理控制台"
              aria-label="打开管理控制台"
              onClick={openAdminConsole}
            >
              <ShieldCheck size={16} /> <span>管理台</span>
            </button>
          )}
          <div className="account-control">
            <button
              className="login-button"
              title={currentUser ? '打开账号菜单' : '登录或注册'}
              aria-expanded={currentUser ? accountPanelOpen : undefined}
              onClick={() => currentUser ? toggleHeaderPopover('account') : setLoginOpen(true)}
            >
              <CircleUserRound size={16} /> <span>{currentUser?.display_name ?? '登录'}</span>
            </button>
            {currentUser && accountPanelOpen && (
              <div className="account-popover">
                <div className="account-summary">
                  <button
                    type="button"
                    className="account-avatar-edit"
                    title="更换头像与头像框"
                    aria-label="更换头像与头像框"
                    onClick={() => { setAvatarFramePickerOpen(true); closeHeaderPopover() }}
                  >
                    <LevelAvatar level={currentUser.level} initials={currentUser.display_name.slice(0, 1)} size="lg" name={currentUser.display_name} avatar={currentUser.avatar} frame={currentUser.avatar_frame} />
                    <span><Pencil size={11} /> 更换</span>
                  </button>
                  <div className="account-summary-text">
                    <strong className={currentUser.level >= 6 ? 'nickname-legendary' : ''}>{currentUser.display_name}</strong>
                    <LevelBadge level={currentUser.level} />
                    <small>{currentUser.is_admin ? '管理员 · 满级' : `经验 ${currentUser.experience}`} · {sessionsLoading ? '加载登录设备…' : `${authSessions.length} 个活跃会话`}</small>
                  </div>
                </div>
                {profileEditing ? (
                  <div className="profile-editor">
                    <input
                      autoFocus
                      maxLength={80}
                      value={profileDisplayName}
                      onChange={(event) => setProfileDisplayName(event.target.value)}
                      onKeyDown={(event) => { if (event.key === 'Enter') void submitProfileUpdate() }}
                      placeholder="昵称"
                    />
                    <div>
                      <button disabled={profileSubmitting || profileDisplayName.trim().length < 2} onClick={() => void submitProfileUpdate()}>
                        {profileSubmitting ? '保存中…' : '保存昵称'}
                      </button>
                      <button disabled={profileSubmitting} onClick={() => setProfileEditing(false)}>取消</button>
                    </div>
                  </div>
                ) : (
                  <button onClick={() => { setProfileDisplayName(currentUser.display_name); setProfileEditing(true) }}>编辑昵称</button>
                )}
                <div className="session-list" aria-label="登录设备">
                  {authSessions.map((session) => (
                    <div className="session-row" key={session.id}>
                      <span>
                        <strong>{session.current ? '当前设备' : '其他设备'}</strong>
                        <small>{new Date(session.created_at).toLocaleString('zh-CN')}</small>
                      </span>
                      <button
                        disabled={revokingSessionID !== null || logoutSubmitting}
                        onClick={() => void revokeSession(session)}
                      >
                        {revokingSessionID === session.id ? '处理中…' : '退出'}
                      </button>
                    </div>
                  ))}
                </div>
                {currentUser.has_password ? (
                  passwordEditing ? (
                    <div className="password-editor">
                      <input
                        type="password"
                        autoComplete="current-password"
                        placeholder="当前密码"
                        value={currentPassword}
                        onChange={(event) => setCurrentPassword(event.target.value)}
                      />
                      <input
                        type="password"
                        autoComplete="new-password"
                        placeholder="新密码（至少 8 位）"
                        value={newPassword}
                        onChange={(event) => setNewPassword(event.target.value)}
                      />
                      <div>
                        <button disabled={passwordSubmitting || !currentPassword || newPassword.length < 8} onClick={() => void submitPasswordChange()}>
                          {passwordSubmitting ? '修改中…' : '确认修改'}
                        </button>
                        <button disabled={passwordSubmitting} onClick={() => setPasswordEditing(false)}>取消</button>
                      </div>
                    </div>
                  ) : (
                    <button onClick={() => setPasswordEditing(true)}>修改密码</button>
                  )
                ) : (
                  <small className="oauth-account-note">GitHub 登录账号无需站内密码</small>
                )}
                <button onClick={() => { setAvatarFramePickerOpen(true); closeHeaderPopover() }}>
                  更换头像与头像框
                </button>
                <button onClick={() => { setActiveTab('我的收藏'); closeProject(); closeHeaderPopover() }}>
                  查看我的收藏
                </button>
                <button onClick={() => { setActiveTab('我的关注'); closeProject(); closeHeaderPopover() }}>
                  查看我的关注
                </button>
                <button onClick={() => { setAuthorCenterOpen(true); closeHeaderPopover() }}>
                  作者项目中心
                </button>
                <button onClick={() => { setAccessKeyOpen(true); closeHeaderPopover() }}>
                  AccessKey 管理
                </button>
                {currentUser.is_admin && <button onClick={openAdminConsole}>
                  管理控制台
                </button>}
                <button disabled={logoutSubmitting} onClick={() => void performLogout(false)}>退出当前设备</button>
                <button disabled={logoutSubmitting} onClick={() => void performLogout(true)}>{logoutSubmitting ? '处理中…' : '退出所有设备'}</button>
              </div>
            )}
          </div>
          <button className="icon-button mobile-only" title="打开菜单" aria-label="打开菜单" onClick={() => toggleHeaderPopover('mobileMenu')}><Menu size={19} /></button>
        </div>
      </header>

      {selectedProject ? (
        <ProjectDetail
          project={selectedProject}
          detailTab={detailTab}
          setDetailTab={setDetailTab}
          isSaved={saved.includes(selectedProject.id)}
          isFollowed={followed.includes(selectedProject.id)}
          onBack={closeProject}
          onOpenProfile={openProfile}
          onToggleSaved={() => toggleSaved(selectedProject.id)}
          onToggleFollowed={() => toggleFollowed(selectedProject.id)}
          onShare={() => {
            navigator.clipboard?.writeText(window.location.href)
            showToast('项目链接已复制')
            // 分享加经验，best-effort：失败不影响复制链接。仅登录用户计入。
            if (currentUser) void shareProject(selectedProject.slug).catch(() => {})
          }}
          onReport={() => openReport({ type: 'project', id: selectedProject.slug, label: selectedProject.name })}
          onReportComment={(commentId) => openReport({ type: 'comment', id: commentId, label: '文档评论' })}
          comments={comments}
          selectedQuote={selectedQuote}
          composerAnchor={composerAnchor}
          commentComposerOpen={commentComposerOpen}
          setCommentComposerOpen={setCommentComposerOpen}
          draftComment={draftComment}
          setDraftComment={setDraftComment}
          onSelection={handleSelection}
          onSubmitComment={submitComment}
          onResolveComment={resolveComment}
          onLikeComment={toggleCommentLike}
          onReplyComment={replyToComment}
          onDeleteReply={deleteReply}
          onDeleteComment={deleteComment}
          onEditComment={editComment}
          onEditReply={editReply}
          showToast={showToast}
          detailState={detailState}
          documentState={documentState}
          documentTree={documentTree}
          activeDocument={activeDocument}
          onOpenDocument={openDocument}
          commentsState={commentsState}
          commentSubmitting={commentSubmitting}
          resolvingCommentID={resolvingCommentID}
          deletingCommentID={deletingCommentID}
          currentUserID={currentUser?.id ?? ''}
          currentUser={currentUser}
          onPublishedDocumentSaved={updatePublishedDocument}
        />
      ) : (
        <Home
          activeTab={activeTab}
          activeCategory={activeCategory}
          setActiveCategory={setActiveCategory}
          filteredProjects={filteredProjects}
          saved={saved}
          onOpenProject={openProject}
          onToggleSaved={toggleSaved}
          gatewayState={gatewayState}
          catalogState={catalogState}
          onOpenDocs={() => setOpenDocsOpen(true)}
          onOpenProfile={(userId) => setProfileUserId(userId)}
          onOpenProjectSlug={(slug) => openProjectBySlug(slug)}
          onSubmitProject={() => {
            if (currentUser) {
              setAuthorCenterOpen(true)
            } else {
              setLoginOpen(true)
            }
          }}
        />
      )}

      {openDocsOpen && <Suspense fallback={null}><OpenApiDocs onClose={() => setOpenDocsOpen(false)} /></Suspense>}

      {profileUserId && <Suspense fallback={null}><ErrorBoundary label="用户主页"><UserProfile
        userId={profileUserId}
        onClose={() => setProfileUserId(null)}
        onOpenProject={(slug) => { setProfileUserId(null); openProjectBySlug(slug) }}
      /></ErrorBoundary></Suspense>}

      {toast && <div className="toast"><Check size={16} /> {toast}</div>}
      {loginOpen && <LoginModal
        onClose={() => setLoginOpen(false)}
        onAuthenticated={(user) => {
          setCurrentUser(user)
          setLoginOpen(false)
          showToast(`欢迎，${user.display_name}`)
        }}
      />}
      {searchPanelOpen && <Suspense fallback={null}>
        <DocumentSearchPanel
          onOpenResult={openSearchResult}
          onOpenProject={(projectSlug) => openProjectBySlug(projectSlug)}
          onClose={() => setSearchPanelOpen(false)}
        />
      </Suspense>}
      {authorCenterOpen && currentUser && <ErrorBoundary label="作者项目中心"><AuthorProjectCenter
        onClose={() => setAuthorCenterOpen(false)}
        onChanged={() => {
          showToast('项目状态已更新')
        }}
      /></ErrorBoundary>}
      {adminConsoleOpen && currentUser?.is_admin && <Suspense fallback={null}><ErrorBoundary label="管理控制台"><AdminConsole
        onClose={closeAdminConsole}
        currentUser={currentUser}
      /></ErrorBoundary></Suspense>}
      {accessKeyOpen && currentUser && <Suspense fallback={null}><ErrorBoundary label="AccessKey 管理"><AccessKeyManager
        onClose={() => setAccessKeyOpen(false)}
      /></ErrorBoundary></Suspense>}
      {avatarFramePickerOpen && currentUser && <Suspense fallback={null}><ErrorBoundary label="头像与头像框"><AvatarFramePicker
        currentUser={currentUser}
        onClose={() => setAvatarFramePickerOpen(false)}
        onChanged={(user) => setCurrentUser(user)}
      /></ErrorBoundary></Suspense>}
      {reportTarget && <Suspense fallback={null}><ReportDialog
        target={reportTarget}
        onClose={() => setReportTarget(null)}
        onSubmitted={(message) => { setReportTarget(null); showToast(message) }}
      /></Suspense>}
      <Suspense fallback={null}><AiAssistant
        projectSlug={selectedProject?.slug ?? null}
        projectName={selectedProject?.name ?? null}
        currentUser={currentUser}
        onRequestLogin={() => setLoginOpen(true)}
      /></Suspense>
    </div>
  )
}

function Home({
  activeTab,
  activeCategory,
  setActiveCategory,
  filteredProjects,
  saved,
  onOpenProject,
  onToggleSaved,
  gatewayState,
  catalogState,
  onOpenDocs,
  onOpenProfile,
  onOpenProjectSlug,
  onSubmitProject,
}: {
  activeTab: string
  activeCategory: string
  setActiveCategory: (category: string) => void
  filteredProjects: Project[]
  saved: string[]
  onOpenProject: (project: Project) => void
  onToggleSaved: (projectId: string) => void
  gatewayState: GatewayState
  catalogState: CatalogState
  onOpenDocs: () => void
  onOpenProfile: (userId: string) => void
  onOpenProjectSlug: (slug: string) => void
  onSubmitProject: () => void
}) {
  const [stats, setStats] = useState<SiteStats | null>(null)
  const [statsState, setStatsState] = useState<'loading' | 'ready' | 'offline'>('loading')
  const [hotTags, setHotTags] = useState<TagCount[]>([])
  const [tagFilter, setTagFilter] = useState<string | null>(null)
  const [activity, setActivity] = useState<ActivityItem[]>([])
  const [leaderboard, setLeaderboard] = useState<LeaderboardUser[]>([])
  const [leaderboardState, setLeaderboardState] = useState<'loading' | 'ready' | 'offline'>('loading')
  const [featuredSlug, setFeaturedSlug] = useState('')
  const [filterOpen, setFilterOpen] = useState(false)
  const [stackFilter, setStackFilter] = useState<string | null>(null)
  const [licenseFilter, setLicenseFilter] = useState<string | null>(null)
  const filterRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    const controller = new AbortController()
    getSiteStats(controller.signal)
      .then((response) => {
        setStats(response.data)
        setStatsState('ready')
      })
      .catch(() => setStatsState('offline'))
    return () => controller.abort()
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    getFeaturedProjects(controller.signal)
      .then((response) => setFeaturedSlug(response.data[0]?.slug ?? ''))
      .catch(() => setFeaturedSlug(''))
    return () => controller.abort()
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    getHotTags(controller.signal)
      .then((response) => setHotTags(response.data))
      .catch(() => setHotTags([]))
    return () => controller.abort()
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    getActivity(controller.signal)
      .then((response) => setActivity(response.data))
      .catch(() => setActivity([]))
    return () => controller.abort()
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    getLeaderboard(8, controller.signal)
      .then((response) => {
        setLeaderboard(response.data)
        setLeaderboardState('ready')
      })
      .catch(() => setLeaderboardState('offline'))
    return () => controller.abort()
  }, [])

  // 更多筛选弹层：点外部关闭。
  useEffect(() => {
    if (!filterOpen) return
    const handlePointerDown = (event: PointerEvent) => {
      if (event.target instanceof Node && filterRef.current?.contains(event.target)) return
      setFilterOpen(false)
    }
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setFilterOpen(false)
    }
    document.addEventListener('pointerdown', handlePointerDown, true)
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('pointerdown', handlePointerDown, true)
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [filterOpen])

  const availableStacks = useMemo(() => {
    const stacks = new Set<string>()
    for (const project of filteredProjects) {
      for (const item of project.stack) stacks.add(item)
    }
    return [...stacks].sort((left, right) => left.localeCompare(right))
  }, [filteredProjects])

  const availableLicenses = useMemo(() => {
    const licenses = new Set<string>()
    for (const project of filteredProjects) {
      if (project.license) licenses.add(project.license)
    }
    return [...licenses].sort((left, right) => left.localeCompare(right))
  }, [filteredProjects])

  const availableTags = useMemo(() => {
    const counts = new Map<string, number>()
    for (const project of filteredProjects) {
      for (const tag of project.tags) {
        counts.set(tag, (counts.get(tag) ?? 0) + 1)
      }
    }
    return [...counts.entries()]
      .map(([name, count]) => ({ name, count }))
      .sort((left, right) => right.count - left.count || left.name.localeCompare(right.name))
  }, [filteredProjects])

  const visibleProjects = useMemo(
    () => filteredProjects.filter((project) =>
      (!stackFilter || project.stack.includes(stackFilter)) &&
      (!licenseFilter || project.license === licenseFilter) &&
      (!tagFilter || project.tags.includes(tagFilter))),
    [filteredProjects, stackFilter, licenseFilter, tagFilter],
  )

  // 顶栏标签的真实语义：趋势按浏览量、最新更新按时间、社区按讨论数。
  const sortedProjects = useMemo(() => {
    const sorted = [...visibleProjects]
    if (activeTab === '趋势') {
      sorted.sort((left, right) => (right.viewsValue ?? 0) - (left.viewsValue ?? 0))
    } else if (activeTab === '最新更新') {
      sorted.sort((left, right) => (right.updatedAt ?? '').localeCompare(left.updatedAt ?? ''))
    } else if (activeTab === '社区') {
      sorted.sort((left, right) => right.comments - left.comments)
    }
    return sorted
  }, [visibleProjects, activeTab])

  const hasExtraFilter = stackFilter !== null || licenseFilter !== null || tagFilter !== null
  const resetExtraFilters = () => {
    setStackFilter(null)
    setLicenseFilter(null)
    setTagFilter(null)
  }

  // 本周精选：优先展示管理员配置的推荐项目；未配置时取最近更新的项目。
  const curated = useMemo(() => {
    if (filteredProjects.length === 0) return null
    if (featuredSlug) {
      const featured = filteredProjects.find((project) => project.slug === featuredSlug)
      if (featured) return featured
    }
    return [...filteredProjects].sort((left, right) => (left.updated ?? '').localeCompare(right.updated ?? ''))[
      filteredProjects.length - 1
    ] ?? filteredProjects[0]
  }, [filteredProjects, featuredSlug])

  const formatCount = (value: number) => new Intl.NumberFormat('zh-CN', { notation: 'compact', maximumFractionDigits: 1 }).format(value)
  const formatInteger = (value: number) => new Intl.NumberFormat('zh-CN').format(value)
  const formatRelativeTime = (value: string) => {
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return ''
    const minutes = Math.floor((Date.now() - date.getTime()) / 60000)
    if (minutes < 1) return '刚刚'
    if (minutes < 60) return `${minutes} 分钟前`
    const hours = Math.floor(minutes / 60)
    if (hours < 24) return `${hours} 小时前`
    const days = Math.floor(hours / 24)
    if (days < 30) return `${days} 天前`
    return date.toLocaleDateString('zh-CN')
  }

  return (
    <main>
      <section className="hero-band">
        <div className="hero-copy">
          <span className="eyebrow"><span className="status-dot" /> 为 Agent 开发者整理的开源项目空间</span>
          <h1>找到下一次<br /><em>有用的连接。</em></h1>
          <p>浏览真实可用的 Agent 项目，阅读实现文档，查看关键代码，并把好的想法带回你的工作流。</p>
          <div className="hero-actions">
            <button className="primary-button" onClick={() => document.getElementById('project-grid')?.scrollIntoView({ behavior: 'smooth' })}>开始探索 <ArrowUpRight size={16} /></button>
            <button className="text-button" onClick={onSubmitProject}>提交开源项目 <Upload size={16} /></button>
          </div>
        </div>
        <div className="hero-visual hero-media-card" aria-label="AI 开源项目动态插画">
          <video
            className="hero-media"
            poster="/hero-ai-poster.webp"
            autoPlay
            muted
            loop
            playsInline
            preload="metadata"
            aria-hidden="true"
            onCanPlay={(event) => { void event.currentTarget.play().catch(() => undefined) }}
          >
            <source src="/hero-ai.webm" type="video/webm" />
          </video>
          <div className="hero-media-caption"><span>AI · OPEN SOURCE</span><span>持续生长的开发者社区</span></div>
        </div>
      </section>

      <section className="metrics-strip">
        <div><strong>{statsState === 'ready' && stats ? formatInteger(stats.projects) : '-'}</strong><span>公开项目</span></div>
        <div><strong>{statsState === 'ready' && stats ? formatInteger(stats.updated_today) : '-'}</strong><span>今日更新</span></div>
        <div><strong>{statsState === 'ready' && stats ? formatCount(stats.downloads) : '-'}</strong><span>累计下载</span></div>
        <div><strong>{statsState === 'ready' && stats ? formatInteger(stats.documents) : '-'}</strong><span>在线文档</span></div>
        <div className="metrics-note"><GitBranch size={17} /><span>中文项目优先，欢迎分享你的 Agent 实验。</span></div>
      </section>

      <section className="content-section" id="project-grid">
        <div className="section-heading">
          <div><span className="section-kicker">PROJECT INDEX</span><h2>{activeTab === '探索' ? '探索项目' : activeTab}</h2></div>
          <button className="outline-button" onClick={() => { setActiveCategory('全部项目'); resetExtraFilters(); document.getElementById('project-grid')?.scrollIntoView({ behavior: 'smooth' }) }}>查看全部 <ChevronRight size={15} /></button>
        </div>
        <div className="filter-row">
          {categories.map((category) => <button key={category} className={`filter-chip ${activeCategory === category ? 'active' : ''}`} onClick={() => setActiveCategory(category)}>{category}</button>)}
          <div className="filter-more-wrap" ref={filterRef}>
            <button className={`filter-more ${hasExtraFilter ? 'has-filter' : ''}`} aria-expanded={filterOpen} onClick={() => setFilterOpen((open) => !open)}><Tag size={14} /> {hasExtraFilter ? '已筛选' : '更多筛选'} <ChevronDown size={14} /></button>
            {filterOpen && (
              <div className="filter-popover" role="menu" aria-label="更多筛选">
                <div className="filter-popover-group">
                  <span className="filter-popover-label">技术栈</span>
                  <div className="filter-option-list">
                    {availableStacks.length === 0 && <span className="filter-popover-empty">暂无可选技术栈</span>}
                    {availableStacks.map((stack) => (
                      <button key={stack} type="button" className={stackFilter === stack ? 'active' : ''} onClick={() => setStackFilter(stackFilter === stack ? null : stack)}>{stack}</button>
                    ))}
                  </div>
                </div>
                <div className="filter-popover-group">
                  <span className="filter-popover-label">许可证</span>
                  <div className="filter-option-list">
                    {availableLicenses.length === 0 && <span className="filter-popover-empty">暂无可选许可证</span>}
                    {availableLicenses.map((license) => (
                      <button key={license} type="button" className={licenseFilter === license ? 'active' : ''} onClick={() => setLicenseFilter(licenseFilter === license ? null : license)}>{license}</button>
                    ))}
                  </div>
                </div>
                <div className="filter-popover-group">
                  <span className="filter-popover-label">标签</span>
                  <div className="filter-option-list">
                    {availableTags.length === 0 && <span className="filter-popover-empty">暂无可选标签</span>}
                    {availableTags.map((tag) => (
                      <button key={tag.name} type="button" className={tagFilter === tag.name ? 'active' : ''} onClick={() => setTagFilter(tagFilter === tag.name ? null : tag.name)}>{tag.name} <small>{tag.count}</small></button>
                    ))}
                  </div>
                </div>
                {hasExtraFilter && <button type="button" className="filter-popover-reset" onClick={resetExtraFilters}>清除筛选</button>}
              </div>
            )}
          </div>
        </div>
        {hotTags.length > 0 && (
          <div className="hot-tags-row" aria-label="热门标签">
            {hotTags.slice(0, 12).map((tag) => (
              <button key={tag.name} type="button" className={tagFilter === tag.name ? 'active' : ''} onClick={() => setTagFilter(tagFilter === tag.name ? null : tag.name)}>
                {tag.name} <small>{tag.count}</small>
              </button>
            ))}
          </div>
        )}

        {catalogState !== 'online' && (
          <div className={`catalog-notice ${catalogState}`}>
            <span className="gateway-state-dot" />
            {catalogState === 'checking' ? '正在同步项目目录…' : '当前展示演示数据，项目 API 暂时不可用。'}
          </div>
        )}

        {sortedProjects.length ? (
          <div className="project-grid">
            {sortedProjects.map((project) => <ProjectCard key={project.id} project={project} isSaved={saved.includes(project.id)} onOpen={() => onOpenProject(project)} onToggleSaved={() => onToggleSaved(project.id)} />)}
          </div>
        ) : <div className="empty-state"><Search size={24} /><h3>没有找到匹配项目</h3><p>{hasExtraFilter ? '试试调整技术栈或许可证筛选条件。' : '试试其他关键词或清空筛选条件。'}</p></div>}
      </section>

      <section className="editorial-band">
        <div><span className="section-kicker">FROM THE COMMUNITY</span><h2>好的项目，值得被读懂。</h2><p>项目不只是一个仓库地址。我们希望让每个 Agent 的背景、设计选择和使用方式都能被清楚地留下来。</p></div>
        <div className="editorial-quote"><MessageSquare size={20} /><p>“把复杂的 Agent 工程，整理成可以被下一位开发者接住的知识。”</p><span>新猿译码编辑部</span></div>
      </section>

      <section className="content-section compact-section">
        {curated && (
          <>
            <div className="section-heading"><div><span className="section-kicker">CURATED NOTE</span><h2>本周精选</h2></div><button className="text-button" onClick={() => onOpenProject(curated)}>阅读项目手记 <ArrowUpRight size={15} /></button></div>
            <div className="curated-row">
              <div className="curated-cover cover-blue"><div className="cover-label">FEATURED PROJECT</div><span>{curated.name}</span></div>
              <div className="curated-copy"><span className="meta-label">{curated.category} · 最近更新于 {curated.updated}</span><h3>{curated.summary}</h3><p>{curated.description}</p><button className="text-button" onClick={() => onOpenProject(curated)}>打开项目 <ArrowUpRight size={15} /></button></div>
            </div>
          </>
        )}
      </section>
      {leaderboardState === 'ready' && leaderboard.length > 0 && (
        <section className="content-section compact-section">
          <div className="section-heading">
            <div><span className="section-kicker">CONTRIBUTORS</span><h2>开发者榜单</h2></div>
            <span className="meta-label">按社区经验值排序</span>
          </div>
          <div className="leaderboard-list">
            {leaderboard.map((user, index) => (
              <button type="button" className="leaderboard-row" key={user.id} onClick={() => onOpenProfile(user.id)}>
                <span className="leaderboard-rank">#{index + 1}</span>
                <LevelAvatar level={user.level} initials={user.display_name.slice(0, 1)} size="sm" name={user.display_name} avatar={user.avatar} frame={user.avatar_frame} />
                <span className="leaderboard-name">{user.display_name}</span>
                <LevelBadge level={user.level} />
                <span className="leaderboard-exp">{user.experience} 经验</span>
              </button>
            ))}
          </div>
        </section>
      )}
      {activity.length > 0 && (
        <section className="content-section compact-section">
          <div className="section-heading">
            <div><span className="section-kicker">COMMUNITY</span><h2>社区动态</h2></div>
          </div>
          <div className="activity-list">
            {activity.map((item, index) => (
              <button key={`${item.type}-${item.project_slug}-${index}`} type="button" className="activity-row" onClick={() => { if (item.project_slug) onOpenProjectSlug(item.project_slug) }}>
                <span className={`activity-icon is-${item.type}`}>
                  {item.type === 'project_published' ? <Rocket size={14} /> : item.type === 'project_updated' ? <RefreshCw size={14} /> : <MessageSquare size={14} />}
                </span>
                <span className="activity-text">
                  <strong>{item.type === 'comment' ? `${item.title} 有新评论` : item.type === 'project_published' ? `项目发布：${item.title}` : `项目更新：${item.title}`}</strong>
                  {item.summary && <small>{item.summary}</small>}
                </span>
                <span className="activity-time">{formatRelativeTime(item.created_at)}</span>
              </button>
            ))}
          </div>
        </section>
      )}
      <section className="content-section compact-section">
        <div className="section-heading">
          <div><span className="section-kicker">OPEN API / FOR AGENTS</span><h2>开放能力</h2></div>
          <button className="outline-button" onClick={onOpenDocs}>查看接口文档 <ArrowUpRight size={15} /></button>
        </div>
        <div className="curated-row">
          <div className="curated-cover cover-blue"><div className="cover-label">OPEN API</div><span>Publish<br />via Agent</span></div>
          <div className="curated-copy">
            <span className="meta-label">面向 AI Agent · MCP · 自动化脚本</span>
            <h3>用 Open API 把项目自动发布到平台</h3>
            <p>通过 Bearer AccessKey 鉴权，调用开放接口即可预签名上传代码、创建草稿并提交审核，让你的 Agent 技能或工具链把成果直接发布到新猿译码。</p>
            <button className="primary-button" onClick={onOpenDocs}>查看接口文档 <ArrowUpRight size={16} /></button>
          </div>
        </div>
      </section>
      <footer className="site-footer">
        <span>© 2026 新猿译码</span>
        <span>一套面向 Agent 开发者的开放索引</span>
        <a className="footer-rss" href="/feed.xml" title="RSS 订阅最新项目">RSS 订阅 <ArrowUpRight size={12} /></a>
        <span className={`gateway-state ${gatewayState.status}`}>
          <span className="gateway-state-dot" />
          {gatewayState.status === 'online'
            ? `Gateway ${gatewayState.info.api_version} 已连接`
            : gatewayState.status === 'checking'
              ? '正在连接 Gateway'
              : '演示数据 · Gateway 未连接'}
        </span>
      </footer>
    </main>
  )
}

function ProjectCard({ project, isSaved, onOpen, onToggleSaved }: { project: Project; isSaved: boolean; onOpen: () => void; onToggleSaved: () => void }) {
  return (
    <article className="project-card">
      <button className={`project-cover ${project.accent}`} onClick={onOpen} aria-label={`打开 ${project.name}`}>
        {project.hasCover
          ? <img className="project-cover-image" src={`/api/v1/projects/${encodeURIComponent(project.slug)}/resources/cover`} alt="" loading="lazy" />
          : <><div className="cover-orbit orbit-one" /><div className="cover-orbit orbit-two" /><div className="cover-orbit orbit-three" /><span className="cover-monogram">{project.name.slice(0, 1)}</span></>}
        <span className="cover-index">{project.category}</span>
      </button>
      <div className="project-card-body">
        <div className="card-title-row"><div><span className="project-category">{project.category}</span><h3><button onClick={onOpen}>{project.name}</button></h3></div><button className={`icon-button ${isSaved ? 'saved' : ''}`} title={isSaved ? '取消收藏' : '收藏项目'} aria-label={isSaved ? '取消收藏' : '收藏项目'} onClick={onToggleSaved}><Heart size={17} fill={isSaved ? 'currentColor' : 'none'} /></button></div>
        <p>{project.summary}</p>
        <div className="tag-row">{project.tags.slice(0, 2).map((tag) => <span key={tag}>{tag}</span>)}</div>
        <div className="card-footer"><span><Eye size={13} /> {project.views}</span><span><Download size={13} /> {project.downloads}</span><span><Star size={13} fill="currentColor" /> {project.stars}</span><span className="card-updated">{project.updated}</span></div>
      </div>
    </article>
  )
}

function ProjectDetail({
  project,
  detailTab,
  setDetailTab,
  isSaved,
  isFollowed,
  onBack,
  onOpenProfile,
  onToggleSaved,
  onToggleFollowed,
  onShare,
  onReport,
  onReportComment,
  comments,
  selectedQuote,
  composerAnchor,
  commentComposerOpen,
  setCommentComposerOpen,
  draftComment,
  setDraftComment,
  onSelection,
  onSubmitComment,
  onResolveComment,
  onLikeComment,
  onReplyComment,
  onDeleteReply,
  onDeleteComment,
  onEditComment,
  onEditReply,
  showToast,
  detailState,
  documentState,
  documentTree,
  activeDocument,
  onOpenDocument,
  commentsState,
  commentSubmitting,
  resolvingCommentID,
  deletingCommentID,
  currentUserID,
  currentUser,
  onPublishedDocumentSaved,
}: {
  project: Project
  detailTab: string
  setDetailTab: (tab: string) => void
  isSaved: boolean
  isFollowed: boolean
  onBack: () => void
  onOpenProfile: (userId: string) => void
  onToggleSaved: () => void
  onToggleFollowed: () => void
  onShare: () => void
  onReport: () => void
  onReportComment: (commentId: string) => void
  comments: CommentItem[]
  selectedQuote: string
  composerAnchor: { top: number } | null
  commentComposerOpen: boolean
  setCommentComposerOpen: (open: boolean) => void
  draftComment: string
  setDraftComment: (value: string) => void
  onSelection: () => void
  onSubmitComment: () => void
  onResolveComment: (commentId: string) => void
  onLikeComment: (commentId: string, liked: boolean) => void
  onReplyComment: (commentId: string, body: string) => Promise<boolean>
  onDeleteReply: (commentId: string, replyId: string) => Promise<boolean>
  onDeleteComment: (commentId: string) => Promise<boolean>
  onEditComment: (commentId: string, body: string) => Promise<boolean>
  onEditReply: (commentId: string, replyId: string, body: string) => Promise<boolean>
  showToast: (message: string) => void
  detailState: CatalogState
  documentState: CatalogState
  documentTree: DocumentNode[]
  activeDocument: DocumentDetail | null
  onOpenDocument: (documentSlug: string) => void
  commentsState: CatalogState
  commentSubmitting: boolean
  resolvingCommentID: string | null
  deletingCommentID: string | null
  currentUserID: string
  currentUser: AuthUser | null
  onPublishedDocumentSaved: (markdown: string) => void
}) {
  const [collaborationAccess, setCollaborationAccess] = useState<CollaborationAccess | null>(null)
  const [collaborationEditing, setCollaborationEditing] = useState(false)
  const [collaborationInitialMarkdown, setCollaborationInitialMarkdown] = useState('')
  const [permissionsOpen, setPermissionsOpen] = useState(false)
  const [moreOpen, setMoreOpen] = useState(false)
  const moreMenuRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    if (!moreOpen) return
    const handlePointerDown = (event: PointerEvent) => {
      if (event.target instanceof Node && moreMenuRef.current?.contains(event.target)) return
      setMoreOpen(false)
    }
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setMoreOpen(false)
    }
    document.addEventListener('pointerdown', handlePointerDown, true)
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('pointerdown', handlePointerDown, true)
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [moreOpen])

  useEffect(() => {
    const controller = new AbortController()
    setCollaborationAccess(null)
    setCollaborationEditing(false)
    setPermissionsOpen(false)
    setMoreOpen(false)
    getProjectCollaborationAccess(project.slug, controller.signal)
      .then((response) => setCollaborationAccess(response.data))
      .catch(() => setCollaborationAccess({ role: 'viewer', can_edit: false, can_manage: false }))
    return () => controller.abort()
  }, [currentUser?.id, project.slug])

  const startCollaboration = () => {
    if (!currentUser || !collaborationAccess?.can_edit) return
    // 协作目标跟随当前打开的文档；未打开文档时才编辑项目正文。
    setCollaborationInitialMarkdown(activeDocument?.markdown ?? project.description)
    setCollaborationEditing(true)
  }

  return (
    <main className="detail-page">
      <div className="detail-topbar"><button className="back-button" onClick={onBack}><ArrowLeft size={16} /> 返回项目索引</button><span className="breadcrumb">/ {project.category} / {project.name}</span></div>
      {detailState !== 'online' && (
        <div className={`detail-sync-state ${detailState}`}>
          <span className="gateway-state-dot" />
          {detailState === 'checking' ? '正在同步项目详情…' : '详情 API 暂时不可用，当前展示缓存内容。'}
        </div>
      )}
      <section className="detail-intro">
        <div className={`detail-mark ${project.accent}`}><span>{project.name.slice(0, 1)}</span></div>
        <div className="detail-copy"><span className="eyebrow">{project.status} · 最后更新于 {project.updated}</span><h1>{project.name}</h1><p>{project.description}</p><div className="detail-meta">{project.ownerId ? <button type="button" className="author-profile-link" onClick={() => onOpenProfile(project.ownerId!)} title="查看作者主页"><CircleUserRound size={15} /> {project.authorName || project.maintainer}</button> : <span><CircleUserRound size={15} /> {project.maintainer}</span>}<span><GitBranch size={15} /> {project.repo}</span><span><Tag size={15} /> {project.license}</span></div></div>
        <div className="detail-actions">
          {collaborationAccess?.can_edit && <button className="primary-button" onClick={startCollaboration}><Pencil size={15} /> 协作编辑</button>}
          {collaborationAccess?.can_manage && <button className="icon-button" title="管理协作权限" aria-label="管理协作权限" onClick={() => setPermissionsOpen(true)}><ShieldCheck size={17} /></button>}
          <button className={`outline-button ${isSaved ? 'is-saved' : ''}`} onClick={onToggleSaved}><Heart size={15} fill={isSaved ? 'currentColor' : 'none'} /> {isSaved ? '已收藏' : '收藏'}</button>
          <button className={`outline-button ${isFollowed ? 'is-saved' : ''}`} onClick={onToggleFollowed}><Bell size={15} fill={isFollowed ? 'currentColor' : 'none'} /> {isFollowed ? '已关注' : '关注'}</button>
          <button className="icon-button" title="分享项目" aria-label="分享项目" onClick={onShare}><Share2 size={17} /></button>
          <button className="icon-button" title="举报项目" aria-label="举报项目" onClick={onReport}><Flag size={16} /></button>
          <div className="detail-more-wrap" ref={moreMenuRef}>
            <button className="icon-button" title="更多操作" aria-label="更多操作" aria-expanded={moreOpen} onClick={() => setMoreOpen((open) => !open)}><MoreHorizontal size={18} /></button>
            {moreOpen && (
              <div className="action-menu" role="menu" aria-label="项目更多操作">
                {project.repo && <button type="button" role="menuitem" onClick={() => { window.open(project.repo, '_blank', 'noopener,noreferrer'); setMoreOpen(false) }}><GitBranch size={14} /> 在 GitHub 查看</button>}
                <button type="button" role="menuitem" onClick={() => { setDetailTab('代码预览'); setMoreOpen(false) }}><FileCode2 size={14} /> 打开代码预览</button>
                <button type="button" role="menuitem" onClick={() => { setDetailTab('下载资源'); setMoreOpen(false) }}><Download size={14} /> 打开下载资源</button>
                <button type="button" role="menuitem" onClick={() => { onShare(); setMoreOpen(false) }}><Copy size={14} /> 复制项目链接</button>
              </div>
            )}
          </div>
        </div>
      </section>
      <div className="detail-stats"><div><strong>{project.views}</strong><span>浏览</span></div><div><strong>{project.downloads}</strong><span>下载</span></div><div><strong>{project.stars}</strong><span>Stars</span></div><div><strong>{project.comments}</strong><span>讨论</span></div><div><strong>{project.currentVersion ? `v${project.currentVersion}` : '-'}</strong><span>当前版本</span></div></div>
      <nav className="detail-tabs">{['项目概览', '文档阅读', '代码预览', '下载资源'].map((tab) => <button key={tab} className={detailTab === tab ? 'active' : ''} onClick={() => setDetailTab(tab)}>{tab}</button>)}</nav>

      {collaborationEditing && currentUser ? (
        <section className="detail-collaboration-workspace">
          <header>
            <div><span className="section-kicker">LIVE DOCUMENT / {collaborationAccess?.role.toUpperCase()}</span><h2>协作编辑《{activeDocument?.title ?? project.name}》</h2><p>修改会实时同步给其他编辑者，并自动更新公开内容。每篇文档有独立的协作房间。</p></div>
            <button className="outline-button" onClick={() => setCollaborationEditing(false)}><X size={15} /> 退出编辑</button>
          </header>
          {/* 协作编辑器涉及 WebSocket 与 Yjs，异常时至少要保住“退出编辑”入口。 */}
          <ErrorBoundary label="实时协作编辑器">
            <Suspense fallback={<div className="collaboration-loading">正在加载实时协作编辑器…</div>}>
              <CollaborativeMarkdownEditor
                slug={activeDocument ? `${project.slug}/${activeDocument.slug}` : project.slug}
                initialMarkdown={collaborationInitialMarkdown}
                user={currentUser}
                webSocketURL={getProjectCollaborationWebSocketURL(project.slug, activeDocument?.slug)}
                onSaved={(markdown) => {
                  onPublishedDocumentSaved(markdown)
                  showToast('协作文档已保存')
                }}
              />
            </Suspense>
          </ErrorBoundary>
        </section>
      ) : (
        <>
          {/* 每个标签各自设边界：某个标签内容异常不应连带项目头部与导航一起白屏。 */}
          {detailTab === '文档阅读' && <ErrorBoundary label="文档阅读" onReset={() => activeDocument && onOpenDocument(activeDocument.slug)}><DocumentView project={project} onOpenProfile={onOpenProfile} documentState={documentState} documentTree={documentTree} activeDocument={activeDocument} onOpenDocument={onOpenDocument} comments={comments} commentsState={commentsState} commentSubmitting={commentSubmitting} resolvingCommentID={resolvingCommentID} deletingCommentID={deletingCommentID} currentUserID={currentUserID} selectedQuote={selectedQuote} composerAnchor={composerAnchor} commentComposerOpen={commentComposerOpen} setCommentComposerOpen={setCommentComposerOpen} draftComment={draftComment} setDraftComment={setDraftComment} onSelection={onSelection} onSubmitComment={onSubmitComment} onResolveComment={onResolveComment} onLikeComment={onLikeComment} onReplyComment={onReplyComment} onDeleteReply={onDeleteReply} onDeleteComment={onDeleteComment} onEditComment={onEditComment} onEditReply={onEditReply} onReportComment={onReportComment} showToast={showToast} /></ErrorBoundary>}
          {detailTab === '代码预览' && <ErrorBoundary label="代码预览"><CodeView project={project} onCopy={() => showToast('代码已复制到剪贴板')} showToast={showToast} /></ErrorBoundary>}
      {detailTab === '下载资源' && <ErrorBoundary label="下载资源"><DownloadView project={project} /></ErrorBoundary>}
          {detailTab === '项目概览' && <ErrorBoundary label="项目概览"><OverviewView project={project} onRead={() => setDetailTab('文档阅读')} /></ErrorBoundary>}
        </>
      )}
      {permissionsOpen && collaborationAccess?.can_manage && <ProjectCollaborationPermissions projectSlug={project.slug} onClose={() => setPermissionsOpen(false)} showToast={showToast} />}
    </main>
  )
}

function ProjectCollaborationPermissions({
  projectSlug,
  onClose,
  showToast,
}: {
  projectSlug: string
  onClose: () => void
  showToast: (message: string) => void
}) {
  const [collaborators, setCollaborators] = useState<ProjectCollaborator[]>([])
  const [email, setEmail] = useState('')
  const [role, setRole] = useState<'editor' | 'viewer'>('editor')
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [removingID, setRemovingID] = useState<string | null>(null)
  const [error, setError] = useState('')
  usePageScrollLock()

  useEffect(() => {
    const controller = new AbortController()
    getProjectCollaborators(projectSlug, controller.signal)
      .then((response) => setCollaborators(response.data))
      .catch((caught: unknown) => {
        if (caught instanceof DOMException && caught.name === 'AbortError') return
        setError(caught instanceof Error ? caught.message : '协作者列表加载失败')
      })
      .finally(() => setLoading(false))
    return () => controller.abort()
  }, [projectSlug])

  const saveCollaborator = async (targetEmail: string, targetRole: 'editor' | 'viewer') => {
    setSubmitting(true)
    setError('')
    try {
      const response = await setProjectCollaborator(projectSlug, { email: targetEmail, role: targetRole })
      setCollaborators((current) => {
        const existing = current.some((item) => item.user_id === response.data.user_id)
        return existing
          ? current.map((item) => item.user_id === response.data.user_id ? response.data : item)
          : [...current, response.data]
      })
      setEmail('')
      showToast(targetRole === 'editor' ? '已授予编辑权限' : '已设为只读')
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '协作者保存失败')
    } finally {
      setSubmitting(false)
    }
  }

  const removeCollaborator = async (collaborator: ProjectCollaborator) => {
    setRemovingID(collaborator.user_id)
    setError('')
    try {
      await deleteProjectCollaborator(projectSlug, collaborator.user_id)
      setCollaborators((current) => current.filter((item) => item.user_id !== collaborator.user_id))
      showToast('已移除协作者')
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '移除协作者失败')
    } finally {
      setRemovingID(null)
    }
  }

  return (
    <div className="modal-backdrop" role="presentation">
      <section className="collaboration-permissions" role="dialog" aria-modal="true" aria-label="管理协作权限">
        <button className="icon-button modal-close" aria-label="关闭" onClick={onClose}><X size={17} /></button>
        <header><span className="section-kicker">ACCESS CONTROL</span><h2>管理协作权限</h2><p>只有可编辑成员能够进入实时协作空间；只读成员仍可浏览公开文档。</p></header>
        <div className="collaborator-invite">
          <input type="email" value={email} onChange={(event) => setEmail(event.target.value)} placeholder="输入已注册用户的邮箱" />
          <select value={role} onChange={(event) => setRole(event.target.value as 'editor' | 'viewer')}>
            <option value="editor">可编辑</option>
            <option value="viewer">只读</option>
          </select>
          <button className="primary-button" disabled={submitting || !email.trim()} onClick={() => void saveCollaborator(email.trim(), role)}><UserPlus size={15} /> {submitting ? '保存中…' : '添加成员'}</button>
        </div>
        {error && <div className="auth-error">{error}</div>}
        <div className="collaborator-list">
          {loading ? <p>正在加载协作者…</p> : collaborators.length === 0 ? <p className="empty-copy">还没有额外协作者。</p> : collaborators.map((collaborator) => (
            <article key={collaborator.user_id}>
              <span className="avatar small-avatar">{collaborator.display_name.slice(0, 1)}</span>
              <div><strong>{collaborator.display_name}</strong><small>{collaborator.email}</small></div>
              <select
                value={collaborator.role}
                disabled={submitting || removingID !== null}
                onChange={(event) => void saveCollaborator(collaborator.email, event.target.value as 'editor' | 'viewer')}
              >
                <option value="editor">可编辑</option>
                <option value="viewer">只读</option>
              </select>
              <button className="icon-button danger" title="移除协作者" aria-label={`移除 ${collaborator.display_name}`} disabled={removingID !== null} onClick={() => void removeCollaborator(collaborator)}>
                <Trash2 size={15} />
              </button>
            </article>
          ))}
        </div>
      </section>
    </div>
  )
}

function DocumentView({ project, onOpenProfile, documentState, documentTree, activeDocument, onOpenDocument, comments, commentsState, commentSubmitting, resolvingCommentID, deletingCommentID, currentUserID, selectedQuote, composerAnchor, commentComposerOpen, setCommentComposerOpen, draftComment, setDraftComment, onSelection, onSubmitComment, onResolveComment, onLikeComment, onReplyComment, onDeleteReply, onDeleteComment, onEditComment, onEditReply, onReportComment, showToast }: { project: Project; onOpenProfile: (userId: string) => void; documentState: CatalogState; documentTree: DocumentNode[]; activeDocument: DocumentDetail | null; onOpenDocument: (documentSlug: string) => void; comments: CommentItem[]; commentsState: CatalogState; commentSubmitting: boolean; resolvingCommentID: string | null; deletingCommentID: string | null; currentUserID: string; selectedQuote: string; composerAnchor: { top: number } | null; commentComposerOpen: boolean; setCommentComposerOpen: (open: boolean) => void; draftComment: string; setDraftComment: (value: string) => void; onSelection: () => void; onSubmitComment: () => void; onResolveComment: (commentId: string) => void; onLikeComment: (commentId: string, liked: boolean) => void; onReplyComment: (commentId: string, body: string) => Promise<boolean>; onDeleteReply: (commentId: string, replyId: string) => Promise<boolean>; onDeleteComment: (commentId: string) => Promise<boolean>; onEditComment: (commentId: string, body: string) => Promise<boolean>; onEditReply: (commentId: string, replyId: string, body: string) => Promise<boolean>; onReportComment: (commentId: string) => void; showToast: (message: string) => void }) {
  const articleContentRef = useRef<HTMLDivElement | null>(null)
  const [lightbox, setLightbox] = useState<{ src: string; alt: string } | null>(null)
  const [searchKeyword, setSearchKeyword] = useState('')
  const [commentFilter, setCommentFilter] = useState<'all' | 'open' | 'resolved'>('all')
  const [commentFilterOpen, setCommentFilterOpen] = useState(false)
  const commentFilterRef = useRef<HTMLDivElement | null>(null)
  const documentSearch = useDocumentSearch(articleContentRef, searchKeyword, activeDocument?.id ?? '')

  useEffect(() => {
    if (!commentFilterOpen) return
    const handlePointerDown = (event: PointerEvent) => {
      if (event.target instanceof Node && commentFilterRef.current?.contains(event.target)) return
      setCommentFilterOpen(false)
    }
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setCommentFilterOpen(false)
    }
    document.addEventListener('pointerdown', handlePointerDown, true)
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('pointerdown', handlePointerDown, true)
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [commentFilterOpen])

  const visibleComments = commentFilter === 'all'
    ? comments
    : comments.filter((comment) => comment.status === commentFilter)
  // 阅读增强：正文排版偏好、目录滚动高亮、阅读进度/回到顶部、上下篇导航。
  const readerPrefs = useReaderPreferences()
  const activeHeadingId = useScrollSpy(activeDocument?.outline, activeDocument?.id ?? '')
  const scrollMetrics = useScrollMetrics(articleContentRef)
  const flatDocuments = useMemo(() => flattenDocumentTree(documentTree), [documentTree])

  // 切换文档时重置阅读态，避免上一篇的搜索词和图片预览沿用到新文档。
  useEffect(() => {
    setSearchKeyword('')
    setLightbox(null)
  }, [activeDocument?.id])

  const headingId = (children: ReactNode) => {
    const title = reactNodeText(children)
    return activeDocument?.outline.find((item) => item.title === title)?.id
  }
  const stableBlockID = (children: ReactNode, type: string) => {
    const text = reactNodeText(children).trim()
    return activeDocument?.blocks.find((item) => item.type === type && (item.text === text || item.text.includes(text) || text.includes(item.text)))?.id
  }
  const renderDocumentNodes = (nodes: DocumentNode[], depth = 0): ReactNode => nodes.map((node) => (
    <div key={node.id}>
      <button className={`tree-item ${node.slug === activeDocument?.slug ? 'active' : ''} ${depth ? 'indent' : ''}`} disabled={documentState === 'checking'} onClick={() => onOpenDocument(node.slug)}>
        {node.children.length ? <ChevronRight size={14} /> : <FileText size={15} />} {node.title}
      </button>
      {node.children.length > 0 && renderDocumentNodes(node.children, depth + 1)}
    </div>
  ))
  const copyDocumentLink = async () => {
    if (!activeDocument) return
    const anchor = activeDocument.outline[0]?.id
    const url = new URL(window.location.href)
    url.hash = anchor ? `#${anchor}` : ''
    try {
      await navigator.clipboard.writeText(url.toString())
      showToast('文档链接已复制')
    } catch {
      showToast('浏览器未允许复制，请手动复制地址栏链接')
    }
  }
  const markdownDownloadURL = activeDocument
    ? `data:text/markdown;charset=utf-8,${encodeURIComponent(activeDocument.markdown)}`
    : undefined

  // 复制到指定小节的锚点链接。
  const copySectionLink = (anchor: string) => {
    const url = new URL(window.location.href)
    url.hash = `#${anchor}`
    navigator.clipboard?.writeText(url.toString())
      .then(() => showToast('小节链接已复制'))
      .catch(() => showToast('浏览器未允许复制，请手动复制地址栏链接'))
  }

  // 点击评论引用时，平滑滚动到锚定的原文区块并短暂高亮。
  const locateBlock = (blockId: string) => {
    if (!blockId) return
    const el = articleContentRef.current?.querySelector<HTMLElement>(`[data-block-id="${window.CSS?.escape?.(blockId) ?? blockId}"]`)
    if (!el) {
      showToast('未找到对应的原文位置，正文可能已更新')
      return
    }
    el.scrollIntoView({ behavior: 'smooth', block: 'center' })
    el.classList.add('block-located')
    setTimeout(() => el.classList.remove('block-located'), 2200)
  }

  const renderHeading = (level: 1 | 2 | 3, children: ReactNode) => {
    const id = headingId(children) ?? ''
    const Tag = `h${level}` as 'h1' | 'h2' | 'h3'
    return <Tag id={id || undefined} className="reader-heading">
      {children}
      <HeadingAnchor id={id} onCopy={() => copySectionLink(id)} />
    </Tag>
  }

  return (
    <section className="doc-workspace">
      {activeDocument && <ReadingProgressBar progress={scrollMetrics.progress} />}
      <aside className="doc-sidebar"><div className="sidebar-heading"><span>文档目录</span><button className="icon-button quiet" title="收起目录" aria-label="收起目录"><ChevronDown size={15} /></button></div><div className="doc-project-label"><div className="mini-mark">{project.name.slice(0, 1)}</div><div><strong>{project.name}</strong><small>文档 v{activeDocument?.version ?? project.currentVersion ?? '-'}</small></div></div><nav className="doc-tree">{documentTree.length ? renderDocumentNodes(documentTree) : <span className="doc-tree-empty">暂无文档</span>}</nav>{activeDocument?.outline.length ? <nav className="doc-outline" aria-label="本文大纲"><span className="meta-label">ON THIS PAGE</span>{activeDocument.outline.map((item) => <button key={item.id} className={`${item.level > 1 ? 'indent' : ''} ${item.id === activeHeadingId ? 'is-current' : ''}`.trim()} aria-current={item.id === activeHeadingId ? 'location' : undefined} onClick={() => document.getElementById(item.id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })}>{item.title}</button>)}</nav> : null}<div className="sidebar-bottom"><span className="meta-label">DOCUMENT STATUS</span><p><span className="status-dot" /> 已审核 · 公开可读</p></div></aside>
      <article className="document-article" onMouseUp={onSelection}>
        <div className="article-toolbar"><span className="meta-label">{activeDocument ? activeDocument.title : '文档'}</span>{activeDocument && activeDocument.revision ? <span className="article-revision-meta" title={`这篇文章的第 ${activeDocument.revision} 个修订版本`}><History size={12} /> 修订 v{activeDocument.revision}{activeDocument.updated_by_name ? ` · 最后由 ${activeDocument.updated_by_name} 更新` : ''}</span> : null}<div className="article-toolbar-actions">{activeDocument && <DocumentSearchBox keyword={searchKeyword} onKeywordChange={setSearchKeyword} total={documentSearch.total} activeIndex={documentSearch.activeIndex} onNext={documentSearch.next} onPrevious={documentSearch.previous} />}{activeDocument && <ReaderSettingsControl controller={readerPrefs} />}<button className="tool-button" title="复制文档链接" disabled={!activeDocument} onClick={() => void copyDocumentLink()}><Copy size={14} /> 链接</button>{activeDocument ? <a className="tool-button" title="下载 Markdown" href={markdownDownloadURL} download={`${project.slug}-${activeDocument.slug}.md`} onClick={() => showToast('Markdown 下载已开始')}><Download size={14} /> 下载</a> : <button className="tool-button" title="下载 Markdown" disabled><Download size={14} /> 下载</button>}</div></div>
        {documentState === 'offline' && <div className="document-sync-state offline"><span className="gateway-state-dot" />文档服务暂时不可用，请稍后重试。</div>}
        <div className="article-content reader-surface" ref={articleContentRef} style={readerPrefs.style}>
          {activeDocument ? (
            <ReactMarkdown
              remarkPlugins={[remarkGfm]}
              rehypePlugins={[rehypeHighlight]}
              urlTransform={(url) => url.startsWith('oss://') ? url : defaultUrlTransform(url)}
              components={{
                h1: ({ children }) => renderHeading(1, children),
                h2: ({ children }) => renderHeading(2, children),
                h3: ({ children }) => renderHeading(3, children),
                p: ({ children }) => <p data-block-id={stableBlockID(children, 'paragraph')}>{children}</p>,
                // 代码块的容器由 code 节点自己渲染，因此去掉外层 pre 避免嵌套。
                pre: ({ children }) => <>{children}</>,
                code: ({ children, className }) => {
                  const language = /language-(\w+)/.exec(className ?? '')?.[1] ?? ''
                  const text = reactNodeText(children)
                  if (language === 'mermaid') return <MermaidDiagram source={text.trim()} />
                  // 无语言且不含换行的视为行内代码。
                  if (!language && !text.includes('\n')) {
                    return <code className={className}>{children}</code>
                  }
                  return <span data-block-id={stableBlockID(children, 'code')}>
                    <CodeBlock
                      language={language}
                      text={text}
                      onCopied={(ok) => showToast(ok ? '代码已复制' : '浏览器拒绝了剪贴板写入，请手动复制')}
                    >
                      <code className={className}>{children}</code>
                    </CodeBlock>
                  </span>
                },
                img: ({ src, alt }) => {
                  const resolved = markdownAssetURL(src, false, project.slug) ?? ''
                  return <img
                    className="reader-image"
                    src={resolved}
                    alt={alt ?? ''}
                    loading="lazy"
                    title="点击查看大图"
                    onClick={() => setLightbox({ src: resolved, alt: alt ?? '' })}
                  />
                },
                a: ({ href, children }) => {
                  const embed = href ? bilibiliEmbedURL(href) : ''
                  if (embed) return <BilibiliEmbed url={embed} title={reactNodeText(children)} />
                  return <a href={markdownAssetURL(href, false, project.slug)} target="_blank" rel="noreferrer">{children}</a>
                },
              }}
            >
              {activeDocument.markdown}
            </ReactMarkdown>
          ) : documentState === 'checking' ? (
            <div className="article-empty">正在加载文档…</div>
          ) : (
            // 不再展示任何示例正文：假内容会让作者误以为自己的文档已发布。
            <div className="article-empty">
              <FileText size={26} />
              <h3>该项目暂无可阅读文档</h3>
              <p>作者在项目中心写入并发布正文后，这里会展示真实内容。</p>
            </div>
          )}
        </div>
        {activeDocument && <DocumentPager items={flatDocuments} currentSlug={activeDocument.slug} onOpen={onOpenDocument} />}
        {commentComposerOpen && <div
          className={`selection-composer ${composerAnchor ? 'is-anchored' : ''}`}
          style={composerAnchor ? { top: `${composerAnchor.top}px` } : undefined}
        ><div className="composer-quote">“{selectedQuote}”</div><textarea autoFocus value={draftComment} disabled={commentSubmitting} onChange={(event) => setDraftComment(event.target.value)} placeholder="写下你的评论..." /><div className="composer-actions"><EmojiPicker onSelect={(emoji) => setDraftComment(draftComment + emoji)} /><button className="text-button" disabled={commentSubmitting} onClick={() => setCommentComposerOpen(false)}>取消</button><button className="primary-button small" disabled={commentSubmitting || !draftComment.trim()} onClick={onSubmitComment}><Send size={14} /> {commentSubmitting ? '发布中…' : '发布评论'}</button></div></div>}
      </article>
      <aside className="comments-sidebar"><div className="comments-heading"><div><span className="meta-label">DISCUSSION</span><h3>文档评论 <span>{comments.length} 条线程 · {comments.reduce((total, comment) => total + comment.replyCount, 0)} 条回复</span></h3></div><div className="comment-filter-wrap" ref={commentFilterRef}><button className="icon-button quiet" title="评论筛选" aria-label="评论筛选" aria-expanded={commentFilterOpen} onClick={() => setCommentFilterOpen((open) => !open)}><MoreHorizontal size={17} /></button>{commentFilterOpen && <div className="action-menu comment-filter-menu" role="menu" aria-label="评论筛选"><button type="button" role="menuitem" className={commentFilter === 'all' ? 'active' : ''} onClick={() => { setCommentFilter('all'); setCommentFilterOpen(false) }}>全部</button><button type="button" role="menuitem" className={commentFilter === 'open' ? 'active' : ''} onClick={() => { setCommentFilter('open'); setCommentFilterOpen(false) }}>未解决</button><button type="button" role="menuitem" className={commentFilter === 'resolved' ? 'active' : ''} onClick={() => { setCommentFilter('resolved'); setCommentFilterOpen(false) }}>已解决</button></div>}</div></div><button className="new-comment-button" disabled={commentsState === 'checking'} onClick={() => setCommentComposerOpen(true)}><MessageSquare size={15} /> {commentsState === 'checking' ? '加载评论中…' : '添加评论'}</button>{commentFilter !== 'all' && <div className="comment-filter-note">正在查看{commentFilter === 'open' ? '未解决' : '已解决'}评论，<button type="button" onClick={() => setCommentFilter('all')}>显示全部</button></div>}<div className="comment-list">{visibleComments.length ? visibleComments.map((comment) => <CommentCard key={comment.id} comment={comment} onOpenProfile={onOpenProfile} currentUserID={currentUserID} resolving={resolvingCommentID === comment.id} deleting={deletingCommentID === comment.id} onResolve={() => onResolveComment(comment.id)} onLike={onLikeComment} onReply={(body) => onReplyComment(comment.id, body)} onDeleteReply={(replyId) => onDeleteReply(comment.id, replyId)} onDelete={() => onDeleteComment(comment.id)} onEdit={(body) => onEditComment(comment.id, body)} onEditReply={(replyId, body) => onEditReply(comment.id, replyId, body)} onReport={() => onReportComment(comment.id)} onLocate={locateBlock} />) : <div className="comment-filter-empty">没有{commentFilter === 'open' ? '未解决' : '已解决'}的评论</div>}</div><div className={`realtime-note ${commentsState}`}><span className="status-dot" /> {commentsState === 'offline' ? '评论同步暂时离线' : commentsState === 'checking' ? '评论同步中…' : '评论实时同步中'}</div></aside>
      {lightbox && <ImageLightbox src={lightbox.src} alt={lightbox.alt} onClose={() => setLightbox(null)} />}
      {activeDocument && <BackToTopButton visible={scrollMetrics.scrolled} />}
    </section>
  )
}

function CommentCard({ comment, onOpenProfile, currentUserID, resolving, deleting, onResolve, onLike, onReply, onDeleteReply, onDelete, onEdit, onEditReply, onReport, onLocate }: { comment: CommentItem; onOpenProfile: (userId: string) => void; currentUserID: string; resolving: boolean; deleting: boolean; onResolve: () => void; onLike: (commentId: string, liked: boolean) => void; onReply: (body: string) => Promise<boolean>; onDeleteReply: (replyId: string) => Promise<boolean>; onDelete: () => Promise<boolean>; onEdit: (body: string) => Promise<boolean>; onEditReply: (replyId: string, body: string) => Promise<boolean>; onReport: () => void; onLocate: (blockId: string) => void }) {
  const [replyOpen, setReplyOpen] = useState(false)
  const [replyDraft, setReplyDraft] = useState('')
  const [replySubmitting, setReplySubmitting] = useState(false)
  const [deletingReplyID, setDeletingReplyID] = useState<string | null>(null)
  const [editing, setEditing] = useState(false)
  const [editDraft, setEditDraft] = useState(comment.text)
  const [editSubmitting, setEditSubmitting] = useState(false)
  const [editingReplyID, setEditingReplyID] = useState<string | null>(null)
  const [replyEditDraft, setReplyEditDraft] = useState('')
  const [replyEditSubmitting, setReplyEditSubmitting] = useState(false)
  const [moreOpen, setMoreOpen] = useState(false)
  const [copied, setCopied] = useState(false)
  const moreMenuRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    if (!moreOpen) return
    const handlePointerDown = (event: PointerEvent) => {
      if (event.target instanceof Node && moreMenuRef.current?.contains(event.target)) return
      setMoreOpen(false)
    }
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setMoreOpen(false)
    }
    document.addEventListener('pointerdown', handlePointerDown, true)
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('pointerdown', handlePointerDown, true)
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [moreOpen])

  const copyCommentLink = () => {
    const url = new URL(window.location.href)
    url.hash = `comment-${comment.id}`
    navigator.clipboard?.writeText(url.toString())
      .then(() => {
        setCopied(true)
        window.setTimeout(() => setCopied(false), 1600)
      })
      .catch(() => undefined)
  }

  const submitReply = async () => {
    if (!replyDraft.trim()) return
    setReplySubmitting(true)
    const created = await onReply(replyDraft)
    setReplySubmitting(false)
    if (created) {
      setReplyDraft('')
      setReplyOpen(false)
    }
  }
  const deleteReply = async (replyId: string) => {
    setDeletingReplyID(replyId)
    await onDeleteReply(replyId)
    setDeletingReplyID(null)
  }
  const submitEdit = async () => {
    if (!editDraft.trim()) return
    setEditSubmitting(true)
    const updated = await onEdit(editDraft)
    setEditSubmitting(false)
    if (updated) setEditing(false)
  }
  const submitReplyEdit = async (replyId: string) => {
    if (!replyEditDraft.trim()) return
    setReplyEditSubmitting(true)
    const updated = await onEditReply(replyId, replyEditDraft)
    setReplyEditSubmitting(false)
    if (updated) setEditingReplyID(null)
  }

  return (
    <article id={`comment-${comment.id}`} className={`comment-card ${comment.status === 'resolved' ? 'resolved' : ''} ${comment.authorLevel >= 6 ? 'legendary' : ''}`}>
      <div className="comment-card-head">
        {comment.authorId
          ? <button type="button" className="author-profile-link avatar-link" onClick={() => onOpenProfile(comment.authorId)} title={`查看 ${comment.user} 的主页`} aria-label={`查看 ${comment.user} 的主页`}><LevelAvatar level={comment.authorLevel} initials={comment.initials} size="sm" name={comment.user} avatar={comment.authorAvatar} frame={comment.authorFrame} /></button>
          : <LevelAvatar level={comment.authorLevel} initials={comment.initials} size="sm" name={comment.user} avatar={comment.authorAvatar} frame={comment.authorFrame} />}
        <div>
          <span className="name-row">{comment.authorId ? <button type="button" className={`author-profile-link ${comment.authorLevel >= 6 ? 'nickname-legendary' : ''}`} onClick={() => onOpenProfile(comment.authorId)}><strong>{comment.user}</strong></button> : <strong className={comment.authorLevel >= 6 ? 'nickname-legendary' : ''}>{comment.user}</strong>}<LevelBadge level={comment.authorLevel} /></span>
          <small>{comment.time}{comment.edited ? ' · 已编辑' : ''}{comment.authorRegion ? ` · IP 属地：${comment.authorRegion}` : ''}</small>
        </div>
        {comment.status === 'resolved' ? <Check size={15} className="resolved-icon" /> : (
          <div className="comment-more-wrap" ref={moreMenuRef}>
            <button className="comment-more" title="更多评论操作" aria-label="更多评论操作" aria-expanded={moreOpen} onClick={() => setMoreOpen((open) => !open)}><MoreHorizontal size={15} /></button>
            {moreOpen && (
              <div className="action-menu comment-more-menu" role="menu" aria-label="评论操作">
                <button type="button" role="menuitem" onClick={() => { void copyCommentLink(); setMoreOpen(false) }}><Copy size={13} /> {copied ? '已复制' : '复制评论链接'}</button>
                {comment.quote && comment.blockId && <button type="button" role="menuitem" onClick={() => { onLocate(comment.blockId); setMoreOpen(false) }}><Link2 size={13} /> 定位到原文</button>}
              </div>
            )}
          </div>
        )}
      </div>
      {comment.quote && <button className="comment-quote" title="跳转到原文" onClick={() => onLocate(comment.blockId)}>“{comment.quote}”</button>}
      {editing
        ? <div className="inline-edit"><textarea autoFocus value={editDraft} disabled={editSubmitting} onChange={(event) => setEditDraft(event.target.value)} /><div><EmojiPicker onSelect={(emoji) => setEditDraft((value) => value + emoji)} /><button disabled={editSubmitting} onClick={() => setEditing(false)}>取消</button><button disabled={editSubmitting || !editDraft.trim()} onClick={submitEdit}>{editSubmitting ? '保存中…' : '保存'}</button></div></div>
        : <p>{comment.text}</p>}
      <button className={`comment-like ${comment.liked ? 'liked' : ''}`} onClick={() => onLike(comment.id, comment.liked)} aria-pressed={comment.liked} title={comment.liked ? '取消点赞' : '点赞'}><ThumbsUp size={13} fill={comment.liked ? 'currentColor' : 'none'} />{comment.likeCount > 0 ? ` ${comment.likeCount}` : ''}</button>
      {comment.replies.length > 0 && (
        <div className="comment-replies">
          {comment.replies.map((reply) => (
            <div className={`comment-reply ${reply.authorLevel >= 6 ? 'legendary' : ''}`} key={reply.id}>
              <div className="reply-head">{reply.authorId ? <button type="button" className="author-profile-link avatar-link" onClick={() => onOpenProfile(reply.authorId)} title={`查看 ${reply.user} 的主页`} aria-label={`查看 ${reply.user} 的主页`}><LevelAvatar level={reply.authorLevel} initials={reply.initials} size="sm" name={reply.user} avatar={reply.authorAvatar} frame={reply.authorFrame} /></button> : <LevelAvatar level={reply.authorLevel} initials={reply.initials} size="sm" name={reply.user} avatar={reply.authorAvatar} frame={reply.authorFrame} />}{reply.authorId ? <button type="button" className={`author-profile-link ${reply.authorLevel >= 6 ? 'nickname-legendary' : ''}`} onClick={() => onOpenProfile(reply.authorId)}><strong>{reply.user}</strong></button> : <strong className={reply.authorLevel >= 6 ? 'nickname-legendary' : ''}>{reply.user}</strong>}<LevelBadge level={reply.authorLevel} /><span><small>{reply.time}{reply.edited ? ' · 已编辑' : ''}</small>{currentUserID !== '' && reply.authorId === currentUserID && <><button disabled={replyEditSubmitting || deletingReplyID === reply.id} onClick={() => { setEditingReplyID(reply.id); setReplyEditDraft(reply.text) }}>编辑</button><button disabled={deletingReplyID === reply.id} onClick={() => deleteReply(reply.id)}>{deletingReplyID === reply.id ? '删除中…' : '删除'}</button></>}</span></div>
              {editingReplyID === reply.id
                ? <div className="inline-edit"><textarea autoFocus value={replyEditDraft} disabled={replyEditSubmitting} onChange={(event) => setReplyEditDraft(event.target.value)} /><div><EmojiPicker onSelect={(emoji) => setReplyEditDraft((value) => value + emoji)} /><button disabled={replyEditSubmitting} onClick={() => setEditingReplyID(null)}>取消</button><button disabled={replyEditSubmitting || !replyEditDraft.trim()} onClick={() => submitReplyEdit(reply.id)}>{replyEditSubmitting ? '保存中…' : '保存'}</button></div></div>
                : <p>{reply.text}</p>}
              <button className={`comment-like ${reply.liked ? 'liked' : ''}`} onClick={() => onLike(reply.id, reply.liked)} aria-pressed={reply.liked} title={reply.liked ? '取消点赞' : '点赞'}><ThumbsUp size={12} fill={reply.liked ? 'currentColor' : 'none'} />{reply.likeCount > 0 ? ` ${reply.likeCount}` : ''}</button>
            </div>
          ))}
        </div>
      )}
      {replyOpen && <div className="reply-composer"><textarea autoFocus value={replyDraft} disabled={replySubmitting} onChange={(event) => setReplyDraft(event.target.value)} placeholder="写下回复…" /><div><EmojiPicker onSelect={(emoji) => setReplyDraft((value) => value + emoji)} /><button disabled={replySubmitting} onClick={() => setReplyOpen(false)}>取消</button><button disabled={replySubmitting || !replyDraft.trim()} onClick={submitReply}>{replySubmitting ? '发布中…' : '发布回复'}</button></div></div>}
      {comment.status === 'open'
        ? <div className="comment-actions">{currentUserID !== '' && comment.authorId === currentUserID && <button disabled={resolving || deleting || replySubmitting} onClick={onResolve}>{resolving ? '解决中…' : '解决'}</button>}<button disabled={resolving || deleting || replySubmitting} onClick={() => setReplyOpen((open) => !open)}>回复</button>{comment.authorId !== currentUserID && <button className="comment-report" title="举报评论" onClick={onReport}><Flag size={12} /> 举报</button>}{currentUserID !== '' && comment.authorId === currentUserID && <><button disabled={editSubmitting || deleting} onClick={() => { setEditDraft(comment.text); setEditing(true) }}>编辑</button><button disabled={deleting} onClick={onDelete}>{deleting ? '删除中…' : '删除'}</button></>}</div>
        : <span className="resolved-label">已解决</span>}
    </article>
  )
}

function OverviewView({ project, onRead }: { project: Project; onRead: () => void }) {
  const highlights = project.highlights ?? ['项目结构清晰', '文档公开可读', '支持快速复用']
  const useCases = project.useCases ?? project.tags
  return <section className="overview-view"><div className="overview-main"><span className="section-kicker">ABOUT THIS PROJECT</span><h2>{project.summary}</h2><MarkdownCanvas markdown={project.description} projectSlug={project.slug} /><div className="feature-list">{highlights.map((highlight) => <div key={highlight}><Check size={16} /><span>{highlight}</span></div>)}</div><button className="primary-button" onClick={onRead}>打开文档 <BookOpen size={16} /></button></div><div className={`overview-visual ${project.accent}`}><div className="visual-topline"><span>VERSION / {project.currentVersion ?? '-'}</span><span>READY</span></div><div className="overview-lines">{useCases.slice(0, 4).map((useCase) => <span key={useCase}>{useCase}</span>)}</div><div className="overview-bottom"><span>{useCases.length} 个适用场景</span><span>{project.status}</span></div></div></section>
}

function formatFileSize(bytes: number) {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

// 代码阅读偏好持久化：自动换行开关记在 localStorage，跨会话保留。
const CODE_WRAP_STORAGE_KEY = 'nyym:code-view:wrap'

function readCodeWrapPref() {
  try {
    return window.localStorage.getItem(CODE_WRAP_STORAGE_KEY) === '1'
  } catch {
    return false
  }
}

// 行锚点写进 URL hash：`#L{n}` 满足永久链接语义，附带 file= 让同一项目页内可重新定位到对应文件。
function buildCodeLineHash(filePath: string, line: number) {
  return `#L${line}&file=${encodeURIComponent(filePath)}`
}

function parseCodeLineHash(hash: string): { line: number | null; file: string | null } {
  const raw = hash.replace(/^#/, '')
  let line: number | null = null
  let file: string | null = null
  for (const part of raw.split('&')) {
    const matched = /^L(\d+)$/.exec(part)
    if (matched) {
      line = Number(matched[1])
      continue
    }
    if (part.startsWith('file=')) {
      try {
        file = decodeURIComponent(part.slice(5))
      } catch {
        file = null
      }
    }
  }
  return { line, file }
}

// 扩展名 → 展示标签与色调，供树内色点与顶栏语言标识共用。
const CODE_FILE_TYPES: Record<string, { label: string; tone: string }> = {
  ts: { label: 'TS', tone: 'ts' }, tsx: { label: 'TSX', tone: 'ts' }, mts: { label: 'TS', tone: 'ts' }, cts: { label: 'TS', tone: 'ts' }, d: { label: 'DTS', tone: 'ts' },
  js: { label: 'JS', tone: 'js' }, jsx: { label: 'JSX', tone: 'js' }, mjs: { label: 'JS', tone: 'js' }, cjs: { label: 'JS', tone: 'js' },
  json: { label: 'JSON', tone: 'data' }, sql: { label: 'SQL', tone: 'data' },
  css: { label: 'CSS', tone: 'style' }, scss: { label: 'SCSS', tone: 'style' }, sass: { label: 'SASS', tone: 'style' }, less: { label: 'LESS', tone: 'style' },
  html: { label: 'HTML', tone: 'markup' }, htm: { label: 'HTML', tone: 'markup' }, xml: { label: 'XML', tone: 'markup' }, vue: { label: 'VUE', tone: 'markup' }, svg: { label: 'SVG', tone: 'markup' },
  md: { label: 'MD', tone: 'doc' }, markdown: { label: 'MD', tone: 'doc' }, mdx: { label: 'MDX', tone: 'doc' }, rst: { label: 'RST', tone: 'doc' }, txt: { label: 'TXT', tone: 'doc' },
  yml: { label: 'YAML', tone: 'config' }, yaml: { label: 'YAML', tone: 'config' }, toml: { label: 'TOML', tone: 'config' }, ini: { label: 'INI', tone: 'config' }, env: { label: 'ENV', tone: 'config' }, conf: { label: 'CONF', tone: 'config' },
  go: { label: 'GO', tone: 'sys' }, rs: { label: 'RUST', tone: 'sys' }, c: { label: 'C', tone: 'sys' }, h: { label: 'C', tone: 'sys' }, cpp: { label: 'C++', tone: 'sys' }, cc: { label: 'C++', tone: 'sys' }, hpp: { label: 'C++', tone: 'sys' }, java: { label: 'JAVA', tone: 'sys' }, kt: { label: 'KT', tone: 'sys' }, swift: { label: 'SWIFT', tone: 'sys' },
  py: { label: 'PY', tone: 'script' }, rb: { label: 'RB', tone: 'script' }, php: { label: 'PHP', tone: 'script' }, sh: { label: 'SH', tone: 'script' }, bash: { label: 'SH', tone: 'script' }, zsh: { label: 'SH', tone: 'script' },
  png: { label: 'IMG', tone: 'media' }, jpg: { label: 'IMG', tone: 'media' }, jpeg: { label: 'IMG', tone: 'media' }, gif: { label: 'IMG', tone: 'media' }, webp: { label: 'IMG', tone: 'media' }, ico: { label: 'IMG', tone: 'media' },
}

function codeFileMeta(name: string) {
  const lower = name.toLowerCase()
  const readme = /^readme(\.|$)/.test(lower)
  const dot = lower.lastIndexOf('.')
  const ext = dot > 0 ? lower.slice(dot + 1) : ''
  const known = CODE_FILE_TYPES[ext]
  if (known) return { label: known.label, tone: known.tone, readme }
  return { label: (ext || 'FILE').toUpperCase().slice(0, 4), tone: 'other', readme }
}

function CodeView({ project, onCopy, showToast }: {
  project: Project
  onCopy: () => void
  showToast: (message: string) => void
}) {
  const [entries, setEntries] = useState<CodeEntry[]>([])
  const [treeState, setTreeState] = useState<'loading' | 'ready' | 'error'>('loading')
  const [treeError, setTreeError] = useState('')
  const [truncated, setTruncated] = useState(false)
  const [fileSearch, setFileSearch] = useState('')
  const [activePath, setActivePath] = useState('')
  const [activeFile, setActiveFile] = useState<CodeFile | null>(null)
  const [fileState, setFileState] = useState<'idle' | 'loading' | 'ready' | 'error'>('idle')
  const [fileError, setFileError] = useState('')
  // README 只在首次加载目录时自动打开，避免用户切换文件后被重置。
  const autoOpenedRef = useRef('')
  // 阅读增强：自动换行开关、行锚点、深链接滚动与键盘导航所需的状态与引用。
  const [wrap, setWrap] = useState(readCodeWrapPref)
  const [selectedLine, setSelectedLine] = useState<number | null>(null)
  const sectionRef = useRef<HTMLElement | null>(null)
  const treeRef = useRef<HTMLDivElement | null>(null)
  const codeRef = useRef<HTMLDivElement | null>(null)
  const pendingScrollRef = useRef(false)
  const keyboardNavRef = useRef(false)

  useEffect(() => {
    const controller = new AbortController()
    const timer = window.setTimeout(() => {
      setTreeState('loading')
      getProjectCodeTree(project.slug, fileSearch.trim() || undefined, controller.signal)
        .then((response) => {
          setEntries(response.data)
          setTruncated(response.truncated)
          setTreeState('ready')
          const readme = response.readme_path
          if (!fileSearch.trim() && readme && autoOpenedRef.current !== project.slug) {
            autoOpenedRef.current = project.slug
            setActivePath(readme)
          }
        })
        .catch((error: unknown) => {
          if (error instanceof DOMException && error.name === 'AbortError') return
          setEntries([])
          setTreeState('error')
          setTreeError(error instanceof ApiError ? error.message : '代码目录加载失败')
        })
    }, fileSearch ? 250 : 0)
    return () => {
      window.clearTimeout(timer)
      controller.abort()
    }
  }, [fileSearch, project.slug])

  useEffect(() => {
    if (!activePath) {
      setActiveFile(null)
      setFileState('idle')
      return
    }
    const controller = new AbortController()
    setFileState('loading')
    getProjectCodeFile(project.slug, activePath, controller.signal)
      .then((response) => {
        setActiveFile(response.data)
        setFileState('ready')
      })
      .catch((error: unknown) => {
        if (error instanceof DOMException && error.name === 'AbortError') return
        setActiveFile(null)
        setFileState('error')
        setFileError(error instanceof ApiError ? error.message : '代码文件加载失败')
      })
    return () => controller.abort()
  }, [activePath, project.slug])

  const openFile = (path: string) => {
    setSelectedLine(null)
    setActivePath(path)
  }

  const copyContent = () => {
    if (!activeFile) return
    navigator.clipboard?.writeText(activeFile.content).then(onCopy).catch(() => {
      showToast('浏览器拒绝了剪贴板写入，请手动复制')
    })
  }

  const toggleWrap = () => setWrap((previous) => {
    const next = !previous
    try {
      window.localStorage.setItem(CODE_WRAP_STORAGE_KEY, next ? '1' : '0')
    } catch {
      /* 隐私模式下写入失败可忽略，仅本次会话不持久化。 */
    }
    return next
  })

  // 锚定某一行：高亮 + 写入 `#L{n}` hash（replaceState 不新增历史记录）。
  const anchorLine = (line: number) => {
    setSelectedLine(line)
    if (!activeFile) return
    const url = new URL(window.location.href)
    url.hash = buildCodeLineHash(activeFile.path, line)
    window.history.replaceState(window.history.state, '', url.toString())
  }

  const copyLineLink = (line: number) => {
    if (!activeFile) return
    anchorLine(line)
    const url = new URL(window.location.href)
    url.hash = buildCodeLineHash(activeFile.path, line)
    navigator.clipboard?.writeText(url.toString())
      .then(() => showToast('行链接已复制'))
      .catch(() => showToast('浏览器未允许复制，请手动复制地址栏链接'))
  }

  const sourceText = activeFile ? activeFile.content.replace(/\n$/, '') : ''
  const highlightedLines = useHighlightedLines(sourceText, activeFile?.language ?? '')
  const pathSegments = activeFile ? activeFile.path.split('/') : []
  const activeMeta = activeFile ? codeFileMeta(pathSegments[pathSegments.length - 1] ?? '') : null

  // 深链接：进入代码预览时若地址带 #L{n}，打开对应文件并标记待滚动。
  useEffect(() => {
    const { line, file } = parseCodeLineHash(window.location.hash)
    if (line == null) return
    if (file) {
      // 抢占 README 自动打开，避免目录加载完成后被覆盖。
      autoOpenedRef.current = project.slug
      setActivePath(file)
    }
    setSelectedLine(line)
    pendingScrollRef.current = true
    // 仅在挂载时读取一次初始 hash。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // 文件就绪后，把待滚动的锚定行滚到视图中央。
  useEffect(() => {
    if (fileState !== 'ready' || !pendingScrollRef.current || selectedLine == null) return
    pendingScrollRef.current = false
    const target = codeRef.current?.querySelector<HTMLElement>(`[data-line="${selectedLine}"]`)
    target?.scrollIntoView({ behavior: 'smooth', block: 'center' })
  }, [fileState, selectedLine, highlightedLines])

  // 键盘上下键在（已筛选的）文件列表间移动，仅当焦点位于代码预览区内时生效。
  useEffect(() => {
    const handleKey = (event: KeyboardEvent) => {
      if (event.key !== 'ArrowUp' && event.key !== 'ArrowDown') return
      if (!sectionRef.current?.contains(document.activeElement)) return
      const target = event.target as HTMLElement | null
      if (target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable)) return
      const files = entries.filter((entry) => !entry.dir)
      if (files.length === 0) return
      event.preventDefault()
      const current = files.findIndex((entry) => entry.path === activePath)
      const nextIndex = event.key === 'ArrowDown'
        ? (current < 0 ? 0 : Math.min(current + 1, files.length - 1))
        : (current < 0 ? 0 : Math.max(current - 1, 0))
      const next = files[nextIndex]
      if (next && next.path !== activePath) {
        keyboardNavRef.current = true
        openFile(next.path)
      }
    }
    window.addEventListener('keydown', handleKey)
    return () => window.removeEventListener('keydown', handleKey)
  }, [entries, activePath])

  // 选中文件时把树内条目滚入视野；键盘导航时同时移交焦点，方便连续按键。
  useEffect(() => {
    if (!activePath) return
    const active = treeRef.current?.querySelector<HTMLElement>('.file-item.active')
    if (!active) return
    active.scrollIntoView({ block: 'nearest' })
    if (keyboardNavRef.current) {
      keyboardNavRef.current = false
      active.focus()
    }
  }, [activePath, entries])

  return (
    <section className="code-view" ref={sectionRef}>
      <div className="file-tree" ref={treeRef}>
        <div className="sidebar-heading">
          <span>代码目录</span>
          {truncated && <span className="meta-label">已截断</span>}
        </div>
        <label className="file-search">
          <Search size={14} />
          <input
            value={fileSearch}
            onChange={(event) => setFileSearch(event.target.value)}
            placeholder="搜索文件名"
          />
        </label>
        {treeState === 'loading' && <div className="file-tree-empty">正在加载代码目录…</div>}
        {treeState === 'error' && <div className="file-tree-empty">{treeError}</div>}
        {treeState === 'ready' && entries.length === 0 && (
          <div className="file-tree-empty">{fileSearch.trim() ? '没有匹配的文件' : '代码包为空'}</div>
        )}
        {treeState === 'ready' && entries.map((entry) => {
          const meta = entry.dir ? null : codeFileMeta(entry.name)
          return (
            <button
              key={entry.path}
              className={`file-item ${entry.path === activePath ? 'active' : ''} ${entry.dir ? 'is-dir' : ''}`}
              style={{ paddingLeft: `${10 + entry.path.split('/').length * 8}px` }}
              disabled={entry.dir}
              title={meta ? `${entry.path} · ${meta.label}` : entry.path}
              onClick={() => { if (!entry.dir) openFile(entry.path) }}
            >
              <span>{entry.dir ? <ChevronRight size={14} /> : <span className={`ft-dot ft-${meta?.tone ?? 'other'}`} aria-hidden="true" />}</span>
              <span>{entry.name}{meta?.readme && <span className="file-readme-tag">README</span>}</span>
              {!entry.dir && <small>{formatFileSize(entry.size)}</small>}
            </button>
          )
        })}
        {project.repo && <div className="file-tree-note"><GitBranch size={16} /><span>{project.repo}</span></div>}
      </div>
      <div className="source-panel">
        <div className="source-head">
          <span className="code-toolbar-left">
            {activeFile ? (
              <nav className="code-breadcrumb" aria-label="文件路径">
                <button type="button" className="code-crumb-home" title="显示全部文件" aria-label="显示全部文件" onClick={() => setFileSearch('')}>
                  <HomeIcon size={13} />
                </button>
                {pathSegments.map((segment, index) => {
                  const isLast = index === pathSegments.length - 1
                  const dirPath = pathSegments.slice(0, index + 1).join('/')
                  return (
                    <span className="code-crumb-item" key={dirPath}>
                      <ChevronRight size={12} className="code-crumb-sep" aria-hidden="true" />
                      {isLast ? (
                        <span className="code-crumb is-leaf" title={activeFile.path}>{segment}</span>
                      ) : (
                        <button type="button" className="code-crumb is-link" title={`筛选 ${dirPath}/ 下的文件`} onClick={() => setFileSearch(dirPath)}>{segment}</button>
                      )}
                    </span>
                  )
                })}
              </nav>
            ) : '选择左侧文件查看源码'}
          </span>
          <span className="source-head-actions">
            {activeFile && activeMeta && (
              <span className="code-lang-chip" title={`文件类型 · ${activeMeta.label}`}>
                <span className={`ft-dot ft-${activeMeta.tone}`} aria-hidden="true" />{activeMeta.label}
              </span>
            )}
            {activeFile && <span className="code-file-size">{formatFileSize(activeFile.size)}</span>}
            <button
              type="button"
              className="tool-button code-wrap-toggle"
              aria-pressed={wrap}
              title={wrap ? '切换为横向滚动' : '切换为自动换行'}
              onClick={toggleWrap}
            >
              <WrapText size={14} /> {wrap ? '换行' : '不换行'}
            </button>
            {activeFile && (
              <a
                className="tool-button"
                href={getProjectCodeFileDownloadURL(project.slug, activeFile.path)}
              >
                <Download size={14} /> 下载此文件
              </a>
            )}
            <a className="tool-button" href={getProjectCodeArchiveURL(project.slug)}>
              <Download size={14} /> 下载代码包
            </a>
            <button className="tool-button" disabled={!activeFile} onClick={copyContent}>
              <Copy size={14} /> 复制代码
            </button>
          </span>
        </div>
        {fileState === 'loading' && <div className="source-placeholder">正在加载文件…</div>}
        {fileState === 'error' && <div className="source-placeholder">{fileError}</div>}
        {fileState === 'idle' && <div className="source-placeholder">该项目代码包已就绪，点击左侧文件开始阅读。</div>}
        {fileState === 'ready' && activeFile && (
          <>
            {activeFile.truncated && (
              <div className="source-notice">文件较大，仅显示前 512 KB，完整内容请下载代码包。</div>
            )}
            <div className={`code-reader ${wrap ? 'is-wrap' : ''}`} ref={codeRef}>
              {/* 行号独立成列，逐行渲染让锚点高亮铺满整行、换行时行号仍与首行对齐。 */}
              <ol className="code-reader-lines">
                {highlightedLines.map((lineHTML, index) => {
                  const lineNo = index + 1
                  return (
                    <li
                      key={index}
                      data-line={lineNo}
                      className={`code-reader-line ${selectedLine === lineNo ? 'is-active' : ''}`}
                    >
                      <span className="code-reader-gutter">
                        <button
                          type="button"
                          className="code-reader-no"
                          onClick={() => anchorLine(lineNo)}
                          title={`锚定第 ${lineNo} 行`}
                          aria-label={`锚定第 ${lineNo} 行`}
                        >
                          {lineNo}
                        </button>
                        <button
                          type="button"
                          className="code-reader-link"
                          onClick={() => copyLineLink(lineNo)}
                          title={`复制第 ${lineNo} 行链接`}
                          aria-label={`复制第 ${lineNo} 行链接`}
                        >
                          <Link2 size={12} />
                        </button>
                      </span>
                      {/* highlight.js 会转义输入，回退路径也做了转义，逐行注入不会产生可执行 HTML。 */}
                      <code
                        className="code-reader-code"
                        dangerouslySetInnerHTML={{ __html: lineHTML || '\u200b' }}
                      />
                    </li>
                  )
                })}
              </ol>
            </div>
          </>
        )}
      </div>
    </section>
  )
}

function DownloadView({ project }: { project: Project }) {
  const resourceURL = (kind: 'code' | 'document') => `/api/v1/projects/${encodeURIComponent(project.slug)}/resources/${kind}`
  const downloadControl = (kind: 'code' | 'document', label: string) => project.resources?.[kind]
    ? <a className="icon-button" title={label} aria-label={label} href={resourceURL(kind)}><Download size={16} /></a>
    : <span className="resource-missing" title={`作者尚未上传${label}，上传后即可下载`}>-</span>
  return <section className="download-view"><div className="download-intro"><span className="section-kicker">RELEASES</span><h2>选择一个资源开始。</h2><p>当前展示的是项目公开资源。下载记录会计入项目统计，资源由作者维护。</p>{project.resources?.code ? <a className="outline-button" href={resourceURL('code')}><Download size={15} /> 下载代码包</a> : <button className="outline-button" disabled title="作者尚未上传代码包"><Download size={15} /> 代码包未上传</button>}</div><div className="resource-list"><div className="resource-row"><div className="resource-icon"><Code2 size={18} /></div><div><strong>{project.slug}-v{project.currentVersion ?? 'latest'}</strong><small>代码包 · {project.license}</small></div><span>v{project.currentVersion ?? '-'}</span>{downloadControl('code', '下载代码包')}</div><div className="resource-row"><div className="resource-icon"><FileText size={18} /></div><div><strong>项目文档</strong><small>文档 · 作者维护</small></div><span>DOC</span>{downloadControl('document', '下载文档')}</div><div className="resource-row"><div className="resource-icon"><Play size={18} /></div><div><strong>产品演示</strong><small>Bilibili 外链 · 视频链接保存在文档中</small></div><span>VIDEO</span><span className="resource-missing" title="演示视频由作者在文档中嵌入">-</span></div></div></section>
}

function EmojiPicker({ onSelect }: { onSelect: (emoji: string) => void }) {
  const [open, setOpen] = useState(false)
  // 根据按钮下方剩余空间选择弹层方向，避免在底部评论框里被视窗遮挡。
  const [placement, setPlacement] = useState<'down' | 'up'>('down')
  const containerRef = useRef<HTMLSpanElement | null>(null)

  useEffect(() => {
    if (!open) return
    // 自行处理外部点击：emoji-mart 的 onClickOutside 会在任意文档点击时触发，
    // 与按钮的 toggle 互相干扰，导致关闭后无法再次打开。
    const handlePointerDown = (event: PointerEvent) => {
      const target = event.target
      if (target instanceof Node && containerRef.current?.contains(target)) return
      setOpen(false)
    }
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false)
    }
    document.addEventListener('pointerdown', handlePointerDown, true)
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('pointerdown', handlePointerDown, true)
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [open])

  const toggle = () => {
    setOpen((value) => {
      if (value) return false
      const rect = containerRef.current?.getBoundingClientRect()
      const spaceBelow = rect ? window.innerHeight - rect.bottom : 0
      setPlacement(spaceBelow < 380 ? 'up' : 'down')
      return true
    })
  }

  return <span className="emoji-picker" ref={containerRef}>
    <button type="button" title="插入 Emoji" onClick={toggle}>😊</button>
    {open && <span className={`emoji-popover placement-${placement}`}>
      <Suspense fallback={<span className="emoji-loading">正在加载 Emoji…</span>}>
        <EmojiMartPicker
          onSelect={(emoji) => {
            onSelect(emoji)
            setOpen(false)
          }}
        />
      </Suspense>
    </span>}
  </span>
}

function usePageScrollLock() {
  useEffect(() => {
    const body = document.body
    const documentElement = document.documentElement
    const scrollX = window.scrollX
    const scrollY = window.scrollY
    const previousOverflow = body.style.overflow
    const previousPaddingRight = body.style.paddingRight
    const previousPosition = body.style.position
    const previousTop = body.style.top
    const previousLeft = body.style.left
    const previousRight = body.style.right
    const previousWidth = body.style.width
    const previousDocumentOverflow = documentElement.style.overflow
    const scrollbarWidth = window.innerWidth - document.documentElement.clientWidth

    documentElement.style.overflow = 'hidden'
    body.style.overflow = 'hidden'
    body.style.position = 'fixed'
    body.style.top = `${-scrollY}px`
    body.style.left = `${-scrollX}px`
    body.style.right = '0'
    body.style.width = '100%'
    if (scrollbarWidth > 0) body.style.paddingRight = `${scrollbarWidth}px`

    return () => {
      documentElement.style.overflow = previousDocumentOverflow
      body.style.overflow = previousOverflow
      body.style.paddingRight = previousPaddingRight
      body.style.position = previousPosition
      body.style.top = previousTop
      body.style.left = previousLeft
      body.style.right = previousRight
      body.style.width = previousWidth
      window.scrollTo(scrollX, scrollY)
    }
  }, [])
}

function encodedObjectKey(value: string) {
  const bytes = new TextEncoder().encode(value)
  let binary = ''
  bytes.forEach((byte) => { binary += String.fromCharCode(byte) })
  return window.btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

function markdownAssetURL(url: string | undefined, authorPreview: boolean, projectSlug?: string) {
  if (!url?.startsWith('oss://')) return url
  const key = encodedObjectKey(url.slice(6))
  return authorPreview
    ? `/api/v1/files/author-asset?key=${encodeURIComponent(key)}`
    : `/api/v1/projects/${encodeURIComponent(projectSlug ?? '')}/assets?key=${encodeURIComponent(key)}`
}

function MarkdownCanvas({ markdown, authorPreview = false, projectSlug }: { markdown: string; authorPreview?: boolean; projectSlug?: string }) {
  return <div className="markdown-canvas">
    {markdown.trim() ? <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      rehypePlugins={[rehypeHighlight]}
      urlTransform={(url) => url.startsWith('oss://') ? url : defaultUrlTransform(url)}
      components={{
        code: ({ children, className }) => className === 'language-mermaid'
          ? <MermaidDiagram source={String(children).trim()} />
          : <code className={className}>{children}</code>,
        img: ({ src, alt }) => <img src={markdownAssetURL(src, authorPreview, projectSlug)} alt={alt ?? ''} loading="lazy" />,
        a: ({ href, children }) => <a href={markdownAssetURL(href, authorPreview, projectSlug)} target="_blank" rel="noreferrer">{children}</a>,
      }}
    >{markdown}</ReactMarkdown> : <div className="markdown-empty">开始输入 Markdown，预览会实时显示在这里。</div>}
  </div>
}

const emptyManagedProject: ManagedProjectInput = {
  slug: '', name: '', summary: '', description: '', category: '',
  tags: [], tech_stack: [], license: 'MIT', repository_url: '',
  cover_object_key: '', document_object_key: '', code_object_key: '', current_version: '0.1.0',
}

// 统计正文的字数（中文字符 + 英文/数字词）、字符数与预计阅读时长（约 300 字/分钟）。
function analyzeEditorContent(text: string) {
  const cjk = (text.match(/[㐀-䶿一-鿿豈-﫿]/g) ?? []).length
  const words = (text.match(/[A-Za-z0-9]+/g) ?? []).length
  const wordCount = cjk + words
  const charCount = Array.from(text).length
  const readingMinutes = wordCount === 0 ? 0 : Math.max(1, Math.round(wordCount / 300))
  return { wordCount, charCount, readingMinutes }
}

function extractEditorOutline(markdown: string) {
  let inFence = false
  return markdown.split('\n').flatMap((line, lineIndex) => {
    if (/^\s*```/.test(line)) {
      inFence = !inFence
      return []
    }
    if (inFence) return []
    const match = /^(#{1,3})\s+(.+?)\s*#*\s*$/.exec(line)
    return match ? [{ level: match[1].length, title: match[2], lineIndex }] : []
  })
}

// 把保存时间格式化成中文相对时间，用于“已保存 · …”徽标。
function formatSavedRelative(timestamp: number, now: number) {
  const seconds = Math.max(0, Math.floor((now - timestamp) / 1000))
  if (seconds < 60) return '刚刚'
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes} 分钟前`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours} 小时前`
  return `${Math.floor(hours / 24)} 天前`
}

function AuthorProjectCenter({ onClose, onChanged }: { onClose: () => void; onChanged: () => void }) {
  usePageScrollLock()
  const [projects, setProjects] = useState<ManagedProject[]>([])
  const [metricsByProject, setMetricsByProject] = useState<Record<string, ProjectMetrics>>({})
  const [authorComments, setAuthorComments] = useState<AuthorCommentItem[]>([])
  const [authorCommentsLoading, setAuthorCommentsLoading] = useState(false)
  const [authorCommentsError, setAuthorCommentsError] = useState('')
  const [input, setInput] = useState<ManagedProjectInput>(emptyManagedProject)
  const [activeProject, setActiveProject] = useState<ManagedProject | null>(null)
  const [tagsText, setTagsText] = useState('')
  const [stackText, setStackText] = useState('')
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [uploading, setUploading] = useState<string | null>(null)
  const [editorMode, setEditorMode] = useState<'rich' | 'write' | 'split' | 'preview'>('rich')
  const [saveState, setSaveState] = useState<'idle' | 'saving' | 'saved'>('idle')
  const [error, setError] = useState('')
  // 选中具体文档时编辑区切到该文档正文；为空时编辑项目自身正文。
  const [activeDocument, setActiveDocument] = useState<ProjectDocument | null>(null)
  const [documentDraft, setDocumentDraft] = useState('')
  const [documentSaveState, setDocumentSaveState] = useState<'idle' | 'saving' | 'saved'>('idle')
  // 保存成功后递增，通知文档树重拉数据。
  const [documentTreeToken, setDocumentTreeToken] = useState(0)
  const [authorToast, setAuthorToast] = useState('')
  // 历史版本面板只在编辑具体文档时可用，项目正文没有独立的修订历史。
  const [revisionPanelOpen, setRevisionPanelOpen] = useState(false)
  const editorRef = useRef<HTMLTextAreaElement | null>(null)
  const editorPageRef = useRef<HTMLFormElement | null>(null)
  const richEditorRef = useRef<RichMarkdownEditorHandle | null>(null)
  // 记录已载入草稿的文档 id，用于区分“切换文档”和“保存后回写”。
  const loadedDocumentID = useRef('')
  // 最近一次保存成功的时间戳，配合 nowTick 刷新相对时间徽标。
  const [savedAt, setSavedAt] = useState<number | null>(null)
  const [nowTick, setNowTick] = useState(() => Date.now())
  // Markdown 源码 textarea 的撤销/重做历史（富文本模式沿用编辑器自带撤销）。
  const undoStack = useRef<string[]>([])
  const redoStack = useRef<string[]>([])
  const lastRecordRef = useRef<{ key: string; time: number }>({ key: '', time: 0 })
  const [undoCount, setUndoCount] = useState(0)
  const [redoCount, setRedoCount] = useState(0)

  // 当前编辑目标：选中文档时编辑文档正文，否则编辑项目正文。
  const editingKey = activeDocument?.id ?? activeProject?.id ?? 'new-project'
  const sourceValue = activeDocument ? documentDraft : input.description
  const editorOutline = useMemo(() => extractEditorOutline(sourceValue), [sourceValue])

  const jumpToOutlineHeading = (headingIndex: number, lineIndex: number) => {
    if (editorMode === 'write' || editorMode === 'split') {
      const offset = sourceValue.split('\n').slice(0, lineIndex).reduce((total, line) => total + line.length + 1, 0)
      editorRef.current?.focus()
      editorRef.current?.setSelectionRange(offset, offset)
      const lineHeight = 23.4
      if (editorRef.current) editorRef.current.scrollTop = Math.max(0, lineIndex * lineHeight - 80)
      return
    }
    const headings = editorPageRef.current?.querySelectorAll('.ProseMirror h1, .ProseMirror h2, .ProseMirror h3, .markdown-canvas h1, .markdown-canvas h2, .markdown-canvas h3')
    headings?.[headingIndex]?.scrollIntoView({ behavior: 'smooth', block: 'center' })
  }
  const applySourceValue = (value: string) => {
    if (activeDocument) setDocumentDraft(value.slice(0, 200000))
    else setInput((current) => ({ ...current, description: value.slice(0, 50000) }))
  }
  const contentStats = useMemo(() => analyzeEditorContent(sourceValue), [sourceValue])

  const showAuthorToast = (message: string) => {
    setAuthorToast(message)
    window.setTimeout(() => setAuthorToast(''), 2400)
  }

  // 仅在真正切换文档时重新载入草稿。
  // 保存成功后服务端返回的正文也会更新 activeDocument，
  // 用 ref 比对文档 id 可避免那次更新把 saved 状态覆盖回 idle。
  useEffect(() => {
    const documentID = activeDocument?.id ?? ''
    if (loadedDocumentID.current === documentID) return
    loadedDocumentID.current = documentID
    setDocumentDraft(activeDocument?.markdown ?? '')
    setDocumentSaveState('idle')
    setSavedAt(null)
  }, [activeDocument])

  // 文档正文自动保存，与项目草稿保存互不影响。
  useEffect(() => {
    if (!activeDocument || !activeProject) return
    if (documentDraft === activeDocument.markdown) return
    setDocumentSaveState('saving')
    const timer = window.setTimeout(() => {
      updateAuthorProjectDocument(activeProject.id, activeDocument.id, {
        parent_id: activeDocument.parent_id,
        slug: activeDocument.slug,
        title: activeDocument.title,
        markdown: documentDraft,
      })
        .then((response) => {
          setActiveDocument(response.data)
          setDocumentSaveState('saved')
          setSavedAt(Date.now())
          setDocumentTreeToken((current) => current + 1)
        })
        .catch((reason: unknown) => {
          setDocumentSaveState('idle')
          showAuthorToast(reason instanceof ApiError ? reason.message : '文档保存失败')
        })
    }, 1200)
    return () => window.clearTimeout(timer)
  }, [activeDocument, activeProject, documentDraft])

  // 每 30s 刷新一次，用于“已保存 · N 分钟前”的相对时间显示。
  useEffect(() => {
    const timer = window.setInterval(() => setNowTick(Date.now()), 30000)
    return () => window.clearInterval(timer)
  }, [])

  // 切换编辑目标（文档/项目）时清空撤销历史，避免跨文档撤销。
  useEffect(() => {
    undoStack.current = []
    redoStack.current = []
    lastRecordRef.current = { key: editingKey, time: 0 }
    setUndoCount(0)
    setRedoCount(0)
  }, [editingKey])

  const loadProjects = () => {
    setLoading(true)
    getAuthorProjects()
      .then(async (response) => {
        setProjects(response.data)
        const entries = await Promise.all(response.data.map(async (project) => {
          try {
            const metrics = await getAuthorProjectMetrics(project.id)
            return [project.id, metrics.data] as const
          } catch {
            return null
          }
        }))
        setMetricsByProject(Object.fromEntries(
          entries.filter((entry): entry is readonly [string, ProjectMetrics] => entry !== null),
        ))
      })
      .catch((reason: unknown) => setError(reason instanceof Error ? reason.message : '项目加载失败'))
      .finally(() => setLoading(false))
  }

  useEffect(loadProjects, [])

  useEffect(() => {
    if (!activeProject) {
      setAuthorComments([])
      return
    }
    const controller = new AbortController()
    setAuthorCommentsLoading(true)
    setAuthorCommentsError('')
    getAuthorProjectComments(activeProject.id, controller.signal)
      .then((response) => setAuthorComments(response.data))
      .catch((reason: unknown) => {
        if (reason instanceof DOMException && reason.name === 'AbortError') return
        setAuthorCommentsError(reason instanceof ApiError ? reason.message : '评论加载失败')
      })
      .finally(() => setAuthorCommentsLoading(false))
    return () => controller.abort()
  }, [activeProject?.id])

  const projectPayload = useMemo<ManagedProjectInput>(() => ({
    ...input,
    tags: tagsText.split(',').map((value) => value.trim()).filter(Boolean),
    tech_stack: stackText.split(',').map((value) => value.trim()).filter(Boolean),
  }), [input, tagsText, stackText])
  const activeProjectID = activeProject?.id
  const activeProjectStatus = activeProject?.status

  useEffect(() => {
    if (!activeProjectID || (activeProjectStatus !== 'draft' && activeProjectStatus !== 'rejected')) return
    if (projectPayload.name.trim().length < 2 || projectPayload.slug.length < 1 || projectPayload.summary.trim().length < 10 ||
      projectPayload.description.trim().length < 20 || !projectPayload.category.trim() || !projectPayload.license.trim() || !projectPayload.current_version.trim()) return
    setSaveState('saving')
    const timer = window.setTimeout(() => {
      updateAuthorProject(activeProjectID, projectPayload)
        .then((response) => {
          setActiveProject(response.data)
          setProjects((current) => current.map((project) => project.id === response.data.id ? response.data : project))
          setSaveState('saved')
          setSavedAt(Date.now())
        })
        .catch((reason: unknown) => {
          setSaveState('idle')
          setError(reason instanceof Error ? reason.message : '自动保存失败')
        })
    }, 1200)
    return () => window.clearTimeout(timer)
  }, [activeProjectID, activeProjectStatus, projectPayload])

  const update = (field: keyof ManagedProjectInput, value: string) => {
    setInput((current) => ({ ...current, [field]: value }))
  }

  const saveProject = async (event: FormEvent) => {
    event.preventDefault()
    setSubmitting(true)
    setError('')
    try {
      const response = activeProject
        ? await updateAuthorProject(activeProject.id, projectPayload)
        : await createAuthorProject(projectPayload)
      setActiveProject(response.data)
      setSaveState('saved')
      setSavedAt(Date.now())
      loadProjects()
      onChanged()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '草稿保存失败')
    } finally {
      setSubmitting(false)
    }
  }

  const submitProject = async (projectID: string) => {
    setSubmitting(true)
    setError('')
    try {
      await submitAuthorProject(projectID)
      if (activeProject?.id === projectID) setActiveProject((current) => current ? { ...current, status: 'pending_review' } : null)
      loadProjects()
      onChanged()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '提交审核失败')
    } finally {
      setSubmitting(false)
    }
  }

  const selectProject = (project: ManagedProject) => {
    setActiveProject(project)
    setInput({
      slug: project.slug, name: project.name, summary: project.summary,
      description: project.description, category: project.category, tags: project.tags,
      tech_stack: project.tech_stack, license: project.license,
      repository_url: project.repository_url, cover_object_key: project.cover_object_key,
      document_object_key: project.document_object_key, code_object_key: project.code_object_key,
      current_version: project.current_version,
    })
    setTagsText(project.tags.join(', '))
    setStackText(project.tech_stack.join(', '))
    setSaveState('idle')
    setSavedAt(null)
    setError('')
  }

  const newProject = () => {
    setActiveProject(null)
    setInput({ ...emptyManagedProject })
    setTagsText('')
    setStackText('')
    setSaveState('idle')
    setSavedAt(null)
    setError('')
  }

  // 记录一次撤销快照。coalesce=true 时把连续输入合并成一步（间隔 <500ms 只记第一次）。
  const recordHistory = (previous: string, coalesce = true) => {
    const now = Date.now()
    const last = lastRecordRef.current
    if (coalesce && last.key === editingKey && undoStack.current.length > 0 && now - last.time < 500) {
      lastRecordRef.current = { key: editingKey, time: now }
      return
    }
    undoStack.current.push(previous)
    if (undoStack.current.length > 200) undoStack.current.shift()
    redoStack.current = []
    lastRecordRef.current = { key: editingKey, time: now }
    setUndoCount(undoStack.current.length)
    setRedoCount(0)
  }

  const restoreCaretToEnd = (value: string) => {
    window.requestAnimationFrame(() => {
      const textarea = editorRef.current
      if (!textarea) return
      textarea.focus()
      textarea.setSelectionRange(value.length, value.length)
    })
  }

  const undoSource = () => {
    if (undoStack.current.length === 0) return
    const previous = undoStack.current.pop() as string
    redoStack.current.push(sourceValue)
    lastRecordRef.current = { key: editingKey, time: 0 }
    applySourceValue(previous)
    setUndoCount(undoStack.current.length)
    setRedoCount(redoStack.current.length)
    restoreCaretToEnd(previous)
  }

  const redoSource = () => {
    if (redoStack.current.length === 0) return
    const nextValue = redoStack.current.pop() as string
    undoStack.current.push(sourceValue)
    lastRecordRef.current = { key: editingKey, time: 0 }
    applySourceValue(nextValue)
    setUndoCount(undoStack.current.length)
    setRedoCount(redoStack.current.length)
    restoreCaretToEnd(nextValue)
  }

  const handleSourceChange = (value: string) => {
    recordHistory(sourceValue)
    applySourceValue(value)
  }

  const handleSourceKeyDown = (event: ReactKeyboardEvent<HTMLTextAreaElement>) => {
    if (!(event.metaKey || event.ctrlKey)) return
    const key = event.key.toLowerCase()
    if (key === 'z' && !event.shiftKey) {
      event.preventDefault()
      undoSource()
    } else if (key === 'y' || (key === 'z' && event.shiftKey)) {
      event.preventDefault()
      redoSource()
    }
  }

  const insertMarkdown = (before: string, after = '', placeholder = '') => {
    const markdown = before + placeholder + after
    if (editorMode === 'rich') {
      richEditorRef.current?.insertMarkdown(markdown)
      return
    }
    const textarea = editorRef.current
    if (!textarea) {
      recordHistory(sourceValue, false)
      applySourceValue(sourceValue + markdown)
      return
    }
    const start = textarea.selectionStart
    const end = textarea.selectionEnd
    const selected = sourceValue.slice(start, end) || placeholder
    const next = sourceValue.slice(0, start) + before + selected + after + sourceValue.slice(end)
    recordHistory(sourceValue, false)
    applySourceValue(next)
    window.requestAnimationFrame(() => {
      textarea.focus()
      textarea.setSelectionRange(start + before.length, start + before.length + selected.length)
    })
  }

  const uploadInline = async (file: File, kind: 'image' | 'document' | 'code') => {
    setUploading(`inline-${kind}`)
    setError('')
    try {
      const objectKey = await uploadProjectFile(file, kind)
      const markdown = kind === 'image'
        ? `\n![${file.name}](oss://${objectKey})\n`
        : `\n[📎 ${file.name}](oss://${objectKey})\n`
      insertMarkdown(markdown)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '附件上传失败')
    } finally {
      setUploading(null)
    }
  }

  const uploadRichImage = async (file: File) => {
    setUploading('inline-image')
    setError('')
    try {
      const objectKey = await uploadProjectFile(file, 'image')
      return `oss://${objectKey}`
    } catch (reason) {
      const message = reason instanceof Error ? reason.message : '图片上传失败'
      setError(message)
      throw reason
    } finally {
      setUploading(null)
    }
  }

  const uploadRichFile = async (file: File) => {
    const kind = /\.(zip|tar|gz|tgz)$/i.test(file.name) ? 'code' : 'document'
    setUploading(`inline-${kind}`)
    setError('')
    try {
      const objectKey = await uploadProjectFile(file, kind)
      return `oss://${objectKey}`
    } catch (reason) {
      const message = reason instanceof Error ? reason.message : '附件上传失败'
      setError(message)
      throw reason
    } finally {
      setUploading(null)
    }
  }

  const uploadFile = async (file: File, kind: 'image' | 'document' | 'code', field: 'cover_object_key' | 'document_object_key' | 'code_object_key') => {
    setUploading(kind)
    setError('')
    try {
      const objectKey = await uploadProjectFile(file, kind)
      setInput((current) => ({ ...current, [field]: objectKey }))
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '文件上传失败')
    } finally {
      setUploading(null)
    }
  }

  const statusLabel = (status: ManagedProject['status']) => ({
    draft: '草稿', pending_review: '待审核', published: '已发布',
    rejected: '已驳回', archived: '已下架',
  })[status]

  // 保存状态徽标：未保存改动 / 保存中… / 已保存(相对时间) / 草稿。
  const currentSaveState = activeDocument ? documentSaveState : saveState
  const isDirty = activeDocument
    ? documentDraft !== (activeDocument.markdown ?? '')
    : activeProject
      ? input.description !== (activeProject.description ?? '')
      : input.description.trim().length > 0
  const saveBadge = currentSaveState === 'saving'
    ? { key: 'saving', label: '保存中…' }
    : isDirty
      ? { key: 'dirty', label: '未保存改动' }
      : savedAt !== null
        ? { key: 'saved', label: `已保存 · ${formatSavedRelative(savedAt, nowTick)}` }
        : currentSaveState === 'saved'
          ? { key: 'saved', label: '已保存' }
          : { key: 'idle', label: '草稿' }
  const historyEnabled = editorMode === 'write' || editorMode === 'split'

  return <div className="author-editor-page">
    <section className="author-center document-author-center" aria-label="作者项目中心">
      <button className="icon-button modal-close" onClick={onClose} aria-label="关闭"><X size={17} /></button>
      {authorToast && <div className="author-toast"><Check size={14} /> {authorToast}</div>}
      <header className="author-editor-head"><div><span className="section-kicker">AUTHOR / DOCUMENT</span><h2>{activeDocument ? activeDocument.title : activeProject ? activeProject.name : '创建项目文档'}</h2><p>{activeDocument ? `正在编辑文档：${activeDocument.slug}` : '像在线文档一样使用 Markdown、图表、图片和附件组织项目内容。'}</p></div><div className="save-indicator">{activeDocument ? (documentSaveState === 'saving' ? '正在保存文档…' : documentSaveState === 'saved' ? '文档已保存' : '文档') : saveState === 'saving' ? '正在自动保存…' : saveState === 'saved' ? '已自动保存' : '草稿'}</div></header>
      <div className="document-editor-layout">
        <aside className="author-project-rail">
          <button className="primary-button" onClick={newProject}>＋ 新建项目</button>
          <nav className="editor-outline" aria-label="文章目录">
            <h3>文章目录</h3>
            {editorOutline.length ? editorOutline.map((heading, index) => (
              <button key={`${heading.lineIndex}-${heading.title}`} type="button" className={`outline-level-${heading.level}`} onClick={() => jumpToOutlineHeading(index, heading.lineIndex)} title={heading.title}>
                {heading.title}
              </button>
            )) : <p className="empty-copy">添加标题后，目录会自动生成。</p>}
          </nav>
          <h3>我的项目</h3>
          {loading ? <p>正在加载…</p> : projects.length === 0 ? <p className="empty-copy">还没有项目草稿。</p> : projects.map((project) =>
            <button className={activeProject?.id === project.id ? 'active' : ''} key={project.id} onClick={() => selectProject(project)}>
              <strong>{project.name}</strong><small>{statusLabel(project.status)} · v{project.current_version}{metricsByProject[project.id] ? ` · 浏览 ${metricsByProject[project.id].views} · 下载 ${metricsByProject[project.id].downloads}` : ''}</small>
            </button>)}
          {activeProject && <Suspense fallback={<p>正在加载文档目录…</p>}>
            <ProjectDocumentTree
              projectID={activeProject.id}
              selectedDocumentID={activeDocument?.id ?? ''}
              refreshToken={documentTreeToken}
              onSelect={setActiveDocument}
              showToast={showAuthorToast}
            />
          </Suspense>}
          {activeProject && (
            <div className="author-comments">
              <h3>项目评论 {authorComments.length > 0 ? `(${authorComments.length})` : ''}</h3>
              {authorCommentsLoading ? <p className="empty-copy">正在加载评论…</p>
                : authorCommentsError ? <p className="empty-copy">{authorCommentsError}</p>
                  : authorComments.length === 0 ? <p className="empty-copy">暂无评论。</p>
                    : <div className="author-comment-list">
                      {authorComments.map((item) => (
                        <div className="author-comment" key={item.id}>
                          <span className="author-comment-meta">
                            <strong>{item.document_title || item.document_id}</strong>
                            <span>{item.author_name} · {new Date(item.created_at).toLocaleString('zh-CN')}</span>
                          </span>
                          <p>{item.quote && <em>“{item.quote}”</em>}{item.body}</p>
                          <span className={`author-comment-status ${item.status === 'resolved' ? 'resolved' : ''}`}>{item.status === 'resolved' ? '已解决' : '未解决'}</span>
                        </div>
                      ))}
                    </div>}
            </div>
          )}
        </aside>
        <form ref={editorPageRef} className="document-project-editor" onSubmit={(event) => void saveProject(event)}>
          <div className="document-title-row">
            {activeDocument ? (
              <div className="document-context-banner">
                当前编辑文档《{activeDocument.title}》
                <button type="button" onClick={() => setActiveDocument(null)}>回到项目正文</button>
              </div>
            ) : (
              <input className="document-title-input" required minLength={2} maxLength={120} value={input.name} onChange={(event) => update('name', event.target.value)} placeholder="无标题项目" />
            )}
            <div className="editor-mode-switch">
              <button type="button" className={editorMode === 'rich' ? 'active' : ''} onClick={() => setEditorMode('rich')}>富文本</button>
              <button type="button" className={editorMode === 'write' ? 'active' : ''} onClick={() => setEditorMode('write')}>Markdown</button>
              <button type="button" className={editorMode === 'split' ? 'active' : ''} onClick={() => setEditorMode('split')}>分屏</button>
              <button type="button" className={editorMode === 'preview' ? 'active' : ''} onClick={() => setEditorMode('preview')}>预览</button>
            </div>
          </div>
          {!activeDocument && <input className="document-summary-input" required minLength={10} maxLength={300} value={input.summary} onChange={(event) => update('summary', event.target.value)} placeholder="用一句话介绍这个项目…" />}
          <div className="markdown-toolbar">
            <button type="button" title="撤销 (Ctrl/⌘+Z)" aria-label="撤销" disabled={!historyEnabled || undoCount === 0} onClick={undoSource}>↶</button>
            <button type="button" title="重做 (Ctrl/⌘+Shift+Z)" aria-label="重做" disabled={!historyEnabled || redoCount === 0} onClick={redoSource}>↷</button>
            <span className="toolbar-divider" aria-hidden="true" />
            <button type="button" title="粗体" aria-label="粗体" onClick={() => insertMarkdown('**', '**', '粗体')}>B</button>
            <button type="button" title="斜体" aria-label="斜体" onClick={() => insertMarkdown('*', '*', '斜体')}><em>I</em></button>
            <button type="button" title="行内代码" aria-label="行内代码" onClick={() => insertMarkdown('`', '`', '代码')}>{'`码`'}</button>
            <span className="toolbar-divider" aria-hidden="true" />
            <button type="button" title="小标题" aria-label="小标题" onClick={() => insertMarkdown('\n## ', '\n', '小标题')}>H2</button>
            <button type="button" title="无序列表" aria-label="无序列表" onClick={() => insertMarkdown('\n- ', '\n', '列表项')}>• 列表</button>
            <button type="button" title="任务列表" aria-label="任务列表" onClick={() => insertMarkdown('\n- [ ] ', '\n', '待办事项')}>☑ 任务</button>
            <button type="button" title="引用" aria-label="引用" onClick={() => insertMarkdown('\n> ', '\n', '引用内容')}>❝ 引用</button>
            <button type="button" title="插入分割线" aria-label="插入分割线" onClick={() => insertMarkdown('\n\n---\n\n')}>分割线</button>
            <span className="toolbar-divider" aria-hidden="true" />
            <button type="button" title="链接" aria-label="链接" onClick={() => insertMarkdown('[', '](https://)', '链接文字')}>🔗 链接</button>
            <button type="button" title="表格" aria-label="表格" onClick={() => insertMarkdown('\n| 列 1 | 列 2 |\n| --- | --- |\n| 内容 | 内容 |\n')}>▦ 表格</button>
            <button type="button" title="代码块" aria-label="代码块" onClick={() => insertMarkdown('\n```text\n', '\n```\n', '代码')}>{'</>'}</button>
            <button type="button" title="图表" aria-label="图表" onClick={() => insertMarkdown('\n```mermaid\ngraph TD\n  A[开始] --> B[完成]\n```\n')}>图表</button>
            <span className="toolbar-divider" aria-hidden="true" />
            <label className="toolbar-upload">图片<input type="file" accept="image/jpeg,image/png,image/webp,image/gif" onChange={(event) => { const file = event.target.files?.[0]; if (file) void uploadInline(file, 'image') }} /></label>
            <label className="toolbar-upload">附件<input type="file" accept=".pdf,.md,.txt,.zip,.tar,.gz,.tgz" onChange={(event) => { const file = event.target.files?.[0]; if (file) void uploadInline(file, /\.(zip|tar|gz|tgz)$/i.test(file.name) ? 'code' : 'document') }} /></label>
            <EmojiPicker onSelect={(emoji) => insertMarkdown(emoji)} />
            {uploading?.startsWith('inline-') && <span className="toolbar-upload-hint">上传中…</span>}
            <div className="editor-toolbar-status">
              <span className="editor-word-count" aria-label={`字数 ${contentStats.wordCount}，字符 ${contentStats.charCount}`}>
                {contentStats.wordCount} 字<span className="editor-word-count-extra"> · {contentStats.charCount} 字符{contentStats.readingMinutes > 0 ? ` · 约 ${contentStats.readingMinutes} 分钟` : ''}</span>
              </span>
              <span className={`editor-save-badge is-${saveBadge.key}`} role="status" aria-live="polite">
                <span className="editor-save-dot" aria-hidden="true" />{saveBadge.label}
              </span>
              <button
                type="button"
                className="editor-history-button"
                disabled={!activeDocument}
                title={activeDocument ? '查看历史版本并回滚' : '选择一篇文档后可查看历史版本'}
                onClick={() => setRevisionPanelOpen(true)}
              >
                <History size={13} /> 历史版本{activeDocument ? ` · v${activeDocument.version}` : ''}
              </button>
            </div>
          </div>
          <div className={`markdown-workspace mode-${editorMode}`}>
            {editorMode === 'rich' && <Suspense fallback={<div className="rich-editor-loading">正在加载富文本编辑器…</div>}><RichMarkdownEditor ref={richEditorRef} documentKey={activeDocument?.id ?? activeProject?.id ?? 'new-project'} value={activeDocument ? documentDraft : input.description} onChange={(markdown) => activeDocument ? setDocumentDraft(markdown.slice(0, 200000)) : update('description', markdown.slice(0, 50000))} onUploadImage={uploadRichImage} onUploadFile={uploadRichFile} onNotify={showAuthorToast} /></Suspense>}
            {(editorMode === 'write' || editorMode === 'split') && <textarea ref={editorRef} className="markdown-source" required={!activeDocument} minLength={activeDocument ? 0 : 20} maxLength={activeDocument ? 200000 : 50000} value={activeDocument ? documentDraft : input.description} onChange={(event) => handleSourceChange(event.target.value)} onKeyDown={handleSourceKeyDown} placeholder={'# 项目介绍\n\n从这里开始，用 Markdown 编写你的项目文档…'} />}
            {(editorMode === 'preview' || editorMode === 'split') && <MarkdownCanvas markdown={activeDocument ? documentDraft : input.description} authorPreview />}
          </div>
          <details className="project-metadata">
            <summary>项目设置与发布资源</summary>
          <div className="form-grid">
            <label>项目标识<input required pattern="[a-z0-9]+(?:-[a-z0-9]+)*" placeholder="my-project" value={input.slug} onChange={(event) => update('slug', event.target.value.toLowerCase())} /></label>
            <label>分类<input required maxLength={80} value={input.category} onChange={(event) => update('category', event.target.value)} /></label>
            <label>当前版本<input required maxLength={40} value={input.current_version} onChange={(event) => update('current_version', event.target.value)} /></label>
            <label>许可证<input required maxLength={40} value={input.license} onChange={(event) => update('license', event.target.value)} /></label>
            <label>仓库地址<input type="url" maxLength={500} value={input.repository_url} onChange={(event) => update('repository_url', event.target.value)} /></label>
            <label>标签（逗号分隔）<input value={tagsText} onChange={(event) => setTagsText(event.target.value)} /></label>
            <label>技术栈（逗号分隔）<input value={stackText} onChange={(event) => setStackText(event.target.value)} /></label>
          </div>
          <div className="form-grid project-files">
            <label>封面图<input type="file" accept="image/jpeg,image/png,image/webp,image/gif" onChange={(event) => { const file = event.target.files?.[0]; if (file) void uploadFile(file, 'image', 'cover_object_key') }} /><small>{input.cover_object_key ? '已上传' : uploading === 'image' ? '上传中…' : '最大 10 MB'}</small></label>
            <label>项目文档<input type="file" accept=".pdf,.md,.txt" onChange={(event) => { const file = event.target.files?.[0]; if (file) void uploadFile(file, 'document', 'document_object_key') }} /><small>{input.document_object_key ? '已上传' : uploading === 'document' ? '上传中…' : '最大 50 MB'}</small></label>
            <label>代码包<input type="file" accept=".zip,.tar,.gz,.tgz" onChange={(event) => { const file = event.target.files?.[0]; if (file) void uploadFile(file, 'code', 'code_object_key') }} /><small>{input.code_object_key ? '已上传' : uploading === 'code' ? '上传中…' : '最大 500 MB'}</small></label>
          </div>
          </details>
          {error && <div className="auth-error">{error}</div>}
          <div className="document-editor-actions">
            <button className="outline-button" type="submit" disabled={submitting || uploading !== null}>{submitting ? '保存中…' : '保存草稿'}</button>
            {activeProject && (activeProject.status === 'draft' || activeProject.status === 'rejected') && <button className="primary-button" type="button" disabled={submitting || saveState === 'saving'} onClick={() => void submitProject(activeProject.id)}>提交审核</button>}
          </div>
        </form>
      </div>
    </section>
    {revisionPanelOpen && activeDocument && activeProject && (
      <Suspense fallback={null}>
        <DocumentRevisionPanel
          projectID={activeProject.id}
          document={activeDocument}
          currentMarkdown={documentDraft}
          onClose={() => setRevisionPanelOpen(false)}
          onRestored={(restored) => {
            // 回滚后端已落库，这里把编辑器草稿一起换成回滚结果，
            // 否则自动保存会立刻用旧草稿把回滚覆盖回去。
            loadedDocumentID.current = restored.id
            setActiveDocument(restored)
            setDocumentDraft(restored.markdown)
            setDocumentSaveState('saved')
            setSavedAt(Date.now())
            setDocumentTreeToken((current) => current + 1)
          }}
          onNotify={showAuthorToast}
        />
      </Suspense>
    )}
  </div>
}

function ThemePanel({ themeMode, skin, onModeChange, onSkinChange }: { themeMode: ThemeMode; skin: Skin; onModeChange: (mode: ThemeMode) => void; onSkinChange: (nextSkin: Skin) => void }) {
  const modes: { id: ThemeMode; label: string; icon: ReactNode }[] = [
    { id: 'light', label: '浅色', icon: <Sun size={15} /> },
    { id: 'dark', label: '深色', icon: <Moon size={15} /> },
    { id: 'system', label: '跟随系统', icon: <MonitorCog size={15} /> },
  ]
  return (
    <div className="theme-popover" onMouseDown={(event) => event.stopPropagation()}>
      <div className="theme-popover-heading"><span><Palette size={15} /> 外观</span><small>偏好设置</small></div>
      <div className="theme-mode-list">
        {modes.map((mode) => <button key={mode.id} className={themeMode === mode.id ? 'selected' : ''} onClick={() => onModeChange(mode.id)}>{mode.icon}<span>{mode.label}</span>{themeMode === mode.id && <Check size={14} />}</button>)}
      </div>
      <div className="theme-popover-divider" />
      <span className="theme-section-label">主题</span>
      <div className="skin-list">
        {themes.map((item) => (
          <button
            key={item.id}
            className={`skin-option${skin === item.id ? ' selected' : ''}`}
            aria-pressed={skin === item.id}
            onClick={() => onSkinChange(item.id)}
          >
            <span className="skin-swatch" style={{ background: item.swatch }} />
            <span className="skin-option-text">
              <strong>{item.label}</strong>
              <small>{item.description}</small>
            </span>
            {skin === item.id && <Check size={14} />}
          </button>
        ))}
      </div>
    </div>
  )
}

function LoginModal({ onClose, onAuthenticated }: { onClose: () => void; onAuthenticated: (user: AuthUser) => void }) {
  const resetToken = new URLSearchParams(window.location.search).get('reset_token') ?? ''
  const [mode, setMode] = useState<'login' | 'register' | 'forgot' | 'reset'>(resetToken ? 'reset' : 'login')
  const [email, setEmail] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [password, setPassword] = useState('')
  const [passwordConfirmation, setPasswordConfirmation] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  const submit = async () => {
    setSubmitting(true)
    setError('')
    setNotice('')
    try {
      if (mode === 'forgot') {
        if (!email.trim()) throw new Error('请输入注册邮箱。')
        await requestPasswordReset(email.trim())
        setNotice('如果该邮箱已注册，重置链接会在几分钟内发送。')
        return
      }
      if (mode === 'reset') {
        if (!resetToken || password.length < 8 || password !== passwordConfirmation) {
          throw new Error('请填写两次相同且至少 8 位的新密码。')
        }
        await confirmPasswordReset({ token: resetToken, new_password: password })
        window.history.replaceState({}, '', window.location.pathname)
        setPassword('')
        setPasswordConfirmation('')
        setMode('login')
        setNotice('密码已重置，请使用新密码登录。')
        return
      }
      if (!email.trim() || password.length < 8 || (mode === 'register' && displayName.trim().length < 2)) {
        throw new Error('请填写有效邮箱、至少 8 位密码和昵称。')
      }
      const response = mode === 'login'
        ? await login({ email: email.trim(), password })
        : await register({ email: email.trim(), display_name: displayName.trim(), password })
      onAuthenticated(response.data)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '操作失败，请稍后重试。')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="modal-backdrop" onMouseDown={onClose}>
      <div className="login-modal" onMouseDown={(event) => event.stopPropagation()}>
        <button className="modal-close icon-button" title="关闭登录窗口" aria-label="关闭登录窗口" onClick={onClose}><X size={18} /></button>
        <BrandMark className="brand-mark-svg large" size={42} />
        <h2>{mode === 'login' ? '登录新猿译码' : mode === 'register' ? '创建社区账号' : mode === 'forgot' ? '找回密码' : '设置新密码'}</h2>
        <p>{mode === 'forgot' ? '输入注册邮箱，我们会发送一个 30 分钟内有效的重置链接。' : mode === 'reset' ? '设置新密码后，所有已登录设备都会退出。' : mode === 'login' ? '登录后可以参与讨论，并管理自己的评论。' : '注册后，评论作者将由服务端绑定到你的账号。'}</p>
        {(mode === 'login' || mode === 'register') && (
          <>
            <button className="provider-button github" onClick={() => window.location.assign('/api/v1/auth/oauth/github/start')}><GitBranch size={17} /> 使用 GitHub 登录 <ArrowUpRight size={14} /></button>
            <button className="provider-button wechat" onClick={() => window.location.assign('/api/v1/auth/oauth/wechat/start')}><span className="wechat-icon">微</span> 使用微信登录 <ArrowUpRight size={14} /></button>
            <div className="login-divider"><span>或使用邮箱</span></div>
            <div className="auth-mode-tabs">
              <button className={mode === 'login' ? 'active' : ''} onClick={() => { setMode('login'); setError(''); setNotice('') }}>登录</button>
              <button className={mode === 'register' ? 'active' : ''} onClick={() => { setMode('register'); setError(''); setNotice('') }}>注册</button>
            </div>
          </>
        )}
        <div className="auth-form">
          {mode === 'register' && <input autoFocus value={displayName} maxLength={80} onChange={(event) => setDisplayName(event.target.value)} placeholder="昵称" autoComplete="name" />}
          {mode !== 'reset' && <input autoFocus={mode === 'login' || mode === 'forgot'} value={email} onChange={(event) => setEmail(event.target.value)} placeholder="邮箱" type="email" autoComplete="email" />}
          {mode !== 'forgot' && <input value={password} minLength={8} maxLength={128} onChange={(event) => setPassword(event.target.value)} onKeyDown={(event) => { if (event.key === 'Enter' && mode !== 'reset') void submit() }} placeholder={mode === 'reset' ? '新密码（至少 8 位）' : '密码（至少 8 位）'} type="password" autoComplete={mode === 'login' ? 'current-password' : 'new-password'} />}
          {mode === 'reset' && <input value={passwordConfirmation} minLength={8} maxLength={128} onChange={(event) => setPasswordConfirmation(event.target.value)} onKeyDown={(event) => { if (event.key === 'Enter') void submit() }} placeholder="再次输入新密码" type="password" autoComplete="new-password" />}
          {error && <div className="auth-error">{error}</div>}
          {notice && <div className="auth-notice">{notice}</div>}
          <button className="primary-button" disabled={submitting} onClick={submit}>{submitting ? '提交中…' : mode === 'login' ? '登录' : mode === 'register' ? '注册并登录' : mode === 'forgot' ? '发送重置邮件' : '重置密码'}</button>
          {mode === 'login' && <button className="text-button auth-secondary" onClick={() => { setMode('forgot'); setError(''); setNotice('') }}>忘记密码？</button>}
          {(mode === 'forgot' || mode === 'reset') && <button className="text-button auth-secondary" onClick={() => { setMode('login'); setError(''); setNotice('') }}>返回登录</button>}
        </div>
        <small>会话保存在安全的 HttpOnly Cookie 中，前端无法读取登录令牌。</small>
      </div>
    </div>
  )
}

export default App
