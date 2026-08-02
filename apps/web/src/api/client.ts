export type ServiceInfo = {
  service: string
  api_version: string
  status: string
}

export type AuthUser = {
  id: string
  email: string
  display_name: string
  has_password: boolean
  is_admin: boolean
  experience: number
  level: number
}

export type AuthResponse = {
  data: AuthUser
  request_id: string
}

export type AuthSession = {
  id: string
  created_at: string
  expires_at: string
  current: boolean
}

export type SessionListResponse = {
  data: AuthSession[]
  request_id: string
}

export type FavoriteListResponse = {
  data: string[]
  request_id: string
}

export type FollowListResponse = {
  data: string[]
  request_id: string
}

export type AcceptedResponse = {
  request_id: string
}

export type ManagedProject = {
  id: string
  owner_id: string
  slug: string
  name: string
  summary: string
  description: string
  category: string
  tags: string[]
  tech_stack: string[]
  license: string
  repository_url: string
  cover_object_key: string
  document_object_key: string
  code_object_key: string
  current_version: string
  status: 'draft' | 'pending_review' | 'published' | 'rejected' | 'archived'
  review_reason: string
  submitted_at?: string
  published_at?: string
  created_at: string
  updated_at: string
}

export type ManagedProjectInput = {
  slug: string
  name: string
  summary: string
  description: string
  category: string
  tags: string[]
  tech_stack: string[]
  license: string
  repository_url: string
  cover_object_key: string
  document_object_key: string
  code_object_key: string
  current_version: string
}

export type ManagedProjectListResponse = {
  data: ManagedProject[]
  request_id: string
}

export type ManagedProjectResponse = {
  data: ManagedProject
  request_id: string
}

export type CollaborationAccess = {
  role: 'owner' | 'editor' | 'viewer'
  can_edit: boolean
  can_manage: boolean
}

export type ProjectCollaborator = {
  project_id: string
  user_id: string
  email: string
  display_name: string
  role: 'editor' | 'viewer'
  invited_by: string
  created_at: string
  updated_at: string
}

export type CollaborationAccessResponse = {
  data: CollaborationAccess
  request_id: string
}

export type ProjectCollaboratorListResponse = {
  data: ProjectCollaborator[]
  request_id: string
}

export type ProjectCollaboratorResponse = {
  data: ProjectCollaborator
  request_id: string
}

export type ProjectSummary = {
  id: string
  slug: string
  name: string
  summary: string
  category: string
  tags: string[]
  stack: string[]
  license: string
  status: string
  maintainer: string
  updated_at: string
  metrics: {
    views?: number
    downloads: number
    stars: number
    comments: number
  }
}

export type ProjectListResponse = {
  data: ProjectSummary[]
  pagination: {
    page: number
    page_size: number
    total: number
    total_pages: number
  }
  request_id: string
}

export type ProjectDetail = ProjectSummary & {
  description: string
  highlights: string[]
  use_cases: string[]
  repository: string
  current_version: string
  // owner_id / author_name 让前端把项目链接到作者公开主页；种子项目为空。
  owner_id?: string
  author_name?: string
  resources: {
    cover: boolean
    document: boolean
    code: boolean
  }
}

export type ProjectDetailResponse = {
  data: ProjectDetail
  request_id: string
}

export type PublicUserProfile = {
  id: string
  display_name: string
  level: number
  joined_at: string
  projects: ProjectSummary[]
  stats: {
    projects_count: number
    total_views: number
    total_downloads: number
  }
}

export type PublicUserProfileResponse = {
  data: PublicUserProfile
  request_id: string
}

export type DocumentNode = {
  id: string
  slug: string
  title: string
  order: number
  children: DocumentNode[]
}

export type DocumentOutlineItem = {
  id: string
  title: string
  level: number
}

export type DocumentBlock = {
  id: string
  type: string
  text: string
}

export type DocumentDetail = {
  id: string
  project_id: string
  slug: string
  title: string
  version: string
  updated_at: string
  markdown: string
  outline: DocumentOutlineItem[]
  blocks: DocumentBlock[]
}

export type DocumentListResponse = {
  data: DocumentNode[]
  request_id: string
}

export type DocumentDetailResponse = {
  data: DocumentDetail
  request_id: string
}

