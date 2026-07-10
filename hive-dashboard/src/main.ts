import { createApiClient, type AdminStats, type ApiClient, type Count, type CreateUserRequest, type Health, type Memory, type MemoryList, type MemoryListParams, type MemorySearch, type OverviewGrowth, type OverviewProjectSyncHealth, type OverviewStats, type ProjectBlockRequest, type ProjectListResponse, type ProjectSummary, type SyncAttemptSummary, type User } from './api/client'
import { parseDashboardFilters } from './api/urlFilters'
import { createSessionStore, type AuthState, type SessionStore } from './auth/session'
import { renderBrand } from './components/Brand'
import { renderSidebar, type UserLevel } from './components/Sidebar'
import { activityFeedFromApi, appendActivityPage } from './domain/activityFeed'
import { projectsFromApi, relativeActivityAgeLabel, type ActivityFeedViewModel, type CurrentProfileViewModel, type DashboardScreenKey, type OverviewFixtureViewModel, type ProjectListViewModel, type ProjectSyncStatus } from './domain/dashboard'
import { memoryListToDiscoveryData, type KnowledgeDiscoveryData } from './domain/knowledgeDiscovery'
import { dashboardFixtures } from './fixtures/hive-dashboard/index'
import { renderAuditSync } from './views/AuditSync'
import { renderKnowledgeBrowser } from './views/KnowledgeBrowser'
import { renderMemories, type MemoryDetailData, type MemoryDetailRoute, type MemoryDetailViewState } from './views/Memories'
import { renderOverview, type ViewState } from './views/Overview'
import { renderProjects } from './views/Projects'
import { renderUsers, type UserManagementActions, type UserManagementActionType } from './views/Users'
import { renderActivityFeed } from './views/ActivityFeed'
import './styles.css'

export const DEFAULT_MEMORY_SEARCH_QUERY = 'dashboard'
export const DEFAULT_ACTIVITY_LIMIT = 20
export const LOGIN_TIMEOUT_MS = 15_000
const LOGIN_TIMEOUT_MESSAGE = 'Sign in timed out. Please try again.'
const LEGACY_GLOBAL_SEARCH_PATH = '/dashboard/globalSearch'

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
  projects?: ViewState<ProjectListViewModel>
  activity: ViewState<ActivityFeedViewModel>
  knowledgeBrowser: ViewState<KnowledgeDiscoveryData>
}
export type DashboardState = { status: 'loading' } | { status: 'ready'; data: Partial<LoadedDashboardData> }

export type AppActions = {
  onLogin(email: string, password: string): Promise<void> | void
  onLogout(): void
  onNavigate?(path: string): void
  onCreateUser?(request: CreateUserRequest): Promise<void>
  onSetUserLevel?(username: string, level: UserLevel): Promise<void>
  onGrantAdmin?(username: string): Promise<void>
  onDeactivateUser?(username: string): Promise<void>
  onResetTemporaryPassword?(username: string, temporaryPassword: string): Promise<void>
  onActivateUser?(username: string): Promise<void>
  onLoadMoreActivity?(): Promise<void>
  onBlockProject?(request: ProjectBlockRequest): Promise<void>
}

export type UserManagementState = {
  pendingAction?: { username: string; type: UserManagementActionType }
  mutationError?: string
  refreshError?: string
}

export type ProjectBlockState = {
  pendingProject?: string
  pendingOperationId?: number
  mutationError?: string
  refreshError?: string
}

export type RenderAppOptions = {
  container: HTMLElement
  state: AuthState
  actions: AppActions
  dashboard?: DashboardState
  routePath?: string
  userManagementState?: UserManagementState
  projectBlockState?: ProjectBlockState
  disposeActivityFeed?: () => void
  setActivityFeedDispose?: (fn: () => void) => void
}

type ScreenRoute = {
  path: string
  load?: keyof LoadedDashboardData
  render: (vs: ViewState<unknown>, routePath: string, actions: AppActions, context: RouteRenderContext) => HTMLElement
}

