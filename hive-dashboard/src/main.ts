import { createApiClient, type AdminStats, type ApiClient, type Count, type Health, type Memory, type MemoryList, type MemoryListParams, type MemorySearch, type MemorySearchParams, type OverviewGrowth, type OverviewProjectSyncHealth, type OverviewStats, type SyncAttemptSummary, type User } from './api/client'
import { parseDashboardFilters } from './api/urlFilters'
import { createSessionStore, type AuthState, type SessionStore } from './auth/session'
import { control } from './components/dom'
import { renderSidebar, type UserLevel } from './components/Sidebar'
import { activityFeedFromApi, appendActivityPage } from './domain/activityFeed'
import type { ActivityFeedViewModel, CurrentProfileViewModel, DashboardScreenKey, OverviewFixtureViewModel, ProjectListFixtureViewModel, ProjectSyncStatus } from './domain/dashboard'
import { memoryListToDiscoveryData, memorySearchToDiscoveryData, type KnowledgeDiscoveryData } from './domain/knowledgeDiscovery'
import { dashboardFixtures } from './fixtures/hive-dashboard/index'
import { projectsFixture } from './fixtures/hive-dashboard/explore'
import { renderAuditSync } from './views/AuditSync'
import { renderGlobalSearch } from './views/GlobalSearch'
import { renderKnowledgeBrowser } from './views/KnowledgeBrowser'
import { renderMemories, type MemoryDetailData, type MemoryDetailRoute, type MemoryDetailViewState } from './views/Memories'
import { renderOverview, type ViewState } from './views/Overview'
import { renderProjects } from './views/Projects'
import { renderUsers, type UserManagementActionType } from './views/Users'
import { renderActivityFeed } from './views/ActivityFeed'
import './styles.css'

export const DEFAULT_MEMORY_SEARCH_QUERY = 'dashboard'
export const DEFAULT_ACTIVITY_LIMIT = 20

const OVERVIEW_LABELS = {
  totalMemories: 'Total Memories',
  activeProjects: 'Active Projects',
  healthyDaemons: 'Healthy Daemons',
  openConflicts: 'Open Conflicts',
  knowledgeGrowth: 'Knowledge Growth'
} as const


export type UsersData = { users: User[] }
export type MemoriesData = { recent: MemoryList; search: MemorySearch }
export type MemoryDetailStateData = MemoryDetailData
export type AuditSyncData = SyncAttemptSummary
export type LoadedDashboardData = {
  overview: ViewState<OverviewFixtureViewModel>
  users: ViewState<UsersData>
  memories: ViewState<MemoriesData>
  memoryDetail?: MemoryDetailViewState
  audit: ViewState<AuditSyncData>
  projects: ViewState<ProjectListFixtureViewModel>
  activity: ViewState<ActivityFeedViewModel>
  knowledgeBrowser: ViewState<KnowledgeDiscoveryData>
  globalSearch: ViewState<KnowledgeDiscoveryData>
}
export type DashboardState = { status: 'loading' } | { status: 'ready'; data: Partial<LoadedDashboardData> }

export type AppActions = {
  onLogin(email: string, password: string): Promise<void> | void
  onLogout(): void
  onNavigate?(path: string): void
  onSetUserLevel?(username: string, level: UserLevel): Promise<void>
  onGrantAdmin?(username: string): Promise<void>
  onDeactivateUser?(username: string): Promise<void>
  onLoadMoreActivity?(): Promise<void>
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
  userManagementState?: UserManagementState
  disposeActivityFeed?: () => void
  setActivityFeedDispose?: (fn: () => void) => void
}

type ScreenRoute = {
  path: string
  load?: keyof LoadedDashboardData
  render: (vs: ViewState<unknown>, routePath: string, actions: AppActions) => HTMLElement
}

const HIDDEN_DASHBOARD_SCREENS = new Set<DashboardScreenKey>([
  'knowledgeGraph',
  'contributors',
  'developerTimeline',
  'syncStatus',
  'analytics',
  'conflictViewer'
])

