import { appendDashboardFilters, type DashboardUrlFilters, type MemoryDiscoveryUrlFilters } from './urlFilters'

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
export type OverviewProjectSyncHealth = { project: string; status: 'healthy' | 'degraded' | 'unknown' | string; region: string; contributor_count: number }
export type OverviewStats = {
  daemon_health: { healthy: number; total: number }
  conflicts: { open: number }
  sync_health_by_project: OverviewProjectSyncHealth[]
  live_activity: { count: number; newest_sync_id: string }
  most_active_projects: Count[]
}
export type OverviewGrowth = { knowledge_growth: { label: string; value: number }[] }
export type Memory = { id: string; sync_id: string; project: string; category: string; title: string; content: string; tags: string[]; files_affected: string[]; created_by: string; created_at: string; updated_at: string; synced_at: string }
export type MemoryList = { memories: Memory[]; total: number; limit: number; offset: number }
export type MemorySearch = { memories: Memory[]; total: number; query: string; limit: number; offset: number }
export type AuditLog = { id: string; occurred_at: string; action: string; outcome: string; entry_count: number; metadata: Record<string, unknown> }
export type AuditLogList = { audit_logs: AuditLog[]; total: number; limit: number; offset: number }
export type SyncAttemptSummaryWindowKey = '24h' | '7d' | '30d'
export type SyncAttemptDimensionCount = { key: string; count: number }
export type SyncAttemptSummaryWindow = {
  window: SyncAttemptSummaryWindowKey
  total: number
  successes: number
  failures: number
  failure_rate: number
  last_success_at?: string | null
  last_failure_at?: string | null
  by_developer: SyncAttemptDimensionCount[]
  by_project: SyncAttemptDimensionCount[]
  by_client: SyncAttemptDimensionCount[]
  by_daemon: SyncAttemptDimensionCount[]
  by_outcome: SyncAttemptDimensionCount[]
  by_error_code: SyncAttemptDimensionCount[]
  top_errors: SyncAttemptDimensionCount[]
}
export type SyncAttemptSummary = { windows: SyncAttemptSummaryWindow[] }
export type MutationMessage = { message: string }
export type ProjectSummary = {
  name: string
  memoryCount: number
  sessionCount: number
  lastActivityAt?: string | null
  syncHealth?: 'healthy' | 'degraded' | 'unknown' | string | null
}
export type ProjectListResponse = { projects: ProjectSummary[]; total: number }
export type ActivityFeedParams = { limit?: number; cursor?: string }
export type ActivityFeedEntry = {
  id: string
  event_type: 'create' | 'update' | 'delete' | string
  occurred_at: string
  actor: string
  project: string
  category: string
  title: string
  summary: string
  memory_sync_id?: string | null
}
export type ActivityFeedResponse = { entries: ActivityFeedEntry[]; next_cursor?: string | null }
export type MemoryListParams = MemoryDiscoveryUrlFilters
export type MemorySearchParams = Required<Pick<MemoryDiscoveryUrlFilters, 'query'>> & Omit<MemoryDiscoveryUrlFilters, 'query'>
export type AuditLogParams = { project?: string; actor_user_id?: string; action?: string; outcome?: string; since?: string; until?: string; limit?: number; offset?: number }
export type SyncAttemptSummaryParams = { window?: SyncAttemptSummaryWindowKey; project?: string; dev_id?: string; client?: string; daemon_id?: string; outcome?: 'success' | 'failure'; error_code?: string }
export type ApiErrorCode = 'NETWORK_ERROR' | 'NON_JSON_RESPONSE' | 'UNAUTHORIZED' | 'FORBIDDEN' | 'VALIDATION_ERROR' | 'NOT_FOUND' | 'CONFLICT' | 'SERVER_ERROR' | 'REQUEST_FAILED' | string
type Fetcher = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
export type ApiClient = {
  login(email: string, password: string): Promise<LoginResponse>
  currentUser(token: string): Promise<User>
  health(): Promise<Health>
  adminStats(token: string): Promise<AdminStats>
  overviewStats(token: string): Promise<OverviewStats>
  overviewGrowth(token: string): Promise<OverviewGrowth>
  adminUsers(token: string): Promise<{ users: User[] }>
  setUserLevel(token: string, username: string, level: UserLevel): Promise<MutationMessage>
  grantAdmin(token: string, username: string): Promise<MutationMessage>
  deactivateUser(token: string, username: string): Promise<MutationMessage>
  memories(token: string, params?: MemoryListParams): Promise<MemoryList>
  searchMemories(token: string, params: MemorySearchParams): Promise<MemorySearch>
  memory(token: string, id: string): Promise<Memory>
  auditLogs(token: string, params?: AuditLogParams): Promise<AuditLogList>
  syncAttemptSummary(token: string, params?: SyncAttemptSummaryParams): Promise<SyncAttemptSummary>
  activity(token: string, params?: ActivityFeedParams): Promise<ActivityFeedResponse>
  projects(token: string): Promise<ProjectListResponse>
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
    overviewStats(token) {
      return request<OverviewStats>('/admin/overview/stats', authGet(token))
    },
    overviewGrowth(token) {
      return request<OverviewGrowth>('/admin/overview/growth', authGet(token))
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
      return request<MemoryList>(withMemoryListQuery('/memories', params), authGet(token))
    },
    searchMemories(token, params) {
      return request<MemorySearch>(withMemoryDiscoveryQuery('/memories/search', params), authGet(token))
    },
    memory(token, id) {
      return request<Memory>(`/memories/${encodeURIComponent(id)}`, authGet(token))
    },
    auditLogs(token, params = {}) {
      return request<AuditLogList>(withQuery('/admin/audit-logs', params), authGet(token))
    },
    syncAttemptSummary(token, params = {}) {
      return request<SyncAttemptSummary>(withQuery('/admin/sync-attempts/summary', params), authGet(token))
    },
    activity(token, params = {}) {
      return request<ActivityFeedResponse>(activityPath(params), authGet(token))
    },
    projects(token) {
      return request<ProjectListResponse>('/projects', authGet(token))
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

function withMemoryListQuery(path: string, params: MemoryListParams): string {
  return appendDashboardFilters(path, {
    query: params.query,
    project: params.project,
    category: params.category,
    from: params.from,
    until: params.until,
    limit: params.limit,
    offset: params.offset
  })
}

function withMemoryDiscoveryQuery(path: string, params: MemoryDiscoveryUrlFilters): string {
  return appendDashboardFilters(path, {
    query: params.query,
    project: params.project,
    category: params.category,
    from: params.from,
    until: params.until,
    limit: params.limit,
    offset: params.offset
  })
}

function activityPath(params: ActivityFeedParams): string {
  const query = new URLSearchParams()
  query.set('limit', String(params.limit ?? 20))
  if (params.cursor) query.set('cursor', params.cursor)
  return `/activity?${query.toString()}`
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
