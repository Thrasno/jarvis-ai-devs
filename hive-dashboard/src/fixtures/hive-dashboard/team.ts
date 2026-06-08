import type {
  ContributorPrimitiveViewModel,
  ContributorRole,
  ContributorRoleSummaryViewModel,
  ContributorSyncStatus,
  DaemonHealthSummaryViewModel,
  TeamScreenFixturesViewModel
} from '../../domain/dashboard'
import { dashboardContributors } from './shared'

export const contributorsFixture = {
  screen: 'contributors',
  roleSummary: summarizeRoles(dashboardContributors),
  cards: dashboardContributors
} as const

export const developerTimelineFixture = {
  screen: 'developerTimeline',
  selectedContributorId: 'ada-okafor',
  groups: [
    {
      dateLabel: 'Today',
      sessions: [{ id: 'ada-session-2026-06-08', title: 'Data pipeline sync recovery', projectId: 'data-pipeline', memoryIds: ['local-first-crdt-reconnect'] }]
    },
    {
      dateLabel: 'Yesterday',
      sessions: [{ id: 'ada-session-2026-06-07', title: 'Billing worker conflict review', projectId: 'billing-worker', memoryIds: ['conflict-lww-preserve-loser-8'] }]
    },
    {
      dateLabel: '05 Jun 2026',
      sessions: [{ id: 'ada-session-2026-06-05', title: 'Local-first architecture notes', projectId: 'data-pipeline', memoryIds: ['local-first-crdt-reconnect'] }]
    }
  ]
} as const

export const syncStatusFixture = {
  screen: 'syncStatus',
  daemonHealth: summarizeHealth(dashboardContributors),
  rows: dashboardContributors.map((contributor) => ({
    contributorId: contributor.id,
    contributorHandle: contributor.handle,
    kind: contributor.kind,
    projectIds: contributor.projectIds,
    health: contributor.status,
    lastSyncLabel: contributor.lastSyncLabel,
    consecutiveFailures: failuresFor(contributor.status)
  }))
} as const

export const teamScreenFixtures = {
  contributors: contributorsFixture,
  developerTimeline: developerTimelineFixture,
  syncStatus: syncStatusFixture
} as const satisfies TeamScreenFixturesViewModel

function summarizeRoles(contributors: readonly ContributorPrimitiveViewModel[]): ContributorRoleSummaryViewModel {
  return {
    admins: countBy(contributors, 'role', 'admin'),
    members: countBy(contributors, 'role', 'member'),
    viewers: countBy(contributors, 'role', 'viewer')
  }
}

function summarizeHealth(contributors: readonly ContributorPrimitiveViewModel[]): DaemonHealthSummaryViewModel {
  return {
    healthy: countBy(contributors, 'status', 'healthy'),
    total: contributors.length,
    degraded: countBy(contributors, 'status', 'degraded'),
    unknown: countBy(contributors, 'status', 'unknown'),
    inactive: countBy(contributors, 'status', 'inactive')
  }
}

function countBy<T extends 'role' | 'status'>(
  contributors: readonly ContributorPrimitiveViewModel[],
  field: T,
  value: T extends 'role' ? ContributorRole : ContributorSyncStatus
): number {
  return contributors.filter((contributor) => contributor[field] === value).length
}

function failuresFor(status: ContributorSyncStatus): number {
  if (status === 'degraded') return 2
  if (status === 'unknown') return 1
  if (status === 'inactive') return 7
  return 0
}
