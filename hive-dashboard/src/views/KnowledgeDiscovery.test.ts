import { describe, expect, it } from 'vitest'
import type { ViewState } from './Overview'
import { dashboardMemories, knowledgeBrowserFixture } from '../fixtures/hive-dashboard'
import type { KnowledgeDiscoveryData } from '../domain/knowledgeDiscovery'
import type { MemoryViewModel } from '../domain/dashboard'
import {
  buildDiscoveryPageLink,
  filterDiscoveryMemories,
  paginateDiscoveryMemories,
  renderKnowledgeDiscovery,
  segmentDiscoveryHighlight
} from './KnowledgeDiscovery'

describe('Knowledge discovery pure helpers', () => {
  it('filters memories by query, category, project, author, date range, and tag from URL parameters', () => {
    const results = filterDiscoveryMemories(dashboardMemories, '?query=auth&category=architecture&project=auth-service&author=sergei-abramov&from=2026-06-06&until=2026-06-06&tag=security')

    expect(results.map((memory) => memory.id)).toEqual(['gateway-auth-boundary'])
  })

  it('treats developer as an author alias and ignores unsupported empty URL values', () => {
    const results = filterDiscoveryMemories(dashboardMemories, '?query=vector&developer=agent-07&category=&project=&tag=')

    expect(results.map((memory) => memory.id)).toEqual(['vector-store-single-writer'])
  })

  it('returns an empty result when tag and date filters exclude every memory', () => {
    const results = filterDiscoveryMemories(dashboardMemories, '?tag=security&from=2026-06-07&until=2026-06-07')

    expect(results).toEqual([])
  })

  it('paginates with URL limit and offset defaults while preserving total counts', () => {
    const page = paginateDiscoveryMemories(dashboardMemories, '?limit=3&offset=3')

    expect(page.items.map((memory) => memory.id)).toEqual([
      'split-ingest-worker-gateway',
      'token-refresh-cold-start',
      'local-first-crdt-reconnect'
    ])
    expect(page).toMatchObject({ limit: 3, offset: 3, total: 8, previousOffset: 0, nextOffset: 6 })
  })

  it('omits pagination offsets that would move before the first item or after the final page', () => {
    expect(paginateDiscoveryMemories(dashboardMemories, '?limit=3&offset=0')).toMatchObject({ previousOffset: null, nextOffset: 3 })
    expect(paginateDiscoveryMemories(dashboardMemories, '?limit=3&offset=6')).toMatchObject({ previousOffset: 3, nextOffset: null })
  })

  it('clamps out-of-range offsets to the final populated page when filtered results exist', () => {
    const page = paginateDiscoveryMemories(dashboardMemories, '?limit=3&offset=999')

    expect(page.items.map((memory) => memory.id)).toEqual(['vector-dimension-pinned', 'conflict-lww-preserve-loser'])
    expect(page).toMatchObject({ limit: 3, offset: 6, total: 8, previousOffset: 3, nextOffset: null })
  })

  it('segments highlight matches without losing the surrounding text', () => {
    expect(segmentDiscoveryHighlight('Gateway owns the auth boundary', ['auth', 'boundary'])).toEqual([
      { text: 'Gateway owns the ', highlighted: false },
      { text: 'auth', highlighted: true },
      { text: ' ', highlighted: false },
      { text: 'boundary', highlighted: true }
    ])
  })

  it('keeps unmatched text as one non-highlighted segment', () => {
    expect(segmentDiscoveryHighlight('Vector store is single-writer', ['auth'])).toEqual([
      { text: 'Vector store is single-writer', highlighted: false }
    ])
  })

  it('builds page links with existing discovery filters and the requested offset', () => {
    expect(buildDiscoveryPageLink('/dashboard/knowledgeBrowser', '?query=auth&tag=security&limit=3&offset=0', 3)).toBe(
      '/dashboard/knowledgeBrowser?query=auth&tag=security&limit=3&offset=3'
    )
  })
})

