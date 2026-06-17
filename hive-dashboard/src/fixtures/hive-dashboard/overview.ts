import type { OverviewFixtureViewModel } from '../../domain/dashboard'
import { conflictViewerFixture } from './governance'
import { dashboardContributors, dashboardProjects } from './shared'

const healthyContributors = dashboardContributors.filter((contributor) => contributor.status === 'healthy').length
const demoFixtureSource = 'Demo fixture data — live data is unavailable.'

export const hiveOverviewFixture = {
  screen: 'overview',
  totalMemories: { label: 'Total Memories', value: 22375, displayValue: '22.4k', sourceLabel: demoFixtureSource },
  activeProjects: { label: 'Active Projects', value: dashboardProjects.length, displayValue: '8', sourceLabel: demoFixtureSource },
  healthyDaemons: { label: 'Healthy Daemons', value: healthyContributors, totalValue: dashboardContributors.length, displayValue: `${healthyContributors}/${dashboardContributors.length}`, sourceLabel: 'Demo fixture data — live daemon counts are unavailable.' },
  openConflicts: { label: 'Open Conflicts', value: conflictViewerFixture.summary.open, sourceLabel: 'Demo fixture data — live conflict counts are unavailable.' },
  knowledgeGrowth: {
    label: 'Knowledge Growth',
    sourceLabel: 'Demo fixture data — live historical knowledge growth is unavailable.',
    points: [
      { label: 'Feb', value: 16720 },
      { label: 'Mar', value: 18140 },
      { label: 'Apr', value: 19680 },
      { label: 'May', value: 21110 },
      { label: 'Jun', value: 22375 }
    ]
  },
  syncHealthByProject: dashboardProjects,
  syncHealthByProjectSourceLabel: 'Demo fixture data — live per-project sync health is unavailable.',
  liveActivity: {
    summary: '3 memories saved in the last hour',
    newestMemoryId: 'gateway-auth-boundary',
    updatedAtLabel: 'Demo fixture data — live activity freshness is unavailable.'
  },
  mostActiveProjects: [
    { label: 'data-pipeline', value: 5210 },
    { label: 'core-api', value: 4821 },
    { label: 'web-client', value: 3577 },
    { label: 'auth-service', value: 2940 },
    { label: 'search-index', value: 2104 }
  ]
} as const satisfies OverviewFixtureViewModel
