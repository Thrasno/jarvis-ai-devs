import { type ApiClient, createApiClient, type User } from '../api/client'

type AuthApi = Pick<ApiClient, 'login' | 'currentUser'>

export const sessionTokenKey = 'hive-dashboard.jwt'
export type AuthState =
  | { status: 'anonymous'; error?: string }
  | { status: 'authenticated'; token: string; user: User }

export type SessionStore = {
  getState(): AuthState
  login(email: string, password: string): Promise<AuthState>
  bootstrap(): Promise<AuthState>
  logout(): AuthState
}

export function createSessionStore(options: { api?: AuthApi; storage?: Storage } = {}): SessionStore {
  const api = options.api ?? createApiClient()
  const storage = options.storage ?? sessionStorage
  let state: AuthState = { status: 'anonymous' }

  const setAuthenticated = (token: string, user: User): AuthState => {
    storage.setItem(sessionTokenKey, token)
    state = { status: 'authenticated', token, user }
    return state
  }

  const clear = (): AuthState => {
    storage.removeItem(sessionTokenKey)
    state = { status: 'anonymous' }
    return state
  }

  return {
    getState() {
      return state
    },
    async login(email, password) {
      const response = await api.login(email, password)
      return setAuthenticated(response.token, response.user)
    },
    async bootstrap() {
      const token = storage.getItem(sessionTokenKey)
      if (!token) {
        state = { status: 'anonymous' }
        return state
      }
      try {
        return setAuthenticated(token, await api.currentUser(token))
      } catch {
        return clear()
      }
    },
    logout() {
      return clear()
    }
  }
}
