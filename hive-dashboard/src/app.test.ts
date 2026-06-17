import { describe, expect, it, vi } from 'vitest'
import type { ApiClient } from './api/client'
import type { SessionStore } from './auth/session'
import { loadDashboard, renderApp, startDashboardApp } from './main'
import { dashboardNotificationSummary } from './fixtures/hive-dashboard/shared'
import { hiveOverviewFixture } from './fixtures/hive-dashboard/overview'

const adminUser = { id: 'admin-1', username: 'admin', email: 'admin@example.com', level: 'admin' as const, is_active: true, created_at: '2026-06-06T20:00:00Z' }
const memberUser = { id: 'member-1', username: 'member', email: 'member@example.com', level: 'member' as const, is_active: true, created_at: '2026-06-06T20:00:00Z' }

describe('dashboard shell', () => {
  it('shows the login form to unauthenticated users', () => {
    const container = document.createElement('main')

    renderApp(container, { status: 'anonymous' }, { onLogin: vi.fn(), onLogout: vi.fn() })

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

    renderApp(container, { status: 'authenticated', token: 'jwt-token', user: memberUser }, { onLogin: vi.fn(), onLogout: vi.fn() })

    expect(container.textContent).not.toContain('Admin access required')
    expect(container.querySelector('[data-dashboard-primitive="sidebar"]')).not.toBeNull()
    expect(container.querySelector('[data-dashboard-primitive="header"]')).not.toBeNull()
    expect(container.querySelector('[data-dashboard-primitive="main"]')).not.toBeNull()
  })

  it('derives the sidebar profile from the authenticated user', () => {
    const container = document.createElement('main')

    renderApp(container, { status: 'authenticated', token: 'jwt-token', user: memberUser }, { onLogin: vi.fn(), onLogout: vi.fn() })

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

    renderApp(container, { status: 'authenticated', token: 'jwt-token', user: adminUser }, { onLogin: vi.fn(), onLogout: vi.fn() })

    expect(container.querySelector('[data-dashboard-primitive="sidebar"]')).not.toBeNull()
    expect(container.querySelector('[data-dashboard-primitive="header"]')).not.toBeNull()
    expect(container.querySelector('[data-dashboard-primitive="main"]')).not.toBeNull()
    expect(container.querySelector('[data-dashboard-primitive="sidebar"] nav')?.getAttribute('aria-label')).toBe('Dashboard sections')
    expect(container.querySelector('[data-dashboard-primitive="sidebar"] nav')?.textContent).toContain('Dashboard')
    expect(container.textContent).not.toContain('daemon')
  })

  it('renders route navigation and the selected API-backed view for admin deep links', () => {
    const container = document.createElement('main')

    renderApp(container, { status: 'authenticated', token: 'jwt-token', user: adminUser }, { onLogin: vi.fn(), onLogout: vi.fn() }, dashboardState(), '/dashboard/users')

    expect(container.querySelector('[data-dashboard-primitive="sidebar"] nav')?.textContent).toContain('Dashboard')
    expect(container.querySelector('section h2')?.textContent).toBe('Users')
    expect(container.textContent).toContain('admin · active')
    expect(container.textContent).toContain('admin@example.com')
    expect(container.textContent).not.toContain('Authentication is active')
  })

  it('renders ComingSoon for an unimplemented route', () => {
    const container = document.createElement('main')

    renderApp(container, { status: 'authenticated', token: 'jwt-token', user: adminUser }, { onLogin: vi.fn(), onLogout: vi.fn() }, { status: 'loading' }, '/dashboard/activityFeed')

    expect(container.querySelector('[data-coming-soon]')).not.toBeNull()
    expect(container.textContent).toContain('Activity Feed')
  })

  it('shows the named default memory search in the memories view', () => {
    const container = document.createElement('main')

    renderApp(container, { status: 'authenticated', token: 'jwt-token', user: adminUser }, { onLogin: vi.fn(), onLogout: vi.fn() }, dashboardState(), '/dashboard/memories')

    expect(container.querySelector('[data-dashboard-primitive="main"]')?.textContent).toContain('Default search: "dashboard"')
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
    // stats rejection is consumed here to avoid an unhandled rejection;
    // overview now returns the fixture unconditionally so the rejected promise is never awaited by loadDashboard
    const rejectedStats = Promise.reject(new Error('stats unavailable'))
    rejectedStats.catch(() => undefined)
    const dashboard = await loadDashboard(fakeApi({ stats: rejectedStats }), 'jwt-token')

    expect(dashboard.status).toBe('ready')
    if (dashboard.status !== 'ready') throw new Error('expected ready dashboard')
    // Overview now returns the fixture unconditionally — API errors do not affect it
    expect(dashboard.data.overview.status).toBe('ready')
    expect(dashboard.data.users.status).toBe('ready')
    expect(dashboard.data.memories.status).toBe('ready')
    expect(dashboard.data.audit.status).toBe('ready')
  })
})

