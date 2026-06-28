import {
  memoryCategories,
  type ActivityFeedFixtureViewModel,
  type ExploreScreenFixturesViewModel,
  type KnowledgeBrowserFixtureViewModel,
  type KnowledgeGraphFixtureViewModel,
  type MemoryViewModel,
  type ProjectListFixtureViewModel
} from '../../domain/dashboard'
import { dashboardContributors, dashboardMemories, dashboardProjects } from './shared'
import { hiveOverviewFixture } from './overview'

const browserMemories = buildMemories(41)
const categoryCounts = countMemoriesByCategory(browserMemories)

export const projectsFixture = {
  screen: 'projects',
  totalProjects: dashboardProjects.length,
  sourceLabel: 'Demo fixture data — live project summaries are unavailable.',
  healthEvaluationDate: '2026-06-18T00:00:00.000Z',
  projects: dashboardProjects
} as const satisfies ProjectListFixtureViewModel

export const knowledgeBrowserFixture = {
  screen: 'knowledgeBrowser',
  sourceLabel: 'Fixture-backed discovery data — live facets and filters are unavailable.',
  categoryFilters: [
    { category: 'all', label: 'All', count: 41, selected: true },
    ...memoryCategories.map((category, index) => ({
      category,
      label: category.replace('_', ' '),
      count: categoryCounts[category],
      selected: false
    }))
  ],
  memories: browserMemories,
  metadata: { page: 1, pageSize: 10, totalMemories: 41, exportCount: 41 }
} as const satisfies KnowledgeBrowserFixtureViewModel

export const knowledgeGraphFixture = {
  screen: 'knowledgeGraph',
  nodes: [
    ...dashboardProjects.map((project) => ({ id: project.id, label: project.name, kind: 'project' as const })),
    ...dashboardContributors.map((contributor) => ({ id: contributor.id, label: contributor.handle, kind: 'contributor' as const })),
    ...browserMemories.slice(0, 16).map((memory) => ({ id: memory.id, label: memory.title, kind: 'memory' as const })),
    ...memoryCategories.map((category) => ({ id: `category-${category}`, label: category, kind: 'category' as const }))
  ],
  links: Array.from({ length: 33 }, (_, index) => ({
    source: dashboardProjects[index % dashboardProjects.length].id,
    target: browserMemories[index % browserMemories.length].id,
    strength: index === 0 ? 3 : (index % 3) + 1
  }))
} as const satisfies KnowledgeGraphFixtureViewModel

export const activityFeedFixture = {
  screen: 'activityFeed',
  groups: [
    { dateLabel: 'Today', entries: browserMemories.slice(0, 3).map((memory, index) => toActivity(memory, index)) },
    { dateLabel: 'Yesterday', entries: browserMemories.slice(3, 6).map((memory, index) => toActivity(memory, index + 3)) },
    { dateLabel: '05 Jun 2026', entries: browserMemories.slice(6, 9).map((memory, index) => toActivity(memory, index + 6)) }
  ]
} as const satisfies ActivityFeedFixtureViewModel

export const exploreScreenFixtures = {
  overview: hiveOverviewFixture,
  projects: projectsFixture,
  knowledgeBrowser: knowledgeBrowserFixture,
  knowledgeGraph: knowledgeGraphFixture,
  activityFeed: activityFeedFixture
} as const satisfies ExploreScreenFixturesViewModel

function buildMemories(count: number): MemoryViewModel[] {
  return Array.from({ length: count }, (_, index) => {
    const source = dashboardMemories[index % dashboardMemories.length]
    const id = index === 0 ? source.id : `${source.id}-${index + 1}`
    return { ...source, id, savedAtLabel: `${source.savedAtLabel} · item ${index + 1}` }
  })
}

function countMemoriesByCategory(memories: readonly MemoryViewModel[]): Record<MemoryViewModel['category'], number> {
  return Object.fromEntries(
    memoryCategories.map((category) => [
      category,
      memories.filter((memory) => memory.category === category).length
    ])
  ) as Record<MemoryViewModel['category'], number>
}

function toActivity(memory: MemoryViewModel, index: number) {
  const contributor = dashboardContributors[index % dashboardContributors.length]
  return {
    id: `activity-${memory.id}`,
    eventType: 'create',
    eventLabel: 'Created',
    title: memory.title,
    summary: memory.content,
    actorHandle: contributor.handle,
    projectId: memory.projectId,
    category: memory.category,
    sourceLabel: memory.category,
    timeLabel: index < 3 ? `${index + 1}m ago` : `${index + 1}h ago`,
    absoluteTimeLabel: memory.savedAtLabel,
    relativeTimeLabel: index < 3 ? `${index + 1}m ago` : `${index + 1}h ago`,
    memorySyncId: memory.id
  }
}
