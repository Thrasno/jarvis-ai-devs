import { describe, expect, it, vi } from 'vitest'
import { createSessionStore } from './session'

const adminUser = { id: 'user-1', username: 'admin', email: 'admin@example.com', level: 'admin' as const, is_active: true, created_at: '2026-06-06T20:00:00Z' }

describe('dashboard auth session', () => {
  it('stores the JWT in sessionStorage after login', async () => {
    const api = {
      login: vi.fn().mockResolvedValue({ token: 'jwt-token', user: adminUser }),
      currentUser: vi.fn()
    }
    const session = createSessionStore({ api })

    const state = await session.login('admin@example.com', 'secret')

    expect(state).toEqual({ status: 'authenticated', token: 'jwt-token', user: adminUser })
    expect(sessionStorage.getItem('hive-dashboard.jwt')).toBe('jwt-token')
  })

  it('bootstraps an existing session using /auth/me', async () => {
    sessionStorage.setItem('hive-dashboard.jwt', 'stored-token')
    const api = {
      login: vi.fn(),
      currentUser: vi.fn().mockResolvedValue(adminUser)
    }
    const session = createSessionStore({ api })

    const state = await session.bootstrap()

    expect(api.currentUser).toHaveBeenCalledWith('stored-token')
    if (state.status !== 'authenticated') {
      throw new Error('expected an authenticated state')
    }
    expect(state.user.email).toBe('admin@example.com')
  })

  it('clears the stored JWT when bootstrap fails', async () => {
    sessionStorage.setItem('hive-dashboard.jwt', 'expired-token')
    const api = {
      login: vi.fn(),
      currentUser: vi.fn().mockRejectedValue(new Error('unauthorized'))
    }
    const session = createSessionStore({ api })

    const state = await session.bootstrap()

    expect(state).toEqual({ status: 'anonymous' })
    expect(sessionStorage.getItem('hive-dashboard.jwt')).toBeNull()
  })
})