export type DocumentComment = {
  id: string
  document_id: string
  parent_id: string | null
  block_id: string
  author_id?: string
  author: string
  author_level?: number
  quote: string
  body: string
  status: 'open' | 'resolved'
  created_at: string
  updated_at: string
  resolved_at: string | null
  replies: DocumentComment[]
  reply_count: number
  like_count?: number
  liked?: boolean
}

export type CommentListResponse = {
  data: DocumentComment[]
  request_id: string
}

export type CommentResponse = {
  data: DocumentComment
  request_id: string
}

export type ProjectDocument = {
  id: string
  project_id: string
  parent_id: string | null
  slug: string
  title: string
  markdown: string
  sort_order: number
  created_by?: string
  created_at: string
  updated_at: string
}

export type ProjectDocumentInput = {
  parent_id?: string | null
  slug: string
  title: string
  markdown: string
}

export type ProjectDocumentResponse = {
  data: ProjectDocument
  request_id: string
}

export type ProjectDocumentListResponse = {
  data: ProjectDocument[]
  tree: DocumentNode[]
  request_id: string
}

export type CodeEntry = {
  path: string
  name: string
  dir: boolean
  size: number
}

export type CodeTreeResponse = {
  data: CodeEntry[]
  readme_path?: string
  truncated: boolean
  request_id: string
}

export type CodeFile = {
  path: string
  size: number
  language: string
  content: string
  truncated: boolean
}

export type CodeFileResponse = {
  data: CodeFile
  request_id: string
}

export type AppNotification = {
  id: string
  actor_id?: string
  actor_name?: string
  type: 'comment.replied' | 'project.approved' | 'project.rejected' | 'project.updated'
  title: string
  body?: string
  project_slug?: string
  document_slug?: string
  comment_id?: string
  read_at?: string
  created_at: string
}

export type NotificationListResponse = {
  data: AppNotification[]
  unread_count: number
  request_id: string
}

type ApiErrorBody = {
  error?: {
    code?: string
    message?: string
  }
  request_id?: string
}

export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly requestId?: string

  constructor(message: string, status: number, code: string, requestId?: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.requestId = requestId
  }
}

const apiBaseURL = (import.meta.env.VITE_API_BASE_URL ?? '').replace(/\/$/, '')

export async function apiRequest<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers)
  headers.set('Accept', 'application/json')

  const response = await fetch(`${apiBaseURL}${path}`, {
    ...options,
    credentials: 'include',
    headers,
  })

  if (!response.ok) {
    const body = await response.json().catch(() => ({})) as ApiErrorBody
    throw new ApiError(
      body.error?.message ?? `请求失败（HTTP ${response.status}）`,
      response.status,
      body.error?.code ?? 'request_failed',
      response.headers.get('X-Request-ID') ?? body.request_id,
    )
  }

  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}

export function getCurrentUser(signal?: AbortSignal) {
  return apiRequest<AuthResponse>('/api/v1/auth/me', { signal })
}

export function updateCurrentUser(input: { display_name: string }) {
  return apiRequest<AuthResponse>('/api/v1/auth/me', {
    method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input),
  })
}

export function register(input: { email: string; display_name: string; password: string }) {
  return apiRequest<AuthResponse>('/api/v1/auth/register', {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input),
  })
}

export function login(input: { email: string; password: string }) {
  return apiRequest<AuthResponse>('/api/v1/auth/login', {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input),
  })
}

export function logout() {
  return apiRequest<void>('/api/v1/auth/logout', { method: 'POST' })
}

export function logoutAll() {
  return apiRequest<void>('/api/v1/auth/logout-all', { method: 'POST' })
}

export function getAuthSessions(signal?: AbortSignal) {
  return apiRequest<SessionListResponse>('/api/v1/auth/sessions', { signal })
}

export function revokeAuthSession(sessionID: string) {
  return apiRequest<void>(`/api/v1/auth/sessions/${encodeURIComponent(sessionID)}`, { method: 'DELETE' })
}

export function changePassword(input: { current_password: string; new_password: string }) {
  return apiRequest<void>('/api/v1/auth/password', {
    method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input),
  })
}

export function requestPasswordReset(email: string) {
  return apiRequest<AcceptedResponse>('/api/v1/auth/password-reset/request', {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email }),
  })
}

export function confirmPasswordReset(input: { token: string; new_password: string }) {
  return apiRequest<void>('/api/v1/auth/password-reset/confirm', {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input),
  })
}

