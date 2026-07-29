import type { ProjectListResponse, ProjectSummary } from '../api/client'

export const dashboardScreenKeys = [
  'overview',
  'memories',
  'projects',
  'knowledgeBrowser',
  'knowledgeGraph',
  'activityFeed',
  'contributors',
  'developerTimeline',
  'syncStatus',
  'analytics',
  'userManagement',
  'auditLog',
  'account',
  'conflictViewer'
] as const

export type DashboardScreenKey = typeof dashboardScreenKeys[number]
export type NavigationGroupKey = 'explore' | 'team' | 'insights' | 'governance'
export type MemoryCategory = typeof memoryCategories[number]
export type ProjectSyncStatus = 'healthy' | 'degraded' | 'unknown'
export type ContributorSyncStatus = ProjectSyncStatus | 'inactive'
export type ContributorRole = 'admin' | 'member' | 'viewer'
export type ActorKind = 'human' | 'agent'
export type AuditEventKind = 'sync_reject' | 'role_change' | 'deactivation' | 'project_merge' | 'conflict'
export type ConflictStatus = 'open' | 'resolved'

export const memoryCategories = [
  'architecture',
  'bugfix',
  'decision',
  'discovery',
  'pattern',
  'config',
  'preference',
  'session_summary'
] as const

export type NavigationEntryViewModel = {
  readonly screen: DashboardScreenKey
  readonly label: string
}

export type NavigationGroupViewModel = {
  readonly key: NavigationGroupKey
  readonly label: string
  readonly entries: readonly NavigationEntryViewModel[]
}

export type CurrentProfileViewModel = {
  readonly initials: string
  readonly name: string
  readonly email: string
  readonly role: ContributorRole
  readonly logoutLabel: string
}

export type ProjectPrimitiveViewModel = {
  readonly id: string
  readonly name: string
  readonly region: string
  readonly status: ProjectSyncStatus
  readonly memoryCount: number
  readonly contributorCount: number
  readonly lastSyncLabel: string
  readonly lastMemoryAt?: string | null
}

export type ContributorPrimitiveViewModel = {
  readonly id: string
  readonly handle: string
  readonly displayName: string
  readonly email: string
  readonly role: ContributorRole
  readonly kind: ActorKind
  readonly status: ContributorSyncStatus
  readonly memoryCount: number
  readonly lastSyncLabel: string
  readonly projectIds: readonly string[]
}

export type MemoryViewModel = {
  readonly id: string
  readonly title: string
  readonly content: string
  readonly category: MemoryCategory
  readonly projectId: string
  readonly authorId: string
  readonly authorLabel: string
  readonly tags: readonly string[]
  readonly savedAt: string
  readonly savedAtLabel: string
  readonly versionCount?: number
}

export type NotificationViewModel = {
  readonly id: string
  readonly title: string
  readonly category: MemoryCategory
  readonly actorHandle: string
  readonly projectName: string
  readonly timeLabel: string
  readonly unread: boolean
}

export type NotificationSummaryViewModel = {
  readonly unread: number
  readonly total: number
}

export type SyncDaemonViewModel = {
  readonly contributorId: string
  readonly projectId: string
  readonly health: ProjectSyncStatus
  readonly lastSyncLabel: string
  readonly consecutiveFailures: number
  readonly ratePerMinute: number | null
}

export type ChartPointViewModel = {
  readonly label: string
  readonly value: number
}

export type ChartSeriesViewModel = {
  readonly label: string
  readonly points: readonly ChartPointViewModel[]
  readonly sourceLabel?: string
}

export type MetricCardViewModel = {
  readonly label: string
  readonly value: number
  readonly totalValue?: number
  readonly displayValue?: string
  readonly trendLabel?: string
  readonly sourceLabel?: string
}

export type OverviewSyncHealthProjectViewModel = {
  readonly id: string
  readonly name: string
  readonly region: string
  readonly status: ProjectSyncStatus
  readonly contributorCount: number
  readonly lastActivityLabel: string
}

export type OverviewLiveActivityViewModel = {
  readonly count: number
  readonly newestSyncId?: string
}

