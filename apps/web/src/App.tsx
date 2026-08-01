import { isValidElement, lazy, Suspense, useEffect, useMemo, useRef, useState, type FormEvent, type ReactNode } from 'react'
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
  GitBranch,
  Heart,
  ThumbsUp,
  Menu,
  MessageSquare,
  MonitorCog,
  Moon,
  MoreHorizontal,
  Palette,
  Pencil,
  Play,
  Search,
  Send,
  Share2,
  ShieldCheck,
  Sparkles,
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
  getProjects,
  getServiceInfo,
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
  type CollaborationAccess,
  type ProjectCollaborator,
  type ProjectDetail as APIProjectDetail,
  type ProjectSummary,
  type ServiceInfo,
} from './api/client'
import AiAssistant from './AiAssistant'
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
import AdminConsole from './AdminConsole'
import AccessKeyManager from './AccessKeyManager'
import OpenApiDocs from './OpenApiDocs'
import { BrandMark } from './BrandMark'
import { LevelAvatar, LevelBadge } from './LevelAvatar'
import { bilibiliEmbedURL, useDocumentSearch } from './documentReaderUtils'
import { useHighlightedCode } from './codeHighlight'
import { themes, applyTheme, isThemeId, type ThemeId } from './themes'
import './App.css'

const EmojiMartPicker = lazy(() => import('./EmojiMartPicker'))
const RichMarkdownEditor = lazy(() => import('./RichMarkdownEditor'))
const CollaborativeMarkdownEditor = lazy(() => import('./CollaborativeMarkdownEditor'))
const ProjectDocumentTree = lazy(() => import('./ProjectDocumentTree'))
const DocumentSearchPanel = lazy(() => import('./DocumentSearchPanel'))

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
  highlights?: string[]
  useCases?: string[]
  currentVersion?: string
  resources?: { cover: boolean; document: boolean; code: boolean }
}

type CommentItem = {
  id: string
  blockId: string
  authorId: string
  authorLevel: number
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
    highlights: demoProject?.highlights,
    useCases: demoProject?.useCases,
    currentVersion: demoProject?.currentVersion,
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
  }
}