export function getAuthorProjects(signal?: AbortSignal) {
  return apiRequest<ManagedProjectListResponse>('/api/v1/author/projects', { signal })
}

export function createAuthorProject(input: ManagedProjectInput) {
  return apiRequest<ManagedProjectResponse>('/api/v1/author/projects', {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input),
  })
}

export function submitAuthorProject(projectID: string) {
  return apiRequest<ManagedProjectResponse>(`/api/v1/author/projects/${encodeURIComponent(projectID)}/submit`, {
    method: 'POST',
  })
}

export function updateAuthorProject(projectID: string, input: ManagedProjectInput) {
  return apiRequest<ManagedProjectResponse>(`/api/v1/author/projects/${encodeURIComponent(projectID)}`, {
    method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input),
  })
}

// 跨文档搜索。highlight 是服务端生成的带 <em> 标记的片段。
export type SearchHit = {
  project_slug: string
  project_name: string
  document_slug: string
  title: string
  highlight: string[]
  score: number
  updated_at: string
}

export type SearchResponse = {
  data: SearchHit[]
  query: string
  total: number
  request_id: string
}

export function searchDocuments(query: string, signal?: AbortSignal) {
  return apiRequest<SearchResponse>(
    `/api/v1/search?q=${encodeURIComponent(query)}`,
    { signal },
  )
}

export type HotSearchTerm = { term: string; count: number }
export type HotSearchResponse = { data: HotSearchTerm[]; request_id: string }

export function getHotSearchTerms(signal?: AbortSignal) {
  return apiRequest<HotSearchResponse>('/api/v1/search/hot', { signal })
}

// recordProjectView 打一个浏览 beacon（累加项目浏览量）。公开、fire-and-forget。
export function recordProjectView(projectSlug: string) {
  return apiRequest<void>(`/api/v1/projects/${encodeURIComponent(projectSlug)}/view`, { method: 'POST' })
}

export function getPendingProjectReviews(signal?: AbortSignal) {
  return apiRequest<ManagedProjectListResponse>('/api/v1/admin/reviews', { signal })
}

// ==================== 管理控制台 ====================

export type AdminStats = {
  users: number
  projects_total: number
  projects_by_status: Record<string, number>
  pending_reviews: number
  comments: number
}

export type AdminStatsResponse = { data: AdminStats; request_id: string }

export type AdminUser = {
  id: string
  email: string
  display_name: string
  experience: number
  level: number
  is_admin: boolean
  created_at: string
  banned: boolean
}

export type AdminUserListResponse = {
  data: AdminUser[]
  total: number
  page: number
  page_size: number
  request_id: string
}

export type AdminProjectListResponse = {
  data: ManagedProject[]
  total: number
  page: number
  page_size: number
  request_id: string
}

export type ApiKey = {
  id: string
  owner_id: string
  name: string
  prefix: string
  created_at: string
  revoked_at?: string | null
}

export type ApiKeyListResponse = { data: ApiKey[]; request_id: string }

export type IssuedApiKeyResponse = {
  data: { key: ApiKey; plaintext: string }
  request_id: string
}

export type AdminAuditEntry = {
  id: string
  actor_id: string
  actor_email: string
  action: string
  target: string
  detail: string
  created_at: string
}

export type AdminAuditListResponse = { data: AdminAuditEntry[]; request_id: string }

export type UserRegistrationPoint = { date: string; count: number }
export type UserLevelBucket = { level: number; count: number }

export type AdminUserStats = {
  total_users: number
  banned: number
  days: number
  registrations: UserRegistrationPoint[]
  level_histogram: UserLevelBucket[]
}

export type AdminUserStatsResponse = { data: AdminUserStats; request_id: string }

export function getAdminStats(signal?: AbortSignal) {
  return apiRequest<AdminStatsResponse>('/api/v1/admin/stats', { signal })
}

export function getAdminUsers(params: { search?: string; page?: number; page_size?: number }, signal?: AbortSignal) {
  const query = new URLSearchParams()
  if (params.search) query.set('search', params.search)
  if (params.page) query.set('page', String(params.page))
  if (params.page_size) query.set('page_size', String(params.page_size))
  const suffix = query.toString() ? `?${query.toString()}` : ''
  return apiRequest<AdminUserListResponse>(`/api/v1/admin/users${suffix}`, { signal })
}

export function banUser(userID: string, reason: string) {
  return apiRequest<void>(`/api/v1/admin/users/${encodeURIComponent(userID)}/ban`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ reason }),
  })
}

