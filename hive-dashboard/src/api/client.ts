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
type Fetcher = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
export type ApiClient = {
  login(email: string, password: string): Promise<LoginResponse>
  currentUser(token: string): Promise<User>
  health(): Promise<Health>
  adminStats(token: string): Promise<AdminStats>
  adminUsers(token: string): Promise<{ users: User[] }>
  memories(token: string, params?: QueryParams): Promise<MemoryList>
  searchMemories(token: string, query: string, params?: QueryParams): Promise<MemorySearch>
  auditLogs(token: string, params?: QueryParams): Promise<AuditLogList>
}
export type QueryParams = Record<string, string | number | undefined>

export class ApiError extends Error {
  constructor(message: string, readonly status: number) {
    super(message)
    this.name = 'ApiError'
  }
}

export function createApiClient(options: { baseUrl?: string; fetch?: Fetcher } = {}): ApiClient {
  const baseUrl = options.baseUrl ?? ''
  const fetcher = options.fetch ?? fetch

  async function request<T>(path: string, init: RequestInit): Promise<T> {
    const response = await fetcher(`${baseUrl}${path}`, init)
    const payload = await json(response)

    if (!response.ok) {
      throw new ApiError(errorMessage(payload) ?? response.statusText ?? 'request failed', response.status)
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
    memories(token, params = {}) {
      return request<MemoryList>(withQuery('/memories', params), authGet(token))
    },
    searchMemories(token, query, params = {}) {
      return request<MemorySearch>(withQuery('/memories/search', { query, ...params }), authGet(token))
    },
    auditLogs(token, params = {}) {
      return request<AuditLogList>(withQuery('/admin/audit-logs', params), authGet(token))
    }
  }
}

function authGet(token: string): RequestInit {
  return { method: 'GET', headers: { Authorization: `Bearer ${token}` } }
}

function withQuery(path: string, params: QueryParams): string {
  const query = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== '') query.set(key, String(value))
  }
  const suffix = query.toString()
  return suffix ? `${path}?${suffix}` : path
}

async function json(response: Response): Promise<unknown> {
  const text = await response.text()
  return text === '' ? null : (JSON.parse(text) as unknown)
}

function errorMessage(payload: unknown): string | null {
  const value = payload && typeof payload === 'object' && 'error' in payload ? payload.error : null
  return typeof value === 'string' ? value : null
}
