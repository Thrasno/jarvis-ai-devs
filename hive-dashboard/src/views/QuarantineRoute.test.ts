import { describe, expect, it, vi } from 'vitest'
import { ApiError, type ApiClient, type QuarantineDetailResponse, type QuarantineSummary, type User } from '../api/client'
import { type AuthState, type SessionStore } from '../auth/session'
import { renderApp, startDashboardApp } from '../main'

const admin = { id: 'admin-1', username: 'admin', email: 'admin@example.com', level: 'admin' as const, is_active: true, created_at: '2026-08-01T00:00:00Z' }

describe('Quarantine Center route', () => {
  it('renders the admin route without using fixture data as a fallback', () => {
    const container = document.createElement('main')

    renderApp({
      container,
      state: { status: 'authenticated', token: 'token', user: admin },
      routePath: '/dashboard/quarantines',
      actions: { onLogin: vi.fn(), onLogout: vi.fn() },
      dashboard: { status: 'ready', data: { quarantines: { status: 'error', message: 'Quarantine Center is not supported by this server.' } } }
    })

    expect(container.querySelector('h2')?.textContent).toBe('Quarantine Center')
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('not supported')
    expect(container.textContent).not.toContain('Ada Okafor')
  })

  it('denies non-admin route rendering before quarantine metadata is displayed', () => {
    const container = document.createElement('main')
    const member = { ...admin, level: 'member' as const }

    renderApp({
      container,
      state: { status: 'authenticated', token: 'token', user: member },
      routePath: '/dashboard/quarantines',
      actions: { onLogin: vi.fn(), onLogout: vi.fn() },
      dashboard: { status: 'ready', data: { quarantines: { status: 'ready', data: { summaries: [{ project: 'Secret', canonical_project_key: 'secret', generation: 1, action: 'BLOCK', state: 'QUARANTINING', transitioned_at: '2026-08-01T00:00:00Z' }] } } } }
    })

    expect(container.querySelector('[role="alert"]')?.textContent).toContain('Administrator access required')
    expect(container.textContent).not.toContain('Secret')
  })

  it('reloads retained quarantine state when the independently disabled center is re-enabled', async () => {
    const originalPath = window.location.pathname
    history.replaceState(null, '', '/dashboard/quarantines')
    const container = document.createElement('main')
    document.body.append(container)
    const api = quarantineApi()
    vi.mocked(api.quarantines)
      .mockRejectedValueOnce(new ApiError('not found', 404, 'NOT_FOUND'))
      .mockResolvedValueOnce({ quarantines: [quarantineDetail()] })
    const cleanup = startDashboardApp(container, { api, session: authenticatedSession() })

    try {
      await flushRoute()
      expect(container.textContent).toContain('not found')
      expect(container.textContent).not.toContain('Jarvis Dev')

      window.dispatchEvent(new PopStateEvent('popstate'))
      await flushRoute()

      expect(api.quarantines).toHaveBeenCalledTimes(2)
      expect(container.textContent).toContain('Jarvis Dev')
      expect(container.textContent).toContain('ada')
    } finally {
      cleanup()
      container.remove()
      history.replaceState(null, '', originalPath)
    }
  })

  it('uses the real route to request the next cursor and restores scroll context after the rerender', async () => {
    const originalPath = window.location.pathname
    history.replaceState(null, '', '/dashboard/quarantines')
    const container = document.createElement('main')
    document.body.append(container)
    const firstPage = quarantineDetail({ next_cursor: 'cursor-2', progress: [{ username: 'ada', state: 'applied' }] })
    const secondPage = quarantineDetail({ next_cursor: undefined, progress: [{ username: 'bea', state: 'pending' }] })
    const progress = vi.fn()
      .mockResolvedValueOnce(firstPage)
      .mockResolvedValueOnce(secondPage)
    const api = quarantineApi({ quarantineProgress: progress })
    const session = authenticatedSession()
    const cleanup = startDashboardApp(container, { api, session })

    try {
      await flushRoute()
      const view = container.querySelector<HTMLElement>('[data-dashboard-view="quarantine"]')!
      view.scrollTop = 180
      view.dispatchEvent(new Event('scroll'))
      container.querySelector<HTMLButtonElement>('button[data-quarantine-next-page]')!.click()
      await flushRoute()

      expect(progress).toHaveBeenLastCalledWith('token', 'jarvis-dev', 14, 'cursor-2', expect.any(AbortSignal))
      expect(container.textContent).toContain('bea')
      expect(container.querySelector<HTMLElement>('[data-dashboard-view="quarantine"]')?.scrollTop).toBe(180)
    } finally {
      cleanup()
      container.remove()
      history.replaceState(null, '', originalPath)
    }
  })

  it('routes release through the guarded controller, aborts it on teardown, and ignores its late completion', async () => {
    const originalPath = window.location.pathname
    history.replaceState(null, '', '/dashboard/quarantines')
    const container = document.createElement('main')
    document.body.append(container)
    const release = deferred<Awaited<ReturnType<ApiClient['blockProject']>>>()
    const api = quarantineApi({ blockProject: release.promise })
    const cleanup = startDashboardApp(container, { api, session: authenticatedSession() })

    try {
      await flushRoute()
      container.querySelector<HTMLButtonElement>('button[data-quarantine-release]')!.click()
      await Promise.resolve()

      expect(api.blockProject).toHaveBeenCalledWith('token', expect.objectContaining({ action: 'unblock', project: 'Jarvis Dev' }), expect.any(AbortSignal))
      expect(container.textContent).toContain('Releasing…')
      const signal = vi.mocked(api.blockProject).mock.calls[0]?.[2]
      cleanup()
      release.resolve({ command_id: 'late', project: 'Jarvis Dev', canonical_project_key: 'jarvis-dev', reason: 'late', blocked_at: '2026-08-01T00:00:00Z' })
      await flushRoute()

      expect(signal?.aborted).toBe(true)
      expect(container.textContent).toContain('Releasing…')
    } finally {
      container.remove()
      history.replaceState(null, '', originalPath)
    }
  })

  it('keeps the competing selected generation after a real release completion loses route ownership', async () => {
    const originalPath = window.location.pathname
    history.replaceState(null, '', '/dashboard/quarantines')
    const container = document.createElement('main')
    document.body.append(container)
    const staleRelease = deferred<Awaited<ReturnType<ApiClient['blockProject']>>>()
    const current = quarantineDetail({ progress: [{ username: 'ada', state: 'applied' }] })
    const competing = quarantineDetail({
      project: 'Other Project', canonical_project_key: 'other-project', generation: 15,
      progress: [{ username: 'zoe', state: 'pending' }]
    })
    const progress = vi.fn(async (_token: string, project: string) => project === competing.canonical_project_key ? competing : current)
    const api = quarantineApi({ quarantineProgress: progress, blockProject: staleRelease.promise })
    vi.mocked(api.quarantines).mockResolvedValue({ quarantines: [current, competing] })
    const cleanup = startDashboardApp(container, { api, session: authenticatedSession() })

    try {
      await flushRoute()
      container.querySelector<HTMLButtonElement>('button[data-quarantine-release]')!.click()
      await Promise.resolve()
      const signal = vi.mocked(api.blockProject).mock.calls[0]?.[2]
      expect(api.blockProject).toHaveBeenCalledWith('token', expect.objectContaining({ project: 'Jarvis Dev', action: 'unblock' }), expect.any(AbortSignal))

      container.querySelector<HTMLButtonElement>('button[data-quarantine-project="other-project"]')!.click()
      await flushRoute()
      staleRelease.resolve({ command_id: 'late', project: 'Jarvis Dev', canonical_project_key: 'jarvis-dev', reason: 'late', blocked_at: '2026-08-01T00:00:00Z' })
      await flushRoute()

      expect(signal?.aborted).toBe(true)
      expect(window.location.search).toBe('?project=other-project')
      expect(container.textContent).toContain('Other Project')
      expect(container.textContent).toContain('zoe')
      expect(container.textContent).not.toContain('Releasing…')
      expect(container.textContent).not.toContain('ada')
    } finally {
      cleanup()
      container.remove()
      history.replaceState(null, '', originalPath)
    }
  })

  it('ends the real route session when guarded release receives a 401 response', async () => {
    const originalPath = window.location.pathname
    history.replaceState(null, '', '/dashboard/quarantines')
    const container = document.createElement('main')
    document.body.append(container)
    const session = authenticatedSession()
    const api = quarantineApi()
    vi.mocked(api.blockProject).mockRejectedValue(new ApiError('expired', 401, 'UNAUTHORIZED'))
    const cleanup = startDashboardApp(container, { api, session })

    try {
      await flushRoute()
      container.querySelector<HTMLButtonElement>('button[data-quarantine-release]')!.click()
      await flushRoute()

      expect(session.logout).toHaveBeenCalledOnce()
      expect(session.getState()).toEqual({ status: 'anonymous' })
      expect(container.querySelector('h1')?.textContent).toContain('Sign in')
    } finally {
      cleanup()
      container.remove()
      history.replaceState(null, '', originalPath)
    }
  })
})

