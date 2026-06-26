import type { KnowledgeDiscoveryData } from '../domain/knowledgeDiscovery'
import type { ViewState } from './Overview'
import { renderKnowledgeDiscovery } from './KnowledgeDiscovery'

export function renderGlobalSearch(state: ViewState<KnowledgeDiscoveryData>, filters: string | URLSearchParams = window.location.search, options: { onNavigate?: (path: string) => void } = {}): HTMLElement {
  return renderKnowledgeDiscovery({
    mode: 'search',
    title: 'Global Search',
    path: '/dashboard/globalSearch',
    filters,
    state,
    onFilterSubmit: options.onNavigate
  })
}
