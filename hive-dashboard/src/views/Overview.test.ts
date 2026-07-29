import { describe, expect, it } from 'vitest'
import { hiveOverviewFixture } from '../fixtures/hive-dashboard/overview'
import type { OverviewFixtureViewModel } from '../domain/dashboard'
import { renderOverview } from './Overview'

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

  it('renders KPI cards as native links to their destination views', () => {
    const view = renderOverview({ status: 'ready', data: hiveOverviewFixture })
    const metrics = Array.from(view.querySelectorAll<HTMLElement>('.dashboard-metric'))

    expect(metrics).toHaveLength(4)
    expect(metrics.map((metric) => ({ name: accessibleName(metric), href: metric.getAttribute('href') }))).toEqual([
      { name: 'Total Memories: 22.4k. View Knowledge Browser', href: '/dashboard/knowledgeBrowser' },
      { name: 'Active Projects: 8. View Projects', href: '/dashboard/projects' },
      { name: 'SYNCING USERS · 24H: 56% · 5/9. View User Management', href: '/dashboard/userManagement' },
      { name: 'DEGRADED PROJECTS: 2 / 5. View degraded Projects', href: '/dashboard/projects?health=degraded' }
    ])
    expect(metrics.every((metric) => metric.tagName === 'A')).toBe(true)
    expect(metrics.every((metric) => metric.querySelector('a, button, input, select, textarea') === null)).toBe(true)
  })

  it('renders sync health display as "78% · 7/9" for syncing users card', () => {
    const fixture: OverviewFixtureViewModel = {
      ...hiveOverviewFixture,
      syncingUsers: { label: 'SYNCING USERS · 24H', value: 7, displayValue: '7/9' }
    }
    const view = renderOverview({ status: 'ready', data: fixture })
    expect(getByRole(view, 'link', { name: 'SYNCING USERS · 24H: 78% · 7/9. View User Management' })).toBeDefined()
  })

  it('renders exactly 0 / 0 without a percentage for zero syncing users', () => {
    const fixture = {
      ...hiveOverviewFixture,
      syncingUsers: { label: 'SYNCING USERS · 24H', value: 0, totalValue: 0, displayValue: '0 / 0' }
    } as unknown as OverviewFixtureViewModel

    const view = renderOverview({ status: 'ready', data: fixture })

    expect(getByRole(view, 'link', { name: 'SYNCING USERS · 24H: 0 / 0. View User Management' })).toBeDefined()
  })

  it('renders degraded projects from the overview fixture contract', () => {
    const view = renderOverview({ status: 'ready', data: hiveOverviewFixture })
    expect(
      getByRole(view, 'link', {
        name: 'DEGRADED PROJECTS: 2 / 5. View degraded Projects'
      })
    ).toBeDefined()
  })

  it('renders the canonical degraded-project total instead of open conflicts', () => {
    const fixture = {
      ...hiveOverviewFixture,
      degradedProjects: { label: 'DEGRADED PROJECTS', value: 2, totalValue: 5, displayValue: '2 / 5' }
    } as unknown as OverviewFixtureViewModel

    const view = renderOverview({ status: 'ready', data: fixture })

    expect(getByRole(view, 'link', { name: 'DEGRADED PROJECTS: 2 / 5. View degraded Projects' })).toBeDefined()
    expect(view.textContent).not.toContain('Open Conflicts')
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
    expect(getByRole(view, 'link', { name: 'DEGRADED PROJECTS: 2 / 5. View degraded Projects' })).toBeDefined()
    // No descriptive summary text — polish spec removes these sentences
    expect(getByRole(view, 'figure', { name: 'Knowledge Growth' }).textContent).not.toContain('Knowledge growth over time.')
    expect(syncHealth.textContent).not.toContain('unavailable')
  })

  it('renders an accessible sync health table with associated column headers', () => {
    const view = renderOverview({ status: 'ready', data: hiveOverviewFixture })
    const syncHealth = getByRole(view, 'region', { name: 'Sync health by project' })
    const table = syncHealth.querySelector('table[aria-label="Sync health by project"]')
    const headers = Array.from(table?.querySelectorAll('th[scope="col"]') ?? []).map((header) => header.textContent)

    expect(table).not.toBeNull()
    expect(headers).toEqual(['Status', 'Project', 'Contributors', 'Last sync'])
    expect(table?.querySelectorAll('tbody tr')).toHaveLength(5)
  })

  it('orders sync health by priority and activity, caps it at five, and links to Projects', () => {
    const highProjectCountFixture: OverviewFixtureViewModel = {
      ...hiveOverviewFixture,
      syncHealthByProject: Array.from({ length: 12 }, (_, index) => {
        const project = hiveOverviewFixture.syncHealthByProject[index % hiveOverviewFixture.syncHealthByProject.length]
        return {
          ...project,
          name: `project-${index + 1}`
        }
      })
    }
    const view = renderOverview({ status: 'ready', data: highProjectCountFixture })
    const syncHealthRegion = getByRole(view, 'region', { name: 'Sync health by project' })
    const rows = Array.from(syncHealthRegion.querySelectorAll<HTMLElement>('tbody tr'))
    const footerLink = getByRole(syncHealthRegion, 'link', { name: 'View all projects' })

    expect(syncHealthRegion.classList.contains('dashboard-sync-health')).toBe(true)
    expect(rows).toHaveLength(5)
    expect(rows.map((row) => row.querySelector('.dashboard-sync-health__project')?.textContent)).toEqual([
      'project-11',
      'project-3',
      'project-6',
      'project-8',
      'project-5'
    ])
    expect(syncHealthRegion.textContent).toContain('Showing 5 of 12')
    expect(footerLink.getAttribute('href')).toBe('/dashboard/projects')
  })

  it('renders textual shared status badges and responsive row grouping hooks', () => {
    const view = renderOverview({ status: 'ready', data: hiveOverviewFixture })
    const syncHealth = getByRole(view, 'region', { name: 'Sync health by project' })
    const row = syncHealth.querySelector<HTMLElement>('tbody tr')
    const badge = row?.querySelector('[data-dashboard-primitive="status"]')

    expect(row?.classList.contains('dashboard-sync-health__row')).toBe(true)
    expect(badge?.textContent).toBe('DEGRADED')
    expect(badge?.getAttribute('data-dashboard-status')).toBe('warning')
    expect(row?.querySelector('.dashboard-sync-health__status')).not.toBeNull()
    expect(row?.querySelector('.dashboard-sync-health__project')).not.toBeNull()
    expect(row?.querySelector('.dashboard-sync-health__metric')).not.toBeNull()
    expect(row?.querySelector('.dashboard-sync-health__activity')).not.toBeNull()
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
    const syncHealth = getByRole(view, 'region', { name: 'Sync health by project' })

    const emptyRegionRow = getByRole(syncHealth, 'row', { name: 'core-api: Healthy, 6 contributors, 2m ago' })
    const whitespaceRegionRow = getByRole(syncHealth, 'row', { name: 'billing-worker: Degraded, 3 contributors, 38m ago' })

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
    expect(getByRole(view, 'link', { name: 'Total Memories: 0. View Knowledge Browser' })).toBeDefined()
    expect(getByRole(view, 'link', { name: 'Active Projects: 0. View Projects' })).toBeDefined()
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
    expect(syncHealth.querySelector('table')).toBeNull()
    expect(queryAllByRole(syncHealth, 'row')).toHaveLength(0)
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

  it('renders degraded, unknown, and healthy badges without relying on color', () => {
    const view = renderOverview({ status: 'ready', data: hiveOverviewFixture })
    const syncHealth = getByRole(view, 'region', { name: 'Sync health by project' })
    const badges = Array.from(syncHealth.querySelectorAll('[data-dashboard-primitive="status"]'))

    expect(badges.map((badge) => badge.textContent)).toEqual(['DEGRADED', 'DEGRADED', 'UNKNOWN', 'HEALTHY', 'HEALTHY'])
  })

  it('preserves the #311 containment hooks for the neighboring overview panels', () => {
    const view = renderOverview({ status: 'ready', data: hiveOverviewFixture })
    const operationsRow = view.querySelectorAll('.dashboard-overview__row')[0]
    const panels = operationsRow?.querySelectorAll('.dashboard-panel--flush')

    expect(panels).toHaveLength(2)
    expect(panels?.[0].textContent).toContain('Knowledge growth')
    expect(panels?.[1].querySelector('.dashboard-sync-health')).not.toBeNull()
    expect(panels?.[1].querySelector('.dashboard-sync-health__list')).toBeNull()
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

  it('Knowledge Growth panel has exactly one title element — no nested heading duplication', () => {
    const view = renderOverview({ status: 'ready', data: hiveOverviewFixture })
    const knowledgePanel = view.querySelector('[data-dashboard-primitive="panel"]')
    if (!knowledgePanel) throw new Error('Expected at least one panel')
    // flushPanel renders one <h2 class="dashboard-panel__title">; chart must not add a second
    const headings = Array.from(view.querySelectorAll('.dashboard-panel__title'))
    // Each panel section has exactly one .dashboard-panel__title; none inside the chart figure
    for (const panel of view.querySelectorAll('[data-dashboard-primitive="panel"]')) {
      const titlesInPanel = panel.querySelectorAll('.dashboard-panel__title')
      expect(titlesInPanel).toHaveLength(1)
      // No duplicate headings inside the chart figure nested within the panel
      const figure = panel.querySelector('[role="figure"]')
      if (figure) {
        expect(figure.querySelector('h2')).toBeNull()
      }
    }
    expect(headings.length).toBeGreaterThanOrEqual(4) // 4 panels minimum
  })

  it('Knowledge Growth chart content does not contain descriptive summary text', () => {
    const view = renderOverview({ status: 'ready', data: hiveOverviewFixture })
    const knowledgeChart = getByRole(view, 'figure', { name: 'Knowledge Growth' })
    expect(knowledgeChart.querySelector('.chart-summary')).toBeNull()
    expect(knowledgeChart.querySelector('p')).toBeNull()
    expect(knowledgeChart.textContent).not.toContain('over time')
  })

  it('Most active projects chart content does not contain descriptive summary text', () => {
    const view = renderOverview({ status: 'ready', data: hiveOverviewFixture })
    const chart = getByRole(view, 'figure', { name: 'Most active projects' })
    expect(chart.querySelector('.chart-summary')).toBeNull()
    expect(chart.querySelector('h2')).toBeNull()
    expect(chart.textContent).not.toContain('Most active projects by live memory count')
  })

  it('panel titles use dashboard-panel__title class and small mono styling hook', () => {
    const view = renderOverview({ status: 'ready', data: hiveOverviewFixture })
    const panelTitles = view.querySelectorAll('.dashboard-panel__title')
    expect(panelTitles.length).toBeGreaterThanOrEqual(4)
    panelTitles.forEach((title) => {
      expect(title.tagName).toBe('H2')
    })
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


  it('renders only the safe Member concepts and excludes hostile Admin payload fields', () => {
    const member = {
      screen: 'overview' as const,
      capability: 'member' as const,
      totalMemories: { label: 'Total Memories', value: 4 },
      activeProjects: { label: 'Active Projects', value: 1 },
      liveActivity: { count: 2 },
      mostActiveProjects: [{ label: 'jarvis-dev', value: 4 }]
    }
    const view = renderOverview({ status: 'ready', data: member })
    const metrics = getAllByRole(view, 'link')

    expect(metrics).toHaveLength(2)
    expect(metrics.map((metric) => metric.getAttribute('href'))).toEqual(['/dashboard/knowledgeBrowser', '/dashboard/projects'])
    expect(view.textContent).toContain('Total Memories')
    expect(view.textContent).toContain('Active Projects')
    expect(view.textContent).toContain('Live activity')
    expect(view.textContent).toContain('Most active projects')
    for (const forbidden of ['Healthy Daemons', 'DEGRADED PROJECTS', 'Knowledge Growth', 'Sync health', 'Newest sync', 'newest_sync_id', 'daemon_health', 'operations', 'contributor', 'actor', 'timestamp']) {
      expect(view.textContent?.toLowerCase()).not.toContain(forbidden.toLowerCase())
    }
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

function elementRole(element: HTMLElement): string | undefined {
  const explicitRole = element.getAttribute('role')?.split(' ')[0]
  if (explicitRole) return explicitRole

  if (/^H[1-6]$/.test(element.tagName)) return 'heading'
  if (element.tagName === 'A' && element.hasAttribute('href')) return 'link'
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
