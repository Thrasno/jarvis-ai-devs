import { describe, expect, it, vi } from 'vitest'
import type { ActivityFeedResponse, ApiClient, MemoryList, MemorySearch, OverviewGrowth, OverviewStats } from './api/client'
import type { SessionStore } from './auth/session'
import { loadDashboard, renderApp, startDashboardApp } from './main'
import { dashboardNotificationSummary } from './fixtures/hive-dashboard/shared'
import { hiveOverviewFixture } from './fixtures/hive-dashboard/overview'
import { projectsFixture } from './fixtures/hive-dashboard/explore'
import { memoryListToDiscoveryData, memorySearchToDiscoveryData } from './domain/knowledgeDiscovery'

const adminUser = { id: 'admin-1', username: 'admin', email: 'admin@example.com', level: 'admin' as const, is_active: true, created_at: '2026-06-06T20:00:00Z' }
const memberUser = { id: 'member-1', username: 'member', email: 'member@example.com', level: 'member' as const, is_active: true, created_at: '2026-06-06T20:00:00Z' }

describe('dashboard shell', () => {
  it('shows the login form to unauthenticated users', () => {
    const container = document.createElement('main')

    renderApp({ container, state: { status: 'anonymous' }, actions: { onLogin: vi.fn(), onLogout: vi.fn() } })

    expect(container.querySelector('h1')?.textContent).toBe('Sign in to Hive API')
    expect(container.querySelector('form')?.getAttribute('data-dashboard-primitive')).toBe('panel')
    expect(container.querySelector('input[name="email"]')?.getAttribute('type')).toBe('email')
    expect(container.querySelector('button[type="submit"]')?.getAttribute('data-dashboard-primitive')).toBe('control')
    expect(container.textContent).not.toContain('daemon')
  })

  it('shows a useful error and keeps the login form when login fails', async () => {
    const container = document.createElement('main')
    const session = fakeSessionStore({ status: 'anonymous' })
    vi.mocked(session.login).mockRejectedValue(new Error('invalid credentials'))

    const cleanup = startDashboardApp(container, { api: fakeApi(), session })
    try {
      await Promise.resolve()
      container.querySelector<HTMLInputElement>('input[name="email"]')!.value = 'admin@example.com'
      container.querySelector<HTMLInputElement>('input[name="password"]')!.value = 'wrong'

      container.querySelector('form')!.dispatchEvent(new SubmitEvent('submit', { bubbles: true, cancelable: true }))
      await Promise.resolve()
      await Promise.resolve()

      expect(session.login).toHaveBeenCalledWith('admin@example.com', 'wrong')
      expect(container.querySelector('h1')?.textContent).toBe('Sign in to Hive API')
      expect(container.querySelector('[role="alert"]')?.textContent).toContain('invalid credentials')
    } finally {
      cleanup()
    }
  })

  it('renders the full shell (sidebar + header + main) for any authenticated user', () => {
    const container = document.createElement('main')

    renderApp({ container, state: { status: 'authenticated', token: 'jwt-token', user: memberUser }, actions: { onLogin: vi.fn(), onLogout: vi.fn() } })

    expect(container.textContent).not.toContain('Admin access required')
    expect(container.querySelector('[data-dashboard-primitive="sidebar"]')).not.toBeNull()
    expect(container.querySelector('[data-dashboard-primitive="header"]')).not.toBeNull()
    expect(container.querySelector('[data-dashboard-primitive="main"]')).not.toBeNull()
  })

  it('derives the sidebar profile from the authenticated user', () => {
    const container = document.createElement('main')

    renderApp({ container, state: { status: 'authenticated', token: 'jwt-token', user: memberUser }, actions: { onLogin: vi.fn(), onLogout: vi.fn() } })

    const profile = container.querySelector('[data-sidebar-profile]')
    expect(profile).not.toBeNull()
    expect(profile?.textContent).toContain('member')
    expect(profile?.textContent).toContain('member@example.com')
    expect(profile?.querySelector('[data-dashboard-status]')?.textContent).toBe('member')
    expect(profile?.textContent).not.toContain('Ada Okafor')
    expect(profile?.textContent).not.toContain('ada.okafor@nexus.dev')
  })

  it('renders the protected shell for an admin identity', () => {
    const container = document.createElement('main')

    renderApp({ container, state: { status: 'authenticated', token: 'jwt-token', user: adminUser }, actions: { onLogin: vi.fn(), onLogout: vi.fn() } })

    expect(container.querySelector('[data-dashboard-primitive="sidebar"]')).not.toBeNull()
    expect(container.querySelector('[data-dashboard-primitive="header"]')).not.toBeNull()
    expect(container.querySelector('[data-dashboard-primitive="main"]')).not.toBeNull()
    expect(container.querySelector('[data-dashboard-primitive="sidebar"] nav')?.getAttribute('aria-label')).toBe('Dashboard sections')
    expect(container.querySelector('[data-dashboard-primitive="sidebar"] nav')?.textContent).toContain('Dashboard')
    expect(container.textContent).not.toContain('daemon')
  })

  it('renders route navigation and the selected API-backed view for admin deep links', () => {
    const container = document.createElement('main')

    renderApp({ container, state: { status: 'authenticated', token: 'jwt-token', user: adminUser }, actions: { onLogin: vi.fn(), onLogout: vi.fn() }, dashboard: dashboardState(), routePath: '/dashboard/users' })

    expect(container.querySelector('[data-dashboard-primitive="sidebar"] nav')?.textContent).toContain('Dashboard')
    expect(container.querySelector('section h2')?.textContent).toBe('Users')
    expect(container.textContent).toContain('admin · active')
    expect(container.textContent).toContain('admin@example.com')
    expect(container.textContent).not.toContain('Authentication is active')
  })

  it('wires user management actions to the API and refreshes users after success', async () => {
    const container = document.createElement('main')
    document.body.append(container)
    const session = fakeSessionStore({ status: 'authenticated', token: 'jwt-token', user: adminUser })
    const usersBefore = { users: [adminUser, memberUser] }
    const promotedMember = { ...memberUser, level: 'viewer' as const }
    const usersAfter = { users: [adminUser, promotedMember] }
    const api = fakeApi({ users: [Promise.resolve(usersBefore), Promise.resolve(usersAfter)] })
    const originalPath = `${window.location.pathname}${window.location.search}${window.location.hash}`
    history.pushState(null, '', '/dashboard/users')

    const cleanup = startDashboardApp(container, { api, session })
    try {
      await flushDashboard()

      container.querySelector<HTMLButtonElement>('[aria-label="member management actions"] button')!.click()
      expect(container.querySelector('[role="dialog"]')?.textContent).toContain('Change member level to viewer?')
      container.querySelector<HTMLButtonElement>('[role="dialog"] button')!.click()
      await flushDashboard()

      expect(api.setUserLevel).toHaveBeenCalledWith('jwt-token', 'member', 'viewer')
      expect(api.adminUsers).toHaveBeenCalledTimes(2)
      expect(container.textContent).toContain('Level: viewer')
      expect(container.textContent).not.toContain('Level: member')
    } finally {
      cleanup()
      history.pushState(null, '', originalPath)
      container.remove()
    }
  })

  it('wires admin-seat grants to the API and refreshes users after success', async () => {
    const container = document.createElement('main')
    document.body.append(container)
    const session = fakeSessionStore({ status: 'authenticated', token: 'jwt-token', user: adminUser })
    const adminMember = { ...memberUser, level: 'admin' as const }
    const api = fakeApi({
      users: [
        Promise.resolve({ users: [adminUser, memberUser] }),
        Promise.resolve({ users: [adminUser, adminMember] })
      ]
    })
    const originalPath = `${window.location.pathname}${window.location.search}${window.location.hash}`
    history.pushState(null, '', '/dashboard/users')

    const cleanup = startDashboardApp(container, { api, session })
    try {
      await flushDashboard()

      container.querySelector<HTMLButtonElement>('button[aria-label="Grant admin to member"]')!.click()
      container.querySelector<HTMLButtonElement>('[role="dialog"] button')!.click()
      await flushDashboard()

      expect(api.grantAdmin).toHaveBeenCalledWith('jwt-token', 'member')
      expect(api.adminUsers).toHaveBeenCalledTimes(2)
      expect(container.textContent).toContain('Admin seat: yes')
    } finally {
      cleanup()
      history.pushState(null, '', originalPath)
      container.remove()
    }
  })

  it('shows pending and API error state for user management mutations without refreshing on failure', async () => {
    const container = document.createElement('main')
    document.body.append(container)
    const session = fakeSessionStore({ status: 'authenticated', token: 'jwt-token', user: adminUser })
    const mutation = deferred<Awaited<ReturnType<ApiClient['deactivateUser']>>>()
    const api = fakeApi({
      users: [Promise.resolve({ users: [adminUser, memberUser] })],
      deactivateUser: mutation.promise
    })
    const originalPath = `${window.location.pathname}${window.location.search}${window.location.hash}`
    history.pushState(null, '', '/dashboard/users')

    const cleanup = startDashboardApp(container, { api, session })
    try {
      await flushDashboard()

      container.querySelector<HTMLButtonElement>('button[aria-label="Deactivate member"]')!.click()
      container.querySelector<HTMLButtonElement>('[role="dialog"] button')!.click()
      await flushDashboard()

      expect(container.querySelector<HTMLButtonElement>('button[aria-label="Deactivating member"]')?.disabled).toBe(true)
      mutation.reject(new Error('insufficient active admins'))
      await flushDashboard()

      expect(api.deactivateUser).toHaveBeenCalledWith('jwt-token', 'member')
      expect(api.adminUsers).toHaveBeenCalledTimes(1)
      expect(container.querySelector('[role="alert"]')?.textContent).toContain('insufficient active admins')
      expect(container.textContent).toContain('member@example.com')
    } finally {
      cleanup()
      history.pushState(null, '', originalPath)
      container.remove()
    }
  })

  it('does not expose user management controls to non-admin sessions', async () => {
    const container = document.createElement('main')
    document.body.append(container)
    const session = fakeSessionStore({ status: 'authenticated', token: 'jwt-token', user: memberUser })
    const api = fakeApi({ users: [Promise.resolve({ users: [adminUser, memberUser] })] })
    const originalPath = `${window.location.pathname}${window.location.search}${window.location.hash}`
    history.pushState(null, '', '/dashboard/users')

    const cleanup = startDashboardApp(container, { api, session })
    try {
      await flushDashboard()

      expect(container.querySelector('section h2')?.textContent).toBe('Users')
      expect(container.textContent).toContain('member@example.com')
      expect(container.querySelector('[aria-label="member management actions"]')).toBeNull()
      expect(container.querySelector('button[aria-label="Grant admin to member"]')).toBeNull()
    } finally {
      cleanup()
      history.pushState(null, '', originalPath)
      container.remove()
    }
  })

  it('keeps the plain audit sync legacy alias available', () => {
    const container = document.createElement('main')

    renderApp({ container, state: { status: 'authenticated', token: 'jwt-token', user: adminUser }, actions: { onLogin: vi.fn(), onLogout: vi.fn() }, dashboard: dashboardState(), routePath: '/dashboard/audit-sync' })

    expect(container.querySelector('section h2')?.textContent).toBe('Sync attempt audit reliability')
    expect(container.textContent).not.toContain('Memory detail is unavailable')
  })

  it('renders ComingSoon for an unimplemented route', () => {
    const container = document.createElement('main')

    renderApp({ container, state: { status: 'authenticated', token: 'jwt-token', user: adminUser }, actions: { onLogin: vi.fn(), onLogout: vi.fn() }, dashboard: { status: 'loading' }, routePath: '/dashboard/knowledgeGraph' })

    expect(container.querySelector('[data-coming-soon]')).not.toBeNull()
    expect(container.textContent).toContain('Knowledge Graph')
  })

  it('renders Activity Feed from route-loaded API data instead of fixture data', () => {
    const container = document.createElement('main')

    renderApp({
      container,
      state: { status: 'authenticated', token: 'jwt-token', user: adminUser },
      actions: { onLogin: vi.fn(), onLogout: vi.fn() },
      dashboard: { status: 'ready', data: { activity: { status: 'ready', data: activityViewState() } } },
      routePath: '/dashboard/activityFeed'
    })

    expect(container.querySelector('[data-coming-soon]')).toBeNull()
    expect(container.querySelector('section h2')?.textContent).toBe('Activity Feed')
    expect(container.textContent).toContain('Real backend activity')
    expect(container.textContent).not.toContain('Demo fixture data')
  })

  it('does not start Activity Feed polling timers when rendering the real route', () => {
    const container = document.createElement('main')
    const setIntervalSpy = vi.spyOn(window, 'setInterval')

    try {
      renderApp({
        container,
        state: { status: 'authenticated', token: 'jwt-token', user: adminUser },
        actions: { onLogin: vi.fn(), onLogout: vi.fn() },
        dashboard: { status: 'ready', data: { activity: { status: 'ready', data: activityViewState() } } },
        routePath: '/dashboard/activityFeed',
      })

      expect(container.querySelector('section h2')?.textContent).toBe('Activity Feed')
      expect(setIntervalSpy).not.toHaveBeenCalled()
    } finally {
      setIntervalSpy.mockRestore()
    }
  })

  it('loads Activity Feed through /activity?limit=20 when the route opens', async () => {
    const container = document.createElement('main')
    document.body.append(container)
    const session = fakeSessionStore({ status: 'authenticated', token: 'jwt-token', user: adminUser })
    const api = fakeApi({ activity: [Promise.resolve(activityResponse('event-1', 'Real backend activity', 'cursor-2'))] })
    const originalPath = `${window.location.pathname}${window.location.search}${window.location.hash}`
    history.pushState(null, '', '/dashboard/activityFeed')

    const cleanup = startDashboardApp(container, { api, session })
    try {
      await flushDashboard()

      expect(api.activity).toHaveBeenCalledWith('jwt-token', { limit: 20 })
      expect(container.textContent).toContain('Real backend activity')
      expect(container.textContent).not.toContain('Demo fixture data')
    } finally {
      cleanup()
      history.pushState(null, '', originalPath)
      container.remove()
    }
  })

  it('renders route-loaded empty Activity Feed responses without fixture fallback or Load More', async () => {
    const container = document.createElement('main')
    document.body.append(container)
    const session = fakeSessionStore({ status: 'authenticated', token: 'jwt-token', user: adminUser })
    const api = fakeApi({ activity: [Promise.resolve({ entries: [] })] })
    const originalPath = `${window.location.pathname}${window.location.search}${window.location.hash}`
    history.pushState(null, '', '/dashboard/activityFeed')

    const cleanup = startDashboardApp(container, { api, session })
    try {
      await flushDashboard()

      expect(api.activity).toHaveBeenCalledWith('jwt-token', { limit: 20 })
      expect(container.querySelector('[role="status"]')?.textContent).toContain('No activity entries found')
      expect(container.querySelector('button[data-load-more-activity]')).toBeNull()
      expect(container.textContent).not.toContain('Demo fixture data')
    } finally {
      cleanup()
      history.pushState(null, '', originalPath)
      container.remove()
    }
  })

  it('renders route-loaded Activity Feed failures without fixture fallback', async () => {
    const container = document.createElement('main')
    document.body.append(container)
    const session = fakeSessionStore({ status: 'authenticated', token: 'jwt-token', user: adminUser })
    const api = fakeApi({ activity: [() => Promise.reject(new Error('activity API unavailable'))] })
    const originalPath = `${window.location.pathname}${window.location.search}${window.location.hash}`
    history.pushState(null, '', '/dashboard/activityFeed')

    const cleanup = startDashboardApp(container, { api, session })
    try {
      await flushDashboard()

      expect(api.activity).toHaveBeenCalledWith('jwt-token', { limit: 20 })
      expect(container.querySelector('[role="alert"]')?.textContent).toContain('activity API unavailable')
      expect(container.querySelector('button[data-load-more-activity]')).toBeNull()
      expect(container.textContent).not.toContain('Demo fixture data')
      expect(container.textContent).not.toContain('Real backend activity')
    } finally {
      cleanup()
      history.pushState(null, '', originalPath)
      container.remove()
    }
  })

  it('does not offer Activity Feed Load More when route-loaded next_cursor is null or undefined', async () => {
    for (const response of [
      { ...activityResponse('event-null-cursor', 'Null cursor activity'), next_cursor: null },
      { entries: activityResponse('event-undefined-cursor', 'Undefined cursor activity').entries }
    ]) {
      const container = document.createElement('main')
      document.body.append(container)
      const session = fakeSessionStore({ status: 'authenticated', token: 'jwt-token', user: adminUser })
      const api = fakeApi({ activity: [Promise.resolve(response)] })
      const originalPath = `${window.location.pathname}${window.location.search}${window.location.hash}`
      history.pushState(null, '', '/dashboard/activityFeed')

      const cleanup = startDashboardApp(container, { api, session })
      try {
        await flushDashboard()

        expect(container.textContent).toContain(response.entries[0].title)
        expect(container.querySelector('button[data-load-more-activity]')).toBeNull()
      } finally {
        cleanup()
        history.pushState(null, '', originalPath)
        container.remove()
      }
    }
  })

  it('appends Activity Feed Load More pages and keeps existing entries visible on pagination failure', async () => {
    const container = document.createElement('main')
    document.body.append(container)
    const session = fakeSessionStore({ status: 'authenticated', token: 'jwt-token', user: adminUser })
    const api = fakeApi({
      activity: [
        Promise.resolve(activityResponse('event-1', 'First page activity', 'cursor-2')),
        Promise.resolve(activityResponse('event-2', 'Second page activity', 'cursor-3')),
        () => Promise.reject(new Error('next page failed'))
      ]
    })
    const originalPath = `${window.location.pathname}${window.location.search}${window.location.hash}`
    history.pushState(null, '', '/dashboard/activityFeed')

    const cleanup = startDashboardApp(container, { api, session })
    try {
      await flushDashboard()
      container.querySelector<HTMLButtonElement>('button[data-load-more-activity]')!.click()
      await flushDashboard()

      expect(api.activity).toHaveBeenNthCalledWith(2, 'jwt-token', { limit: 20, cursor: 'cursor-2' })
      expect(container.textContent).toContain('First page activity')
      expect(container.textContent).toContain('Second page activity')

      container.querySelector<HTMLButtonElement>('button[data-load-more-activity]')!.click()
      await flushDashboard()

      expect(container.textContent).toContain('First page activity')
      expect(container.textContent).toContain('Second page activity')
      expect(container.textContent).toContain('next page failed')
    } finally {
      cleanup()
      history.pushState(null, '', originalPath)
      container.remove()
    }
  })

  it('shows the named default memory search in the memories view', () => {
    const container = document.createElement('main')

    renderApp({ container, state: { status: 'authenticated', token: 'jwt-token', user: adminUser }, actions: { onLogin: vi.fn(), onLogout: vi.fn() }, dashboard: dashboardState(), routePath: '/dashboard/memories' })

    expect(container.querySelector('[data-dashboard-primitive="main"]')?.textContent).toContain('Default search: "dashboard"')
  })

  it('renders the Projects view instead of ComingSoon for an authenticated member', () => {
    const container = document.createElement('main')

    renderApp({ container, state: { status: 'authenticated', token: 'jwt-token', user: memberUser }, actions: { onLogin: vi.fn(), onLogout: vi.fn() }, dashboard: dashboardState(), routePath: '/dashboard/projects' })

    expect(container.querySelector('[data-coming-soon]')).toBeNull()
    expect(container.querySelector('[role="note"]')?.textContent).toBe('Demo fixture data — live project summaries are unavailable.')
    expect(container.querySelector('[aria-label="Project summaries"]')).not.toBeNull()
    expect(container.querySelector<HTMLAnchorElement>('a[href="/dashboard/memories?project=core-api"]')?.textContent).toBe('Browse memories')
  })

  it('matches the memories route when project-scoped browse navigation includes query and hash', () => {
    const container = document.createElement('main')

    renderApp({ container, state: { status: 'authenticated', token: 'jwt-token', user: adminUser }, actions: { onLogin: vi.fn(), onLogout: vi.fn() }, dashboard: dashboardState(), routePath: '/dashboard/memories?project=core-api#results' })

    expect(container.querySelector('section h2')?.textContent).toBe('Memories')
    expect(container.querySelector('[data-coming-soon]')).toBeNull()
    expect(container.querySelector('[data-dashboard-primitive="main"]')?.textContent).toContain('Default search: "dashboard"')
  })

  it('renders the Knowledge Browser discovery route instead of ComingSoon', () => {
    const container = document.createElement('main')

    renderApp({ container, state: { status: 'authenticated', token: 'jwt-token', user: adminUser }, actions: { onLogin: vi.fn(), onLogout: vi.fn() }, dashboard: dashboardState(), routePath: '/dashboard/knowledgeBrowser?query=auth&limit=1' })

    expect(container.querySelector('[data-coming-soon]')).toBeNull()
    expect(container.querySelector('section h2')?.textContent).toBe('Knowledge Browser')
    expect(container.querySelector('[role="note"]')?.textContent).toContain('Live Hive API data')
    expect(container.querySelector('input[name="query"]')).toBeNull()
    expect(container.querySelector('article[role="listitem"]')?.textContent).toContain('Gateway owns the auth boundary')
  })

  it('renders the Global Search discovery route with live memory data instead of fixture highlights', () => {
    const container = document.createElement('main')

    renderApp({ container, state: { status: 'authenticated', token: 'jwt-token', user: adminUser }, actions: { onLogin: vi.fn(), onLogout: vi.fn() }, dashboard: dashboardState(), routePath: '/dashboard/globalSearch?query=auth&limit=1' })

    expect(container.querySelector('[data-coming-soon]')).toBeNull()
    expect(container.querySelector('section h2')?.textContent).toBe('Global Search')
    expect(container.querySelector('[role="note"]')?.textContent).toContain('Live Hive API data')
    expect(container.querySelector('mark')).toBeNull()
  })

  it('loads Knowledge Browser from the live memories API using URL filters', async () => {
    const container = document.createElement('main')
    document.body.append(container)
    const session = fakeSessionStore({ status: 'authenticated', token: 'jwt-token', user: adminUser })
    const api = fakeApi({ memories: [Promise.resolve(memoryListResponse([memory({ title: 'Live browse memory' })], { total: 1, limit: 5, offset: 10 }))] })
    const originalPath = `${window.location.pathname}${window.location.search}${window.location.hash}`
    history.pushState(null, '', '/dashboard/knowledgeBrowser?project=jarvis-dev&category=decision&from=2026-06-01&until=2026-06-30&limit=5&offset=10')

    const cleanup = startDashboardApp(container, { api, session })
    try {
      await Promise.resolve()
      expect(container.textContent).toContain('Loading live memories')
      await flushDashboard()

      expect(api.memories).toHaveBeenCalledWith('jwt-token', { project: 'jarvis-dev', category: 'decision', from: '2026-06-01', until: '2026-06-30', limit: 5, offset: 10 })
      expect(container.textContent).toContain('Live browse memory')
      expect(container.textContent).not.toContain('Fixture-backed discovery data')
    } finally {
      cleanup()
      history.pushState(null, '', originalPath)
      container.remove()
    }
  })

  it('renders explicit Knowledge Browser API failures and empty responses without fixture fallback', async () => {
    for (const [response, assertion] of [
      [() => Promise.reject(new Error('browse API unavailable')), (container: HTMLElement) => expect(container.querySelector('[role="alert"]')?.textContent).toContain('browse API unavailable')],
      [Promise.resolve(memoryListResponse([], { total: 0 })), (container: HTMLElement) => expect(container.querySelector('[role="status"]')?.textContent).toBe('No live memories match the current filters.')]
    ] as const) {
      const container = document.createElement('main')
      document.body.append(container)
      const session = fakeSessionStore({ status: 'authenticated', token: 'jwt-token', user: adminUser })
      const api = fakeApi({ memories: [response] })
      const originalPath = `${window.location.pathname}${window.location.search}${window.location.hash}`
      history.pushState(null, '', '/dashboard/knowledgeBrowser?limit=5')

      const cleanup = startDashboardApp(container, { api, session })
      try {
        await flushDashboard()

        assertion(container)
        const mainText = container.querySelector('[data-dashboard-primitive="main"]')?.textContent ?? ''
        expect(mainText).not.toContain('Fixture-backed discovery data')
        expect(mainText).not.toContain('Gateway owns the auth boundary')
      } finally {
        cleanup()
        history.pushState(null, '', originalPath)
        container.remove()
      }
    }
  })

  it('reloads Global Search when URL filters change and links results to memory detail routes', async () => {
    const container = document.createElement('main')
    document.body.append(container)
    const session = fakeSessionStore({ status: 'authenticated', token: 'jwt-token', user: adminUser })
    const api = fakeApi({ search: [
      Promise.resolve(memorySearchResponse([memory({ id: 'mem-1', title: 'First query memory' })], { query: 'first' })),
      Promise.resolve(memorySearchResponse([memory({ id: 'mem-2', title: 'Second query memory' })], { query: 'second' }))
    ] })
    const originalPath = `${window.location.pathname}${window.location.search}${window.location.hash}`
    history.pushState(null, '', '/dashboard/globalSearch?query=first&project=jarvis-dev&limit=5&offset=0')

    const cleanup = startDashboardApp(container, { api, session })
    try {
      await flushDashboard()
      expect(api.searchMemories).toHaveBeenNthCalledWith(1, 'jwt-token', { query: 'first', project: 'jarvis-dev', limit: 5, offset: 0 })
      expect(container.querySelector('a[href="/dashboard/memories/mem-1"]')?.textContent).toBe('Open memory')

      container.querySelector<HTMLInputElement>('input[name="query"]')!.value = 'second'
      container.querySelector('form')!.dispatchEvent(new SubmitEvent('submit', { bubbles: true, cancelable: true }))
      await flushDashboard()

      expect(window.location.pathname + window.location.search).toBe('/dashboard/globalSearch?query=second&project=jarvis-dev&limit=5')
      expect(api.searchMemories).toHaveBeenNthCalledWith(2, 'jwt-token', { query: 'second', project: 'jarvis-dev', limit: 5 })
      expect(container.textContent).toContain('Second query memory')
      expect(container.querySelector('mark')).toBeNull()
    } finally {
      cleanup()
      history.pushState(null, '', originalPath)
      container.remove()
    }
  })

  it('shows a prompt and does not call Global Search when no query exists', async () => {
    const container = document.createElement('main')
    document.body.append(container)
    const session = fakeSessionStore({ status: 'authenticated', token: 'jwt-token', user: adminUser })
    const api = fakeApi()
    const originalPath = `${window.location.pathname}${window.location.search}${window.location.hash}`
    history.pushState(null, '', '/dashboard/globalSearch?limit=5')

    const cleanup = startDashboardApp(container, { api, session })
    try {
      await flushDashboard()

      expect(api.searchMemories).not.toHaveBeenCalled()
      expect(container.querySelector<HTMLInputElement>('input[name="query"]')?.value).toBe('')
      expect(container.querySelector('[role="status"]')?.textContent).toBe('Enter a search query to find live memories.')
      expect(container.querySelector('[data-dashboard-primitive="main"]')?.textContent).not.toContain('Gateway owns the auth boundary')
    } finally {
      cleanup()
      history.pushState(null, '', originalPath)
      container.remove()
    }
  })

  it('omits unsupported author and tag filters from live Global Search API params', async () => {
    const container = document.createElement('main')
    document.body.append(container)
    const session = fakeSessionStore({ status: 'authenticated', token: 'jwt-token', user: adminUser })
    const api = fakeApi({ search: [Promise.resolve(memorySearchResponse([memory({ title: 'Live search memory' })], { query: 'auth' }))] })
    const originalPath = `${window.location.pathname}${window.location.search}${window.location.hash}`
    history.pushState(null, '', '/dashboard/globalSearch?query=auth&project=jarvis-dev&author=admin-1&tag=security&limit=5')

    const cleanup = startDashboardApp(container, { api, session })
    try {
      await flushDashboard()

      expect(api.searchMemories).toHaveBeenCalledWith('jwt-token', { query: 'auth', project: 'jarvis-dev', limit: 5 })
      expect(container.querySelector('input[name="author"]')).toBeNull()
      expect(container.querySelector('input[name="tag"]')).toBeNull()
    } finally {
      cleanup()
      history.pushState(null, '', originalPath)
      container.remove()
    }
  })

  it('keeps stale Global Search responses from overwriting the current query results', async () => {
    const container = document.createElement('main')
    document.body.append(container)
    const session = fakeSessionStore({ status: 'authenticated', token: 'jwt-token', user: adminUser })
    const firstSearch = deferred<MemorySearch>()
    const api = fakeApi({ search: [
      firstSearch.promise,
      Promise.resolve(memorySearchResponse([memory({ id: 'mem-2', title: 'Second query memory' })], { query: 'second' }))
    ] })
    const originalPath = `${window.location.pathname}${window.location.search}${window.location.hash}`
    history.pushState(null, '', '/dashboard/globalSearch?query=first&project=jarvis-dev&limit=5')

    const cleanup = startDashboardApp(container, { api, session })
    try {
      await flushDashboard()
      expect(api.searchMemories).toHaveBeenNthCalledWith(1, 'jwt-token', { query: 'first', project: 'jarvis-dev', limit: 5 })

      history.pushState(null, '', '/dashboard/globalSearch?query=second&project=jarvis-dev&limit=5')
      window.dispatchEvent(new PopStateEvent('popstate'))
      await flushDashboard()

      expect(api.searchMemories).toHaveBeenNthCalledWith(2, 'jwt-token', { query: 'second', project: 'jarvis-dev', limit: 5 })
      expect(container.textContent).toContain('Second query memory')

      firstSearch.resolve(memorySearchResponse([memory({ id: 'mem-1', title: 'First query memory' })], { query: 'first' }))
      await flushDashboard()

      expect(window.location.pathname + window.location.search).toBe('/dashboard/globalSearch?query=second&project=jarvis-dev&limit=5')
      expect(container.textContent).toContain('Second query memory')
      expect(container.textContent).not.toContain('First query memory')
    } finally {
      cleanup()
      history.pushState(null, '', originalPath)
      container.remove()
    }
  })

  it('keeps stale Knowledge Browser responses from overwriting the current filters', async () => {
    const container = document.createElement('main')
    document.body.append(container)
    const session = fakeSessionStore({ status: 'authenticated', token: 'jwt-token', user: adminUser })
    const firstBrowse = deferred<MemoryList>()
    const api = fakeApi({ memories: [
      firstBrowse.promise,
      Promise.resolve(memoryListResponse([memory({ id: 'mem-2', title: 'Second filter memory' })], { total: 1, limit: 5 }))
    ] })
    const originalPath = `${window.location.pathname}${window.location.search}${window.location.hash}`
    history.pushState(null, '', '/dashboard/knowledgeBrowser?project=jarvis-dev&category=decision&limit=5')

    const cleanup = startDashboardApp(container, { api, session })
    try {
      await flushDashboard()
      expect(api.memories).toHaveBeenNthCalledWith(1, 'jwt-token', { project: 'jarvis-dev', category: 'decision', limit: 5 })

      history.pushState(null, '', '/dashboard/knowledgeBrowser?project=jarvis-dev&category=architecture&limit=5')
      window.dispatchEvent(new PopStateEvent('popstate'))
      await flushDashboard()

      expect(api.memories).toHaveBeenNthCalledWith(2, 'jwt-token', { project: 'jarvis-dev', category: 'architecture', limit: 5 })
      expect(container.textContent).toContain('Second filter memory')

      firstBrowse.resolve(memoryListResponse([memory({ id: 'mem-1', title: 'First filter memory' })], { total: 1, limit: 5 }))
      await flushDashboard()

      expect(window.location.pathname + window.location.search).toBe('/dashboard/knowledgeBrowser?project=jarvis-dev&category=architecture&limit=5')
      expect(container.textContent).toContain('Second filter memory')
      expect(container.textContent).not.toContain('First filter memory')
    } finally {
      cleanup()
      history.pushState(null, '', originalPath)
      container.remove()
    }
  })

  it('shows a controlled unavailable state for memory detail deep links', () => {
    const container = document.createElement('main')

    renderApp({ container, state: { status: 'authenticated', token: 'jwt-token', user: adminUser }, actions: { onLogin: vi.fn(), onLogout: vi.fn() }, dashboard: dashboardState(), routePath: '/dashboard/memories/gateway-auth-boundary' })

    expect(container.querySelector('section h2')?.textContent).toBe('Memories')
    expect(container.querySelector('[data-coming-soon]')).toBeNull()
    expect(container.querySelector('[role="status"]')?.textContent).toBe('Memory detail is unavailable in this fixture-backed dashboard slice.')
  })

  it('shows memory detail unavailable state before API-backed memories data is ready', () => {
    for (const dashboard of [
      { status: 'loading' as const },
      { status: 'ready' as const, data: { memories: { status: 'error' as const, message: 'memories unavailable' } } }
    ]) {
      const container = document.createElement('main')

      renderApp({ container, state: { status: 'authenticated', token: 'jwt-token', user: adminUser }, actions: { onLogin: vi.fn(), onLogout: vi.fn() }, dashboard, routePath: '/dashboard/memories/gateway-auth-boundary' })

      expect(container.querySelector('section h2')?.textContent).toBe('Memories')
      expect(container.querySelector('[role="status"]')?.textContent).toBe('Memory detail is unavailable in this fixture-backed dashboard slice.')
      expect(container.textContent).not.toContain('Loading memories…')
      expect(container.textContent).not.toContain('memories unavailable')
    }
  })

  it('shows the controlled unavailable state for malformed memory detail URLs', () => {
    const container = document.createElement('main')

    expect(() => renderApp({ container, state: { status: 'authenticated', token: 'jwt-token', user: adminUser }, actions: { onLogin: vi.fn(), onLogout: vi.fn() }, dashboard: dashboardState(), routePath: '/dashboard/memories/%E0%A4%A' })).not.toThrow()

    expect(container.querySelector('section h2')?.textContent).toBe('Memories')
    expect(container.querySelector('[data-coming-soon]')).toBeNull()
    expect(container.querySelector('[role="status"]')?.textContent).toBe('Memory detail is unavailable in this fixture-backed dashboard slice.')
  })

  it('keeps memory detail IDs from colliding with legacy route aliases', () => {
    for (const routePath of ['/dashboard/memories/users', '/dashboard/memories/audit-sync']) {
      const container = document.createElement('main')

      renderApp({ container, state: { status: 'authenticated', token: 'jwt-token', user: adminUser }, actions: { onLogin: vi.fn(), onLogout: vi.fn() }, dashboard: dashboardState(), routePath })

      expect(container.querySelector('section h2')?.textContent).toBe('Memories')
      expect(container.querySelector('[role="status"]')?.textContent).toBe('Memory detail is unavailable in this fixture-backed dashboard slice.')
      expect(container.textContent).not.toContain('Users')
      expect(container.textContent).not.toContain('Audit Sync')
    }
  })

  it('does not render stale dashboard data after logout while a dashboard load is pending', async () => {
    const container = document.createElement('main')
    document.body.append(container)
    const health = deferred<Awaited<ReturnType<ApiClient['health']>>>()
    const session = fakeSessionStore({ status: 'authenticated', token: 'first-token', user: adminUser })

    const cleanup = startDashboardApp(container, { api: fakeApi({ health: health.promise }), session })
    try {
      await Promise.resolve()

      container.querySelector<HTMLButtonElement>('button')?.click()
      expect(container.querySelector('h1')?.textContent).toBe('Sign in to Hive API')

      health.resolve({ status: 'ok', db: 'connected', version: '1.0.0' })
      await Promise.resolve()
      await Promise.resolve()
      await Promise.resolve()

      expect(container.querySelector('h1')?.textContent).toBe('Sign in to Hive API')
      expect(container.textContent).not.toContain('API status ok')
    } finally {
      cleanup()
      container.remove()
    }
  })

  it('does not mutate the DOM when bootstrap resolves after cleanup', async () => {
    const container = document.createElement('main')
    container.innerHTML = '<p>preserved after cleanup</p>'
    const bootstrap = deferred<ReturnType<SessionStore['getState']>>()
    const session = fakeSessionStore({ status: 'authenticated', token: 'jwt-token', user: adminUser })
    vi.mocked(session.bootstrap).mockReturnValue(bootstrap.promise)

    const cleanup = startDashboardApp(container, { api: fakeApi(), session })
    cleanup()
    bootstrap.resolve({ status: 'authenticated', token: 'jwt-token', user: adminUser })
    await flushDashboard()

    expect(container.innerHTML).toBe('<p>preserved after cleanup</p>')
    expect(container.textContent).not.toContain('Hive API Dashboard')
  })

  it('does not mutate the DOM when dashboard loading resolves after cleanup', async () => {
    const container = document.createElement('main')
    const health = deferred<Awaited<ReturnType<ApiClient['health']>>>()
    const session = fakeSessionStore({ status: 'authenticated', token: 'jwt-token', user: adminUser })

    const cleanup = startDashboardApp(container, { api: fakeApi({ health: health.promise }), session })
    try {
      await Promise.resolve()
      expect(container.querySelector('h1')?.textContent).toBe('Hive API Dashboard')
      expect(container.textContent).not.toContain('API status ok')

      const renderedBeforeCleanup = container.innerHTML
      cleanup()
      health.resolve({ status: 'ok', db: 'connected', version: '1.0.0' })
      await flushDashboard()

      expect(container.innerHTML).toBe(renderedBeforeCleanup)
      expect(container.textContent).not.toContain('API status ok')
    } finally {
      cleanup()
    }
  })

  it('renders a recoverable login form when bootstrap rejects', async () => {
    const container = document.createElement('main')
    const session = fakeSessionStore({ status: 'anonymous' })
    vi.mocked(session.bootstrap).mockRejectedValue(new Error('storage unavailable'))

    const cleanup = startDashboardApp(container, { api: fakeApi(), session })
    try {
      await Promise.resolve()
      await Promise.resolve()

      expect(container.querySelector('h1')?.textContent).toBe('Sign in to Hive API')
      expect(container.querySelector('input[name="email"]')).not.toBeNull()
      expect(container.querySelector('[role="alert"]')?.textContent).toContain('Unable to restore your session')
    } finally {
      cleanup()
    }
  })

  it('keeps successful dashboard panels visible when one endpoint fails', async () => {
    const rejectedStats = Promise.reject(new Error('stats unavailable'))
    const dashboard = await loadDashboard(fakeApi({ stats: rejectedStats }), 'jwt-token')

    expect(dashboard.status).toBe('ready')
    if (dashboard.status !== 'ready') throw new Error('expected ready dashboard')
    expect(dashboard.data.overview).toEqual({ status: 'error', message: 'stats unavailable' })
    expect(dashboard.data.users.status).toBe('ready')
    expect(dashboard.data.memories.status).toBe('ready')
    expect(dashboard.data.audit.status).toBe('ready')
    expect(dashboard.data.projects).toEqual({ status: 'ready', data: projectsFixture })
  })

  it('loads the audit route from production sync attempt summaries instead of fixture or audit-log data', async () => {
    const api = fakeApi()

    const dashboard = await loadDashboard(api, 'jwt-token')

    expect(dashboard.status).toBe('ready')
    expect(api.syncAttemptSummary).toHaveBeenCalledWith('jwt-token')
    expect(api.auditLogs).not.toHaveBeenCalled()
    expect(dashboard.data.audit.status).toBe('ready')
    if (dashboard.data.audit.status !== 'ready') throw new Error('expected ready audit data')
    expect(dashboard.data.audit.data.windows.map((window) => window.window)).toEqual(['24h', '7d', '30d'])
  })

  it('surfaces unhealthy live health as an overview error instead of fixture-complemented daemon counts', async () => {
    for (const [health, message] of [
      [{ status: 'degraded', db: 'connected', version: '1.0.0' }, 'Hive API health is degraded: status degraded'],
      [{ status: 'ok', db: 'disconnected', version: '1.0.0' }, 'Hive API health is degraded: database disconnected']
    ] as const) {
      const dashboard = await loadDashboard(fakeApi({ health: Promise.resolve(health) }), 'jwt-token')

      expect(dashboard.status).toBe('ready')
      expect(dashboard.data.overview).toMatchObject({ status: 'error', message })
      expect(dashboard.data.users.status).toBe('ready')
      expect(dashboard.data.memories.status).toBe('ready')
      expect(dashboard.data.audit.status).toBe('ready')
      expect(dashboard.data.projects).toEqual({ status: 'ready', data: projectsFixture })
    }
  })

  it('maps overview KPI metrics from live admin stats', async () => {
    const dashboard = await loadDashboard(fakeApi({ stats: Promise.resolve(adminStats({ totalMemories: 1234, activeProjectCounts: [10, 0, 5] })) }), 'jwt-token')

    expect(dashboard.status).toBe('ready')
    if (dashboard.status !== 'ready' || dashboard.data.overview.status !== 'ready') throw new Error('expected ready overview')
    expect(dashboard.data.overview.data.totalMemories).toMatchObject({ value: 1234, displayValue: '1.2k' })
    expect(dashboard.data.overview.data.activeProjects).toMatchObject({ value: 2, displayValue: '2' })
    expect(dashboard.data.overview.data.totalMemories.sourceLabel).toBeUndefined()
    expect(dashboard.data.overview.data.activeProjects.sourceLabel).toBeUndefined()
    expect(dashboard.data.overview.data.openConflicts).toMatchObject({ value: 3 })
    expect(dashboard.data.overview.data.openConflicts.sourceLabel).toBeUndefined()
    expect(dashboard.data.overview.data.knowledgeGrowth.points).toEqual([{ label: 'Jun', value: 44 }])
    expect(dashboard.data.overview.data.syncHealthByProjectSourceLabel).toBeUndefined()
  })

  it('loads overview health, admin totals, overview stats, and overview growth together', async () => {
    const api = fakeApi({
      stats: Promise.resolve(adminStats({ totalMemories: 4321, activeProjectCounts: [7, 0, 2] })),
      overviewStats: Promise.resolve(overviewStats({ openConflicts: 6, liveActivityCount: 8, newestSyncId: 'sync-live' })),
      overviewGrowth: Promise.resolve({ knowledge_growth: [{ label: 'Jun', value: 99 }] })
    })

    const dashboard = await loadDashboard(api, 'jwt-token')

    expect(api.health).toHaveBeenCalledTimes(1)
    expect(api.adminStats).toHaveBeenCalledWith('jwt-token')
    expect(api.overviewStats).toHaveBeenCalledWith('jwt-token')
    expect(api.overviewGrowth).toHaveBeenCalledWith('jwt-token')
    expect(dashboard.status).toBe('ready')
    if (dashboard.data.overview.status !== 'ready') throw new Error('expected ready overview')
    expect(dashboard.data.overview.data.totalMemories).toMatchObject({ value: 4321, displayValue: '4.3k' })
    expect(dashboard.data.overview.data.activeProjects).toMatchObject({ value: 2, displayValue: '2' })
    expect(dashboard.data.overview.data.healthyDaemons).toMatchObject({ value: 1, totalValue: 2, displayValue: '1/2' })
    expect(dashboard.data.overview.data.openConflicts).toMatchObject({ value: 6 })
    expect(dashboard.data.overview.data.openConflicts.sourceLabel).toBeUndefined()
    expect(dashboard.data.overview.data.liveActivity).toEqual({ count: 8, newestSyncId: 'sync-live' })
    expect(dashboard.data.overview.data.mostActiveProjects).toEqual([{ label: 'jarvis-dev', value: 11 }])
    expect(dashboard.data.overview.data.knowledgeGrowth.points).toEqual([{ label: 'Jun', value: 99 }])
  })

  it('renders route-loaded overview data without visible fixture complements', async () => {
    const container = document.createElement('main')
    document.body.append(container)
    const session = fakeSessionStore({ status: 'authenticated', token: 'jwt-token', user: adminUser })
    const api = fakeApi({ overviewStats: Promise.resolve(overviewStats({ liveActivityCount: 2, newestSyncId: 'sync-live' })) })
    const originalPath = `${window.location.pathname}${window.location.search}${window.location.hash}`
    history.pushState(null, '', '/dashboard')

    const cleanup = startDashboardApp(container, { api, session })
    try {
      await flushDashboard()

      expect(api.health).toHaveBeenCalledTimes(1)
      expect(api.adminStats).toHaveBeenCalledWith('jwt-token')
      expect(api.overviewStats).toHaveBeenCalledWith('jwt-token')
      expect(api.overviewGrowth).toHaveBeenCalledWith('jwt-token')
      const overview = container.querySelector<HTMLElement>('[data-dashboard-primitive="main"] section[role="region"]')
      expect(overview?.textContent).toContain('Newest sync: sync-live')
      expect(overview?.textContent).toContain('jarvis-dev')
      expect(overview?.textContent).not.toContain('Demo fixture data')
      expect(overview?.textContent).not.toContain('Gateway owns the auth boundary')
    } finally {
      cleanup()
      history.pushState(null, '', originalPath)
      container.remove()
    }
  })
})

