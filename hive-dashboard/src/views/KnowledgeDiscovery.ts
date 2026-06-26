import { appendDashboardFilters, parseDashboardFilters, type DashboardUrlFilters } from '../api/urlFilters'
import { emptyState, panel, text } from '../components/dom'
import type { KnowledgeDiscoveryCard, KnowledgeDiscoveryData } from '../domain/knowledgeDiscovery'
import { memoryCategories, type MemoryViewModel, type SearchResultViewModel } from '../domain/dashboard'
import type { ViewState } from './Overview'

export type DiscoveryHighlightSegment = {
  readonly text: string
  readonly highlighted: boolean
}

export type DiscoveryPage<T> = {
  readonly items: readonly T[]
  readonly limit: number
  readonly offset: number
  readonly total: number
  readonly previousOffset: number | null
  readonly nextOffset: number | null
}

export type DiscoveryMode = 'browse' | 'search'

export type DiscoveryRenderInput = {
  readonly mode: DiscoveryMode
  readonly title: string
  readonly path: string
  readonly filters: string | URLSearchParams | DashboardUrlFilters
  readonly state: ViewState<KnowledgeDiscoveryData>
  readonly onFilterSubmit?: (path: string) => void
}

type DiscoveryRenderableMemory = MemoryViewModel | SearchResultViewModel

type DiscoveryMemory = MemoryViewModel & {
  readonly highlights: readonly string[]
}

export function renderKnowledgeDiscovery(input: DiscoveryRenderInput): HTMLElement {
  const filters = normalizeFilters(input.filters)
  const root = panel(input.title)

  root.append(renderFilterForm(input, filters), sourceNotice('Live Hive API data'))

  if (input.state.status === 'loading') {
    root.append(text('Loading live memories…'))
    return root
  }

  if (input.state.status === 'error') {
    const message = text(input.state.message, 'dashboard-state state')
    message.setAttribute('role', 'alert')
    root.append(message)
    return root
  }

  const page = input.state.data
  if (page.total === 0) {
    root.append(emptyState('No live memories match the current filters.'))
    return root
  }

  const list = document.createElement('div')
  list.className = 'dashboard-discovery-results'
  list.setAttribute('role', 'list')
  list.setAttribute('aria-label', `${input.title} results`)
  list.append(...page.items.map((memory) => renderResultCard(memory, input.mode)))
  root.append(list, renderPagination(input.path, filters, page))
  return root
}

export function filterDiscoveryMemories<T extends MemoryViewModel>(
  memories: readonly T[],
  input: string | URLSearchParams | DashboardUrlFilters
): readonly T[] {
  const filters = normalizeFilters(input)
  const query = normalize(filters.query)
  const category = normalize(filters.category)
  const project = normalize(filters.project)
  const author = normalize(filters.author ?? filters.developer)
  const from = parseDay(filters.from ?? filters.since)
  const until = parseDay(filters.until)
  const tags = tagsFor(filters).map(normalize).filter(isPresent)

  return memories.filter((memory) => {
    if (query && !matchesQuery(memory, query)) return false
    if (category && category !== 'all' && normalize(memory.category) !== category) return false
    if (project && normalize(memory.projectId) !== project) return false
    if (author && normalize(memory.authorId) !== author && normalize(memory.authorLabel) !== author) return false
    if (from && memoryDay(memory) < from) return false
    if (until && memoryDay(memory) > until) return false
    if (tags.length > 0 && !tags.every((tag) => memory.tags.some((memoryTag) => normalize(memoryTag) === tag))) return false
    return true
  })
}

export function paginateDiscoveryMemories<T extends MemoryViewModel>(
  memories: readonly T[],
  input: string | URLSearchParams | DashboardUrlFilters
): DiscoveryPage<T> {
  const filters = normalizeFilters(input)
  const limit = positiveInteger(filters.limit, 10)
  const requestedOffset = nonNegativeInteger(filters.offset, 0)
  const offset = clampDiscoveryOffset(requestedOffset, limit, memories.length)
  const items = memories.slice(offset, offset + limit)
  return {
    items,
    limit,
    offset,
    total: memories.length,
    previousOffset: offset <= 0 ? null : Math.max(0, offset - limit),
    nextOffset: offset + limit >= memories.length ? null : offset + limit
  }
}

function clampDiscoveryOffset(offset: number, limit: number, total: number): number {
  if (total <= 0 || offset < total) return offset
  return Math.floor((total - 1) / limit) * limit
}

export function segmentDiscoveryHighlight(text: string, highlights: readonly string[]): readonly DiscoveryHighlightSegment[] {
  const terms = highlights.map(normalize).filter(isPresent)
  if (terms.length === 0) return [{ text, highlighted: false }]

  const segments: DiscoveryHighlightSegment[] = []
  let cursor = 0
  while (cursor < text.length) {
    const match = nextMatch(text, terms, cursor)
    if (!match) {
      segments.push({ text: text.slice(cursor), highlighted: false })
      break
    }
    if (match.start > cursor) segments.push({ text: text.slice(cursor, match.start), highlighted: false })
    segments.push({ text: text.slice(match.start, match.end), highlighted: true })
    cursor = match.end
  }
  return segments
}

