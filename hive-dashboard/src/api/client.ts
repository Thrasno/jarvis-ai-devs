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
type Fetcher = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
export type ApiClient = {
  login(email: string, password: string): Promise<LoginResponse>
  currentUser(token: string): Promise<User>
}

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
    }
  }
}

async function json(response: Response): Promise<unknown> {
  const text = await response.text()
  return text === '' ? null : (JSON.parse(text) as unknown)
}

function errorMessage(payload: unknown): string | null {
  const value = payload && typeof payload === 'object' && 'error' in payload ? payload.error : null
  return typeof value === 'string' ? value : null
}