export function unbanUser(userID: string) {
  return apiRequest<void>(`/api/v1/admin/users/${encodeURIComponent(userID)}/ban`, { method: 'DELETE' })
}

export function getAdminProjects(params: { status?: string; page?: number; page_size?: number }, signal?: AbortSignal) {
  const query = new URLSearchParams()
  if (params.status) query.set('status', params.status)
  if (params.page) query.set('page', String(params.page))
  if (params.page_size) query.set('page_size', String(params.page_size))
  const suffix = query.toString() ? `?${query.toString()}` : ''
  return apiRequest<AdminProjectListResponse>(`/api/v1/admin/projects${suffix}`, { signal })
}

export function takedownProject(projectID: string, reason: string) {
  return apiRequest<ManagedProjectResponse>(`/api/v1/admin/projects/${encodeURIComponent(projectID)}/takedown`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ reason }),
  })
}

export function getApiKeys(signal?: AbortSignal) {
  return apiRequest<ApiKeyListResponse>('/api/v1/admin/api-keys', { signal })
}

export function issueApiKey(name: string) {
  return apiRequest<IssuedApiKeyResponse>('/api/v1/admin/api-keys', {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name }),
  })
}

export function revokeApiKey(keyID: string) {
  return apiRequest<void>(`/api/v1/admin/api-keys/${encodeURIComponent(keyID)}`, { method: 'DELETE' })
}

// 用户自助 AccessKey：任意登录用户管理自己的 Open API 密钥（Bearer 鉴权，以本人身份行事）。
export function getMyApiKeys(signal?: AbortSignal) {
  return apiRequest<ApiKeyListResponse>('/api/v1/auth/api-keys', { signal })
}

export function createMyApiKey(name: string) {
  return apiRequest<IssuedApiKeyResponse>('/api/v1/auth/api-keys', {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name }),
  })
}

export function revokeMyApiKey(keyID: string) {
  return apiRequest<void>(`/api/v1/auth/api-keys/${encodeURIComponent(keyID)}`, { method: 'DELETE' })
}

export function getAdminAudit(params: { limit: number; action?: string }, signal?: AbortSignal) {
  const query = new URLSearchParams({ limit: String(params.limit) })
  if (params.action) query.set('action', params.action)
  return apiRequest<AdminAuditListResponse>(`/api/v1/admin/audit?${query.toString()}`, { signal })
}

export function getAdminUserStats(days: number, signal?: AbortSignal) {
  return apiRequest<AdminUserStatsResponse>(`/api/v1/admin/user-stats?days=${days}`, { signal })
}

// 内容举报。登录用户举报项目或评论，管理员在控制台处理。
export type ReportTargetType = 'project' | 'comment'

export type ReportInput = {
  target_type: ReportTargetType
  target_id: string
  reason: string
  detail?: string
}

export type ContentReport = {
  id: string
  reporter_id: string
  reporter_email?: string
  reporter_name?: string
  target_type: string
  target_id: string
  reason: string
  detail: string
  status: string
  created_at: string
  resolved_at?: string | null
  resolver_id?: string | null
}

export type ContentReportListResponse = { data: ContentReport[]; request_id: string }

export function submitReport(input: ReportInput) {
  return apiRequest<{ request_id: string }>('/api/v1/reports', {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input),
  })
}

export function getAdminReports(status?: string, signal?: AbortSignal) {
  const suffix = status ? `?status=${encodeURIComponent(status)}` : ''
  return apiRequest<ContentReportListResponse>(`/api/v1/admin/reports${suffix}`, { signal })
}

export function resolveReport(id: string, action: 'resolve' | 'dismiss') {
  return apiRequest<{ data: { id: string; status: string }; request_id: string }>(
    `/api/v1/admin/reports/${encodeURIComponent(id)}/${action}`,
    { method: 'POST', headers: { 'Content-Type': 'application/json' } },
  )
}

// 作者端文档树管理。权限为项目所有者或 editor 协作者。
function authorDocumentsPath(projectID: string) {
  return `/api/v1/author/projects/${encodeURIComponent(projectID)}/documents`
}

export function getAuthorProjectDocuments(projectID: string, signal?: AbortSignal) {
  return apiRequest<ProjectDocumentListResponse>(authorDocumentsPath(projectID), { signal })
}

