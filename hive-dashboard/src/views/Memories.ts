import type { Memory, MemoryList, MemorySearch } from '../api/client'
import { append, control, error, list, panel, text } from '../components/dom'
import { markdownViewer } from '../components/MarkdownViewer'
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
  const page = document.createElement('section')
  page.className = 'memory-detail'
  page.append(backButton(options.onBackToMemories))

  if (!route || route.kind === 'none') return append(page, detailStateTitle())
  if (route.kind === 'malformed') return detailError(append(page, detailStateTitle()), 'Malformed memory ID. Open a valid memory link from Search or Knowledge Browser.')
  if (!detail || detail.status === 'loading') return append(page, detailStateTitle(), statusText(`Loading memory ${route.id}…`))
  if (detail.status === 'error') return detailError(append(page, detailStateTitle()), detail.message)

  const memory = detail.data.memory
  const header = memoryHeader(memory)
  const content = document.createElement('main')
  content.className = 'memory-detail__document'
  content.append(memory.content ? markdownViewer(memory.content, `${memory.title} content`) : statusText('This memory has no content.'))

  const layout = document.createElement('div')
  layout.className = 'memory-detail__layout'
  layout.append(content, memoryDetails(memory))
  return append(page, header, layout)
}

function detailStateTitle(): HTMLHeadingElement {
  const title = document.createElement('h1')
  title.textContent = 'Memories'
  return title
}

function backButton(onBackToMemories?: () => void): HTMLButtonElement {
  const button = control('← Back to memories')
  button.className = 'memory-detail__back'
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

function memoryHeader(memory: Memory): HTMLElement {
  const header = document.createElement('header')
  header.className = 'memory-detail__header'

  const badges = document.createElement('div')
  badges.className = 'memory-detail__badges'
  badges.append(badge(memory.category), badge(memory.project))

  const title = document.createElement('h1')
  title.textContent = memory.title

  const context = document.createElement('p')
  context.className = 'memory-detail__context'
  context.append(`By ${memory.created_by} · Created `, dateTime(memory.created_at), ' · Updated ', dateTime(memory.updated_at))

  const actions = document.createElement('div')
  actions.className = 'memory-detail__actions'
  const feedback = document.createElement('p')
  feedback.className = 'memory-detail__copy-feedback'
  feedback.setAttribute('aria-live', 'polite')
  actions.append(copyButton('Copy Markdown', memory.content, 'Markdown', feedback), copyButton('Copy link', window.location.href, 'Link', feedback), feedback)

  return append(header, badges, title, context, actions)
}

function badge(value: string): HTMLElement {
  const node = document.createElement('span')
  node.className = 'memory-detail__badge'
  node.textContent = value
  return node
}

function dateTime(value: string): HTMLTimeElement {
  const node = document.createElement('time')
  node.dateTime = value
  const parsed = new Date(value)
  node.textContent = Number.isNaN(parsed.getTime()) ? value : new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(parsed)
  return node
}

function memoryDetails(memory: Memory): HTMLElement {
  const aside = document.createElement('aside')
  aside.className = 'memory-detail__aside'
  aside.setAttribute('aria-labelledby', 'memory-details-title')
  const title = document.createElement('h2')
  title.id = 'memory-details-title'
  title.textContent = 'Memory Details'
  const metadata = document.createElement('dl')
  metadata.append(
    metadataItem('Project', memory.project),
    metadataItem('Category', memory.category),
    metadataItem('Author', memory.created_by),
    metadataItem('Created', dateTime(memory.created_at)),
    metadataItem('Updated', dateTime(memory.updated_at)),
    metadataItem('Synced', memory.synced_at ? dateTime(memory.synced_at) : 'Not synced')
  )
  return append(aside, title, metadata, valuesSection('Tags', memory.tags, 'memory-detail__tag', 'No tags'), valuesSection('Affected files', memory.files_affected, 'memory-detail__file', 'No affected files'), identifiers(memory))
}

function metadataItem(label: string, value: string | HTMLElement): DocumentFragment {
  const fragment = document.createDocumentFragment()
  const term = document.createElement('dt')
  term.textContent = label
  const description = document.createElement('dd')
  description.append(value)
  fragment.append(term, description)
  return fragment
}

function valuesSection(titleText: string, values: readonly string[], itemClass: string, emptyText: string): HTMLElement {
  const section = document.createElement('section')
  const title = document.createElement('h3')
  title.textContent = titleText
  const list = document.createElement('ul')
  list.className = `${itemClass}-list`
  const visibleValues = values.filter(Boolean)
  for (const value of visibleValues) {
    const li = document.createElement('li')
    li.className = itemClass
    li.textContent = value
    list.append(li)
  }
  if (visibleValues.length === 0) {
    const empty = document.createElement('li')
    empty.className = 'dashboard-state'
    empty.textContent = emptyText
    list.append(empty)
  }
  return append(section, title, list)
}

function identifiers(memory: Memory): HTMLDetailsElement {
  const disclosure = document.createElement('details')
  const summary = document.createElement('summary')
  summary.textContent = 'Technical identifiers'
  const values = document.createElement('dl')
  values.append(metadataItem('Memory ID', memory.id), metadataItem('Sync ID', memory.sync_id))
  disclosure.append(summary, values)
  return disclosure
}

function copyButton(label: string, value: string, subject: string, feedback: HTMLElement): HTMLButtonElement {
  const button = document.createElement('button')
  button.type = 'button'
  button.className = 'memory-detail__copy'
  button.textContent = label
  button.addEventListener('click', () => {
    void copyText(value).then((copied) => {
      feedback.setAttribute('role', copied ? 'status' : 'alert')
      feedback.textContent = copied ? `${subject} copied.` : `Could not copy ${subject.toLowerCase()}.`
    })
  })
  return button
}

async function copyText(value: string): Promise<boolean> {
  try {
    if (!navigator.clipboard?.writeText) throw new Error('Clipboard API unavailable')
    await navigator.clipboard.writeText(value)
    return true
  } catch {
    const field = document.createElement('textarea')
    field.value = value
    field.setAttribute('readonly', '')
    field.style.position = 'fixed'
    field.style.opacity = '0'
    document.body.append(field)
    field.select()
    try {
      return document.execCommand?.('copy') === true
    } catch {
      return false
    } finally {
      field.remove()
    }
  }
}