export function buildDiscoveryPageLink(path: string, input: string | URLSearchParams | DashboardUrlFilters, offset: number): string {
  const filters = normalizeFilters(input)
  return appendDashboardFilters(path, { ...filters, offset })
}

function renderFilterForm(input: DiscoveryRenderInput, filters: DashboardUrlFilters): HTMLFormElement {
  const form = document.createElement('form')
  form.className = 'dashboard-discovery-filters'
  form.method = 'get'
  form.action = input.path
  form.setAttribute('role', 'search')
  form.addEventListener('submit', (event) => {
    if (!input.onFilterSubmit) return
    event.preventDefault()
    input.onFilterSubmit(discoveryPathFromForm(input.path, form))
  })
  form.append(
    labelledInput('Search memories', 'query', filters.query),
    labelledSelect('Category', 'category', filters.category),
    labelledInput('Project', 'project', filters.project),
    labelledInput('Author', 'author', filters.author ?? filters.developer),
    labelledInput('From', 'from', filters.from ?? filters.since, 'date'),
    labelledInput('Until', 'until', filters.until, 'date'),
    labelledInput('Tag', 'tag', firstTag(filters)),
    hiddenLimit(filters.limit),
    submitButton(input.mode === 'search' ? 'Search' : 'Apply filters')
  )
  return form
}

function renderResultCard(memory: DiscoveryMemory, mode: DiscoveryMode): HTMLElement {
  const card = document.createElement('article')
  card.className = 'dashboard-discovery-card'
  card.setAttribute('role', 'listitem')
  card.setAttribute('aria-label', `${memory.title} memory`)

  const title = document.createElement('h3')
  title.textContent = memory.title
  card.append(
    title,
    metadataLine(memory),
    summary(memory, mode),
    tagList(memory.tags),
    detailLink(memory.id)
  )
  return card
}

function metadataLine(memory: DiscoveryMemory): HTMLElement {
  return text(
    `${memory.category} · ${memory.projectId} · ${memory.authorLabel} · Saved ${memory.savedAtLabel}`,
    'dashboard-discovery-card__metadata'
  )
}

function summary(memory: DiscoveryMemory, mode: DiscoveryMode): HTMLElement {
  const paragraph = text('', 'dashboard-discovery-card__summary')
  paragraph.append(document.createTextNode(memory.content))
  return paragraph
}

function renderHighlightSegment(segment: DiscoveryHighlightSegment): Text | HTMLElement {
  if (!segment.highlighted) return document.createTextNode(segment.text)
  const mark = document.createElement('mark')
  mark.textContent = segment.text
  return mark
}

function tagList(tags: readonly string[]): HTMLElement {
  const list = document.createElement('ul')
  list.className = 'dashboard-discovery-tags'
  list.setAttribute('aria-label', 'Memory tags')
  for (const tag of tags) {
    const item = document.createElement('li')
    item.textContent = tag
    list.append(item)
  }
  return list
}

function detailLink(memoryId: string): HTMLAnchorElement {
  const link = document.createElement('a')
  link.className = 'dashboard-control control dashboard-discovery-card__action'
  link.href = `/dashboard/memories/${encodeURIComponent(memoryId)}`
  link.textContent = 'Open memory'
  return link
}

function renderPagination(path: string, filters: DashboardUrlFilters, page: Pick<KnowledgeDiscoveryData, 'items' | 'total' | 'previousOffset' | 'nextOffset'>): HTMLElement {
  const nav = document.createElement('nav')
  nav.className = 'dashboard-discovery-pagination'
  nav.setAttribute('aria-label', 'Discovery pages')
  nav.append(text(`Showing ${page.items.length} of ${page.total} live memories.`))
  if (page.previousOffset !== null) nav.append(pageLink('Previous page', path, filters, page.previousOffset))
  if (page.nextOffset !== null) nav.append(pageLink('Next page', path, filters, page.nextOffset))
  return nav
}

function pageLink(label: string, path: string, filters: DashboardUrlFilters, offset: number): HTMLAnchorElement {
  const link = document.createElement('a')
  link.href = buildDiscoveryPageLink(path, filters, offset)
  link.textContent = label
  return link
}

function sourceNotice(message: string): HTMLElement {
  const note = text(message, 'dashboard-source-note')
  note.setAttribute('role', 'note')
  return note
}

function labelledInput(label: string, name: string, value: string | null | undefined, type = 'text'): HTMLElement {
  const wrapper = document.createElement('label')
  wrapper.textContent = label
  const input = document.createElement('input')
  input.name = name
  input.type = type
  if (value) input.setAttribute('value', value)
  wrapper.append(input)
  return wrapper
}

