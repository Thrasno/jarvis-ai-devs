import { describe, expect, it } from 'vitest'
import {
  currentDashboardProfile,
  dashboardCategories,
  dashboardContributors,
  dashboardMemories,
  dashboardNavigationGroups,
  dashboardNotificationSummary,
  dashboardNotifications,
  dashboardProjects,
  dashboardScreenFixtures,
  exploreScreenFixtures,
  governanceScreenFixtures,
  hiveOverviewFixture,
  insightsScreenFixtures,
  teamScreenFixtures,
  syncHealthStatuses
} from './index'

describe('Hive dashboard shared fixtures', () => {
  it('exports shared fixture data through the barrel without requiring DOM or API clients', () => {
    expect(currentDashboardProfile).toMatchObject({
      name: 'Ada Okafor',
      email: 'ada.okafor@nexus.dev',
      role: 'admin'
    })
    expect(dashboardProjects.map((project) => project.name)).toContain('core-api')
    expect(dashboardContributors.map((contributor) => contributor.handle)).toContain('agent-07')
  })

  it('models only visible real-route sidebar navigation', () => {
    expect(dashboardNavigationGroups.map((group) => group.label)).toEqual([
      'Explore',
      'Governance'
    ])

    const screenLabels = dashboardNavigationGroups.flatMap((group) => group.entries.map((entry) => entry.label))

    expect(screenLabels).toEqual([
      'Dashboard',
      'Projects',
      'Knowledge Browser',
      'Activity Feed',
      'User Management',
      'Audit Log',
      'Quarantine Center'
    ])
    expect(screenLabels).toHaveLength(7)
  })

  it('preserves the PDF profile and notification summary counts', () => {
    expect(currentDashboardProfile).toMatchObject({
      initials: 'AO',
      name: 'Ada Okafor',
      role: 'admin',
      logoutLabel: 'Logout'
    })
    expect(dashboardNotificationSummary).toEqual({ unread: 3, total: 7 })
    expect(dashboardNotifications.filter((notification) => notification.unread)).toHaveLength(3)
    expect(dashboardNotifications).toHaveLength(7)
  })

  it('exports shared project, contributor, category, and sync-status primitives from the PDF', () => {
    expect(dashboardProjects).toHaveLength(8)
    expect(dashboardProjects.map((project) => [project.name, project.memoryCount, project.status])).toEqual([
      ['core-api', 4821, 'healthy'],
      ['auth-service', 2940, 'healthy'],
      ['billing-worker', 1633, 'degraded'],
      ['web-client', 3577, 'healthy'],
      ['data-pipeline', 5210, 'healthy'],
      ['mobile-sdk', 1188, 'degraded'],
      ['infra-terraform', 902, 'healthy'],
      ['search-index', 2104, 'unknown']
    ])
    expect(dashboardContributors).toHaveLength(9)
    expect(dashboardCategories).toEqual([
      'architecture',
      'bugfix',
      'decision',
      'discovery',
      'pattern',
      'config',
      'preference',
      'session_summary'
    ])
    expect(syncHealthStatuses).toEqual(['healthy', 'degraded', 'unknown'])
  })

  it('includes representative shared memories without creating screen-specific fixtures', () => {
    expect(dashboardMemories.map((memory) => [memory.title, memory.category, memory.projectId])).toEqual([
      ['Gateway owns the auth boundary, not services', 'architecture', 'auth-service'],
      ['Vector store is single-writer, replicas read-only', 'architecture', 'core-api'],
      ['Redis evicts under 2GB — bumped maxmemory policy', 'bugfix', 'core-api'],
      ['Split monolith ingest into worker + gateway', 'architecture', 'mobile-sdk'],
      ['Race condition in token refresh on cold start', 'bugfix', 'auth-service'],
      ['Local-first: CRDT merge on reconnect', 'architecture', 'data-pipeline'],
      ['Vector dim is 1536 — do not change', 'config', 'core-api'],
      ['Conflicts resolve last-writer-wins, never silent drop', 'decision', 'data-pipeline']
    ])
    expect(dashboardMemories.filter((memory) => memory.versionCount === 3)).toHaveLength(3)
  })
})

