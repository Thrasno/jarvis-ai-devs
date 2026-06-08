import { describe, expect, it } from 'vitest'
import { renderOverview } from './Overview'

describe('overview view', () => {
  it('renders API health and aggregate stats', () => {
    const view = renderOverview({ status: 'ready', data: { health: { status: 'ok', db: 'connected', version: '1.0.0' }, stats: { users: { total: 3, active: 2, by_level: { admin: 1 } }, memories: { total: 9, by_project: [{ project: 'jarvis-dev', count: 7 }], by_category: [{ category: 'decision', count: 4 }], last_synced_at: '2026-06-06T20:00:00Z' } } } })

    expect(view.textContent).toContain('API status ok')
    expect(view.textContent).toContain('3 users')
    expect(view.textContent).toContain('9 memories')
    expect(view.textContent).toContain('jarvis-dev: 7')
    expect(view.getAttribute('role')).toBe('region')
    expect(view.querySelector('[data-dashboard-status]')?.getAttribute('aria-label')).toBe('Healthy status: ok')
    expect(view.querySelector('[data-dashboard-primitive="metric"]')?.getAttribute('aria-label')).toBe('Users: 3 users, 2 active')
  })

  it('renders an error state without daemon controls', () => {
    const view = renderOverview({ status: 'error', message: 'stats unavailable' })

    expect(view.getAttribute('role')).toBe('alert')
    expect(view.getAttribute('data-state')).toBe('error')
    expect(view.textContent).toContain('stats unavailable')
    expect(view.textContent?.toLowerCase()).not.toMatch(/daemon|start|stop|configure/)
  })
})
