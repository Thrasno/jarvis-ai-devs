import { createApiClient, type AdminStats, type ApiClient, type Health, type MemoryList, type MemorySearch, type SyncAttemptSummary, type User } from './api/client'
import { createSessionStore, type AuthState, type SessionStore } from './auth/session'
import { control } from './components/dom'
import { comingSoon } from './components/ComingSoon'
import { renderNotificationDrawer } from './components/NotificationDrawer'
import { renderSidebar, type UserLevel } from './components/Sidebar'
import type { ActivityFeedFixtureViewModel, CurrentProfileViewModel, DashboardScreenKey, OverviewFixtureViewModel, ProjectListFixtureViewModel } from './domain/dashboard'
import { dashboardFixtures } from './fixtures/hive-dashboard/index'
import { activityFeedFixture, projectsFixture } from './fixtures/hive-dashboard/explore'
import { hiveOverviewFixture } from './fixtures/hive-dashboard/overview'
import { renderAuditSync } from './views/AuditSync'
import { renderGlobalSearch } from './views/GlobalSearch'
import { renderKnowledgeBrowser } from './views/KnowledgeBrowser'
import { renderMemories } from './views/Memories'
import { renderOverview, type ViewState } from './views/Overview'
import { renderProjects } from './views/Projects'
import { renderUsers, type UserManagementActionType } from './views/Users'
import { renderActivityFeed } from './views/ActivityFeed'
import './styles.css'

export const DEFAULT_MEMORY_SEARCH_QUERY = 'dashboard'


export type UsersData = { users: User[] }
export type MemoriesData = { recent: MemoryList; search: MemorySearch }
export type AuditSyncData = SyncAttemptSummary
export type LoadedDashboardData = {
  overview: ViewState<OverviewFixtureViewModel>
  users: ViewState<UsersData>
  memories: ViewState<MemoriesData>
  audit: ViewState<AuditSyncData>
  projects: ViewState<ProjectListFixtureViewModel>
}
export type DashboardState = { status: 'loading' } | { status: 'ready'; data: Partial<LoadedDashboardData> }

export type AppActions = {
  onLogin(email: string, password: string): Promise<void> | void
  onLogout(): void
  onNavigate?(path: string): void
  onToggleDrawer?(): void
  onMarkAllRead?(): void
  onSetUserLevel?(username: string, level: UserLevel): Promise<void>
  onGrantAdmin?(username: string): Promise<void>
  onDeactivateUser?(username: string): Promise<void>
}

export type DrawerState = {
  drawerOpen: boolean
  readIds: ReadonlySet<string>
  summaryUnread?: number
}

export type UserManagementState = {
  pendingAction?: { username: string; type: UserManagementActionType }
  mutationError?: string
}

export type RenderAppOptions = {
  container: HTMLElement
  state: AuthState
  actions: AppActions
  dashboard?: DashboardState
  routePath?: string
  drawerState?: DrawerState
  userManagementState?: UserManagementState
  drawerJustOpened?: boolean
  disposeActivityFeed?: () => void
  setActivityFeedDispose?: (fn: () => void) => void
}

type ScreenRoute = {
  path: string
  load?: keyof LoadedDashboardData
  render: (vs: ViewState<unknown>, routePath: string) => HTMLElement
  placeholderLabel?: string
}