type RouteRenderContext = {
  auth: Extract<AuthState, { status: 'authenticated' }>
  projectBlockState: ProjectBlockState
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
    render: (vs, _routePath, actions, context) => renderProjects(vs as ViewState<ProjectListViewModel>, {
      currentUserLevel: context.auth.user.level,
      onBlockProject: actions.onBlockProject,
      pendingBlockProject: context.projectBlockState.pendingProject,
      mutationError: context.projectBlockState.mutationError,
      refreshError: context.projectBlockState.refreshError
    })
  },
  knowledgeBrowser: {
    path: '/dashboard/knowledgeBrowser',
    load: 'knowledgeBrowser',
    render: (vs, routePath, actions) => renderKnowledgeBrowser(vs as ViewState<KnowledgeDiscoveryData>, queryFromRoutePath(routePath), { onNavigate: actions.onNavigate, detailOriginPath: routeAndQueryFromRoutePath(routePath) })
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
 * Per-route titles for the shell topbar.
 * Each entry: [pageTitle, eyebrow subtitle].
 */
const SCREEN_TITLES: Record<DashboardScreenKey, [string, string]> = {
  overview:         ['Hive Overview',       'central memory · live sync · governance'],
  projects:         ['Projects',            'knowledge by repository'],
  memories:         ['Knowledge Browser',   'explore, filter & export team memory'],
  knowledgeBrowser: ['Knowledge Browser',   'explore, filter & export team memory'],
  activityFeed:     ['Activity Feed',       'recently saved memory across the team'],
  userManagement:   ['User Management',     'roles, access & governance'],
  auditLog:         ['Audit Log',           'system operations & governance events'],
  // Hidden/deferred screens fall back to their nearest visible equivalents
  knowledgeGraph:   ['Hive Overview',       'central memory · live sync · governance'],
  contributors:     ['Hive Overview',       'central memory · live sync · governance'],
  developerTimeline:['Hive Overview',       'central memory · live sync · governance'],
  syncStatus:       ['Hive Overview',       'central memory · live sync · governance'],
  analytics:        ['Hive Overview',       'central memory · live sync · governance'],
  conflictViewer:   ['Hive Overview',       'central memory · live sync · governance'],
}

/**
 * Resolve the active DashboardScreenKey from a URL path.
 * Strips trailing slash, matches exact path against ROUTES table.
 * Falls back to 'overview'.
 */
function screenFromPath(routePath: string): DashboardScreenKey {
  const canonical = canonicalDashboardRoutePath(routePath)
  const normalized = canonical.split(/[?#]/, 1)[0].replace(/\/$/, '')
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
  projectBlockState = {},
  disposeActivityFeed = () => {},
  setActivityFeedDispose = () => {}
}: RenderAppOptions): void {
  if (state.status === 'anonymous') {
    disposeActivityFeed()
  }
  container.replaceChildren()
  state.status === 'anonymous'
    ? renderLogin(container, state, actions)
    : renderShell(container, state, actions, dashboard, routePath, userManagementState, projectBlockState, disposeActivityFeed, setActivityFeedDispose)
}

function renderLogin(
  container: HTMLElement,
  state: Extract<AuthState, { status: 'anonymous' }>,
  actions: AppActions
): void {
  const login = document.createElement('section')
  login.className = 'dashboard-login'
  const form = document.createElement('form')
  form.className = 'dashboard-panel panel login-card'
  form.dataset.dashboardPrimitive = 'panel'
  form.innerHTML = `
    ${renderBrand({ variant: 'login' })}
    <h1 class="login-card__title">Sign in to NEXUS HIVE</h1>
    ${state.error ? `<p class="error" role="alert">${escapeHtml(state.error)}</p>` : ''}
    <label>Email<input name="email" type="email" autocomplete="email" required /></label>
    <label>Password<input name="password" type="password" autocomplete="current-password" required /></label>
    <button type="submit">Sign in</button>
  `
  const submit = form.querySelector<HTMLButtonElement>('button[type="submit"]')
  submit?.classList.add('dashboard-control', 'control')
  submit?.setAttribute('data-dashboard-primitive', 'control')
  let pending = false
  form.addEventListener('submit', async (event) => {
    event.preventDefault()
    if (pending) return
    pending = true
    if (submit) {
      submit.disabled = true
      submit.textContent = 'Signing in…'
    }
    const data = new FormData(form)
    try {
      await actions.onLogin(String(data.get('email') ?? ''), String(data.get('password') ?? ''))
    } finally {
      if (!container.contains(form)) return
      pending = false
      if (submit) {
        submit.disabled = false
        submit.textContent = 'Sign in'
      }
    }
  })
  login.append(form)
  container.append(login)
}

function renderShell(
  container: HTMLElement,
  state: Extract<AuthState, { status: 'authenticated' }>,
  actions: AppActions,
  dashboard: DashboardState,
  routePath: string,
  userManagementState: UserManagementState,
  projectBlockState: ProjectBlockState,
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
  layout.append(sidebarContainer)

  // Main area (header + content)
  const mainArea = document.createElement('div')
  mainArea.className = 'dashboard-main-area'

  // Header band
  const header = document.createElement('header')
  header.className = 'dashboard-header'
  header.dataset.dashboardPrimitive = 'header'
  header.setAttribute('role', 'banner')

  // Per-route title + eyebrow (no brand in the topbar — brand lives only in sidebar)
  const [pageTitle, eyebrow] = SCREEN_TITLES[activeScreen]
  const titleGroup = document.createElement('div')
  titleGroup.className = 'dashboard-header__title-group'
  titleGroup.innerHTML = `
    <p class="dashboard-header__eyebrow">${escapeHtml(eyebrow)}</p>
    <h1 class="dashboard-header__title">${escapeHtml(pageTitle)}</h1>
  `
  header.append(titleGroup)

  // Search slot — real input field that navigates to the canonical Knowledge Browser search surface.
  // Deliberately NOT a <form> to avoid interfering with in-view forms.
  const searchWrapper = document.createElement('div')
  searchWrapper.className = 'dashboard-header__search-wrapper'
  searchWrapper.setAttribute('role', 'search')
  const searchSlot = document.createElement('input')
  searchSlot.type = 'search'
  searchSlot.className = 'dashboard-header__search'
  searchSlot.placeholder = 'Search all memories…'
  searchSlot.setAttribute('aria-label', 'Search memories')
  searchSlot.addEventListener('keydown', (event) => {
    if (event.key === 'Enter') {
      event.preventDefault()
      actions.onNavigate?.(knowledgeBrowserPathFromRoutePath(routePath, searchSlot.value))
    }
  })
  searchSlot.addEventListener('click', () => actions.onNavigate?.(knowledgeBrowserPathFromRoutePath(routePath)))
  searchWrapper.append(searchSlot)
  header.append(searchWrapper)
  mainArea.append(header)

  // Content area
  const mainContent = document.createElement('main')
  mainContent.className = 'dashboard-content'
  mainContent.dataset.dashboardPrimitive = 'main'
  mainContent.append(renderAuthenticatedView(activeScreen, dashboard, routePath, state, actions, userManagementState, projectBlockState, disposeActivityFeed, setActivityFeedDispose))
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
  projectBlockState: ProjectBlockState,
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
      refreshError: userManagementState.refreshError,
      actions: userManagementActionsFromAppActions(actions)
    })
  }
  if (screen === 'memories') {
    const detailRoute = memoryDetailRouteFromPath(routePath)
    return renderMemories(stateFor(state, 'memories') as ViewState<MemoriesData>, {
      detailRoute,
      detail: detailRoute.kind === 'valid' ? memoryDetailForRoute(state, detailRoute.id) : undefined,
      onBackToMemories: () => actions.onNavigate?.(memoryDetailBackPathFromRoute(routePath))
    })
  }
  if (!route.load) {
    // Fixture-only / ComingSoon route
    return route.render({ status: 'loading' }, routePath, actions, { auth, projectBlockState })
  }
  const viewState = stateFor(state, route.load)
  return route.render(viewState as ViewState<unknown>, routePath, actions, { auth, projectBlockState })
}

