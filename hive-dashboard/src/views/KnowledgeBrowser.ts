import type { KnowledgeDiscoveryData } from '../domain/knowledgeDiscovery'
import type { ViewState } from './Overview'
import { renderKnowledgeDiscovery } from './KnowledgeDiscovery'

export function renderKnowledgeBrowser(state: ViewState<KnowledgeDiscoveryData>, filters: string | URLSearchParams = window.location.search, options: { onNavigate?: (path: string) => void; detailOriginPath?: string } = {}): HTMLElement {
  return renderKnowledgeDiscovery({
    mode: 'browse',
    title: 'Knowledge Browser',
    path: '/dashboard/knowledgeBrowser',
    filters,
    detailOriginPath: options.detailOriginPath,
    state,
    onFilterSubmit: options.onNavigate
  })
}
