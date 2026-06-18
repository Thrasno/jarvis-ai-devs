import type { Memory, MemoryList, MemorySearch } from '../api/client'
import { append, error, list, panel, text } from '../components/dom'
import type { ViewState } from './Overview'

export function renderMemories(state: ViewState<{ recent: MemoryList; search: MemorySearch }>, options: { detailId?: string | null } = {}): HTMLElement {
  const card = panel('Memories')
  if (options.detailId) {
    const unavailable = text('Memory detail is unavailable in this fixture-backed dashboard slice.', 'dashboard-state state')
    unavailable.setAttribute('role', 'status')
    return append(card, unavailable)
  }
  if (state.status === 'loading') return append(card, text('Loading memories…'))
  if (state.status === 'error') return error(card, state.message)
  return append(card,
    text('Recent memories'),
    list(describe(state.data.recent.memories), 'No recent memories found'),
    text(`Default search: "${state.data.search.query}"`),
    list(describe(state.data.search.memories), `No memories matched "${state.data.search.query}"`)
  )
}

function describe(memories: Memory[]): string[] {
  return memories.map((memory) => `${memory.title} — ${memory.category} · ${memory.project}`)
}
