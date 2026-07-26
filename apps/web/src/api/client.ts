export type ServiceInfo = {
  service: string
  api_version: string
  status: string
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

  return response.json() as Promise<T>
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

export function getDocuments(projectSlug: string, signal?: AbortSignal) {
  return apiRequest<DocumentListResponse>(`/api/v1/projects/${encodeURIComponent(projectSlug)}/documents`, { signal })
}

export function getDocument(projectSlug: string, documentSlug: string, signal?: AbortSignal) {
  return apiRequest<DocumentDetailResponse>(
    `/api/v1/projects/${encodeURIComponent(projectSlug)}/documents/${encodeURIComponent(documentSlug)}`,
    { signal },
  )
}
