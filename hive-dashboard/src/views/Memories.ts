import type { Memory, MemoryList, MemorySearch } from '../api/client'
import { append, control, error, list, panel, text } from '../components/dom'
import type { ViewState } from './Overview'

export type MemoryDetailRoute =
  | { kind: 'none' }
  | { kind: 'valid'; id: string; routeKey: string }
  | { kind: 'malformed'; raw: string }

export type MemoryDetailData = { routeId: string; memory: Memory }
export type MemoryDetailViewState = ViewState<MemoryDetailData> | { status: 'error'; message: string; routeId?: string }

type MemoriesViewOptions = {
  detailRoute?: MemoryDetailRoute
  detail?: MemoryDetailViewState
  onBackToMemories?: () => void
}

export function renderMemories(state: ViewState<{ recent: MemoryList; search: MemorySearch }>, options: MemoriesViewOptions = {}): HTMLElement {
  if (options.detailRoute && options.detailRoute.kind !== 'none') return renderMemoryDetail(options)

  const card = panel('Memories')
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

function renderMemoryDetail(options: MemoriesViewOptions): HTMLElement {
  const route = options.detailRoute
  const detail = options.detail
  const title = detail?.status === 'ready' ? detail.data.memory.title : 'Memories'
  const card = panel(title)
  card.append(backButton(options.onBackToMemories))

  if (!route || route.kind === 'none') return card
  if (route.kind === 'malformed') return detailError(card, 'Malformed memory ID. Open a valid memory link from Search or Knowledge Browser.')
  if (!detail || detail.status === 'loading') return append(card, statusText(`Loading memory ${route.id}…`))
  if (detail.status === 'error') return detailError(card, detail.message)

  const memory = detail.data.memory
  return append(card,
    text(memory.content),
    detailList(memory),
    optionalList(memory.tags, 'Tag'),
    optionalList(memory.files_affected, 'File')
  )
}

function backButton(onBackToMemories?: () => void): HTMLButtonElement {
  const button = control('Back to memories')
  button.setAttribute('aria-label', 'Back to memories')
  button.addEventListener('click', () => onBackToMemories?.())
  return button
}

function statusText(message: string): HTMLElement {
  const node = text(message, 'dashboard-state state')
  node.setAttribute('role', 'status')
  return node
}

function detailError<T extends HTMLElement>(root: T, message: string): T {
  const node = text(message, 'dashboard-state state')
  node.setAttribute('role', 'alert')
  root.dataset.state = 'error'
  root.append(node)
  return root
}

function detailList(memory: Memory): HTMLElement {
  const items = [
    `Project: ${memory.project}`,
    `Category: ${memory.category}`,
    `Created by: ${memory.created_by}`,
    `Created: ${memory.created_at}`,
    `Updated: ${memory.updated_at}`,
    memory.synced_at ? `Synced: ${memory.synced_at}` : '',
    `Sync ID: ${memory.sync_id}`,
    `Memory ID: ${memory.id}`
  ].filter(Boolean)
  return list(items)
}

function optionalList(values: readonly string[], label: string): HTMLElement {
  const items = values.filter(Boolean).map((value) => `${label}: ${value}`)
  const node = document.createElement('ul')
  for (const item of items) {
    const li = document.createElement('li')
    li.textContent = item
    node.append(li)
  }
  return node
}
