import { createApiClient, type ApiClient, type AuditLogList, type MemoryList, type MemorySearch, type User } from './api/client'
import { createSessionStore, type AuthState, type SessionStore } from './auth/session'
import { control } from './components/dom'
import { comingSoon } from './components/ComingSoon'
import { renderNotificationDrawer } from './components/NotificationDrawer'
import { renderSidebar, type UserLevel } from './components/Sidebar'
import type { DashboardScreenKey } from './domain/dashboard'
import { dashboardFixtures } from './fixtures/hive-dashboard/index'
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
  overview?: ViewState<OverviewData>
  users?: ViewState<UsersData>
  memories?: ViewState<MemoriesData>
  audit?: ViewState<AuditSyncData>
}
export type DashboardState = { status: 'loading' } | { status: 'ready'; data: Partial<LoadedDashboardData> }

type AppActions = {
  onLogin(email: string, password: string): Promise<void> | void
  onLogout(): void
  onNavigate?(path: string): void
  onToggleDrawer?(): void
  onMarkAllRead?(): void
}

type DrawerState = {
  drawerOpen: boolean
  readIds: ReadonlySet<string>
  summaryUnread?: number
}

type ScreenRoute = {
  path: string
  load?: keyof LoadedDashboardData
  render: (vs: ViewState<unknown>) => HTMLElement
  placeholderLabel?: string
}

export const ROUTES: Record<DashboardScreenKey, ScreenRoute> = {
  overview: {
    path: '/dashboard',
    load: 'overview',
    render: (vs) => renderOverview(vs as ViewState<OverviewData>)
  },
  memories: {
    path: '/dashboard/memories',
    load: 'memories',
    render: (vs) => renderMemories(vs as ViewState<MemoriesData>)
  },
  userManagement: {
    path: '/dashboard/userManagement',
    load: 'users',
    render: (vs) => renderUsers(vs as ViewState<UsersData>)
  },
  auditLog: {
    path: '/dashboard/auditLog',
    load: 'audit',
    render: (vs) => renderAuditSync(vs as ViewState<AuditSyncData>)
  },
  projects: {
    path: '/dashboard/projects',
    placeholderLabel: 'Projects',
    render: () => comingSoon('Projects')
  },
  knowledgeBrowser: {
    path: '/dashboard/knowledgeBrowser',
    placeholderLabel: 'Knowledge Browser',
    render: () => comingSoon('Knowledge Browser')
  },
  globalSearch: {
    path: '/dashboard/globalSearch',
    placeholderLabel: 'Global Search',
    render: () => comingSoon('Global Search')
  },
  knowledgeGraph: {
    path: '/dashboard/knowledgeGraph',
    placeholderLabel: 'Knowledge Graph',
    render: () => comingSoon('Knowledge Graph')
  },
  activityFeed: {
    path: '/dashboard/activityFeed',
    placeholderLabel: 'Activity Feed',
    render: () => comingSoon('Activity Feed')
  },
  contributors: {
    path: '/dashboard/contributors',
    placeholderLabel: 'Contributors',
    render: () => comingSoon('Contributors')
  },
  developerTimeline: {
    path: '/dashboard/developerTimeline',
    placeholderLabel: 'Developer Timeline',
    render: () => comingSoon('Developer Timeline')
  },
  syncStatus: {
    path: '/dashboard/syncStatus',
    placeholderLabel: 'Sync Status',
    render: () => comingSoon('Sync Status')
  },
  analytics: {
    path: '/dashboard/analytics',
    placeholderLabel: 'Analytics',
    render: () => comingSoon('Analytics')
  },
  conflictViewer: {
    path: '/dashboard/conflictViewer',
    placeholderLabel: 'Conflict Viewer',
    render: () => comingSoon('Conflict Viewer')
  }
}

/**
 * Resolve the active DashboardScreenKey from a URL path.
 * Strips trailing slash, matches exact path against ROUTES table.
 * Falls back to 'overview'.
 */
