import type { InsightsScreenFixturesViewModel } from '../../domain/dashboard'
import { dashboardContributors, dashboardProjects } from './shared'

export const analyticsFixture = {
  screen: 'analytics',
  banner: {
    title: 'Organization-wide analytics',
    subtitle: 'Memory creation, category distribution, project activity, developer contribution, and sync reliability.'
  },
  kpis: [
    { label: 'Total Memories', value: 22400, displayValue: '22.4k' },
    { label: 'Contributors', value: 8, displayValue: '8' },
    { label: 'Categories', value: 8, displayValue: '8' },
    { label: 'Peak Activity', value: 157, displayValue: '157/day' }
  ],
  activityOverTime: {
    label: 'Activity over time',
    points: [
      { label: 'Mon', value: 82 },
      { label: 'Tue', value: 96 },
      { label: 'Wed', value: 118 },
      { label: 'Thu', value: 134 },
      { label: 'Fri', value: 157 },
      { label: 'Sat', value: 141 },
      { label: 'Sun', value: 128 }
    ]
  },
  memoriesByCategory: [
    { label: 'architecture', value: 4200 },
    { label: 'bugfix', value: 3600 },
    { label: 'decision', value: 3100 },
    { label: 'discovery', value: 2800 },
    { label: 'pattern', value: 2500 },
    { label: 'config', value: 2200 },
    { label: 'preference', value: 1800 },
    { label: 'session_summary', value: 2200 }
  ],
  mostActiveProjects: dashboardProjects.slice(0, 5).map((project) => ({ label: project.name, value: project.memoryCount })),
  memoriesByDeveloper: dashboardContributors
    .filter((contributor) => contributor.kind === 'human')
    .slice(0, 8)
    .map((contributor) => ({ label: contributor.displayName, value: contributor.memoryCount })),
  syncSuccessRatio: [
    { label: 'Successful', value: 94 },
    { label: 'Rejected', value: 4 },
    { label: 'Retried', value: 2 }
  ]
} as const

export const insightsScreenFixtures = {
  analytics: analyticsFixture
} as const satisfies InsightsScreenFixturesViewModel
