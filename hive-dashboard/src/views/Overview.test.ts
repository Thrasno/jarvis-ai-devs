import { describe, expect, it } from 'vitest'
import { conflictViewerFixture } from '../fixtures/hive-dashboard/governance'
import { hiveOverviewFixture } from '../fixtures/hive-dashboard/overview'
import type { OverviewFixtureViewModel } from '../domain/dashboard'
import { renderOverview, type ViewState } from './Overview'

describe('overview view', () => {
  it('renders loading state', () => {
    const view = renderOverview({ status: 'loading' })
    expect(getByRole(view, 'region', { name: 'Hive Overview' })).toBe(view)
    expect(view.textContent).toContain('Loading overview')
  })

  it('renders error state', () => {
    const view = renderOverview({ status: 'error', message: 'network failure' })
    const alert = getByRole(view, 'alert')
    expect(alert.textContent).toContain('network failure')
  })

  it('renders KPI cards for all four metrics', () => {
    const view = renderOverview({ status: 'ready', data: hiveOverviewFixture })
    const metrics = getAllByRole(view, 'group')

    expect(metrics).toHaveLength(4)
    expect(metrics.map(accessibleName)).toEqual(
      expect.arrayContaining([
        'Total Memories: 22.4k, Demo fixture data — live data is unavailable.',
        'Active Projects: 8, Demo fixture data — live data is unavailable.',
        'Healthy Daemons: 56% · 5/9, Demo fixture data — live daemon counts are unavailable.',
        `Open Conflicts: ${conflictViewerFixture.summary.open}, Demo fixture data — live conflict counts are unavailable.`
      ])
    )
  })

  it('renders sync health display as "78% · 7/9" for healthyDaemons card', () => {
    const fixture: OverviewFixtureViewModel = {
      ...hiveOverviewFixture,
      healthyDaemons: { label: 'Healthy Daemons', value: 7, displayValue: '7/9' }
    }
    const view = renderOverview({ status: 'ready', data: fixture })
    expect(getByRole(view, 'group', { name: 'Healthy Daemons: 78% · 7/9' })).toBeDefined()
  })

  it('renders Open Conflicts from the governance fixture contract', () => {
    const view = renderOverview({ status: 'ready', data: hiveOverviewFixture })
    expect(
      getByRole(view, 'group', {
        name: `Open Conflicts: ${conflictViewerFixture.summary.open}, Demo fixture data — live conflict counts are unavailable.`
      })
    ).toBeDefined()
  })

  it('renders Knowledge Growth chart with 5 data points', () => {
    const view = renderOverview({ status: 'ready', data: hiveOverviewFixture })
    const chart = getByRole(view, 'figure', { name: 'Knowledge Growth' })
    const marks = getAllByRole(chart, 'listitem')

    expect(marks).toHaveLength(5)
    for (const point of hiveOverviewFixture.knowledgeGrowth.points) {
      expect(getByRole(chart, 'listitem', { name: `Knowledge Growth point ${point.label}: ${point.value}` })).toBeDefined()
    }
  })

  it('renders Knowledge Growth chart label', () => {
    const view = renderOverview({ status: 'ready', data: hiveOverviewFixture })
    expect(getByRole(view, 'figure', { name: 'Knowledge Growth' })).toBeDefined()
  })

  it('labels fixture-backed overview fields as demo data', () => {
    const view = renderOverview({ status: 'ready', data: hiveOverviewFixture })
    const syncHealth = getByRole(view, 'region', { name: 'Sync health by project' })

    expect(getByRole(view, 'group', { name: /Open Conflicts: .*Demo fixture data/ })).toBeDefined()
    expect(getByRole(view, 'figure', { name: 'Knowledge Growth' }).textContent).toContain(
      'Demo fixture data — live historical knowledge growth is unavailable.'
    )
    expect(syncHealth.textContent).toContain('Demo fixture data — live per-project sync health is unavailable.')
  })

  it('renders sync health rows — one row per project', () => {
    const view = renderOverview({ status: 'ready', data: hiveOverviewFixture })
    const syncHealth = getByRole(view, 'list', { name: 'Sync health by project rows' })
    const rows = getAllByRole(syncHealth, 'listitem')

    expect(rows).toHaveLength(hiveOverviewFixture.syncHealthByProject.length)
  })

  it('renders sync health rows with status badge and project name', () => {
    const view = renderOverview({ status: 'ready', data: hiveOverviewFixture })
    const project = hiveOverviewFixture.syncHealthByProject[0]
    const syncHealth = getByRole(view, 'list', { name: 'Sync health by project rows' })

    const row = getByRole(syncHealth, 'listitem', {
      name: `${project.name}: Healthy, ${project.memoryCount} memories, last synced ${project.lastSyncLabel}`
    })
    expect(row).toBeDefined()
    expect(getByLabelText(row, 'Healthy status: healthy')).toBeDefined()
  })

  it('handles zero-value metrics safely', () => {
    const zeroFixture: OverviewFixtureViewModel = {
      ...hiveOverviewFixture,
      totalMemories: { label: 'Total Memories', value: 0 },
      activeProjects: { label: 'Active Projects', value: 0 }
    }
    const view = renderOverview({ status: 'ready', data: zeroFixture })
    expect(getByRole(view, 'group', { name: 'Total Memories: 0' })).toBeDefined()
    expect(getByRole(view, 'group', { name: 'Active Projects: 0' })).toBeDefined()
  })

  it('handles empty syncHealthByProject safely', () => {
    const emptyFixture: OverviewFixtureViewModel = {
      ...hiveOverviewFixture,
      syncHealthByProject: []
    }
    const view = renderOverview({ status: 'ready', data: emptyFixture })
    const syncHealth = getByRole(view, 'region', { name: 'Sync health by project' })
    const emptyState = getByRole(syncHealth, 'status')

    expect(emptyState.textContent).toBe(
      'No project sync health data is available. Demo fixture data — live per-project sync health is unavailable.'
    )
    expect(queryAllByRole(syncHealth, 'listitem')).toHaveLength(0)
  })

  it('renders chart marks even when all growth values are zero', () => {
    const fixture: OverviewFixtureViewModel = {
      ...hiveOverviewFixture,
      knowledgeGrowth: {
        ...hiveOverviewFixture.knowledgeGrowth,
        points: hiveOverviewFixture.knowledgeGrowth.points.map((p) => ({ label: p.label, value: 0 }))
      }
    }
    const view = renderOverview({ status: 'ready', data: fixture })
    const chart = getByRole(view, 'figure', { name: 'Knowledge Growth' })
    const marks = getAllByRole(chart, 'listitem')

    expect(marks).toHaveLength(hiveOverviewFixture.knowledgeGrowth.points.length)
    for (const point of hiveOverviewFixture.knowledgeGrowth.points) {
      expect(getByRole(chart, 'listitem', { name: `Knowledge Growth point ${point.label}: 0` })).toBeDefined()
    }
  })

  it('renders degraded project badge distinct from healthy', () => {
    const view = renderOverview({ status: 'ready', data: hiveOverviewFixture })
    const syncHealth = getByRole(view, 'list', { name: 'Sync health by project rows' })

    const healthyRow = getByRole(syncHealth, 'listitem', { name: 'core-api: Healthy, 4821 memories, last synced 2m ago' })
    const degradedRow = getByRole(syncHealth, 'listitem', {
      name: 'billing-worker: Degraded, 1633 memories, last synced 38m ago'
    })

    expect(getByLabelText(healthyRow, 'Healthy status: healthy').textContent).toBe('Healthy')
    expect(getByLabelText(degradedRow, 'Degraded status: degraded').textContent).toBe('Degraded')
  })
})

