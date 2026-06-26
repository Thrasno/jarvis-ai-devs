import { describe, expect, it } from 'vitest'
import { knowledgeBrowserFixture } from '../fixtures/hive-dashboard'
import { renderGlobalSearch } from './GlobalSearch'

describe('Global Search view', () => {
  it('composes Search mode over the global search fixture with query-first controls', () => {
    const view = renderGlobalSearch({ status: 'ready', data: { items: knowledgeBrowserFixture.memories.slice(0, 1).map((memory) => ({ ...memory, highlights: [] })), total: 1, limit: 3, offset: 0, previousOffset: null, nextOffset: null } }, '?query=auth&limit=3')

    const controls = Array.from(view.querySelectorAll('form[role="search"] label')).map((label) => label.textContent ?? '')
    expect(controls[0]).toContain('Search memories')
    expect(controls[1]).toContain('Category')
    expect(view.querySelector('h2')?.textContent).toBe('Global Search')
    expect(view.querySelector('[role="note"]')?.textContent).toBe('Live Hive API data')
    expect(view.querySelector('input[name="query"]')?.getAttribute('value')).toBe('auth')
    expect(view.querySelector('mark')).toBeNull()
  })

  it('uses fixture search results and links selected results to existing memory detail routes', () => {
    const view = renderGlobalSearch({ status: 'ready', data: { items: knowledgeBrowserFixture.memories.slice(0, 1).map((memory) => ({ ...memory, highlights: [] })), total: 1, limit: 1, offset: 0, previousOffset: null, nextOffset: null } }, '?query=auth&limit=1')

    expect(view.querySelector('article[role="listitem"]')?.textContent).toContain('Gateway owns the auth boundary')
    expect(view.querySelector('a[href="/dashboard/memories/gateway-auth-boundary"]')?.textContent).toBe('Open memory')
    expect(view.textContent).not.toMatch(/export|edit|sync diagnostics|permission/i)
  })
})
