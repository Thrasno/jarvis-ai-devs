import { globalSearchFixture } from '../fixtures/hive-dashboard'
import { renderKnowledgeDiscovery } from './KnowledgeDiscovery'

export function renderGlobalSearch(filters: string | URLSearchParams = window.location.search): HTMLElement {
  return renderKnowledgeDiscovery({
    mode: 'search',
    title: 'Global Search',
    sourceLabel: globalSearchFixture.sourceLabel,
    path: '/dashboard/globalSearch',
    filters,
    memories: globalSearchFixture.results
  })
}
