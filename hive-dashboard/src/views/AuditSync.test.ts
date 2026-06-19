import { describe, expect, it } from 'vitest'
import { renderAuditSync } from './AuditSync'

describe('audit and sync view', () => {
  it('renders sync attempt reliability summaries for 24h, 7d, and 30d windows', () => {
    const view = renderAuditSync({ status: 'ready', data: summaryFixture() })

    expect(view.textContent).toContain('24h audit window — 3 attempts · 2 successes · 1 failure · 33.3% failure rate')
    expect(view.textContent).toContain('7d audit window — 5 attempts · 4 successes · 1 failure · 20.0% failure rate')
    expect(view.textContent).toContain('30d audit window — 8 attempts · 7 successes · 1 failure · 12.5% failure rate')
    expect(view.textContent).toContain('Last success: 2026-06-19T09:00:00Z')
    expect(view.textContent).toContain('Top error: NETWORK_ERROR (1)')
  })

  it('uses audit and reliability copy without live health or current status claims', () => {
    const text = renderAuditSync({ status: 'ready', data: summaryFixture() }).textContent?.toLowerCase() ?? ''

    expect(text).toContain('audit')
    expect(text).toContain('reliability')
    expect(text).not.toMatch(/real-time|live daemon|current status|healthy|degraded|unknown|start|stop|configure/)
  })

  it('renders empty history as a neutral audit state', () => {
    const view = renderAuditSync({ status: 'ready', data: { windows: [{ ...emptyWindow('24h') }, { ...emptyWindow('7d') }, { ...emptyWindow('30d') }] } })

    expect(view.textContent).toContain('No sync attempt history has been recorded for these audit windows.')
    expect(view.textContent?.toLowerCase()).not.toMatch(/failure|degraded|unknown|healthy/)
  })

  it('renders loading and error states', () => {
    expect(renderAuditSync({ status: 'loading' }).textContent).toContain('Loading sync attempt audit summary…')
    expect(renderAuditSync({ status: 'error', message: 'audit unavailable' }).textContent).toContain('audit unavailable')
  })
})

function summaryFixture() {
  return {
    windows: [
      windowFixture('24h', 3, 2, 1, 0.3333),
      windowFixture('7d', 5, 4, 1, 0.2),
      windowFixture('30d', 8, 7, 1, 0.125)
    ]
  }
}

function windowFixture(window: '24h' | '7d' | '30d', total: number, successes: number, failures: number, failure_rate: number) {
  return {
    window,
    total,
    successes,
    failures,
    failure_rate,
    last_success_at: '2026-06-19T09:00:00Z',
    last_failure_at: '2026-06-19T08:00:00Z',
    by_developer: [{ key: 'ada@example.com', count: total }],
    by_project: [{ key: 'jarvis-dev', count: total }],
    by_client: [{ key: 'hive-daemon', count: total }],
    by_daemon: [{ key: 'daemon-1', count: total }],
    by_outcome: [{ key: 'success', count: successes }, { key: 'failure', count: failures }],
    by_error_code: [{ key: 'NETWORK_ERROR', count: failures }],
    top_errors: [{ key: 'NETWORK_ERROR', count: failures }]
  }
}

function emptyWindow(window: '24h' | '7d' | '30d') {
  return { ...windowFixture(window, 0, 0, 0, 0), last_success_at: null, last_failure_at: null, top_errors: [] }
}