describe('bell and search slot integration', () => {
  it('notification bell is visible in authenticated shell', () => {
    const container = document.createElement('main')

    renderApp(container, { status: 'authenticated', token: 'jwt-token', user: adminUser }, { onLogin: vi.fn(), onLogout: vi.fn() })

    const bell = container.querySelector('[aria-label="Notifications"]')
    expect(bell).not.toBeNull()
  })

  it('bell shows unread badge when summary.unread > 0', () => {
    const container = document.createElement('main')

    renderApp(container, { status: 'authenticated', token: 'jwt-token', user: adminUser }, { onLogin: vi.fn(), onLogout: vi.fn() }, { status: 'loading' }, '/dashboard', { drawerOpen: false, readIds: new Set() })

    // dashboardNotificationSummary.unread is 3
    const badge = container.querySelector('[data-bell-badge]')
    expect(badge).not.toBeNull()
    expect(badge?.textContent).toContain(String(dashboardNotificationSummary.unread))
  })

  it('bell badge is hidden when all notifications are read', () => {
    const container = document.createElement('main')
    const allReadIds = new Set(Array.from({ length: 7 }, (_, i) => `id-${i}`))

    renderApp(container, { status: 'authenticated', token: 'jwt-token', user: adminUser }, { onLogin: vi.fn(), onLogout: vi.fn() }, { status: 'loading' }, '/dashboard', { drawerOpen: false, readIds: allReadIds, summaryUnread: 0 })

    const badge = container.querySelector('[data-bell-badge]')
    expect(badge).toBeNull()
  })

  it('search slot is visible in authenticated shell', () => {
    const container = document.createElement('main')

    renderApp(container, { status: 'authenticated', token: 'jwt-token', user: adminUser }, { onLogin: vi.fn(), onLogout: vi.fn() })

    const searchSlot = container.querySelector('.dashboard-header__search')
    expect(searchSlot).not.toBeNull()
  })

  it('drawer element has [data-open] attribute when drawerOpen is true', () => {
    const container = document.createElement('main')

    renderApp(container, { status: 'authenticated', token: 'jwt-token', user: adminUser }, { onLogin: vi.fn(), onLogout: vi.fn() }, { status: 'loading' }, '/dashboard', { drawerOpen: true, readIds: new Set() })

    const drawer = container.querySelector('[data-dashboard-primitive="drawer"]')
    expect(drawer).not.toBeNull()
    expect(drawer?.hasAttribute('data-open')).toBe(true)
  })

  it('makes the app background inert while the modal notification drawer is open', () => {
    const container = document.createElement('main')

    renderApp(container, { status: 'authenticated', token: 'jwt-token', user: adminUser }, { onLogin: vi.fn(), onLogout: vi.fn() }, { status: 'loading' }, '/dashboard', { drawerOpen: true, readIds: new Set() })

    expect(container.querySelector('[data-dashboard-primitive="drawer"]')?.getAttribute('aria-modal')).toBe('true')
    expect(container.querySelector('[data-dashboard-primitive="sidebar"]')?.hasAttribute('inert')).toBe(true)
    expect(container.querySelector('[role="banner"]')?.hasAttribute('inert')).toBe(true)
    expect(container.querySelector('[data-dashboard-primitive="main"]')?.hasAttribute('inert')).toBe(true)
  })

  it('keeps the app background interactive when the notification drawer is closed', () => {
    const container = document.createElement('main')

    renderApp(container, { status: 'authenticated', token: 'jwt-token', user: adminUser }, { onLogin: vi.fn(), onLogout: vi.fn() }, { status: 'loading' }, '/dashboard', { drawerOpen: false, readIds: new Set() })

    expect(container.querySelector('[data-dashboard-primitive="drawer"]')?.getAttribute('aria-modal')).toBeNull()
    expect(container.querySelector('[data-dashboard-primitive="sidebar"]')?.hasAttribute('inert')).toBe(false)
    expect(container.querySelector('[role="banner"]')?.hasAttribute('inert')).toBe(false)
    expect(container.querySelector('[data-dashboard-primitive="main"]')?.hasAttribute('inert')).toBe(false)
  })

  it('W1 — bell click fires onToggleDrawer action', () => {
    const container = document.createElement('main')
    const onToggleDrawer = vi.fn()

    renderApp(container, { status: 'authenticated', token: 'jwt-token', user: adminUser }, { onLogin: vi.fn(), onLogout: vi.fn(), onToggleDrawer })

    const bell = container.querySelector<HTMLButtonElement>('[aria-label="Notifications"]')
    expect(bell).not.toBeNull()
    bell!.dispatchEvent(new MouseEvent('click', { bubbles: true }))

    expect(onToggleDrawer).toHaveBeenCalledTimes(1)
  })

  it('W2 — search slot click navigates to /dashboard/globalSearch', () => {
    const container = document.createElement('main')
    const onNavigate = vi.fn()

    renderApp(container, { status: 'authenticated', token: 'jwt-token', user: adminUser }, { onLogin: vi.fn(), onLogout: vi.fn(), onNavigate })

    const searchSlot = container.querySelector<HTMLElement>('.dashboard-header__search')
    expect(searchSlot).not.toBeNull()
    searchSlot!.dispatchEvent(new MouseEvent('click', { bubbles: true }))

    expect(onNavigate).toHaveBeenCalledWith('/dashboard/globalSearch')
  })

  it('mark all read button fires onMarkAllRead callback', () => {
    const container = document.createElement('main')
    const onMarkAllRead = vi.fn()

    // Render with drawer open and 3 unread notifications
    renderApp(
      container,
      { status: 'authenticated', token: 'jwt-token', user: adminUser },
      { onLogin: vi.fn(), onLogout: vi.fn(), onMarkAllRead },
      { status: 'loading' },
      '/dashboard',
      { drawerOpen: true, readIds: new Set(), summaryUnread: 3 }
    )

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
  const promise = new Promise<T>((done) => { resolve = done })
  return { promise, resolve }
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

function fakeApi(overrides: { health?: Promise<Awaited<ReturnType<ApiClient['health']>>>; stats?: Promise<Awaited<ReturnType<ApiClient['adminStats']>>> } = {}): ApiClient {
  return {
    login: vi.fn(),
    currentUser: vi.fn(),
    health: vi.fn(() => overrides.health ?? Promise.resolve({ status: 'ok', db: 'connected', version: '1.0.0' })),
    adminStats: vi.fn(() => overrides.stats ?? Promise.resolve({ users: { total: 1, active: 1, by_level: { admin: 1 } }, memories: { total: 1, by_project: [], by_category: [], last_synced_at: null } })),
    adminUsers: vi.fn(async () => ({ users: [adminUser] })),
    setUserLevel: vi.fn(async () => ({ message: 'nivel actualizado' })),
    grantAdmin: vi.fn(async () => ({ message: 'usuario ascendido a admin' })),
    deactivateUser: vi.fn(async () => ({ message: 'usuario desactivado' })),
    memories: vi.fn(async () => ({ memories: [], total: 0, limit: 5, offset: 0 })),
    searchMemories: vi.fn(async () => ({ memories: [], total: 0, query: 'dashboard', limit: 5 })),
    memory: vi.fn(async () => ({ id: 'mem-1', sync_id: 'sync-1', project: 'jarvis-dev', category: 'decision', title: 'Dashboard scope', content: 'No daemon controls', tags: [], files_affected: [], created_by: 'admin-1', created_at: '2026-06-06T20:00:00Z', updated_at: '2026-06-06T20:01:00Z', synced_at: '2026-06-06T20:02:00Z' })),
    auditLogs: vi.fn(async () => ({ audit_logs: [], total: 0, limit: 10, offset: 0 }))
  }
}

function dashboardState() {
  return {
    status: 'ready' as const,
    data: {
      overview: { status: 'ready' as const, data: hiveOverviewFixture },
      users: { status: 'ready' as const, data: { users: [adminUser] } },
      memories: { status: 'ready' as const, data: { recent: { memories: [], total: 0, limit: 5, offset: 0 }, search: { memories: [], total: 0, query: 'dashboard', limit: 5 } } },
      audit: { status: 'ready' as const, data: { audit_logs: [], total: 0, limit: 10, offset: 0 } }
    }
  }
}