function quarantineApi(overrides: { quarantineProgress?: Promise<QuarantineDetailResponse> | ReturnType<typeof vi.fn>; blockProject?: Promise<Awaited<ReturnType<ApiClient['blockProject']>>> } = {}): ApiClient {
  return {
    quarantines: vi.fn().mockResolvedValue({ quarantines: [quarantineDetail()] }),
    quarantineProgress: typeof overrides.quarantineProgress === 'function' ? overrides.quarantineProgress : vi.fn().mockResolvedValue(overrides.quarantineProgress ?? quarantineDetail()),
    blockProject: vi.fn(() => overrides.blockProject ?? Promise.resolve({ command_id: 'cmd-1', project: 'Jarvis Dev', canonical_project_key: 'jarvis-dev', reason: 'release', blocked_at: '2026-08-01T00:00:00Z' }))
  } as unknown as ApiClient
}

function authenticatedSession(): SessionStore {
  let state: AuthState = { status: 'authenticated', token: 'token', user: admin }
  const logout = vi.fn(() => {
    state = { status: 'anonymous' }
    return state
  })
  return {
    getState: () => state,
    login: vi.fn(),
    loginWithOwnership: vi.fn(),
    bootstrap: vi.fn(async () => state),
    logout
  }
}

function quarantineDetail(overrides: Partial<QuarantineDetailResponse> = {}): QuarantineDetailResponse {
  return {
    project: 'Jarvis Dev', canonical_project_key: 'jarvis-dev', generation: 14, action: 'BLOCK', state: 'QUARANTINING', transitioned_at: '2026-08-01T00:00:00Z', totals: { active: 2, acknowledged: 1, pending: 1 }, progress: [{ username: 'ada', state: 'applied' }], ...overrides
  }
}

async function flushRoute(): Promise<void> {
  await Promise.resolve()
  await Promise.resolve()
  await Promise.resolve()
  await Promise.resolve()
  await Promise.resolve()
  await Promise.resolve()
  await Promise.resolve()
  await Promise.resolve()
}

function deferred<T>(): { promise: Promise<T>; resolve(value: T): void } {
  let resolve!: (value: T) => void
  return { promise: new Promise<T>((complete) => { resolve = complete }), resolve }
}