type QueryOptions = { name?: string | RegExp }

function getByRole(root: HTMLElement, role: string, options: QueryOptions = {}): HTMLElement {
  const matches = getAllByRole(root, role, options)
  if (matches.length !== 1) throw new Error(`Expected exactly one ${role}, found ${matches.length}`)
  return matches[0]
}

function getAllByRole(root: HTMLElement, role: string, options: QueryOptions = {}): HTMLElement[] {
  const matches = queryAllByRole(root, role, options)
  if (matches.length === 0) throw new Error(`Expected at least one ${role}`)
  return matches
}

function queryAllByRole(root: HTMLElement, role: string, options: QueryOptions = {}): HTMLElement[] {
  return [root, ...Array.from(root.querySelectorAll<HTMLElement>('*'))].filter((element) => {
    if (elementRole(element) !== role) return false
    return options.name === undefined || matchesName(accessibleName(element), options.name)
  })
}

function getByLabelText(root: HTMLElement, label: string | RegExp): HTMLElement {
  const matches = [root, ...Array.from(root.querySelectorAll<HTMLElement>('[aria-label]'))].filter((element) =>
    matchesName(accessibleName(element), label)
  )
  if (matches.length !== 1) throw new Error(`Expected exactly one labelled element, found ${matches.length}`)
  return matches[0]
}

function elementRole(element: HTMLElement): string | undefined {
  const explicitRole = element.getAttribute('role')?.split(' ')[0]
  if (explicitRole) return explicitRole

  if (/^H[1-6]$/.test(element.tagName)) return 'heading'
  if (element.tagName === 'OL' || element.tagName === 'UL') return 'list'
  if (element.tagName === 'LI') return 'listitem'
  return undefined
}

function accessibleName(element: HTMLElement): string {
  const label = element.getAttribute('aria-label')
  if (label) return normalize(label)

  const labelledBy = element.getAttribute('aria-labelledby')
  if (labelledBy) {
    return normalize(
      labelledBy
        .split(/\s+/)
        .map((id) => element.ownerDocument.getElementById(id)?.textContent ?? element.querySelector(`#${id}`)?.textContent ?? '')
        .join(' ')
    )
  }

  return normalize(element.textContent ?? '')
}

function matchesName(actual: string, expected: string | RegExp): boolean {
  return typeof expected === 'string' ? actual === expected : expected.test(actual)
}

function normalize(value: string): string {
  return value.replace(/\s+/g, ' ').trim()
}
