import { describe, expect, it } from 'vitest'
import { control, emptyState, grid, metricCard, panel, stack, statusBadge, statusDot, statusLabel, text } from './dom'

describe('dashboard DOM primitives', () => {
  it('renders base layout primitives with accessible structure', () => {
    const dashboardPanel = panel('System overview', [
      stack([
        text('API status ok'),
        grid([metricCard({ label: 'Users', value: '3', detail: '2 active' })])
      ])
    ])

    expect(dashboardPanel.getAttribute('role')).toBe('region')
    expect(dashboardPanel.getAttribute('aria-labelledby')).toBeTruthy()
    expect(dashboardPanel.querySelector('h2')?.textContent).toBe('System overview')
    expect(dashboardPanel.querySelector('[data-dashboard-primitive="stack"]')?.textContent).toContain('API status ok')
    expect(dashboardPanel.querySelector('[data-dashboard-primitive="grid"]')?.textContent).toContain('Users')
    expect(dashboardPanel.querySelector('[data-dashboard-primitive="metric"]')?.getAttribute('aria-label')).toBe('Users: 3, 2 active')
  })

  it('renders controls and state primitives with non-color semantics', () => {
    const refresh = control('Refresh dashboard', { disabled: true })
    const empty = emptyState('No recent memories found')

    expect(refresh.tagName).toBe('BUTTON')
    expect(refresh.textContent).toBe('Refresh dashboard')
    expect(refresh.disabled).toBe(true)
    expect(refresh.getAttribute('aria-disabled')).toBe('true')
    expect(empty.getAttribute('role')).toBe('status')
    expect(empty.getAttribute('data-state')).toBe('empty')
    expect(empty.textContent).toBe('No recent memories found')
  })

  it('maps known statuses to stable accessible meanings', () => {
    const healthy = statusBadge('healthy')
    const degraded = statusBadge('degraded')
    const inactive = statusBadge('inactive')

    expect(healthy.textContent).toBe('Healthy')
    expect(healthy.getAttribute('data-dashboard-status')).toBe('healthy')
    expect(healthy.getAttribute('aria-label')).toBe('Healthy status: healthy')
    expect(degraded.textContent).toBe('Degraded')
    expect(degraded.getAttribute('data-dashboard-status')).toBe('warning')
    expect(degraded.getAttribute('aria-label')).toBe('Degraded status: degraded')
    expect(inactive.textContent).toBe('Inactive')
    expect(inactive.getAttribute('data-dashboard-status')).toBe('inactive')
    expect(inactive.getAttribute('aria-label')).toBe('Inactive status: inactive')
  })

  it('falls back unknown statuses to neutral semantics', () => {
    const badge = statusBadge('paused')

    expect(badge.textContent).toBe('Unknown')
    expect(badge.getAttribute('data-dashboard-status')).toBe('neutral')
    expect(badge.getAttribute('aria-label')).toBe('Neutral status: paused')
  })

  it('renders compact status dots with optional row-owned semantics', () => {
    const semantic = statusDot('healthy')
    const decorative = statusDot('degraded', { decorative: true })

    expect(semantic.getAttribute('data-dashboard-primitive')).toBe('status-dot')
    expect(semantic.getAttribute('data-dashboard-status')).toBe('healthy')
    expect(semantic.getAttribute('role')).toBe('img')
    expect(semantic.getAttribute('aria-label')).toBe('Healthy status: healthy')
    expect(decorative.getAttribute('data-dashboard-status')).toBe('warning')
    expect(decorative.getAttribute('aria-hidden')).toBe('true')
    expect(decorative.getAttribute('aria-label')).toBeNull()
  })

  it('centralizes status labels for view semantics', () => {
    expect(statusLabel('healthy')).toBe('Healthy')
    expect(statusLabel('degraded')).toBe('Degraded')
    expect(statusLabel('paused')).toBe('Unknown')
  })
})