export const ROUTES: Record<DashboardScreenKey, ScreenRoute> = {
  overview: {
    path: '/dashboard',
    load: 'overview',
    render: (vs) => renderOverview(vs as ViewState<OverviewFixtureViewModel>)
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
    load: 'projects',
    render: (vs) => renderProjects(vs as ViewState<ProjectListFixtureViewModel>)
  },
  knowledgeBrowser: {
    path: '/dashboard/knowledgeBrowser',
    load: 'knowledgeBrowser',
    render: (vs, routePath, actions) => renderKnowledgeBrowser(vs as ViewState<KnowledgeDiscoveryData>, queryFromRoutePath(routePath), { onNavigate: actions.onNavigate })
  },
  globalSearch: {
    path: '/dashboard/globalSearch',
    load: 'globalSearch',
    render: (vs, routePath, actions) => renderGlobalSearch(vs as ViewState<KnowledgeDiscoveryData>, queryFromRoutePath(routePath), { onNavigate: actions.onNavigate })
  },
  knowledgeGraph: {
    path: '/dashboard/knowledgeGraph',
    load: 'overview',
    render: (vs) => renderOverview(vs as ViewState<OverviewFixtureViewModel>)
  },
  activityFeed: {
    path: '/dashboard/activityFeed',
    load: 'activity',
    render: (vs) => renderActivityFeed(vs as ViewState<ActivityFeedViewModel>, { onNavigate: () => {} }).element,
  },
  contributors: {
    path: '/dashboard/contributors',
    load: 'overview',
    render: (vs) => renderOverview(vs as ViewState<OverviewFixtureViewModel>)
  },
  developerTimeline: {
    path: '/dashboard/developerTimeline',
    load: 'overview',
    render: (vs) => renderOverview(vs as ViewState<OverviewFixtureViewModel>)
  },
  syncStatus: {
    path: '/dashboard/syncStatus',
    load: 'overview',
    render: (vs) => renderOverview(vs as ViewState<OverviewFixtureViewModel>)
  },
  analytics: {
    path: '/dashboard/analytics',
    load: 'overview',
    render: (vs) => renderOverview(vs as ViewState<OverviewFixtureViewModel>)
  },
  conflictViewer: {
    path: '/dashboard/conflictViewer',
    load: 'overview',
    render: (vs) => renderOverview(vs as ViewState<OverviewFixtureViewModel>)
  }
}

/**
 * Resolve the active DashboardScreenKey from a URL path.
 * Strips trailing slash, matches exact path against ROUTES table.
 * Falls back to 'overview'.
 */
