import type { AuditEventKind, ConflictViewModel, GovernanceScreenFixturesViewModel, ManagedUserViewModel } from '../../domain/dashboard'
import { dashboardContributors } from './shared'

const auditEventTypes = ['sync_reject', 'role_change', 'deactivation', 'project_merge', 'conflict'] as const satisfies readonly AuditEventKind[]

export const userManagementFixture = {
  screen: 'userManagement',
  adminSeats: { used: 3, total: 3 },
  roleControls: ['admin', 'member', 'viewer'],
  users: dashboardContributors.map((contributor): ManagedUserViewModel => ({
    contributorId: contributor.id,
    displayName: contributor.displayName,
    email: contributor.email,
    role: contributor.role,
    status: contributor.status === 'inactive' ? 'inactive' : 'active',
    currentUser: contributor.id === 'ada-okafor'
  }))
} as const

export const auditLogFixture = {
  screen: 'auditLog',
  filters: { eventTypes: auditEventTypes },
  rows: [
    auditEvent('audit-sync-reject-1', '08 Jun 2026 · 10:42', 'sync_reject', 'agent-07', 'Rejected stale sync batch for core-api after vector clock check.'),
    auditEvent('audit-role-change-1', '08 Jun 2026 · 09:18', 'role_change', 'a.okafor', 'Changed Jun Tanaka from viewer to member.'),
    auditEvent('audit-deactivation-1', '07 Jun 2026 · 17:05', 'deactivation', 'm.lindqvist', 'Deactivated Sergei Abramov after offboarding workflow completed.'),
    auditEvent('audit-project-merge-1', '07 Jun 2026 · 14:31', 'project_merge', 'agent-11', 'Merged web-client knowledge shard into core-api analytics namespace.'),
    auditEvent('audit-conflict-1', '06 Jun 2026 · 22:47', 'conflict', 'a.okafor', 'Resolved token refresh cold-start conflict by keeping revision r3.')
  ],
  pagination: { page: 1, pageSize: 25, totalRows: 128 }
} as const

export const conflictViewerFixture = {
  screen: 'conflictViewer',
  summary: { open: 2, resolved: 1 },
  conflicts: [
    conflict(
      'conflict-retry-budget',
      'Sync retry budget policy',
      'architecture/sync-retry-budget',
      'core-api',
      'open',
      '08 Jun 2026 · 10:55',
      'agent-07',
      'r4',
      'Retry budget remains 5 attempts with jitter before sync rejection.',
      'mikael-lindqvist',
      'r3',
      'Retry budget remains 3 attempts before sync rejection.',
      [
        { kind: 'unchanged', text: 'Retry budget remains ' },
        { kind: 'removed', text: '3 attempts' },
        { kind: 'added', text: '5 attempts with jitter' }
      ]
    ),
    conflict(
      'conflict-auth-boundary',
      'Gateway auth boundary ownership',
      'architecture/auth-boundary',
      'auth-service',
      'open',
      '08 Jun 2026 · 09:07',
      'sergei-abramov',
      'r7',
      'Gateway owns token verification and services trust signed identity headers.',
      'remy-delacroix',
      'r6',
      'Services verify tokens locally before handling signed identity headers.',
      [
        { kind: 'removed', text: 'Services verify tokens locally' },
        { kind: 'added', text: 'Gateway owns token verification' },
        { kind: 'unchanged', text: ' before handling signed identity headers.' }
      ]
    ),
    conflict(
      'conflict-token-refresh',
      'Token refresh cold-start fix',
      'bugfix/token-refresh-cold-start',
      'auth-service',
      'resolved',
      '06 Jun 2026 · 22:47',
      'a.okafor',
      'r3',
      'Refresh calls are serialized during cold start to prevent duplicate rotation.',
      'agent-11',
      'r2',
      'Refresh calls retry on duplicate rotation failures during cold start.',
      [
        { kind: 'unchanged', text: 'Refresh calls ' },
        { kind: 'removed', text: 'retry on duplicate rotation failures' },
        { kind: 'added', text: 'are serialized' }
      ]
    )
  ]
} as const

export const governanceScreenFixtures = {
  userManagement: userManagementFixture,
  auditLog: auditLogFixture,
  conflictViewer: conflictViewerFixture
} as const satisfies GovernanceScreenFixturesViewModel

function auditEvent(id: string, occurredAtLabel: string, kind: AuditEventKind, actor: string, detail: string) {
  return { id, occurredAtLabel, kind, actor, detail }
}

function conflict(
  id: string,
  title: string,
  topicKey: string,
  projectId: string,
  status: ConflictViewModel['status'],
  detectedAtLabel: string,
  winningAuthorId: string,
  winningRevision: string,
  winningExcerpt: string,
  losingAuthorId: string,
  losingRevision: string,
  losingExcerpt: string,
  diffSegments: ConflictViewModel['diffSegments']
): ConflictViewModel {
  return {
    id,
    title,
    topicKey,
    projectId,
    status,
    detectedAtLabel,
    winning: { authorId: winningAuthorId, revision: winningRevision, excerpt: winningExcerpt },
    losing: { authorId: losingAuthorId, revision: losingRevision, excerpt: losingExcerpt },
    diffSegments,
    canRestoreLosingVersion: status === 'open'
  }
}