export type OverviewCommonViewModel = {
  readonly screen: 'overview'
  readonly totalMemories: MetricCardViewModel
  readonly activeProjects: MetricCardViewModel
  readonly liveActivity: OverviewLiveActivityViewModel
  readonly mostActiveProjects: readonly ChartPointViewModel[]
}

export type MemberOverviewViewModel = OverviewCommonViewModel & {
  readonly capability: 'member'
}

export type AdminOverviewViewModel = OverviewCommonViewModel & {
  readonly capability: 'admin'
  readonly healthyDaemons: MetricCardViewModel
  readonly degradedProjects: MetricCardViewModel
  readonly knowledgeGrowth: ChartSeriesViewModel
  readonly syncHealthByProject: readonly OverviewSyncHealthProjectViewModel[]
  readonly syncHealthByProjectSourceLabel?: string
  readonly liveActivity: OverviewLiveActivityViewModel & { readonly newestSyncId: string }
}

export type OverviewViewModel = MemberOverviewViewModel | AdminOverviewViewModel
export type OverviewFixtureViewModel = AdminOverviewViewModel

export type ProjectListFixtureViewModel = {
  readonly screen: 'projects'
  readonly totalProjects: number
  readonly sourceLabel?: string
  readonly healthEvaluationDate: string
  readonly projects: readonly ProjectPrimitiveViewModel[]
}

export type ProjectLiveSummaryViewModel = {
  readonly name: string
  readonly memoryCount: number
  readonly sessionCount: number
  readonly lastActivityLabel: string
  readonly syncHealth: ProjectSyncStatus
  readonly browsePath: string
  readonly blocked: boolean
  readonly canonicalProjectKey: string
  readonly blockReason?: string
  readonly exportMarker?: string
  readonly blockAckStatus?: string
}

export type ProjectListViewModel = {
  readonly screen: 'projects'
  readonly totalProjects: number
  readonly projects: readonly ProjectLiveSummaryViewModel[]
}

export type CategoryFilterViewModel = {
  readonly category: MemoryCategory | 'all'
  readonly label: string
  readonly count: number
  readonly selected: boolean
}

export type BrowserPageMetadataViewModel = {
  readonly page: number
  readonly pageSize: number
  readonly totalMemories: number
  readonly exportCount: number
}

export type KnowledgeBrowserFixtureViewModel = {
  readonly screen: 'knowledgeBrowser'
  readonly sourceLabel: string
  readonly categoryFilters: readonly CategoryFilterViewModel[]
  readonly memories: readonly MemoryViewModel[]
  readonly metadata: BrowserPageMetadataViewModel
}

export type KnowledgeGraphNodeKind = 'project' | 'contributor' | 'memory' | 'category'

export type KnowledgeGraphNodeViewModel = {
  readonly id: string
  readonly label: string
  readonly kind: KnowledgeGraphNodeKind
}

export type KnowledgeGraphLinkViewModel = {
  readonly source: string
  readonly target: string
  readonly strength: number
}

export type KnowledgeGraphFixtureViewModel = {
  readonly screen: 'knowledgeGraph'
  readonly nodes: readonly KnowledgeGraphNodeViewModel[]
  readonly links: readonly KnowledgeGraphLinkViewModel[]
}

export type ActivityEntryViewModel = {
  readonly id: string
  readonly eventType: string
  readonly eventLabel: string
  readonly title: string
  readonly summary: string
  readonly actorHandle: string
  readonly projectId: string
  readonly category: MemoryCategory
  readonly sourceLabel: string
  readonly timeLabel: string
  readonly absoluteTimeLabel: string
  readonly relativeTimeLabel: string
  readonly memorySyncId?: string
}

export type ActivityGroupViewModel = {
  readonly dateLabel: string
  readonly entries: readonly ActivityEntryViewModel[]
}

export type ActivityFeedViewModel = {
  readonly screen: 'activityFeed'
  readonly groups: readonly ActivityGroupViewModel[]
  readonly nextCursor?: string
  readonly loadingMore?: boolean
  readonly paginationError?: string
}

export type ActivityFeedFixtureViewModel = ActivityFeedViewModel

