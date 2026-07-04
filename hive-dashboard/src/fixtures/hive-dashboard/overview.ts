import type { OverviewFixtureViewModel } from '../../domain/dashboard'
import { conflictViewerFixture } from './governance'
import { dashboardContributors, dashboardProjects } from './shared'

const healthyContributors = dashboardContributors.filter((contributor) => contributor.status === 'healthy').length

export const hiveOverviewFixture = {
  screen: 'overview',
  totalMemories: { label: 'Total Memories', value: 22375, displayValue: '22.4k' },
  activeProjects: { label: 'Active Projects', value: dashboardProjects.length, displayValue: '8' },
  healthyDaemons: { label: 'Healthy Daemons', value: healthyContributors, totalValue: dashboardContributors.length, displayValue: `${healthyContributors}/${dashboardContributors.length}` },
  openConflicts: { label: 'Open Conflicts', value: conflictViewerFixture.summary.open },
  knowledgeGrowth: {
    label: 'Knowledge Growth',
    points: [
      { label: 'Feb', value: 16720 },
      { label: 'Mar', value: 18140 },
      { label: 'Apr', value: 19680 },
      { label: 'May', value: 21110 },
      { label: 'Jun', value: 22375 }
    ]
  },
  syncHealthByProject: dashboardProjects.map((project) => ({
    id: project.id,
    name: project.name,
    region: project.region,
    status: project.status,
    contributorCount: project.contributorCount,
    lastActivityLabel: project.lastSyncLabel
  })),
  liveActivity: {
    count: 3,
    newestSyncId: 'sync-gateway-auth-boundary'
  },
  mostActiveProjects: [
    { label: 'data-pipeline', value: 5210 },
    { label: 'core-api', value: 4821 },
    { label: 'web-client', value: 3577 },
    { label: 'auth-service', value: 2940 },
    { label: 'search-index', value: 2104 }
  ]
} as const satisfies OverviewFixtureViewModel
