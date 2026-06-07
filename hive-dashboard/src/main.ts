import { createApiClient, type ApiClient } from './api/client'
import { createSessionStore, type AuthState } from './auth/session'
import { renderAuditSync } from './views/AuditSync'
import { renderMemories } from './views/Memories'
import { renderOverview, type OverviewData, type ViewState } from './views/Overview'
import { renderUsers } from './views/Users'
import './styles.css'

type DashboardData = OverviewData & {
  users: Parameters<typeof renderUsers>[0] extends ViewState<infer T> ? T : never
  memories: Parameters<typeof renderMemories>[0] extends ViewState<infer T> ? T : never
  audit: Parameters<typeof renderAuditSync>[0] extends ViewState<infer T> ? T : never
}
type DashboardState = ViewState<DashboardData>

type AppActions = {
  onLogin(email: string, password: string): Promise<void> | void
  onLogout(): void
  onNavigate?(path: string): void
}

export function renderApp(container: HTMLElement, state: AuthState, actions: AppActions, dashboard: DashboardState = { status: 'loading' }, routePath = window.location.pathname): void {
  container.replaceChildren()
  state.status === 'anonymous' ? renderLogin(container, actions) : renderShell(container, state, actions, dashboard, routePath)
}

function renderLogin(container: HTMLElement, actions: AppActions): void {
  const form = document.createElement('form')
  form.className = 'card login-card'
  form.innerHTML = `
    <p class="eyebrow">Hive API</p>
    <h1>Sign in to Hive API</h1>
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
  const ready = state.status === 'ready' ? state.data : null
  const stateFor = <T>(data: T | null): ViewState<T> => state.status === 'error' ? state : ready ? { status: 'ready', data: data as T } : { status: 'loading' }
  const path = routePath.replace(/\/$/, '')
  if (path.endsWith('/users')) wrapper.append(renderUsers(stateFor(ready?.users ?? null)))
  else if (path.endsWith('/memories')) wrapper.append(renderMemories(stateFor(ready?.memories ?? null)))
  else if (path.endsWith('/audit-sync')) wrapper.append(renderAuditSync(stateFor(ready?.audit ?? null)))
  else wrapper.append(renderOverview(stateFor(ready ? { health: ready.health, stats: ready.stats } : null)))
  return wrapper
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

async function loadDashboard(api: ApiClient, token: string): Promise<DashboardState> {
  try {
    const [health, stats, users, recent, search, audit] = await Promise.all([
      api.health(), api.adminStats(token), api.adminUsers(token), api.memories(token, { limit: 5 }), api.searchMemories(token, 'dashboard', { limit: 5 }), api.auditLogs(token, { limit: 10 })
    ])
    return { status: 'ready', data: { health, stats, users, memories: { recent, search }, audit } }
  } catch (error) {
    return { status: 'error', message: error instanceof Error ? error.message : 'dashboard data unavailable' }
  }
}

const root = document.getElementById('app')
if (root) {
  const api = createApiClient()
  const session = createSessionStore({ api })
  let dashboard: DashboardState = { status: 'loading' }
  const rerender = (state: AuthState) => renderApp(root, state, actions, dashboard)
  const actions: AppActions = {
    async onLogin(email, password) {
      await setState(await session.login(email, password))
    },
    onLogout() {
      dashboard = { status: 'loading' }
      rerender(session.logout())
    },
    onNavigate(path) {
      history.pushState(null, '', path)
      rerender(session.getState())
    }
  }

  async function setState(state: AuthState): Promise<void> {
    rerender(state)
    if (state.status === 'authenticated' && state.user.level === 'admin') {
      dashboard = await loadDashboard(api, state.token)
      rerender(state)
    }
  }

  window.addEventListener('popstate', () => rerender(session.getState()))
  session.bootstrap().then(setState)
}
