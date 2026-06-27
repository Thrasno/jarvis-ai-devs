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
        'Total Memories: 22.4k',
        'Active Projects: 8',
        'Healthy Daemons: 56% · 5/9',
        `Open Conflicts: ${conflictViewerFixture.summary.open}`
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
        name: `Open Conflicts: ${conflictViewerFixture.summary.open}`
      })
    ).toBeDefined()
  })

  it('renders Knowledge Growth chart as SVG with polyline and polygon', () => {
    const view = renderOverview({ status: 'ready', data: hiveOverviewFixture })
    const chart = getByRole(view, 'figure', { name: 'Knowledge Growth' })
    const svg = chart.querySelector('svg')

    expect(svg).not.toBeNull()
    expect(svg?.querySelector('polyline')).not.toBeNull()
    expect(svg?.querySelector('polygon')).not.toBeNull()
    expect(svg?.querySelector('line')).not.toBeNull()
  })

  it('renders Knowledge Growth chart label', () => {
    const view = renderOverview({ status: 'ready', data: hiveOverviewFixture })
    expect(getByRole(view, 'figure', { name: 'Knowledge Growth' })).toBeDefined()
  })

  it('does not expose fixture/demo source labels in the overview', () => {
    const view = renderOverview({ status: 'ready', data: hiveOverviewFixture })
    const syncHealth = getByRole(view, 'region', { name: 'Sync health by project' })

    expect(view.textContent).not.toContain('Demo fixture data')
    expect(getByRole(view, 'group', { name: `Open Conflicts: ${conflictViewerFixture.summary.open}` })).toBeDefined()
    expect(getByRole(view, 'figure', { name: 'Knowledge Growth' }).textContent).toContain('Knowledge growth over time.')
    expect(syncHealth.textContent).not.toContain('unavailable')
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
      name: `${project.name}: Healthy, ${project.contributorCount} contributors, region ${project.region}`
    })
    expect(row).toBeDefined()
    expect(getByLabelText(row, 'Healthy status: healthy')).toBeDefined()
    expect(row.textContent).not.toContain('memories')
    expect(row.textContent).not.toContain('last synced')
  })

  it('omits sync health region text and ARIA copy when region is blank', () => {
    const billingWorker = hiveOverviewFixture.syncHealthByProject.find((project) => project.name === 'billing-worker')
    if (!billingWorker) throw new Error('Missing billing-worker fixture')
    const fixture: OverviewFixtureViewModel = {
      ...hiveOverviewFixture,
      syncHealthByProject: [
        { ...hiveOverviewFixture.syncHealthByProject[0], region: '' },
        { ...billingWorker, region: '   ' }
      ]
    }
    const view = renderOverview({ status: 'ready', data: fixture })
    const syncHealth = getByRole(view, 'list', { name: 'Sync health by project rows' })

    const emptyRegionRow = getByRole(syncHealth, 'listitem', { name: 'core-api: Healthy, 6 contributors' })
    const whitespaceRegionRow = getByRole(syncHealth, 'listitem', { name: 'billing-worker: Degraded, 3 contributors' })

    expect(accessibleName(emptyRegionRow)).not.toContain('region')
    expect(accessibleName(whitespaceRegionRow)).not.toContain('region')
    expect(emptyRegionRow.textContent).not.toContain('region')
    expect(whitespaceRegionRow.textContent).not.toContain('region')
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

    expect(emptyState.textContent).toBe('No project sync health data is available.')
    expect(queryAllByRole(syncHealth, 'listitem')).toHaveLength(0)
  })

  it('renders chart as SVG even when all growth values are zero', () => {
    const fixture: OverviewFixtureViewModel = {
      ...hiveOverviewFixture,
      knowledgeGrowth: {
        ...hiveOverviewFixture.knowledgeGrowth,
        points: hiveOverviewFixture.knowledgeGrowth.points.map((p) => ({ label: p.label, value: 0 }))
      }
    }
    const view = renderOverview({ status: 'ready', data: fixture })
    const chart = getByRole(view, 'figure', { name: 'Knowledge Growth' })
    const svg = chart.querySelector('svg')

    expect(svg).not.toBeNull()
    // Polyline and polygon still rendered even for zero-value data
    expect(svg?.querySelector('polyline')).not.toBeNull()
    expect(svg?.querySelector('polygon')).not.toBeNull()
  })

  it('renders degraded project badge distinct from healthy', () => {
    const view = renderOverview({ status: 'ready', data: hiveOverviewFixture })
    const syncHealth = getByRole(view, 'list', { name: 'Sync health by project rows' })

    const healthyRow = getByRole(syncHealth, 'listitem', { name: 'core-api: Healthy, 6 contributors, region eu-west-1' })
    const degradedRow = getByRole(syncHealth, 'listitem', {
      name: 'billing-worker: Degraded, 3 contributors, region us-east-1'
    })

    expect(getByLabelText(healthyRow, 'Healthy status: healthy').textContent).toBe('Healthy')
    expect(getByLabelText(degradedRow, 'Degraded status: degraded').textContent).toBe('Degraded')
  })

  it('renders live activity using only count and newest sync id from the backend', () => {
    const fixture: OverviewFixtureViewModel = {
      ...hiveOverviewFixture,
      liveActivity: { count: 7, newestSyncId: 'sync-777' }
    }
    const view = renderOverview({ status: 'ready', data: fixture })
    const activity = getByRole(view, 'region', { name: 'Live activity' })

    expect(activity.textContent).toContain('7 recent sync events')
    expect(activity.textContent).toContain('Newest sync: sync-777')
    expect(activity.textContent).not.toContain('Saved')
    expect(activity.textContent).not.toContain('ago')
  })

  it('renders an empty live activity state without fixture fallback', () => {
    const fixture: OverviewFixtureViewModel = {
      ...hiveOverviewFixture,
      liveActivity: { count: 0, newestSyncId: '' }
    }
    const view = renderOverview({ status: 'ready', data: fixture })
    const activity = getByRole(view, 'region', { name: 'Live activity' })

    expect(getByRole(activity, 'status').textContent).toBe('No recent activity is available.')
    expect(activity.textContent).not.toContain('Demo fixture data')
  })

  it('renders most-active projects as SVG bar chart from live project counts', () => {
    const fixture: OverviewFixtureViewModel = {
      ...hiveOverviewFixture,
      mostActiveProjects: [{ label: 'jarvis-dev', value: 14 }, { label: 'hive-api', value: 9 }]
    }
    const view = renderOverview({ status: 'ready', data: fixture })
    const chart = getByRole(view, 'figure', { name: 'Most active projects' })
    const svg = chart.querySelector('svg')

    expect(svg).not.toBeNull()
    expect(svg?.querySelector('rect')).not.toBeNull()
    // Project labels should appear in the SVG text elements
    const svgTexts = Array.from(svg?.querySelectorAll('text') ?? []).map((t) => t.textContent)
    expect(svgTexts.some((t) => t?.includes('jarvis-dev'))).toBe(true)
    expect(svgTexts.some((t) => t?.includes('hive-api'))).toBe(true)
    expect(chart.textContent).not.toContain('data-pipeline')
  })

  it('renders an empty most-active projects state without fixture projects', () => {
    const fixture: OverviewFixtureViewModel = {
      ...hiveOverviewFixture,
      mostActiveProjects: []
    }
    const view = renderOverview({ status: 'ready', data: fixture })
    const chart = getByRole(view, 'figure', { name: 'Most active projects' })

    // Empty state uses the standard emptyState component with role=status
    const emptyEl = chart.querySelector('[role="status"]')
    expect(emptyEl?.textContent).toBe('No chart data is available for Most active projects.')
    expect(chart.textContent).not.toContain('data-pipeline')
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
