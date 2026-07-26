export type ServiceInfo = {
  service: string
  api_version: string
  status: string
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