describe('bell and search slot integration', () => {
  it('notification bell is visible in authenticated shell', () => {
    const container = document.createElement('main')

    renderApp({ container, state: { status: 'authenticated', token: 'jwt-token', user: adminUser }, actions: { onLogin: vi.fn(), onLogout: vi.fn() } })

    const bell = container.querySelector('[aria-label="Notifications"]')
    expect(bell).not.toBeNull()
  })

  it('bell shows unread badge when summary.unread > 0', () => {
    const container = document.createElement('main')

    renderApp({ container, state: { status: 'authenticated', token: 'jwt-token', user: adminUser }, actions: { onLogin: vi.fn(), onLogout: vi.fn() }, dashboard: { status: 'loading' }, routePath: '/dashboard', drawerState: { drawerOpen: false, readIds: new Set() } })

    // dashboardNotificationSummary.unread is 3
    const badge = container.querySelector('[data-bell-badge]')
    expect(badge).not.toBeNull()
    expect(badge?.textContent).toContain(String(dashboardNotificationSummary.unread))
  })

  it('bell badge is hidden when all notifications are read', () => {
    const container = document.createElement('main')
    const allReadIds = new Set(Array.from({ length: 7 }, (_, i) => `id-${i}`))

    renderApp({ container, state: { status: 'authenticated', token: 'jwt-token', user: adminUser }, actions: { onLogin: vi.fn(), onLogout: vi.fn() }, dashboard: { status: 'loading' }, routePath: '/dashboard', drawerState: { drawerOpen: false, readIds: allReadIds, summaryUnread: 0 } })

    const badge = container.querySelector('[data-bell-badge]')
    expect(badge).toBeNull()
  })

  it('search slot is visible in authenticated shell', () => {
    const container = document.createElement('main')

    renderApp({ container, state: { status: 'authenticated', token: 'jwt-token', user: adminUser }, actions: { onLogin: vi.fn(), onLogout: vi.fn() } })

    const searchSlot = container.querySelector('.dashboard-header__search')
    expect(searchSlot).not.toBeNull()
  })

  it('drawer element has [data-open] attribute when drawerOpen is true', () => {
    const container = document.createElement('main')

    renderApp({ container, state: { status: 'authenticated', token: 'jwt-token', user: adminUser }, actions: { onLogin: vi.fn(), onLogout: vi.fn() }, dashboard: { status: 'loading' }, routePath: '/dashboard', drawerState: { drawerOpen: true, readIds: new Set() } })

    const drawer = container.querySelector('[data-dashboard-primitive="drawer"]')
    expect(drawer).not.toBeNull()
    expect(drawer?.hasAttribute('data-open')).toBe(true)
  })

  it('makes the app background inert while the modal notification drawer is open', () => {
    const container = document.createElement('main')

    renderApp({ container, state: { status: 'authenticated', token: 'jwt-token', user: adminUser }, actions: { onLogin: vi.fn(), onLogout: vi.fn() }, dashboard: { status: 'loading' }, routePath: '/dashboard', drawerState: { drawerOpen: true, readIds: new Set() } })

    expect(container.querySelector('[data-dashboard-primitive="drawer"]')?.getAttribute('aria-modal')).toBe('true')
    expect(container.querySelector('[data-dashboard-primitive="sidebar"]')?.hasAttribute('inert')).toBe(true)
    expect(container.querySelector('[role="banner"]')?.hasAttribute('inert')).toBe(true)
    expect(container.querySelector('[data-dashboard-primitive="main"]')?.hasAttribute('inert')).toBe(true)
  })

  it('keeps the app background interactive when the notification drawer is closed', () => {
    const container = document.createElement('main')

    renderApp({ container, state: { status: 'authenticated', token: 'jwt-token', user: adminUser }, actions: { onLogin: vi.fn(), onLogout: vi.fn() }, dashboard: { status: 'loading' }, routePath: '/dashboard', drawerState: { drawerOpen: false, readIds: new Set() } })

    expect(container.querySelector('[data-dashboard-primitive="drawer"]')?.getAttribute('aria-modal')).toBeNull()
    expect(container.querySelector('[data-dashboard-primitive="sidebar"]')?.hasAttribute('inert')).toBe(false)
    expect(container.querySelector('[role="banner"]')?.hasAttribute('inert')).toBe(false)
    expect(container.querySelector('[data-dashboard-primitive="main"]')?.hasAttribute('inert')).toBe(false)
  })

  it('W1 — bell click fires onToggleDrawer action', () => {
    const container = document.createElement('main')
    const onToggleDrawer = vi.fn()

    renderApp({ container, state: { status: 'authenticated', token: 'jwt-token', user: adminUser }, actions: { onLogin: vi.fn(), onLogout: vi.fn(), onToggleDrawer } })

    const bell = container.querySelector<HTMLButtonElement>('[aria-label="Notifications"]')
    expect(bell).not.toBeNull()
    bell!.dispatchEvent(new MouseEvent('click', { bubbles: true }))

    expect(onToggleDrawer).toHaveBeenCalledTimes(1)
  })

  it('W2 — search slot click navigates to /dashboard/globalSearch', () => {
    const container = document.createElement('main')
    const onNavigate = vi.fn()

    renderApp({ container, state: { status: 'authenticated', token: 'jwt-token', user: adminUser }, actions: { onLogin: vi.fn(), onLogout: vi.fn(), onNavigate } })

    const searchSlot = container.querySelector<HTMLElement>('.dashboard-header__search')
    expect(searchSlot).not.toBeNull()
    searchSlot!.dispatchEvent(new MouseEvent('click', { bubbles: true }))

    expect(onNavigate).toHaveBeenCalledWith('/dashboard/globalSearch')
  })

  it('header search preserves discovery query filters when navigating to Global Search', () => {
    const container = document.createElement('main')
    const onNavigate = vi.fn()

    renderApp({
      container,
      state: { status: 'authenticated', token: 'jwt-token', user: adminUser },
      actions: { onLogin: vi.fn(), onLogout: vi.fn(), onNavigate },
      routePath: '/dashboard/knowledgeBrowser?query=auth&category=architecture#results'
    })

    const searchSlot = container.querySelector<HTMLElement>('.dashboard-header__search')
    expect(searchSlot).not.toBeNull()
    searchSlot!.dispatchEvent(new MouseEvent('click', { bubbles: true }))

    expect(onNavigate).toHaveBeenCalledWith('/dashboard/globalSearch?query=auth&category=architecture')
  })

  it('header search opens the Global Search route in the running app', async () => {
    const container = document.createElement('main')
    document.body.append(container)
    const session = fakeSessionStore({ status: 'authenticated', token: 'jwt-token', user: adminUser })
    const originalPath = `${window.location.pathname}${window.location.search}${window.location.hash}`
    history.pushState(null, '', '/dashboard')

    const cleanup = startDashboardApp(container, { api: fakeApi(), session })
    try {
      await flushDashboard()
      container.querySelector<HTMLButtonElement>('.dashboard-header__search')!.click()
      await flushDashboard()

      expect(window.location.pathname).toBe('/dashboard/globalSearch')
      expect(container.querySelector('section h2')?.textContent).toBe('Global Search')
      expect(container.querySelector('[data-coming-soon]')).toBeNull()
    } finally {
      cleanup()
      history.pushState(null, '', originalPath)
      container.remove()
    }
  })

  it('initializes discovery filters from the current URL in the running app', async () => {
    const container = document.createElement('main')
    document.body.append(container)
    const session = fakeSessionStore({ status: 'authenticated', token: 'jwt-token', user: adminUser })
    const originalPath = `${window.location.pathname}${window.location.search}${window.location.hash}`
    history.pushState(null, '', '/dashboard/knowledgeBrowser?query=auth&limit=1')

    const cleanup = startDashboardApp(container, { api: fakeApi(), session })
    try {
      await flushDashboard()

      expect(container.querySelector('section h2')?.textContent).toBe('Knowledge Browser')
      expect(container.querySelector('input[name="query"]')).toBeNull()
      expect(container.querySelectorAll('article[role="listitem"]')).toHaveLength(1)
    } finally {
      cleanup()
      history.pushState(null, '', originalPath)
      container.remove()
    }
  })

  it('mark all read button fires onMarkAllRead callback', () => {
    const container = document.createElement('main')
    const onMarkAllRead = vi.fn()

    // Render with drawer open and 3 unread notifications
    renderApp({
      container,
      state: { status: 'authenticated', token: 'jwt-token', user: adminUser },
      actions: { onLogin: vi.fn(), onLogout: vi.fn(), onMarkAllRead },
      dashboard: { status: 'loading' },
      routePath: '/dashboard',
      drawerState: { drawerOpen: true, readIds: new Set(), summaryUnread: 3 }
    })

    // Drawer must be open and badge visible before click
    expect(container.querySelector('[data-dashboard-primitive="drawer"]')?.hasAttribute('data-open')).toBe(true)
    expect(container.querySelector('[data-bell-badge]')).not.toBeNull()

    const markAllReadBtn = container.querySelector<HTMLButtonElement>('[data-mark-all-read]')
    expect(markAllReadBtn).not.toBeNull()
    markAllReadBtn!.dispatchEvent(new MouseEvent('click', { bubbles: true }))

    expect(onMarkAllRead).toHaveBeenCalledTimes(1)
  })

  it('opens and closes the notification drawer through startDashboardApp state', async () => {
    const container = document.createElement('main')
    document.body.append(container)
    const session = fakeSessionStore({ status: 'authenticated', token: 'jwt-token', user: adminUser })

    const cleanup = startDashboardApp(container, { api: fakeApi(), session })
    try {
      await flushDashboard()

      const bell = container.querySelector<HTMLButtonElement>('[data-bell]')
      expect(bell).not.toBeNull()
      expect(drawer(container)?.hasAttribute('hidden')).toBe(true)
      expect(drawer(container)?.getAttribute('role')).toBeNull()

      bell!.click()

      expect(drawer(container)?.hasAttribute('data-open')).toBe(true)
      expect(drawer(container)?.hasAttribute('hidden')).toBe(false)
      expect(drawer(container)?.getAttribute('role')).toBe('dialog')
      expect(drawer(container)?.getAttribute('aria-modal')).toBe('true')
      expect(container.querySelector('[data-dashboard-primitive="sidebar"]')?.hasAttribute('inert')).toBe(true)
      expect(container.querySelector('[role="banner"]')?.hasAttribute('inert')).toBe(true)
      expect(container.querySelector('[data-dashboard-primitive="main"]')?.hasAttribute('inert')).toBe(true)
      expect(document.activeElement).toBe(container.querySelector('[data-drawer-close]'))

      container.querySelector<HTMLButtonElement>('[data-drawer-close]')!.click()

      expect(drawer(container)?.hasAttribute('data-open')).toBe(false)
      expect(drawer(container)?.hasAttribute('hidden')).toBe(true)
      expect(drawer(container)?.hasAttribute('inert')).toBe(true)
      expect(drawer(container)?.getAttribute('aria-hidden')).toBe('true')
      expect(drawer(container)?.getAttribute('role')).toBeNull()
      expect(drawer(container)?.getAttribute('aria-modal')).toBeNull()
      expect(container.querySelector('[data-dashboard-primitive="sidebar"]')?.hasAttribute('inert')).toBe(false)
      expect(container.querySelector('[role="banner"]')?.hasAttribute('inert')).toBe(false)
      expect(container.querySelector('[data-dashboard-primitive="main"]')?.hasAttribute('inert')).toBe(false)
      expect(document.activeElement).toBe(container.querySelector('[data-bell]'))
    } finally {
      cleanup()
      container.remove()
    }
  })

  it('closes the open notification drawer with Escape and restores bell focus', async () => {
    const container = document.createElement('main')
    document.body.append(container)
    const session = fakeSessionStore({ status: 'authenticated', token: 'jwt-token', user: adminUser })

    const cleanup = startDashboardApp(container, { api: fakeApi(), session })
    try {
      await flushDashboard()

      const bell = container.querySelector<HTMLButtonElement>('[data-bell]')
      expect(bell).not.toBeNull()
      bell!.click()
      expect(drawer(container)?.hasAttribute('data-open')).toBe(true)
      expect(document.activeElement).toBe(container.querySelector('[data-drawer-close]'))

      const event = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })
      container.querySelector<HTMLButtonElement>('[data-drawer-close]')!.dispatchEvent(event)

      expect(event.defaultPrevented).toBe(true)
      expect(drawer(container)?.hasAttribute('data-open')).toBe(false)
      expect(drawer(container)?.hasAttribute('hidden')).toBe(true)
      expect(document.activeElement).toBe(container.querySelector('[data-bell]'))
    } finally {
      cleanup()
      container.remove()
    }
  })

  it('does not respond to popstate events after cleanup', async () => {
    const container = document.createElement('main')
    document.body.append(container)
    const session = fakeSessionStore({ status: 'authenticated', token: 'jwt-token', user: adminUser })
    const originalPath = `${window.location.pathname}${window.location.search}${window.location.hash}`
    history.pushState(null, '', '/dashboard')

    const cleanup = startDashboardApp(container, { api: fakeApi(), session })
    try {
      await flushDashboard()
      expect(container.querySelector('[data-coming-soon]')).toBeNull()

      const renderedBeforeCleanup = container.innerHTML
      cleanup()
      history.pushState(null, '', '/dashboard/activityFeed')
      window.dispatchEvent(new PopStateEvent('popstate'))

      expect(container.innerHTML).toBe(renderedBeforeCleanup)
      expect(container.querySelector('[data-coming-soon]')).toBeNull()
    } finally {
      cleanup()
      history.pushState(null, '', originalPath)
      container.remove()
    }
  })

  it('marks all notifications read through startDashboardApp and restores bell focus', async () => {
    const container = document.createElement('main')
    document.body.append(container)
    const session = fakeSessionStore({ status: 'authenticated', token: 'jwt-token', user: adminUser })

    const cleanup = startDashboardApp(container, { api: fakeApi(), session })
    try {
      await flushDashboard()

      expect(container.querySelector('[data-bell-badge]')?.textContent).toBe(String(dashboardNotificationSummary.unread))
      container.querySelector<HTMLButtonElement>('[data-bell]')!.click()
      expect(drawer(container)?.hasAttribute('data-open')).toBe(true)

      container.querySelector<HTMLButtonElement>('[data-mark-all-read]')!.click()

      expect(container.querySelector('[data-bell-badge]')).toBeNull()
      expect(drawer(container)?.hasAttribute('data-open')).toBe(false)
      expect(drawer(container)?.hasAttribute('hidden')).toBe(true)
      expect(container.querySelectorAll('[data-unread-indicator]').length).toBe(0)
      expect(document.activeElement).toBe(container.querySelector('[data-bell]'))
    } finally {
      cleanup()
      container.remove()
    }
  })
})

