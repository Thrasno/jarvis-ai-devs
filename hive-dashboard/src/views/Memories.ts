import type { Memory, MemoryList, MemorySearch } from '../api/client'
import { append, error, list, panel, text, type ViewState } from './Overview'

export function renderMemories(state: ViewState<{ recent: MemoryList; search: MemorySearch }>): HTMLElement {
  const card = panel('Memories')
  if (state.status === 'loading') return append(card, text('Loading memories…'))
  if (state.status === 'error') return error(card, state.message)
  return append(card,
    text('Recent memories'),
    list(describe(state.data.recent.memories), 'No recent memories found'),
    text(`Search results for "${state.data.search.query}"`),
    list(describe(state.data.search.memories), `No memories matched "${state.data.search.query}"`)
  )
}

function describe(memories: Memory[]): string[] {
  return memories.map((memory) => `${memory.title} — ${memory.category} · ${memory.project}`)
}