function screenFromPath(routePath: string): DashboardScreenKey {
  const normalized = routePath.replace(/\/$/, '')
  // Handle legacy /dashboard/audit-sync alias
  if (normalized.endsWith('/audit-sync')) return 'auditLog'
  // Handle legacy /dashboard/users alias
  if (normalized.endsWith('/users')) return 'userManagement'
  for (const [key, route] of Object.entries(ROUTES) as [DashboardScreenKey, ScreenRoute][]) {
    if (route.path === normalized) return key
  }
  return 'overview'
}

export function renderApp(
  container: HTMLElement,
  state: AuthState,
  actions: AppActions,
  dashboard: DashboardState = { status: 'loading' },
  routePath = window.location.pathname,
  drawerState: DrawerState = { drawerOpen: false, readIds: new Set() }
): void {
  container.replaceChildren()
  state.status === 'anonymous'
    ? renderLogin(container, state, actions)
    : renderShell(container, state, actions, dashboard, routePath, drawerState)
}

function renderLogin(
  container: HTMLElement,
  state: Extract<AuthState, { status: 'anonymous' }>,
  actions: AppActions
): void {
  const form = document.createElement('form')
  form.className = 'dashboard-panel panel login-card'
  form.dataset.dashboardPrimitive = 'panel'
  form.innerHTML = `
    <p class="eyebrow">Hive API</p>
    <h1>Sign in to Hive API</h1>
    ${state.error ? `<p class="error" role="alert">${escapeHtml(state.error)}</p>` : ''}
    <label>Email<input name="email" type="email" autocomplete="email" required /></label>
    <label>Password<input name="password" type="password" autocomplete="current-password" required /></label>
    <button type="submit">Sign in</button>
  `
  form.querySelector('button')?.classList.add('dashboard-control', 'control')
  form.querySelector('button')?.setAttribute('data-dashboard-primitive', 'control')
  form.addEventListener('submit', async (event) => {
    event.preventDefault()
    const data = new FormData(form)
    await actions.onLogin(String(data.get('email') ?? ''), String(data.get('password') ?? ''))
  })
  container.append(form)
}