function drawer(container: HTMLElement): HTMLElement | null {
  return container.querySelector('[data-dashboard-primitive="drawer"]')
}

async function flushDashboard(): Promise<void> {
  await Promise.resolve()
  await Promise.resolve()
  await Promise.resolve()
  await Promise.resolve()
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((done, fail) => {
    resolve = done
    reject = fail
  })
  return { promise, resolve, reject }
}

function fakeSessionStore(initial: ReturnType<SessionStore['getState']>): SessionStore {
  let state = initial
  return {
    getState: () => state,
    login: vi.fn(async () => state),
    bootstrap: vi.fn(async () => state),
    logout: vi.fn(() => {
      state = { status: 'anonymous' }
      return state
    })
  }
}

function fakeApi(overrides: {
  health?: Promise<Awaited<ReturnType<ApiClient['health']>>>
  stats?: Promise<Awaited<ReturnType<ApiClient['adminStats']>>>
  overviewStats?: Promise<OverviewStats>
  overviewGrowth?: Promise<OverviewGrowth>
  users?: Promise<Awaited<ReturnType<ApiClient['adminUsers']>>>[]
  setUserLevel?: Promise<Awaited<ReturnType<ApiClient['setUserLevel']>>>
  grantAdmin?: Promise<Awaited<ReturnType<ApiClient['grantAdmin']>>>
  deactivateUser?: Promise<Awaited<ReturnType<ApiClient['deactivateUser']>>>
  activity?: Array<Promise<ActivityFeedResponse> | (() => Promise<ActivityFeedResponse>)>
  memories?: Array<Promise<MemoryList> | (() => Promise<MemoryList>)>
  search?: Array<Promise<MemorySearch> | (() => Promise<MemorySearch>)>
} = {}): ApiClient {
  const userResponses = [...(overrides.users ?? [])]
  const activityResponses = [...(overrides.activity ?? [])]
  const memoryResponses = [...(overrides.memories ?? [])]
  const searchResponses = [...(overrides.search ?? [])]
  return {
    login: vi.fn(),
    currentUser: vi.fn(),
    health: vi.fn(() => overrides.health ?? Promise.resolve({ status: 'ok', db: 'connected', version: '1.0.0' })),
    adminStats: vi.fn(() => overrides.stats ?? Promise.resolve({ users: { total: 1, active: 1, by_level: { admin: 1 } }, memories: { total: 1, by_project: [], by_category: [], last_synced_at: null } })),
    overviewStats: vi.fn(() => overrides.overviewStats ?? Promise.resolve(overviewStats())),
    overviewGrowth: vi.fn(() => overrides.overviewGrowth ?? Promise.resolve({ knowledge_growth: [{ label: 'Jun', value: 44 }] })),
    adminUsers: vi.fn(() => userResponses.shift() ?? Promise.resolve({ users: [adminUser] })),
    setUserLevel: vi.fn(() => overrides.setUserLevel ?? Promise.resolve({ message: 'level updated' })),
    grantAdmin: vi.fn(() => overrides.grantAdmin ?? Promise.resolve({ message: 'admin granted' })),
    deactivateUser: vi.fn(() => overrides.deactivateUser ?? Promise.resolve({ message: 'user deactivated' })),
    memories: vi.fn(() => {
      const next = memoryResponses.shift()
      if (typeof next === 'function') return next()
      return next ?? Promise.resolve(memoryListResponse([memory({ title: 'Gateway owns the auth boundary, not services' })]))
    }),
    searchMemories: vi.fn(() => {
      const next = searchResponses.shift()
      if (typeof next === 'function') return next()
      return next ?? Promise.resolve(memorySearchResponse([memory({ title: 'Gateway owns the auth boundary, not services' })]))
    }),
    memory: vi.fn(async () => ({ id: 'mem-1', sync_id: 'sync-1', project: 'jarvis-dev', category: 'decision', title: 'Dashboard scope', content: 'No daemon controls', tags: [], files_affected: [], created_by: 'admin-1', created_at: '2026-06-06T20:00:00Z', updated_at: '2026-06-06T20:01:00Z', synced_at: '2026-06-06T20:02:00Z' })),
    auditLogs: vi.fn(async () => ({ audit_logs: [], total: 0, limit: 10, offset: 0 })),
    syncAttemptSummary: vi.fn(async () => syncAttemptSummaryFixture()),
    activity: vi.fn(() => {
      const next = activityResponses.shift()
      if (typeof next === 'function') return next()
      return next ?? Promise.resolve(activityResponse('event-1', 'Real backend activity'))
    })
  }
}

