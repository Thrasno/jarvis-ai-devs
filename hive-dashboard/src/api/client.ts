import { appendDashboardFilters, type DashboardUrlFilters } from './urlFilters'

export type UserLevel = 'viewer' | 'member' | 'admin'
export type User = {
  id: string
  username: string
  email: string
  level: UserLevel
  is_active: boolean
  created_at: string
}
export type LoginResponse = { token: string; expires_at?: string; user: User }
export type Health = { status: string; db: string; version: string }
export type AdminStats = { users: { total: number; active: number; by_level: Record<string, number> }; memories: { total: number; by_project: Count[]; by_category: Count[]; last_synced_at: string | null } }
export type Count = { project?: string; category?: string; count: number }
export type Memory = { id: string; sync_id: string; project: string; category: string; title: string; content: string; tags: string[]; files_affected: string[]; created_by: string; created_at: string; updated_at: string; synced_at: string }
export type MemoryList = { memories: Memory[]; total: number; limit: number; offset: number }
export type MemorySearch = { memories: Memory[]; total: number; query: string; limit: number }
export type AuditLog = { id: string; occurred_at: string; action: string; outcome: string; entry_count: number; metadata: Record<string, unknown> }
export type AuditLogList = { audit_logs: AuditLog[]; total: number; limit: number; offset: number }
export type MutationMessage = { message: string }
export type MemoryListParams = { project?: string; category?: string; limit?: number; offset?: number }
export type MemorySearchParams = { query: string; project?: string; limit?: number; offset?: number }
export type AuditLogParams = { project?: string; actor_user_id?: string; action?: string; outcome?: string; since?: string; until?: string; limit?: number; offset?: number }
export type ApiErrorCode = 'NETWORK_ERROR' | 'NON_JSON_RESPONSE' | 'UNAUTHORIZED' | 'FORBIDDEN' | 'VALIDATION_ERROR' | 'NOT_FOUND' | 'CONFLICT' | 'SERVER_ERROR' | 'REQUEST_FAILED' | string
type Fetcher = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
export type ApiClient = {
  login(email: string, password: string): Promise<LoginResponse>
  currentUser(token: string): Promise<User>
  health(): Promise<Health>
  adminStats(token: string): Promise<AdminStats>
  adminUsers(token: string): Promise<{ users: User[] }>
  setUserLevel(token: string, username: string, level: UserLevel): Promise<MutationMessage>
  grantAdmin(token: string, username: string): Promise<MutationMessage>
  deactivateUser(token: string, username: string): Promise<MutationMessage>
  memories(token: string, params?: MemoryListParams): Promise<MemoryList>
  searchMemories(token: string, params: MemorySearchParams): Promise<MemorySearch>
  memory(token: string, id: string): Promise<Memory>
  auditLogs(token: string, params?: AuditLogParams): Promise<AuditLogList>
}

export class ApiError extends Error {
  constructor(message: string, readonly status: number, readonly code: ApiErrorCode = codeForStatus(status), readonly details?: unknown) {
    super(message)
    this.name = 'ApiError'
  }
}

export function createApiClient(options: { baseUrl?: string; fetch?: Fetcher } = {}): ApiClient {
  const baseUrl = options.baseUrl ?? ''
  const fetcher = options.fetch ?? fetch

  async function request<T>(path: string, init: RequestInit): Promise<T> {
    let response: Response
    try {
      response = await fetcher(`${baseUrl}${path}`, init)
    } catch (error) {
      throw normalizeNetworkError(error)
    }

    const payload = await readPayload(response)

    if (!response.ok) {
      throw normalizeResponseError(response, payload)
    }
    return payload as T
  }

  return {
    login(email, password) {
      return request<LoginResponse>('/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, password })
      })
    },
    currentUser(token) {
      return request<User>('/auth/me', {
        method: 'GET',
        headers: { Authorization: `Bearer ${token}` }
      })
    },
    health() {
      return request<Health>('/health', { method: 'GET' })
    },
    adminStats(token) {
      return request<AdminStats>('/admin/stats', authGet(token))
    },
    adminUsers(token) {
      return request<{ users: User[] }>('/admin/users', authGet(token))
    },
    setUserLevel(token, username, level) {
      return request<MutationMessage>(`/admin/users/${encodeURIComponent(username)}/level`, authPost(token, { level }))
    },
    grantAdmin(token, username) {
      return request<MutationMessage>(`/admin/users/${encodeURIComponent(username)}/grant-admin`, authPost(token))
    },
    deactivateUser(token, username) {
      return request<MutationMessage>(`/admin/users/${encodeURIComponent(username)}/deactivate`, authPost(token))
    },
    memories(token, params = {}) {
      return request<MemoryList>(withQuery('/memories', params), authGet(token))
    },
    searchMemories(token, params) {
      return request<MemorySearch>(withQuery('/memories/search', params), authGet(token))
    },
    memory(token, id) {
      return request<Memory>(`/memories/${encodeURIComponent(id)}`, authGet(token))
    },
    auditLogs(token, params = {}) {
      return request<AuditLogList>(withQuery('/admin/audit-logs', params), authGet(token))
    }
  }
}

function authGet(token: string): RequestInit {
  return { method: 'GET', headers: { Authorization: `Bearer ${token}` } }
}

function authPost(token: string, body?: unknown): RequestInit {
  const init: RequestInit = { method: 'POST', headers: { Authorization: `Bearer ${token}` } }
  if (body !== undefined) {
    init.headers = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' }
    init.body = JSON.stringify(body)
  }
  return init
}

function withQuery(path: string, params: DashboardUrlFilters): string {
  return appendDashboardFilters(path, params)
}

async function readPayload(response: Response): Promise<unknown> {
  const text = await response.text()
  if (text === '') return null
  try {
    return JSON.parse(text) as unknown
  } catch {
    return { nonJsonBody: text }
  }
}

function normalizeNetworkError(error: unknown): ApiError {
  if (error instanceof ApiError) return error
  return new ApiError('Network request failed', 0, 'NETWORK_ERROR', error instanceof Error ? error.message : undefined)
}

function normalizeResponseError(response: Response, payload: unknown): ApiError {
  const message = stringField(payload, 'error') ?? stringField(payload, 'message') ?? (response.statusText || 'Request failed')
  if (objectField(payload, 'nonJsonBody') !== undefined) {
    return new ApiError(message, response.status, 'NON_JSON_RESPONSE')
  }
  return new ApiError(message, response.status, stringField(payload, 'code') ?? codeForStatus(response.status), objectField(payload, 'details'))
}

function codeForStatus(status: number): ApiErrorCode {
  if (status === 0) return 'NETWORK_ERROR'
  if (status === 400 || status === 422) return 'VALIDATION_ERROR'
  if (status === 401) return 'UNAUTHORIZED'
  if (status === 403) return 'FORBIDDEN'
  if (status === 404) return 'NOT_FOUND'
  if (status === 409) return 'CONFLICT'
  if (status >= 500) return 'SERVER_ERROR'
  return 'REQUEST_FAILED'
}

function stringField(payload: unknown, key: string): string | null {
  if (!payload || typeof payload !== 'object' || !(key in payload)) return null
  const value = payload[key as keyof typeof payload]
  return typeof value === 'string' && value !== '' ? value : null
}

function objectField(payload: unknown, key: string): unknown {
  if (!payload || typeof payload !== 'object' || !(key in payload)) return undefined
  return payload[key as keyof typeof payload]
}
