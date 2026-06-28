import type { KnowledgeDiscoveryData } from '../domain/knowledgeDiscovery'
import type { KnowledgeDiscoveryCard } from '../domain/knowledgeDiscovery'
import { appendDashboardFilters, parseDashboardFilters, type DashboardUrlFilters } from '../api/urlFilters'
import type { ViewState } from './Overview'

export function renderKnowledgeBrowser(state: ViewState<KnowledgeDiscoveryData>, filters: string | URLSearchParams = window.location.search, options: { onNavigate?: (path: string) => void; detailOriginPath?: string } = {}): HTMLElement {
  const parsedFilters = parseBrowserFilters(filters)
  const root = document.createElement('section')
  root.className = 'dashboard-knowledge-browser'
  root.dataset.dashboardView = 'knowledge-browser'

  root.append(renderHero(parsedFilters, options.onNavigate))
  const readyItems = state.status === 'ready' ? state.data.items : []
  root.append(renderAdvancedFilters(readyItems, parsedFilters, options.onNavigate), renderCategoryChips(readyItems, parsedFilters, options.onNavigate))

  if (state.status === 'loading') {
    root.append(statusMessage('Loading live memories…'))
    return root
  }

  if (state.status === 'error') {
    root.append(alertMessage(state.message))
    return root
  }

  const page = state.data
  if (page.total === 0) {
    root.append(emptyMessage('No live memories match this browse query.'))
    return root
  }

  const detailOriginPath = options.detailOriginPath ?? appendDashboardFilters('/dashboard/knowledgeBrowser', browserLiveFilters(parsedFilters))
  const grid = document.createElement('div')
  grid.className = 'dashboard-knowledge-browser__grid'
  grid.setAttribute('role', 'list')
  grid.setAttribute('aria-label', 'Knowledge Browser results')
  grid.append(...page.items.map((memory) => renderCard(memory, detailOriginPath)))
  root.append(grid, renderPagination(page, parsedFilters))
  return root
}

function renderHero(filters: DashboardUrlFilters, onNavigate?: (path: string) => void): HTMLElement {
  const hero = document.createElement('div')
  hero.className = 'dashboard-knowledge-browser__hero'

  const copy = document.createElement('div')
  copy.className = 'dashboard-knowledge-browser__hero-copy'
  const eyebrow = document.createElement('p')
  eyebrow.className = 'dashboard-knowledge-browser__eyebrow'
  eyebrow.textContent = 'Live knowledge discovery'
  const title = document.createElement('h2')
  title.textContent = 'Explore team memory'
  const description = document.createElement('p')
  description.textContent = 'Search, filter, and open production Hive memories without leaving the browser route.'
  const source = document.createElement('p')
  source.className = 'dashboard-knowledge-browser__source'
  source.setAttribute('role', 'note')
  source.textContent = 'Live Hive API browse data · unsupported tag and developer filters are not active in this MVP.'
  copy.append(eyebrow, title, description, source)

  const controls = document.createElement('div')
  controls.className = 'dashboard-knowledge-browser__hero-controls'
  controls.append(renderSearchForm(filters, onNavigate), renderExportAffordance())
  hero.append(copy, controls)
  return hero
}

function renderSearchForm(filters: DashboardUrlFilters, onNavigate?: (path: string) => void): HTMLFormElement {
  const form = document.createElement('form')
  form.className = 'dashboard-knowledge-browser__search'
  form.method = 'get'
  form.action = '/dashboard/knowledgeBrowser'
  form.setAttribute('role', 'search')

  const label = document.createElement('label')
  label.textContent = 'Search Knowledge Browser'
  const input = document.createElement('input')
  input.type = 'search'
  input.name = 'query'
  input.placeholder = 'Search live memories…'
  input.value = filters.query?.trim() ?? ''
  label.append(input)

  const button = document.createElement('button')
  button.type = 'submit'
  button.textContent = 'Search'
  form.append(label, hiddenInput('project', filters.project), hiddenInput('category', categoryParam(filters.category)), hiddenInput('from', filters.from), hiddenInput('until', filters.until), hiddenInput('limit', filters.limit), button)
  form.addEventListener('submit', (event) => {
    if (!onNavigate) return
    event.preventDefault()
    const data = new FormData(form)
    onNavigate(appendDashboardFilters('/dashboard/knowledgeBrowser', {
      query: stringFormValue(data, 'query'),
      project: stringFormValue(data, 'project'),
      category: stringFormValue(data, 'category'),
      from: stringFormValue(data, 'from'),
      until: stringFormValue(data, 'until'),
      limit: numberFormValue(data, 'limit')
    }))
  })
  return form
}

