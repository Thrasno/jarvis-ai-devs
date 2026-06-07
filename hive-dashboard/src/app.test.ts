import { describe, expect, it, vi } from 'vitest'
import type { ApiClient } from './api/client'
import type { SessionStore } from './auth/session'
import { loadDashboard, renderApp, startDashboardApp } from './main'

const adminUser = { id: 'admin-1', username: 'admin', email: 'admin@example.com', level: 'admin' as const, is_active: true, created_at: '2026-06-06T20:00:00Z' }

describe('dashboard shell', () => {
  it('shows the login form to unauthenticated users', () => {
    const container = document.createElement('main')

    renderApp(container, { status: 'anonymous' }, { onLogin: vi.fn(), onLogout: vi.fn() })

    expect(container.querySelector('h1')?.textContent).toBe('Sign in to Hive API')
    expect(container.querySelector('input[name="email"]')?.getAttribute('type')).toBe('email')
    expect(container.textContent).not.toContain('daemon')
  })

  it('shows a useful error and keeps the login form when login fails', async () => {
    const container = document.createElement('main')
    const session = fakeSessionStore({ status: 'anonymous' })
    vi.mocked(session.login).mockRejectedValue(new Error('invalid credentials'))

    startDashboardApp(container, { api: fakeApi(), session })
    await Promise.resolve()
    container.querySelector<HTMLInputElement>('input[name="email"]')!.value = 'admin@example.com'
    container.querySelector<HTMLInputElement>('input[name="password"]')!.value = 'wrong'

    container.querySelector('form')!.dispatchEvent(new SubmitEvent('submit', { bubbles: true, cancelable: true }))
    await Promise.resolve()
    await Promise.resolve()

    expect(session.login).toHaveBeenCalledWith('admin@example.com', 'wrong')
    expect(container.querySelector('h1')?.textContent).toBe('Sign in to Hive API')
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('invalid credentials')
  })

  it('denies admin shell content to a non-admin identity', () => {
    const container = document.createElement('main')
    const memberUser = { ...adminUser, level: 'member' as const, username: 'member' }

    renderApp(container, { status: 'authenticated', token: 'jwt-token', user: memberUser }, { onLogin: vi.fn(), onLogout: vi.fn() })

    expect(container.textContent).toContain('Admin access required')
    expect(container.textContent).not.toContain('Admin dashboard')
  })

  it('renders the protected shell for an admin identity', () => {
    const container = document.createElement('main')

    renderApp(container, { status: 'authenticated', token: 'jwt-token', user: adminUser }, { onLogin: vi.fn(), onLogout: vi.fn() })

    expect(container.querySelector('h1')?.textContent).toBe('Hive API Dashboard')
    expect(container.querySelector('nav')?.textContent).toContain('Overview')
    expect(container.textContent).toContain('Loading dashboard data')
    expect(container.textContent).not.toContain('daemon')
  })

  it('renders route navigation and the selected API-backed view for admin deep links', () => {
    const container = document.createElement('main')

    renderApp(container, { status: 'authenticated', token: 'jwt-token', user: adminUser }, { onLogin: vi.fn(), onLogout: vi.fn() }, dashboardState(), '/dashboard/users')

    expect(container.querySelector('nav')?.textContent).toContain('Overview')
    expect(container.querySelector('a[href="/dashboard/memories"]')?.textContent).toBe('Memories')
    expect(container.querySelector('section h2')?.textContent).toBe('Users')
    expect(container.textContent).toContain('admin · active')
    expect(container.textContent).toContain('admin@example.com')
    expect(container.textContent).not.toContain('Authentication is active')
  })

  it('shows the named default memory search in the memories view', () => {
    const container = document.createElement('main')

    renderApp(container, { status: 'authenticated', token: 'jwt-token', user: adminUser }, { onLogin: vi.fn(), onLogout: vi.fn() }, dashboardState(), '/dashboard/memories')

    expect(container.textContent).toContain('Default search: "dashboard"')
  })

  it('does not render stale dashboard data after logout while a dashboard load is pending', async () => {
    const container = document.createElement('main')
    document.body.append(container)
    const health = deferred<Awaited<ReturnType<ApiClient['health']>>>()
    const session = fakeSessionStore({ status: 'authenticated', token: 'first-token', user: adminUser })

    startDashboardApp(container, { api: fakeApi({ health: health.promise }), session })
    await Promise.resolve()

    container.querySelector<HTMLButtonElement>('button')?.click()
    expect(container.querySelector('h1')?.textContent).toBe('Sign in to Hive API')

    health.resolve({ status: 'ok', db: 'connected', version: '1.0.0' })
    await Promise.resolve()
    await Promise.resolve()
    await Promise.resolve()

    expect(container.querySelector('h1')?.textContent).toBe('Sign in to Hive API')
    expect(container.textContent).not.toContain('API status ok')
  })

  it('renders a recoverable login form when bootstrap rejects', async () => {
    const container = document.createElement('main')
    const session = fakeSessionStore({ status: 'anonymous' })
    vi.mocked(session.bootstrap).mockRejectedValue(new Error('storage unavailable'))

    startDashboardApp(container, { api: fakeApi(), session })
    await Promise.resolve()
    await Promise.resolve()

    expect(container.querySelector('h1')?.textContent).toBe('Sign in to Hive API')
    expect(container.querySelector('input[name="email"]')).not.toBeNull()
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('Unable to restore your session')
  })

  it('keeps successful dashboard panels visible when one endpoint fails', async () => {
    const dashboard = await loadDashboard(fakeApi({ stats: Promise.reject(new Error('stats unavailable')) }), 'jwt-token')

    expect(dashboard.status).toBe('ready')
    if (dashboard.status !== 'ready') throw new Error('expected ready dashboard')
    expect(dashboard.data.overview).toEqual({ status: 'error', message: 'stats unavailable' })
    expect(dashboard.data.users.status).toBe('ready')
    expect(dashboard.data.memories.status).toBe('ready')
    expect(dashboard.data.audit.status).toBe('ready')
  })
})

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
    memories: vi.fn(async () => ({ memories: [], total: 0, limit: 5, offset: 0 })),
    searchMemories: vi.fn(async () => ({ memories: [], total: 0, query: 'dashboard', limit: 5 })),
    auditLogs: vi.fn(async () => ({ audit_logs: [], total: 0, limit: 10, offset: 0 }))
  }
}

function dashboardState() {
  return {
    status: 'ready' as const,
    data: {
      overview: { status: 'ready' as const, data: { health: { status: 'ok', db: 'connected', version: '1.0.0' }, stats: { users: { total: 1, active: 1, by_level: { admin: 1 } }, memories: { total: 1, by_project: [], by_category: [], last_synced_at: null } } } },
      users: { status: 'ready' as const, data: { users: [adminUser] } },
      memories: { status: 'ready' as const, data: { recent: { memories: [], total: 0, limit: 5, offset: 0 }, search: { memories: [], total: 0, query: 'dashboard', limit: 5 } } },
      audit: { status: 'ready' as const, data: { audit_logs: [], total: 0, limit: 10, offset: 0 } }
    }
  }
}