export type ExploreScreenFixturesViewModel = {
  readonly overview: OverviewFixtureViewModel
  readonly projects: ProjectListFixtureViewModel
  readonly knowledgeBrowser: KnowledgeBrowserFixtureViewModel
  readonly knowledgeGraph: KnowledgeGraphFixtureViewModel
  readonly activityFeed: ActivityFeedFixtureViewModel
}

export type ContributorRoleSummaryViewModel = {
  readonly admins: number
  readonly members: number
  readonly viewers: number
}

export type ContributorsFixtureViewModel = {
  readonly screen: 'contributors'
  readonly roleSummary: ContributorRoleSummaryViewModel
  readonly cards: readonly ContributorPrimitiveViewModel[]
}

export type TimelineSessionViewModel = {
  readonly id: string
  readonly title: string
  readonly projectId: string
  readonly memoryIds: readonly string[]
}

export type TimelineGroupViewModel = {
  readonly dateLabel: string
  readonly sessions: readonly TimelineSessionViewModel[]
}

export type DeveloperTimelineFixtureViewModel = {
  readonly screen: 'developerTimeline'
  readonly selectedContributorId: string
  readonly groups: readonly TimelineGroupViewModel[]
}

export type DaemonHealthSummaryViewModel = {
  readonly healthy: number
  readonly total: number
  readonly degraded: number
  readonly unknown: number
  readonly inactive: number
}

export type SyncStatusRowViewModel = {
  readonly contributorId: string
  readonly contributorHandle: string
  readonly kind: ActorKind
  readonly projectIds: readonly string[]
  readonly health: ContributorSyncStatus
  readonly lastSyncLabel: string
  readonly consecutiveFailures: number
}

export type SyncStatusFixtureViewModel = {
  readonly screen: 'syncStatus'
  readonly daemonHealth: DaemonHealthSummaryViewModel
  readonly rows: readonly SyncStatusRowViewModel[]
}

export type TeamScreenFixturesViewModel = {
  readonly contributors: ContributorsFixtureViewModel
  readonly developerTimeline: DeveloperTimelineFixtureViewModel
  readonly syncStatus: SyncStatusFixtureViewModel
}

export type AuditEventViewModel = {
  readonly id: string
  readonly occurredAtLabel: string
  readonly kind: AuditEventKind
  readonly actor: string
  readonly detail: string
}

export type AnalyticsFixtureViewModel = {
  readonly screen: 'analytics'
  readonly banner: {
    readonly title: string
    readonly subtitle: string
  }
  readonly kpis: readonly MetricCardViewModel[]
  readonly activityOverTime: ChartSeriesViewModel
  readonly memoriesByCategory: readonly ChartPointViewModel[]
  readonly mostActiveProjects: readonly ChartPointViewModel[]
  readonly memoriesByDeveloper: readonly ChartPointViewModel[]
  readonly syncSuccessRatio: readonly ChartPointViewModel[]
}

export type InsightsScreenFixturesViewModel = {
  readonly analytics: AnalyticsFixtureViewModel
}

export type ManagedUserStatus = 'active' | 'inactive'

export type ManagedUserViewModel = {
  readonly contributorId: string
  readonly displayName: string
  readonly email: string
  readonly role: ContributorRole
  readonly status: ManagedUserStatus
  readonly currentUser: boolean
}

export type UserManagementFixtureViewModel = {
  readonly screen: 'userManagement'
  readonly adminSeats: {
    readonly used: number
    readonly total: number
  }
  readonly roleControls: readonly ContributorRole[]
  readonly users: readonly ManagedUserViewModel[]
}

export type AuditLogFixtureViewModel = {
  readonly screen: 'auditLog'
  readonly filters: {
    readonly eventTypes: readonly AuditEventKind[]
  }
  readonly rows: readonly AuditEventViewModel[]
  readonly pagination: {
    readonly page: number
    readonly pageSize: number
    readonly totalRows: number
  }
}

export type ConflictVersionViewModel = {
  readonly authorId: string
  readonly revision: string
  readonly excerpt: string
}