function renderExportAffordance(): HTMLElement {
  const wrapper = document.createElement('div')
  wrapper.className = 'dashboard-knowledge-browser__export'
  const button = document.createElement('button')
  button.type = 'button'
  button.disabled = true
  button.dataset.knowledgeBrowserExport = 'true'
  button.setAttribute('aria-describedby', 'knowledge-browser-export-status')
  button.textContent = 'Export'
  const copy = document.createElement('p')
  copy.id = 'knowledge-browser-export-status'
  copy.dataset.knowledgeBrowserExportCopy = 'true'
  copy.textContent = 'Export is deferred for the MVP. No download or export API is available yet.'
  wrapper.append(button, copy)
  return wrapper
}

function renderAdvancedFilters(items: readonly KnowledgeDiscoveryCard[], filters: DashboardUrlFilters, onNavigate?: (path: string) => void): HTMLFormElement {
  const form = document.createElement('form')
  form.className = 'dashboard-knowledge-browser__filters'
  form.method = 'get'
  form.action = '/dashboard/knowledgeBrowser'
  form.setAttribute('aria-label', 'Knowledge Browser live filters')
  form.append(
    hiddenInput('query', filters.query),
    filterInput('Project', 'project', filters.project),
    filterSelect('Category', 'category', filters.category, categoryChipValues(items, filters)),
    filterInput('From', 'from', filters.from, 'date'),
    filterInput('Until', 'until', filters.until, 'date'),
    hiddenInput('limit', filters.limit),
    filterButton('Apply filters')
  )
  form.addEventListener('submit', (event) => {
    if (!onNavigate) return
    event.preventDefault()
    const data = new FormData(form)
    onNavigate(appendDashboardFilters('/dashboard/knowledgeBrowser', {
      query: stringFormValue(data, 'query'),
      project: stringFormValue(data, 'project'),
      category: categoryParam(stringFormValue(data, 'category')),
      from: stringFormValue(data, 'from'),
      until: stringFormValue(data, 'until'),
      limit: numberFormValue(data, 'limit')
    }))
  })
  return form
}

function filterInput(label: string, name: string, value: DashboardUrlFilters[keyof DashboardUrlFilters], type = 'text'): HTMLElement {
  const wrapper = document.createElement('label')
  wrapper.textContent = label
  const input = document.createElement('input')
  input.name = name
  input.type = type
  if (typeof value === 'string') input.value = value
  wrapper.append(input)
  return wrapper
}

function filterSelect(label: string, name: string, value: DashboardUrlFilters[keyof DashboardUrlFilters], categories: readonly string[]): HTMLElement {
  const wrapper = document.createElement('label')
  wrapper.textContent = label
  const select = document.createElement('select')
  select.name = name
  const activeCategory = categoryParam(typeof value === 'string' ? value : undefined)
  select.append(selectOption('All categories', 'all', !activeCategory))
  for (const category of categories) select.append(selectOption(category, category, category === activeCategory))
  wrapper.append(select)
  return wrapper
}

function selectOption(label: string, value: string, selected: boolean): HTMLOptionElement {
  const option = document.createElement('option')
  option.value = value
  option.textContent = label
  option.selected = selected
  return option
}

function filterButton(label: string): HTMLButtonElement {
  const button = document.createElement('button')
  button.type = 'submit'
  button.textContent = label
  return button
}

function renderCategoryChips(items: readonly KnowledgeDiscoveryCard[], filters: DashboardUrlFilters, onNavigate?: (path: string) => void): HTMLElement {
  const nav = document.createElement('nav')
  nav.className = 'dashboard-knowledge-browser__chips'
  nav.setAttribute('aria-label', 'Memory categories')
  const categories = categoryChipValues(items, filters)
  nav.append(categoryChip('All', undefined, filters, onNavigate), ...categories.map((category) => categoryChip(category, category, filters, onNavigate)))
  return nav
}

function categoryChipValues(items: readonly KnowledgeDiscoveryCard[], filters: DashboardUrlFilters): string[] {
  const categories = new Set<string>(items.map((item) => item.category))
  const activeCategory = categoryParam(filters.category)
  if (activeCategory) categories.add(activeCategory)
  return Array.from(categories).sort()
}

function categoryChip(label: string, category: string | undefined, filters: DashboardUrlFilters, onNavigate?: (path: string) => void): HTMLAnchorElement {
  const href = appendDashboardFilters('/dashboard/knowledgeBrowser', { ...browserLiveFilters(filters), category, offset: undefined })
  const link = document.createElement('a')
  link.href = href
  link.dataset.knowledgeCategoryChip = category ?? 'all'
  link.textContent = label
  const activeCategory = categoryParam(filters.category)
  if ((!category && !activeCategory) || category === activeCategory) link.setAttribute('aria-current', 'true')
  link.addEventListener('click', (event) => {
    if (!onNavigate) return
    event.preventDefault()
    onNavigate(href)
  })
  return link
}