function userManagementActionsFromAppActions(actions: AppActions): UserManagementActions {
  const onCreateUser = actions.onCreateUser
  const onSetUserLevel = actions.onSetUserLevel
  const onDeactivateUser = actions.onDeactivateUser
  const onResetTemporaryPassword = actions.onResetTemporaryPassword
  const onActivateUser = actions.onActivateUser

  const userActions: UserManagementActions = {
    onSetUserLevel: (username, level) => onSetUserLevel?.(username, level) ?? Promise.resolve(),
    onDeactivateUser: (username) => onDeactivateUser?.(username) ?? Promise.resolve()
  }
  if (onCreateUser) userActions.onCreateUser = (request) => onCreateUser(request)
  if (onResetTemporaryPassword) userActions.onResetTemporaryPassword = (username, temporaryPassword) => onResetTemporaryPassword(username, temporaryPassword)
  if (onActivateUser) userActions.onActivateUser = (username) => onActivateUser(username)
  return userActions
}

function queryFromRoutePath(routePath: string): string {
  const queryStart = routePath.indexOf('?')
  if (queryStart === -1) return ''
  const hashStart = routePath.indexOf('#', queryStart)
  return hashStart === -1 ? routePath.slice(queryStart) : routePath.slice(queryStart, hashStart)
}