function screenFromPath(routePath: string): DashboardScreenKey {
  const normalized = routePath.split(/[?#]/, 1)[0].replace(/\/$/, '')
  if (memoryDetailRouteFromPath(routePath).kind !== 'none') return 'memories'
  // Handle legacy /dashboard/audit-sync alias
  if (normalized.endsWith('/audit-sync')) return 'auditLog'
  // Handle legacy /dashboard/users alias
  if (normalized.endsWith('/users')) return 'userManagement'
  for (const [key, route] of Object.entries(ROUTES) as [DashboardScreenKey, ScreenRoute][]) {
    if (route.path === normalized) return HIDDEN_DASHBOARD_SCREENS.has(key) ? 'overview' : key
  }
  return 'overview'
}

export function renderApp({
  container,
  state,
  actions,
  dashboard = { status: 'loading' },
  routePath = window.location.pathname,
  userManagementState = {},
  disposeActivityFeed = () => {},
  setActivityFeedDispose = () => {}
}: RenderAppOptions): void {
  if (state.status === 'anonymous') {
    disposeActivityFeed()
  }
  container.replaceChildren()
  state.status === 'anonymous'
    ? renderLogin(container, state, actions)
    : renderShell(container, state, actions, dashboard, routePath, userManagementState, disposeActivityFeed, setActivityFeedDispose)
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
  userManagementState: UserManagementState,
  disposeActivityFeed: () => void = () => {},
  setActivityFeedDispose: (fn: () => void) => void = () => {}
): void {
  const activeScreen = screenFromPath(routePath)
  const userLevel = state.user.level as UserLevel

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

  header.append(searchSlot)
  mainArea.append(header)

  // Content area
  const mainContent = document.createElement('main')
  mainContent.className = 'dashboard-content'
  mainContent.dataset.dashboardPrimitive = 'main'
  mainContent.append(renderAuthenticatedView(activeScreen, dashboard, routePath, state, actions, userManagementState, disposeActivityFeed, setActivityFeedDispose))
  mainArea.append(mainContent)

  layout.append(mainArea)

  // Shell wrapper
  const shell = document.createElement('section')
  shell.className = 'dashboard-page shell'
  shell.dataset.dashboardPrimitive = 'page'
  shell.append(layout)
  container.append(shell)
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
      stateFor(state, 'activity') as ViewState<ActivityFeedViewModel>,
      {
        onNavigate: (p) => actions.onNavigate?.(p),
        onLoadMore: () => { void actions.onLoadMoreActivity?.() }
      }
    )
    setActivityFeedDispose(handle.dispose)
    return handle.element
  }

  const route = ROUTES[screen]
  if (HIDDEN_DASHBOARD_SCREENS.has(screen)) {
    return renderOverview(stateFor(state, 'overview') as ViewState<OverviewFixtureViewModel>)
  }
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
  if (screen === 'memories') {
    const detailRoute = memoryDetailRouteFromPath(routePath)
    return renderMemories(stateFor(state, 'memories') as ViewState<MemoriesData>, {
      detailRoute,
      detail: detailRoute.kind === 'valid' ? memoryDetailForRoute(state, detailRoute.id) : undefined,
      onBackToMemories: () => actions.onNavigate?.('/dashboard/memories')
    })
  }
  if (!route.load) {
    // Fixture-only / ComingSoon route
    return route.render({ status: 'loading' }, routePath, actions)
  }
  const viewState = stateFor(state, route.load)
  return route.render(viewState as ViewState<unknown>, routePath, actions)
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

function memoryDetailRouteFromPath(routePath: string): MemoryDetailRoute {
  const normalized = routePath.split(/[?#]/, 1)[0].replace(/\/$/, '')
  const prefix = '/dashboard/memories/'
  if (!normalized.startsWith(prefix)) return { kind: 'none' }
  const id = normalized.slice(prefix.length).trim()
  if (!id) return { kind: 'none' }
  try {
    const decoded = decodeURIComponent(id).trim()
    return decoded ? { kind: 'valid', id: decoded, routeKey: decoded } : { kind: 'malformed', raw: id }
  } catch {
    return { kind: 'malformed', raw: id }
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

function memoryDetailForRoute(state: DashboardState, routeId: string): MemoryDetailViewState {
  if (state.status !== 'ready') return { status: 'loading' }
  const detail = state.data.memoryDetail
  if (!detail) return { status: 'loading' }
  if (detail.status === 'ready') return detail.data.routeId === routeId ? detail : { status: 'loading' }
  const detailRouteId = 'routeId' in detail ? detail.routeId : undefined
  if (detail.status === 'error' && detailRouteId && detailRouteId !== routeId) return { status: 'loading' }
  return detail
}

// Keep loadDashboard for backwards compat with the existing test that calls it directly
export async function loadDashboard(api: ApiClient, token: string): Promise<{ status: 'ready'; data: LoadedDashboardData }> {
  const [health, stats, overviewStatsResult, overviewGrowthResult, users, recent, search, audit, activity] = await Promise.allSettled([
    api.health(), api.adminStats(token), api.overviewStats(token), api.overviewGrowth(token), api.adminUsers(token), api.memories(token, { limit: 5 }), api.searchMemories(token, { query: DEFAULT_MEMORY_SEARCH_QUERY, limit: 5 }), api.syncAttemptSummary(token), api.activity(token, { limit: DEFAULT_ACTIVITY_LIMIT })
  ])
  return {
    status: 'ready',
    data: {
      overview: overviewState(health, stats, overviewStatsResult, overviewGrowthResult),
      users: settledState(users),
      memories: combinedState(recent, search, (recent, search) => ({ recent, search })),
      audit: settledState(audit),
      activity: activityState(activity),
      projects: { status: 'ready', data: projectsFixture },
      knowledgeBrowser: discoveryListState(recent),
      globalSearch: discoverySearchState(search)
    }
  }
}

export async function loadForRoute(
  screen: DashboardScreenKey,
  api: ApiClient,
  token: string,
  cache: DashboardState,
  routePath = ROUTES[screen].path
): Promise<DashboardState> {
  const route = ROUTES[screen]
  if (screen === 'memories') {
    const detailRoute = memoryDetailRouteFromPath(routePath)
    if (detailRoute.kind === 'malformed') return cache
    if (detailRoute.kind === 'valid') return loadMemoryDetail(detailRoute, api, token, cache)
  }
  if (HIDDEN_DASHBOARD_SCREENS.has(screen)) return loadForRoute('overview', api, token, cache, ROUTES.overview.path)
  if (!route.load) return cache

  const key = route.load
  // Already cached
  if (!isQuerySensitiveDiscoveryKey(key) && cache.status === 'ready' && cache.data[key] !== undefined) return cache

  const existingData = cache.status === 'ready' ? cache.data : {}

  let slice: ViewState<unknown>
  try {
    slice = await fetchSlice(key, api, token, routePath)
  } catch (error) {
    slice = { status: 'error', message: messageFor(error) }
  }

  return {
    status: 'ready',
    data: { ...existingData, [key]: slice }
  }
}

async function loadMemoryDetail(
  route: Extract<MemoryDetailRoute, { kind: 'valid' }>,
  api: ApiClient,
  token: string,
  cache: DashboardState
): Promise<DashboardState> {
  const existingData = cache.status === 'ready' ? cache.data : {}

  let slice: MemoryDetailViewState
  try {
    const memory = await api.memory(token, route.id)
    slice = memoryDetailState(route.id, memory)
  } catch (error) {
    slice = { status: 'error', message: messageFor(error), routeId: route.id }
  }

  return {
    status: 'ready',
    data: { ...existingData, memoryDetail: slice }
  }
}

function memoryDetailState(routeId: string, memory: Memory): MemoryDetailViewState {
  return { status: 'ready', data: { routeId, memory } }
}

async function fetchSlice(key: keyof LoadedDashboardData, api: ApiClient, token: string, routePath = ''): Promise<ViewState<unknown>> {
  switch (key) {
    case 'memoryDetail':
      return { status: 'loading' }
    case 'overview': {
      const [health, stats, overviewStatsResult, overviewGrowthResult] = await Promise.allSettled([api.health(), api.adminStats(token), api.overviewStats(token), api.overviewGrowth(token)])
      return overviewState(health, stats, overviewStatsResult, overviewGrowthResult)
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
    case 'activity': {
      const result = await Promise.allSettled([api.activity(token, { limit: DEFAULT_ACTIVITY_LIMIT })])
      return activityState(result[0])
    }
    case 'knowledgeBrowser': {
      const response = await api.memories(token, memoryListParamsFromRoute(routePath))
      return { status: 'ready', data: memoryListToDiscoveryData(response) }
    }
    case 'globalSearch': {
      const params = memorySearchParamsFromRoute(routePath)
      if (!params) return { status: 'ready', data: emptyDiscoveryData(routePath) }
      const response = await api.searchMemories(token, params)
      return { status: 'ready', data: memorySearchToDiscoveryData(response) }
    }
  }
}

function discoveryListState(result: PromiseSettledResult<MemoryList>): ViewState<KnowledgeDiscoveryData> {
  return result.status === 'fulfilled'
    ? { status: 'ready', data: memoryListToDiscoveryData(result.value) }
    : { status: 'error', message: messageFor(result.reason) }
}

function discoverySearchState(result: PromiseSettledResult<MemorySearch>): ViewState<KnowledgeDiscoveryData> {
  return result.status === 'fulfilled'
    ? { status: 'ready', data: memorySearchToDiscoveryData(result.value) }
    : { status: 'error', message: messageFor(result.reason) }
}

function isQuerySensitiveDiscoveryKey(key: keyof LoadedDashboardData): boolean {
  return key === 'knowledgeBrowser' || key === 'globalSearch'
}

function isQuerySensitiveDiscoveryScreen(screen: DashboardScreenKey): boolean {
  const key = ROUTES[screen].load
  return key !== undefined && isQuerySensitiveDiscoveryKey(key)
}

function routeAndQueryFromRoutePath(routePath: string): string {
  return routePath.split('#', 1)[0]
}

function memoryDetailRouteKeyForScreen(screen: DashboardScreenKey, routePath: string): string | undefined {
  if (screen !== 'memories') return undefined
  const route = memoryDetailRouteFromPath(routePath)
  return route.kind === 'valid' ? route.routeKey : undefined
}

function memoryListParamsFromRoute(routePath: string): MemoryListParams {
  const filters = parseDashboardFilters(queryFromRoutePath(routePath))
  return {
    project: filters.project,
    category: filters.category && filters.category !== 'all' ? filters.category : undefined,
    from: filters.from,
    until: filters.until,
    limit: filters.limit,
    offset: filters.offset
  }
}

function memorySearchParamsFromRoute(routePath: string): MemorySearchParams | null {
  const filters = parseDashboardFilters(queryFromRoutePath(routePath))
  const query = filters.query?.trim()
  if (!query) return null
  return {
    query,
    project: filters.project,
    category: filters.category && filters.category !== 'all' ? filters.category : undefined,
    from: filters.from,
    until: filters.until,
    limit: filters.limit,
    offset: filters.offset
  }
}

function emptyDiscoveryData(routePath: string): KnowledgeDiscoveryData {
  const filters = parseDashboardFilters(queryFromRoutePath(routePath))
  return {
    items: [],
    total: 0,
    limit: filters.limit ?? 10,
    offset: filters.offset ?? 0,
    previousOffset: null,
    nextOffset: null
  }
}

function activityState(result: PromiseSettledResult<Awaited<ReturnType<ApiClient['activity']>>>): ViewState<ActivityFeedViewModel> {
  return result.status === 'fulfilled'
    ? { status: 'ready', data: activityFeedFromApi(result.value) }
    : { status: 'error', message: messageFor(result.reason) }
}

function overviewState(
  health: PromiseSettledResult<Health>,
  stats: PromiseSettledResult<AdminStats>,
  overviewStatsResult: PromiseSettledResult<OverviewStats>,
  overviewGrowthResult: PromiseSettledResult<OverviewGrowth>
): ViewState<OverviewFixtureViewModel> {
  if (health.status === 'rejected') return { status: 'error', message: messageFor(health.reason) }
  if (stats.status === 'rejected') return { status: 'error', message: messageFor(stats.reason) }
  if (overviewStatsResult.status === 'rejected') return { status: 'error', message: messageFor(overviewStatsResult.reason) }
  if (overviewGrowthResult.status === 'rejected') return { status: 'error', message: messageFor(overviewGrowthResult.reason) }

  const healthMessage = degradedHealthMessage(health.value)
  if (healthMessage) return { status: 'error', message: healthMessage }

  return { status: 'ready', data: overviewFromLiveApi(stats.value, overviewStatsResult.value, overviewGrowthResult.value) }
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

function overviewFromLiveApi(stats: AdminStats, overviewStats: OverviewStats, overviewGrowth: OverviewGrowth): OverviewFixtureViewModel {
  const activeProjects = activeProjectCount(stats)
  return {
    screen: 'overview',
    totalMemories: { label: OVERVIEW_LABELS.totalMemories, value: stats.memories.total, displayValue: compactNumber(stats.memories.total) },
    activeProjects: { label: OVERVIEW_LABELS.activeProjects, value: activeProjects, displayValue: String(activeProjects) },
    healthyDaemons: {
      label: OVERVIEW_LABELS.healthyDaemons,
      value: overviewStats.daemon_health.healthy,
      totalValue: overviewStats.daemon_health.total,
      displayValue: `${overviewStats.daemon_health.healthy}/${overviewStats.daemon_health.total}`
    },
    openConflicts: { label: OVERVIEW_LABELS.openConflicts, value: overviewStats.conflicts.open },
    knowledgeGrowth: { label: OVERVIEW_LABELS.knowledgeGrowth, points: overviewGrowth.knowledge_growth },
    syncHealthByProject: overviewStats.sync_health_by_project.map(syncHealthProjectFromApi),
    liveActivity: {
      count: overviewStats.live_activity.count,
      newestSyncId: overviewStats.live_activity.newest_sync_id
    },
    mostActiveProjects: overviewStats.most_active_projects.map(projectCountPoint)
  }
}

function syncHealthProjectFromApi(project: OverviewProjectSyncHealth): OverviewFixtureViewModel['syncHealthByProject'][number] {
  return {
    id: project.project,
    name: project.project,
    region: project.region,
    status: projectSyncStatus(project.status),
    contributorCount: project.contributor_count
  }
}

function projectSyncStatus(status: string): ProjectSyncStatus {
  return status === 'healthy' || status === 'degraded' || status === 'unknown' ? status : 'unknown'
}

function projectCountPoint(count: Count): OverviewFixtureViewModel['mostActiveProjects'][number] {
  return { label: count.project ?? count.category ?? 'unknown', value: count.count }
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
  let userManagementState: UserManagementState = {}
  let disposed = false

  const rerender = (state: AuthState) => {
    if (disposed) return
    renderApp({
      container: root,
      state,
      actions,
      dashboard,
      routePath: currentRoutePath(),
      userManagementState,
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
      userManagementState = {}
      rerender(session.logout())
    },
    async onNavigate(path) {
      if (disposed) return
      history.pushState(null, '', path)
      loadVersion += 1
      activeScreen = screenFromPath(path)
      const state = session.getState()
      rerender(state)
      if (state.status === 'authenticated') {
        await loadAndRender(state, activeScreen)
      }
    },
    onSetUserLevel(username, level) {
      return runUserMutation(username, 'level', (token) => api.setUserLevel(token, username, level))
    },
    onGrantAdmin(username) {
      return runUserMutation(username, 'grant-admin', (token) => api.grantAdmin(token, username))
    },
    onDeactivateUser(username) {
      return runUserMutation(username, 'deactivate', (token) => api.deactivateUser(token, username))
    },
    async onLoadMoreActivity() {
      await loadMoreActivity()
    }
  }

  async function loadMoreActivity(): Promise<void> {
    if (disposed) return
    const state = session.getState()
    if (state.status !== 'authenticated' || dashboard.status !== 'ready') return
    const currentSlice = dashboard.data.activity
    if (!currentSlice || currentSlice.status !== 'ready' || !currentSlice.data.nextCursor || currentSlice.data.loadingMore) return

    const version = loadVersion
    const loadingData = { ...currentSlice.data, loadingMore: true, paginationError: undefined }
    dashboard = { status: 'ready', data: { ...dashboard.data, activity: { status: 'ready', data: loadingData } } }
    rerender(state)

    try {
      const page = await api.activity(state.token, { limit: DEFAULT_ACTIVITY_LIMIT, cursor: currentSlice.data.nextCursor })
      if (disposed) return
      const current = session.getState()
      if (version !== loadVersion || current.status !== 'authenticated' || current.token !== state.token || dashboard.status !== 'ready') return
      const latest = dashboard.data.activity
      const base = latest?.status === 'ready' ? latest.data : currentSlice.data
      dashboard = { status: 'ready', data: { ...dashboard.data, activity: { status: 'ready', data: appendActivityPage(base, page) } } }
      rerender(current)
    } catch (error) {
      if (disposed) return
      const current = session.getState()
      if (version !== loadVersion || current.status !== 'authenticated' || current.token !== state.token || dashboard.status !== 'ready') return
      const latest = dashboard.data.activity
      const base = latest?.status === 'ready' ? latest.data : currentSlice.data
      dashboard = { status: 'ready', data: { ...dashboard.data, activity: { status: 'ready', data: { ...base, loadingMore: false, paginationError: messageFor(error) } } } }
      rerender(current)
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
    const routePath = currentRoutePath()
    const discoveryRouteKey = isQuerySensitiveDiscoveryScreen(screen) ? routeAndQueryFromRoutePath(routePath) : undefined
    const memoryDetailRouteKey = memoryDetailRouteKeyForScreen(screen, routePath)
    const loaded = await loadForRoute(screen, api, state.token, dashboard, routePath)
    if (disposed) return
    const current = session.getState()
    if (version !== loadVersion || current.status !== 'authenticated' || current.token !== state.token) return
    if (discoveryRouteKey !== undefined && (screenFromPath(currentRoutePath()) !== screen || routeAndQueryFromRoutePath(currentRoutePath()) !== discoveryRouteKey)) return
    if (memoryDetailRouteKey !== undefined && memoryDetailRouteKeyForScreen(screen, currentRoutePath()) !== memoryDetailRouteKey) return
    dashboard = loaded
    rerender(current)
  }

  async function setState(state: AuthState): Promise<void> {
    if (disposed) return
    const version = loadVersion + 1
    loadVersion = version
    const routePath = currentRoutePath()
    const screen = screenFromPath(routePath)
    const discoveryRouteKey = isQuerySensitiveDiscoveryScreen(screen) ? routeAndQueryFromRoutePath(routePath) : undefined
    const memoryDetailRouteKey = memoryDetailRouteKeyForScreen(screen, routePath)
    activeScreen = screen
    rerender(state)
    if (state.status === 'authenticated') {
      const loaded = await loadForRoute(screen, api, state.token, dashboard, routePath)
      if (disposed) return
      const current = session.getState()
      if (version !== loadVersion || current.status !== 'authenticated' || current.token !== state.token) return
      if (discoveryRouteKey !== undefined && (screenFromPath(currentRoutePath()) !== screen || routeAndQueryFromRoutePath(currentRoutePath()) !== discoveryRouteKey)) return
      if (memoryDetailRouteKey !== undefined && memoryDetailRouteKeyForScreen(screen, currentRoutePath()) !== memoryDetailRouteKey) return
      dashboard = loaded
      rerender(current)
    }
  }

  const handler = () => {
    if (disposed) return
    loadVersion += 1
    const state = session.getState()
    activeScreen = screenFromPath(currentRoutePath())
    rerender(state)
    if (state.status === 'authenticated') {
      void loadAndRender(state, activeScreen)
    }
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