function labelledSelect(label: string, name: string, value: string | null | undefined): HTMLElement {
  const wrapper = document.createElement('label')
  wrapper.textContent = label
  const select = document.createElement('select')
  select.name = name
  select.append(option('All', 'all', value === 'all' || !value))
  for (const category of memoryCategories) select.append(option(category.replace('_', ' '), category, value === category))
  wrapper.append(select)
  return wrapper
}

function option(label: string, value: string, selected: boolean): HTMLOptionElement {
  const option = document.createElement('option')
  option.value = value
  option.textContent = label
  option.selected = selected
  return option
}

function hiddenLimit(limit: number | null | undefined): HTMLInputElement {
  const input = document.createElement('input')
  input.type = 'hidden'
  input.name = 'limit'
  input.value = String(positiveInteger(limit, 10))
  return input
}

function submitButton(label: string): HTMLButtonElement {
  const button = document.createElement('button')
  button.type = 'submit'
  button.textContent = label
  return button
}

function toDiscoveryMemory(memory: DiscoveryRenderableMemory): DiscoveryMemory {
  if ('memoryId' in memory) {
    return {
      id: memory.memoryId,
      title: memory.title,
      content: memory.excerpt,
      category: memory.category,
      projectId: memory.projectId,
      authorId: memory.authorId,
      authorLabel: memory.authorLabel,
      tags: memory.tags,
      savedAt: memory.savedAt,
      savedAtLabel: memory.savedAtLabel,
      highlights: memory.highlights
    }
  }
  return { ...memory, highlights: [] }
}

function normalizeFilters(input: string | URLSearchParams | DashboardUrlFilters): DashboardUrlFilters {
  if (typeof input === 'string' || input instanceof URLSearchParams) return parseDashboardFilters(input)
  return input
}

function matchesQuery(memory: MemoryViewModel, query: string): boolean {
  return [memory.title, memory.content, memory.category, memory.projectId, memory.authorId, memory.authorLabel, ...memory.tags]
    .map(normalize)
    .some((value) => value.includes(query))
}

function tagsFor(filters: DashboardUrlFilters): readonly string[] {
  if (Array.isArray(filters.tags)) return filters.tags.filter((tag): tag is string => typeof tag === 'string')
  if (Array.isArray(filters.tag)) return filters.tag.filter((tag): tag is string => typeof tag === 'string')
  return typeof filters.tag === 'string' ? [filters.tag] : []
}

function firstTag(filters: DashboardUrlFilters): string | null {
  const [tag] = tagsFor(filters)
  return tag ?? null
}

function discoveryPathFromForm(path: string, form: HTMLFormElement): string {
  const data = new FormData(form)
  const filters: DashboardUrlFilters = {
    query: stringFormValue(data, 'query'),
    category: categoryFormValue(data),
    project: stringFormValue(data, 'project'),
    from: stringFormValue(data, 'from'),
    until: stringFormValue(data, 'until'),
    tag: stringFormValue(data, 'tag'),
    limit: numberFormValue(data, 'limit')
  }
  return appendDashboardFilters(path, filters)
}

function stringFormValue(data: FormData, key: string): string | undefined {
  const value = String(data.get(key) ?? '').trim()
  return value === '' ? undefined : value
}

function categoryFormValue(data: FormData): string | undefined {
  const value = stringFormValue(data, 'category')
  return !value || value === 'all' ? undefined : value
}

function numberFormValue(data: FormData, key: string): number | undefined {
  const value = stringFormValue(data, key)
  if (!value) return undefined
  const parsed = Number(value)
  return Number.isInteger(parsed) ? parsed : undefined
}

function memoryDay(memory: MemoryViewModel): string {
  return memory.savedAt.slice(0, 10)
}

function parseDay(value: string | null | undefined): string | null {
  const normalized = normalize(value)
  if (!/^\d{4}-\d{2}-\d{2}/.test(normalized)) return null
  return normalized.slice(0, 10)
}

function positiveInteger(value: number | null | undefined, fallback: number): number {
  return Number.isInteger(value) && value !== null && value !== undefined && value > 0 ? value : fallback
}

function nonNegativeInteger(value: number | null | undefined, fallback: number): number {
  return Number.isInteger(value) && value !== null && value !== undefined && value >= 0 ? value : fallback
}

function normalize(value: string | null | undefined): string {
  return value?.trim().toLowerCase() ?? ''
}

function isPresent(value: string): boolean {
  return value !== ''
}

function nextMatch(text: string, terms: readonly string[], start: number): { readonly start: number; readonly end: number } | null {
  const lowerText = text.toLowerCase()
  let best: { start: number; end: number } | null = null
  for (const term of terms) {
    const index = lowerText.indexOf(term, start)
    if (index === -1) continue
    if (!best || index < best.start || (index === best.start && term.length > best.end - best.start)) {
      best = { start: index, end: index + term.length }
    }
  }
  return best
}