function knowledgeBrowserPathFromRoutePath(routePath: string, submittedQuery = ''): string {
  const queryOverride = submittedQuery.trim()
  if (queryOverride) return `${ROUTES.knowledgeBrowser.path}?${new URLSearchParams({ query: queryOverride }).toString()}`
  const normalized = routePath.split(/[?#]/, 1)[0].replace(/\/$/, '')
  const query = queryFromRoutePath(routePath)
  if (query && (normalized === ROUTES.knowledgeBrowser.path || normalized === LEGACY_GLOBAL_SEARCH_PATH)) {
    return `${ROUTES.knowledgeBrowser.path}${query}`
  }
  return ROUTES.knowledgeBrowser.path
}

function memoryDetailBackPathFromRoute(routePath: string): string {
  return safeDashboardReturnPath(new URLSearchParams(queryFromRoutePath(routePath)).get('returnTo')) ?? ROUTES.knowledgeBrowser.path
}

function safeDashboardReturnPath(value: string | null): string | undefined {
  const candidate = value?.trim()
  if (!candidate || candidate.startsWith('//')) return undefined
  try {
    const route = new URL(candidate, window.location.origin)
    if (route.origin !== window.location.origin) return undefined
    const normalized = canonicalDashboardRoutePath(`${route.pathname}${route.search}${route.hash}`)
    const normalizedPath = normalized.split(/[?#]/, 1)[0]
    if (!isDiscoveryReturnPath(normalizedPath)) return undefined
    const path = normalized
    return path
  } catch {
    return undefined
  }
}

function isDiscoveryReturnPath(pathname: string): boolean {
  return pathname === ROUTES.knowledgeBrowser.path
}

function canonicalDashboardRoutePath(routePath: string): string {
  try {
    const route = new URL(routePath, window.location.origin)
    if (route.origin !== window.location.origin) return routePath
    if (route.pathname.replace(/\/$/, '') !== LEGACY_GLOBAL_SEARCH_PATH) return routePath
    return `${ROUTES.knowledgeBrowser.path}${route.search}${route.hash}`
  } catch {
    return routePath
  }
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

function projectsLoadingState(state: DashboardState): DashboardState {
  if (state.status !== 'ready') return state
  return { status: 'ready', data: { ...state.data, projects: { status: 'loading' } } }
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
      knowledgeBrowser: discoveryListState(recent)
    }
  }
}

export async function loadForRoute(
  screen: DashboardScreenKey,
  api: ApiClient,
  token: string,
  cache: DashboardState,
  routePath = ROUTES[screen].path,
  userLevel?: UserLevel | string
): Promise<DashboardState> {
  const route = ROUTES[screen]
  if (screen === 'memories') {
    const detailRoute = memoryDetailRouteFromPath(routePath)
    if (detailRoute.kind === 'malformed') return cache
    if (detailRoute.kind === 'valid') return loadMemoryDetail(detailRoute, api, token, cache)
  }
  if (HIDDEN_DASHBOARD_SCREENS.has(screen)) return loadForRoute('overview', api, token, cache, ROUTES.overview.path, userLevel)
  if (!route.load) return cache

  const key = route.load
  // Already cached
  if (key !== 'projects' && !isQuerySensitiveDiscoveryKey(key) && cache.status === 'ready' && cache.data[key] !== undefined) return cache

  const existingData = cache.status === 'ready' ? cache.data : {}

  let slice: ViewState<unknown>
  try {
    slice = await fetchSlice(key, api, token, routePath, userLevel)
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

async function fetchSlice(key: keyof LoadedDashboardData, api: ApiClient, token: string, routePath = '', userLevel?: UserLevel | string): Promise<ViewState<unknown>> {
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
    case 'projects': {
      const response = await loadProjects(api, token, userLevel)
      return { status: 'ready', data: projectsFromApi(response) }
    }
    case 'activity': {
      const result = await Promise.allSettled([api.activity(token, { limit: DEFAULT_ACTIVITY_LIMIT })])
      return activityState(result[0])
    }
    case 'knowledgeBrowser': {
      const response = await api.memories(token, memoryListParamsFromRoute(routePath))
      return { status: 'ready', data: memoryListToDiscoveryData(response) }
    }
  }
}

async function loadProjects(api: ApiClient, token: string, userLevel?: UserLevel | string): Promise<ProjectListResponse> {
  const response = await api.projects(token)
  if (userLevel !== 'admin' || response.projects.length === 0) return response
  const statuses = await Promise.all(response.projects.map((project) => api.projectBlockStatus(token, project.name)))
  const projects = response.projects.map((project, index) => projectWithBlockStatus(project, statuses[index]))
  return { ...response, projects }
}

function projectWithBlockStatus(project: ProjectSummary, status: Awaited<ReturnType<ApiClient['projectBlockStatus']>>): ProjectSummary {
  return {
    ...project,
    blocked: status.blocked,
    canonicalProjectKey: status.blocked || !project.canonicalProjectKey ? status.canonical_project_key : project.canonicalProjectKey,
    blockReason: status.reason ?? null,
    blockAckStatus: status.ack?.status ?? null
  }
}

function discoveryListState(result: PromiseSettledResult<MemoryList>): ViewState<KnowledgeDiscoveryData> {
  return result.status === 'fulfilled'
    ? { status: 'ready', data: memoryListToDiscoveryData(result.value) }
    : { status: 'error', message: messageFor(result.reason) }
}

function isQuerySensitiveDiscoveryKey(key: keyof LoadedDashboardData): boolean {
  return key === 'knowledgeBrowser'
}

function isQuerySensitiveDiscoveryScreen(screen: DashboardScreenKey): boolean {
  const key = ROUTES[screen].load
  return key !== undefined && isQuerySensitiveDiscoveryKey(key)
}

function routeAndQueryFromRoutePath(routePath: string): string {
  return canonicalDashboardRoutePath(routePath)
}

function memoryDetailRouteKeyForScreen(screen: DashboardScreenKey, routePath: string): string | undefined {
  if (screen !== 'memories') return undefined
  const route = memoryDetailRouteFromPath(routePath)
  return route.kind === 'valid' ? route.routeKey : undefined
}

function memoryListParamsFromRoute(routePath: string): MemoryListParams {
  const filters = parseDashboardFilters(queryFromRoutePath(routePath))
  const query = filters.query?.trim()
  return {
    query: query || undefined,
    project: filters.project,
    category: filters.category && filters.category !== 'all' ? filters.category : undefined,
    from: filters.from,
    until: filters.until,
    limit: filters.limit,
    offset: filters.offset
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
  return `NEXUS HIVE health is degraded: ${issues.join(', ')}`
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
    contributorCount: project.contributor_count,
    lastActivityLabel: relativeActivityAgeLabel(project.last_activity_at)
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
  let projectBlockState: ProjectBlockState = {}
  let projectBlockOperationId = 0
  let loginAttemptVersion = 0
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
      projectBlockState,
      disposeActivityFeed,
      setActivityFeedDispose
    })
  }

  const actions: AppActions = {
    async onLogin(email, password) {
      if (disposed) return
      const attemptVersion = ++loginAttemptVersion
      const ownsLoginAttempt = () => !disposed && attemptVersion === loginAttemptVersion
      // Dashboard browsers support AbortController; ownership remains the guard if a fetch ignores abort.
      const controller = new AbortController()
      const timeout = window.setTimeout(() => {
        if (!ownsLoginAttempt()) {
          controller.abort()
          return
        }
        loginAttemptVersion += 1
        controller.abort()
        rerender({ status: 'anonymous', error: LOGIN_TIMEOUT_MESSAGE })
      }, LOGIN_TIMEOUT_MS)
      try {
        const state = await session.loginWithOwnership(email, password, ownsLoginAttempt, controller.signal)
        window.clearTimeout(timeout)
        if (disposed || attemptVersion !== loginAttemptVersion) return
        await setState(state)
      } catch (error) {
        if (disposed || attemptVersion !== loginAttemptVersion) return
        rerender({ status: 'anonymous', error: loginErrorMessage(error) })
      } finally {
        window.clearTimeout(timeout)
      }
    },
    onLogout() {
      if (disposed) return
      loginAttemptVersion += 1
      loadVersion += 1
      dashboard = { status: 'loading' }
      userManagementState = {}
      projectBlockState = {}
      rerender(session.logout())
    },
    async onNavigate(path) {
      if (disposed) return
      history.pushState(null, '', canonicalDashboardRoutePath(path))
      loadVersion += 1
      projectBlockState = {}
      activeScreen = screenFromPath(currentRoutePath())
      if (activeScreen === 'projects') dashboard = projectsLoadingState(dashboard)
      const state = session.getState()
      rerender(state)
      if (state.status === 'authenticated') {
        await loadAndRender(state, activeScreen)
      }
    },
    onCreateUser(request) {
      return runUserMutation(request.username, 'create', (token) => api.createUser(token, request))
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
    onResetTemporaryPassword(username, temporaryPassword) {
      return runUserMutation(username, 'reset-password', (token) => api.resetTemporaryPassword(token, username, temporaryPassword))
    },
    onActivateUser(username) {
      return runUserMutation(username, 'activate', (token) => api.activateUser(token, username))
    },
    async onLoadMoreActivity() {
      await loadMoreActivity()
    },
    async onBlockProject(request) {
      await runProjectBlock(request)
    }
  }

  async function runProjectBlock(request: ProjectBlockRequest): Promise<void> {
    if (disposed) return
    const state = session.getState()
    if (state.status !== 'authenticated') return
    if (projectBlockState.pendingProject) return
    const version = loadVersion
    const operationId = projectBlockOperationId + 1
    projectBlockOperationId = operationId
    projectBlockState = { pendingProject: request.project, pendingOperationId: operationId }
    rerender(state)
    try {
      await api.blockProject(state.token, request)
    } catch (error) {
      if (disposed) return
      const current = session.getState()
      if (version !== loadVersion || current.status !== 'authenticated' || current.token !== state.token) {
        clearProjectBlockPendingAction(request.project, operationId, current)
        return
      }
      projectBlockState = { mutationError: `Project block failed: ${messageFor(error)}.` }
      rerender(current)
      return
    }
    if (disposed) return
    const current = session.getState()
    if (version !== loadVersion || current.status !== 'authenticated' || current.token !== state.token || screenFromPath(currentRoutePath()) !== 'projects') {
      clearProjectBlockPendingAction(request.project, operationId, current)
      return
    }
    const existingData = dashboard.status === 'ready' ? dashboard.data : {}
    let slice: ViewState<ProjectListViewModel>
    try {
      slice = await fetchSlice('projects', api, state.token, currentRoutePath(), state.user.level) as ViewState<ProjectListViewModel>
    } catch (error) {
      projectBlockState = { refreshError: `Block succeeded, but Projects could not be refreshed: ${messageFor(error)}. Refresh the page to confirm the latest state.` }
      rerender(current)
      return
    }
    if (disposed) return
    const refreshedCurrent = session.getState()
    if (version !== loadVersion || refreshedCurrent.status !== 'authenticated' || refreshedCurrent.token !== state.token || screenFromPath(currentRoutePath()) !== 'projects') {
      clearProjectBlockPendingAction(request.project, operationId, refreshedCurrent)
      return
    }
    projectBlockState = {}
    dashboard = { status: 'ready', data: { ...existingData, projects: slice } }
    rerender(refreshedCurrent)
  }

  function clearProjectBlockPendingAction(pendingProject: string, pendingOperationId: number, state: AuthState): void {
    if (projectBlockState.pendingProject !== pendingProject || projectBlockState.pendingOperationId !== pendingOperationId) return
    projectBlockState = {}
    if (!disposed) rerender(state)
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
    const pendingAction = { username, type }
    userManagementState = { pendingAction }
    rerender(state)
    try {
      await mutate(state.token)
      if (disposed) return
      const current = session.getState()
      if (version !== loadVersion || current.status !== 'authenticated' || current.token !== state.token) {
        await recoverUsersAfterStaleSuccessfulMutation(pendingAction, current, state.token)
        return
      }
      const existingData = dashboard.status === 'ready' ? dashboard.data : {}
      let slice: ViewState<UsersData>
      try {
        slice = await fetchSlice('users', api, state.token) as ViewState<UsersData>
      } catch (refreshError) {
        userManagementState = { refreshError: usersRefreshFailureMessage(refreshError) }
        dashboard = { status: 'ready', data: existingData }
        rerender(current)
        return
      }
      if (disposed) return
      const refreshedCurrent = session.getState()
      if (version !== loadVersion || refreshedCurrent.status !== 'authenticated' || refreshedCurrent.token !== state.token) {
        await recoverUsersAfterStaleSuccessfulMutation(pendingAction, refreshedCurrent, state.token)
        return
      }
      if (slice.status === 'error') {
        userManagementState = { refreshError: usersRefreshFailureMessage(slice.message) }
        dashboard = { status: 'ready', data: existingData }
        rerender(refreshedCurrent)
        return
      }
      dashboard = { status: 'ready', data: { ...existingData, users: slice as ViewState<UsersData> } }
      userManagementState = {}
      rerender(refreshedCurrent)
    } catch (error) {
      if (disposed) return
      const current = session.getState()
      if (version !== loadVersion || current.status !== 'authenticated' || current.token !== state.token) {
        clearUserPendingAction(pendingAction, current)
        return
      }
      userManagementState = { mutationError: messageFor(error) }
      rerender(current)
    }
  }

  async function recoverUsersAfterStaleSuccessfulMutation(
    pendingAction: { username: string; type: UserManagementActionType },
    state: AuthState,
    mutationToken: string
  ): Promise<void> {
    invalidateUsersCache()
    clearUserPendingAction(pendingAction, state)
    if (state.status !== 'authenticated' || state.token !== mutationToken) return
    if (screenFromPath(currentRoutePath()) !== 'userManagement') return
    await loadAndRender(state, 'userManagement')
  }

  function invalidateUsersCache(): void {
    if (dashboard.status !== 'ready' || dashboard.data.users === undefined) return
    const data = { ...dashboard.data }
    delete data.users
    dashboard = { status: 'ready', data }
  }

  function clearUserPendingAction(
    pendingAction: { username: string; type: UserManagementActionType },
    state: AuthState
  ): void {
    const currentPending = userManagementState.pendingAction
    if (!currentPending || currentPending.username !== pendingAction.username || currentPending.type !== pendingAction.type) return
    const nextState: UserManagementState = {}
    if (userManagementState.mutationError) nextState.mutationError = userManagementState.mutationError
    if (userManagementState.refreshError) nextState.refreshError = userManagementState.refreshError
    userManagementState = nextState
    if (!disposed) rerender(state)
  }

  function usersRefreshFailureMessage(error: unknown): string {
    const detail = typeof error === 'string' ? error : messageFor(error)
    return `Mutation succeeded, but users could not be refreshed: ${detail}. Refresh the page to confirm the latest state.`
  }

  async function loadAndRender(state: Extract<AuthState, { status: 'authenticated' }>, screen: DashboardScreenKey): Promise<void> {
    if (disposed) return
    const version = loadVersion
    const routePath = canonicalizeCurrentRoutePath()
    const discoveryRouteKey = isQuerySensitiveDiscoveryScreen(screen) ? routeAndQueryFromRoutePath(routePath) : undefined
    const memoryDetailRouteKey = memoryDetailRouteKeyForScreen(screen, routePath)
    const loaded = await loadForRoute(screen, api, state.token, dashboard, routePath, state.user.level)
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
    const routePath = canonicalizeCurrentRoutePath()
    const screen = screenFromPath(routePath)
    const discoveryRouteKey = isQuerySensitiveDiscoveryScreen(screen) ? routeAndQueryFromRoutePath(routePath) : undefined
    const memoryDetailRouteKey = memoryDetailRouteKeyForScreen(screen, routePath)
    activeScreen = screen
    if (activeScreen === 'projects') dashboard = projectsLoadingState(dashboard)
    rerender(state)
    if (state.status === 'authenticated') {
      const loaded = await loadForRoute(screen, api, state.token, dashboard, routePath, state.user.level)
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
    projectBlockState = {}
    const state = session.getState()
    canonicalizeCurrentRoutePath()
    activeScreen = screenFromPath(currentRoutePath())
    if (activeScreen === 'projects') dashboard = projectsLoadingState(dashboard)
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

function canonicalizeCurrentRoutePath(): string {
  const current = currentRoutePath()
  const canonical = canonicalDashboardRoutePath(current)
  if (canonical !== current) history.replaceState(null, '', canonical)
  return canonical
}

const root = document.getElementById('app')
if (root) {
  startDashboardApp(root)
}
