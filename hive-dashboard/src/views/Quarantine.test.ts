import { describe, expect, it, vi } from 'vitest'
import type { QuarantineDetailResponse, QuarantineSummary } from '../api/client'
import { createQuarantineController, renderQuarantine } from './Quarantine'

const summary: QuarantineSummary = {
  project: 'Jarvis Dev',
  canonical_project_key: 'jarvis-dev',
  generation: 14,
  action: 'BLOCK',
  state: 'QUARANTINING',
  transitioned_at: '2026-08-01T12:00:00Z'
}

const detail: QuarantineDetailResponse = {
  ...summary,
  totals: { active: 3, acknowledged: 2, pending: 1 },
  progress: [
    { username: 'ada', state: 'applied', acknowledged_at: '2026-08-01T12:02:00Z' },
    { username: 'bea', state: 'pending' },
    { username: 'cyd', state: 'failed', acknowledged_at: '2026-08-01T12:03:00Z' }
  ]
}

describe('Quarantine Center', () => {
  it('renders only the approved username progress projection with semantic table and release confirmation', () => {
    const view = renderQuarantine({ status: 'ready', data: { summaries: [summary], detail } }, { selectedProject: summary.canonical_project_key, filter: '' })

    expect(view.querySelector('h2')?.textContent).toBe('Quarantine Center')
    expect(view.querySelectorAll('th').length).toBe(4)
    expect(view.textContent).toContain('ada')
    expect(view.textContent).toContain('No ACK received')
    expect(view.textContent).toContain('Release immediately')
    expect(view.textContent).not.toContain('email')
    expect(view.textContent).not.toContain('device')
    expect(view.querySelector<HTMLButtonElement>('button[data-quarantine-release]')?.disabled).toBe(false)
  })

  it('keeps filter and selection while discarding older generation refreshes and treating capability absence as terminal', async () => {
    const fetchDetail = vi.fn()
      .mockResolvedValueOnce({ ...detail, generation: 15 })
      .mockResolvedValueOnce(detail)
    const controller = createQuarantineController({ fetchDetail, release: vi.fn() })

    controller.select(summary.canonical_project_key)
    controller.setFilter('ada')
    await controller.refresh()
    await controller.refresh()

    expect(controller.state.selectedProject).toBe('jarvis-dev')
    expect(controller.state.filter).toBe('ada')
    expect(controller.state.detail?.generation).toBe(15)

    controller.unsupported()
    expect(controller.state.pollingStopped).toBe(true)
    expect(controller.state.message).toContain('not supported')
  })

  it('reports a release failure without clearing the selected quarantined detail', async () => {
    const release = vi.fn().mockRejectedValue(new Error('forbidden'))
    const controller = createQuarantineController({ fetchDetail: vi.fn().mockResolvedValue(detail), release })
    controller.select(summary.canonical_project_key)
    await controller.refresh()

    await controller.release()

    expect(release).toHaveBeenCalledWith(expect.objectContaining({ project: 'Jarvis Dev', generation: 14 }), expect.any(AbortSignal))
    expect(controller.state.detail?.state).toBe('QUARANTINING')
    expect(controller.state.message).toContain('forbidden')
  })

  it('stops polling on authorization failure and backs off transient refresh failures', async () => {
    const denied = Object.assign(new Error('forbidden'), { status: 403 })
    const controller = createQuarantineController({ fetchDetail: vi.fn().mockRejectedValueOnce(new Error('timeout')).mockRejectedValueOnce(denied), release: vi.fn() })
    controller.select(summary.canonical_project_key)

    await controller.refresh()
    expect(controller.state.retryDelayMs).toBe(30_000)
    await controller.refresh()

    expect(controller.state.pollingStopped).toBe(true)
    expect(controller.state.message).toContain('access denied')
  })

  it('polls the selected route every fifteen seconds, pauses while hidden, and resumes when visible', async () => {
    vi.useFakeTimers()
    const fetchDetail = vi.fn().mockResolvedValue(detail)
    const controller = createQuarantineController({ fetchDetail, release: vi.fn() })
    controller.select(summary.canonical_project_key)

    controller.startPolling()
    await vi.advanceTimersByTimeAsync(15_000)
    expect(fetchDetail).toHaveBeenCalledTimes(1)

    controller.setVisibility(true)
    await vi.advanceTimersByTimeAsync(30_000)
    expect(fetchDetail).toHaveBeenCalledTimes(1)

    controller.setVisibility(false)
    await vi.advanceTimersByTimeAsync(15_000)
    expect(fetchDetail).toHaveBeenCalledTimes(2)
    controller.destroy()
    vi.useRealTimers()
  })

  it('aborts destroyed refreshes and retains the requested cursor', async () => {
    let resolveDetail: ((value: QuarantineDetailResponse) => void) | undefined
    const fetchDetail = vi.fn((_project: string, _generation: number | undefined, _after: string | undefined, signal: AbortSignal) => new Promise<QuarantineDetailResponse>((resolve) => {
      resolveDetail = resolve
      signal.addEventListener('abort', () => resolve({ ...detail, generation: 13 }))
    }))
    const controller = createQuarantineController({ fetchDetail, release: vi.fn() })
    controller.select(summary.canonical_project_key)
    controller.setCursor('next-page')

    const refresh = controller.refresh()
    controller.destroy()
    resolveDetail?.({ ...detail, generation: 13 })
    await refresh

    expect(fetchDetail.mock.calls[0]?.[2]).toBe('next-page')
    expect(fetchDetail.mock.calls[0]?.[3].aborted).toBe(true)
    expect(controller.state.detail).toBeUndefined()
  })

  it('terminates the session on 401 while preserving filter, selection, cursor, and scroll during a newer refresh', async () => {
    const onUnauthorized = vi.fn()
    const controller = createQuarantineController({
      fetchDetail: vi.fn()
        .mockRejectedValueOnce(Object.assign(new Error('expired'), { status: 401 }))
        .mockResolvedValueOnce({ ...detail, generation: 15, next_cursor: 'page-2' }),
      release: vi.fn(),
      onUnauthorized
    })
    controller.select(summary.canonical_project_key)
    controller.setFilter('ada')
    controller.setCursor('page-1')
    controller.setScrollTop(240)
    await controller.refresh()

    expect(onUnauthorized).toHaveBeenCalledOnce()
    expect(controller.state.pollingStopped).toBe(true)

    const fresh = createQuarantineController({ fetchDetail: vi.fn().mockResolvedValue({ ...detail, generation: 15, next_cursor: 'page-2' }), release: vi.fn() })
    fresh.select(summary.canonical_project_key)
    fresh.setFilter('ada')
    fresh.setCursor('page-1')
    fresh.setScrollTop(240)
    await fresh.refresh()

    expect(fresh.state).toMatchObject({ selectedProject: 'jarvis-dev', filter: 'ada', cursor: 'page-1', nextCursor: 'page-2', scrollTop: 240 })
  })
})