export function createAuthorProjectDocument(projectID: string, input: ProjectDocumentInput) {
  return apiRequest<ProjectDocumentResponse>(authorDocumentsPath(projectID), {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input),
  })
}

export function updateAuthorProjectDocument(
  projectID: string, documentID: string, input: ProjectDocumentInput,
) {
  return apiRequest<ProjectDocumentResponse>(
    `${authorDocumentsPath(projectID)}/${encodeURIComponent(documentID)}`,
    { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input) },
  )
}

export function moveAuthorProjectDocument(
  projectID: string, documentID: string, move: { parent_id?: string | null; sort_order: number },
) {
  return apiRequest<ProjectDocumentResponse>(
    `${authorDocumentsPath(projectID)}/${encodeURIComponent(documentID)}/move`,
    { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(move) },
  )
}

export function deleteAuthorProjectDocument(projectID: string, documentID: string) {
  return apiRequest<void>(
    `${authorDocumentsPath(projectID)}/${encodeURIComponent(documentID)}`,
    { method: 'DELETE' },
  )
}

export function reviewProject(projectID: string, action: 'approve' | 'reject', reason: string) {
  return apiRequest<ManagedProjectResponse>(`/api/v1/admin/reviews/${encodeURIComponent(projectID)}/${action}`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ reason }),
  })
}

export async function uploadProjectFile(file: File, kind: 'image' | 'document' | 'code') {
  const extension = file.name.toLowerCase().split('.').pop() ?? ''
  const inferredTypes: Record<string, string> = {
    md: 'text/markdown', txt: 'text/plain', pdf: 'application/pdf',
    zip: 'application/zip', gz: 'application/gzip', tgz: 'application/gzip',
    tar: 'application/x-tar',
  }
  const contentType = file.type || inferredTypes[extension] || 'application/octet-stream'
  const authorization = await apiRequest<{
    data: { object_key: string; method: string; url: string; headers: Record<string, string>; expires_at: string }
  }>('/api/v1/files/presign-upload', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ filename: file.name, content_type: contentType, size: file.size, kind }),
  })
  const response = await fetch(authorization.data.url, {
    method: authorization.data.method,
    headers: authorization.data.headers,
    body: file,
  })
  if (!response.ok) throw new Error(`OSS 上传失败（HTTP ${response.status}）`)
  return authorization.data.object_key
}

export function getFavorites(signal?: AbortSignal) {
  return apiRequest<FavoriteListResponse>('/api/v1/favorites', { signal })
}

export function getFollows(signal?: AbortSignal) {
  return apiRequest<FollowListResponse>('/api/v1/follows', { signal })
}

export function getNotifications(signal?: AbortSignal) {
  return apiRequest<NotificationListResponse>('/api/v1/notifications', { signal })
}

export function getProjectCodeTree(projectSlug: string, query?: string, signal?: AbortSignal) {
  const parameters = new URLSearchParams()
  if (query) parameters.set('q', query)
  const queryString = parameters.toString()
  return apiRequest<CodeTreeResponse>(
    `/api/v1/projects/${encodeURIComponent(projectSlug)}/code${queryString ? `?${queryString}` : ''}`,
    { signal },
  )
}

export function getProjectCodeFile(projectSlug: string, filePath: string, signal?: AbortSignal) {
  return apiRequest<CodeFileResponse>(
    `/api/v1/projects/${encodeURIComponent(projectSlug)}/code/file?path=${encodeURIComponent(filePath)}`,
    { signal },
  )
}

// getProjectCodeFileDownloadURL 返回代码包内单个文件的下载地址。
// 服务端返回完整内容（不受预览截断限制）并强制以附件形式下载。
export function getProjectCodeFileDownloadURL(projectSlug: string, filePath: string) {
  return `${apiBaseURL}/api/v1/projects/${encodeURIComponent(projectSlug)}/code/file/download` +
    `?path=${encodeURIComponent(filePath)}`
}

export function getProjectCodeArchiveURL(projectSlug: string) {
  return `${apiBaseURL}/api/v1/projects/${encodeURIComponent(projectSlug)}/resources/code`
}

export function markNotificationRead(notificationID: string) {
  return apiRequest<void>(`/api/v1/notifications/${encodeURIComponent(notificationID)}/read`, {
    method: 'POST',
  })
}

export function markAllNotificationsRead() {
  return apiRequest<void>('/api/v1/notifications/read-all', { method: 'POST' })
}