function mapDocumentComment(comment: APIDocumentComment): CommentItem {
  return {
    id: comment.id,
    blockId: comment.block_id,
    authorId: comment.author_id ?? '',
    authorLevel: comment.author_level ?? 1,
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
  const [search, setSearch] = useState('')
  const [selectedProject, setSelectedProject] = useState<Project | null>(null)
  const [detailTab, setDetailTab] = useState('文档阅读')
  const [saved, setSaved] = useState<string[]>([])
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
  const [accountPanelOpen, setAccountPanelOpen] = useState(false)
  const [notifications, setNotifications] = useState<AppNotification[]>([])
  const [unreadNotificationCount, setUnreadNotificationCount] = useState(0)
  const [notificationPanelOpen, setNotificationPanelOpen] = useState(false)
  const [notificationsLoading, setNotificationsLoading] = useState(false)
  const [markingNotifications, setMarkingNotifications] = useState(false)
  const [authorCenterOpen, setAuthorCenterOpen] = useState(false)
  const [adminConsoleOpen, setAdminConsoleOpen] = useState(false)
  const [accessKeyOpen, setAccessKeyOpen] = useState(false)
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
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false)
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
  const [themePanelOpen, setThemePanelOpen] = useState(false)
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
    const controller = new AbortController()
    getCurrentUser(controller.signal)
      .then((response) => setCurrentUser(response.data))
      .catch(() => setCurrentUser(null))
    return () => controller.abort()
  }, [])

  useEffect(() => {
    if (!currentUser) {
      setNotifications([])
      setUnreadNotificationCount(0)
      setNotificationPanelOpen(false)
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
        query: search.trim() || undefined,
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
  }, [activeCategory, search])

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

  const filteredProjects = useMemo(() => {
    const normalizedSearch = search.trim().toLowerCase()
    return catalogProjects.filter((project) => {
      if (activeTab === '我的收藏' && !saved.includes(project.id)) return false
      const matchesCategory = activeCategory === '全部项目' || project.category === activeCategory
      const haystack = [project.name, project.summary, project.category, ...project.tags, ...project.stack]
        .join(' ')
        .toLowerCase()
      return matchesCategory && (!normalizedSearch || haystack.includes(normalizedSearch))
    })
  }, [activeCategory, activeTab, catalogProjects, saved, search])

  const showToast = (message: string) => {
    setToast(message)
    window.setTimeout(() => setToast(''), 2400)
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
    setNotificationPanelOpen(false)
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
      setAccountPanelOpen(false)
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
        setAccountPanelOpen(false)
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
      <header className="site-header">
        <button className="brand" onClick={closeProject} aria-label="返回首页">
          <BrandMark className="brand-mark-svg" size={32} title="新猿译码" />
          <span>
            <strong>新猿译码</strong>
            <small>AGENT OPEN SOURCE HUB</small>
          </span>
        </button>

        <nav className={`main-nav ${mobileMenuOpen ? 'is-open' : ''}`}>
          {['探索', '趋势', '最新更新', '社区'].map((item) => (
            <button key={item} className={activeTab === item ? 'active' : ''} onClick={() => { setActiveTab(item); closeProject(); setMobileMenuOpen(false) }}>
              {item}
            </button>
          ))}
        </nav>

        <div className="header-actions">
          <label className="global-search">
            <Search size={16} />
            <input value={search} onChange={(event) => setSearch(event.target.value)} placeholder="搜索项目、文档或技术栈" />
            <kbd>⌘ K</kbd>
          </label>
          <div className="theme-control">
            <button className="icon-button quiet" title="切换主题" aria-label="切换主题" aria-expanded={themePanelOpen} onClick={() => setThemePanelOpen((open) => !open)}>
              {themeMode === 'dark' ? <Moon size={18} /> : themeMode === 'light' ? <Sun size={18} /> : <MonitorCog size={18} />}
            </button>
            {themePanelOpen && <ThemePanel themeMode={themeMode} skin={skin} onModeChange={(mode) => { setThemeMode(mode); setThemePanelOpen(false) }} onSkinChange={setSkin} />}
          </div>
          <button
            className="icon-button quiet"
            title="搜索全站文档"
            aria-label="搜索全站文档"
            onClick={() => setSearchPanelOpen(true)}
          >
            <Search size={17} />
          </button>
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
                setNotificationPanelOpen((open) => !open)
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
          <div className="account-control">
            <button
              className="login-button"
              title={currentUser ? '打开账号菜单' : '登录或注册'}
              aria-expanded={currentUser ? accountPanelOpen : undefined}
              onClick={() => currentUser ? setAccountPanelOpen((open) => !open) : setLoginOpen(true)}
            >
              <CircleUserRound size={16} /> {currentUser?.display_name ?? '登录'}
            </button>
            {currentUser && accountPanelOpen && (
              <div className="account-popover">
                <div className="account-summary">
                  <LevelAvatar level={currentUser.level} initials={currentUser.display_name.slice(0, 1)} size="lg" name={currentUser.display_name} />
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
                <button onClick={() => { setActiveTab('我的收藏'); closeProject(); setAccountPanelOpen(false) }}>
                  查看我的收藏
                </button>
                <button onClick={() => { setAuthorCenterOpen(true); setAccountPanelOpen(false) }}>
                  作者项目中心
                </button>
                <button onClick={() => { setAccessKeyOpen(true); setAccountPanelOpen(false) }}>
                  AccessKey 管理
                </button>
                {currentUser.is_admin && <button onClick={() => { setAdminConsoleOpen(true); setAccountPanelOpen(false) }}>
                  管理控制台
                </button>}
                <button disabled={logoutSubmitting} onClick={() => void performLogout(false)}>退出当前设备</button>
                <button disabled={logoutSubmitting} onClick={() => void performLogout(true)}>{logoutSubmitting ? '处理中…' : '退出所有设备'}</button>
              </div>
            )}
          </div>
          <button className="icon-button mobile-only" title="打开菜单" aria-label="打开菜单" onClick={() => setMobileMenuOpen((open) => !open)}><Menu size={19} /></button>
        </div>
      </header>

      {selectedProject ? (
        <ProjectDetail
          project={selectedProject}
          detailTab={detailTab}
          setDetailTab={setDetailTab}
          isSaved={saved.includes(selectedProject.id)}
          onBack={closeProject}
          onToggleSaved={() => toggleSaved(selectedProject.id)}
          onShare={() => {
            navigator.clipboard?.writeText(window.location.href)
            showToast('项目链接已复制')
            // 分享加经验，best-effort：失败不影响复制链接。仅登录用户计入。
            if (currentUser) void shareProject(selectedProject.slug).catch(() => {})
          }}
          onDownload={() => showToast('演示下载已开始')}
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
        />
      )}

      {openDocsOpen && <OpenApiDocs onClose={() => setOpenDocsOpen(false)} />}

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
          onClose={() => setSearchPanelOpen(false)}
        />
      </Suspense>}
      {authorCenterOpen && currentUser && <ErrorBoundary label="作者项目中心"><AuthorProjectCenter
        onClose={() => setAuthorCenterOpen(false)}
        onChanged={() => {
          showToast('项目状态已更新')
        }}
      /></ErrorBoundary>}
      {adminConsoleOpen && currentUser?.is_admin && <ErrorBoundary label="管理控制台"><AdminConsole
        onClose={() => setAdminConsoleOpen(false)}
        currentUser={currentUser}
      /></ErrorBoundary>}
      {accessKeyOpen && currentUser && <ErrorBoundary label="AccessKey 管理"><AccessKeyManager
        onClose={() => setAccessKeyOpen(false)}
      /></ErrorBoundary>}
      <AiAssistant
        projectSlug={selectedProject?.slug ?? null}
        projectName={selectedProject?.name ?? null}
        currentUser={currentUser}
        onRequestLogin={() => setLoginOpen(true)}
      />
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
}) {
  return (
    <main>
      <section className="hero-band">
        <div className="hero-copy">
          <span className="eyebrow"><span className="status-dot" /> 为 Agent 开发者整理的开源项目空间</span>
          <h1>找到下一次<br /><em>有用的连接。</em></h1>
          <p>浏览真实可用的 Agent 项目，阅读实现文档，查看关键代码，并把好的想法带回你的工作流。</p>
          <div className="hero-actions">
            <button className="primary-button" onClick={() => document.getElementById('project-grid')?.scrollIntoView({ behavior: 'smooth' })}>开始探索 <ArrowUpRight size={16} /></button>
            <button className="text-button" onClick={() => window.open('https://github.com', '_blank')}>提交开源项目 <Upload size={16} /></button>
          </div>
        </div>
        <div className="hero-visual" aria-label="Agent 工作流预览">
          <div className="visual-topline"><span>LIVE / PROJECT MAP</span><span>2026.07</span></div>
          <div className="signal-grid">
            <div className="signal-line line-one" /><div className="signal-line line-two" /><div className="signal-line line-three" />
            <div className="signal-node node-a"><span>R</span><small>RETRIEVE</small></div>
            <div className="signal-node node-b"><span>P</span><small>PLAN</small></div>
            <div className="signal-node node-c"><span>T</span><small>TOOL</small></div>
            <div className="signal-node node-d"><span>↗</span><small>OUTPUT</small></div>
          </div>
          <div className="visual-footer"><span>372 个公开项目</span><span className="visual-pulse">正在更新</span></div>
        </div>
      </section>

      <section className="metrics-strip">
        <div><strong>372</strong><span>公开项目</span></div>
        <div><strong>86</strong><span>今日更新</span></div>
        <div><strong>12.4k</strong><span>本周下载</span></div>
        <div><strong>98%</strong><span>项目可读</span></div>
        <div className="metrics-note"><Sparkles size={17} /><span>中文项目优先，欢迎分享你的 Agent 实验。</span></div>
      </section>

      <section className="content-section" id="project-grid">
        <div className="section-heading">
          <div><span className="section-kicker">PROJECT INDEX / 01</span><h2>{activeTab === '探索' ? '探索项目' : activeTab}</h2></div>
          <button className="outline-button">查看全部 <ChevronRight size={15} /></button>
        </div>
        <div className="filter-row">
          {categories.map((category) => <button key={category} className={`filter-chip ${activeCategory === category ? 'active' : ''}`} onClick={() => setActiveCategory(category)}>{category}</button>)}
          <button className="filter-more"><Tag size={14} /> 更多筛选 <ChevronDown size={14} /></button>
        </div>

        {catalogState !== 'online' && (
          <div className={`catalog-notice ${catalogState}`}>
            <span className="gateway-state-dot" />
            {catalogState === 'checking' ? '正在同步项目目录…' : '当前展示演示数据，项目 API 暂时不可用。'}
          </div>
        )}

        {filteredProjects.length ? (
          <div className="project-grid">
            {filteredProjects.map((project) => <ProjectCard key={project.id} project={project} isSaved={saved.includes(project.id)} onOpen={() => onOpenProject(project)} onToggleSaved={() => onToggleSaved(project.id)} />)}
          </div>
        ) : <div className="empty-state"><Search size={24} /><h3>没有找到匹配项目</h3><p>试试其他关键词或清空筛选条件。</p></div>}
      </section>

      <section className="editorial-band">
        <div><span className="section-kicker">FROM THE COMMUNITY / 02</span><h2>好的项目，值得被读懂。</h2><p>项目不只是一个仓库地址。我们希望让每个 Agent 的背景、设计选择和使用方式都能被清楚地留下来。</p></div>
        <div className="editorial-quote"><MessageSquare size={20} /><p>“把复杂的 Agent 工程，整理成可以被下一位开发者接住的知识。”</p><span>新猿译码编辑部</span></div>
      </section>

      <section className="content-section compact-section">
        <div className="section-heading"><div><span className="section-kicker">CURATED NOTE / 03</span><h2>本周精选</h2></div><button className="text-button">阅读编辑手记 <ArrowUpRight size={15} /></button></div>
        <div className="curated-row">
          <div className="curated-cover cover-blue"><div className="cover-label">FIELD NOTE 07</div><span>Agent<br />Observability</span></div>
          <div className="curated-copy"><span className="meta-label">编辑推荐 · 6 分钟阅读</span><h3>为什么 Agent 需要自己的“运行日志”？</h3><p>从单次回答到可回放的任务链路，观察每一步工具调用，才能让实验走向生产。</p><button className="text-button">打开文章 <ArrowUpRight size={15} /></button></div>
        </div>
      </section>
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
  const projectIndex = projects.findIndex((item) => item.id === project.id)
  return (
    <article className="project-card">
      <button className={`project-cover ${project.accent}`} onClick={onOpen} aria-label={`打开 ${project.name}`}>
        <div className="cover-orbit orbit-one" /><div className="cover-orbit orbit-two" /><div className="cover-orbit orbit-three" />
        <span className="cover-monogram">{project.name.slice(0, 1)}</span><span className="cover-index">0{Math.max(projectIndex, 0) + 1} / 04</span>
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
  onBack,
  onToggleSaved,
  onShare,
  onDownload,
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
  onBack: () => void
  onToggleSaved: () => void
  onShare: () => void
  onDownload: () => void
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

  useEffect(() => {
    const controller = new AbortController()
    setCollaborationAccess(null)
    setCollaborationEditing(false)
    setPermissionsOpen(false)
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
        <div className="detail-copy"><span className="eyebrow">{project.status} · 最后更新于 {project.updated}</span><h1>{project.name}</h1><p>{project.description}</p><div className="detail-meta"><span><CircleUserRound size={15} /> {project.maintainer}</span><span><GitBranch size={15} /> {project.repo}</span><span><Tag size={15} /> {project.license}</span></div></div>
        <div className="detail-actions">
          {collaborationAccess?.can_edit && <button className="primary-button" onClick={startCollaboration}><Pencil size={15} /> 协作编辑</button>}
          {collaborationAccess?.can_manage && <button className="icon-button" title="管理协作权限" aria-label="管理协作权限" onClick={() => setPermissionsOpen(true)}><ShieldCheck size={17} /></button>}
          <button className={`outline-button ${isSaved ? 'is-saved' : ''}`} onClick={onToggleSaved}><Heart size={15} fill={isSaved ? 'currentColor' : 'none'} /> {isSaved ? '已收藏' : '收藏'}</button>
          <button className="icon-button" title="分享项目" aria-label="分享项目" onClick={onShare}><Share2 size={17} /></button>
          <button className="icon-button" title="更多操作" aria-label="更多操作"><MoreHorizontal size={18} /></button>
        </div>
      </section>
      <div className="detail-stats"><div><strong>{project.views}</strong><span>浏览</span></div><div><strong>{project.downloads}</strong><span>下载</span></div><div><strong>{project.stars}</strong><span>Stars</span></div><div><strong>{project.comments}</strong><span>讨论</span></div><div><strong>{project.currentVersion ? `v${project.currentVersion}` : '—'}</strong><span>当前版本</span></div></div>
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
          {detailTab === '文档阅读' && <ErrorBoundary label="文档阅读" onReset={() => activeDocument && onOpenDocument(activeDocument.slug)}><DocumentView project={project} documentState={documentState} documentTree={documentTree} activeDocument={activeDocument} onOpenDocument={onOpenDocument} comments={comments} commentsState={commentsState} commentSubmitting={commentSubmitting} resolvingCommentID={resolvingCommentID} deletingCommentID={deletingCommentID} currentUserID={currentUserID} selectedQuote={selectedQuote} composerAnchor={composerAnchor} commentComposerOpen={commentComposerOpen} setCommentComposerOpen={setCommentComposerOpen} draftComment={draftComment} setDraftComment={setDraftComment} onSelection={onSelection} onSubmitComment={onSubmitComment} onResolveComment={onResolveComment} onLikeComment={onLikeComment} onReplyComment={onReplyComment} onDeleteReply={onDeleteReply} onDeleteComment={onDeleteComment} onEditComment={onEditComment} onEditReply={onEditReply} showToast={showToast} /></ErrorBoundary>}
          {detailTab === '代码预览' && <ErrorBoundary label="代码预览"><CodeView project={project} onCopy={() => showToast('代码已复制到剪贴板')} showToast={showToast} /></ErrorBoundary>}
          {detailTab === '下载资源' && <ErrorBoundary label="下载资源"><DownloadView project={project} onDownload={onDownload} /></ErrorBoundary>}
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

function DocumentView({ project, documentState, documentTree, activeDocument, onOpenDocument, comments, commentsState, commentSubmitting, resolvingCommentID, deletingCommentID, currentUserID, selectedQuote, composerAnchor, commentComposerOpen, setCommentComposerOpen, draftComment, setDraftComment, onSelection, onSubmitComment, onResolveComment, onLikeComment, onReplyComment, onDeleteReply, onDeleteComment, onEditComment, onEditReply, showToast }: { project: Project; documentState: CatalogState; documentTree: DocumentNode[]; activeDocument: DocumentDetail | null; onOpenDocument: (documentSlug: string) => void; comments: CommentItem[]; commentsState: CatalogState; commentSubmitting: boolean; resolvingCommentID: string | null; deletingCommentID: string | null; currentUserID: string; selectedQuote: string; composerAnchor: { top: number } | null; commentComposerOpen: boolean; setCommentComposerOpen: (open: boolean) => void; draftComment: string; setDraftComment: (value: string) => void; onSelection: () => void; onSubmitComment: () => void; onResolveComment: (commentId: string) => void; onLikeComment: (commentId: string, liked: boolean) => void; onReplyComment: (commentId: string, body: string) => Promise<boolean>; onDeleteReply: (commentId: string, replyId: string) => Promise<boolean>; onDeleteComment: (commentId: string) => Promise<boolean>; onEditComment: (commentId: string, body: string) => Promise<boolean>; onEditReply: (commentId: string, replyId: string, body: string) => Promise<boolean>; showToast: (message: string) => void }) {
  const articleContentRef = useRef<HTMLDivElement | null>(null)
  const [lightbox, setLightbox] = useState<{ src: string; alt: string } | null>(null)
  const [searchKeyword, setSearchKeyword] = useState('')
  const documentSearch = useDocumentSearch(articleContentRef, searchKeyword, activeDocument?.id ?? '')

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
      <aside className="doc-sidebar"><div className="sidebar-heading"><span>文档目录</span><button className="icon-button quiet" title="收起目录" aria-label="收起目录"><ChevronDown size={15} /></button></div><div className="doc-project-label"><div className="mini-mark">{project.name.slice(0, 1)}</div><div><strong>{project.name}</strong><small>文档 v{activeDocument?.version ?? project.currentVersion ?? '—'}</small></div></div><nav className="doc-tree">{documentTree.length ? renderDocumentNodes(documentTree) : <span className="doc-tree-empty">暂无文档</span>}</nav>{activeDocument?.outline.length ? <nav className="doc-outline" aria-label="本文大纲"><span className="meta-label">ON THIS PAGE</span>{activeDocument.outline.map((item) => <button key={item.id} className={item.level > 1 ? 'indent' : ''} onClick={() => document.getElementById(item.id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })}>{item.title}</button>)}</nav> : null}<div className="sidebar-bottom"><span className="meta-label">DOCUMENT STATUS</span><p><span className="status-dot" /> 已审核 · 公开可读</p></div></aside>
      <article className="document-article" onMouseUp={onSelection}>
        <div className="article-toolbar"><span className="meta-label">{activeDocument ? activeDocument.title : '文档'}</span><div className="article-toolbar-actions">{activeDocument && <DocumentSearchBox keyword={searchKeyword} onKeywordChange={setSearchKeyword} total={documentSearch.total} activeIndex={documentSearch.activeIndex} onNext={documentSearch.next} onPrevious={documentSearch.previous} />}<button className="tool-button" title="复制文档链接" disabled={!activeDocument} onClick={() => void copyDocumentLink()}><Copy size={14} /> 链接</button>{activeDocument ? <a className="tool-button" title="下载 Markdown" href={markdownDownloadURL} download={`${project.slug}-${activeDocument.slug}.md`} onClick={() => showToast('Markdown 下载已开始')}><Download size={14} /> 下载</a> : <button className="tool-button" title="下载 Markdown" disabled><Download size={14} /> 下载</button>}</div></div>
        {documentState === 'offline' && <div className="document-sync-state offline"><span className="gateway-state-dot" />文档服务暂时不可用，请稍后重试。</div>}
        <div className="article-content" ref={articleContentRef}>
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
        {commentComposerOpen && <div
          className={`selection-composer ${composerAnchor ? 'is-anchored' : ''}`}
          style={composerAnchor ? { top: `${composerAnchor.top}px` } : undefined}
        ><div className="composer-quote">“{selectedQuote}”</div><textarea autoFocus value={draftComment} disabled={commentSubmitting} onChange={(event) => setDraftComment(event.target.value)} placeholder="写下你的评论..." /><div className="composer-actions"><EmojiPicker onSelect={(emoji) => setDraftComment(draftComment + emoji)} /><button className="text-button" disabled={commentSubmitting} onClick={() => setCommentComposerOpen(false)}>取消</button><button className="primary-button small" disabled={commentSubmitting || !draftComment.trim()} onClick={onSubmitComment}><Send size={14} /> {commentSubmitting ? '发布中…' : '发布评论'}</button></div></div>}
      </article>
      <aside className="comments-sidebar"><div className="comments-heading"><div><span className="meta-label">DISCUSSION</span><h3>文档评论 <span>{comments.length} 条线程 · {comments.reduce((total, comment) => total + comment.replyCount, 0)} 条回复</span></h3></div><button className="icon-button quiet" title="评论筛选" aria-label="评论筛选"><MoreHorizontal size={17} /></button></div><button className="new-comment-button" disabled={commentsState === 'checking'} onClick={() => setCommentComposerOpen(true)}><MessageSquare size={15} /> {commentsState === 'checking' ? '加载评论中…' : '添加评论'}</button><div className="comment-list">{comments.map((comment) => <CommentCard key={comment.id} comment={comment} currentUserID={currentUserID} resolving={resolvingCommentID === comment.id} deleting={deletingCommentID === comment.id} onResolve={() => onResolveComment(comment.id)} onLike={onLikeComment} onReply={(body) => onReplyComment(comment.id, body)} onDeleteReply={(replyId) => onDeleteReply(comment.id, replyId)} onDelete={() => onDeleteComment(comment.id)} onEdit={(body) => onEditComment(comment.id, body)} onEditReply={(replyId, body) => onEditReply(comment.id, replyId, body)} onLocate={locateBlock} />)}</div><div className={`realtime-note ${commentsState}`}><span className="status-dot" /> {commentsState === 'offline' ? '评论同步暂时离线' : commentsState === 'checking' ? '评论同步中…' : '评论实时同步中'}</div></aside>
      {lightbox && <ImageLightbox src={lightbox.src} alt={lightbox.alt} onClose={() => setLightbox(null)} />}
    </section>
  )
}

function CommentCard({ comment, currentUserID, resolving, deleting, onResolve, onLike, onReply, onDeleteReply, onDelete, onEdit, onEditReply, onLocate }: { comment: CommentItem; currentUserID: string; resolving: boolean; deleting: boolean; onResolve: () => void; onLike: (commentId: string, liked: boolean) => void; onReply: (body: string) => Promise<boolean>; onDeleteReply: (replyId: string) => Promise<boolean>; onDelete: () => Promise<boolean>; onEdit: (body: string) => Promise<boolean>; onEditReply: (replyId: string, body: string) => Promise<boolean>; onLocate: (blockId: string) => void }) {
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
        <LevelAvatar level={comment.authorLevel} initials={comment.initials} size="sm" name={comment.user} />
        <div>
          <span className="name-row"><strong className={comment.authorLevel >= 6 ? 'nickname-legendary' : ''}>{comment.user}</strong><LevelBadge level={comment.authorLevel} /></span>
          <small>{comment.time}{comment.edited ? ' · 已编辑' : ''}</small>
        </div>
        {comment.status === 'resolved' ? <Check size={15} className="resolved-icon" /> : <button className="comment-more" title="更多评论操作" aria-label="更多评论操作"><MoreHorizontal size={15} /></button>}
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
              <div className="reply-head"><LevelAvatar level={reply.authorLevel} initials={reply.initials} size="sm" name={reply.user} /><strong className={reply.authorLevel >= 6 ? 'nickname-legendary' : ''}>{reply.user}</strong><LevelBadge level={reply.authorLevel} /><span><small>{reply.time}{reply.edited ? ' · 已编辑' : ''}</small>{currentUserID !== '' && reply.authorId === currentUserID && <><button disabled={replyEditSubmitting || deletingReplyID === reply.id} onClick={() => { setEditingReplyID(reply.id); setReplyEditDraft(reply.text) }}>编辑</button><button disabled={deletingReplyID === reply.id} onClick={() => deleteReply(reply.id)}>{deletingReplyID === reply.id ? '删除中…' : '删除'}</button></>}</span></div>
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
        ? <div className="comment-actions">{currentUserID !== '' && comment.authorId === currentUserID && <button disabled={resolving || deleting || replySubmitting} onClick={onResolve}>{resolving ? '解决中…' : '解决'}</button>}<button disabled={resolving || deleting || replySubmitting} onClick={() => setReplyOpen((open) => !open)}>回复</button>{currentUserID !== '' && comment.authorId === currentUserID && <><button disabled={editSubmitting || deleting} onClick={() => { setEditDraft(comment.text); setEditing(true) }}>编辑</button><button disabled={deleting} onClick={onDelete}>{deleting ? '删除中…' : '删除'}</button></>}</div>
        : <span className="resolved-label">已解决</span>}
    </article>
  )
}

function OverviewView({ project, onRead }: { project: Project; onRead: () => void }) {
  const highlights = project.highlights ?? ['项目结构清晰', '文档公开可读', '支持快速复用']
  const useCases = project.useCases ?? project.tags
  return <section className="overview-view"><div className="overview-main"><span className="section-kicker">ABOUT THIS PROJECT</span><h2>{project.summary}</h2><MarkdownCanvas markdown={project.description} projectSlug={project.slug} /><div className="feature-list">{highlights.map((highlight) => <div key={highlight}><Check size={16} /><span>{highlight}</span></div>)}</div><button className="primary-button" onClick={onRead}>打开文档 <BookOpen size={16} /></button></div><div className={`overview-visual ${project.accent}`}><div className="visual-topline"><span>VERSION / {project.currentVersion ?? '—'}</span><span>READY</span></div><div className="overview-lines">{useCases.slice(0, 4).map((useCase) => <span key={useCase}>{useCase}</span>)}</div><div className="overview-bottom"><span>{useCases.length} 个适用场景</span><span>{project.status}</span></div></div></section>
}

function formatFileSize(bytes: number) {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
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

  const copyContent = () => {
    if (!activeFile) return
    navigator.clipboard?.writeText(activeFile.content).then(onCopy).catch(() => {
      showToast('浏览器拒绝了剪贴板写入，请手动复制')
    })
  }

  const lines = activeFile ? activeFile.content.replace(/\n$/, '').split('\n') : []
  const highlighted = useHighlightedCode(
    activeFile ? activeFile.content.replace(/\n$/, '') : '',
    activeFile?.language ?? '',
  )

  return (
    <section className="code-view">
      <div className="file-tree">
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
        {treeState === 'ready' && entries.map((entry) => (
          <button
            key={entry.path}
            className={`file-item ${entry.path === activePath ? 'active' : ''} ${entry.dir ? 'is-dir' : ''}`}
            style={{ paddingLeft: `${10 + entry.path.split('/').length * 8}px` }}
            disabled={entry.dir}
            title={entry.path}
            onClick={() => { if (!entry.dir) setActivePath(entry.path) }}
          >
            <span>{entry.dir ? <ChevronRight size={14} /> : <FileCode2 size={15} />}</span>
            <span>{entry.name}</span>
            {!entry.dir && <small>{formatFileSize(entry.size)}</small>}
          </button>
        ))}
        {project.repo && <div className="file-tree-note"><GitBranch size={16} /><span>{project.repo}</span></div>}
      </div>
      <div className="source-panel">
        <div className="source-head">
          <span>{activeFile ? `${activeFile.path}　·　${formatFileSize(activeFile.size)}` : '选择左侧文件查看源码'}</span>
          <span className="source-head-actions">
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
            <pre className="source-code">
              {/* 行号独立成列，与高亮内容按行高对齐，无需切分高亮后的 HTML。 */}
              <span className="source-gutter" aria-hidden="true">
                {lines.map((_, index) => <span key={index}>{index + 1}</span>)}
              </span>
              {/* highlight.js 会转义输入，回退路径也做了转义，此处不会注入 HTML。 */}
              <code
                className={`hljs language-${activeFile.language}`}
                dangerouslySetInnerHTML={{ __html: highlighted }}
              />
            </pre>
          </>
        )}
      </div>
    </section>
  )
}