function renderShell(
  container: HTMLElement,
  state: Extract<AuthState, { status: 'authenticated' }>,
  actions: AppActions,
  dashboard: DashboardState,
  routePath: string,
  drawerState: DrawerState
): void {
  const activeScreen = screenFromPath(routePath)
  const userLevel = state.user.level as UserLevel
  const notificationSummary = dashboardFixtures.shared.notificationSummary
  const unreadCount = drawerState.summaryUnread !== undefined
    ? drawerState.summaryUnread
    : notificationSummary.unread

  // Root layout
  const layout = document.createElement('div')
  layout.className = 'dashboard-app-layout'
  layout.dataset.dashboardPrimitive = 'layout'

  // Sidebar
  const sidebarContainer = document.createElement('div')
  renderSidebar(sidebarContainer, {
    groups: dashboardFixtures.shared.navigationGroups,
    currentPath: ROUTES[activeScreen].path,
    userLevel,
    profile: dashboardFixtures.shared.profile,
    onNavigate: (path) => actions.onNavigate?.(path),
    onLogout: actions.onLogout
  })
  layout.append(sidebarContainer)

  // Main area (header + content)
  const mainArea = document.createElement('div')
  mainArea.className = 'dashboard-main-area'

  // Header band
  const header = document.createElement('header')
  header.className = 'dashboard-header'
  header.dataset.dashboardPrimitive = 'header'
  header.setAttribute('role', 'banner')
  header.innerHTML = `<p class="eyebrow">Hive API</p><h1 class="dashboard-header__title">Hive API Dashboard</h1>`

  // Search slot
  const searchSlot = document.createElement('input')
  searchSlot.type = 'search'
  searchSlot.className = 'dashboard-header__search'
  searchSlot.placeholder = 'Search memories…'
  searchSlot.setAttribute('aria-label', 'Search memories')
  searchSlot.addEventListener('click', () => actions.onNavigate?.('/dashboard/globalSearch'))
  searchSlot.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') actions.onNavigate?.('/dashboard/globalSearch')
  })

  // Bell button with optional unread badge
  const bellWrapper = document.createElement('div')
  bellWrapper.className = 'dashboard-header__bell-wrapper'

  const bellButton = control('Notifications')
  bellButton.setAttribute('aria-label', 'Notifications')
  bellButton.className = 'dashboard-header__bell dashboard-control control'
  bellButton.dataset.dashboardPrimitive = 'control'
  bellButton.addEventListener('click', () => actions.onToggleDrawer?.())

  bellWrapper.append(bellButton)

  if (unreadCount > 0) {
    const badge = document.createElement('span')
    badge.className = 'dashboard-header__bell-badge'
    badge.dataset.bellBadge = ''
    badge.setAttribute('aria-label', `${unreadCount} unread notifications`)
    badge.textContent = String(unreadCount)
    bellWrapper.append(badge)
  }

  header.append(searchSlot, bellWrapper)
  mainArea.append(header)

  // Content area
  const mainContent = document.createElement('main')
  mainContent.className = 'dashboard-content'
  mainContent.dataset.dashboardPrimitive = 'main'
  mainContent.append(renderAuthenticatedView(activeScreen, dashboard))
  mainArea.append(mainContent)

  layout.append(mainArea)

  // Notification drawer (fixed overlay, always rendered, shown via data-open)
  const drawerContainer = document.createElement('div')
  renderNotificationDrawer(drawerContainer, {
    notifications: dashboardFixtures.shared.notifications,
    summary: notificationSummary,
    readIds: drawerState.readIds,
    onMarkAllRead: () => actions.onMarkAllRead?.(),
    onClose: () => actions.onToggleDrawer?.()
  })
  const drawerEl = drawerContainer.querySelector('[data-dashboard-primitive="drawer"]')
  if (drawerEl && drawerState.drawerOpen) {
    drawerEl.setAttribute('data-open', '')
  }
  layout.append(drawerContainer)

  // Shell wrapper
  const shell = document.createElement('section')
  shell.className = 'dashboard-page shell'
  shell.dataset.dashboardPrimitive = 'page'
  shell.append(layout)
  container.append(shell)
}

function renderAuthenticatedView(screen: DashboardScreenKey, state: DashboardState): HTMLElement {
  const route = ROUTES[screen]
  if (!route.load) {
    // Fixture-only / ComingSoon route
    return route.render({ status: 'loading' })
  }
  const viewState = stateFor(state, route.load)
  return route.render(viewState as ViewState<unknown>)
}

function stateFor<K extends keyof LoadedDashboardData>(
  state: DashboardState,
  key: K
): ViewState<unknown> {
  if (state.status !== 'ready') return { status: 'loading' }
  const slice = state.data[key]
  return slice ?? { status: 'loading' }
}