export function getNotificationEventsURL() {
  return `${apiBaseURL}/api/v1/notifications/events`
}

// AI 项目助手。后端以 SSE 流式返回增量文本，前端用 fetch + ReadableStream 读取，
// 因此这里只暴露端点地址，具体的流式解析放在组件里（需要携带 cookie 与请求体）。
export type AssistantChatMessage = { role: 'user' | 'assistant'; content: string }

export function getProjectAssistantURL(projectSlug: string) {
  return `${apiBaseURL}/api/v1/projects/${encodeURIComponent(projectSlug)}/assistant`
}

export function setProjectFavorite(projectSlug: string, favorite: boolean) {
  return apiRequest<void>(`/api/v1/projects/${encodeURIComponent(projectSlug)}/favorite`, {
    method: favorite ? 'POST' : 'DELETE',
  })
}

export function setProjectFollow(projectSlug: string, follow: boolean) {
  return apiRequest<void>(`/api/v1/projects/${encodeURIComponent(projectSlug)}/follow`, {
    method: follow ? 'POST' : 'DELETE',
  })
}

// shareProject 记录一次分享（给分享者加经验，每项目每人一次）。仅登录用户有效。
export function shareProject(projectSlug: string) {
  return apiRequest<void>(`/api/v1/projects/${encodeURIComponent(projectSlug)}/share`, {
    method: 'POST',
  })
}

// setCommentLike 点赞或取消点赞一条文档评论（幂等）。
export function setCommentLike(projectSlug: string, documentSlug: string, commentID: string, like: boolean) {
  return apiRequest<void>(
    `/api/v1/projects/${encodeURIComponent(projectSlug)}/documents/${encodeURIComponent(documentSlug)}/comments/${encodeURIComponent(commentID)}/like`,
    { method: like ? 'POST' : 'DELETE' },
  )
}

export function getServiceInfo(signal?: AbortSignal) {
  return apiRequest<ServiceInfo>('/api/v1', { signal })
}

export function getProjects(
  filters: { query?: string; category?: string; page?: number; pageSize?: number; sort?: 'updated' | 'downloads' | 'stars' },
  signal?: AbortSignal,
) {
  const parameters = new URLSearchParams()
  if (filters.query) parameters.set('q', filters.query)
  if (filters.category) parameters.set('category', filters.category)
  if (filters.page) parameters.set('page', String(filters.page))
  if (filters.pageSize) parameters.set('page_size', String(filters.pageSize))
  if (filters.sort) parameters.set('sort', filters.sort)
  const queryString = parameters.toString()

  return apiRequest<ProjectListResponse>(`/api/v1/projects${queryString ? `?${queryString}` : ''}`, { signal })
}

export function getProject(slug: string, signal?: AbortSignal) {
  return apiRequest<ProjectDetailResponse>(`/api/v1/projects/${encodeURIComponent(slug)}`, { signal })
}

// getUserProfile 获取用户公开主页（昵称、等级、注册时间、已发布项目与统计）。公开、无需登录。
export function getUserProfile(userID: string, signal?: AbortSignal) {
  return apiRequest<PublicUserProfileResponse>(
    `/api/v1/users/${encodeURIComponent(userID)}/profile`,
    { signal },
  )
}

export function getProjectCollaborationAccess(slug: string, signal?: AbortSignal) {
  return apiRequest<CollaborationAccessResponse>(
    `/api/v1/projects/${encodeURIComponent(slug)}/collaboration/access`,
    { signal },
  )
}

export function getProjectCollaborators(slug: string, signal?: AbortSignal) {
  return apiRequest<ProjectCollaboratorListResponse>(
    `/api/v1/projects/${encodeURIComponent(slug)}/collaborators`,
    { signal },
  )
}

export function setProjectCollaborator(
  slug: string,
  input: { email: string; role: 'editor' | 'viewer' },
) {
  return apiRequest<ProjectCollaboratorResponse>(
    `/api/v1/projects/${encodeURIComponent(slug)}/collaborators`,
    { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input) },
  )
}

export function deleteProjectCollaborator(slug: string, userID: string) {
  return apiRequest<void>(
    `/api/v1/projects/${encodeURIComponent(slug)}/collaborators/${encodeURIComponent(userID)}`,
    { method: 'DELETE' },
  )
}

