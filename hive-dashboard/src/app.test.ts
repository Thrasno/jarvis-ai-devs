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
    expect(container.textContent).toContain('Admin dashboard')
    expect(container.textContent).not.toContain('daemon')
  })
})
