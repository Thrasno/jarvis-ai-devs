import { describe, expect, it } from 'vitest'
import { hiveOverviewFixture } from '../fixtures/hive-dashboard/overview'
import type { OverviewFixtureViewModel } from '../domain/dashboard'
import { renderOverview, type ViewState } from './Overview'

describe('overview view', () => {
  it('renders loading state', () => {
    const view = renderOverview({ status: 'loading' })
    expect(view.textContent).toContain('Loading overview')
    expect(view.getAttribute('role')).toBe('region')
  })

  it('renders error state', () => {
    const view = renderOverview({ status: 'error', message: 'network failure' })
    expect(view.getAttribute('role')).toBe('alert')
    expect(view.getAttribute('data-state')).toBe('error')
    expect(view.textContent).toContain('network failure')
  })

  it('renders KPI cards for all four metrics', () => {
    const view = renderOverview({ status: 'ready', data: hiveOverviewFixture })
    const metrics = Array.from(view.querySelectorAll('[data-dashboard-primitive="metric"]'))
    expect(metrics.length).toBe(4)

    const labels = metrics.map((m) => m.querySelector('.dashboard-metric-label')?.textContent ?? '')
    expect(labels).toContain('Total Memories')
    expect(labels).toContain('Active Projects')
    expect(labels).toContain('Healthy Daemons')
    expect(labels).toContain('Open Conflicts')
  })

  it('renders sync health display as "78% · 7/9" for healthyDaemons card', () => {
    const fixture: OverviewFixtureViewModel = {
      ...hiveOverviewFixture,
      healthyDaemons: { label: 'Healthy Daemons', value: 7, displayValue: '7/9' }
    }
    const view = renderOverview({ status: 'ready', data: fixture })
    expect(view.textContent).toContain('78% · 7/9')
  })

  it('renders Open Conflicts card with value 0', () => {
    const view = renderOverview({ status: 'ready', data: hiveOverviewFixture })
    const metrics = Array.from(view.querySelectorAll('[data-dashboard-primitive="metric"]'))
    const conflictsMetric = metrics.find(
      (m) => m.querySelector('.dashboard-metric-label')?.textContent === 'Open Conflicts'
    )
    expect(conflictsMetric).toBeDefined()
    expect(conflictsMetric?.querySelector('.dashboard-metric-value')?.textContent).toBe('0')
  })

  it('renders Knowledge Growth chart with 5 data points', () => {
    const view = renderOverview({ status: 'ready', data: hiveOverviewFixture })
    const marks = view.querySelectorAll('[data-chart-point]')
    expect(marks.length).toBe(5)
  })

  it('renders Knowledge Growth chart label', () => {
    const view = renderOverview({ status: 'ready', data: hiveOverviewFixture })
    const chart = view.querySelector('[data-chart-kind]')
    expect(chart).toBeDefined()
    expect(chart?.getAttribute('aria-label')).toBe('Knowledge Growth')
  })

  it('renders sync health rows — one row per project', () => {
    const view = renderOverview({ status: 'ready', data: hiveOverviewFixture })
    const stack = view.querySelector('[data-dashboard-primitive="stack"]')
    expect(stack).toBeDefined()
    const rows = Array.from(stack?.children ?? [])
    expect(rows.length).toBe(hiveOverviewFixture.syncHealthByProject.length)
  })

  it('renders sync health rows with status badge and project name', () => {
    const view = renderOverview({ status: 'ready', data: hiveOverviewFixture })
    const stack = view.querySelector('[data-dashboard-primitive="stack"]')
    const firstRow = stack?.children[0]
    expect(firstRow).toBeDefined()
    expect(firstRow?.querySelector('[data-dashboard-primitive="status"]')).toBeDefined()
    const rowText = firstRow?.textContent ?? ''
    expect(rowText).toContain(hiveOverviewFixture.syncHealthByProject[0].name)
  })

  it('handles zero-value metrics safely', () => {
    const zeroFixture: OverviewFixtureViewModel = {
      ...hiveOverviewFixture,
      totalMemories: { label: 'Total Memories', value: 0 },
      activeProjects: { label: 'Active Projects', value: 0 }
    }
    const view = renderOverview({ status: 'ready', data: zeroFixture })
    const metrics = Array.from(view.querySelectorAll('[data-dashboard-primitive="metric"]'))
    const labels = metrics.map((m) => m.querySelector('.dashboard-metric-label')?.textContent ?? '')
    expect(labels).toContain('Total Memories')
    expect(labels).toContain('Active Projects')
  })

  it('handles empty syncHealthByProject safely', () => {
    const emptyFixture: OverviewFixtureViewModel = {
      ...hiveOverviewFixture,
      syncHealthByProject: []
    }
    const view = renderOverview({ status: 'ready', data: emptyFixture })
    const stack = view.querySelector('[data-dashboard-primitive="stack"]')
    expect(stack).toBeDefined()
    expect(stack?.children.length).toBe(0)
  })
})