function memoryListResponse(memories: MemoryList['memories'], overrides: Partial<Omit<MemoryList, 'memories'>> = {}): MemoryList {
  return { memories, total: memories.length, limit: 10, offset: 0, ...overrides }
}

function memorySearchResponse(memories: MemorySearch['memories'], overrides: Partial<Omit<MemorySearch, 'memories'>> = {}): MemorySearch {
  return { memories, total: memories.length, query: 'dashboard', limit: 10, offset: 0, ...overrides }
}

function memory(overrides: Partial<MemoryList['memories'][number]> = {}): MemoryList['memories'][number] {
  return {
    id: 'mem-1',
    sync_id: 'sync-1',
    project: 'jarvis-dev',
    category: 'decision',
    title: 'Dashboard scope',
    content: 'No daemon controls',
    tags: [],
    files_affected: [],
    created_by: 'admin-1',
    created_at: '2026-06-06T20:00:00Z',
    updated_at: '2026-06-06T20:01:00Z',
    synced_at: '2026-06-06T20:02:00Z',
    ...overrides
  }
}

function syncAttemptSummaryFixture() {
  return {
    windows: [
      syncAttemptWindow('24h', 3, 2, 1, 0.3333),
      syncAttemptWindow('7d', 5, 4, 1, 0.2),
      syncAttemptWindow('30d', 8, 7, 1, 0.125)
    ]
  }
}

