import { createApiClient, type ApiClient, type AuditLogList, type MemoryList, type MemorySearch, type User } from './api/client'
import { createSessionStore, type AuthState, type SessionStore } from './auth/session'
import { renderAuditSync } from './views/AuditSync'
import { renderMemories } from './views/Memories'
import { renderOverview, type OverviewData, type ViewState } from './views/Overview'
import { renderUsers } from './views/Users'
import './styles.css'

export const DEFAULT_MEMORY_SEARCH_QUERY = 'dashboard'

export type UsersData = { users: User[] }
export type MemoriesData = { recent: MemoryList; search: MemorySearch }
export type AuditSyncData = AuditLogList
export type LoadedDashboardData = {
  overview: ViewState<OverviewData>
  users: ViewState<UsersData>
  memories: ViewState<MemoriesData>
  audit: ViewState<AuditSyncData>
}
export type DashboardState = { status: 'loading' } | { status: 'ready'; data: LoadedDashboardData }

type AppActions = {
  onLogin(email: string, password: string): Promise<void> | void
  onLogout(): void
  onNavigate?(path: string): void
}

export function renderApp(container: HTMLElement, state: AuthState, actions: AppActions, dashboard: DashboardState = { status: 'loading' }, routePath = window.location.pathname, loginError = ''): void {
  container.replaceChildren()
  state.status === 'anonymous' ? renderLogin(container, actions, loginError) : renderShell(container, state, actions, dashboard, routePath)
}

function renderLogin(container: HTMLElement, actions: AppActions, loginError: string): void {
  const form = document.createElement('form')
  form.className = 'card login-card'
  form.innerHTML = `
    <p class="eyebrow">Hive API</p>
    <h1>Sign in to Hive API</h1>
    ${loginError ? `<p class="error" role="alert">${escapeHtml(loginError)}</p>` : ''}
    <label>Email<input name="email" type="email" autocomplete="email" required /></label>
    <label>Password<input name="password" type="password" autocomplete="current-password" required /></label>
    <button type="submit">Sign in</button>
  `
  form.addEventListener('submit', async (event) => {
    event.preventDefault()
    const data = new FormData(form)
    await actions.onLogin(String(data.get('email') ?? ''), String(data.get('password') ?? ''))
  })
  container.append(form)
}

function renderShell(container: HTMLElement, state: Extract<AuthState, { status: 'authenticated' }>, actions: AppActions, dashboard: DashboardState, routePath: string): void {
  const header = document.createElement('header')
  header.innerHTML = '<p class="eyebrow">Hive API</p><h1>Hive API Dashboard</h1>'
  const identity = document.createElement('p')
  identity.textContent = `Signed in as ${state.user.email}`
  const logout = document.createElement('button')
  logout.type = 'button'
  logout.textContent = 'Sign out'
  logout.addEventListener('click', actions.onLogout)
  header.append(identity, logout)

  const panel = state.user.level === 'admin' ? renderAdminView(routePath, dashboard, actions) : document.createElement('article')
  if (state.user.level !== 'admin') {
    panel.className = 'card'
    panel.innerHTML = '<h2>Admin access required</h2><p>This dashboard requires an admin Hive API identity.</p>'
  }

  const shell = document.createElement('section')
  shell.className = 'shell'
  shell.append(header, panel)
  container.append(shell)
}

function renderAdminView(routePath: string, state: DashboardState, actions: AppActions): HTMLElement {
  const wrapper = document.createElement('article')
  wrapper.append(nav(actions))
  const path = routePath.replace(/\/$/, '')
  if (path.endsWith('/users')) wrapper.append(renderUsers(stateFor(state, 'users')))
  else if (path.endsWith('/memories')) wrapper.append(renderMemories(stateFor(state, 'memories')))
  else if (path.endsWith('/audit-sync')) wrapper.append(renderAuditSync(stateFor(state, 'audit')))
  else wrapper.append(renderOverview(stateFor(state, 'overview')))
  return wrapper
}