describe('Hive dashboard Explore fixtures', () => {
  it('covers the five visible Explore screens with PDF representative values', () => {
    expect(Object.keys(exploreScreenFixtures)).toEqual([
      'overview',
      'projects',
      'knowledgeBrowser',
      'knowledgeGraph',
      'activityFeed'
    ])

    expect(hiveOverviewFixture.totalMemories).toEqual({
      label: 'Total Memories',
      value: 22375,
      displayValue: '22.4k'
    })
    expect(hiveOverviewFixture.liveActivity).toEqual({ count: 3, newestSyncId: 'sync-gateway-auth-boundary' })
    expect(exploreScreenFixtures.projects.projects).toHaveLength(8)
    expect(exploreScreenFixtures.projects.healthEvaluationDate).toBe('2026-06-18T00:00:00.000Z')
    expect(exploreScreenFixtures.knowledgeBrowser.memories).toHaveLength(41)
    expect(exploreScreenFixtures.knowledgeBrowser.metadata.exportCount).toBe(41)
    expect(exploreScreenFixtures.knowledgeGraph.nodes).toHaveLength(41)
    expect(exploreScreenFixtures.knowledgeGraph.links).toHaveLength(33)
    expect(exploreScreenFixtures.activityFeed.groups).toHaveLength(3)
    expect('livePolling' in exploreScreenFixtures.activityFeed).toBe(false)
  })

  it('labels Projects as non-live fixture data and exposes health derivation inputs', () => {
    expect(exploreScreenFixtures.projects.sourceLabel).toBe('Demo fixture data — live project summaries are unavailable.')
    expect(exploreScreenFixtures.projects.sourceLabel.toLowerCase()).not.toContain('live production')
    expect(exploreScreenFixtures.projects.projects.map((project) => [project.name, project.lastMemoryAt])).toEqual([
      ['core-api', '2026-06-06T01:37:00.000Z'],
      ['auth-service', '2026-06-06T10:33:00.000Z'],
      ['billing-worker', '2026-06-04T09:10:00.000Z'],
      ['web-client', '2026-06-05T11:20:00.000Z'],
      ['data-pipeline', '2026-06-05T16:39:00.000Z'],
      ['mobile-sdk', '2024-06-05T20:44:00.000Z'],
      ['infra-terraform', '2026-06-05T16:13:00.000Z'],
      ['search-index', null]
    ])
  })

  it('labels discovery fixtures as source-limited instead of live API-backed', () => {
    expect(exploreScreenFixtures.knowledgeBrowser.sourceLabel).toBe('Fixture-backed discovery data — live facets and filters are unavailable.')
    expect(exploreScreenFixtures.knowledgeBrowser.sourceLabel.toLowerCase()).not.toContain('live production')
  })

  it('exposes filterable discovery authors, dates, tags, and projects on browser memories', () => {
    expect(exploreScreenFixtures.knowledgeBrowser.memories.slice(0, 3).map((memory) => ({
      authorId: memory.authorId,
      authorLabel: memory.authorLabel,
      projectId: memory.projectId,
      savedAt: memory.savedAt,
      tags: memory.tags
    }))).toEqual([
      {
        authorId: 'sergei-abramov',
        authorLabel: 'Sergei Abramov',
        projectId: 'auth-service',
        savedAt: '2026-06-06T10:33:00.000Z',
        tags: ['security', 'tokens']
      },
      {
        authorId: 'agent-07',
        authorLabel: 'agent-07',
        projectId: 'core-api',
        savedAt: '2026-06-06T01:37:00.000Z',
        tags: ['hnsw', 'replication', 'postgres']
      },
      {
        authorId: 'sergei-abramov',
        authorLabel: 'Sergei Abramov',
        projectId: 'core-api',
        savedAt: '2026-06-05T21:57:00.000Z',
        tags: ['redis', 'perf']
      }
    ])
  })

  it('keeps Knowledge Browser category facet counts aligned with filterable memories', () => {
    const { categoryFilters, memories } = exploreScreenFixtures.knowledgeBrowser

    for (const filter of categoryFilters) {
      const matchingMemories = filter.category === 'all'
        ? memories
        : memories.filter((memory) => memory.category === filter.category)

      expect(filter.count).toBe(matchingMemories.length)
      if (filter.count > 0) expect(matchingMemories.length).toBeGreaterThan(0)
    }
  })

  it('keeps Explore datasets and graph data as plain render-agnostic arrays', () => {
    expect(Array.isArray(hiveOverviewFixture.knowledgeGrowth.points)).toBe(true)
    expect(hiveOverviewFixture.knowledgeGrowth.points.map((point) => point.value)).toEqual([16720, 18140, 19680, 21110, 22375])
    expect(Array.isArray(hiveOverviewFixture.syncHealthByProject)).toBe(true)
    expect(Array.isArray(exploreScreenFixtures.knowledgeBrowser.categoryFilters)).toBe(true)
    expect(Array.isArray(exploreScreenFixtures.knowledgeGraph.nodes)).toBe(true)
    expect(Array.isArray(exploreScreenFixtures.knowledgeGraph.links)).toBe(true)
    expect(exploreScreenFixtures.knowledgeGraph.nodes[0]).toEqual({ id: 'core-api', label: 'core-api', kind: 'project' })
    expect(exploreScreenFixtures.knowledgeGraph.links[0]).toEqual({ source: 'core-api', target: 'gateway-auth-boundary', strength: 3 })
  })
})

