import { knowledgeBrowserFixture } from '../fixtures/hive-dashboard'
import { renderKnowledgeDiscovery } from './KnowledgeDiscovery'

export function renderKnowledgeBrowser(filters: string | URLSearchParams = window.location.search): HTMLElement {
  return renderKnowledgeDiscovery({
    mode: 'browse',
    title: 'Knowledge Browser',
    sourceLabel: knowledgeBrowserFixture.sourceLabel,
    path: '/dashboard/knowledgeBrowser',
    filters,
    memories: knowledgeBrowserFixture.memories
  })
}