export const ROUTES: Record<DashboardScreenKey, ScreenRoute> = {
  overview: {
    path: '/dashboard',
    load: 'overview',
    render: (vs) => renderOverview(vs as ViewState<OverviewFixtureViewModel>)
  },
  memories: {
    path: '/dashboard/memories',
    load: 'memories',
    render: (vs, routePath) => renderMemories(vs as ViewState<MemoriesData>, { detailId: memoryDetailIdFromPath(routePath) })
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
    load: 'projects',
    render: (vs) => renderProjects(vs as ViewState<ProjectListFixtureViewModel>)
  },
  knowledgeBrowser: {
    path: '/dashboard/knowledgeBrowser',
    render: (_vs, routePath) => renderKnowledgeBrowser(queryFromRoutePath(routePath))
  },
  globalSearch: {
    path: '/dashboard/globalSearch',
    render: (_vs, routePath) => renderGlobalSearch(queryFromRoutePath(routePath))
  },
  knowledgeGraph: {
    path: '/dashboard/knowledgeGraph',
    placeholderLabel: 'Knowledge Graph',
    render: () => comingSoon('Knowledge Graph')
  },
  activityFeed: {
    path: '/dashboard/activityFeed',
    placeholderLabel: 'Activity Feed',
    // Handled by the activityFeed special-case in renderAuthenticatedView; this render is never called.
    render: () => comingSoon('Activity Feed'),
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
  const normalized = routePath.split(/[?#]/, 1)[0].replace(/\/$/, '')
  if (memoryDetailIdFromPath(routePath)) return 'memories'
  // Handle legacy /dashboard/audit-sync alias
  if (normalized.endsWith('/audit-sync')) return 'auditLog'
  // Handle legacy /dashboard/users alias
  if (normalized.endsWith('/users')) return 'userManagement'
  for (const [key, route] of Object.entries(ROUTES) as [DashboardScreenKey, ScreenRoute][]) {
    if (route.path === normalized) return key
  }
  return 'overview'
}

export function renderApp({
  container,
  state,
  actions,
  dashboard = { status: 'loading' },
  routePath = window.location.pathname,
  drawerState = { drawerOpen: false, readIds: new Set() },
  userManagementState = {},
  drawerJustOpened = false,
  disposeActivityFeed = () => {},
  setActivityFeedDispose = () => {}
}: RenderAppOptions): void {
  if (state.status === 'anonymous') {
    disposeActivityFeed()
  }
  container.replaceChildren()
  state.status === 'anonymous'
    ? renderLogin(container, state, actions)
    : renderShell(container, state, actions, dashboard, routePath, drawerState, userManagementState, drawerJustOpened, disposeActivityFeed, setActivityFeedDispose)
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
  drawerState: DrawerState,
  userManagementState: UserManagementState,
  drawerJustOpened = false,
  disposeActivityFeed: () => void = () => {},
  setActivityFeedDispose: (fn: () => void) => void = () => {}
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
    profile: profileFromUser(state.user),
    onNavigate: (path) => actions.onNavigate?.(path),
    onLogout: actions.onLogout
  })
  const sidebar = sidebarContainer.querySelector<HTMLElement>('[data-dashboard-primitive="sidebar"]')
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
  const searchSlot = document.createElement('button')
  searchSlot.type = 'button'
  searchSlot.className = 'dashboard-header__search'
  searchSlot.textContent = 'Search memories…'
  searchSlot.setAttribute('aria-label', 'Search memories')
  searchSlot.addEventListener('click', () => actions.onNavigate?.(globalSearchPathFromRoutePath(routePath)))

  // Bell button with optional unread badge
  const bellWrapper = document.createElement('div')
  bellWrapper.className = 'dashboard-header__bell-wrapper'

  const bellButton = control('Notifications')
  bellButton.setAttribute('aria-label', 'Notifications')
  bellButton.className = 'dashboard-header__bell dashboard-control control'
  bellButton.dataset.dashboardPrimitive = 'control'
  bellButton.dataset.bell = ''
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
  mainContent.append(renderAuthenticatedView(activeScreen, dashboard, routePath, state, actions, userManagementState, disposeActivityFeed, setActivityFeedDispose))
  mainArea.append(mainContent)

  setModalBackgroundState([sidebar, header, mainContent], drawerState.drawerOpen)

  layout.append(mainArea)

  // Notification drawer (fixed overlay, always rendered, shown via data-open)
  const drawerContainer = document.createElement('div')
  renderNotificationDrawer(drawerContainer, {
    notifications: dashboardFixtures.shared.notifications,
    summary: { ...notificationSummary, unread: drawerState.summaryUnread ?? notificationSummary.unread },
    readIds: drawerState.readIds,
    open: drawerState.drawerOpen,
    onMarkAllRead: () => actions.onMarkAllRead?.(),
    onClose: () => actions.onToggleDrawer?.()
  })
  const drawerEl = drawerContainer.querySelector('[data-dashboard-primitive="drawer"]')
  layout.append(drawerContainer)

  // Shell wrapper
  const shell = document.createElement('section')
  shell.className = 'dashboard-page shell'
  shell.dataset.dashboardPrimitive = 'page'
  shell.append(layout)
  container.append(shell)

  // Focus the close button only when the drawer was just opened (not on data-load re-renders).
  // Called after container.append(shell) so the element is attached to the document.
  if (drawerJustOpened && drawerEl) {
    drawerEl.querySelector<HTMLElement>('[data-drawer-close]')?.focus()
  }
}

function setModalBackgroundState(elements: readonly (HTMLElement | null)[], modalOpen: boolean): void {
  for (const element of elements) {
    if (!element) continue
    if (modalOpen) {
      element.setAttribute('inert', '')
      element.setAttribute('aria-hidden', 'true')
    } else {
      element.removeAttribute('inert')
      element.removeAttribute('aria-hidden')
    }
  }
}

function profileFromUser(user: User): CurrentProfileViewModel {
  const email = user.email.trim()
  const name = user.username.trim() || email.split('@')[0] || email
  return {
    initials: initialsFor(name),
    name,
    email,
    role: user.level,
    logoutLabel: 'Logout'
  }
}

function initialsFor(name: string): string {
  const parts = name
    .split(/[\s._-]+/)
    .map((part) => part.trim())
    .filter(Boolean)

  const initials = parts.length > 1
    ? `${parts[0][0]}${parts[1][0]}`
    : name.slice(0, 2)

  return initials.toUpperCase()
}

function renderAuthenticatedView(
  screen: DashboardScreenKey,
  state: DashboardState,
  routePath: string,
  auth: Extract<AuthState, { status: 'authenticated' }>,
  actions: AppActions,
  userManagementState: UserManagementState,
  disposeActivityFeed: () => void,
  setActivityFeedDispose: (fn: () => void) => void
): HTMLElement {
  disposeActivityFeed()

  if (screen === 'activityFeed') {
    const handle = renderActivityFeed(
      { status: 'ready', data: activityFeedFixture } as ViewState<ActivityFeedFixtureViewModel>,
      {
        onNavigate: (p) => actions.onNavigate?.(p),
        scheduler: { setInterval: window.setInterval.bind(window), clearInterval: window.clearInterval.bind(window) }
      }
    )
    setActivityFeedDispose(handle.dispose)
    return handle.element
  }

  const route = ROUTES[screen]
  if (screen === 'userManagement') {
    return renderUsers(stateFor(state, 'users') as ViewState<UsersData>, {
      currentUsername: auth.user.username,
      currentLevel: auth.user.level,
      pendingAction: userManagementState.pendingAction,
      mutationError: userManagementState.mutationError,
      actions: {
        onSetUserLevel: (username, level) => actions.onSetUserLevel?.(username, level) ?? Promise.resolve(),
        onGrantAdmin: (username) => actions.onGrantAdmin?.(username) ?? Promise.resolve(),
        onDeactivateUser: (username) => actions.onDeactivateUser?.(username) ?? Promise.resolve()
      }
    })
  }
  if (!route.load) {
    // Fixture-only / ComingSoon route
    return route.render({ status: 'loading' }, routePath)
  }
  const viewState = stateFor(state, route.load)
  return route.render(viewState as ViewState<unknown>, routePath)
}

function queryFromRoutePath(routePath: string): string {
  const queryStart = routePath.indexOf('?')
  if (queryStart === -1) return ''
  const hashStart = routePath.indexOf('#', queryStart)
  return hashStart === -1 ? routePath.slice(queryStart) : routePath.slice(queryStart, hashStart)
}

function globalSearchPathFromRoutePath(routePath: string): string {
  const normalized = routePath.split(/[?#]/, 1)[0].replace(/\/$/, '')
  const query = queryFromRoutePath(routePath)
  if (query && (normalized === ROUTES.knowledgeBrowser.path || normalized === ROUTES.globalSearch.path)) {
    return `${ROUTES.globalSearch.path}${query}`
  }
  return ROUTES.globalSearch.path
}

function memoryDetailIdFromPath(routePath: string): string | null {
  const normalized = routePath.split(/[?#]/, 1)[0].replace(/\/$/, '')
  const prefix = '/dashboard/memories/'
  if (!normalized.startsWith(prefix)) return null
  const id = normalized.slice(prefix.length).trim()
  if (!id) return null
  try {
    return decodeURIComponent(id)
  } catch {
    return id
  }
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
    api.health(), api.adminStats(token), api.adminUsers(token), api.memories(token, { limit: 5 }), api.searchMemories(token, { query: DEFAULT_MEMORY_SEARCH_QUERY, limit: 5 }), api.syncAttemptSummary(token)
  ])
  return {
    status: 'ready',
    data: {
      overview: overviewState(health, stats),
      users: settledState(users),
      memories: combinedState(recent, search, (recent, search) => ({ recent, search })),
      audit: settledState(audit),
      projects: { status: 'ready', data: projectsFixture }
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
      return overviewState(health, stats)
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
      const result = await Promise.allSettled([api.syncAttemptSummary(token)])
      return settledState(result[0])
    }
    case 'projects':
      return { status: 'ready', data: projectsFixture }
  }
}

function overviewState(
  health: PromiseSettledResult<Health>,
  stats: PromiseSettledResult<AdminStats>
): ViewState<OverviewFixtureViewModel> {
  if (health.status === 'rejected') return { status: 'error', message: messageFor(health.reason) }
  if (stats.status === 'rejected') return { status: 'error', message: messageFor(stats.reason) }

  const healthMessage = degradedHealthMessage(health.value)
  if (healthMessage) return { status: 'error', message: healthMessage }

  return { status: 'ready', data: overviewFromLiveApiWithFixtureComplements(stats.value) }
}

function degradedHealthMessage(health: Health): string | null {
  const status = health.status.trim() || 'unknown'
  const db = health.db.trim() || 'unknown'
  const apiReady = status.toLowerCase() === 'ok'
  const dbReady = db.toLowerCase() === 'connected'

  if (apiReady && dbReady) return null

  const issues = [
    ...(apiReady ? [] : [`status ${status}`]),
    ...(dbReady ? [] : [`database ${db}`])
  ]
  return `Hive API health is degraded: ${issues.join(', ')}`
}

function overviewFromLiveApiWithFixtureComplements(stats: AdminStats): OverviewFixtureViewModel {
  const healthyTotal = hiveOverviewFixture.healthyDaemons.totalValue ?? hiveOverviewFixture.healthyDaemons.value
  const healthyValue = hiveOverviewFixture.healthyDaemons.value
  const activeProjects = activeProjectCount(stats)
  return {
    ...hiveOverviewFixture,
    totalMemories: { label: hiveOverviewFixture.totalMemories.label, value: stats.memories.total, displayValue: compactNumber(stats.memories.total) },
    activeProjects: { label: hiveOverviewFixture.activeProjects.label, value: activeProjects, displayValue: String(activeProjects) },
    healthyDaemons: { ...hiveOverviewFixture.healthyDaemons, value: healthyValue, totalValue: healthyTotal, displayValue: `${healthyValue}/${healthyTotal}` }
  }
}

function activeProjectCount(stats: AdminStats): number {
  return stats.memories.by_project.filter((project) => project.count > 0).length
}

function compactNumber(value: number): string {
  return value >= 1000 ? `${(value / 1000).toFixed(1)}k` : String(value)
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

export function startDashboardApp(root: HTMLElement, options: StartOptions = {}): () => void {
  const api = options.api ?? createApiClient()
  const session = options.session ?? createSessionStore({ api })
  let dashboard: DashboardState = { status: 'loading' }
  let activeActivityFeedDispose: (() => void) | undefined
  function disposeActivityFeed(): void {
    activeActivityFeedDispose?.()
    activeActivityFeedDispose = undefined
  }
  function setActivityFeedDispose(fn: () => void): void {
    activeActivityFeedDispose = fn
  }
  let loadVersion = 0
  let activeScreen: DashboardScreenKey = 'overview'
  let drawerOpen = false
  let readIds: Set<string> = new Set()
  let summaryUnread: number = dashboardFixtures.shared.notificationSummary.unread
  let userManagementState: UserManagementState = {}
  let disposed = false

  let drawerJustOpened = false

  const rerender = (state: AuthState) => {
    if (disposed) return
    renderApp({
      container: root,
      state,
      actions,
      dashboard,
      routePath: currentRoutePath(),
      drawerState: { drawerOpen, readIds, summaryUnread },
      userManagementState,
      drawerJustOpened,
      disposeActivityFeed,
      setActivityFeedDispose
    })
  }

  const actions: AppActions = {
    async onLogin(email, password) {
      if (disposed) return
      try {
        await setState(await session.login(email, password))
      } catch (error) {
        if (disposed) return
        rerender({ status: 'anonymous', error: loginErrorMessage(error) })
      }
    },
    onLogout() {
      if (disposed) return
      loadVersion += 1
      dashboard = { status: 'loading' }
      drawerOpen = false
      readIds = new Set()
      summaryUnread = dashboardFixtures.shared.notificationSummary.unread
      userManagementState = {}
      rerender(session.logout())
    },
    async onNavigate(path) {
      if (disposed) return
      history.pushState(null, '', path)
      activeScreen = screenFromPath(path)
      const state = session.getState()
      rerender(state)
      if (state.status === 'authenticated') {
        await loadAndRender(state, activeScreen)
      }
    },
    onToggleDrawer() {
      if (disposed) return
      const wasOpen = drawerOpen
      drawerOpen = !drawerOpen
      drawerJustOpened = !wasOpen
      rerender(session.getState())
      drawerJustOpened = false
      if (wasOpen) {
        root.querySelector<HTMLElement>('[data-bell]')?.focus()
      }
    },
    onMarkAllRead() {
      if (disposed) return
      for (const n of dashboardFixtures.shared.notifications) readIds.add(n.id)
      summaryUnread = 0
      drawerOpen = false
      rerender(session.getState())
      root.querySelector<HTMLElement>('[data-bell]')?.focus()
    },
    onSetUserLevel(username, level) {
      return runUserMutation(username, 'level', (token) => api.setUserLevel(token, username, level))
    },
    onGrantAdmin(username) {
      return runUserMutation(username, 'grant-admin', (token) => api.grantAdmin(token, username))
    },
    onDeactivateUser(username) {
      return runUserMutation(username, 'deactivate', (token) => api.deactivateUser(token, username))
    }
  }

  async function runUserMutation(
    username: string,
    type: UserManagementActionType,
    mutate: (token: string) => Promise<unknown>
  ): Promise<void> {
    if (disposed || userManagementState.pendingAction) return
    const state = session.getState()
    if (state.status !== 'authenticated') return
    const version = loadVersion
    userManagementState = { pendingAction: { username, type } }
    rerender(state)
    try {
      await mutate(state.token)
      const slice = await fetchSlice('users', api, state.token)
      if (disposed) return
      const current = session.getState()
      if (version !== loadVersion || current.status !== 'authenticated' || current.token !== state.token) return
      const existingData = dashboard.status === 'ready' ? dashboard.data : {}
      dashboard = { status: 'ready', data: { ...existingData, users: slice as ViewState<UsersData> } }
      userManagementState = {}
      rerender(current)
    } catch (error) {
      if (disposed) return
      userManagementState = { mutationError: messageFor(error) }
      rerender(session.getState())
    }
  }

  async function loadAndRender(state: Extract<AuthState, { status: 'authenticated' }>, screen: DashboardScreenKey): Promise<void> {
    if (disposed) return
    const version = loadVersion
    const loaded = await loadForRoute(screen, api, state.token, dashboard)
    if (disposed) return
    const current = session.getState()
    if (version !== loadVersion || current.status !== 'authenticated' || current.token !== state.token) return
    dashboard = loaded
    rerender(current)
  }

  async function setState(state: AuthState): Promise<void> {
    if (disposed) return
    const version = loadVersion + 1
    loadVersion = version
    activeScreen = screenFromPath(currentRoutePath())
    rerender(state)
    if (state.status === 'authenticated') {
      const loaded = await loadForRoute(activeScreen, api, state.token, dashboard)
      if (disposed) return
      const current = session.getState()
      if (version !== loadVersion || current.status !== 'authenticated' || current.token !== state.token) return
      dashboard = loaded
      rerender(current)
    }
  }

  const handler = () => {
    if (disposed) return
    rerender(session.getState())
  }
  window.addEventListener('popstate', handler)
  session
    .bootstrap()
    .then(setState)
    .catch(() => {
      if (disposed) return
      rerender({ status: 'anonymous', error: 'Unable to restore your session. Please sign in again.' })
    })
  return () => {
    if (disposed) return
    disposed = true
    loadVersion += 1
    window.removeEventListener('popstate', handler)
    disposeActivityFeed()
  }
}

function currentRoutePath(): string {
  return `${window.location.pathname}${window.location.search}${window.location.hash}`
}

const root = document.getElementById('app')
if (root) {
  startDashboardApp(root)
}
