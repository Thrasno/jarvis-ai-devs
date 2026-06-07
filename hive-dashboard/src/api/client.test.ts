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

  it('loads read-only admin dashboard endpoints with a bearer token', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse({ status: 'ok', db: 'connected', version: '1.0.0' }))
      .mockResolvedValueOnce(jsonResponse({ users: { total: 2, active: 1, by_level: { admin: 1 } }, memories: { total: 4, by_project: [], by_category: [], last_synced_at: null } }))
      .mockResolvedValueOnce(jsonResponse({ users: [adminUser] }))
    const client = createApiClient({ fetch: fetchMock })

    await expect(client.health()).resolves.toMatchObject({ status: 'ok', db: 'connected' })
    await expect(client.adminStats('jwt-token')).resolves.toMatchObject({ users: { total: 2 } })
    await expect(client.adminUsers('jwt-token')).resolves.toEqual({ users: [adminUser] })

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/health', { method: 'GET' })
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/admin/stats', { method: 'GET', headers: { Authorization: 'Bearer jwt-token' } })
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/admin/users', { method: 'GET', headers: { Authorization: 'Bearer jwt-token' } })
  })

  it('loads memories, search, and audit logs through existing read-only endpoints', async () => {
    const memory = { id: 'mem-1', sync_id: 'sync-1', project: 'jarvis-dev', category: 'decision', title: 'Dashboard scope', content: 'No daemon controls', tags: [], files_affected: [], created_by: 'admin-1', created_at: '2026-06-06T20:00:00Z', updated_at: '2026-06-06T20:01:00Z', synced_at: '2026-06-06T20:02:00Z' }
    const audit = { id: 'audit-1', occurred_at: '2026-06-06T20:03:00Z', action: 'sync_push', outcome: 'success', entry_count: 2, metadata: { pushed_count: 2 } }
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse({ memories: [memory], total: 1, limit: 5, offset: 0 }))
      .mockResolvedValueOnce(jsonResponse({ memories: [memory], total: 1, query: 'dashboard scope', limit: 5 }))
      .mockResolvedValueOnce(jsonResponse({ audit_logs: [audit], total: 1, limit: 10, offset: 0 }))
    const client = createApiClient({ fetch: fetchMock })

    await expect(client.memories('jwt-token', { limit: 5 })).resolves.toMatchObject({ total: 1 })
    await expect(client.searchMemories('jwt-token', 'dashboard scope', { limit: 5 })).resolves.toMatchObject({ query: 'dashboard scope' })
    await expect(client.auditLogs('jwt-token', { action: 'sync_push', limit: 10 })).resolves.toMatchObject({ total: 1 })

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/memories?limit=5', { method: 'GET', headers: { Authorization: 'Bearer jwt-token' } })
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/memories/search?query=dashboard+scope&limit=5', { method: 'GET', headers: { Authorization: 'Bearer jwt-token' } })
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/admin/audit-logs?action=sync_push&limit=10', { method: 'GET', headers: { Authorization: 'Bearer jwt-token' } })
  })
})

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}