function stateFor<K extends keyof LoadedDashboardData>(state: DashboardState, key: K): LoadedDashboardData[K] | { status: 'loading' } {
  return state.status === 'ready' ? state.data[key] : { status: 'loading' }
}

function nav(actions: AppActions): HTMLElement {
  const node = document.createElement('nav')
  const links = [['Overview', '/dashboard'], ['Users', '/dashboard/users'], ['Memories', '/dashboard/memories'], ['Audit & sync', '/dashboard/audit-sync']]
  for (const [label, href] of links) {
    const link = document.createElement('a')
    link.href = href
    link.textContent = label
    link.addEventListener('click', (event) => {
      if (!actions.onNavigate) return
      event.preventDefault()
      actions.onNavigate(href)
    })
    node.append(link)
  }
  return node
}

export async function loadDashboard(api: ApiClient, token: string): Promise<DashboardState> {
  const [health, stats, users, recent, search, audit] = await Promise.allSettled([
    api.health(), api.adminStats(token), api.adminUsers(token), api.memories(token, { limit: 5 }), api.searchMemories(token, DEFAULT_MEMORY_SEARCH_QUERY, { limit: 5 }), api.auditLogs(token, { limit: 10 })
  ])
  return {
    status: 'ready',
    data: {
      overview: combinedState(health, stats, (health, stats) => ({ health, stats })),
      users: settledState(users),
      memories: combinedState(recent, search, (recent, search) => ({ recent, search })),
      audit: settledState(audit)
    }
  }
}

function settledState<T>(result: PromiseSettledResult<T>): ViewState<T> {
  return result.status === 'fulfilled' ? { status: 'ready', data: result.value } : { status: 'error', message: messageFor(result.reason) }
}

function combinedState<A, B, T>(first: PromiseSettledResult<A>, second: PromiseSettledResult<B>, combine: (first: A, second: B) => T): ViewState<T> {
  if (first.status === 'rejected') return { status: 'error', message: messageFor(first.reason) }
  if (second.status === 'rejected') return { status: 'error', message: messageFor(second.reason) }
  return { status: 'ready', data: combine(first.value, second.value) }
}

function messageFor(error: unknown): string {
  return error instanceof Error ? error.message : 'dashboard data unavailable'
}

function loginMessageFor(error: unknown): string {
  return error instanceof Error ? error.message : 'Unable to sign in'
}

function escapeHtml(value: string): string {
  return value.replace(/[&<>"]/g, (char) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[char] ?? char))
}

type StartOptions = { api?: ApiClient; session?: SessionStore }

export function startDashboardApp(root: HTMLElement, options: StartOptions = {}): void {
  const api = options.api ?? createApiClient()
  const session = options.session ?? createSessionStore({ api })
  let dashboard: DashboardState = { status: 'loading' }
  let loadVersion = 0
  let loginError = ''
  const rerender = (state: AuthState) => renderApp(root, state, actions, dashboard, window.location.pathname, loginError)
  const actions: AppActions = {
    async onLogin(email, password) {
      loginError = ''
      try {
        await setState(await session.login(email, password))
      } catch (error) {
        loginError = loginMessageFor(error)
        rerender({ status: 'anonymous' })
      }
    },
    onLogout() {
      loadVersion += 1
      dashboard = { status: 'loading' }
      loginError = ''
      rerender(session.logout())
    },
    onNavigate(path) {
      history.pushState(null, '', path)
      rerender(session.getState())
    }
  }

  async function setState(state: AuthState): Promise<void> {
    const version = loadVersion + 1
    loadVersion = version
    rerender(state)
    if (state.status === 'authenticated' && state.user.level === 'admin') {
      const loaded = await loadDashboard(api, state.token)
      const current = session.getState()
      if (version !== loadVersion || current.status !== 'authenticated' || current.token !== state.token) return
      dashboard = loaded
      rerender(current)
    }
  }

  window.addEventListener('popstate', () => rerender(session.getState()))
  session.bootstrap().then(setState).catch(() => {
    loginError = ''
    rerender({ status: 'anonymous' })
  })
}

const root = document.getElementById('app')
if (root) {
  startDashboardApp(root)
}