function renderCard(memory: KnowledgeDiscoveryCard, detailOriginPath: string): HTMLElement {
  const card = document.createElement('article')
  card.className = 'dashboard-knowledge-browser-card'
  card.setAttribute('role', 'listitem')
  card.setAttribute('aria-label', `${memory.title} memory`)

  const header = document.createElement('div')
  header.className = 'dashboard-knowledge-browser-card__header'
  const badge = document.createElement('span')
  badge.className = 'dashboard-knowledge-browser-card__badge'
  badge.textContent = memory.category
  const project = document.createElement('span')
  project.className = 'dashboard-knowledge-browser-card__project'
  project.textContent = memory.projectId
  header.append(badge, project)

  const title = document.createElement('h3')
  title.textContent = memory.title
  const summary = document.createElement('p')
  summary.className = 'dashboard-knowledge-browser-card__summary'
  summary.textContent = memory.content
  const meta = document.createElement('p')
  meta.className = 'dashboard-knowledge-browser-card__metadata'
  meta.textContent = `${memory.authorLabel} · Saved ${memory.savedAtLabel}`
  card.append(header, title, summary, meta, tagList(memory.tags), detailLink(memory.id, detailOriginPath))
  return card
}

function tagList(tags: readonly string[]): HTMLElement {
  const list = document.createElement('ul')
  list.className = 'dashboard-knowledge-browser-card__tags'
  list.setAttribute('aria-label', 'Memory tags')
  for (const tag of tags) {
    const item = document.createElement('li')
    item.textContent = tag
    list.append(item)
  }
  return list
}

function detailLink(memoryId: string, originPath: string): HTMLAnchorElement {
  const link = document.createElement('a')
  link.className = 'dashboard-control control dashboard-knowledge-browser-card__action'
  link.href = `/dashboard/memories/${encodeURIComponent(memoryId)}?${new URLSearchParams({ returnTo: originPath }).toString()}`
  link.textContent = 'Open memory'
  return link
}

function renderPagination(page: KnowledgeDiscoveryData, filters: DashboardUrlFilters): HTMLElement {
  const nav = document.createElement('nav')
  nav.className = 'dashboard-knowledge-browser__pagination'
  nav.setAttribute('aria-label', 'Knowledge Browser pages')
  const summary = document.createElement('p')
  summary.textContent = `Showing ${page.items.length} of ${page.total} live memories.`
  nav.append(summary)
  if (page.previousOffset !== null) nav.append(pageLink('Previous page', filters, page.previousOffset))
  if (page.nextOffset !== null) nav.append(pageLink('Next page', filters, page.nextOffset))
  return nav
}

function pageLink(label: string, filters: DashboardUrlFilters, offset: number): HTMLAnchorElement {
  const link = document.createElement('a')
  link.href = appendDashboardFilters('/dashboard/knowledgeBrowser', { ...browserLiveFilters(filters), offset })
  link.textContent = label
  return link
}

function statusMessage(message: string): HTMLElement {
  const element = document.createElement('p')
  element.className = 'dashboard-state state'
  element.setAttribute('role', 'status')
  element.textContent = message
  return element
}

function emptyMessage(message: string): HTMLElement {
  const element = statusMessage(message)
  element.dataset.state = 'empty'
  return element
}

function alertMessage(message: string): HTMLElement {
  const element = document.createElement('p')
  element.className = 'dashboard-state state'
  element.setAttribute('role', 'alert')
  element.textContent = message
  return element
}

function parseBrowserFilters(input: string | URLSearchParams): DashboardUrlFilters {
  return parseDashboardFilters(input)
}

function browserLiveFilters(filters: DashboardUrlFilters): DashboardUrlFilters {
  return {
    query: filters.query,
    project: filters.project,
    category: categoryParam(filters.category),
    from: filters.from,
    until: filters.until,
    limit: filters.limit,
    offset: filters.offset
  }
}

function categoryParam(category: DashboardUrlFilters['category']): string | undefined {
  const value = category?.trim()
  return value && value !== 'all' ? value : undefined
}

function hiddenInput(name: string, value: DashboardUrlFilters[keyof DashboardUrlFilters]): HTMLInputElement {
  const input = document.createElement('input')
  input.type = 'hidden'
  input.name = name
  if (typeof value === 'number') input.value = String(value)
  if (typeof value === 'string') input.value = value
  return input
}

function stringFormValue(data: FormData, key: string): string | undefined {
  const value = String(data.get(key) ?? '').trim()
  return value === '' ? undefined : value
}

function numberFormValue(data: FormData, key: string): number | undefined {
  const value = stringFormValue(data, key)
  if (!value) return undefined
  const parsed = Number(value)
  return Number.isInteger(parsed) && parsed > 0 ? parsed : undefined
}
