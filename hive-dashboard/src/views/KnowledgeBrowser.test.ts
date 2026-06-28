import { describe, expect, it, vi } from 'vitest'
import { knowledgeBrowserFixture } from '../fixtures/hive-dashboard'
import type { KnowledgeDiscoveryData } from '../domain/knowledgeDiscovery'
import { renderKnowledgeBrowser } from './KnowledgeBrowser'

describe('Knowledge Browser view', () => {
  it('renders a bespoke dense browse surface without the generic outer panel', () => {
    const view = renderKnowledgeBrowser(ready(knowledgeBrowserFixture.memories.slice(0, 2), { total: 4, limit: 2, nextOffset: 2 }), '?query=auth&limit=2')

    expect(view.classList.contains('dashboard-panel')).toBe(false)
    expect(view.classList.contains('panel')).toBe(false)
    expect(view.classList.contains('dashboard-knowledge-browser')).toBe(true)
    expect(view.querySelector('h2')?.textContent).toBe('Explore team memory')
    expect(view.querySelector<HTMLInputElement>('input[name="query"]')?.value).toBe('auth')
    expect(view.querySelector('[role="note"]')?.textContent).toContain('Live Hive API browse data')
    expect(view.querySelector('[data-knowledge-browser-export]')?.textContent).toContain('Export')
    expect(view.querySelector<HTMLButtonElement>('[data-knowledge-browser-export]')?.disabled).toBe(true)
    expect(view.querySelector('[data-knowledge-browser-export-copy]')?.textContent).toContain('Export is deferred for the MVP')

    const cards = Array.from(view.querySelectorAll('article[role="listitem"]'))
    expect(cards).toHaveLength(2)
    expect(cards[0]?.textContent).toContain('Gateway owns the auth boundary, not services')
    expect(cards[0]?.textContent).toContain('architecture')
    expect(cards[0]?.textContent).toContain('auth-service')
    expect(cards[0]?.textContent).toContain('security')
    expect(cards[0]?.querySelector('a')?.getAttribute('href')).toBe(detailHref('gateway-auth-boundary', '/dashboard/knowledgeBrowser?query=auth&limit=2'))
    expect(view.querySelector('mark')).toBeNull()
  })

  it('submits in-page search and category chips on the Knowledge Browser route', () => {
    const onNavigate = vi.fn()
    const view = renderKnowledgeBrowser(ready(knowledgeBrowserFixture.memories.slice(0, 3), { total: 3, limit: 10 }), '?query=auth&project=auth-service&limit=10', { onNavigate })

    const search = view.querySelector<HTMLInputElement>('input[name="query"]')!
    search.value = 'vector clocks'
    view.querySelector('form[role="search"]')!.dispatchEvent(new SubmitEvent('submit', { bubbles: true, cancelable: true }))

    expect(onNavigate).toHaveBeenCalledWith('/dashboard/knowledgeBrowser?query=vector+clocks&project=auth-service&limit=10')
    const chipLabels = Array.from(view.querySelectorAll('[data-knowledge-category-chip]')).map((chip) => chip.textContent?.trim())
    expect(chipLabels).toEqual(['All', 'architecture', 'bugfix'])
    expect(view.querySelector<HTMLAnchorElement>('[data-knowledge-category-chip="bugfix"]')?.getAttribute('href')).toBe('/dashboard/knowledgeBrowser?query=auth&project=auth-service&category=bugfix&limit=10')
  })

  it('restores advanced live filters for project, category, and date range without unsupported controls', () => {
    const view = renderKnowledgeBrowser(ready(knowledgeBrowserFixture.memories.slice(0, 3), { total: 3, limit: 10 }), '?query=auth&project=auth-service&category=bugfix&from=2026-06-01&until=2026-06-30&limit=10')
    const advancedFilters = view.querySelector<HTMLFormElement>('form[aria-label="Knowledge Browser live filters"]')

    expect(advancedFilters).not.toBeNull()
    expect(advancedFilters?.querySelector<HTMLInputElement>('input[name="project"]')?.value).toBe('auth-service')
    expect(advancedFilters?.querySelector<HTMLInputElement>('input[name="from"]')?.value).toBe('2026-06-01')
    expect(advancedFilters?.querySelector<HTMLInputElement>('input[name="until"]')?.value).toBe('2026-06-30')
    expect(advancedFilters?.querySelector<HTMLSelectElement>('select[name="category"]')?.value).toBe('bugfix')
    expect(advancedFilters?.querySelector('input[name="developer"]')).toBeNull()
    expect(advancedFilters?.querySelector('input[name="tag"]')).toBeNull()
    expect(view.querySelector<HTMLAnchorElement>('[data-knowledge-category-chip="bugfix"]')?.getAttribute('aria-current')).toBe('true')
  })

  it('submits advanced filters to supported browse params and keeps category chips in sync', () => {
    const onNavigate = vi.fn()
    const view = renderKnowledgeBrowser(ready(knowledgeBrowserFixture.memories.slice(0, 3), { total: 3, limit: 10 }), '?query=auth&category=architecture&limit=10&offset=20', { onNavigate })
    const advancedFilters = view.querySelector<HTMLFormElement>('form[aria-label="Knowledge Browser live filters"]')!

    advancedFilters.querySelector<HTMLInputElement>('input[name="project"]')!.value = 'platform-api'
    advancedFilters.querySelector<HTMLInputElement>('input[name="from"]')!.value = '2026-05-01'
    advancedFilters.querySelector<HTMLInputElement>('input[name="until"]')!.value = '2026-05-31'
    advancedFilters.querySelector<HTMLSelectElement>('select[name="category"]')!.value = 'bugfix'
    advancedFilters.dispatchEvent(new SubmitEvent('submit', { bubbles: true, cancelable: true }))

    expect(onNavigate).toHaveBeenCalledWith('/dashboard/knowledgeBrowser?query=auth&project=platform-api&category=bugfix&from=2026-05-01&until=2026-05-31&limit=10')
    expect(view.querySelector<HTMLAnchorElement>('[data-knowledge-category-chip="architecture"]')?.getAttribute('aria-current')).toBe('true')
    expect(view.querySelector<HTMLAnchorElement>('[data-knowledge-category-chip="all"]')?.getAttribute('href')).toBe('/dashboard/knowledgeBrowser?query=auth&limit=10')
  })

  it('renders loading, error, empty, and pagination states with accessible semantics', () => {
    const loading = renderKnowledgeBrowser({ status: 'loading' }, '?query=auth')
    const failed = renderKnowledgeBrowser({ status: 'error', message: 'browse API unavailable' }, '?query=auth')
    const empty = renderKnowledgeBrowser(ready([], { total: 0, limit: 10 }), '?query=absent')
    const paged = renderKnowledgeBrowser(ready(knowledgeBrowserFixture.memories.slice(0, 1), { total: 3, limit: 1, nextOffset: 1 }), '?query=auth&category=architecture&limit=1')

    expect(loading.querySelector('[role="status"]')?.textContent).toContain('Loading live memories')
    expect(failed.querySelector('[role="alert"]')?.textContent).toContain('browse API unavailable')
    expect(empty.querySelector('[role="status"]')?.textContent).toContain('No live memories match this browse query')
    expect(paged.querySelector('nav[aria-label="Knowledge Browser pages"]')?.textContent).toContain('Showing 1 of 3')
    expect(paged.querySelector('a[href="/dashboard/knowledgeBrowser?query=auth&category=architecture&limit=1&offset=1"]')?.textContent).toBe('Next page')
  })

  it('preserves the active category chip when filtered results are empty', () => {
    const view = renderKnowledgeBrowser(ready([], { total: 0, limit: 10 }), '?query=absent&category=bugfix&limit=10')
    const chipLabels = Array.from(view.querySelectorAll('[data-knowledge-category-chip]')).map((chip) => chip.textContent?.trim())
    const activeChip = view.querySelector<HTMLAnchorElement>('[data-knowledge-category-chip="bugfix"]')

    expect(chipLabels).toEqual(['All', 'bugfix'])
    expect(activeChip?.getAttribute('aria-current')).toBe('true')
    expect(activeChip?.getAttribute('href')).toBe('/dashboard/knowledgeBrowser?query=absent&category=bugfix&limit=10')
    expect(view.querySelector('[role="status"]')?.textContent).toContain('No live memories match this browse query')
  })

  it('keeps export visible but inert without creating downloads or API actions', () => {
    const view = renderKnowledgeBrowser(ready(knowledgeBrowserFixture.memories.slice(0, 1)), '?query=auth')
    const exportButton = view.querySelector<HTMLButtonElement>('[data-knowledge-browser-export]')

    expect(exportButton?.tagName.toLowerCase()).toBe('button')
    expect(exportButton?.type).toBe('button')
    expect(exportButton?.disabled).toBe(true)
    expect(exportButton?.getAttribute('aria-describedby')).toBe('knowledge-browser-export-status')
    expect(view.querySelector('a[download]')).toBeNull()
  })
})

function ready(memories: typeof knowledgeBrowserFixture.memories, overrides: Partial<KnowledgeDiscoveryData> = {}) {
  return { status: 'ready' as const, data: { items: memories.map((memory) => ({ ...memory, highlights: [] })), total: memories.length, limit: 10, offset: 0, previousOffset: null, nextOffset: null, ...overrides } }
}

function detailHref(memoryId: string, returnTo: string): string {
  return `/dashboard/memories/${encodeURIComponent(memoryId)}?${new URLSearchParams({ returnTo }).toString()}`
}