export type ConflictDiffSegmentViewModel = {
  readonly kind: 'unchanged' | 'added' | 'removed'
  readonly text: string
}

export type ConflictViewModel = {
  readonly id: string
  readonly title: string
  readonly topicKey: string
  readonly projectId: string
  readonly status: ConflictStatus
  readonly detectedAtLabel: string
  readonly winning: ConflictVersionViewModel
  readonly losing: ConflictVersionViewModel
  readonly diffSegments: readonly ConflictDiffSegmentViewModel[]
  readonly canRestoreLosingVersion: boolean
}

export type ConflictViewerFixtureViewModel = {
  readonly screen: 'conflictViewer'
  readonly summary: {
    readonly open: number
    readonly resolved: number
  }
  readonly conflicts: readonly ConflictViewModel[]
}

export type GovernanceScreenFixturesViewModel = {
  readonly userManagement: UserManagementFixtureViewModel
  readonly auditLog: AuditLogFixtureViewModel
  readonly conflictViewer: ConflictViewerFixtureViewModel
}

export type DashboardScreenFixturesViewModel = ExploreScreenFixturesViewModel &
  TeamScreenFixturesViewModel &
  InsightsScreenFixturesViewModel &
  GovernanceScreenFixturesViewModel

export type DashboardFixturesViewModel = {
  readonly shared: {
    readonly profile: CurrentProfileViewModel
    readonly navigationGroups: readonly NavigationGroupViewModel[]
    readonly notificationSummary: NotificationSummaryViewModel
    readonly notifications: readonly NotificationViewModel[]
    readonly projects: readonly ProjectPrimitiveViewModel[]
    readonly contributors: readonly ContributorPrimitiveViewModel[]
    readonly memories: readonly MemoryViewModel[]
  }
  readonly screens: DashboardScreenFixturesViewModel
}

export function projectsFromApi(response: ProjectListResponse): ProjectListViewModel {
  return {
    screen: 'projects',
    totalProjects: response.total,
    projects: response.projects.map(projectFromApi)
  }
}

function projectFromApi(project: ProjectSummary): ProjectLiveSummaryViewModel {
  return {
    name: project.name,
    memoryCount: project.memoryCount,
    sessionCount: project.sessionCount,
    lastActivityLabel: lastActivityLabel(project.lastActivityAt),
    syncHealth: normalizedProjectSyncHealth(project.syncHealth),
    browsePath: `/dashboard/knowledgeBrowser?${new URLSearchParams({ project: project.name }).toString()}`,
    blocked: project.blocked === true,
    canonicalProjectKey: project.canonicalProjectKey?.trim() || project.name,
    blockReason: project.blockReason?.trim() || undefined,
    exportMarker: project.exportMarker?.trim() || undefined,
    blockAckStatus: project.blockAckStatus?.trim() || undefined
  }
}

function normalizedProjectSyncHealth(status: ProjectSummary['syncHealth']): ProjectSyncStatus {
  return status === 'healthy' || status === 'degraded' || status === 'unknown' ? status : 'unknown'
}

function lastActivityLabel(value: string | null | undefined): string {
  if (!value) return 'Last activity unavailable'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return 'Last activity unavailable'
  const formatted = new Intl.DateTimeFormat('en-GB', {
    day: '2-digit',
    month: '2-digit',
    year: '2-digit',
    timeZone: 'UTC'
  }).format(date)
  return `Last activity: ${formatted}`
}

export function relativeActivityAgeLabel(value: string | null | undefined, now = new Date()): string {
  if (!value) return 'activity unavailable'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return 'activity unavailable'

  const elapsedMs = Math.max(0, now.getTime() - date.getTime())
  const elapsedMinutes = Math.floor(elapsedMs / 60000)
  if (elapsedMinutes < 1) return 'just now'
  if (elapsedMinutes < 60) return `${elapsedMinutes}m ago`

  const elapsedHours = Math.floor(elapsedMinutes / 60)
  if (elapsedHours < 24) return `${elapsedHours}h ago`

  const elapsedDays = Math.floor(elapsedHours / 24)
  return `${elapsedDays}d ago`
}