describe('Hive dashboard Team fixtures', () => {
  it('covers the three Team screens with shared contributors and role counts', () => {
    expect(Object.keys(teamScreenFixtures)).toEqual(['contributors', 'developerTimeline', 'syncStatus'])

    expect(teamScreenFixtures.contributors.roleSummary).toEqual({ admins: 3, members: 5, viewers: 1 })
    expect(teamScreenFixtures.contributors.cards).toHaveLength(9)
    expect(teamScreenFixtures.developerTimeline.selectedContributorId).toBe('ada-okafor')
    expect(teamScreenFixtures.developerTimeline.groups.map((group) => group.dateLabel)).toEqual([
      'Today',
      'Yesterday',
      '05 Jun 2026'
    ])
    expect(teamScreenFixtures.syncStatus.daemonHealth).toEqual({ healthy: 5, total: 9, degraded: 2, unknown: 1, inactive: 1 })
    expect(teamScreenFixtures.syncStatus.rows).toHaveLength(9)
  })

  it('models sync status rows without backend or rendering dependencies', () => {
    expect(Array.isArray(teamScreenFixtures.syncStatus.rows)).toBe(true)
    expect(teamScreenFixtures.syncStatus.rows.map((row) => [row.contributorHandle, row.health])).toEqual([
      ['a.okafor', 'healthy'],
      ['m.lindqvist', 'healthy'],
      ['r.delacroix', 'healthy'],
      ['j.tanaka', 'degraded'],
      ['s.abramov', 'inactive'],
      ['k.mensah', 'degraded'],
      ['l.fontaine', 'unknown'],
      ['agent-07', 'healthy'],
      ['agent-11', 'healthy']
    ])
  })

  it('links every developer timeline memory id to a shared or generated fixture memory', () => {
    const knownMemoryIds = new Set([
      ...dashboardMemories.map((memory) => memory.id),
      ...exploreScreenFixtures.knowledgeBrowser.memories.map((memory) => memory.id)
    ])
    const timelineMemoryIds = teamScreenFixtures.developerTimeline.groups.flatMap((group) =>
      group.sessions.flatMap((session) => session.memoryIds)
    )

    expect(timelineMemoryIds).toEqual([
      'local-first-crdt-reconnect',
      'conflict-lww-preserve-loser-8',
      'local-first-crdt-reconnect'
    ])
    expect(timelineMemoryIds.filter((memoryId) => !knownMemoryIds.has(memoryId))).toEqual([])
  })
})

describe('Hive dashboard Insights fixtures', () => {
  it('covers the Analytics screen with representative PDF metrics', () => {
    expect(Object.keys(insightsScreenFixtures)).toEqual(['analytics'])

    expect(insightsScreenFixtures.analytics.banner).toEqual({
      title: 'Organization-wide analytics',
      subtitle: 'Memory creation, category distribution, project activity, developer contribution, and sync reliability.'
    })
    expect(insightsScreenFixtures.analytics.kpis).toEqual([
      { label: 'Total Memories', value: 22400, displayValue: '22.4k' },
      { label: 'Contributors', value: 8, displayValue: '8' },
      { label: 'Categories', value: 8, displayValue: '8' },
      { label: 'Peak Activity', value: 157, displayValue: '157/day' }
    ])
  })

  it('keeps Analytics chart datasets as plain typed arrays', () => {
    expect(Array.isArray(insightsScreenFixtures.analytics.activityOverTime.points)).toBe(true)
    expect(insightsScreenFixtures.analytics.activityOverTime.points.map((point) => point.value)).toEqual([
      82, 96, 118, 134, 157, 141, 128
    ])
    expect(insightsScreenFixtures.analytics.memoriesByCategory.map((point) => [point.label, point.value])).toEqual([
      ['architecture', 4200],
      ['bugfix', 3600],
      ['decision', 3100],
      ['discovery', 2800],
      ['pattern', 2500],
      ['config', 2200],
      ['preference', 1800],
      ['session_summary', 2200]
    ])
    expect(insightsScreenFixtures.analytics.syncSuccessRatio.map((point) => [point.label, point.value])).toEqual([
      ['Successful', 94],
      ['Rejected', 4],
      ['Retried', 2]
    ])
  })
})