function syncAttemptWindow(window: '24h' | '7d' | '30d', total: number, successes: number, failures: number, failure_rate: number) {
  return {
    window,
    total,
    successes,
    failures,
    failure_rate,
    last_success_at: '2026-06-19T09:00:00Z',
    last_failure_at: '2026-06-19T08:00:00Z',
    by_developer: [{ key: 'ada@example.com', count: total }],
    by_project: [{ key: 'jarvis-dev', count: total }],
    by_client: [{ key: 'hive-daemon', count: total }],
    by_daemon: [{ key: 'daemon-1', count: total }],
    by_outcome: [{ key: 'success', count: successes }, { key: 'failure', count: failures }],
    by_error_code: [{ key: 'NETWORK_ERROR', count: failures }],
    top_errors: [{ key: 'NETWORK_ERROR', count: failures }]
  }
}

function adminStats(input: { totalMemories: number; activeProjectCounts: readonly number[] }): Awaited<ReturnType<ApiClient['adminStats']>> {
  return {
    users: { total: 1, active: 1, by_level: { admin: 1 } },
    memories: {
      total: input.totalMemories,
      by_project: input.activeProjectCounts.map((count, index) => ({ project: `project-${index + 1}`, count })),
      by_category: [],
      last_synced_at: null
    }
  }
}

