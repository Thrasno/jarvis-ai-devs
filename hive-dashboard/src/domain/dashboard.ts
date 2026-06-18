export const dashboardScreenKeys = [
  'overview',
  'memories',
  'projects',
  'knowledgeBrowser',
  'globalSearch',
  'knowledgeGraph',
  'activityFeed',
  'contributors',
  'developerTimeline',
  'syncStatus',
  'analytics',
  'userManagement',
  'auditLog',
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

export type OverviewFixtureViewModel = {
  readonly screen: 'overview'
  readonly totalMemories: MetricCardViewModel
  readonly activeProjects: MetricCardViewModel
  readonly healthyDaemons: MetricCardViewModel
  readonly openConflicts: MetricCardViewModel
  readonly knowledgeGrowth: ChartSeriesViewModel
  readonly syncHealthByProject: readonly ProjectPrimitiveViewModel[]
  readonly syncHealthByProjectSourceLabel?: string
  readonly liveActivity: {
    readonly summary: string
    readonly newestMemoryId: string
    readonly updatedAtLabel: string
  }
  readonly mostActiveProjects: readonly ChartPointViewModel[]
}

export type ProjectListFixtureViewModel = {
  readonly screen: 'projects'
  readonly totalProjects: number
  readonly sourceLabel?: string
  readonly healthEvaluationDate: string
  readonly projects: readonly ProjectPrimitiveViewModel[]
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

export type SearchResultViewModel = {
  readonly memoryId: string
  readonly title: string
  readonly excerpt: string
  readonly category: MemoryCategory
  readonly projectId: string
  readonly authorId: string
  readonly authorLabel: string
  readonly tags: readonly string[]
  readonly savedAt: string
  readonly savedAtLabel: string
  readonly highlights: readonly string[]
  readonly score: number
}

export type GlobalSearchFixtureViewModel = {
  readonly screen: 'globalSearch'
  readonly sourceLabel: string
  readonly query: string
  readonly highlights: readonly string[]
  readonly results: readonly SearchResultViewModel[]
  readonly metadata: {
    readonly shareLabel: string
    readonly clearLabel: string
    readonly exportCount: number
  }
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
  readonly title: string
  readonly actorHandle: string
  readonly projectId: string
  readonly category: MemoryCategory
  readonly timeLabel: string
}

export type ActivityGroupViewModel = {
  readonly dateLabel: string
  readonly entries: readonly ActivityEntryViewModel[]
}

export type ActivityFeedFixtureViewModel = {
  readonly screen: 'activityFeed'
  readonly groups: readonly ActivityGroupViewModel[]
  readonly livePolling: {
    readonly enabled: boolean
    readonly intervalSeconds: number
  }
}

export type ExploreScreenFixturesViewModel = {
  readonly overview: OverviewFixtureViewModel
  readonly projects: ProjectListFixtureViewModel
  readonly knowledgeBrowser: KnowledgeBrowserFixtureViewModel
  readonly globalSearch: GlobalSearchFixtureViewModel
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