describe('Hive dashboard Governance fixtures', () => {
  it('covers User Management with admin seat usage, current user marker, role controls, and statuses', () => {
    expect(Object.keys(governanceScreenFixtures)).toEqual(['userManagement', 'auditLog', 'conflictViewer'])

    expect(governanceScreenFixtures.userManagement.adminSeats).toEqual({ used: 3, total: 3 })
    expect(governanceScreenFixtures.userManagement.users.map((user) => [user.displayName, user.role, user.status, user.currentUser])).toContainEqual([
      'Ada Okafor',
      'admin',
      'active',
      true
    ])
    expect(governanceScreenFixtures.userManagement.roleControls).toEqual(['admin', 'member', 'viewer'])
    expect(governanceScreenFixtures.userManagement.users.map((user) => user.status)).toContain('inactive')
  })

  it('covers Audit Log event types and large-list pagination metadata', () => {
    expect(governanceScreenFixtures.auditLog.filters.eventTypes).toEqual([
      'sync_reject',
      'role_change',
      'deactivation',
      'project_merge',
      'conflict'
    ])
    expect(governanceScreenFixtures.auditLog.rows.map((row) => row.kind)).toEqual([
      'sync_reject',
      'role_change',
      'deactivation',
      'project_merge',
      'conflict'
    ])
    expect(governanceScreenFixtures.auditLog.rows[0]).toMatchObject({
      occurredAtLabel: '08 Jun 2026 · 10:42',
      actor: 'agent-07',
      detail: 'Rejected stale sync batch for core-api after vector clock check.'
    })
    expect(governanceScreenFixtures.auditLog.pagination).toEqual({ page: 1, pageSize: 25, totalRows: 128 })
  })

  it('covers Conflict Viewer status counts, winning/losing versions, diff segments, and restore availability', () => {
    expect(governanceScreenFixtures.conflictViewer.summary).toEqual({ open: 2, resolved: 1 })
    expect(governanceScreenFixtures.conflictViewer.conflicts.map((conflict) => conflict.status)).toEqual(['open', 'open', 'resolved'])
    expect(governanceScreenFixtures.conflictViewer.conflicts.map((conflict) => conflict.canRestoreLosingVersion)).toEqual([
      true,
      true,
      false
    ])
    expect(governanceScreenFixtures.conflictViewer.conflicts[0].diffSegments).toEqual([
      { kind: 'unchanged', text: 'Retry budget remains ' },
      { kind: 'removed', text: '3 attempts' },
      { kind: 'added', text: '5 attempts with jitter' }
    ])
  })
})

describe('Hive dashboard final screen fixture registry', () => {
  it('keeps hidden placeholder fixture screens out of visible navigation', () => {
    const navScreenIds = dashboardNavigationGroups.flatMap((group) => group.entries.map((entry) => entry.screen))

    expect(navScreenIds).toEqual([
      'overview',
      'projects',
      'knowledgeBrowser',
      'activityFeed',
      'userManagement',
      'auditLog',
      'quarantines'
    ])
    expect(Object.keys(dashboardScreenFixtures)).toContain('analytics')
    expect(Object.keys(dashboardScreenFixtures)).toContain('conflictViewer')
    expect(navScreenIds).not.toContain('analytics')
    expect(navScreenIds).not.toContain('conflictViewer')
    expect(dashboardScreenFixtures.analytics).toBe(insightsScreenFixtures.analytics)
    expect(dashboardScreenFixtures.conflictViewer).toBe(governanceScreenFixtures.conflictViewer)
  })
})
