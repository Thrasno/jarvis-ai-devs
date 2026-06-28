import {
  memoryCategories,
  type ContributorPrimitiveViewModel,
  type CurrentProfileViewModel,
  type MemoryViewModel,
  type NavigationGroupViewModel,
  type NotificationSummaryViewModel,
  type NotificationViewModel,
  type ProjectPrimitiveViewModel,
  type ProjectSyncStatus
} from '../../domain/dashboard'

type ContributorRole = ContributorPrimitiveViewModel['role']
type ContributorKind = ContributorPrimitiveViewModel['kind']
type ContributorStatus = ContributorPrimitiveViewModel['status']
type NotificationCategory = NotificationViewModel['category']

export const dashboardCategories = memoryCategories

export const syncHealthStatuses = ['healthy', 'degraded', 'unknown'] as const satisfies readonly ProjectSyncStatus[]

export const currentDashboardProfile = {
  initials: 'AO',
  name: 'Ada Okafor',
  email: 'ada.okafor@nexus.dev',
  role: 'admin',
  logoutLabel: 'Logout'
} as const satisfies CurrentProfileViewModel

export const dashboardNavigationGroups = [
  {
    key: 'explore',
    label: 'Explore',
    entries: [
      { screen: 'overview', label: 'Dashboard' },
      { screen: 'projects', label: 'Projects' },
      { screen: 'knowledgeBrowser', label: 'Knowledge Browser' },
      { screen: 'activityFeed', label: 'Activity Feed' }
    ]
  },
  {
    key: 'governance',
    label: 'Governance',
    entries: [
      { screen: 'userManagement', label: 'User Management' },
      { screen: 'auditLog', label: 'Audit Log' }
    ]
  }
] as const satisfies readonly NavigationGroupViewModel[]

export const dashboardProjects = [
  project('core-api', 'eu-west-1', 'healthy', 4821, 6, '2m ago', '2026-06-06T01:37:00.000Z'),
  project('auth-service', 'eu-west-1', 'healthy', 2940, 4, '6m ago', '2026-06-06T10:33:00.000Z'),
  project('billing-worker', 'us-east-1', 'degraded', 1633, 3, '38m ago', '2026-06-04T09:10:00.000Z'),
  project('web-client', 'eu-west-1', 'healthy', 3577, 5, '14m ago', '2026-06-05T11:20:00.000Z'),
  project('data-pipeline', 'us-east-1', 'healthy', 5210, 7, '1m ago', '2026-06-05T16:39:00.000Z'),
  project('mobile-sdk', 'ap-south-1', 'degraded', 1188, 2, '3h ago', '2024-06-05T20:44:00.000Z'),
  project('infra-terraform', 'eu-west-1', 'healthy', 902, 3, '51m ago', '2026-06-05T16:13:00.000Z'),
  project('search-index', 'us-east-1', 'unknown', 2104, 4, '1d ago', null)
] as const satisfies readonly ProjectPrimitiveViewModel[]

export const dashboardContributors = [
  contributor('ada-okafor', 'a.okafor', 'Ada Okafor', 'ada.okafor@nexus.dev', 'admin', 'human', 'healthy', 171, '2m ago', ['data-pipeline', 'billing-worker']),
  contributor('mikael-lindqvist', 'm.lindqvist', 'Mikael Lindqvist', 'mikael.l@nexus.dev', 'admin', 'human', 'healthy', 312, '14m ago', ['billing-worker']),
  contributor('remy-delacroix', 'r.delacroix', 'Rémy Delacroix', 'remy.d@nexus.dev', 'admin', 'human', 'healthy', 171, '1h ago', ['mobile-sdk']),
  contributor('jun-tanaka', 'j.tanaka', 'Jun Tanaka', 'jun.tanaka@nexus.dev', 'member', 'human', 'degraded', 124, '38m ago', ['billing-worker', 'mobile-sdk']),
  contributor('sergei-abramov', 's.abramov', 'Sergei Abramov', 'sergei.a@nexus.dev', 'member', 'human', 'inactive', 359, '2d ago', ['auth-service', 'core-api']),
  contributor('kwame-mensah', 'k.mensah', 'Kwame Mensah', 'kwame.m@nexus.dev', 'member', 'human', 'degraded', 406, '3h ago', ['core-api', 'infra-terraform']),
  contributor('lea-fontaine', 'l.fontaine', 'Léa Fontaine', 'lea.fontaine@nexus.dev', 'viewer', 'human', 'unknown', 453, '7h ago', ['core-api', 'data-pipeline']),
  contributor('agent-07', 'agent-07', 'agent-07', 'daemon+07@agents.nexus', 'member', 'agent', 'healthy', 77, 'just now', ['infra-terraform', 'core-api']),
  contributor('agent-11', 'agent-11', 'agent-11', 'daemon+11@agents.nexus', 'member', 'agent', 'healthy', 124, '1m ago', ['web-client'])
] as const satisfies readonly ContributorPrimitiveViewModel[]