// Keep loadDashboard for backwards compat with the existing test that calls it directly
export async function loadDashboard(api: ApiClient, token: string): Promise<{ status: 'ready'; data: LoadedDashboardData }> {
  const [health, stats, users, recent, search, audit] = await Promise.allSettled([
    api.health(), api.adminStats(token), api.adminUsers(token), api.memories(token, { limit: 5 }), api.searchMemories(token, { query: DEFAULT_MEMORY_SEARCH_QUERY, limit: 5 }), api.auditLogs(token, { limit: 10 })
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

export async function loadForRoute(
  screen: DashboardScreenKey,
  api: ApiClient,
  token: string,
  cache: DashboardState
): Promise<DashboardState> {
  const route = ROUTES[screen]
  // Fixture-only routes need no fetch
  if (!route.load) return cache

  const key = route.load
  // Already cached
  if (cache.status === 'ready' && cache.data[key] !== undefined) return cache

  const existingData = cache.status === 'ready' ? cache.data : {}

  let slice: ViewState<unknown>
  try {
    slice = await fetchSlice(key, api, token)
  } catch (error) {
    slice = { status: 'error', message: messageFor(error) }
  }

  return {
    status: 'ready',
    data: { ...existingData, [key]: slice }
  }
}

async function fetchSlice(key: keyof LoadedDashboardData, api: ApiClient, token: string): Promise<ViewState<unknown>> {
  switch (key) {
    case 'overview': {
      const [health, stats] = await Promise.allSettled([api.health(), api.adminStats(token)])
      return combinedState(health, stats, (h, s) => ({ health: h, stats: s }))
    }
    case 'users': {
      const result = await Promise.allSettled([api.adminUsers(token)])
      return settledState(result[0])
    }
    case 'memories': {
      const [recent, search] = await Promise.allSettled([
        api.memories(token, { limit: 5 }),
        api.searchMemories(token, { query: DEFAULT_MEMORY_SEARCH_QUERY, limit: 5 })
      ])
      return combinedState(recent, search, (r, s) => ({ recent: r, search: s }))
    }
    case 'audit': {
      const result = await Promise.allSettled([api.auditLogs(token, { limit: 10 })])
      return settledState(result[0])
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

function loginErrorMessage(error: unknown): string {
  return error instanceof Error && error.message ? error.message : 'Unable to sign in. Check your credentials and try again.'
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
  let activeScreen: DashboardScreenKey = 'overview'
  let drawerOpen = false
  let readIds: Set<string> = new Set()

  const rerender = (state: AuthState) =>
    renderApp(root, state, actions, dashboard, window.location.pathname, { drawerOpen, readIds })

  const actions: AppActions = {
    async onLogin(email, password) {
      try {
        await setState(await session.login(email, password))
      } catch (error) {
        rerender({ status: 'anonymous', error: loginErrorMessage(error) })
      }
    },
    onLogout() {
      loadVersion += 1
      dashboard = { status: 'loading' }
      drawerOpen = false
      readIds = new Set()
      rerender(session.logout())
    },
    async onNavigate(path) {
      history.pushState(null, '', path)
      activeScreen = screenFromPath(path)
      const state = session.getState()
      rerender(state)
      if (state.status === 'authenticated') {
        await loadAndRender(state, activeScreen)
      }
    },
    onToggleDrawer() {
      drawerOpen = !drawerOpen
      rerender(session.getState())
    },
    onMarkAllRead() {
      for (const n of dashboardFixtures.shared.notifications) readIds.add(n.id)
      drawerOpen = false
      rerender(session.getState())
    }
  }

  async function loadAndRender(state: Extract<AuthState, { status: 'authenticated' }>, screen: DashboardScreenKey): Promise<void> {
    const version = loadVersion
    const loaded = await loadForRoute(screen, api, state.token, dashboard)
    const current = session.getState()
    if (version !== loadVersion || current.status !== 'authenticated' || current.token !== state.token) return
    dashboard = loaded
    rerender(current)
  }

  async function setState(state: AuthState): Promise<void> {
    const version = loadVersion + 1
    loadVersion = version
    activeScreen = screenFromPath(window.location.pathname)
    rerender(state)
    if (state.status === 'authenticated') {
      const loaded = await loadForRoute(activeScreen, api, state.token, dashboard)
      const current = session.getState()
      if (version !== loadVersion || current.status !== 'authenticated' || current.token !== state.token) return
      dashboard = loaded
      rerender(current)
    }
  }

  window.addEventListener('popstate', () => rerender(session.getState()))
  session
    .bootstrap()
    .then(setState)
    .catch(() => rerender({ status: 'anonymous', error: 'Unable to restore your session. Please sign in again.' }))
}

const root = document.getElementById('app')
if (root) {
  startDashboardApp(root)
}