describe('Knowledge discovery shared DOM', () => {
  it('renders Browse mode with shared filters, source note, cards, metadata, tags, pagination, and detail actions', () => {
    const view = renderKnowledgeDiscovery({
      mode: 'browse',
      title: 'Knowledge Browser',
      path: '/dashboard/knowledgeBrowser',
      filters: '?query=auth&limit=2&offset=0',
      state: ready(discoveryData(knowledgeBrowserFixture.memories.slice(0, 2), { total: 4, limit: 2, nextOffset: 2 }))
    })

    expect(view.querySelector('form[role="search"]')?.textContent).toContain('Search memories')
    expect(view.querySelector('input[name="query"]')?.getAttribute('value')).toBe('auth')
    expect(view.querySelector('select[name="category"]')).not.toBeNull()
    expect(view.querySelector('input[name="project"]')).not.toBeNull()
    expect(view.querySelector('input[name="author"]')).not.toBeNull()
    expect(view.querySelector('input[name="from"]')).not.toBeNull()
    expect(view.querySelector('input[name="until"]')).not.toBeNull()
    expect(view.querySelector('input[name="tag"]')).not.toBeNull()
    expect(view.querySelector('[role="note"]')?.textContent).toBe('Live Hive API data')

    const cards = Array.from(view.querySelectorAll('article[role="listitem"]'))
    expect(cards).toHaveLength(2)
    expect(cards[0]?.textContent).toContain('Gateway owns the auth boundary')
    expect(cards[0]?.textContent).toContain('architecture')
    expect(cards[0]?.textContent).toContain('auth-service')
    expect(cards[0]?.textContent).toContain('Sergei Abramov')
    expect(cards[0]?.textContent).toContain('Saved 06 Jun 2026')
    expect(cards[0]?.textContent).toContain('security')
    expect(cards[0]?.querySelector('a')?.getAttribute('href')).toBe('/dashboard/memories/gateway-auth-boundary')
    expect(view.querySelector('nav[aria-label="Discovery pages"]')?.textContent).toContain('Next page')
  })

  it('renders Search mode without highlight markup or deferred affordances', () => {
    const view = renderKnowledgeDiscovery({
      mode: 'search',
      title: 'Global Search',
      path: '/dashboard/globalSearch',
      filters: '?query=auth&limit=3',
      state: ready(discoveryData([knowledgeBrowserFixture.memories[0]], { total: 1, limit: 3 }))
    })

    expect(view.textContent).toContain('Global Search')
    expect(view.textContent).toContain('Live Hive API data')
    expect(view.querySelector('mark')).toBeNull()
    expect(view.querySelector('a[href="/dashboard/memories/gateway-auth-boundary"]')?.textContent).toContain('Open memory')
    expect(view.textContent).not.toMatch(/export|edit|sync|permission/i)
  })

  it('renders live loading, explicit error, and empty states without fixture fallback', () => {
    const loading = renderKnowledgeDiscovery({ mode: 'browse', title: 'Knowledge Browser', path: '/dashboard/knowledgeBrowser', filters: '', state: { status: 'loading' } })
    const failed = renderKnowledgeDiscovery({ mode: 'browse', title: 'Knowledge Browser', path: '/dashboard/knowledgeBrowser', filters: '', state: { status: 'error', message: 'browse API unavailable' } })
    const view = renderKnowledgeDiscovery({
      mode: 'browse',
      title: 'Knowledge Browser',
      path: '/dashboard/knowledgeBrowser',
      filters: '?query=does-not-exist',
      state: ready(discoveryData([], { total: 0 }))
    })

    expect(loading.textContent).toContain('Loading live memories')
    expect(failed.querySelector('[role="alert"]')?.textContent).toContain('browse API unavailable')
    expect(view.querySelector('[role="status"]')?.textContent).toBe('No live memories match the current filters.')
    expect(view.querySelector('article[role="listitem"]')).toBeNull()
    expect(view.textContent).not.toContain('fixture')
  })

  it('renders matching memories and back pagination for a bookmarked out-of-range offset', () => {
    const view = renderKnowledgeDiscovery({
      mode: 'browse',
      title: 'Knowledge Browser',
      path: '/dashboard/knowledgeBrowser',
      filters: '?limit=3&offset=999',
      state: ready(discoveryData(knowledgeBrowserFixture.memories.slice(-2), { total: 8, limit: 3, offset: 6, previousOffset: 3 }))
    })

    expect(view.querySelector('[role="status"]')).toBeNull()
    expect(Array.from(view.querySelectorAll('article[role="listitem"]')).map((card) => card.textContent)).toEqual(
      expect.arrayContaining([
        expect.stringContaining('Conflicts resolve last-writer-wins, never silent drop'),
        expect.stringContaining('Gateway owns the auth boundary, not services')
      ])
    )
    expect(view.querySelector('nav[aria-label="Discovery pages"]')?.textContent).toContain('Previous page')
    expect(view.querySelector('a[href="/dashboard/knowledgeBrowser?limit=3&offset=3"]')?.textContent).toBe('Previous page')
  })
})

function ready(data: KnowledgeDiscoveryData): ViewState<KnowledgeDiscoveryData> {
  return { status: 'ready', data }
}

function discoveryData(memories: readonly MemoryViewModel[], overrides: Partial<KnowledgeDiscoveryData> = {}): KnowledgeDiscoveryData {
  return {
    items: memories.map((memory) => ({ ...memory, highlights: [] })),
    total: memories.length,
    limit: 10,
    offset: 0,
    previousOffset: null,
    nextOffset: null,
    ...overrides
  }
}