// getProjectCollaborationWebSocketURL 返回协作通道地址。
// documentSlug 为空时协作项目正文；否则协作指定文档，服务端据此分房间。
export function getProjectCollaborationWebSocketURL(slug: string, documentSlug?: string) {
  const base = apiBaseURL ? new URL(apiBaseURL, window.location.origin) : new URL(window.location.origin)
  base.protocol = base.protocol === 'https:' ? 'wss:' : 'ws:'
  base.pathname = `/api/v1/projects/${encodeURIComponent(slug)}/collaboration/ws`
  base.search = documentSlug ? `?document=${encodeURIComponent(documentSlug)}` : ''
  base.hash = ''
  return base.toString()
}

export function getDocuments(projectSlug: string, signal?: AbortSignal) {
  return apiRequest<DocumentListResponse>(`/api/v1/projects/${encodeURIComponent(projectSlug)}/documents`, { signal })
}

export function getDocument(projectSlug: string, documentSlug: string, signal?: AbortSignal) {
  return apiRequest<DocumentDetailResponse>(
    `/api/v1/projects/${encodeURIComponent(projectSlug)}/documents/${encodeURIComponent(documentSlug)}`,
    { signal },
  )
}

export function getDocumentComments(projectSlug: string, documentSlug: string, signal?: AbortSignal) {
  return apiRequest<CommentListResponse>(
    `/api/v1/projects/${encodeURIComponent(projectSlug)}/documents/${encodeURIComponent(documentSlug)}/comments`,
    { signal },
  )
}

export function getDocumentCommentEventsURL(projectSlug: string, documentSlug: string) {
  return `${apiBaseURL}/api/v1/projects/${encodeURIComponent(projectSlug)}/documents/${encodeURIComponent(documentSlug)}/comments/events`
}

export function createDocumentComment(
  projectSlug: string,
  documentSlug: string,
  input: { block_id: string; quote: string; body: string },
) {
  return apiRequest<CommentResponse>(
    `/api/v1/projects/${encodeURIComponent(projectSlug)}/documents/${encodeURIComponent(documentSlug)}/comments`,
    { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input) },
  )
}

export function resolveDocumentComment(projectSlug: string, documentSlug: string, commentID: string) {
  return apiRequest<CommentResponse>(
    `/api/v1/projects/${encodeURIComponent(projectSlug)}/documents/${encodeURIComponent(documentSlug)}/comments/${encodeURIComponent(commentID)}`,
    { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ status: 'resolved' }) },
  )
}

export function deleteDocumentComment(projectSlug: string, documentSlug: string, commentID: string) {
  return apiRequest<void>(
    `/api/v1/projects/${encodeURIComponent(projectSlug)}/documents/${encodeURIComponent(documentSlug)}/comments/${encodeURIComponent(commentID)}`,
    { method: 'DELETE' },
  )
}

export function updateDocumentComment(
  projectSlug: string,
  documentSlug: string,
  commentID: string,
  body: string,
) {
  return apiRequest<CommentResponse>(
    `/api/v1/projects/${encodeURIComponent(projectSlug)}/documents/${encodeURIComponent(documentSlug)}/comments/${encodeURIComponent(commentID)}`,
    { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ body }) },
  )
}

export function createDocumentCommentReply(
  projectSlug: string,
  documentSlug: string,
  commentID: string,
  input: { body: string },
) {
  return apiRequest<CommentResponse>(
    `/api/v1/projects/${encodeURIComponent(projectSlug)}/documents/${encodeURIComponent(documentSlug)}/comments/${encodeURIComponent(commentID)}/replies`,
    { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input) },
  )
}

export function deleteDocumentCommentReply(
  projectSlug: string,
  documentSlug: string,
  commentID: string,
  replyID: string,
) {
  return apiRequest<void>(
    `/api/v1/projects/${encodeURIComponent(projectSlug)}/documents/${encodeURIComponent(documentSlug)}/comments/${encodeURIComponent(commentID)}/replies/${encodeURIComponent(replyID)}`,
    { method: 'DELETE' },
  )
}

export function updateDocumentCommentReply(
  projectSlug: string,
  documentSlug: string,
  commentID: string,
  replyID: string,
  body: string,
) {
  return apiRequest<CommentResponse>(
    `/api/v1/projects/${encodeURIComponent(projectSlug)}/documents/${encodeURIComponent(documentSlug)}/comments/${encodeURIComponent(commentID)}/replies/${encodeURIComponent(replyID)}`,
    { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ body }) },
  )
}