export const dashboardMemories = [
  memory('gateway-auth-boundary', 'Gateway owns the auth boundary, not services', 'architecture', 'auth-service', 'sergei-abramov', ['security', 'tokens'], '2026-06-06T10:33:00.000Z', '06 Jun 2026 · 10:33'),
  memory('vector-store-single-writer', 'Vector store is single-writer, replicas read-only', 'architecture', 'core-api', 'agent-07', ['hnsw', 'replication', 'postgres'], '2026-06-06T01:37:00.000Z', '06 Jun 2026 · 01:37'),
  memory('redis-maxmemory-policy', 'Redis evicts under 2GB — bumped maxmemory policy', 'bugfix', 'core-api', 'sergei-abramov', ['redis', 'perf'], '2026-06-05T21:57:00.000Z', '05 Jun 2026 · 21:57'),
  memory('split-ingest-worker-gateway', 'Split monolith ingest into worker + gateway', 'architecture', 'mobile-sdk', 'jun-tanaka', ['perf', 'observability'], '2026-06-05T20:44:00.000Z', '05 Jun 2026 · 20:44'),
  memory('token-refresh-cold-start', 'Race condition in token refresh on cold start', 'bugfix', 'auth-service', 'remy-delacroix', ['tokens', 'security'], '2026-06-05T17:48:00.000Z', '05 Jun 2026 · 17:48', 3),
  memory('local-first-crdt-reconnect', 'Local-first: CRDT merge on reconnect', 'architecture', 'data-pipeline', 'ada-okafor', ['crdt', 'offline-first', 'replication'], '2026-06-05T16:39:00.000Z', '05 Jun 2026 · 16:39', 3),
  memory('vector-dimension-pinned', 'Vector dim is 1536 — do not change', 'config', 'core-api', 'kwame-mensah', ['hnsw'], '2026-06-05T16:13:00.000Z', '05 Jun 2026 · 16:13'),
  memory('conflict-lww-preserve-loser', 'Conflicts resolve last-writer-wins, never silent drop', 'decision', 'data-pipeline', 'ada-okafor', ['crdt', 'replication'], '2026-06-05T12:24:00.000Z', '05 Jun 2026 · 12:24', 3)
] as const satisfies readonly MemoryViewModel[]

export const dashboardNotificationSummary = { unread: 3, total: 7 } as const satisfies NotificationSummaryViewModel

export const dashboardNotifications = [
  notification('gateway-auth-boundary', 'Gateway owns the auth boundary, not services', 'architecture', 's.abramov', 'auth-service', 'just now', true),
  notification('vector-store-single-writer', 'Vector store is single-writer, replicas read-only', 'architecture', 'agent-07', 'core-api', '24m ago', true),
  notification('redis-maxmemory-policy', 'Redis evicts under 2GB — bumped maxmemory policy', 'bugfix', 's.abramov', 'core-api', '1h ago', true),
  notification('split-ingest-worker-gateway', 'Split monolith ingest into worker + gateway', 'architecture', 'j.tanaka', 'mobile-sdk', '2h ago', false),
  notification('token-refresh-cold-start', 'Race condition in token refresh on cold start', 'bugfix', 'r.delacroix', 'auth-service', '40m ago', false),
  notification('local-first-crdt-reconnect', 'Local-first: CRDT merge on reconnect', 'architecture', 'a.okafor', 'data-pipeline', '4h ago', false),
  notification('vector-dimension-pinned', 'Vector dim is 1536 — do not change', 'config', 'k.mensah', 'core-api', '3h ago', false)
] as const satisfies readonly NotificationViewModel[]

function project(
  name: string,
  region: string,
  status: ProjectSyncStatus,
  memoryCount: number,
  contributorCount: number,
  lastSyncLabel: string,
  lastMemoryAt: string | null
): ProjectPrimitiveViewModel {
  return { id: name, name, region, status, memoryCount, contributorCount, lastSyncLabel, lastMemoryAt }
}

function contributor(
  id: string,
  handle: string,
  displayName: string,
  email: string,
  role: ContributorRole,
  kind: ContributorKind,
  status: ContributorStatus,
  memoryCount: number,
  lastSyncLabel: string,
  projectIds: readonly string[]
): ContributorPrimitiveViewModel {
  return { id, handle, displayName, email, role, kind, status, memoryCount, lastSyncLabel, projectIds }
}

function notification(
  id: string,
  title: string,
  category: NotificationCategory,
  actorHandle: string,
  projectName: string,
  timeLabel: string,
  unread: boolean
): NotificationViewModel {
  return { id, title, category, actorHandle, projectName, timeLabel, unread }
}

function memory(
  id: string,
  title: string,
  category: MemoryViewModel['category'],
  projectId: string,
  authorId: string,
  tags: readonly string[],
  savedAt: string,
  savedAtLabel: string,
  versionCount?: number
): MemoryViewModel {
  const author = dashboardContributors.find((contributor) => contributor.id === authorId)
  return {
    id,
    title,
    category,
    projectId,
    authorId,
    authorLabel: author?.displayName ?? authorId,
    tags,
    savedAt,
    savedAtLabel,
    content: title,
    ...(versionCount === undefined ? {} : { versionCount })
  }
}
