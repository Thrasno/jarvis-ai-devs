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

    await expect(client.login('admin@example.com', 'wrong')).rejects.toMatchObject({
      name: 'ApiError',
      message: 'invalid credentials',
      status: 401,
      code: 'UNAUTHORIZED'
    })
  })

  it('normalizes network, non-json, and JSON message API errors', async () => {
    const networkClient = createApiClient({ fetch: vi.fn().mockRejectedValue(new TypeError('Failed to fetch')) })
    await expect(networkClient.health()).rejects.toMatchObject({ status: 0, code: 'NETWORK_ERROR', message: 'Network request failed' })

    const htmlClient = createApiClient({ fetch: vi.fn().mockResolvedValue(new Response('<h1>Bad gateway</h1>', { status: 502, statusText: 'Bad Gateway', headers: { 'Content-Type': 'text/html' } })) })
    await expect(htmlClient.health()).rejects.toMatchObject({ status: 502, code: 'NON_JSON_RESPONSE', message: 'Bad Gateway' })

    const jsonClient = createApiClient({ fetch: vi.fn().mockResolvedValue(jsonResponse({ message: 'project is required', code: 'VALIDATION_FAILED', details: { field: 'project' } }, 400)) })
    await expect(jsonClient.memories('jwt-token')).rejects.toMatchObject({ status: 400, code: 'VALIDATION_FAILED', message: 'project is required', details: { field: 'project' } })
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

  it('loads memories, memory detail, search, and audit logs through existing read-only endpoints', async () => {
    const memory = { id: 'mem-1', sync_id: 'sync-1', project: 'jarvis-dev', category: 'decision', title: 'Dashboard scope', content: 'No daemon controls', tags: [], files_affected: [], created_by: 'admin-1', created_at: '2026-06-06T20:00:00Z', updated_at: '2026-06-06T20:01:00Z', synced_at: '2026-06-06T20:02:00Z' }
    const audit = { id: 'audit-1', occurred_at: '2026-06-06T20:03:00Z', action: 'sync_push', outcome: 'success', entry_count: 2, metadata: { pushed_count: 2 } }
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse({ memories: [memory], total: 1, limit: 5, offset: 0 }))
      .mockResolvedValueOnce(jsonResponse(memory))
      .mockResolvedValueOnce(jsonResponse({ memories: [memory], total: 1, query: 'dashboard scope', limit: 5 }))
      .mockResolvedValueOnce(jsonResponse({ audit_logs: [audit], total: 1, limit: 10, offset: 20 }))
    const client = createApiClient({ fetch: fetchMock })

    await expect(client.memories('jwt-token', { project: 'jarvis-dev', category: 'decision', limit: 5 })).resolves.toMatchObject({ total: 1 })
    await expect(client.memory('jwt-token', 'mem-1')).resolves.toMatchObject({ id: 'mem-1' })
    await expect(client.searchMemories('jwt-token', { query: 'dashboard scope', project: 'jarvis-dev', limit: 5 })).resolves.toMatchObject({ query: 'dashboard scope' })
    await expect(client.auditLogs('jwt-token', { action: 'sync_push', outcome: 'success', since: '2026-06-05T00:00:00Z', limit: 10, offset: 20 })).resolves.toMatchObject({ offset: 20 })

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/memories?project=jarvis-dev&category=decision&limit=5', { method: 'GET', headers: { Authorization: 'Bearer jwt-token' } })
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/memories/mem-1', { method: 'GET', headers: { Authorization: 'Bearer jwt-token' } })
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/memories/search?query=dashboard+scope&project=jarvis-dev&limit=5', { method: 'GET', headers: { Authorization: 'Bearer jwt-token' } })
    expect(fetchMock).toHaveBeenNthCalledWith(4, '/admin/audit-logs?since=2026-06-05T00%3A00%3A00Z&action=sync_push&outcome=success&limit=10&offset=20', { method: 'GET', headers: { Authorization: 'Bearer jwt-token' } })
  })

  it('loads admin sync attempt summaries from the production audit endpoint', async () => {
    const summary = {
      windows: [
        {
          window: '24h',
          total: 3,
          successes: 2,
          failures: 1,
          failure_rate: 0.3333,
          last_success_at: '2026-06-19T09:00:00Z',
          last_failure_at: '2026-06-19T08:00:00Z',
          by_developer: [{ key: 'ada@example.com', count: 3 }],
          by_project: [{ key: 'jarvis-dev', count: 3 }],
          by_client: [{ key: 'hive-daemon', count: 3 }],
          by_daemon: [{ key: 'daemon-1', count: 3 }],
          by_outcome: [{ key: 'success', count: 2 }, { key: 'failure', count: 1 }],
          by_error_code: [{ key: 'NETWORK_ERROR', count: 1 }],
          top_errors: [{ key: 'NETWORK_ERROR', count: 1 }]
        }
      ]
    }
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(summary))
    const client = createApiClient({ fetch: fetchMock })

    await expect(client.syncAttemptSummary('jwt-token', { window: '7d', project: 'jarvis-dev', dev_id: 'ada@example.com' })).resolves.toEqual(summary)

    expect(fetchMock).toHaveBeenCalledWith('/admin/sync-attempts/summary?window=7d&project=jarvis-dev&dev_id=ada%40example.com', {
      method: 'GET',
      headers: { Authorization: 'Bearer jwt-token' }
    })
  })

  it('calls existing admin user mutation endpoints with typed bodies', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse({ message: 'nivel actualizado' }))
      .mockResolvedValueOnce(jsonResponse({ message: 'usuario ascendido a admin' }))
      .mockResolvedValueOnce(jsonResponse({ message: 'usuario desactivado' }))
    const client = createApiClient({ fetch: fetchMock })

    await expect(client.setUserLevel('jwt-token', 'member@example.com', 'member')).resolves.toEqual({ message: 'nivel actualizado' })
    await expect(client.grantAdmin('jwt-token', 'new-admin')).resolves.toEqual({ message: 'usuario ascendido a admin' })
    await expect(client.deactivateUser('jwt-token', 'old user')).resolves.toEqual({ message: 'usuario desactivado' })

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/admin/users/member%40example.com/level', {
      method: 'POST',
      headers: { Authorization: 'Bearer jwt-token', 'Content-Type': 'application/json' },
      body: JSON.stringify({ level: 'member' })
    })
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/admin/users/new-admin/grant-admin', { method: 'POST', headers: { Authorization: 'Bearer jwt-token' } })
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/admin/users/old%20user/deactivate', { method: 'POST', headers: { Authorization: 'Bearer jwt-token' } })
  })
})

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}
