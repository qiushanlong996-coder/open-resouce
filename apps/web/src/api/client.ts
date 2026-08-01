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
  type: 'comment.replied' | 'project.approved' | 'project.rejected'
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

export function getPendingProjectReviews(signal?: AbortSignal) {
  return apiRequest<ManagedProjectListResponse>('/api/v1/admin/reviews', { signal })
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

export function setProjectFavorite(projectSlug: string, favorite: boolean) {
  return apiRequest<void>(`/api/v1/projects/${encodeURIComponent(projectSlug)}/favorite`, {
    method: favorite ? 'POST' : 'DELETE',
  })
}

// shareProject 记录一次分享（给分享者加经验，每项目每人一次）。仅登录用户有效。
export function shareProject(projectSlug: string) {
  return apiRequest<void>(`/api/v1/projects/${encodeURIComponent(projectSlug)}/share`, {
    method: 'POST',
  })
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