function overviewStats(input: { openConflicts?: number; liveActivityCount?: number; newestSyncId?: string } = {}): OverviewStats {
  return {
    daemon_health: { healthy: 1, total: 2 },
    conflicts: { open: input.openConflicts ?? 3 },
    sync_health_by_project: [{ project: 'jarvis-dev', status: 'healthy', region: 'local', contributor_count: 2 }],
    live_activity: { count: input.liveActivityCount ?? 4, newest_sync_id: input.newestSyncId ?? 'sync-newest' },
    most_active_projects: [{ project: 'jarvis-dev', count: 11 }]
  }
}

function dashboardState() {
  return {
    status: 'ready' as const,
    data: {
      overview: { status: 'ready' as const, data: hiveOverviewFixture },
      users: { status: 'ready' as const, data: { users: [adminUser] } },
      memories: { status: 'ready' as const, data: { recent: { memories: [], total: 0, limit: 5, offset: 0 }, search: { memories: [], total: 0, query: 'dashboard', limit: 5, offset: 0 } } },
      audit: { status: 'ready' as const, data: syncAttemptSummaryFixture() },
      activity: { status: 'ready' as const, data: activityViewState() },
      projects: { status: 'ready' as const, data: projectsFixture },
      knowledgeBrowser: { status: 'ready' as const, data: memoryListToDiscoveryData(memoryListResponse([memory({ title: 'Gateway owns the auth boundary, not services' })], { limit: 1 })) },
      globalSearch: { status: 'ready' as const, data: memorySearchToDiscoveryData(memorySearchResponse([memory({ title: 'Gateway owns the auth boundary, not services' })], { limit: 1 })) }
    }
  }
}

function activityViewState() {
  return {
    screen: 'activityFeed' as const,
    groups: [{
      dateLabel: 'Today',
      entries: [{ id: 'event-1', title: 'Real backend activity', actorHandle: 'ada@example.com', projectId: 'jarvis-dev', category: 'decision' as const, timeLabel: '09:00', memorySyncId: 'sync-1' }]
    }],
    nextCursor: 'cursor-2'
  }
}

function activityResponse(id: string, title: string, nextCursor?: string): ActivityFeedResponse {
  return {
    entries: [{
      id,
      event_type: 'create',
      occurred_at: '2026-06-25T09:00:00Z',
      actor: 'ada@example.com',
      project: 'jarvis-dev',
      category: 'decision',
      title,
      summary: title,
      memory_sync_id: `sync-${id}`
    }],
    next_cursor: nextCursor ?? null
  }
}
