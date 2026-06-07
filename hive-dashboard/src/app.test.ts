import { describe, expect, it, vi } from 'vitest'
import { renderApp } from './main'

const adminUser = { id: 'admin-1', username: 'admin', email: 'admin@example.com', level: 'admin' as const, is_active: true, created_at: '2026-06-06T20:00:00Z' }

describe('dashboard shell', () => {
  it('shows the login form to unauthenticated users', () => {
    const container = document.createElement('main')

    renderApp(container, { status: 'anonymous' }, { onLogin: vi.fn(), onLogout: vi.fn() })

    expect(container.querySelector('h1')?.textContent).toBe('Sign in to Hive API')
    expect(container.querySelector('input[name="email"]')?.getAttribute('type')).toBe('email')
    expect(container.textContent).not.toContain('daemon')
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
})

function dashboardState() {
  return {
    status: 'ready' as const,
    data: {
      health: { status: 'ok', db: 'connected', version: '1.0.0' },
      stats: { users: { total: 1, active: 1, by_level: { admin: 1 } }, memories: { total: 1, by_project: [], by_category: [], last_synced_at: null } },
      users: { users: [adminUser] },
      memories: { recent: { memories: [], total: 0, limit: 5, offset: 0 }, search: { memories: [], total: 0, query: 'dashboard', limit: 5 } },
      audit: { audit_logs: [], total: 0, limit: 10, offset: 0 }
    }
  }
}