function DownloadView({ project, onDownload }: { project: Project; onDownload: () => void }) {
  const resourceURL = (kind: 'code' | 'document') => `/api/v1/projects/${encodeURIComponent(project.slug)}/resources/${kind}`
  const downloadControl = (kind: 'code' | 'document', label: string) => project.resources?.[kind]
    ? <a className="icon-button" title={label} aria-label={label} href={resourceURL(kind)}><Download size={16} /></a>
    : <button className="icon-button" title={label} aria-label={label} onClick={onDownload}><Download size={16} /></button>
  return <section className="download-view"><div className="download-intro"><span className="section-kicker">RELEASES / 04</span><h2>选择一个资源开始。</h2><p>当前展示的是项目公开资源。下载记录会计入项目统计，资源由作者维护。</p>{project.resources?.code ? <a className="outline-button" href={resourceURL('code')}><Download size={15} /> 下载代码包</a> : <button className="outline-button" onClick={onDownload}><Download size={15} /> 下载代码包</button>}</div><div className="resource-list"><div className="resource-row"><div className="resource-icon"><Code2 size={18} /></div><div><strong>{project.slug}-v{project.currentVersion ?? 'latest'}</strong><small>代码包 · {project.license}</small></div><span>v{project.currentVersion ?? '—'}</span>{downloadControl('code', '下载代码包')}</div><div className="resource-row"><div className="resource-icon"><FileText size={18} /></div><div><strong>项目文档</strong><small>文档 · 作者维护</small></div><span>DOC</span>{downloadControl('document', '下载文档')}</div><div className="resource-row"><div className="resource-icon"><Play size={18} /></div><div><strong>产品演示</strong><small>Bilibili 外链</small></div><span>VIDEO</span><button className="icon-button" title="打开演示视频" aria-label="打开演示视频" onClick={onDownload}><ArrowUpRight size={16} /></button></div></div></section>
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

function AuthorProjectCenter({ onClose, onChanged }: { onClose: () => void; onChanged: () => void }) {
  usePageScrollLock()
  const [projects, setProjects] = useState<ManagedProject[]>([])
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
  const editorRef = useRef<HTMLTextAreaElement | null>(null)
  const richEditorRef = useRef<RichMarkdownEditorHandle | null>(null)
  // 记录已载入草稿的文档 id，用于区分“切换文档”和“保存后回写”。
  const loadedDocumentID = useRef('')

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
          setDocumentTreeToken((current) => current + 1)
        })
        .catch((reason: unknown) => {
          setDocumentSaveState('idle')
          showAuthorToast(reason instanceof ApiError ? reason.message : '文档保存失败')
        })
    }, 1200)
    return () => window.clearTimeout(timer)
  }, [activeDocument, activeProject, documentDraft])

  const loadProjects = () => {
    setLoading(true)
    getAuthorProjects()
      .then((response) => setProjects(response.data))
      .catch((reason: unknown) => setError(reason instanceof Error ? reason.message : '项目加载失败'))
      .finally(() => setLoading(false))
  }

  useEffect(loadProjects, [])

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
    setError('')
  }

  const newProject = () => {
    setActiveProject(null)
    setInput({ ...emptyManagedProject })
    setTagsText('')
    setStackText('')
    setSaveState('idle')
    setError('')
  }

  const insertMarkdown = (before: string, after = '', placeholder = '') => {
    const markdown = before + placeholder + after
    if (editorMode === 'rich') {
      richEditorRef.current?.insertMarkdown(markdown)
      return
    }
    const textarea = editorRef.current
    if (!textarea) {
      update('description', input.description + markdown)
      return
    }
    const start = textarea.selectionStart
    const end = textarea.selectionEnd
    const selected = input.description.slice(start, end) || placeholder
    const next = input.description.slice(0, start) + before + selected + after + input.description.slice(end)
    update('description', next)
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

  return <div className="modal-backdrop" role="presentation">
    <section className="author-center document-author-center" role="dialog" aria-modal="true" aria-label="作者项目中心">
      <button className="icon-button modal-close" onClick={onClose} aria-label="关闭"><X size={17} /></button>
      {authorToast && <div className="author-toast"><Check size={14} /> {authorToast}</div>}
      <header className="author-editor-head"><div><span className="section-kicker">AUTHOR / DOCUMENT</span><h2>{activeDocument ? activeDocument.title : activeProject ? activeProject.name : '创建项目文档'}</h2><p>{activeDocument ? `正在编辑文档：${activeDocument.slug}` : '像在线文档一样使用 Markdown、图表、图片和附件组织项目内容。'}</p></div><div className="save-indicator">{activeDocument ? (documentSaveState === 'saving' ? '正在保存文档…' : documentSaveState === 'saved' ? '文档已保存' : '文档') : saveState === 'saving' ? '正在自动保存…' : saveState === 'saved' ? '已自动保存' : '草稿'}</div></header>
      <div className="document-editor-layout">
        <aside className="author-project-rail">
          <button className="primary-button" onClick={newProject}>＋ 新建项目</button>
          <h3>我的项目</h3>
          {loading ? <p>正在加载…</p> : projects.length === 0 ? <p className="empty-copy">还没有项目草稿。</p> : projects.map((project) =>
            <button className={activeProject?.id === project.id ? 'active' : ''} key={project.id} onClick={() => selectProject(project)}>
              <strong>{project.name}</strong><small>{statusLabel(project.status)} · v{project.current_version}</small>
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
        </aside>
        <form className="document-project-editor" onSubmit={(event) => void saveProject(event)}>
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
            <button type="button" onClick={() => insertMarkdown('**', '**', '粗体')}>B</button>
            <button type="button" onClick={() => insertMarkdown('*', '*', '斜体')}><em>I</em></button>
            <button type="button" onClick={() => insertMarkdown('\n## ', '\n', '小标题')}>H2</button>
            <button type="button" onClick={() => insertMarkdown('\n- ', '\n', '列表项')}>• 列表</button>
            <button type="button" onClick={() => insertMarkdown('\n> ', '\n', '引用内容')}>❝ 引用</button>
            <button type="button" onClick={() => insertMarkdown('\n```text\n', '\n```\n', '代码')}>{'</>'}</button>
            <button type="button" onClick={() => insertMarkdown('\n```mermaid\ngraph TD\n  A[开始] --> B[完成]\n```\n')}>图表</button>
            <label className="toolbar-upload">图片<input type="file" accept="image/jpeg,image/png,image/webp,image/gif" onChange={(event) => { const file = event.target.files?.[0]; if (file) void uploadInline(file, 'image') }} /></label>
            <label className="toolbar-upload">附件<input type="file" accept=".pdf,.md,.txt,.zip,.tar,.gz,.tgz" onChange={(event) => { const file = event.target.files?.[0]; if (file) void uploadInline(file, /\.(zip|tar|gz|tgz)$/i.test(file.name) ? 'code' : 'document') }} /></label>
            <EmojiPicker onSelect={(emoji) => insertMarkdown(emoji)} />
            {uploading?.startsWith('inline-') && <span>上传中…</span>}
          </div>
          <div className={`markdown-workspace mode-${editorMode}`}>
            {editorMode === 'rich' && <Suspense fallback={<div className="rich-editor-loading">正在加载富文本编辑器…</div>}><RichMarkdownEditor ref={richEditorRef} documentKey={activeDocument?.id ?? activeProject?.id ?? 'new-project'} value={activeDocument ? documentDraft : input.description} onChange={(markdown) => activeDocument ? setDocumentDraft(markdown.slice(0, 200000)) : update('description', markdown.slice(0, 50000))} onUploadImage={uploadRichImage} onUploadFile={uploadRichFile} onNotify={showAuthorToast} /></Suspense>}
            {(editorMode === 'write' || editorMode === 'split') && <textarea ref={editorRef} className="markdown-source" required={!activeDocument} minLength={activeDocument ? 0 : 20} maxLength={activeDocument ? 200000 : 50000} value={activeDocument ? documentDraft : input.description} onChange={(event) => activeDocument ? setDocumentDraft(event.target.value) : update('description', event.target.value)} placeholder={'# 项目介绍\n\n从这里开始，用 Markdown 编写你的项目文档…'} />}
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
