import type { DashboardFixturesViewModel, DashboardScreenFixturesViewModel } from '../../domain/dashboard'
import { exploreScreenFixtures } from './explore'
import { governanceScreenFixtures } from './governance'
import { insightsScreenFixtures } from './insights'
import {
  currentDashboardProfile,
  dashboardContributors,
  dashboardMemories,
  dashboardNavigationGroups,
  dashboardNotificationSummary,
  dashboardNotifications,
  dashboardProjects
} from './shared'
import { teamScreenFixtures } from './team'

export * from '../../domain/dashboard'
export * from './explore'
export * from './governance'
export * from './insights'
export * from './overview'
export * from './shared'
export * from './team'

export const dashboardScreenFixtures = {
  ...exploreScreenFixtures,
  ...teamScreenFixtures,
  ...insightsScreenFixtures,
  ...governanceScreenFixtures
} as const satisfies DashboardScreenFixturesViewModel

export const dashboardFixtures = {
  shared: {
    profile: currentDashboardProfile,
    navigationGroups: dashboardNavigationGroups,
    notificationSummary: dashboardNotificationSummary,
    notifications: dashboardNotifications,
    projects: dashboardProjects,
    contributors: dashboardContributors,
    memories: dashboardMemories
  },
  screens: dashboardScreenFixtures
} as const satisfies DashboardFixturesViewModel
