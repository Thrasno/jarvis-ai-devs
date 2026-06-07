import { describe, expect, it, vi } from 'vitest'
import { ApiError, createApiClient } from './client'

const adminUser = { id: 'user-1', username: 'admin', email: 'admin@example.com', level: 'admin', is_active: true, created_at: '2026-06-06T20:00:00Z' }

describe('Hive API client', () => {
  it('logs in against the same-origin auth endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ token: 'jwt-token', user: adminUser }))
    const client = createApiClient({ fetch: fetchMock })

    const response = await client.login('admin@example.com', 'secret')

    expect(response.token).toBe('jwt-token')
    expect(response.user.level).toBe('admin')
    expect(fetchMock).toHaveBeenCalledWith('/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email: 'admin@example.com', password: 'secret' })
    })
  })

  it('loads the current user with a bearer token', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(adminUser))
    const client = createApiClient({ fetch: fetchMock })

    const user = await client.currentUser('jwt-token')

    expect(user.email).toBe('admin@example.com')
    expect(fetchMock).toHaveBeenCalledWith('/auth/me', {
      method: 'GET',
      headers: { Authorization: 'Bearer jwt-token' }
    })
  })

  it('raises API errors with the server error message', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ error: 'invalid credentials' }, 401))
    const client = createApiClient({ fetch: fetchMock })

    await expect(client.login('admin@example.com', 'wrong')).rejects.toEqual(
      new ApiError('invalid credentials', 401)
    )
  })
})

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}
